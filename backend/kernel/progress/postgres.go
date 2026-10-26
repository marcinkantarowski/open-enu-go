package progress

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/open-enu/kernel/event"
	"github.com/open-enu/kernel/scope"
)

// Snapshot is a job as the browser sees it - the body of
// GET /api/progress/{id} and of every update on the job's topic. One shape,
// defined once, so the poll and the push cannot disagree.
type Snapshot struct {
	ID            string         `json:"id"`
	Kind          string         `json:"kind"`
	Label         *string        `json:"label"`
	Total         int            `json:"total"`
	Done          int            `json:"done"`
	Percent       int            `json:"percent"`
	Status        string         `json:"status"`
	Result        map[string]any `json:"result"`
	FailureReason *string        `json:"failureReason"`
	StartedAt     time.Time      `json:"startedAt"`
	FinishedAt    *time.Time     `json:"finishedAt"`
}

// Topic is where a tenant's browsers follow one job; the realtime cookie
// already subscribes to `/tenants/{tid}/progress/{id}`.
func Topic(tenantID, jobID string) string {
	return fmt.Sprintf("/tenants/%s/progress/%s", tenantID, jobID)
}

// Store reads progress_job (kernel migration 0003) under the row-level
// policy: another tenant's job is not found, not forbidden.
type Store struct{ Pool *scope.Pool }

const columns = `id, kind, label, total, done, status, result, failure_reason, started_at, finished_at`

// Get returns the job, or false.
func (s Store) Get(ctx context.Context, id string) (snap Snapshot, found bool, err error) {
	err = s.Pool.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+columns+` FROM progress_job WHERE id = $1`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		if !rows.Next() {
			return rows.Err()
		}
		found = true
		return rows.Scan(&snap.ID, &snap.Kind, &snap.Label, &snap.Total, &snap.Done, &snap.Status, &snap.Result, &snap.FailureReason, &snap.StartedAt, &snap.FinishedAt)
	})
	if snap.Total > 0 {
		snap.Percent = int(math.Round(float64(snap.Done) * 100 / float64(snap.Total)))
	}
	if snap.Result == nil {
		snap.Result = map[string]any{}
	}
	return snap, found, err
}

// Postgres is the Reporter the process runs: every call is a row written
// in its own transaction - outside the bus, because a bar
// that advances two hundred times is not two hundred audit entries - and
// then published on the job's topic. Start outside a tenant scope only
// logs: a job nobody can subscribe to has nothing to persist.
type Postgres struct {
	Store
	Hub event.Publisher
	Log *slog.Logger
}

func (p Postgres) log() *slog.Logger {
	if p.Log == nil {
		return slog.Default()
	}
	return p.Log
}

// Start persists a running job for the tenant in scope and returns its id.
func (p Postgres) Start(ctx context.Context, kind string, total int, label string) (string, error) {
	id := NewID()
	tenant, ok := scope.TenantID(ctx)
	if !ok {
		return Logging{Log: p.Log}.Start(ctx, kind, total, label)
	}
	err := p.Pool.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO progress_job (id, tenant_id, kind, label, total, done, status, result, started_at)
			VALUES ($1, $2, $3, NULLIF($4, ''), $5, 0, $6, '{}'::jsonb, now())`, id, tenant, kind, label, total, StatusRunning)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("progress: start: %w", err)
	}
	p.publish(ctx, tenant, id)
	return id, nil
}

// Advance adds to done, never past total, and replaces the label when one
// is given.
func (p Postgres) Advance(ctx context.Context, jobID string, by int, label string) error {
	return p.update(ctx, jobID, `UPDATE progress_job SET done = LEAST(total, done + $2), label = COALESCE(NULLIF($3, ''), label)
		WHERE id = $1 AND status = 'running'`, by, label)
}

// Finish marks the job done with its result; done becomes total.
func (p Postgres) Finish(ctx context.Context, jobID string, result map[string]any) error {
	res, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if result == nil {
		res = []byte("{}")
	}
	return p.update(ctx, jobID, `UPDATE progress_job SET status = 'done', done = total, result = $2, finished_at = now()
		WHERE id = $1 AND status = 'running'`, res)
}

// Fail marks the job failed with the reason.
func (p Postgres) Fail(ctx context.Context, jobID string, reason string) error {
	return p.update(ctx, jobID, `UPDATE progress_job SET status = 'failed', failure_reason = $2, finished_at = now()
		WHERE id = $1 AND status = 'running'`, reason)
}

func (p Postgres) update(ctx context.Context, jobID, sql string, args ...any) error {
	tenant, ok := scope.TenantID(ctx)
	if !ok {
		p.log().WarnContext(ctx, "progress.update_outside_tenant", "job", jobID)
		return nil
	}
	var changed int64
	err := p.Pool.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		tag, err := tx.Exec(ctx, sql, append([]any{jobID}, args...)...)
		changed = tag.RowsAffected()
		return err
	})
	if err != nil {
		return fmt.Errorf("progress: %w", err)
	}
	// A finished job is final: a late call touches no row and pushes nothing.
	if changed > 0 {
		p.publish(ctx, tenant, jobID)
	}
	return nil
}

// publish pushes the current row to the job's topic. A hub failure is
// logged, not returned: the row is the truth and the browser also polls.
func (p Postgres) publish(ctx context.Context, tenant, jobID string) {
	if p.Hub == nil {
		return
	}
	snap, found, err := p.Get(ctx, jobID)
	if err != nil || !found {
		return
	}
	data, _ := json.Marshal(snap)
	if err := p.Hub.Publish(ctx, Topic(tenant, jobID), data); err != nil {
		p.log().WarnContext(ctx, "progress.publish_failed", "job", jobID, "error", err.Error())
	}
}
