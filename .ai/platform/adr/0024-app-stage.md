# ADR-0024 - One variable says where a checkout runs: `APP_STAGE`

**Status:** accepted · **Date:** 2026-10-22

## Context
A checkout runs on a laptop, on a staging server or in production, and a few things must
differ between them - TLS, who may reach it, whether search engines may index it. When
configuration branches on an environment name, each environment runs a configuration that
exists nowhere else, and the one that matters - production's - is the one exercised least.

## Decision
`APP_STAGE` in `.env` is `local`, `staging` or `production`. It is read by the scripts and
the compose files, and it chooses:

- **what runs**: production is immutable images (`docker/compose.prod.yml`); staging is the
  development stack on a server - hot reload, Mailpit - behind an IP allowlist
  (`docker/compose.dev.yml` + `docker/compose.staging.yml`, `scripts/lib/stage.sh`);
- **the edge's mode**: a local development CA, or Let's Encrypt with security headers and a
  rate limit on a server (`scripts/dev/edge.sh`);
- **indexability**: only production's landing site is indexable; staging answers `noindex`;
- **what the release scripts accept**: `release.sh` and `promote.sh` refuse a checkout that
  is not the stage they expect.

The Go binary has one configuration everywhere. It reads its URLs, DSNs and secrets from
the environment and never branches on the stage; `/health` only reports it.

## Consequences
- Staging is where changes are made and looked at, not a rehearsal of production's
  configuration. What reaches production is exactly the commit staging ran, built into
  images by `make promote` after `make promote-gate` passed on staging
  ([`docs/deployment.md`](../docs/deployment.md)).
- Stage-dependent behaviour is explicit, in a handful of scripts and compose files, never
  inherited configuration.
