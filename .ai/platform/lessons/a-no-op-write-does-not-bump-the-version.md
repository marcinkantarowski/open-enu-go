---
tags: [pgx, optimistic-locking, testing, e2e]
date: 2026-10-25
---
# A write that changes nothing may not bump the version

Whether a write that changes nothing consumes a version is the implementation's choice, and a
test that asserts on a version must not depend on it. **Change the value you write.**

## The trap
The conflict test in `e2e/tests/example.spec.ts` puts two people on the same project: Alice
renames it, then Bob saves against the version both of them read. Bob's save must come back
409 with both versions and the saved record.

With a constant - `Renamed by Alice` - the first run leaves the record holding that name, and
every later run renames it to the name it already has. Whether that rename moves the version
depends on code the test does not see:

- `example_project`'s SQL bumps it unconditionally
  (`SET name = $1, version = version + 1 WHERE id = $2 AND version = $3`), so the version
  moves and the conflict happens.
- An ORM that computes a changeset, an `UPDATE ... WHERE name IS DISTINCT FROM $1`, or a
  handler that returns early when nothing changed writes nothing. The version stays, Bob's
  "stale" `If-Match` is not stale, and his save is accepted.

In the second case the test passes on a clean database and fails on the second run, which
reports as flakiness - the easiest signal in the world to re-run and ignore. It shows only
when the scenario is replayed with the request headers and status codes printed: the same
`If-Match` answered 200 twice in one run is not something a screenshot shows.

## The rule
**A test that asserts on a version must change the value it writes.** A constant in a fixture
is a value that is already there on the second run, and a no-op write is indistinguishable from
a successful one from the client's side.

## Where it is held
`e2e/tests/example.spec.ts` builds both names with `unique()` (`${prefix} ${Date.now()}`), and
the comment beside it says why, because the next person to write a locking test will reach for
a constant too.

## Where else this applies
- Any test of `If-Match` / `ETag` behaviour, and any test that asserts "the second writer is
  refused".
- Seeded data generally: `setup.TenantSetup` requires `SeedExamples` to be idempotent, which
  means a re-seed writes the same values - so a test built on seeded values inherits this
  problem.
- In production terms: where a write skips a no-op, a client that saves an unchanged record
  does not consume a version, so "the version went up" is not a reliable way to detect that
  someone saved.
