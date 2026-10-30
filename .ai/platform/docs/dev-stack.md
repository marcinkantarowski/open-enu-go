# The dev stack

`make builddev` brings the whole product up from a clean checkout, over trusted HTTPS, on
six hosts under `DOMAIN`. Everything in it needs Docker; nothing in `make check` does.

```
check-tools   the host toolchain (go, docker, mkcert, openssl)
env           .env from .env.example, secrets generated once, derived env files
certs         mkcert wildcard for *.${DOMAIN}; the CA trusted on Linux and, under WSL2, Windows
hosts         every host -> 127.0.0.1 (asks for sudo; the fallback prints the block)
up            ports free? -> the shared edge -> compose up --build -> the edge joins the network
deps          npm install in the node container (one workspace, one node_modules volume)
wait          healthchecks, then the API's body, then the edge's route to each host
migrate       `openenu migrate` as the schema owner
smoke         fifteen URLs, each asserting a body fragment, over the real certificate
urls          the table
```

## What runs

| Service | Image | Role |
|---|---|---|
| `postgres` | `pgvector/pgvector:pg18` | the database; `docker/postgres/init/01-init.sh` creates the application role. Volume `postgres` at `/var/lib/postgresql` (18 keeps its data in `18/docker` beneath it) |
| `redis` | `redis:8.10-alpine` | the tenant-scoped cache |
| `mercure` | `dunglas/mercure:v1.1` | SSE hub at `api.${DOMAIN}/.well-known/mercure` |
| `api` | `docker/api/Dockerfile` (dev) | `openenu api` under air - rebuilt on every edit |
| `worker` | same image | `openenu worker` - the outbox (River) |
| `scheduler` | same image | `openenu scheduler` - `module.Task`s on a clock |
| `frontend`, `manager`, `landing` | `docker/node/Dockerfile` (dev) | the three Nuxt dev servers, one image, one shared `node_modules` volume |
| `mailpit` | `axllent/mailpit` | every outbound mail - Identity's verification and reset messages, sent through `kernel/mail` over `MAILER_DSN` |
| `webhook-echo`, `e2e` | profile `e2e` | a receiver for deliveries, the Playwright runner |

