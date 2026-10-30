package internal

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/command"
	"github.com/open-enu/kernel/crypto"
	"github.com/open-enu/kernel/httpx"

	auditc "github.com/open-enu/app/internal/module/audit/contract"
	identityc "github.com/open-enu/app/internal/module/identity/contract"
	tenantc "github.com/open-enu/app/internal/module/tenant/contract"
)

// service is what the controller holds: the other modules' contracts and
// the kernel pieces a thin controller may not import. Every cross-tenant
// read here goes through a contract that lifts isolation by name.
type service struct {
	bus     *command.Bus
	repo    repository
	signer  auth.Signer
	hasher  crypto.PasswordHasher
	tenants tenantc.Reader
	people  identityc.Directory
	minter  identityc.Minter
	audit   auditc.Reader
}

// login is an infrastructure write (the last-login stamp) outside the bus:
// the attempt itself is in the access log. One answer for every failure
// mode - unknown address, wrong password, deactivated account.
func (s service) login(ctx context.Context, email, password string) (OperatorSession, error) {
	o, err := s.repo.byEmailHash(ctx, s.repo.crypto.HashForLookup(email))
	if err != nil {
		s.hasher.Spend(password) // the same time as a wrong password
		return OperatorSession{}, httpx.Unauthenticated(keyInvalidCredentials)
	}
	if !s.hasher.Verify(o.Password, password) || !o.Active {
		return OperatorSession{}, httpx.Unauthenticated(keyInvalidCredentials)
	}
	// A verifier from an older algorithm or cost is replaced now, while the
	// password is in hand (ADR-0022); failing to hash leaves the old one.
	upgraded := ""
	if s.hasher.NeedsRehash(o.Password) {
		upgraded, _ = s.hasher.Hash(password)
	}
	if err := s.repo.recordLogin(ctx, o.ID, upgraded); err != nil {
		return OperatorSession{}, err
	}
	now := time.Now().UTC().Truncate(time.Second)
	o.LastLoginAt = &now
	// `aud: manager` is the claim that makes this token useless on the tenant
	// API, and a tenant token useless here (ADR-0019). The roles ride in
	// perms for the console to show; authority is the realm.
	token, err := s.signer.Sign(auth.Claims{Subject: o.ID, Audience: auth.RealmManager, Permissions: o.Roles}, accessTTL)
	if err != nil {
		return OperatorSession{}, err
	}
	return OperatorSession{Token: token, ExpiresIn: int(accessTTL.Seconds()), Manager: o.view()}, nil
}

func (s service) page(ctx context.Context, page, size int) (ManagedTenantList, error) {
	items, total, err := s.tenants.Page(ctx, (page-1)*size, size)
	if err != nil {
		return ManagedTenantList{}, err
	}
	consultants, err := s.people.Consultants(ctx)
	if err != nil {
		return ManagedTenantList{}, err
	}
	serving := map[string][]string{}
	for _, c := range consultants {
		serving[c.TenantID] = append(serving[c.TenantID], personName(c))
	}
	out := ManagedTenantList{Items: make([]ManagedTenant, 0, len(items)), Meta: ListMeta{Total: total, Page: page, Size: size, Pages: (total + size - 1) / size}}
	for _, t := range items {
		n, err := s.people.CountMembersOf(ctx, t.ID)
		if err != nil {
			return ManagedTenantList{}, err
		}
		out.Items = append(out.Items, ManagedTenant{Id: t.ID, Slug: t.Slug, Name: t.Name, Status: t.Status, Plan: t.Plan, DefaultLocale: t.DefaultLocale,
			Version: t.Version, CreatedAt: t.CreatedAt, ActivatedAt: t.ActivatedAt, Members: n, Consultants: append([]string{}, serving[t.ID]...)})
	}
	return out, nil
}

// tenant is the one workspace, or a 404.
func (s service) tenant(ctx context.Context, id string) (tenantc.Info, error) {
	t, found, err := s.tenants.Describe(ctx, id)
	if err != nil {
		return tenantc.Info{}, err
	}
	if !found {
		return tenantc.Info{}, httpx.NotFound(httpx.KeyNotFound)
	}
	return t, nil
}

func (s service) detail(ctx context.Context, id string) (ManagedTenantDetail, error) {
	t, err := s.tenant(ctx, id)
	if err != nil {
		return ManagedTenantDetail{}, err
	}
	members, err := s.people.MembersOf(ctx, t.ID)
	if err != nil {
		return ManagedTenantDetail{}, err
	}
	out := ManagedTenantDetail{Id: t.ID, Slug: t.Slug, Name: t.Name, Status: t.Status, Plan: t.Plan, DefaultLocale: t.DefaultLocale,
		Version: t.Version, CreatedAt: t.CreatedAt, ActivatedAt: t.ActivatedAt, Members: make([]TenantMember, 0, len(members))}
	for _, m := range members {
		out.Members = append(out.Members, TenantMember{Id: m.ID, Email: m.Email, DisplayName: m.DisplayName, Role: m.Role, Status: m.Status})
	}
	return out, nil
}

func (s service) suspend(ctx context.Context, id string) error {
	if _, err := s.tenant(ctx, id); err != nil {
		return err
	}
	_, err := s.bus.Dispatch(ctx, suspendTenant{ID: id})
	return err
}

