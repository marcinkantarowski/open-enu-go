# Consultant role, and operators and consultants in the console

**Status:** in progress · **Date:** 2026-10-30 · **Decision:** [ADR-0028](../adr/0028-consultant-role.md)

## Why

Someone from outside a client may work in its workspace: a consultant, a DPO. They were an
`admin`, so nothing could tell them from the client's own admin, and an operator could not
see who serves which client, or who the operators are.

## What

- Identity: a fourth role, `consultant`: what an admin holds, and the `.review` permissions,
  which become the consultant's alone. It ranks with admin.
- Identity commands `identity.consultant.assigned` (by address; a person must have an
  account; an existing admin or member is moved to the role; the workspace's owner is
  refused; idempotent) and `identity.consultant.unassigned` (the membership goes, the
  person's refresh tokens into that workspace with it).
- Manager: `GET /api/manager/operators`, `GET /api/manager/consultants`, `POST
  /api/manager/tenants/{id}/consultants`, `DELETE
  /api/manager/tenants/{tenantId}/consultants/{userId}`, and `consultants` on the workspace
  list. Writing needs `ROLE_PLATFORM_SUPER` from the operator's row, as impersonation does:
  it opens a client's data to a person. Each write is on the trail twice, as a suspension is.
- Console: Operators and Consultants pages, a Consultant column, assign and remove on a
  workspace's page.

## Not here

Creating an account from the console (needs working mail): `openenu user create --role
consultant` remains the way. Access tokens already issued run out within fifteen minutes.

## Tests

`manager_test.go` (write, super, trail, refusal, removal), `permissions_test.go`, the security
suite's role sizes, `e2e/tests/consultants.spec.ts`.
