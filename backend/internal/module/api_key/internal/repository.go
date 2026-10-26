package internal

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
)

// repository is the only file with SQL. The table is under row-level
// security: a tenant lists and revokes its own keys and nobody else's. The
// one lift is resolve - a bearer `sk_` arrives before any tenant is known,
// and the key IS what names the tenant.
type repository struct{ db *scope.Pool }

func newRepository(svc *platform.Services) repository { return repository{db: svc.DB} }

var errNotFound = errors.New("api_key: no such key")

func notFound(err error) error {
	if errors.Is(err, errNotFound) {
		return httpx.NotFound(httpx.KeyNotFound)
	}
	return err
}

const columns = `id, tenant_id, name, key_hash, key_prefix, permissions, revoked, expires_at, last_used_at, created_at`

func (r repository) list(ctx context.Context) (items []apiKey, err error) {
	err = r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+columns+` FROM api_key ORDER BY created_at DESC, id`)
		if err != nil {
			return err
		}
		items, err = scan(rows)
		return err
	})
	return items, err
}

func (r repository) get(ctx context.Context, tx *scope.Tx, id string) (apiKey, error) {
	if _, err := uuid.Parse(id); err != nil {
		return apiKey{}, errNotFound
	}
	rows, err := tx.Query(ctx, `SELECT `+columns+` FROM api_key WHERE id = $1`, id)
	if err != nil {
		return apiKey{}, err
	}
	items, err := scan(rows)
	if err != nil {
		return apiKey{}, err
	}
	if len(items) == 0 {
		return apiKey{}, errNotFound
	}
	return items[0], nil
}

func (r repository) insert(ctx context.Context, tx *scope.Tx, k apiKey) error {
	_, err := tx.Exec(ctx, `INSERT INTO api_key (`+columns+`) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		k.ID, k.TenantID, k.Name, k.KeyHash, k.KeyPrefix, k.Permissions, k.Revoked, k.ExpiresAt, k.LastUsedAt, k.CreatedAt)
	return err
}

func (r repository) revoke(ctx context.Context, tx *scope.Tx, id string) error {
	_, err := tx.Exec(ctx, `UPDATE api_key SET revoked = true WHERE id = $1`, id)
	return err
}

// resolve finds the key a bearer secret hashes to, across tenants: the
// request has no tenant until this answers. The caller decides whether the
// key is usable; this only finds it.
func (r repository) resolve(ctx context.Context, hash string) (k apiKey, found bool, err error) {
	err = scope.Unscoped(ctx, "an API key names its tenant; the lookup by hash must run before any tenant is in scope", func(ctx context.Context) error {
		return r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
			rows, err := tx.Query(ctx, `SELECT `+columns+` FROM api_key WHERE key_hash = $1`, hash)
			if err != nil {
				return err
			}
			items, err := scan(rows)
			if err != nil || len(items) == 0 {
				return err
			}
			k, found = items[0], true
			return nil
		})
	})
	return k, found, err
}

// touch stamps last_used_at, in the key's own tenant scope.
func (r repository) touch(ctx context.Context, k apiKey) error {
	return r.db.Run(scope.WithTenant(ctx, k.TenantID), func(ctx context.Context, tx *scope.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE api_key SET last_used_at = now() WHERE id = $1`, k.ID)
		return err
	})
}

func scan(rows pgx.Rows) ([]apiKey, error) {
	defer rows.Close()
	var out []apiKey
	for rows.Next() {
		var k apiKey
		if err := rows.Scan(&k.ID, &k.TenantID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.Permissions, &k.Revoked, &k.ExpiresAt, &k.LastUsedAt, &k.CreatedAt); err != nil {
			return nil, err
		}
		if k.Permissions == nil {
			k.Permissions = []string{}
		}
		out = append(out, k)
	}
	return out, rows.Err()
}