// super reads the operator's row, not their token: a role taken away takes
// effect now, not when the token runs out.
func (s service) super(ctx context.Context, operatorID string) (operator, bool, error) {
	o, err := s.repo.byID(ctx, operatorID)
	if errors.Is(err, errNotFound) {
		return o, false, nil
	}
	if err != nil {
		return o, false, err
	}
	return o, o.Active && slices.Contains(o.Roles, roleSuper), nil
}

// personName is what a list shows for a person with no name.
func personName(m identityc.Member) string {
	if m.DisplayName != nil && *m.DisplayName != "" {
		return *m.DisplayName
	}
	return m.Email
}

func (s service) operators(ctx context.Context) (OperatorList, error) {
	all, err := s.repo.all(ctx)
	if err != nil {
		return OperatorList{}, err
	}
	out := OperatorList{Items: make([]Operator, 0, len(all))}
	for _, o := range all {
		out.Items = append(out.Items, o.view())
	}
	return out, nil
}

// consultants folds the memberships into one row per person, in the order they
// first came, with each workspace's name.
func (s service) consultants(ctx context.Context) (ConsultantList, error) {
	memberships, err := s.people.Consultants(ctx)
	if err != nil {
		return ConsultantList{}, err
	}
	out := ConsultantList{Items: []Consultant{}}
	at := map[string]int{}
	for _, m := range memberships {
		t, err := s.tenant(ctx, m.TenantID)
		if err != nil {
			return ConsultantList{}, err
		}
		i, seen := at[m.ID]
		if !seen {
			i = len(out.Items)
			at[m.ID] = i
			out.Items = append(out.Items, Consultant{Id: m.ID, Email: m.Email, DisplayName: m.DisplayName, Clients: []ConsultantClient{}})
		}
		out.Items[i].Clients = append(out.Items[i].Clients, ConsultantClient{TenantId: t.ID, Name: t.Name})
	}
	return out, nil
}

func (s service) createTenant(ctx context.Context, name, slug string) (CreatedTenant, error) {
	t, err := command.Dispatch[tenantc.Info](ctx, s.bus, createTenant{Name: name, Slug: slug})
	if err != nil {
		return CreatedTenant{}, err
	}
	return CreatedTenant{Id: t.ID}, nil
}

func (s service) addMember(ctx context.Context, operatorID, tenantID string, in AddMemberRequest) (AddedMember, error) {
	if _, err := s.tenant(ctx, tenantID); err != nil {
		return AddedMember{}, err
	}
	_, super, err := s.super(ctx, operatorID)
	if err != nil {
		return AddedMember{}, err
	}
	added, err := command.Dispatch[identityc.MemberAdded](ctx, s.bus, addMember{TenantID: tenantID, Email: string(in.Email),
		DisplayName: in.DisplayName, Role: string(in.Role), Super: super})
	if err != nil {
		return AddedMember{}, err
	}
	out := AddedMember{UserId: added.UserID}
	if added.SetPasswordURL != "" {
		out.SetPasswordUrl = &added.SetPasswordURL
	}
	return out, nil
}

func (s service) unassignConsultant(ctx context.Context, operatorID, tenantID, userID string) error {
	if _, err := s.tenant(ctx, tenantID); err != nil {
		return err
	}
	_, super, err := s.super(ctx, operatorID)
	if err != nil {
		return err
	}
	_, err = s.bus.Dispatch(ctx, unassignConsultant{TenantID: tenantID, UserID: userID, Super: super})
	return err
}

// impersonate audits the attempt before the token exists, then mints.
func (s service) impersonate(ctx context.Context, operatorID, tenantID, userID string) (map[string]any, error) {
	if _, err := s.tenant(ctx, tenantID); err != nil {
		return nil, err
	}
	o, super, err := s.super(ctx, operatorID)
	if err != nil {
		return nil, err
	}
	if _, err := s.bus.Dispatch(ctx, startImpersonation{TenantID: tenantID, UserID: userID, OperatorID: operatorID, Super: super}); err != nil {
		return nil, err
	}
	name := o.displayName()
	return s.minter.Impersonate(ctx, userID, tenantID, identityc.Operator{ID: operatorID, DisplayName: name})
}

func (s service) trail(ctx context.Context, tenantID *string, limit int) (ManagerAuditList, error) {
	entries, err := s.audit.Recent(ctx, tenantID, limit)
	if err != nil {
		return ManagerAuditList{}, err
	}
	out := ManagerAuditList{Items: make([]ManagerAuditEntry, 0, len(entries))}
	for _, e := range entries {
		out.Items = append(out.Items, ManagerAuditEntry{Id: e.ID, Action: e.Action, TenantId: e.TenantID, ActorId: e.ActorID, OnBehalfOfId: e.OnBehalfOfID,
			SubjectId: e.SubjectID, RequestId: e.RequestID, Succeeded: e.Succeeded, FailureReason: e.FailureReason, RecordedAt: e.RecordedAt})
	}
	return out, nil
}

// maxAuditLimit is the contract's page cap.
const maxAuditLimit = auditc.MaxLimit
