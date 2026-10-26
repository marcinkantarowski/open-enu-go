package internal

import (
	"net/http"

	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/platform"
)

// Mount wires the one route.
func Mount(mux *http.ServeMux, svc *platform.Services) {
	badRequest := func(w http.ResponseWriter, r *http.Request, err error) {
		httpx.WriteError(w, r, httpx.BadRequest(httpx.KeyBadRequest).WithCause(err))
	}
	strict := NewStrictHandlerWithOptions(api{repo: newRepository(svc)}, nil, StrictHTTPServerOptions{RequestErrorHandlerFunc: badRequest, ResponseErrorHandlerFunc: httpx.WriteError})
	HandlerWithOptions(strict, StdHTTPServerOptions{BaseRouter: mux, ErrorHandlerFunc: badRequest})
}
