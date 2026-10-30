package internal

import (
	"net/http"
	"os"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/command"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/outbox"
	"github.com/open-enu/kernel/platform"
)

// Mount wires the module: handlers on the bus, the publisher on every
// event, the deliverer on the job, the controller on the mux. Both `openenu
// api` and `openenu worker` call it; only the worker runs the jobs.
func Mount(mux *http.ServeMux, svc *platform.Services) {
	// WEBHOOK_ALLOW_PRIVATE lets an endpoint live on a private network: the
	// dev stack's echo receiver does. Never in production (outbound.go).
	mount(mux, svc, outbound{allowPrivate: os.Getenv("WEBHOOK_ALLOW_PRIVATE") == "true"})
}

// mount is Mount with the outbound policy the deliverer posts under: a test
// hands it one that trusts its own receiver's certificate, or resolves a
// name where the test says.
func mount(mux *http.ServeMux, svc *platform.Services, o outbound) {
	repo := newRepository(svc)
	h := handlers{repo: repo, outbox: svc.Outbox, policy: o}
	command.Register(svc.Bus, h.create)
	command.Register(svc.Bus, h.deactivate)
	command.Register(svc.Bus, h.redeliver)
	d := newDeliverer(svc, o)
	svc.Outbox.SubscribeAll(d.publish)
	outbox.Handle(svc.Outbox, d.run)

	a := api{bus: svc.Bus, repo: repo, guard: auth.Guard{Audit: svc.Audit}, i18n: svc.I18n}
	badRequest := func(w http.ResponseWriter, r *http.Request, err error) {
		httpx.WriteError(w, r, httpx.BadRequest(httpx.KeyBadRequest).WithCause(err))
	}
	strict := NewStrictHandlerWithOptions(a, nil, StrictHTTPServerOptions{RequestErrorHandlerFunc: badRequest, ResponseErrorHandlerFunc: httpx.WriteError})
	HandlerWithOptions(strict, StdHTTPServerOptions{BaseRouter: mux, ErrorHandlerFunc: badRequest})
}

func newDeliverer(svc *platform.Services, o outbound) deliverer {
	return deliverer{repo: newRepository(svc), http: o.client(), policy: o, notifier: svc.Notifier, log: svc.Log}
}
