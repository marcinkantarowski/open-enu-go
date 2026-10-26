package internal

import (
	"net/http"

	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/platform"
)

// Mount wires the one route over every module's declarations
// (platform.Services.Searchable, the registry's).
func Mount(mux *http.ServeMux, svc *platform.Services) {
	a := api{repo: newRepository(svc)}
	for _, d := range svc.Searchable {
		a.types = append(a.types, declaration{entityType: d.EntityType, permission: d.Permission})
	}
	badRequest := func(w http.ResponseWriter, r *http.Request, err error) {
		httpx.WriteError(w, r, httpx.BadRequest(httpx.KeyBadRequest).WithCause(err))
	}
	strict := NewStrictHandlerWithOptions(a, nil, StrictHTTPServerOptions{RequestErrorHandlerFunc: badRequest, ResponseErrorHandlerFunc: httpx.WriteError})
	HandlerWithOptions(strict, StdHTTPServerOptions{BaseRouter: mux, ErrorHandlerFunc: badRequest})
}
