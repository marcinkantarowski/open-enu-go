# ADR-0019 - Realms are isolated by the token's audience, not by which route received it

**Status:** accepted · **Date:** 2026-10-22

## Context
Tenant users and platform operators are two realms, and their tokens are signed with the
same key. If the realm were decided by which part of the API received a token - and the
person then looked up by email - a tenant user whose address matches an operator's would
authenticate as that operator.

## Decision
Every token names its realm in `aud`: `tenant` for people in a workspace, `manager` for
operators (`auth.RealmTenant`, `auth.RealmManager`). The tenant API's `auth.Require` refuses
any other realm with the same 401 as no token; the operator console's routes require
`auth.RequireOperator`. Operators are a separate table (`platform_manager`, the Manager
module), not a role on a person.

API keys are not tokens of a realm of their own: an `sk_...` key is resolved by its hash to
a principal in the tenant realm, with the key's scopes as its permissions.

`internal/security/realm_test.go` proves each crossing from both sides, including a tenant
account deliberately sharing an operator's address.

## Consequences
- Crossing a realm requires forging a signed claim, not guessing an address.
- Impersonation, the one legitimate crossing, is explicit (ADR-0020).
