package internal

import (
	"context"
	"net/http"
	"time"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/command"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
)

// SweepAfterHours is how long an unclaimed upload waits for an owner.
const SweepAfterHours = 24

// Mount wires the two handlers and two routes.
func Mount(mux *http.ServeMux, svc *platform.Services) {
	repo := newRepository(svc)
	if svc.MaxUploadBytes > 0 {
		MaxBytes = svc.MaxUploadBytes
	}
	h := handlers{repo: repo, storage: svc.Storage}
	command.Register(svc.Bus, h.upload)
	command.Register(svc.Bus, h.remove)
	command.Register(svc.Bus, h.claim)
	badRequest := func(w http.ResponseWriter, r *http.Request, err error) {
		httpx.WriteError(w, r, httpx.BadRequest(httpx.KeyBadRequest).WithCause(err))
	}
	strict := NewStrictHandlerWithOptions(api{bus: svc.Bus, repo: repo, guard: auth.Guard{Audit: svc.Audit}}, nil, StrictHTTPServerOptions{RequestErrorHandlerFunc: badRequest, ResponseErrorHandlerFunc: httpx.WriteError})
	// The pipeline caps every body at httpx.DefaultMaxBody; an upload may
	// carry a file of MaxBytes plus the multipart framing and the two short
	// fields around it. readUpload still refuses a file over MaxBytes itself.
	allow := MiddlewareFunc(httpx.AllowBody(MaxBytes + httpx.DefaultMaxBody))
	HandlerWithOptions(strict, StdHTTPServerOptions{BaseRouter: mux, ErrorHandlerFunc: badRequest, Middlewares: []MiddlewareFunc{allow}})
}

// SweepOrphans deletes uploads nobody claimed within `hours`, each through
// the delete command in its own tenant's scope, so each is audited where
// it belongs. dryRun lists what would go.
func SweepOrphans(ctx context.Context, svc *platform.Services, hours int, dryRun bool) ([]string, error) {
	found, err := newRepository(svc).orphans(ctx, time.Now().Add(-time.Duration(hours)*time.Hour))
	ids := make([]string, 0, len(found))
	for _, o := range found {
		ids = append(ids, o.ID)
	}
	if err != nil || dryRun {
		return ids, err
	}
	for _, o := range found {
		if _, err := svc.Bus.Dispatch(scope.WithTenant(ctx, o.TenantID), deleteAttachment{ID: o.ID}); err != nil {
			return ids, err
		}
	}
	return ids, nil
}
