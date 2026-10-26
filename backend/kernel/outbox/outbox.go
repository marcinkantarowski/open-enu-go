// Package outbox is how an event leaves a transaction: as a River job
// inserted in the SAME transaction as the change that raised it, so a
// rollback takes the event with it and a commit guarantees delivery
// (ADR-0015). River is also the job queue, so "a handler
// enqueues background work" and "a handler raises an event" are one
// mechanism with one set of tables, retries and dead letters.
//
// A handler calls Publish(ctx, tx, ev). The worker (`openenu worker`) picks
// the job up, restores the tenant scope and the request id from it (so its
// log lines and audit entries name the request that caused it), pushes a Broadcast event to
// the tenant's browsers and calls every Subscriber for the event's name. A
// subscriber that returns an error is retried by River, up to MaxAttempts,
// then parked in the dead-letter state an operator can see.
package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/open-enu/kernel/event"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/scope"
)

// EventArgs is the job payload: everything a worker needs, and no entity -
// a job runs in another process at another time.
type EventArgs struct {
	Name               string         `json:"name"`
	Subject            string         `json:"subjectId"`
	At                 time.Time      `json:"occurredAt"`
	Data               map[string]any `json:"payload"`
	TenantID           string         `json:"tenantId"`
	Broadcast          bool           `json:"broadcast"`
	RequiresPermission string         `json:"requiresPermission"`
	// RequestID is the X-Request-Id of the request that raised the event,
	// "" when nothing did (a scheduled task).
	RequestID string `json:"requestId,omitempty"`
}

// Kind is River's job kind.
func (EventArgs) Kind() string { return "event" }

// InsertOpts: five attempts, the same budget Enqueue gives a job.
func (EventArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 5} }

// Delivered is the event as a subscriber receives it. It implements
// event.Broadcast so the Broadcaster can push it unchanged.
type Delivered struct{ args EventArgs }

func (d Delivered) EventName() string          { return d.args.Name }
func (d Delivered) SubjectID() string          { return d.args.Subject }
func (d Delivered) OccurredAt() time.Time      { return d.args.At }
func (d Delivered) Payload() map[string]any    { return d.args.Data }
func (d Delivered) RequiresPermission() string { return d.args.RequiresPermission }

// TenantID is the tenant the event was raised in, "" for none.
func (d Delivered) TenantID() string { return d.args.TenantID }

// Subscriber handles one delivered event. Return an error to have River
// retry.
type Subscriber func(ctx context.Context, ev Delivered) error

// Options tunes the worker; zero values are River's defaults.
type Options struct {
	MaxWorkers   int
	PollInterval time.Duration
}

// Outbox publishes events and, when started, works them.
type Outbox struct {
	client      *river.Client[pgx.Tx]
	workers     *river.Workers
	subs        map[string][]Subscriber
	all         []Subscriber
	broadcaster *event.Broadcaster
	log         *slog.Logger
}

// New builds the outbox on the application's pool. River's tables come from
// migrate.Run. Subscribe before Start.
func New(pool *scope.Pool, broadcaster *event.Broadcaster, log *slog.Logger, opts Options) (*Outbox, error) {
	if log == nil {
		log = slog.Default()
	}
	o := &Outbox{subs: map[string][]Subscriber{}, broadcaster: broadcaster, log: log, workers: river.NewWorkers()}
	river.AddWorker(o.workers, &eventWorker{o: o})
	max := opts.MaxWorkers
	if max <= 0 {
		max = 10
	}
	cfg := &river.Config{
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: max}},
		Workers: o.workers,
		Logger:  log,
	}
	if opts.PollInterval > 0 {
		// River insists the poll interval is not shorter than the cooldown.
		cfg.FetchCooldown, cfg.FetchPollInterval = opts.PollInterval, opts.PollInterval
	}
	client, err := river.NewClient(riverpgxv5.New(pool.Pool), cfg)
	if err != nil {
		return nil, fmt.Errorf("outbox: %w", err)
	}
	o.client = client
	return o, nil
}

// Subscribe registers a handler for an event name.
func (o *Outbox) Subscribe(eventName string, s Subscriber) {
	o.subs[eventName] = append(o.subs[eventName], s)
}

// SubscribeAll registers a handler for every event: what an outbound
// webhook publisher needs, since which events a tenant wants is a row in
// its table, not a line in the code. It runs after the named subscribers.
func (o *Outbox) SubscribeAll(s Subscriber) {
	o.all = append(o.all, s)
}

// Publish queues ev in tx. The tenant comes from the context, as the
// handler's did; a Broadcast event is marked so the worker pushes it.
func (o *Outbox) Publish(ctx context.Context, tx *scope.Tx, ev event.Event) error {
	if !event.ValidName(ev.EventName()) {
		return fmt.Errorf("outbox: event name %q is not dotted lowercase", ev.EventName())
	}
	args := EventArgs{Name: ev.EventName(), Subject: ev.SubjectID(), At: ev.OccurredAt(), Data: ev.Payload()}
	args.TenantID, _ = scope.TenantID(ctx)
	args.RequestID = httpx.RequestIDFrom(ctx)
	if b, ok := ev.(event.Broadcast); ok {
		args.Broadcast, args.RequiresPermission = true, b.RequiresPermission()
	}
	if _, err := o.client.InsertTx(ctx, tx.Tx, args, nil); err != nil {
		return fmt.Errorf("outbox: %w", err)
	}
	return nil
}

