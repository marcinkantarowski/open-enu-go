package internal

import (
	"net/http"

	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/platform"
)

// Mount wires the three routes. No command handlers: the module's writes
// are the notifier's (a side effect of an audited change) and read state.
func Mount(mux *http.ServeMux, svc *platform.Services) {
	cat := catalogue{}
	for _, d := range svc.NotificationDefinitions {
		cat[d.Type] = keys{title: d.TitleKey, body: d.BodyKey}
	}
	a := api{repo: newRepository(svc), cat: cat}
	badRequest := func(w http.ResponseWriter, r *http.Request, err error) {
		httpx.WriteError(w, r, httpx.BadRequest(httpx.KeyBadRequest).WithCause(err))
	}
	strict := NewStrictHandlerWithOptions(a, nil, StrictHTTPServerOptions{RequestErrorHandlerFunc: badRequest, ResponseErrorHandlerFunc: httpx.WriteError})
	HandlerWithOptions(strict, StdHTTPServerOptions{BaseRouter: mux, ErrorHandlerFunc: badRequest})
}
