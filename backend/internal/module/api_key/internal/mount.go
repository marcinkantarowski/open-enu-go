package internal

import (
	"context"
	"net/http"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/command"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"

	tenantc "github.com/open-enu/app/internal/module/tenant/contract"
)

// Mount wires the module: two handlers, three routes.
func Mount(mux *http.ServeMux, svc *platform.Services) {
	repo := newRepository(svc)
	h := handlers{repo: repo, declared: svc.Permissions}
	command.Register(svc.Bus, h.create)
	command.Register(svc.Bus, h.revoke)
	a := api{bus: svc.Bus, repo: repo, i18n: svc.I18n, guard: auth.Guard{Audit: svc.Audit}}
	badRequest := func(w http.ResponseWriter, r *http.Request, err error) {
		httpx.WriteError(w, r, httpx.BadRequest(httpx.KeyBadRequest).WithCause(err))
	}
	strict := NewStrictHandlerWithOptions(a, nil, StrictHTTPServerOptions{RequestErrorHandlerFunc: badRequest, ResponseErrorHandlerFunc: httpx.WriteError})
	HandlerWithOptions(strict, StdHTTPServerOptions{BaseRouter: mux, ErrorHandlerFunc: badRequest})
}

// Keys is the resolver the auth middleware consults (module.Module.Keys).
func Keys(svc *platform.Services) auth.KeyResolver {
	return keys{repo: newRepository(svc), tenants: tenantc.NewReader(svc), log: svc.Log}
}

// CreateKeyInput is `openenu apikey create`.
type CreateKeyInput struct {
	Tenant      string // id or slug
	Name        string
	Permissions []string
	Days        int
}

// CreateKey mints a key for a tenant named by id or slug and returns the
// secret - printed once by the console, kept by nobody else.
func CreateKey(ctx context.Context, svc *platform.Services, in CreateKeyInput) (secret string, key ApiKey, err error) {
	tenants := tenantc.NewReader(svc)
	t, found, err := tenants.Describe(ctx, in.Tenant)
	if err != nil {
		return "", ApiKey{}, err
	}
	if !found {
		if t, found, err = tenants.BySlug(ctx, in.Tenant); err != nil {
			return "", ApiKey{}, err
		} else if !found {
			return "", ApiKey{}, httpx.NotFound(httpx.KeyNotFound)
		}
	}
	cmd := createKey{Name: in.Name, Permissions: in.Permissions}
	if in.Days > 0 {
		cmd.ExpiresInDays = &in.Days
	}
	c, err := command.Dispatch[created](scope.WithTenant(ctx, t.ID), svc.Bus, cmd)
	if err != nil {
		return "", ApiKey{}, err
	}
	return c.secret, c.key.view(), nil
}
