# Production's data on a development machine

Status: done (2026-10-30)

## Why

Working against real data on the development stack: what a real tenant's documents, numbers
and edge cases look like, without touching production.

## What

`make copyproddata` (`scripts/remote/copyproddata.sh`) replaces the dev database with
production's. Production is only read: `scripts/remote/prod-data.sh dump|encrypted` writes
to stdout over SSH and leaves no file on the server. `make db-reset` goes back to an empty
database.

## Decisions

- **The master key stays on production.** `APP_ENCRYPTION_KEY` opens every tenant's data.
  Production decrypts every encrypted value (`openenu encrypted export`) and the dev stack
  seals it with its own key (`openenu encrypted import`, refused unless `APP_STAGE=local`).
  The values travel through a pipe, never a file.
- **Encrypted values are found from the data, not from a list.** The kernel knows no
  module's tables and there is no ORM mapping: `kernel/crypto/columns` looks through every
  text column of the public schema for the exact `v1:iv:ciphertext:tag` shape, so a new
  encrypted column is included without anyone remembering. A value that has the shape and
  does not open fails the run.
- **A sibling `<column>_hash` is recomputed.** It is keyed under the master key too; without
  it nobody could log in to the copy. The value is hashed as stored, which is the module's
  one spelling.
- **The import proves itself.** After writing, in the same transaction, it reads every
  encrypted value back with the local key; one that does not open rolls everything back.
- **Nothing copied may reach the outside.** The script refuses anything but `APP_STAGE=local`,
  production's `DOMAIN`, and a `MAILER_DSN` other than Mailpit. The worker and the scheduler
  stay stopped until the River queue is emptied and every webhook endpoint is switched off.
- **No scrubbing.** The point is the real data, so keep the disk encrypted. Uploaded files
  are not copied.

## Tests

`backend/kernel/crypto/columns/columns_test.go` - per-tenant and global values exported
under one key and imported under another, the hash recomputed, a value for an unknown column
skipped, a value that does not open refused.

## Changelog

- 2026-10-30 - first version.
- 2026-10-30 - `make e2e` moved to its own database, so a copy survives `make ci` and a deploy.
