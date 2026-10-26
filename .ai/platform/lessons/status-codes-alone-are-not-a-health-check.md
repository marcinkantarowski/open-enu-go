---
tags: [testing, verification, smoke]
date: 2026-10-25
---
# A status-code-only check reports a broken API as healthy

"Did it respond?" is not "did it work?", and the gap between them is precisely where a
misconfigured runtime lives: a crashed or misrouted process usually still returns
*something*, and often with status 200.

## The trap
A smoke test that asserts status codes for every host goes green on a stack that is not
working:

- A runtime that writes its error into the response body answers 200, with the error as the
  page.
- `frontend` and `manager` render in the browser: every path answers 200 with the same HTML
  shell, whether or not the app inside it boots.
- Three hosts routed to one container answer 200 three times, and two of them are the wrong
  app.
- `/health` is shallow by design - "is this process serving?" - and answers 200 while the
  database is down.

Each check asserts the easiest observable thing rather than the thing that matters.

## The rule
**Every check asserts content, not just a status.** When adding a check, ask what a *broken*
system would return. If the check would pass on that, it is not a check.

## Where it is held
- `scripts/dev/smoke.sh` takes `host | path | status | must-contain | label` per target:
  `"status":"ok"` for the API's `/health` and `/health/deep`, and `content="tenant"`,
  `"operator"` or `"marketing"` for the apps - each app's `app-role` meta tag, which also proves
  each host reaches its own app. When the substring is missing it prints the first 160 bytes of
  the body, so a failure says what arrived instead, rather than only that something was wrong.
- `openenu health` exits 1 unless the body contains `"status":"ok"`; the production API's
  healthcheck runs it (`docker/compose.prod.yml`), and the dev one greps the body
  (`docker/compose.dev.yml`).
- A release is declared healthy only when `/health/deep`, through the edge, answers with
  `"status":"ok"` (`scripts/remote/release.sh`).

## Where else this applies
- Any future health, readiness or deploy gate.
- The same instinct produces a check that cannot fail. `check-shell.sh` treats a ShellCheck run
  that could not happen as a failure, not a pass; `make selftest` proves that a missing route
  trace fails route coverage and that a file which stops naming a version fails
  `make versions-check`. Verify that a new guardrail actually fails before trusting it - and
  see [[sentinel-strings-must-not-be-renameable]] for a check that inverted itself.