No Traefik in the file: routing and TLS belong to the shared edge (`docker/edge/`,
`scripts/dev/edge.sh`) - one container for every project on the Docker host. Each router is
named `${STACK}-<service>` and names the network the edge reaches it on. The edge is two
containers: `enu-edge` (Traefik) and `enu-edge-socket-proxy`, the only one that mounts the
Docker socket, which answers Traefik's read-only container listing and refuses the rest of
the API. `make edge` recreates an edge whose definition changed (compose's config hash),
so a host with an edge from before the proxy gets one on its next `make up`.

## Where the environment comes from

`.env` is the only file edited by hand. The Go binary has **no env file**: compose composes
its environment from `.env` in the `x-backend-env` anchor of `docker/compose.dev.yml`, once,
for `api`, `worker` and `scheduler` alike. The variables after `# == Project variables ==`
in `.env.example` reach the three through `docker/api/project.env`, written by `make env`.
The Nuxt apps and the e2e runner get `frontend/.env`, `manager/.env`, `landing/.env`,
`e2e/.env` - derived, gitignored, never edited. Adding a variable, and the rules `make env`
keeps: [`env-vars.md`](env-vars.md).

| Variable (in the container) | From | Read by |
|---|---|---|
| `DATABASE_URL` | `${DB_USER}_app` + `DB_APP_PASSWORD` | `openenu api/worker/scheduler` - the role that cannot bypass RLS |
| `DATABASE_ADMIN_URL` | `DB_USER` + `DB_PASSWORD` | `openenu migrate` only |
| `REDIS_ADDR`, `REDIS_PASSWORD` | compose, `.env` | `cache.NewRedis` |
| `APP_SECRET` | `.env` | `auth.Signer`, storage URLs; refused under 32 characters |
| `APP_ENCRYPTION_KEY` | `.env` | `crypto.NewDerivedKeys` |
| `MERCURE_URL`, `MERCURE_PUBLISHER_JWT_SECRET` | compose, `.env` | `realtime.Hub` (publishing); the hub's `MERCURE_PUBLISHER_JWT_KEY` |
| `MERCURE_JWT_SECRET` | `MERCURE_SUBSCRIBER_JWT_SECRET` in `.env` | `realtime.Tokens` (the subscriber cookie); the hub's `MERCURE_SUBSCRIBER_JWT_KEY`. A separate key, so the one every browser's cookie is signed with cannot publish |
| `WEBHOOK_ALLOW_PRIVATE` | `"true"` in `compose.dev.yml` only | the Webhook module: lets a delivery reach a private address, which is where `webhook-echo` lives. Never in production - `make prod-check` refuses it |
| `HEARTBEAT_FILE` | `/tmp/openenu-heartbeat` in `compose.prod.yml` | `openenu worker/scheduler` touch it; their healthcheck is `openenu health -heartbeat` |
| `API_URL`, `APP_URL`, `MANAGER_URL`, `SITE_URL` | `https://…${DOMAIN}` | signed URLs; the CORS allowlist (exact origins) |
| `STORAGE_ROOT` | the `storage` volume | `storage.Local` |
| `STORAGE_MAX_UPLOAD_MB` | `.env`, default 25 | `platform.Services.MaxUploadBytes`, the Attachment upload limit |
| `SSL_CERT_FILE` | `docker/certs/rootCA.pem` | Go's TLS roots, so the backend can call `https://*.${DOMAIN}` |
| `API_ADDR` | `:8080` | the listen address |
| `SCHEDULER_TICK_SECONDS`, `SCHEDULER_INTERVAL_SECONDS` | defaults 60 / 86400 | `openenu scheduler` |

Two values the Nuxt apps get are decided by the stage, not only by `.env`:
`NUXT_PUBLIC_INDEXABLE` is `true` on production alone, and `NUXT_PUBLIC_DEMO_CONTENT` (the
`Example` entry in the tenant app's menu) is `DEMO_CONTENT` from `.env` - `false` unless set -
and is written `false` on production whatever `.env` says (`scripts/lib/envgen.sh`).

## Hot reload

The backend containers mount `backend/` and run `air -c .air.toml -- <command>`: a change to
a `.go`, `.sql`, `.json`, `.yaml` or `.toml` file rebuilds into `/tmp` and restarts the process with
SIGTERM first, so the worker drains the job in hand. air polls, because file events do not
cross a bind mount reliably on WSL2 or macOS. The module cache and the build cache are named
volumes shared by the three containers, so one edit compiles each package once. The first
build after `make builddev` is the slow one; `make logs-api` shows it.

## Realtime, through a browser

`POST /api/realtime/token` (kernel, `realtime.Tokens`) sets the `httpOnly`
`__Secure-mercure_access_token` cookie, scoped to `/.well-known/mercure` on the API's host. It
is a Mercure 1.x access token - `typ: at+jwt`, `iss` the API's URL, `aud` the hub's public URL,
an hour's `exp` - whose one `subscribe` authorization detail names the caller's tenant topics
and nothing else: the exact topic `/tenants/<id>/events` and the URL Pattern
`/tenants/<id>/progress/:id`. The publisher's token (`realtime.Hub`) has the same `iss` and
`aud` and grants `publish` on `*`. The hub trusts that pair through `MERCURE_TRUSTED_ISSUERS`
and a pinned `resource_identifier` (compose): pinned, because the API publishes to
`http://mercure` inside the network while the browser subscribes through the edge, and a
per-request audience would differ between the two. Relative topics and patterns resolve
against it.

`useMercure` in ui-kit calls the endpoint, then opens an `EventSource` with credentials and
`match=<topic>` (1.x's exact matcher; the hub answers `topic=` with a 400). Every update the
Broadcaster publishes is private, so a connection without that cookie receives nothing.

The stream belongs to the tab. The signed-in layout `hold()`s it while it is mounted and
every `subscribe()` wants it until it stops; with neither, it closes and its status is
`idle`, which `RealtimeIndicator` does not show. A dropped connection is EventSource's own
retry (`reconnecting`). A refusal - the hub down behind Traefik, or the cookie expired after
its hour - is one EventSource never retries, so `useMercure` fetches a new token and opens a
new stream, backing off from one second to thirty. A 401 or 403 from the token endpoint ends
the retries: no session, or no workspace, is not fixed by asking again.
`e2e/tests/realtime.spec.ts` holds the first rule.

## Browser tests

`make e2e` runs Playwright in the `e2e` profile against this stack; `SPEC=tests/x.spec.ts`
narrows it. `make e2e-fixtures` (run by `make e2e`) creates the accounts the suite signs in
as through `openenu user create` and the operator through `openenu manager create`,
idempotently. The specs are in `e2e/tests/`; `make e2e` is part of `make ci`
([`testing.md`](testing.md)).

## When something is wrong

- `make status`, then `make logs-api` - air's build errors are there, not in the browser.
- `make smoke` says which host fails and what the body lacked.
- The edge answers `404 page not found` for a host: `make up` re-attaches it; a Docker Desktop
  restart replaces the socket the proxy mounted, and `make edge` notices and restarts both.
- A port is taken: `make ports` names the holder and the `.env` variable to change.

Specific failures, symptom first: [`troubleshooting.md`](troubleshooting.md).
