# Events and the outbox

Something happened; who needs to know finds out after the commit, never before, and never
if there was no commit. The mechanism is River: an event is a job inserted in the same
transaction as the change ([ADR-0015](../adr/0015-events-via-outbox.md)), worked by
`openenu worker`.

## Raising an event

```go
// contract/events.go - exported: other modules subscribe by name
type ProjectCreated struct{ ID, TenantID, Name string; At time.Time }

func (e ProjectCreated) EventName() string          { return "example.project.created" }
func (e ProjectCreated) SubjectID() string          { return e.ID }
func (e ProjectCreated) OccurredAt() time.Time      { return e.At }
func (e ProjectCreated) Payload() map[string]any    { return map[string]any{"id": e.ID, "name": e.Name} }
func (e ProjectCreated) RequiresPermission() string { return "" } // present = broadcast to the browser

// handler.go
return project, h.outbox.Publish(ctx, tx, contract.ProjectCreated{...})
```

Names are dotted, lower case, past tense. `Payload()` is explicit: it crosses a trust
boundary when it reaches a browser or a webhook, and "every exported field" is how
internals leak.

## Broadcast is opt in

An event with `RequiresPermission() string` is pushed to the tenant's browsers on
`/tenants/{id}/events` as `{event, subjectId, occurredAt, payload, requiresPermission}`;
one without stays internal. No tenant in the job (a system event) means no topic anybody
is entitled to - dropped with a warning, never a global topic. The hub is Mercure in the
stack (`kernel/realtime.Hub`, every update private) and a recorder in tests; a publish
failure is logged, not retried: the change already happened. A browser subscribes with the
cookie `POST /api/realtime/token` sets, scoped to its own tenant's topics
([`dev-stack.md`](dev-stack.md) → *Realtime*).

## Subscribing

```go
// Mount, in the module that cares
svc.Outbox.Subscribe("example.project.created", func(ctx context.Context, ev outbox.Delivered) error {
	// ctx carries the event's tenant; ev.Payload() is what was published
	return nil
})
```

`svc.Outbox.SubscribeAll(fn)` subscribes to **every** event - what the Webhook module's
publisher is, since which events a tenant wants is a row in its table, not a line in the
code. An all-events subscriber must not fail the event: it logs and returns nil.

A subscriber returning an error is retried by River (five attempts, backoff), then parked
where an operator can see it (`openenu worker` logs; the Manager realm's worker status
screen shows the queue depths and the failed count). Subscribers are registered in `Mount`, which both `api` and `worker` call - the
`api` only inserts, the `worker` only works.

## Jobs

```go
// job.go
type archiveJob struct{ JobID string `json:"jobId"` }
func (archiveJob) Kind() string { return "example.archive" }      // dotted by module

// mount.go
outbox.Handle(svc.Outbox, archiveWorker{bus: svc.Bus, progress: svc.Progress}.run)

// handler.go, inside the command that accepted the work
return result, h.outbox.Enqueue(ctx, tx, archiveJob{JobID: jobID})
```

Ids only, never entities: the job runs in another process at another time. The tenant
and the request id travel with it and the worker's context has both back, so the job's log
lines and audit entries name the request that caused them
([`observability.md`](observability.md)). An event carries the request id the same way. Five attempts, like an event. The
worker reports to `progress` and dispatches a command for anything it writes.

## Why River and not a table of our own

One mechanism for events and background work: the same tables, the same retries, the same
dead-letter state, the same operator view. `InsertTx` is the outbox pattern; the worker is
the dispatcher; there is nothing to poll that we wrote.

## Where it is proved

`kernel/outbox/outbox_test.go`: a rolled-back transaction leaves no job; a committed one
leaves one per event; the worker restores the tenant scope; a broadcast reaches the
tenant's topic with its permission; an internal event does not; a failing subscriber is
retried; a name that is not dotted lowercase is refused at publish.
