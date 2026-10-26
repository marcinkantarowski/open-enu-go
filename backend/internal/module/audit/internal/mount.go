package internal

import (
	"net/http"

	"github.com/open-enu/kernel/command"
	"github.com/open-enu/kernel/gdpr"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/platform"

	"github.com/open-enu/app/internal/module/audit/contract"
)

// Mount wires the one route and the one handler.
func Mount(mux *http.ServeMux, svc *platform.Services) {
	command.Register(svc.Bus, handlers{repo: newRepository(svc)}.anonymise)
	badRequest := func(w http.ResponseWriter, r *http.Request, err error) {
		httpx.WriteError(w, r, httpx.BadRequest(httpx.KeyBadRequest).WithCause(err))
	}
	strict := NewStrictHandlerWithOptions(api{reader: contract.NewReader(svc)}, nil, StrictHTTPServerOptions{RequestErrorHandlerFunc: badRequest, ResponseErrorHandlerFunc: httpx.WriteError})
	HandlerWithOptions(strict, StdHTTPServerOptions{BaseRouter: mux, ErrorHandlerFunc: badRequest})
}

// NewGdpr is the module's subject.
func NewGdpr(svc *platform.Services) gdpr.Subject {
	return subject{repo: newRepository(svc), bus: svc.Bus}
}
