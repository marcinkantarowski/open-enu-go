package internal

import (
	"context"

	"github.com/open-enu/kernel/command"
	"github.com/open-enu/kernel/progress"
)

// archiveJob is the background half of archiving: ids only, never entities -
// it runs in another process at another time. The tenant travels with the
// outbox, not with the job.
type archiveJob struct {
	JobID string `json:"jobId"`
}

// Kind is River's job kind, dotted by module.
func (archiveJob) Kind() string { return "example.archive" }

// dispatcher is what the worker needs of the bus; an interface so the unit
// test can stand in for it without a database.
type dispatcher interface {
	Dispatch(ctx context.Context, cmd command.Command) (any, error)
}

// archiveWorker runs the job: one command for the whole batch, then the
// progress bar's verdict. A write from a job is still a command - the trail
// records it like any other.
type archiveWorker struct {
	bus      dispatcher
	progress progress.Reporter
}

func (w archiveWorker) run(ctx context.Context, j archiveJob) error {
	n, err := w.bus.Dispatch(ctx, archiveActive{JobID: j.JobID})
	if err != nil {
		// Marked failed before returning: the retry may well succeed, but a
		// job that goes quiet is indistinguishable from one still running,
		// and somebody is watching this bar.
		_ = w.progress.Fail(ctx, j.JobID, err.Error())
		return err
	}
	return w.progress.Finish(ctx, j.JobID, map[string]any{"archived": n})
}
