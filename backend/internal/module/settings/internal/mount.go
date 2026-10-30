package internal

import (
	"net/http"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/command"
	"github.com/open-enu/kernel/flags"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/platform"

	tenantc "github.com/open-enu/app/internal/module/tenant/contract"
)

// Mount wires the one handler and four routes.
func Mount(mux *http.ServeMux, svc *platform.Services) {
	repo := newRepository(svc)
	defs, byID := catalogue(svc.FlagDefinitions)
	command.Register(svc.Bus, handlers{repo: repo, defs: byID}.set)
	a := api{repo: repo, write: writer{bus: svc.Bus, repo: repo, tenants: tenantc.NewReader(svc)}, defs: defs, byID: byID, guard: auth.Guard{Audit: svc.Audit}}
	badRequest := func(w http.ResponseWriter, r *http.Request, err error) {
		httpx.WriteError(w, r, httpx.BadRequest(httpx.KeyBadRequest).WithCause(err))
	}
	strict := NewStrictHandlerWithOptions(a, nil, StrictHTTPServerOptions{RequestErrorHandlerFunc: badRequest, ResponseErrorHandlerFunc: httpx.WriteError})
	HandlerWithOptions(strict, StdHTTPServerOptions{BaseRouter: mux, ErrorHandlerFunc: badRequest})
}

// Store is the kernel's flag store over setting_override (module.Module.FlagStore).
func Store(svc *platform.Services) flags.Store { return newRepository(svc) }
