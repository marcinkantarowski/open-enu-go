package internal

import (
	"context"
	"errors"
	"testing"

	"github.com/open-enu/kernel/command"
	"github.com/open-enu/kernel/progress"
)

// The job in isolation. The bus is stood in for, and that is the right call
// for once: under test is the worker's SEQUENCE - one command, then finish;
// or fail, then return the error so River retries - not any rule the bus
// enforces. Those are in the functional suite, against Postgres.
type fakeBus struct {
	result any
	err    error
	got    command.Command
}

func (f *fakeBus) Dispatch(_ context.Context, cmd command.Command) (any, error) {
	f.got = cmd
	return f.result, f.err
}

func TestTheJobReportsItsOutcomeToTheBar(t *testing.T) {
	ctx := context.Background()
	rec := &progress.Recorder{}
	id, _ := rec.Start(ctx, "example.archive", 2, "")
	bus := &fakeBus{result: 2}
	if err := (archiveWorker{bus: bus, progress: rec}).run(ctx, archiveJob{JobID: id}); err != nil {
		t.Fatal(err)
	}
	job, _ := rec.Get(id)
	if cmd, ok := bus.got.(archiveActive); !ok || cmd.JobID != id || job.Status != progress.StatusDone || job.Result["archived"] != 2 {
		t.Fatalf("command %+v, job %+v", bus.got, job)
	}

	// Without this the job simply goes quiet, and a quiet job is
	// indistinguishable from a running one to the person watching it.
	id2, _ := rec.Start(ctx, "example.archive", 1, "")
	boom := errors.New("the database went away")
	err := (archiveWorker{bus: &fakeBus{err: boom}, progress: rec}).run(ctx, archiveJob{JobID: id2})
	job2, _ := rec.Get(id2)
	if !errors.Is(err, boom) || job2.Status != progress.StatusFailed || job2.Reason != boom.Error() {
		t.Fatalf("err %v, job %+v", err, job2)
	}
}
