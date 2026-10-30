# Webhook - `webhook`

Owns outbound webhooks, and outbound only: a workspace registers an `https` URL, subscribes
it to event names, and every matching domain event is signed (Standard Webhooks) and
delivered on the jobs queue with five attempts and a log of each. **The log is the
product**: "did you send it, and what did they say?" is the only question anyone asks.
Nothing here knows which events exist - the publisher subscribes to *every* event
(`svc.Outbox.SubscribeAll`), and the subscription is a row in the tenant's table.

> Budget: 8 KB.

## Surface

- API (`openapi.json`): `GET/POST /api/webhooks` (201 carries `secret` **once** and a
  `warning`), `DELETE /api/webhooks/{id}` (deactivates - never deletes; the log points at
  it), `GET /api/webhooks/deliveries?endpointId=` (newest 50),
  `POST /api/webhooks/deliveries/{id}/redeliver` (202: the same row reset and queued again)
- Permissions: `webhook.view` (endpoints and the log), `webhook.manage` (add, deactivate,
  redeliver); create and deactivate are refused under impersonation
- Commands: `webhook.endpoint.created`, `webhook.endpoint.deactivated`,
  `webhook.delivery.redelivered`. Snapshots never hold the secret
- Tables: `webhook_endpoint` (RLS; `secret` encrypted under the **tenant's** key),
  `webhook_delivery` (RLS; `status pending|delivered|failed`, `attempts`,
  `response_code`, `response_body` truncated to 2000)
- Job: `webhook.deliver` (`job.go`) - POST with `webhook-id`, `webhook-timestamp`,
  `webhook-signature: v1,<base64 HMAC-SHA256(secret, "{id}.{ts}.{body}")>`, ten-second
  timeout; 2xx is `delivered`, anything else one more attempt; the fifth marks `failed`
  and raises `webhook.delivery_failed` to the tenant (feed and mail)
- Notification: `webhook.delivery_failed` (`Module.Notifications`), keys
  `notification.webhook.delivery_failed.{title,body}` in this module's catalogue
- Console: none. A delivery is retried by the worker, or by hand through the redeliver route

## Public contract

Nothing under `contract/`, and the root package exports only `Module`. Other modules raise
events; this module decides nothing about them.

## Notes for agents

- `internal/signer.go` is pinned to a fixed test vector in `signer_test.go`: secret
  `whsec_test_secret`, id `msg_01`, timestamp `1700000000`, payload
  `{"event":"example.project.created"}` → `v1,hRXt/SA8J+qcplkcnKGAoxk7bQb4HuEZfSvU0KLXpvo=`.
  A change that alters these bytes breaks verification for every receiver already
  integrated.
- The worker's writes (`succeeded`, `failed`) are infrastructure, outside the bus: the
  outcome of an HTTP call is not a domain change. The error it returns is what makes River
  retry, so the queue and the log count the same five attempts.
- **An endpoint is never inside our own network** (`internal/outbound.go`): at
  registration and before every attempt the URL must be `https`, not `localhost`, not an IP
  literal outside the public internet, not the port of an internal service
  (`webhook.error.address_refused`); the dial resolves the name once, refuses it unless
  EVERY address is public, and connects to the address it checked (no rebinding); no
  redirect is followed and no proxy is used. A refused attempt is logged with a fixed
  sentence, never the address or a body. `WEBHOOK_ALLOW_PRIVATE=true` lifts the address
  rules (not `https`, ports or redirects) for the dev stack's echo receiver; it defaults to
  false and production must never set it.
- `https` only, and the secret is generated server-side (`whsec_` + 32 random bytes): a
  secret the caller chose is one they may have reused.
- `internal/delivery_test.go` sits in `internal` because the deliverer's client must trust
  the test receiver's certificate; the API tests are black-box in `webhook_test.go`.

## Depends on

Nothing. The notifier and the outbox are the kernel's.
