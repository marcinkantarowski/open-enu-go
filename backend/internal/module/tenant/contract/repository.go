package contract

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/open-enu/kernel/cache"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
)

// NewReader is the Reader over the platform's pool and cache. The read side
// lives in contract/ because it IS the contract: a module describing a
// tenant runs this, not a copy of it. The table is not under row-level
// security, so these reads answer in any scope - which is what a login,
// which has no tenant yet, needs.
func NewReader(svc *platform.Services) Reader {
	return reader{db: svc.DB, cache: svc.Cache}
}

type reader struct {
	db    *scope.Pool
	cache *cache.Tenant
}

// Columns is the SELECT list Scan reads; internal/ shares it.
const Columns = `id, slug, name, status, plan, default_locale, version, created_at, activated_at`

// Scan reads one row in Columns order.
func Scan(rows pgx.Rows) (Info, error) {
	var i Info
	err := rows.Scan(&i.ID, &i.Slug, &i.Name, &i.Status, &i.Plan, &i.DefaultLocale, &i.Version, &i.CreatedAt, &i.ActivatedAt)
	return i, err
}

// DescribeTTL is how long a description is served from the cache.
const DescribeTTL = 5 * time.Minute

func (r reader) Describe(ctx context.Context, id string) (Info, bool, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Info{}, false, nil
	}
	type hit struct {
		Info  Info
		Found bool
	}
	h, err := cache.GetJSON(ctx, r.cache, "tenant", "describe:"+id, DescribeTTL, func(ctx context.Context) (hit, error) {
		i, found, err := r.one(ctx, `WHERE id = $1`, id)
		return hit{i, found}, err
	}, cache.TenantTag(id))
	return h.Info, h.Found, err
}

func (r reader) BySlug(ctx context.Context, slug string) (Info, bool, error) {
	return r.one(ctx, `WHERE slug = $1`, slug)
}

func (r reader) one(ctx context.Context, where string, arg any) (info Info, found bool, err error) {
	err = r.db.Read(ctx, func(ctx context.Context, tx *scope.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+Columns+` FROM tenant `+where, arg)
		if err != nil {
			return err
		}
		defer rows.Close()
		if !rows.Next() {
			return rows.Err()
		}
		found = true
		info, err = Scan(rows)
		return err
	})
	return info, found, err
}

func (r reader) Page(ctx context.Context, offset, limit int) (items []Info, total int, err error) {
	err = r.db.Read(ctx, func(ctx context.Context, tx *scope.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+Columns+` FROM tenant ORDER BY created_at DESC, id LIMIT $1 OFFSET $2`, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			i, err := Scan(rows)
			if err != nil {
				return err
			}
			items = append(items, i)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM tenant`).Scan(&total)
	})
	return items, total, err
}
