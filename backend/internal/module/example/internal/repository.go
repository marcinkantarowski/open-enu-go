package internal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/open-enu/kernel/crypto"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
)

// repository is the only file that runs SQL (rawsql) and the only one that
// opens a read transaction (writeboundary). No WHERE tenant_id anywhere:
// isolation is the row-level policy's (tenancy.md), and repeating it here
// would hide that the policy is what protects you.
type repository struct {
	db     *scope.Pool
	crypto *crypto.Encryptor
}

func newRepository(svc *platform.Services) repository {
	return repository{db: svc.DB, crypto: svc.Crypto}
}

var errNotFound = errors.New("example: no such project")

// notFound maps the repository's miss to the envelope. Another tenant's
// project is NOT FOUND rather than forbidden: the policy removed it from the
// query, so this cannot leak its existence.
func notFound(err error) error {
	if errors.Is(err, errNotFound) {
		return httpx.NotFound(httpx.KeyNotFound)
	}
	return err
}

const columns = `tenant_id, id, name, client_reference, description, attributes, status, version, created_at, archived_at`

// list is one page, newest first unless asked otherwise, and the total.
func (r repository) list(ctx context.Context, p httpx.Pagination) (items []project, total int, err error) {
	order := "created_at DESC"
	switch p.Sort {
	case "name":
		order = "name " + p.Direction
	case "createdAt":
		order = "created_at " + p.Direction
	}
	err = r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+columns+` FROM example_project ORDER BY `+order+`, id LIMIT $1 OFFSET $2`, p.Size, p.Offset())
		if err != nil {
			return err
		}
		if items, err = r.scan(rows); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM example_project`).Scan(&total)
	})
	return items, total, err
}

// find is get in its own read transaction, for the controller.
func (r repository) find(ctx context.Context, id string) (p project, err error) {
	err = r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		p, err = r.get(ctx, tx, id)
		return err
	})
	return p, err
}

func (r repository) get(ctx context.Context, tx *scope.Tx, id string) (project, error) {
	if _, err := uuid.Parse(id); err != nil {
		return project{}, errNotFound
	}
	rows, err := tx.Query(ctx, `SELECT `+columns+` FROM example_project WHERE id = $1`, id)
	if err != nil {
		return project{}, err
	}
	items, err := r.scan(rows)
	if err != nil {
		return project{}, err
	}
	if len(items) == 0 {
		return project{}, errNotFound
	}
	return items[0], nil
}

func (r repository) active(ctx context.Context, tx *scope.Tx) ([]project, error) {
	rows, err := tx.Query(ctx, `SELECT `+columns+` FROM example_project WHERE status = $1 ORDER BY created_at, id`, statusActive)
	if err != nil {
		return nil, err
	}
	return r.scan(rows)
}

func (r repository) insert(ctx context.Context, tx *scope.Tx, p project) error {
	ref, err := r.encrypt(p)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO example_project (`+columns+`) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		p.TenantID, p.ID, p.Name, ref, p.Description, p.Attributes, p.Status, p.Version, p.CreatedAt, p.ArchivedAt)
	return err
}

// rename bumps the version where the row is: zero rows means somebody else
// got there first (scope.ErrStale), never a read-then-write race.
func (r repository) rename(ctx context.Context, tx *scope.Tx, expected int, p project) error {
	tag, err := tx.Exec(ctx, `UPDATE example_project SET name = $1, version = version + 1 WHERE id = $2 AND version = $3`, p.Name, p.ID, expected)
	if err != nil {
		return err
	}
	return scope.CheckVersion(tag)
}

func (r repository) archive(ctx context.Context, tx *scope.Tx, p project) error {
	_, err := tx.Exec(ctx, `UPDATE example_project SET status = $1, archived_at = now(), version = version + 1 WHERE id = $2`, statusArchived, p.ID)
	return err
}

// archivedBefore is what the sweep removes: archived, and long enough ago.
func (r repository) archivedBefore(ctx context.Context, tx *scope.Tx, before time.Time) ([]project, error) {
	rows, err := tx.Query(ctx, `SELECT `+columns+` FROM example_project WHERE status = $1 AND archived_at < $2 ORDER BY archived_at, id`, statusArchived, before)
	if err != nil {
		return nil, err
	}
	return r.scan(rows)
}

func (r repository) delete(ctx context.Context, tx *scope.Tx, p project) error {
	_, err := tx.Exec(ctx, `DELETE FROM example_project WHERE id = $1`, p.ID)
	return err
}

// tenantsWithArchivedBefore is the module's one cross-tenant read, and it
// is a question about tenants rather than rows: which scopes must the
// sweep enter? Unscoped, with the reason the bypass is logged under; the
// rows themselves are then read and deleted inside each tenant's scope.
func (r repository) tenantsWithArchivedBefore(ctx context.Context, before time.Time) (ids []string, err error) {
	err = scope.Unscoped(ctx, "example.sweep: which tenants hold archived projects past retention", func(ctx context.Context) error {
		return r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
			rows, err := tx.Query(ctx, `SELECT DISTINCT tenant_id FROM example_project WHERE status = $1 AND archived_at < $2 ORDER BY tenant_id`, statusArchived, before)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					return err
				}
				ids = append(ids, id)
			}
			return rows.Err()
		})
	})
	return ids, err
}

// hasNamed answers the seed's idempotence question - by identity, not count.
func (r repository) hasNamed(ctx context.Context, name string) (found bool, err error) {
	err = r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM example_project WHERE name = $1)`, name).Scan(&found)
	})
	return found, err
}

// seed writes rows for tenantID outside any request: fixtures and the
// tenant seed. The scope is entered here because there is no principal.
func (r repository) seed(ctx context.Context, tenantID string, ps ...project) error {
	return r.db.Run(scope.WithTenant(ctx, tenantID), func(ctx context.Context, tx *scope.Tx) error {
		for _, p := range ps {
			if err := r.insert(ctx, tx, p); err != nil {
				return err
			}
		}
		return nil
	})
}

// encrypt is the one place the encrypted column is written: under the
// row's tenant key, never the ambient scope's.
func (r repository) encrypt(p project) (*string, error) {
	if p.ClientReference == nil {
		return nil, nil
	}
	ct, err := r.crypto.Encrypt(*p.ClientReference, p.TenantID)
	if err != nil {
		return nil, fmt.Errorf("example: encrypt client reference: %w", err)
	}
	return &ct, nil
}

func (r repository) scan(rows pgx.Rows) ([]project, error) {
	defer rows.Close()
	var out []project
	for rows.Next() {
		var p project
		var ref *string
		if err := rows.Scan(&p.TenantID, &p.ID, &p.Name, &ref, &p.Description, &p.Attributes, &p.Status, &p.Version, &p.CreatedAt, &p.ArchivedAt); err != nil {
			return nil, err
		}
		if ref != nil {
			plain, err := r.crypto.Decrypt(*ref, p.TenantID)
			if err != nil {
				return nil, fmt.Errorf("example: decrypt client reference of %s: %w", p.ID, err)
			}
			p.ClientReference = &plain
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
