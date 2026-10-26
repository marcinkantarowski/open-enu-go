package internal

import (
	"context"
	"encoding/json"
	"time"

	"github.com/open-enu/kernel/cache"
	"github.com/open-enu/kernel/flags"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
)

// cacheTTL bounds how stale a resolved flag may be when the invalidation
// did not reach a node; the write invalidates by tag, so normally it is
// fresh on the next request.
const cacheTTL = time.Minute

// repository is the only file with SQL. setting_override is under row-level
// security; every read and write happens in the tenant's scope, which the
// store enters itself because the resolver names the tenant explicitly.
type repository struct {
	db    *scope.Pool
	cache *cache.Tenant
}

func newRepository(svc *platform.Services) repository {
	return repository{db: svc.DB, cache: svc.Cache}
}

const columns = `id, tenant_id, identifier, value, updated_at`

func (r repository) get(ctx context.Context, tx *scope.Tx, identifier string) (override, bool, error) {
	rows, err := tx.Query(ctx, `SELECT `+columns+` FROM setting_override WHERE identifier = $1`, identifier)
	if err != nil {
		return override{}, false, err
	}
	items, err := scan(rows)
	if err != nil || len(items) == 0 {
		return override{}, false, err
	}
	return items[0], true, nil
}

// forTenant is every override in the ambient tenant's scope.
func (r repository) forTenant(ctx context.Context) (out map[string]any, err error) {
	out = map[string]any{}
	err = r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+columns+` FROM setting_override`)
		if err != nil {
			return err
		}
		items, err := scan(rows)
		for _, o := range items {
			out[o.Identifier] = o.Value
		}
		return err
	})
	return out, err
}

func (r repository) insert(ctx context.Context, tx *scope.Tx, o override) error {
	_, err := tx.Exec(ctx, `INSERT INTO setting_override (`+columns+`) VALUES ($1, $2, $3, $4, $5)`,
		o.ID, o.TenantID, o.Identifier, box(o.Value), o.UpdatedAt)
	return err
}

func (r repository) update(ctx context.Context, tx *scope.Tx, o override) error {
	_, err := tx.Exec(ctx, `UPDATE setting_override SET value = $2, updated_at = $3 WHERE id = $1`, o.ID, box(o.Value), o.UpdatedAt)
	return err
}

// invalidate drops what the store cached for this tenant and this flag.
// Both tags, because both caches are now wrong; called after the commit,
// so no reader can re-cache the old row in between.
func (r repository) invalidate(ctx context.Context, tenantID, identifier string) error {
	if err := r.cache.InvalidateTenant(ctx, tenantID); err != nil {
		return err
	}
	return r.cache.InvalidateModule(ctx, "settings")
}

// Override implements flags.Store: the tenant's value, cached a minute
// under the tenant's tag and the flag's own.
func (r repository) Override(ctx context.Context, tenantID, identifier string) (any, bool, error) {
	ctx = scope.WithTenant(ctx, tenantID)
	b, err := cache.GetJSON(ctx, r.cache, "settings", "flag."+identifier, cacheTTL, func(ctx context.Context) (boxed, error) {
		var b boxed
		err := r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
			o, found, err := r.get(ctx, tx, identifier)
			b = boxed{Found: found, Value: o.Value}
			return err
		})
		return b, err
	}, "settings."+identifier)
	if err != nil {
		return nil, false, err
	}
	return b.Value, b.Found, nil
}

var _ flags.Store = repository{}

func box(v any) []byte {
	b, _ := json.Marshal(map[string]any{"value": v})
	return b
}

func scan(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close()
}) ([]override, error) {
	defer rows.Close()
	var out []override
	for rows.Next() {
		var o override
		var raw []byte
		if err := rows.Scan(&o.ID, &o.TenantID, &o.Identifier, &raw, &o.UpdatedAt); err != nil {
			return nil, err
		}
		var b map[string]any
		if err := json.Unmarshal(raw, &b); err != nil {
			return nil, err
		}
		o.Value = b["value"]
		out = append(out, o)
	}
	return out, rows.Err()
}
