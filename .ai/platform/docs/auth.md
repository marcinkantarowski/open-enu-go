# Authentication

Who is asking. `kernel/auth` verifies a credential, puts a `Principal` on the context and
enters its tenant's scope; everything after that - permissions, the voter, the audit actor -
reads the principal. Issuing credentials (login, refresh cookies, API-key creation) is the
Identity and ApiKey modules' job; the kernel only verifies. Identity hands the kernel a
`crypto.PasswordHasher` - argon2id, stored in the PHC string format
(`$argon2id$v=19$m=65536,t=4,p=1$<salt>$<hash>`), the cost carried in the verifier so it can
change; `NeedsRehash` replaces a verifier written at another cost on the next login
([ADR-0022](../adr/0022-argon2id-passwords.md)). `/api/auth/*` sits behind
`httpx.RateLimit`; ApiKey is the `auth.KeyResolver` the middleware consults (`Module.Keys`).

## Two credentials, one principal

| On the wire | Verified by | Becomes |
|---|---|---|
| `Authorization: Bearer <jwt>` | `auth.Signer` - HS256 under `APP_SECRET`; claims `sub`, `tid`, `aud`, `perms`, `iat`, `exp`, and `imp` + `act` for an impersonation | `Principal{Subject, TenantID, Realm, Permissions, Impersonator}` |
| `Authorization: Bearer sk_...` | `auth.KeyResolver` by the key's SHA-256 - the ApiKey module's table; `MemoryKeys` in tests | the same, with `APIKey: true` and the key's scopes as permissions |

An API key is an opaque `sk_...` string, never a JWT: the server stores only its hash and
resolves the principal from that. A missing header passes through anonymous; a bad one is
401 before any handler. `exp` is required, `alg` must be `HS256`, and an `imp` without `act`
is malformed. An unconfigured signer (secret shorter than 32 bytes) verifies nothing.

## Sessions in the browser

Login answers with a 15-minute access token, which the apps keep in memory only, and sets
the refresh cookie `open_enu_refresh`: `HttpOnly`, `Secure` whenever the API is served over
TLS, `SameSite=Strict`, `Path=/api/auth`, 30 days. `POST /api/auth/refresh` consumes it and
sets the next one in the same family; a token presented twice is a replay and revokes the
whole family ([ADR-0018](../adr/0018-token-transport.md)). Operators get no refresh cookie.

## In a controller

```go
if _, err := auth.Require(ctx, "example.manage"); err != nil { return nil, err } // 401 or 403
if err := a.guard.DenyImpersonated(ctx, "example.project.renamed"); err != nil { return nil, err }
```

`Require` is the permission check: 401 with no principal, 403 without the permission. The
permission says what this *role* may do; whether *this row* may be touched is the voter's,
in the handler (`commands-and-audit.md`).

`Guard.DenyImpersonated` refuses an impersonated session on a destructive action
([ADR-0020](../adr/0020-impersonation.md)): support may look, support may not act in
someone else's name. It records the refusal in the trail with **both** identities - the
subject as actor, the operator as on-behalf-of - because "what did the operator try while
viewing as this user" is the question an incident asks.

## The audit actor

`auth.Actor{}` is the `audit.ActorProvider` the process uses: the principal's subject acts,
its impersonator is on-behalf-of. `platform.InMemory` defaults to nobody; a test that
asserts on actors sets `svc.Actor = auth.Actor{}`.

## Realms

`aud` is the realm: `tenant` for app users, `manager` for operators (`auth.RealmTenant`,
`auth.RealmManager`; [ADR-0019](../adr/0019-realm-isolation-by-audience.md)). One key
signs both, so the realm check is the whole of the isolation: `auth.Require` refuses a
principal whose realm is not `tenant` with the same 401 as no token, and
`auth.RequireOperator` is the manager realm's check. Operators are a separate table
(`platform_manager`, the Manager module), not a role on a user; their token carries their
roles in `perms` for display, but authority is the realm. Logging in there is limited to five
attempts, then one a minute, and issues no refresh cookie.

An impersonation token (`POST /api/manager/tenants/{t}/users/{u}/impersonate`) is a genuine
`aud: tenant` token with `imp: true` and `act: {sub: operator, realm: manager}`, minted by
Identity's `contract.Minter` after Manager has audited the attempt - so a refused attempt is
on record too. It passes the realm check honestly, has no cookie and so cannot renew, and
`Guard.DenyImpersonated` refuses it on destructive actions. `internal/security/realm_test.go`
proves each crossing from both sides.

## Other ways to sign in

Identity checks an email and a password. A project may know its people by other means - a
single sign-on provider, a link sent by mail, credentials held by a system it replaced. The
module that checks those decides only *who* the person is, then calls
Identity's `contract.Minter.StartSession(ctx, userID, tenantID)` and answers with the
`Started` it returns: the login's own body and refresh cookie, from the same code, so the
browser needs no second path and the session renews like any other. That module owns
refusing guesses - it rate-limits its endpoint and answers every failure alike, because
Identity never sees the credential.

In the browser, `login.vue` renders `<InjectionPoint name="auth.login.methods">` below its
form. The module registers a link to its own page there (`injections.ts`), and that page keeps
the answer with `useAuthStore().setSession()`
([`frontend-conventions.md`](frontend-conventions.md)).

## Where it is proved

`kernel/auth/auth_test.go`: round trip, tampering, another secret, `alg: none`, expiry,
`imp` without `act`, the unconfigured signer; both credentials through the middleware; the
scope entered; `Require` on both realms and the guard with its trail entry. The Example suite runs every
route as a person, as a key scoped to `example.view` and as nobody.
