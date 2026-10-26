// Package progress reports how far a long-running job has got.
//
// Every product grows an operation that outlives a request - an import, a
// bulk update, a report. Without one way to report progress each invents
// polling and each UI differs. A job calls Start, then Advance, then Finish
// or Fail; the Postgres reporter persists each update in progress_job
// (kernel migration 0003) and publishes it on Topic -
// `/tenants/{tid}/progress/{jobId}` - which the ui-kit's useProgress
// consumes, and the Progress module serves the read endpoint from Store.
// Logging makes the calls visible in the log, and Recorder lets a test
// assert on them.
package progress

import (
	"context"
	"crypto/rand"
	"log/slog"
	"sync"
)

// Reporter is what a job holds. The id returned by Start is handed back to
// the caller (a 202 body) so it can subscribe.
type Reporter interface {
	Start(ctx context.Context, kind string, total int, label string) (string, error)
	Advance(ctx context.Context, jobID string, by int, label string) error
	Finish(ctx context.Context, jobID string, result map[string]any) error
	Fail(ctx context.Context, jobID string, reason string) error
}

// NewID mints a job id: 26 characters of standard base32 (A-Z, 2-7) from
// crypto/rand - a ULID's length, without the time prefix nothing reads.
func NewID() string { return rand.Text() }

// Logging reports to a structured log. Start and Finish at Info, Advance at
// Debug, Fail at Error.
type Logging struct {
	Log *slog.Logger
}

func (l Logging) log() *slog.Logger {
	if l.Log == nil {
		return slog.Default()
	}
	return l.Log
}

// Start logs and returns a fresh id.
func (l Logging) Start(ctx context.Context, kind string, total int, label string) (string, error) {
	id := NewID()
	l.log().InfoContext(ctx, "progress.start", "job", id, "kind", kind, "total", total, "label", label)
	return id, nil
}

// Advance logs at Debug.
func (l Logging) Advance(ctx context.Context, jobID string, by int, label string) error {
	l.log().DebugContext(ctx, "progress.advance", "job", jobID, "by", by, "label", label)
	return nil
}

// Finish logs the result.
func (l Logging) Finish(ctx context.Context, jobID string, result map[string]any) error {
	l.log().InfoContext(ctx, "progress.finish", "job", jobID, "result", result)
	return nil
}

// Fail logs at Error.
func (l Logging) Fail(ctx context.Context, jobID string, reason string) error {
	l.log().ErrorContext(ctx, "progress.fail", "job", jobID, "reason", reason)
	return nil
}

// Status of a recorded job.
const (
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
)

// Job is what the Recorder knows about one job - the same fields the
// Progress module will store.
type Job struct {
	ID     string
	Kind   string
	Total  int
	Done   int
	Label  string
	Status string
	Result map[string]any
	Reason string
}

// Recorder keeps jobs in memory, for tests.
type Recorder struct {
	mu   sync.Mutex
	jobs map[string]*Job
}

// Start records a running job.
func (r *Recorder) Start(_ context.Context, kind string, total int, label string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.jobs == nil {
		r.jobs = map[string]*Job{}
	}
	id := NewID()
	r.jobs[id] = &Job{ID: id, Kind: kind, Total: total, Label: label, Status: StatusRunning}
	return id, nil
}

// Advance adds to Done and replaces the label when one is given.
func (r *Recorder) Advance(_ context.Context, jobID string, by int, label string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if j, ok := r.jobs[jobID]; ok {
		j.Done += by
		if label != "" {
			j.Label = label
		}
	}
	return nil
}

// Finish marks done.
func (r *Recorder) Finish(_ context.Context, jobID string, result map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if j, ok := r.jobs[jobID]; ok {
		j.Status, j.Result = StatusDone, result
	}
	return nil
}

// Fail marks failed.
func (r *Recorder) Fail(_ context.Context, jobID string, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if j, ok := r.jobs[jobID]; ok {
		j.Status, j.Reason = StatusFailed, reason
	}
	return nil
}

// Get returns a copy of a job.
func (r *Recorder) Get(jobID string) (Job, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[jobID]
	if !ok {
		return Job{}, false
	}
	return *j, true
}
