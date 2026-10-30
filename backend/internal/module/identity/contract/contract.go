// Package contract is the part of identity other modules import: who is a
// member of a workspace, and the one designed way to act as one of them.
// The types are aliases of internal's so there is exactly one definition;
// this package is the published door to it.
package contract

import (
	"context"

	"github.com/open-enu/kernel/command"
	"github.com/open-enu/kernel/platform"

	"github.com/open-enu/app/internal/module/identity/internal"
)

// Member is a person in a workspace as the operator console lists them:
// the USER's id, which is what impersonation takes.
type Member = internal.Member

// Directory answers who belongs to a workspace. Reads span tenants by
// nature (Identity's tables are not tenant-scoped) and are for the manager
// realm, whose authority is the realm itself.
type Directory = internal.Directory

// Workspace is one active membership of a person (WorkspacesOf): what a
// consultant's view across their clients starts from.
type Workspace = internal.Workspace

// Operator names the person behind an impersonated session.
type Operator = internal.Operator

// Minter issues sessions for a person another module vouches for, without
// their password.
//
//   - Impersonate (ADR-0020): marked `imp`, naming the operator in
//     `act`, with no refresh cookie, so it expires in fifteen minutes and
//     cannot renew itself.
//   - StartSession: an ordinary session - a login's answer, refresh cookie
//     included - for a person the calling module authenticated by other
//     means. The caller rate-limits its endpoint and answers every failure
//     alike; its page goes in `<InjectionPoint name="auth.login.methods">`.
type Minter = internal.Minter

// Started is a session StartSession opened. Write(w) answers the request
// as a login does; the calling module's response type wraps it.
type Started = internal.Started

// NewDirectory reads memberships through the platform's pool.
func NewDirectory(svc *platform.Services) Directory { return internal.NewDirectory(svc) }

// NewMinter mints with the platform's signer, under Identity's role rules.
func NewMinter(svc *platform.Services) Minter { return internal.NewMinter(svc) }

// EndTenantSessions is the command that revokes every refresh token whose
// current workspace is TenantID: a suspended workspace's members cannot
// renew into it. `identity.sessions.ended` on the trail.
type EndTenantSessions = internal.EndTenantSessions

// Sessions is the session commands as methods.
type Sessions struct{ Bus *command.Bus }

// EndTenant dispatches EndTenantSessions - inside the caller's transaction
// when the caller is itself a handler.
func (s Sessions) EndTenant(ctx context.Context, tenantID string) error {
	_, err := s.Bus.Dispatch(ctx, EndTenantSessions{TenantID: tenantID})
	return err
}

// AddMember and UnassignConsultant are the commands behind Members (ADR-0028);
// MemberAdded is what adding answers.
type (
	AddMember          = internal.AddMember
	MemberAdded        = internal.MemberAdded
	UnassignConsultant = internal.UnassignConsultant
)

// Members is who belongs to a workspace, as methods. Each dispatch joins the
// caller's transaction when the caller is itself a handler.
type Members struct{ Bus *command.Bus }

// Add puts a person in a workspace, with an account if they have none.
func (m Members) Add(ctx context.Context, c AddMember) (MemberAdded, error) {
	return command.Dispatch[MemberAdded](ctx, m.Bus, c)
}

// UnassignConsultant removes a consultant from a workspace.
func (m Members) UnassignConsultant(ctx context.Context, tenantID, userID string) error {
	_, err := m.Bus.Dispatch(ctx, UnassignConsultant{TenantID: tenantID, UserID: userID})
	return err
}
