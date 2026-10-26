# ADR-0026 - Backups are off-box, encrypted, and restore-tested

**Status:** accepted · **Date:** 2026-10-22

## Context
A nightly `pg_dump` onto the disk that holds the database is the default advice and is not a
backup: one disk failure takes both. A backup that has never been restored is equally a hope
rather than a guarantee.

## Decision
Nightly: `pg_dump -Fc`, encrypted with `age`, copied by `rclone` to any S3-compatible
`BACKUP_REMOTE`. `.env` goes up as a second encrypted bundle, because a database alone does
not bring a dead server back: without `APP_SECRET` every session breaks, and without
`APP_ENCRYPTION_KEY` every encrypted column is lost (ADR-0016). Seven days of nightly dumps
and bundles are kept on the remote; pre-deploy and manual dumps are not pruned.

- The private half of the age key never lives on the server: a backup that whoever holds
  the server can decrypt protects against disk failure and nothing else.
- A run that cannot reach the remote, or would send a dump in the clear, exits 1, so the
  nightly unit shows failed instead of logging a warning nobody reads.
- `make backup-verify` restores the latest **remote** dump into a scratch container of the
  same Postgres version and checks it.
- **`make buildprod` refuses to finish without a reachable `BACKUP_REMOTE`.** Staging is
  not required to have one - its data is disposable - and warns instead.

## Consequences
- Production cannot be stood up one command away from having no backups.
- The server needs `rclone`, `age` and a bucket; `buildprod` installs the first two and
  checks the third.
- The age private key is the operator's responsibility and must be stored off the server.
