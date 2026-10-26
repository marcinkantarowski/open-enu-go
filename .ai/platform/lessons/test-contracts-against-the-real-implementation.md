---
tags: [testing, cache, mocks, go-test]
date: 2026-10-25
---
# Test a contract against the real implementation, not a mock of it

A mock accepts any value. Against one, a test verifies that the code *calls* its collaborator
with the value it intended - which was never in doubt - and cannot verify the one thing that
matters: that a real implementation accepts it and keeps its promises. The mock tests the code
against itself.

## The trap
The tenant cache wrapper builds keys and tags from a tenant id, a module name and a caller's
string. Tested against a mock `cache.Cache` that records calls, it passes: `Set` is called with
exactly the key the wrapper built. Whether two tenants' keys can collide, whether a tag built
from a caller's string stays one plain segment of a key or splits into two, whether
invalidating one tenant drops another's entry - none of it is visible, because the mock holds
no entries. The failure surfaces the first time a real request crosses the boundary, as a
wrong answer or a 500 far from the key that caused it.

A second trap hides in the same test: two tenants must share **one** backend for an isolation
assertion to mean anything. With a backend each, the test passes while proving nothing.

## The rule
**When a collaborator imposes constraints on the values you pass it, test against a real
implementation of that collaborator.** For anything with an in-memory implementation there is
no excuse: it costs nothing and enforces the rules the production backend does.

Mock what is slow, remote or nondeterministic. Do not mock something whose *rules* are the
thing under test.

## Where it is held
- `platform.InMemory` is a complete platform with no backing service - the memory cache and
  storage, recorders for progress, events, audit, notifications and mail, static flags, an
  encryptor under a test key - and each part enforces the real constraints: the memory cache
  still prefixes keys by tenant, the memory storage still refuses another tenant's key. A
  module test mounts the real module on it, and `platform.WithDatabase` adds a testcontainers
  Postgres (`testdb.Postgres`).
- `backend/kernel/cache/cache_test.go` runs three tenants (`a`, `b` and global) through **one**
  `cache.Memory`, and asserts three entries and that invalidating `a` drops only `a`'s;
  `cache/redis_test.go` runs the same scenario against a real Redis. `SanitiseTag` turns
  `{}()/\@:` into `.`, and the test asserts it.
- `backend/kernel/storage/storage_test.go` runs one scenario over every backend (`Memory`, and
  `Local` on `t.TempDir()`), including a file named `../Quarterly Report (final).PDF`.
- `.ai/platform/docs/testing.md` ("What `InMemory` gives you"); `AGENTS.md` (Never list).

## Where else this applies
- Storage keys, which the storage backend constrains.
- Job kinds and queue names, which the queue constrains.
- Header and cookie names and values, which the HTTP layer constrains:
  `backend/kernel/mail/header_test.go` builds a real message to prove a line break in a subject
  or an address cannot add a header.
- Same family as [[an-encryption-key-must-belong-to-the-row]]: both pass every unit test and
  fail the first time a real request crosses the boundary.
