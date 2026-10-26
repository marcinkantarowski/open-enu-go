package outbox

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/riverqueue/river"

	"github.com/open-enu/kernel/event"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/migrate"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/testdb"
)

const tenantA = "11111111-1111-1111-1111-111111111111"

type created struct{ id string }

func (c created) EventName() string          { return "demo.thing.created" }
func (c created) SubjectID() string          { return c.id }
func (c created) OccurredAt() time.Time      { return time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC) }
func (c created) Payload() map[string]any    { return map[string]any{"id": c.id} }
func (c created) RequiresPermission() string { return "demo.view" }

// internalOnly has no RequiresPermission: not a Broadcast, never pushed.
type internalOnly struct{ id string }

func (internalOnly) EventName() string         { return "demo.thing.touched" }
func (i internalOnly) SubjectID() string       { return i.id }
func (internalOnly) OccurredAt() time.Time     { return time.Now() }
func (i internalOnly) Payload() map[string]any { return map[string]any{"id": i.id} }

func TestEventsLeaveWithTheTransactionOrNotAtAll(t *testing.T) {
	db := testdb.Postgres(t)
	ctx := context.Background()
	if _, err := migrate.Run(ctx, db.Admin, nil); err != nil {
		t.Fatal(err)
	}
	pool := &scope.Pool{Pool: db.App}
	hub := &event.Recorder{}
	o, err := New(pool, &event.Broadcaster{Hub: hub}, nil, Options{MaxWorkers: 2, PollInterval: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	var got []Delivered
	o.Subscribe("demo.thing.created", func(ctx context.Context, ev Delivered) error {
		if tid, _ := scope.TenantID(ctx); tid != tenantA {
			return errors.New("scope not restored")
		}
		// The request that raised it, so the subscriber's lines and audit
		// entries name that request.
		if id := httpx.RequestIDFrom(ctx); id != "req-events" {
			return fmt.Errorf("request id not restored: %q", id)
		}
		got = append(got, ev)
		return nil
	})
	attempts := 0
	o.Subscribe("demo.thing.touched", func(context.Context, Delivered) error { attempts++; return errors.New("flaky") })

	jobs := func() int {
		var n int
		if err := db.Admin.QueryRow(ctx, "SELECT count(*) FROM river_job").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	scoped := httpx.WithRequestID(scope.WithTenant(ctx, tenantA), "req-events")
	// Rolled back: no job.
	tx, _ := pool.Begin(scoped)
	if err := o.Publish(scoped, tx, created{"s0"}); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback(ctx)
	if jobs() != 0 {
		t.Fatal("a rolled-back transaction must leave no job")
	}
	// Committed: one job per event.
	if err := pool.Run(scoped, func(ctx context.Context, tx *scope.Tx) error {
		if err := o.Publish(ctx, tx, created{"s1"}); err != nil {
			return err
		}
		return o.Publish(ctx, tx, internalOnly{"s2"})
	}); err != nil {
		t.Fatal(err)
	}
	if jobs() != 2 {
		t.Fatalf("two jobs, got %d", jobs())
	}
	if err := pool.Run(scoped, func(ctx context.Context, tx *scope.Tx) error {
		return o.Publish(ctx, tx, badName{})
	}); err == nil || !strings.Contains(err.Error(), "dotted lowercase") {
		t.Fatalf("name shape: %v", err)
	}

	done, cancel := o.Completed()
	defer cancel()
	if err := o.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer o.Stop(ctx)
	seen := map[string]river.EventKind{}
	deadline := time.After(20 * time.Second)
	for len(seen) < 2 {
		select {
		case ev := <-done:
			seen[ev.Job.Kind+"/"+string(ev.Job.State)] = ev.Kind
			if ev.Kind == river.EventKindJobFailed && ev.Job.Attempt >= 2 {
				seen["failed-twice"] = ev.Kind
			}
		case <-deadline:
			t.Fatalf("jobs did not finish: %v", seen)
		}
	}
	if len(got) != 1 || got[0].SubjectID() != "s1" || got[0].TenantID() != tenantA || got[0].Payload()["id"] != "s1" {
		t.Fatalf("subscriber got %+v", got)
	}
	ups := hub.Updates()
	if len(ups) != 1 || ups[0].Topic != "/tenants/"+tenantA+"/events" || !strings.Contains(string(ups[0].Data), `"requiresPermission":"demo.view"`) {
		t.Fatalf("broadcast: %+v", ups)
	}
	if attempts < 1 {
		t.Fatal("the flaky subscriber was tried")
	}
}

type badName struct{ created }

func (badName) EventName() string { return "Created!" }

type countJob struct{ N int }

func (countJob) Kind() string { return "demo.count" }

func TestAJobIsEnqueuedInTheTransactionAndRunsUnderItsTenantAndRequest(t *testing.T) {
	db := testdb.Postgres(t)
	ctx := context.Background()
	if _, err := migrate.Run(ctx, db.Admin, nil); err != nil {
		t.Fatal(err)
	}
	pool := &scope.Pool{Pool: db.App}
	o, err := New(pool, nil, nil, Options{MaxWorkers: 2, PollInterval: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan string, 1)
	Handle(o, func(ctx context.Context, j countJob) error {
		tenant, _ := scope.TenantID(ctx)
		got <- fmt.Sprintf("%s:%d:%s", tenant, j.N, httpx.RequestIDFrom(ctx))
		return nil
	})
	scoped := httpx.WithRequestID(scope.WithTenant(ctx, tenantA), "req-job")
	if err := pool.Run(scoped, func(ctx context.Context, tx *scope.Tx) error {
		return o.Enqueue(ctx, tx, countJob{N: 7})
	}); err != nil {
		t.Fatal(err)
	}
	if err := o.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer o.Stop(ctx)
	select {
	case s := <-got:
		if s != tenantA+":7:req-job" {
			t.Fatalf("worker saw %q", s)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("job never ran")
	}
}
