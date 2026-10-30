# Manager - `manager`

Owns the platform-operator realm: a separate identity **table**, a separate token audience
(`aud: manager`), a separate app. Operators list and inspect every workspace, suspend one,
read the audit trail across all of them, watch the job queues, and impersonate a member
through the one designed crossing between realms ([ADR-0020](../../../../.ai/platform/adr/0020-impersonation.md)). It is the top of the dependency
graph - which is where the authority to read across tenants belongs - and reaches other
modules only through their `contract/` packages.

> Budget: 8 KB.

## Surface

- API (`openapi.json`): `POST /api/manager/login` (five attempts, then one a minute; no
  refresh, no reset, no registration), `GET /api/manager/tenants`, `GET /api/manager/tenants/{id}`,
  `POST /api/manager/tenants/{id}/suspend`, `POST /api/manager/tenants/{tenantId}/users/{userId}/impersonate`,
  `GET /api/manager/audit`, `GET /api/manager/workers`, `GET /api/manager/operators`,
  `GET /api/manager/consultants`, `POST /api/manager/tenants/{id}/consultants`,
  `DELETE /api/manager/tenants/{tenantId}/consultants/{userId}`
- Permissions: **none**. Operator authority is the realm (`auth.RequireOperator`), not a
  dotted permission - no tenant role can ever satisfy an operator check.
- Commands: `manager.tenant.suspended` (then Tenant's own `tenant.suspended`, then
  Identity's `identity.sessions.ended` - the members' refresh tokens into it are revoked),
  `manager.impersonation.started` (recorded BEFORE the token exists; refused with 403
  `manager.error.super_required` unless the operator's row holds `ROLE_PLATFORM_SUPER`),
  `manager.consultant.assigned` and `.unassigned` (the same refusal, then Identity's own
  command; [ADR-0028](../../../../.ai/platform/adr/0028-consultant-role.md))
- Tables: `platform_manager` (no RLS; e-mail and display name encrypted, `email_hash` lookup)
- Console: `openenu manager create --email --password [--name --super]` - the only way the
  first operator comes to exist; there is no signup endpoint and there should never be one

## Public contract

Nothing under `contract/`. The root package exports `CreateOperatorInput` and `CreateOperator`
for the console and the e2e fixtures.

## Notes for agents

- An impersonation token is a genuine `aud: tenant` token with `imp: true` and `act: {sub,
  realm: manager}`: it passes the realm check honestly rather than being an exception to it.
  No cookie is issued, so it expires in fifteen minutes and cannot renew itself.
  `auth.Guard.DenyImpersonated` refuses it on destructive actions and logs both identities.
- The console hands the session to the tenant app in the URL **fragment**
  (`/impersonate#...`), never the query string: fragments reach no server and no log.
- A suspended workspace locks its members out on their next login, refresh or switch
  (Identity's `enterable`), and its API keys stop authenticating (ApiKey's resolver).
- Login is one 401 in one time for every failure, as Identity's: an unknown address spends a
  verification, and a verifier from an older algorithm or cost is replaced on a successful
  login ([ADR-0022](../../../../.ai/platform/adr/0022-argon2id-passwords.md)).

## Depends on

`tenant`, `identity` (`contract.Directory`, `contract.Minter`, `contract.Sessions`, `contract.Consultants`), `audit` (`contract.Reader`).
