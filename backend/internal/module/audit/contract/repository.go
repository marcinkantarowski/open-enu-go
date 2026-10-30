package contract

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
)

// NewReader reads audit_entry through the platform's pool.
func NewReader(svc *platform.Services) Reader { return reader{db: svc.DB} }

type reader struct{ db *scope.Pool }

const columns = `id, action, tenant_id, actor_id, on_behalf_of_id, subject_id, request_id, succeeded, failure_reason, before, after, context, recorded_at`

func (r reader) Recent(ctx context.Context, tenantID *string, limit int) ([]Entry, error) {
	if limit <= 0 || limit > MaxLimit {
		limit = DefaultLimit
	}
	if tenantID != nil {
		return r.query(scope.WithTenant(ctx, *tenantID), limit)
	}
	var out []Entry
	err := scope.Unscoped(ctx, "operator audit view: the trail across every tenant, for the manager realm only", func(ctx context.Context) error {
		var err error
		out, err = r.query(ctx, limit)
		return err
	})
	return out, err
}

func (r reader) query(ctx context.Context, limit int) (out []Entry, err error) {
	err = r.db.Read(ctx, func(ctx context.Context, tx *scope.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+columns+` FROM audit_entry ORDER BY recorded_at DESC, id DESC LIMIT $1`, limit)
		if err != nil {
			return err
		}
		out, err = scan(rows)
		return err
	})
	return out, err
}

func scan(rows pgx.Rows) ([]Entry, error) {
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.Action, &e.TenantID, &e.ActorID, &e.OnBehalfOfID, &e.SubjectID, &e.RequestID, &e.Succeeded, &e.FailureReason, &e.Before, &e.After, &e.Context, &e.RecordedAt); err != nil {
			return nil, err
		}
		if e.Context == nil {
			e.Context = map[string]any{}
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
