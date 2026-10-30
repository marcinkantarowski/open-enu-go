# ADR-0028 - A consultant is a role in the client's workspace, assigned by an operator

**Status:** accepted · **Date:** 2026-10-30

## Context
A product often has someone from outside a client working in its workspace: an adviser who
prepares and checks the client's work. As an `admin` they could not be told from the
client's own admin, so nothing could say who serves whom, and a check meant to come from
outside (`.review`) was held by the client too.

## Decision
A fourth role, `consultant`, a membership like the others: what an admin holds, plus every
`<name>.review`, which nobody else holds. A check by the client's own people is no check.

An operator assigns and removes one on the workspace. It stays a membership and not an
operator's power, because the consultant works as a person in the tenant realm, with no
crossing (ADR-0019, ADR-0020). The write needs `ROLE_PLATFORM_SUPER`, as impersonation does:
both open a client's data to a person.

## Considered and rejected
- **Telling consultants by their address's domain**: says nothing about permissions, and
  fails for the first consultant from elsewhere.
- **Consultants working through impersonation**: fifteen minutes, read for support, and a
  trail of exceptions for what is daily work.

## Consequences
- A product with no consultants has no one who can pass a `.review`; it declares none.
- A person needs an account before being assigned.
