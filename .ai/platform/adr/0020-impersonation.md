# ADR-0020 - Impersonation is a specified crossing, not a convenience

**Status:** accepted · **Date:** 2026-10-22

## Context
"Support can log in as a customer" is always requested, and usually implemented as "sign a
normal token for them" - which the audit log cannot tell apart from an account takeover.

## Decision
`POST /api/manager/tenants/{id}/users/{uid}/impersonate` first records the attempt on the
trail (`manager.impersonation.started`), so a refused one is on record too; then Identity's
`contract.Minter` issues a genuine tenant-realm session that is:

- fifteen minutes long and **not renewable** - no refresh cookie;
- marked: `imp: true` and `act: {sub: <operator id>, realm: manager}` in the token,
  `impersonating` and the operator in the session body.

Every command under it is recorded with the person as actor and the operator as
on-behalf-of. `auth.Guard.DenyImpersonated` refuses the actions that must stay the
person's own - creating or revoking API keys, webhook endpoints, `Example`'s rename and
archive - and records the refusal with both identities. The console hands the session to the
tenant app in the URL fragment, and the app shows a persistent banner with a way out.

## Consequences
- Support can look; nobody can quietly become a customer.
- A guarded action calls the guard as it is written - `Example` demonstrates it.
- `internal/security/realm_test.go` proves the token is marked, guarded and cannot renew.
