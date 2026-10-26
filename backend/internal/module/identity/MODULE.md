# Identity - `identity`

Owns people: accounts, memberships (a person in a workspace with a role), sessions
(access token + rotating refresh cookie) and the single-use tokens for e-mail verification
and password reset. It decides what a role may do (owner / admin / member) over the
permissions every module declared. It does not own the workspace (Tenant), machine access
(ApiKey) or operators (Manager). Its tables span tenants and are **not** RLS-scoped;
the e-mail is encrypted at rest with a `email_hash` sibling for lookup.

> Budget: 8 KB.

## Surface

- API (`openapi.json`): `POST /api/auth/register` (202),
  `/verify`, `/login`, `/refresh`, `/switch-tenant`, `/logout`, `/forgot-password`,
  `/reset-password`; `GET|PATCH /api/profile`. `/api/auth/*` is rate limited per client.
- Cookie: `open_enu_refresh` - HttpOnly, Secure (when `API_URL` is https), SameSite=Strict,
  `Path=/api/auth`, 30 days, rotated on every use; a replayed token revokes its family.
- Permissions: `identity.member.view`, `identity.member.invite`, `identity.member.manage`,
  `identity.profile.manage`
- Roles: owner → every declared permission; admin → all but `tenant.delete` and
  `api_key.manage`; member → `*.view` and `*.use`. An undeclared permission is nobody's.
- Events: none published; subscribes to `tenant.deleted` (drops memberships)
- Mail: `identity.email.*` keys, links under `APP_URL`
- Tables: `identity_user`, `identity_membership`, `identity_refresh_token`,
  `identity_security_token` (no RLS)
- GDPR: exports account, memberships, sessions; erasure deletes the account
- Console: `openenu user create --email --password --tenant --slug [--role --name --seed]`

## Public contract

`contract.Member` and `contract.Directory` (`MembersOf`, `CountMembersOf`) built by
`contract.NewDirectory` - who belongs to a workspace, for the operator console;
`contract.Operator` and `contract.Minter`, built by `contract.NewMinter` - sessions for a
person another module vouches for:

- `Impersonate` - the one designed way to act as a member without their password
  ([ADR-0020](../../../../.ai/platform/adr/0020-impersonation.md)): a tenant-realm session
  marked `imp`, naming the operator in `act`, with no refresh cookie. Manager decides
  *whether*; this module decides *what a session is*.
- `StartSession(ctx, userID, tenantID)` - an ordinary session for a person a module
  authenticated by other means: single sign-on, a mailed link, credentials
  carried over from a replaced system. It returns a `contract.Started`, opaque, whose
  `Write(w)` answers as a login does - the session body and the refresh cookie - so the
  calling module's response type wraps it in one line. An empty `tenantID` picks the
  person's best workspace; a named one must be enterable. An unknown or inactive person, or a
  workspace they cannot enter, is the login's 401. The caller rate-limits its endpoint and
  answers every failure alike; its page goes in `<InjectionPoint name="auth.login.methods">`
  on `login.vue`, and keeps the answer with `useAuthStore().setSession()`.

The types alias internal's, so the session shape has one definition. The root package also exports
`RefreshCookie`, `CreateUserInput`, `CreateUserResult` and `CreateUser` for the console.

## Notes for agents

- **Sessions are infrastructure, not commands.** Login, refresh, switch and logout write
  tokens in `internal/session.go` over the repository and are not audited as commands; a
  failed login is not a business event. Everything that changes a person IS a command.
- Every credential failure is the same 401 with the same message, in the same time: an
  unknown address spends a verification (`PasswordHasher.Spend`) and the password is checked
  before the account's state. Keep it that way.
- **Registration does not say whether an address has an account**
  ([ADR-0023](../../../../.ai/platform/adr/0023-registration-answers-the-same-for-every-address.md)).
  Every well-formed request answers 202
  `{status: "unverified"}`; a known address creates nothing and its owner is mailed
  (`identity.email.account_exists.*`) with links to log in and to reset the password - or,
  if the account was never verified, a new verification link to its pending workspace. The
  attempt is audited as `identity.user.registered` with `accountExisted: true`.
- A login whose verifier was made at another argon2id cost replaces it with a current one
  in the same statement that stamps `last_login_at`
  ([ADR-0022](../../../../.ai/platform/adr/0022-argon2id-passwords.md)). Not a command: the
  password is the same password.
- No credential is not a failed one: `/refresh` with no cookie at all is an empty 204, not a
  401. Every first visit asks it, and a 401 there is a red console line in a browser that
  did nothing wrong. A cookie that is sent and does not verify is still the 401.
- Register creates the tenant through Tenant's `Provisioner` from inside its own handler. That
  is Tenant's own command, audited as a creation in its own right, and the bus runs it as a
  **savepoint of register's transaction** (`commands-and-audit.md`): the person, the
  membership and the workspace commit together or not at all. `tenant.purge_unverified`
  collects workspaces whose owner never verified, not half-finished registrations.
- The verification, reset and account-exists mails are sent with `tx.AfterCommit`, so no
  mail carries a link to a token a rollback removed.
- A membership is enterable only while its workspace is active: a suspended workspace locks
  its members out on their next login, refresh or switch, with the same 401 as a wrong
  password.

## Depends on

`tenant` (its `contract/`).
