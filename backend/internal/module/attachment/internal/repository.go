package internal

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/storage"
)

// urlTTL is how long a signed download URL lives. Short: the record is
// fetched again for a fresh one, and a leaked link dies on its own.
const urlTTL = 15 * time.Minute

// repository is the only file with SQL. The table is under row-level
// security; the one lift is the sweep's listing, which must see every
// tenant's unclaimed files to clean them.
type repository struct {
	db      *scope.Pool
	storage storage.Storage
}

func newRepository(svc *platform.Services) repository {
	return repository{db: svc.DB, storage: svc.Storage}
}

var errNotFound = errors.New("attachment: no such attachment")

func notFound(err error) error {
	if errors.Is(err, errNotFound) {
		return httpx.NotFound(httpx.KeyNotFound)
	}
	return err
}

const columns = `id, tenant_id, storage_key, filename, content_type, size, owner_type, owner_id, created_at`

func (r repository) find(ctx context.Context, id string) (a attachment, err error) {
	err = r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		a, err = r.get(ctx, tx, id)
		return err
	})
	return a, err
}

func (r repository) get(ctx context.Context, tx *scope.Tx, id string) (attachment, error) {
	if _, err := uuid.Parse(id); err != nil {
		return attachment{}, errNotFound
	}
	rows, err := tx.Query(ctx, `SELECT `+columns+` FROM attachment WHERE id = $1`, id)
	if err != nil {
		return attachment{}, err
	}
	items, err := scan(rows)
	if err != nil {
		return attachment{}, err
	}
	if len(items) == 0 {
		return attachment{}, errNotFound
	}
	return items[0], nil
}

func (r repository) insert(ctx context.Context, tx *scope.Tx, a attachment) error {
	_, err := tx.Exec(ctx, `INSERT INTO attachment (`+columns+`) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		a.ID, a.TenantID, a.StorageKey, a.Filename, a.ContentType, a.Size, a.OwnerType, a.OwnerID, a.CreatedAt)
	return err
}

func (r repository) remove(ctx context.Context, tx *scope.Tx, id string) error {
	_, err := tx.Exec(ctx, `DELETE FROM attachment WHERE id = $1`, id)
	return err
}

// url signs a download link for the record's blob.
func (r repository) url(ctx context.Context, a attachment) (string, error) {
	return r.storage.TemporaryURL(ctx, a.StorageKey, urlTTL)
}

// orphan is what the sweep needs: which row, in which tenant.
type orphan struct{ ID, TenantID string }

// orphans lists files nobody claimed within the window, across tenants.
func (r repository) orphans(ctx context.Context, before time.Time) (out []orphan, err error) {
	err = scope.Unscoped(ctx, "sweeping unclaimed uploads across every tenant", func(ctx context.Context) error {
		return r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
			rows, err := tx.Query(ctx, `SELECT id, tenant_id FROM attachment WHERE owner_id IS NULL AND created_at < $1 ORDER BY created_at`, before)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var o orphan
				if err := rows.Scan(&o.ID, &o.TenantID); err != nil {
					return err
				}
				out = append(out, o)
			}
			return rows.Err()
		})
	})
	return out, err
}

func scan(rows pgx.Rows) ([]attachment, error) {
	defer rows.Close()
	var out []attachment
	for rows.Next() {
		var a attachment
		if err := rows.Scan(&a.ID, &a.TenantID, &a.StorageKey, &a.Filename, &a.ContentType, &a.Size, &a.OwnerType, &a.OwnerID, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
