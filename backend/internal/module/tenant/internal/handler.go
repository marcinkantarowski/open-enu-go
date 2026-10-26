package internal

import (
	"context"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/open-enu/kernel/cache"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/i18n"
	"github.com/open-enu/kernel/outbox"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/setup"

	"github.com/open-enu/app/internal/module/tenant/contract"
)

const (
	keyNameRequired i18n.Key = "tenant.error.name_required"
	keySlugInvalid  i18n.Key = "tenant.error.slug_invalid"
	keySlugTaken    i18n.Key = "tenant.error.slug_taken"
)

// handlers are the only code that receives *scope.Tx (writeboundary).
type handlers struct {
	repo   repository
	outbox *outbox.Outbox
	cache  *cache.Tenant
	setup  *setup.Runner
	locale string
}

func (h handlers) create(ctx context.Context, tx *scope.Tx, c contract.Create) (contract.Info, error) {
	t := newTenant(c, h.locale)
	if t.Name == "" || utf8.RuneCountInString(t.Name) > 200 {
		return contract.Info{}, httpx.BadRequest(keyNameRequired)
	}
	if !ValidSlug(t.Slug) {
		return contract.Info{}, httpx.BadRequest(keySlugInvalid)
	}
	if taken, err := h.repo.slugTaken(ctx, tx, t.Slug); err != nil {
		return contract.Info{}, err
	} else if taken {
		return contract.Info{}, httpx.NewError(http.StatusConflict, httpx.CodeConflict, keySlugTaken)
	}
	if err := h.repo.insert(ctx, tx, t); err != nil {
		return contract.Info{}, err
	}
	tx.Insert(t)
	if err := h.outbox.Publish(ctx, tx, contract.Created{ID: t.ID, Slug: t.Slug, Name: t.Name, At: t.CreatedAt}); err != nil {
		return contract.Info{}, err
	}
	// Every module's defaults, inside the new tenant's scope. A step that
	// writes through a command joins this transaction as a savepoint, so a
	// failure here rolls the tenant and every default back together -
	// partial provisioning is worse than none (kernel/setup). A step that
	// opened its own transaction instead would commit on its own, and hold a
	// second connection while this one waits.
	if h.setup != nil {
		if err := h.setup.OnTenantCreated(scope.WithTenant(ctx, t.ID), t.ID); err != nil {
			return contract.Info{}, err
		}
	}
	return t.Info, nil
}

func (h handlers) activate(ctx context.Context, tx *scope.Tx, c contract.Activate) (contract.Info, error) {
	old, err := h.repo.get(ctx, tx, c.ID)
	if err != nil {
		return contract.Info{}, notFound(err)
	}
	if old.Active() {
		return old.Info, nil
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	return h.transition(ctx, tx, old, contract.StatusActive, &now)
}

func (h handlers) suspend(ctx context.Context, tx *scope.Tx, c contract.Suspend) (contract.Info, error) {
	old, err := h.repo.get(ctx, tx, c.ID)
	if err != nil {
		return contract.Info{}, notFound(err)
	}
	if old.Status == contract.StatusSuspended {
		return old.Info, nil
	}
	return h.transition(ctx, tx, old, contract.StatusSuspended, old.ActivatedAt)
}

func (h handlers) transition(ctx context.Context, tx *scope.Tx, old tenant, status string, activatedAt *time.Time) (contract.Info, error) {
	updated := old
	updated.Status, updated.ActivatedAt, updated.Version = status, activatedAt, old.Version+1
	if err := h.repo.setStatus(ctx, tx, updated); err != nil {
		return contract.Info{}, err
	}
	tx.Update(old, updated)
	h.invalidateAfterCommit(tx, old.ID)
	return updated.Info, nil
}

// invalidateAfterCommit drops the tenant's cached entries once the change is
// visible. Dropping them inside the transaction leaves a window in which a
// concurrent read refills the cache from the old row - and a suspended
// tenant then keeps working until the entry expires.
func (h handlers) invalidateAfterCommit(tx *scope.Tx, id string) {
	tx.AfterCommit(func(ctx context.Context) error { return h.cache.InvalidateTenant(ctx, id) })
}

func (h handlers) delete(ctx context.Context, tx *scope.Tx, c contract.Delete) (contract.Info, error) {
	old, err := h.repo.get(ctx, tx, c.ID)
	if err != nil {
		return contract.Info{}, notFound(err)
	}
	// The event first, while the id still resolves for whoever hears it.
	if err := h.outbox.Publish(ctx, tx, contract.Deleted{ID: old.ID, Slug: old.Slug, At: time.Now().UTC()}); err != nil {
		return contract.Info{}, err
	}
	if err := h.repo.remove(ctx, tx, old.ID); err != nil {
		return contract.Info{}, err
	}
	tx.Delete(old)
	h.invalidateAfterCommit(tx, old.ID)
	return old.Info, nil
}
