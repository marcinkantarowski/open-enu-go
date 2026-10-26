package internal

import (
	"context"
	"net/http"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/command"
	"github.com/open-enu/kernel/gdpr"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/outbox"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"

	tenantc "github.com/open-enu/app/internal/module/tenant/contract"
)

// AuthRatePerMinute and AuthBurst bound every /api/auth/* call per client:
// the limiter is what makes "one 401 for every failure" hold against a
// client that can try a million times.
const (
	AuthRatePerMinute = 60
	AuthBurst         = 30
)

// Mount wires the module. The auth routes sit behind the rate limiter; the
// profile does not - it is behind a bearer token already.
func Mount(mux *http.ServeMux, svc *platform.Services) {
	repo := newRepository(svc)
	h := handlers{repo: repo, tenants: tenantc.Provisioner{Bus: svc.Bus}, hasher: svc.Passwords,
		mail: mailer{mail: svc.Mail, i18n: svc.I18n, appURL: svc.URLs.App, log: svc.Log}}
	command.Register(svc.Bus, h.register)
	command.Register(svc.Bus, h.verify)
	command.Register(svc.Bus, h.requestReset)
	command.Register(svc.Bus, h.reset)
	command.Register(svc.Bus, h.updateProfile)
	command.Register(svc.Bus, h.removeTenantMemberships)
	command.Register(svc.Bus, h.createUser)
	command.Register(svc.Bus, h.erase)
	// A deleted workspace takes its memberships with it.
	svc.Outbox.Subscribe("tenant.deleted", func(ctx context.Context, d outbox.Delivered) error {
		id, _ := d.Payload()["tenantId"].(string)
		_, err := svc.Bus.Dispatch(ctx, removeTenantMemberships{TenantID: id})
		return err
	})

	a := api{bus: svc.Bus, repo: repo, locales: svc.I18n.Enabled(), sessions: sessions{
		repo: repo, tenants: tenantc.NewReader(svc), signer: svc.Signer, hasher: svc.Passwords,
		perms: newResolver(svc.Permissions), secure: svc.URLs.Secure(),
	}}
	badRequest := func(w http.ResponseWriter, r *http.Request, err error) {
		httpx.WriteError(w, r, httpx.BadRequest(httpx.KeyBadRequest).WithCause(err))
	}
	inner := http.NewServeMux()
	strict := NewStrictHandlerWithOptions(a, nil, StrictHTTPServerOptions{RequestErrorHandlerFunc: badRequest, ResponseErrorHandlerFunc: httpx.WriteError})
	HandlerWithOptions(strict, StdHTTPServerOptions{BaseRouter: inner, ErrorHandlerFunc: badRequest})
	mux.Handle("/api/auth/", httpx.RateLimit(AuthRatePerMinute, AuthBurst)(inner))
	mux.Handle("/api/profile", inner)
}

// LocalePreference is the person's stored locale, above the tenant's
// default and Accept-Language. One indexed read per request.
func LocalePreference(svc *platform.Services) httpx.LocalePreference {
	repo := newRepository(svc)
	return func(r *http.Request) string {
		p, ok := auth.From(r.Context())
		if !ok || p.APIKey || p.Subject == "" {
			return ""
		}
		locale, err := repo.localeOf(r.Context(), p.Subject)
		if err != nil {
			return ""
		}
		return locale
	}
}

// NewGdpr is the module's subject.
func NewGdpr(svc *platform.Services) gdpr.Subject {
	return subject{repo: newRepository(svc), bus: svc.Bus}
}

// CreateUserInput is `openenu user create`: an active account with a
// membership in a tenant named by slug, created and activated when missing.
type CreateUserInput struct {
	Email       string
	Password    string
	DisplayName string
	TenantSlug  string
	TenantName  string
	Role        string
	Seed        bool
}

// CreateUserResult says what exists afterwards.
type CreateUserResult struct {
	UserID   string
	TenantID string
	Seeded   bool
}

// CreateUser is idempotent end to end: the tenant by slug, the person by
// address, the membership by the pair. With Seed, every module's
// SeedExamples runs in the tenant's scope.
func CreateUser(ctx context.Context, svc *platform.Services, in CreateUserInput) (CreateUserResult, error) {
	tenants, prov := tenantc.NewReader(svc), tenantc.Provisioner{Bus: svc.Bus}
	t, found, err := tenants.BySlug(ctx, in.TenantSlug)
	if err != nil {
		return CreateUserResult{}, err
	}
	if !found {
		name := in.TenantName
		if name == "" {
			name = in.TenantSlug
		}
		if t, err = prov.Create(ctx, tenantc.Create{Slug: in.TenantSlug, Name: name}); err != nil {
			return CreateUserResult{}, err
		}
	}
	if !t.Active() {
		if err := prov.Activate(ctx, t.ID); err != nil {
			return CreateUserResult{}, err
		}
	}
	var name *string
	if in.DisplayName != "" {
		name = &in.DisplayName
	}
	u, err := command.Dispatch[user](ctx, svc.Bus, createUser{Email: in.Email, Password: in.Password, DisplayName: name, TenantID: t.ID, Role: in.Role})
	if err != nil {
		return CreateUserResult{}, err
	}
	out := CreateUserResult{UserID: u.ID, TenantID: t.ID}
	if in.Seed && svc.Setup != nil {
		if err := svc.Setup.SeedExamples(scope.WithTenant(ctx, t.ID), t.ID); err != nil {
			return out, err
		}
		out.Seeded = true
	}
	return out, nil
}
