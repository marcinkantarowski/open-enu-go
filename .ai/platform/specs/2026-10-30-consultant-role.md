# Consultant role, and operators and consultants in the console

**Status:** in progress · **Date:** 2026-10-30 · **Decision:** [ADR-0028](../adr/0028-consultant-role.md)

## Why

Someone from outside a client may work in its workspace: a consultant, a DPO. They were an
`admin`, so nothing could tell them from the client's own admin, and an operator could not
see who serves which client, or who the operators are.

## What

- Identity: a fourth role, `consultant`: what an admin holds, and the `.review` permissions,
  which become the consultant's alone. It ranks with admin.
- Identity commands `identity.member.added` (by address and role; a new person gets an
  account with a password nobody knows and a single-use link to set one, valid seven days,
  answered once and kept nowhere; an existing admin or member can be moved to consultant, any
  other change of role is refused; the workspace's owner cannot become its consultant) and
  `identity.consultant.unassigned` (the membership goes, the person's refresh tokens into
  that workspace with it).
- Manager: `GET /api/manager/operators`, `GET /api/manager/consultants`, `POST
  /api/manager/tenants` (any operator; active at once), `POST
  /api/manager/tenants/{id}/members`, `DELETE
  /api/manager/tenants/{tenantId}/consultants/{userId}`, and `consultants` on the workspace
  list. Adding a person and removing a consultant need `ROLE_PLATFORM_SUPER` from the
  operator's row, as impersonation does: they open a client's data to a person. Each write
  is on the trail twice, as a suspension is.
- Console: Operators and Consultants pages, a Consultant column, a client created on the
  list, a person added (with the link shown once) and a consultant removed on a client's page.

## Not here

Sending the link by mail, changing a role other than to consultant, removing other members.
Access tokens already issued run out within fifteen minutes.

## Tests

`manager_test.go` (a client, a person and their link, super, trail, refusal, removal), `permissions_test.go`, the security
suite's role sizes, `e2e/tests/consultants.spec.ts`.
