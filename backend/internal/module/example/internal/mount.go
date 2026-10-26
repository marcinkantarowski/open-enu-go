package internal

import (
	"net/http"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/command"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/outbox"
	"github.com/open-enu/kernel/platform"
)

// Mount wires the module: the repository over the platform's pool, the
// handlers on the bus, the job's worker on the outbox, the controller on
// the mux. Both `openenu api` and `openenu worker` call it - the api only
// inserts jobs, the worker only works them, and neither can disagree with
// the other about a type.
func Mount(mux *http.ServeMux, svc *platform.Services) {
	repo := newRepository(svc)
	h := handlers{repo: repo, outbox: svc.Outbox, progress: svc.Progress, search: svc.Search}
	command.Register(svc.Bus, h.create)
	command.Register(svc.Bus, h.rename)
	command.Register(svc.Bus, h.archiveRequested)
	command.Register(svc.Bus, h.archiveActive)
	command.Register(svc.Bus, h.sweep)
	outbox.Handle(svc.Outbox, archiveWorker{bus: svc.Bus, progress: svc.Progress}.run)

	a := api{bus: svc.Bus, repo: repo, flags: svc.Flags, guard: auth.Guard{Audit: svc.Audit}}
	badRequest := func(w http.ResponseWriter, r *http.Request, err error) {
		httpx.WriteError(w, r, httpx.BadRequest(httpx.KeyBadRequest).WithCause(err))
	}
	strict := NewStrictHandlerWithOptions(a, nil, StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  badRequest,
		ResponseErrorHandlerFunc: httpx.WriteError,
	})
	HandlerWithOptions(strict, StdHTTPServerOptions{BaseRouter: mux, ErrorHandlerFunc: badRequest})
}
