# ADR-0011 - No hierarchical organisations in v1; the door stays open

**Status:** accepted · **Date:** 2026-10-22

## Context
Scoping by tenant **and** organisation supports trees inside a tenant - workspaces,
departments, franchises. Genuinely useful, and it doubles the scoping surface: every query,
every token, every permission check, every test.

## Decision
v1 scopes by tenant only.

The door stays open because scoping lives in one place. Modules carry no tenant predicates
in their queries; the kernel's `scope` package sets the transaction's scope and
`scope.Policy` writes every table's policy (ADR-0009). Adding `organization_id` later is a
second transaction-local setting beside `app.tenant_id`, a claim in the token, a column and
a predicate in the policy of the tables that need it - not a rewrite of the modules.

## Consequences
- v1 stays comprehensible; the common case costs nothing.
- The change, when it comes, is concentrated in the kernel's `scope` and `auth` packages
  and the migrations of the tables involved.
- Revisit when a real project needs workspaces inside a tenant.
