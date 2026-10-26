package internal

import (
	"context"

	"github.com/open-enu/kernel/flags"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/scope"
)

// handlers are the only code that receives *scope.Tx (writeboundary).
type handlers struct {
	repo repository
	defs map[string]flags.Definition
}

func (h handlers) set(ctx context.Context, tx *scope.Tx, c setTenantFlag) (override, error) {
	if _, ok := h.defs[c.Identifier]; !ok {
		return override{}, httpx.NotFound(keyUnknown)
	}
	// The row carries the tenant the policy sees; the caller entered that
	// scope before dispatching, and a mismatch is a bug, not a request.
	if tid, ok := scope.TenantID(ctx); !ok || tid != c.TenantID {
		return override{}, httpx.BadRequest(keyWrongScope)
	}
	old, found, err := h.repo.get(ctx, tx, c.Identifier)
	if err != nil {
		return override{}, err
	}
	if !found {
		o := newOverride(c.TenantID, c.Identifier, c.Value)
		if err := h.repo.insert(ctx, tx, o); err != nil {
			return override{}, err
		}
		tx.Insert(o)
		return o, nil
	}
	updated := old
	updated.Value = c.Value
	updated.UpdatedAt = newOverride("", "", nil).UpdatedAt
	if err := h.repo.update(ctx, tx, updated); err != nil {
		return override{}, err
	}
	tx.Update(old, updated)
	return updated, nil
}
