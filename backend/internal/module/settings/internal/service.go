package internal

import (
	"context"

	"github.com/open-enu/kernel/command"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/scope"

	tenantc "github.com/open-enu/app/internal/module/tenant/contract"
)

// writer is what the controller holds instead of the bus, the scope and
// another module's contract: it dispatches the command in the right
// tenant's scope and, once it has committed, drops the caches - a kill
// switch takes effect on the next request, not when an entry expires.
type writer struct {
	bus     *command.Bus
	repo    repository
	tenants tenantc.Reader
}

// own sets a value for the caller's workspace, whose scope is ambient.
func (w writer) own(ctx context.Context, tenantID, identifier string, value any) (override, error) {
	return w.set(ctx, tenantID, identifier, value)
}

// forTenant is the operator's write: the operator is inside no tenant, so
// the target must exist and the write enters its scope, which is also
// what puts the right tenant_id on the row.
func (w writer) forTenant(ctx context.Context, tenantID, identifier string, value any) (override, error) {
	if _, found, err := w.tenants.Describe(ctx, tenantID); err != nil {
		return override{}, err
	} else if !found {
		return override{}, httpx.NotFound(httpx.KeyNotFound)
	}
	return w.set(scope.WithTenant(ctx, tenantID), tenantID, identifier, value)
}

func (w writer) set(ctx context.Context, tenantID, identifier string, value any) (override, error) {
	o, err := command.Dispatch[override](ctx, w.bus, setTenantFlag{TenantID: tenantID, Identifier: identifier, Value: value})
	if err != nil {
		return override{}, err
	}
	return o, w.repo.invalidate(ctx, tenantID, identifier)
}
