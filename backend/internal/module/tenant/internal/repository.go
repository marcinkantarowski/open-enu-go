package internal

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"

	"github.com/open-enu/app/internal/module/tenant/contract"
)

// repository is the write side; contract/repository.go is the read side
// other modules share. The table is not tenant-scoped (it is what a scope
// names), so nothing here depends on the ambient tenant.
type repository struct{ db *scope.Pool }

func newRepository(svc *platform.Services) repository { return repository{db: svc.DB} }

var errNotFound = errors.New("tenant: no such tenant")

func notFound(err error) error {
	if errors.Is(err, errNotFound) {
		return httpx.NotFound(httpx.KeyNotFound)
	}
	return err
}

func (r repository) get(ctx context.Context, tx *scope.Tx, id string) (tenant, error) {
	if _, err := uuid.Parse(id); err != nil {
		return tenant{}, errNotFound
	}
	rows, err := tx.Query(ctx, `SELECT `+contract.Columns+` FROM tenant WHERE id = $1`, id)
	if err != nil {
		return tenant{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return tenant{}, err
		}
		return tenant{}, errNotFound
	}
	info, err := contract.Scan(rows)
	return tenant{info}, err
}

func (r repository) slugTaken(ctx context.Context, tx *scope.Tx, slug string) (taken bool, err error) {
	err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM tenant WHERE slug = $1)`, slug).Scan(&taken)
	return taken, err
}

func (r repository) insert(ctx context.Context, tx *scope.Tx, t tenant) error {
	_, err := tx.Exec(ctx, `INSERT INTO tenant (`+contract.Columns+`) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		t.ID, t.Slug, t.Name, t.Status, t.Plan, t.DefaultLocale, t.Version, t.CreatedAt, t.ActivatedAt)
	return err
}

func (r repository) setStatus(ctx context.Context, tx *scope.Tx, t tenant) error {
	_, err := tx.Exec(ctx, `UPDATE tenant SET status = $2, activated_at = $3, version = $4 WHERE id = $1`, t.ID, t.Status, t.ActivatedAt, t.Version)
	return err
}

func (r repository) remove(ctx context.Context, tx *scope.Tx, id string) error {
	_, err := tx.Exec(ctx, `DELETE FROM tenant WHERE id = $1`, id)
	return err
}

// stalePending lists tenants nobody verified within the window - the purge
// task's input. A read, in its own transaction.
func (r repository) stalePending(ctx context.Context, before time.Time) (items []contract.Info, err error) {
	err = r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+contract.Columns+` FROM tenant WHERE status = $1 AND created_at < $2 ORDER BY created_at`, contract.StatusPending, before)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			i, err := contract.Scan(rows)
			if err != nil {
				return err
			}
			items = append(items, i)
		}
		return rows.Err()
	})
	return items, err
}
