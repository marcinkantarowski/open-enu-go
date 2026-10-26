# ADR-0010 - `Membership(user, tenant, role)` from day one

**Status:** accepted · **Date:** 2026-10-22

## Context
One tenant per user is simpler, and it is what a `tenant_id` on the user table gives you.
Every real SaaS then needs a person in two tenants within a year - agencies, consultants,
your own staff - and retrofitting membership touches every query and every token.

## Decision
A person and a workspace are joined by `identity_membership(user_id, tenant_id, role)` from
the start. The access token carries the **current** tenant (`tid`) and the permissions of
that membership's role (`perms`). `POST /api/auth/switch-tenant` spends the refresh token
and issues a session for another workspace the person belongs to - 401 when they do not.
Roles are per membership, not per person.

## Consequences
- One extra join and a workspace switcher in the UI, forever.
- The expensive migration never happens.
- "Current tenant" is a token concern, which the row-level security of ADR-0009 already
  reads.
