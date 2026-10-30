package internal

import (
	"context"
	"log/slog"

	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/i18n"

	"github.com/open-enu/kernel/scope"

	identityc "github.com/open-enu/app/internal/module/identity/contract"
	tenantc "github.com/open-enu/app/internal/module/tenant/contract"
)

// handlers are the only code that receives *scope.Tx (writeboundary).
// The commands here are decisions rather than rows: the trail entry is
// the write, and the work happens in another module's command or in the
// token the controller mints afterwards.
type handlers struct {
	tenants  tenantc.Provisioner
	sessions identityc.Sessions
	members  identityc.Members
	log      *slog.Logger
}

func (h handlers) suspend(ctx context.Context, _ *scope.Tx, c suspendTenant) (struct{}, error) {
	if err := h.tenants.Suspend(ctx, c.ID); err != nil {
		return struct{}{}, err
	}
	// The members are locked out at their next login; their refresh tokens
	// into this workspace end now, in the same transaction.
	return struct{}{}, h.sessions.EndTenant(ctx, c.ID)
}

const keySuperRequired i18n.Key = "manager.error.super_required"

// startImpersonation refuses a plain operator here, in the handler, so the
// refusal is on the trail as a failed `manager.impersonation.started`.
func (h handlers) startImpersonation(ctx context.Context, _ *scope.Tx, c startImpersonation) (struct{}, error) {
	if !c.Super {
		return struct{}{}, httpx.Forbidden(keySuperRequired)
	}
	h.log.WarnContext(ctx, "impersonation.started", "operator", c.OperatorID, "viewing_as", c.UserID, "tenant", c.TenantID)
	return struct{}{}, nil
}

func (h handlers) createTenant(ctx context.Context, _ *scope.Tx, c createTenant) (tenantc.Info, error) {
	t, err := h.tenants.Create(ctx, tenantc.Create{Name: c.Name, Slug: c.Slug})
	if err != nil {
		return tenantc.Info{}, err
	}
	return t, h.tenants.Activate(ctx, t.ID)
}

func (h handlers) addMember(ctx context.Context, _ *scope.Tx, c addMember) (identityc.MemberAdded, error) {
	if !c.Super {
		return identityc.MemberAdded{}, httpx.Forbidden(keySuperRequired)
	}
	return h.members.Add(ctx, identityc.AddMember{TenantID: c.TenantID, Email: c.Email, DisplayName: c.DisplayName, Role: c.Role})
}

func (h handlers) unassignConsultant(ctx context.Context, _ *scope.Tx, c unassignConsultant) (struct{}, error) {
	if !c.Super {
		return struct{}{}, httpx.Forbidden(keySuperRequired)
	}
	return struct{}{}, h.members.UnassignConsultant(ctx, c.TenantID, c.UserID)
}
