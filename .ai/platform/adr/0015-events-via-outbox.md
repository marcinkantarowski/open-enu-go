# ADR-0015 - Domain events and jobs go through a transactional outbox

**Status:** accepted · **Date:** 2026-10-22

## Context
"Cross-module reactions go through events" means nothing until it says *when* the event
fires. Fired before the commit, another module acts on data that may roll back. Fired after
it, a crash in between loses the event. A queue outside the database cannot take part in the
transaction either way.

## Decision
Events and jobs are **River** jobs - River keeps its queue in Postgres - inserted **in the
handler's own transaction**: `svc.Outbox.Publish(ctx, tx, ev)`. A job exists exactly when
the write that raised it committed.

`openenu worker` runs the subscribers. One that returns an error is retried, up to five
attempts, then left in River's discarded state, where the operator console's worker screen
shows it. An event or job carries the request id it was raised under, and the worker
restores it, so its log lines and audit entries name the request that caused them. Events
that the browser should see are published to the Mercure hub after the commit.

## Consequences
- A rolled-back write raises no event; a committed write always does.
- One queue mechanism, in the database that is already backed up - no broker to run.
- Slightly higher latency than reacting in-process - correct, and unnoticeable.
- A separate long-lived process, which runs the code it started with
  ([`lessons/a-worker-runs-the-code-it-started-with.md`](../lessons/a-worker-runs-the-code-it-started-with.md)).
- Details: [`docs/events-and-outbox.md`](../docs/events-and-outbox.md).
