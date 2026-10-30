package internal

import (
	"context"

	"github.com/open-enu/kernel/platform"
)

// Member is contract.Member.
type Member struct {
	ID          string  `json:"id"`
	TenantID    string  `json:"tenantId"`
	Email       string  `json:"email"`
	DisplayName *string `json:"displayName"`
	Role        string  `json:"role"`
	Status      string  `json:"status"`
	// Locale is the person's stored choice, nil when they never chose: what
	// a mail to them is written in.
	Locale *string `json:"locale"`
}

// Directory is contract.Directory.
type Directory interface {
	MembersOf(ctx context.Context, tenantID string) ([]Member, error)
	CountMembersOf(ctx context.Context, tenantID string) (int, error)
	WorkspacesOf(ctx context.Context, userID string) ([]Workspace, error)
	// Consultants is every consultant's membership, one Member per workspace.
	Consultants(ctx context.Context) ([]Member, error)
}

// Workspace is contract.Workspace.
type Workspace struct{ TenantID, Role string }

// Operator is contract.Operator.
type Operator struct {
	ID          string
	DisplayName string
}

// Minter is contract.Minter: sessions for a person another module vouches
// for. Impersonate's session is the login body with `impersonating: true`
// and the operator added, as the tenant app's /impersonate page expects it.
// StartSession's is a login's, refresh cookie included.
type Minter interface {
	Impersonate(ctx context.Context, userID, tenantID string, op Operator) (map[string]any, error)
	StartSession(ctx context.Context, userID, tenantID string) (Started, error)
}

type directory struct{ repo repository }

// NewDirectory is contract.NewDirectory.
func NewDirectory(svc *platform.Services) Directory { return directory{repo: newRepository(svc)} }

func (d directory) MembersOf(ctx context.Context, tenantID string) ([]Member, error) {
	return d.repo.membersOf(ctx, tenantID)
}

func (d directory) CountMembersOf(ctx context.Context, tenantID string) (int, error) {
	return d.repo.countMembers(ctx, tenantID)
}

func (d directory) Consultants(ctx context.Context) ([]Member, error) {
	return d.repo.consultants(ctx)
}

func (d directory) WorkspacesOf(ctx context.Context, userID string) ([]Workspace, error) {
	ms, err := d.repo.membershipsOf(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := []Workspace{}
	for _, m := range ms {
		if m.Status == membershipActive {
			out = append(out, Workspace{TenantID: m.TenantID, Role: m.Role})
		}
	}
	return out, nil
}