// Start begins working jobs; `openenu worker` calls it, `openenu api` does not.
func (o *Outbox) Start(ctx context.Context) error { return o.client.Start(ctx) }

// Stop waits for running jobs, then stops.
func (o *Outbox) Stop(ctx context.Context) error { return o.client.Stop(ctx) }

// Completed returns a channel of finished jobs, for tests and for the
// operator's queue view. Call the returned func to unsubscribe.
func (o *Outbox) Completed() (<-chan *river.Event, func()) {
	return o.client.Subscribe(river.EventKindJobCompleted, river.EventKindJobFailed)
}

type eventWorker struct {
	river.WorkerDefaults[EventArgs]
	o *Outbox
}

// Work restores the scope and the request id, and delivers.
func (w *eventWorker) Work(ctx context.Context, job *river.Job[EventArgs]) error {
	ctx = restore(ctx, job.Args.TenantID, job.Args.RequestID)
	d := Delivered{args: job.Args}
	if job.Args.Broadcast && w.o.broadcaster != nil {
		w.o.broadcaster.Handle(ctx, d)
	}
	for _, s := range append(append([]Subscriber(nil), w.o.subs[job.Args.Name]...), w.o.all...) {
		if err := s(ctx, d); err != nil {
			return fmt.Errorf("outbox: %s: %w", job.Args.Name, err)
		}
	}
	return nil
}

// Job is background work a handler enqueues: a River job whose Kind is the
// module's dotted name (`example.archive`) and whose fields are everything
// the worker needs - ids, never entities. The tenant and the request id are
// carried by the outbox, not by the job.
type Job interface{ river.JobArgs }

// JobFunc runs one job under the tenant scope it was enqueued in.
type JobFunc[J Job] func(ctx context.Context, job J) error

// Handle registers the worker for J. Called from Mount, before Start; the
// api process registers too, so a job and its worker cannot disagree about
// a type - one binary, two roles.
func Handle[J Job](o *Outbox, fn JobFunc[J]) {
	river.AddWorker(o.workers, &jobWorker[J]{fn: fn})
}

// Enqueue inserts job in tx - the outbox pattern for work as for events: a
// rollback takes the job with it. Five attempts, like an event, unless the
// job's own InsertOpts (river.JobArgsWithInsertOpts) say otherwise - the
// wrapper would hide them from River, so they are read here.
func (o *Outbox) Enqueue(ctx context.Context, tx *scope.Tx, job Job) error {
	tenant, _ := scope.TenantID(ctx)
	opts := river.InsertOpts{MaxAttempts: 5}
	if j, ok := job.(river.JobArgsWithInsertOpts); ok {
		opts = j.InsertOpts()
		if opts.MaxAttempts == 0 {
			opts.MaxAttempts = 5
		}
	}
	_, err := o.client.InsertTx(ctx, tx.Tx, scopedJob{Job: job, Tenant: tenant, RequestID: httpx.RequestIDFrom(ctx)}, &opts)
	if err != nil {
		return fmt.Errorf("outbox: %w", err)
	}
	return nil
}

// scopedJob wraps a Job so the tenant and the request id travel with it
// without every job repeating the fields; it serialises as
// {"tenantId": ..., "requestId": ..., "job": {...}}.
type scopedJob struct {
	Job       Job    `json:"job"`
	Tenant    string `json:"tenantId"`
	RequestID string `json:"requestId,omitempty"`
}

func (s scopedJob) Kind() string { return s.Job.Kind() }

type jobWorker[J Job] struct {
	river.WorkerDefaults[scopedArgs[J]]
	fn JobFunc[J]
}

// scopedArgs is scopedJob as the worker decodes it: the kind comes from the
// type parameter, so the same JSON round-trips.
type scopedArgs[J Job] struct {
	Job       J      `json:"job"`
	Tenant    string `json:"tenantId"`
	RequestID string `json:"requestId,omitempty"`
}

func (s scopedArgs[J]) Kind() string { var z J; return z.Kind() }

func (w *jobWorker[J]) Work(ctx context.Context, job *river.Job[scopedArgs[J]]) error {
	return w.fn(restore(ctx, job.Args.Tenant, job.Args.RequestID), job.Args.Job)
}

// restore puts back on a worker's context what the enqueuing request had: its
// tenant scope, and its request id - which is what a person quotes, and what
// joins this job's lines and audit entries to that request.
func restore(ctx context.Context, tenant, requestID string) context.Context {
	if tenant != "" {
		ctx = scope.WithTenant(ctx, tenant)
	}
	if requestID != "" {
		ctx = httpx.WithRequestID(ctx, requestID)
	}
	return ctx
}
