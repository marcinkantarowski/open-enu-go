---
tags: [guardrails, go-analysis, architecture]
date: 2026-10-25
---
# A guardrail firing on legitimate code is a decision point, not a bug

A new rule catches something legitimate on its first run. How that is answered decides whether
the rule keeps meaning anything.

## The trap
The analyzers in `backend/kernel/analysis/` match on shape - a call, a type, a file name - and
correct code has the shape of the mistake often enough:

- `rawsql` ("pgx `Exec`/`Query`/`QueryRow` only in `repository.go`") meets
  `internal/module/example/project_api_test.go`, which reads `client_reference` straight from
  the table to prove the column holds ciphertext.
- `textlen` ("`len()` of a string is not compared with a limit") meets `len(name) == 0`.
- `writeboundary` ("`*scope.Tx` only in `handler.go`, `repository.go` and `job.go`") meets a
  test that builds the rows it asserts on.

Three responses are available, two of them bad:

1. **Get past it at the site.** `go/analysis` has no per-site ignore comment, and the rules
   key on file names, so the cheap way past one is to move the code into a file the rule
   trusts - a query in a file renamed `repository.go`. Invisible in review, and it carries no
   reason, so the next person cannot tell a considered exception from a shortcut.
2. **Weaken the rule** - for instance allow SQL in any file under `internal/`. That deletes
   the guarantee to fix one test.
3. **Exempt the category, in the rule, with the reasoning written down.** Correct.

## The exemptions, and why each is justified

| Rule | Exempt | Why |
|---|---|---|
| `rawsql` | `_test.go` files may run SQL | a test proves what the RLS policy or the encryption did by reading the table itself; the repository is what would filter or decrypt, so refusing raw SQL there makes both guarantees unverifiable. The literal-reason rule for `scope.Unscoped` still applies |
| `textlen` | a comparison with zero | against zero, `len()` asks "is it empty?", and bytes and characters agree; a limit that really is in bytes says so with `len([]byte(s))` |
| `writeboundary` | `_test.go` files | a test builds the state it asserts on; forcing that through the bus tests the bus, not the behaviour |
| `thincontroller` | reads through `repository.go` | a controller reads through it for a GET, and `writeboundary` and `rawsql` already keep the transaction and the SQL out of `api.go` |
| `rawsql`, `platformservice` | the kernel | the kernel implements the contracts the rules send modules to |

## The rule
When a guardrail fires on code you believe is correct, the question is never "how do I get
past this?" - it is **"is this a category the rule should never have covered, or am I about
to do the thing the rule exists to prevent?"**

Answer it in the rule, in a comment, once. Then it applies to everyone, and the reasoning
survives.

## Where it is held
- Each exemption is a comment beside the condition that grants it, in the analyzer's own file
  (`rawsql/rawsql.go`, `textlen/textlen.go`, `writeboundary/writeboundary.go`,
  `thincontroller/thincontroller.go`).
- The fixtures prove both sides: `rawsql/testdata/.../proof_test.go` runs a query and an
  `Unscoped` with a reason without a finding and still expects one for an empty reason, and
  its `handler.go` expects the SQL to be refused; `textlen/testdata/.../api.go` accepts
  `len(name) == 0` and expects `len(name) > maxName` to be refused.
- `make selftest` plants one violation per analyzer in a real scaffolded module and expects
  `make arch` to fail, and checks the clean copy passes before and after.
- `.ai/platform/docs/guardrails.md` ("Adding a rule"); `AGENTS.md`: never weaken a guardrail
  to make a change pass.

## Where else this applies
- Every rule added from here: expect it to catch something legitimate on its first run.
- `scope.Unscoped(ctx, reason, fn)` is the same idea taken further - an exemption that
  requires a written reason at each site, a non-empty string literal that `rawsql` checks, so
  `grep Unscoped` is a complete review of every place isolation is lifted.
- Adding a category exemption means adding a fixture line where the rule still fires
  *outside* it.
