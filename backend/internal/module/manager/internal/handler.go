package internal

import (
	"context"
	"log/slog"

	"github.com/open-enu/kernel/scope"

	tenantc "github.com/open-enu/app/internal/module/tenant/contract"
)

// handlers are the only code that receives *scope.Tx (writeboundary).
// Both commands here are decisions rather than rows: the trail entry is
// the write, and the work happens in another module's command or in the
// token the controller mints afterwards.
type handlers struct {
	tenants tenantc.Provisioner
	log     *slog.Logger
}

func (h handlers) suspend(ctx context.Context, _ *scope.Tx, c suspendTenant) (struct{}, error) {
	return struct{}{}, h.tenants.Suspend(ctx, c.ID)
}

func (h handlers) startImpersonation(ctx context.Context, _ *scope.Tx, c startImpersonation) (struct{}, error) {
	h.log.WarnContext(ctx, "impersonation.started", "operator", c.OperatorID, "viewing_as", c.UserID, "tenant", c.TenantID)
	return struct{}{}, nil
}
