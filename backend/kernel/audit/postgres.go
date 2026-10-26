package audit

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/open-enu/kernel/scope"
)

// Postgres writes entries to audit_entry (kernel migration 0001). Its own
// short transaction, under bypass: an entry is written after the command's
// transaction ended - so a failed command still leaves its trace - and
// entries from the manager realm carry no tenant, which the policy would
// otherwise refuse.
//
// Never fails. A write error is logged with the entry's action and the entry
// is lost; the alternative is undoing a change that already happened.
type Postgres struct {
	Pool *scope.Pool
	Log  *slog.Logger
}

// Record inserts, or logs why it could not.
func (p Postgres) Record(ctx context.Context, e Entry) {
	log := p.Log
	if log == nil {
		log = slog.Default()
	}
	defer func() {
		if r := recover(); r != nil {
			log.ErrorContext(ctx, "audit.record_failed", "panic", r, "action", e.Action)
		}
	}()
	at := e.At
	if at.IsZero() {
		at = time.Now()
	}
	before, after, cx := jsonOrNull(e.Before), jsonOrNull(e.After), jsonOrNull(e.Context)
	if cx == nil {
		cx = []byte("{}")
	}
	err := scope.Unscoped(context.WithoutCancel(ctx), "audit entries are written after the request's scope ended and may have no tenant", func(ctx context.Context) error {
		return p.Pool.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO audit_entry
				(action, tenant_id, actor_id, on_behalf_of_id, subject_id, request_id, succeeded, failure_reason, before, after, context, recorded_at)
				VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), $7, NULLIF($8, ''), $9, $10, $11, $12)`,
				e.Action, e.TenantID, e.ActorID, e.OnBehalfOf, e.SubjectID, e.RequestID, e.Succeeded, e.FailureReason, before, after, cx, at)
			return err
		})
	})
	if err != nil {
		log.ErrorContext(ctx, "audit.record_failed", "error", err.Error(), "action", e.Action, "subject", e.SubjectID)
	}
}

// jsonOrNull encodes a map, or returns nil for a nil map so the column is
// NULL rather than "null".
func jsonOrNull(m map[string]any) []byte {
	if m == nil {
		return nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return []byte(`{"_unencodable":true}`)
	}
	return b
}
