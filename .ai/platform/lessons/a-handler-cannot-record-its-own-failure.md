---
tags: [river, transactions, observability, workers]
date: 2026-10-25
---
# A handler that records its own failure records nothing

**Inside a transaction, a write followed by an error did not happen.** A handler that writes
why it failed and then returns the error rolls the record back with everything else.

## The trap
Every command handler runs inside the bus's transaction (`backend/kernel/command/bus.go`):
`Dispatch` opens it, hands the handler the `*scope.Tx`, and rolls back on any error. A webhook
delivery written as a command looks right:

```go
func (h handlers) deliver(ctx context.Context, tx *scope.Tx, c deliverOne) (struct{}, error) {
	resp, err := h.http.Do(req)
	// ...
	h.repo.recordAttempt(ctx, tx, c.ID, resp.StatusCode, body) // attempts++, code, body
	return struct{}{}, fmt.Errorf("webhook: receiver answered %d", resp.StatusCode)
}
```

The error is what makes River retry, and River retries. The row does not move: `attempts`
stays 0, `response_code` and `response_body` stay empty, because the error rolled back the
transaction the attempt was written in. The log that exists *specifically* to answer "did you
send it?" is empty about five real HTTP requests, and the worker's log and the delivery log
disagree about how many were made.

Both halves are correct on their own. A transaction makes a handler atomic. Returning the
error is how River's retry is triggered, and re-queueing by hand would be a second, worse retry
mechanism beside the real one. They are simply mutually exclusive, and nothing says so.

A test that calls the repository directly records the attempt and passes. Only running the
attempt the way the queue does - and reading the row afterwards - shows the difference.

## The rule
Anything that must survive a failure is written outside the failing transaction, and committed
before the error is returned.

## Where it is held
- `backend/internal/module/webhook/internal/job.go`: delivery is a River job, not a command.
  `deliverer.fail` calls `repository.failed`, which commits the attempt in a transaction of its
  own, and only then returns the error that makes River retry - the queue and the log count the
  same attempts. On the fifth failure the row turns `failed` and the tenant is notified.
- `delivery_test.go` (`TestTheFifthFailureClosesTheRowAndTellsTheTenant`) runs the attempt
  five times as River would and asserts after each one that `run` returned an error and that
  `attempts` equals the attempt number.
- The bus holds the same rule for its own trail: `Dispatch` records a failed command's audit
  entry after the rollback, and `audit.Postgres` writes it in its own short transaction, so a
  failed command still leaves its trace.
- `AGENTS.md` (Never list).

## Where else this applies
- Any handler that wants to record *why* it failed: delivery logs, import error reports,
  retry counters, "last attempted at" columns.
- Any counter incremented on a path that then returns an error - a rate limiter written this
  way would never count a rejection.
- The inverse needs no ceremony: recording a **success** and returning `nil` commits, which is
  why the delivered path in the same job is one update (`repository.succeeded`) and a `nil`.
