# ADR-0025 - Deploy over SSH, building on the server

**Status:** accepted · **Date:** 2026-10-22

## Context
A new project has a domain and a bare VPS, and often no container registry yet. A
registry-first pipeline blocks the first deploy on infrastructure that does not exist.

## Decision
`make buildprod HOST=... DOMAIN=... REPO=... EMAIL=...` (and `make buildstaging`) runs a
read-only preflight, then over SSH: installs Docker, creates the deploy user in an order
that cannot lock the operator out, pulls the repository with a deploy key, generates the
secrets that are missing - and never regenerates one that exists - builds the images **on
the server**, backs up, migrates with the freshly built binary (`openenu migrate`),
health-gates through the edge, and installs the systemd units. `SYNC=rsync` pushes the
working tree instead, for day one when there is no git remote.

DNS changes and deploy-key registration are separate, deliberate commands (`make dns-apply`,
`make deploy-key`): a build diagnoses and prints the fix, it never mutates a zone or
registers a credential as a side effect.

## Consequences
- One command from a bare VPS to a live product, with no prerequisite beyond SSH and DNS.
- Server-side builds are slower and need disk; acceptable on a single host.
- A registry-based path stays possible later without changing the compose files.
- Not zero-downtime: recreating a single-replica service takes seconds
  ([`docs/deployment.md`](../docs/deployment.md)).
