# Skill - review a change

The checklist a change must pass before it is proposed. Most of it is executable; the parts
that are not are the parts where judgement is actually required.

---

## 1. Run the gate, and read it

```bash
make ci
```

Not `make check` - that is the inner loop, and it deliberately skips the database tests,
the browser suite and the slower guardrails.

**"The tool produced no output" is not "the check passed."** A check that cannot run must be
reported as broken, never as clean: one that silently could not run reports green forever.

## 2. Scope

- [ ] One module. Touching several is an `Ask First`, not a judgement call.
- [ ] No file changed "while I was there". An unrelated fix is a separate change.
- [ ] Nothing invented that the architecture does not describe
      ([`ARCHITECTURE.md`](../ARCHITECTURE.md)). A capability that does not exist is
      reported as missing, not improvised.
- [ ] If something was deleted, it was deleted rather than deprecated - this repository has
      no users yet, and that is the only time deletion is cheap.

## 3. The things a green build does not prove

- [ ] **Is this the second implementation of something?** `grep .ai/inventory.json`. The
      kernel already owns caching, storage, encryption, search, progress, flags and audit;
      re-implementing one locally is the most expensive mistake available here.
- [ ] **Does a new convention ship with the check that fails when it is broken?** In the
      same change, with a breaking fixture and a `selftest` plant for an analyzer.
- [ ] **Was a guardrail weakened to make this pass?** An exemption, a widened allowlist, a
      relaxed budget. Either fix the change, or argue the rule should go and remove it
      outright - never mute it for one file.
- [ ] **Do the tests assert behaviour or implementation?** A test that restates its fakes
      passes forever and proves nothing. Module tests hit real HTTP and a real database for
      that reason.
- [ ] **Does anything assert only a status code?** Assert content too.
- [ ] **If it writes, is the write read back from the database in a test?** An assertion
      against a struct already in memory passes whether or not any SQL ran.

## 4. Security

- [ ] Every new operation calls `auth.Require` with a declared permission, and the refusals
      are tested.
- [ ] Nothing leaves the tenant's scope without a `scope.Unscoped` carrying a written reason.
- [ ] No encrypted column is queried or indexed; lookups go through a `*_hash` sibling.
- [ ] No secret, key, certificate or `.env` is in the diff.
- [ ] An operator-realm command names its tenant (`command.TenantTargeted`), or it will not
      appear in that customer's trail.
- [ ] A public endpoint answers the same for a known and an unknown account, in the same
      time ([ADR-0023](../adr/0023-registration-answers-the-same-for-every-address.md)).

## 5. The harness kept up

- [ ] `MODULE.md` still describes what is there (`make docs-check` compares it).
- [ ] A new module has a Task Router row.
- [ ] `.ai/inventory.json` regenerated if anything was added or renamed.
- [ ] `backend/openapi.json` and `ui-kit/types/api.d.ts` regenerated if the API changed.
- [ ] A mistake that cost real time became a lesson and a row in its index - written when
      the mistake is *understood*, not when it is made.
- [ ] An architectural decision became an ADR, or changed one.
- [ ] In a project, both went under `.ai/adr/`, `.ai/lessons/` - never into `.ai/platform/`.
      A platform decision is departed from with a project ADR that names it, not by editing it.

## 6. Say what actually happened

If tests fail, say so, with the output. If a step was skipped, say which and why. If
something was left out of scope, name it - the user decides whether to cut it, not the
person doing the work.
