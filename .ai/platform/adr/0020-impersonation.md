# ADR-0020 - Impersonation is a specified crossing, not a convenience

**Status:** accepted · **Date:** 2026-10-22

## Context
"Support can log in as a customer" is always requested, and usually implemented as "sign a
normal token for them" - which the audit log cannot tell apart from an account takeover.

## Decision
`POST /api/manager/tenants/{id}/users/{uid}/impersonate` is open to a **super operator**
only (`ROLE_PLATFORM_SUPER`, read from the operator's row, not their token; `openenu manager
create --super`). It first records the attempt on the trail (`manager.impersonation.started`), so a refused one is on record too; then Identity's
`contract.Minter` issues a genuine tenant-realm session that is:

- fifteen minutes long and **not renewable** - no refresh cookie;
- marked: `imp: true` and `act: {sub: <operator id>, realm: manager}` in the token,
  `impersonating` and the operator in the session body.

Every command under it is recorded with the person as actor and the operator as
on-behalf-of. `auth.Guard.DenyImpersonated` refuses the actions that must stay the
person's own - creating or revoking API keys, webhook endpoints, the person's profile, an
upload, a workspace setting, marking the feed read, `Example`'s create, rename and archive - and
records the refusal with both identities. Support may look; support may not act. Every
controller that dispatches a command or writes calls the guard, unless the route is one an
impersonated session is meant to use (2026-10-30 review: the guard was missing on four
routes, so the rule is now the default and a route without it is the exception to justify). The console hands the session to the
tenant app in the URL fragment, and the app shows a persistent banner with a way out.

## Consequences
- Support can look; nobody can quietly become a customer, and a plain operator cannot
  become one at all. A plain operator refused is on the trail as a failed
  `manager.impersonation.started`.
- A guarded action calls the guard as it is written - `Example` demonstrates it.
- `internal/security/realm_test.go` proves the token is marked, guarded and cannot renew.
