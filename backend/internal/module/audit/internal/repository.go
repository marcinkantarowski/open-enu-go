package internal

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
)

// repository is the module's own SQL: the GDPR half. Reads for the API go
// through contract/repository.go.
type repository struct{ db *scope.Pool }

func newRepository(svc *platform.Services) repository { return repository{db: svc.DB} }

// actions lists what a person did or had done in their name, for the export.
func (r repository) actions(ctx context.Context, userID string) (out []map[string]any, err error) {
	err = scope.Unscoped(ctx, "GDPR export: a person's actions across every tenant they were a member of", func(ctx context.Context) error {
		return r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
			rows, err := tx.Query(ctx, `SELECT action, tenant_id, subject_id, succeeded, recorded_at FROM audit_entry
				WHERE actor_id = $1 OR on_behalf_of_id = $1 ORDER BY recorded_at`, userID)
			if err != nil {
				return err
			}
			out, err = pgx.CollectRows(rows, pgx.RowToMap)
			return err
		})
	})
	if out == nil {
		out = []map[string]any{}
	}
	return out, err
}

// anonymise replaces the person with `erased` and drops the snapshots.
func (r repository) anonymise(ctx context.Context, userID string) (n int, err error) {
	err = scope.Unscoped(ctx, "GDPR erasure: anonymising a person's entries across every tenant", func(ctx context.Context) error {
		return r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
			tag, err := tx.Exec(ctx, `UPDATE audit_entry SET
				actor_id = CASE WHEN actor_id = $1 THEN 'erased' ELSE actor_id END,
				on_behalf_of_id = CASE WHEN on_behalf_of_id = $1 THEN 'erased' ELSE on_behalf_of_id END,
				before = NULL, after = NULL
				WHERE actor_id = $1 OR on_behalf_of_id = $1`, userID)
			n = int(tag.RowsAffected())
			return err
		})
	})
	return n, err
}
