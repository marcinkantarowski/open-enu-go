package internal

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/open-enu/kernel/command"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/platform"

	auditc "github.com/open-enu/app/internal/module/audit/contract"
	identityc "github.com/open-enu/app/internal/module/identity/contract"
	tenantc "github.com/open-enu/app/internal/module/tenant/contract"
)

// Login gets the strictest limit in the system: five attempts, then one a
// minute. One operator account reaches every tenant's data.
const (
	LoginRatePerMinute = 1
	LoginBurst         = 5
)

// loginBurst is LoginBurst, unless MANAGER_LOGIN_BURST raises it for `make e2e`,
// which signs in from one address more often than any person. Never in
// production: make prod-check refuses it.
func loginBurst() int {
	if n, err := strconv.Atoi(os.Getenv("MANAGER_LOGIN_BURST")); err == nil && n > 0 {
		return n
	}
	return LoginBurst
}

// Mount wires the handlers and the routes.
func Mount(mux *http.ServeMux, svc *platform.Services) {
	repo := newRepository(svc)
	h := handlers{tenants: tenantc.Provisioner{Bus: svc.Bus}, sessions: identityc.Sessions{Bus: svc.Bus}, members: identityc.Members{Bus: svc.Bus}, log: svc.Log}
	command.Register(svc.Bus, h.suspend)
	command.Register(svc.Bus, h.startImpersonation)
	command.Register(svc.Bus, h.createTenant)
	command.Register(svc.Bus, h.addMember)
	command.Register(svc.Bus, h.unassignConsultant)
	a := api{svc: service{bus: svc.Bus, repo: repo, signer: svc.Signer, hasher: svc.Passwords, tenants: tenantc.NewReader(svc),
		people: identityc.NewDirectory(svc), minter: identityc.NewMinter(svc), audit: auditc.NewReader(svc)}}
	badRequest := func(w http.ResponseWriter, r *http.Request, err error) {
		httpx.WriteError(w, r, httpx.BadRequest(httpx.KeyBadRequest).WithCause(err))
	}
	inner := http.NewServeMux()
	strict := NewStrictHandlerWithOptions(a, nil, StrictHTTPServerOptions{RequestErrorHandlerFunc: badRequest, ResponseErrorHandlerFunc: httpx.WriteError})
	HandlerWithOptions(strict, StdHTTPServerOptions{BaseRouter: inner, ErrorHandlerFunc: badRequest})
	mux.Handle("/api/manager/login", httpx.RateLimit(LoginRatePerMinute, loginBurst())(inner))
	mux.Handle("/api/manager/", inner)
}

// CreateOperatorInput is `openenu manager create`'s.
type CreateOperatorInput struct {
	Email       string
	Password    string
	DisplayName string
	Super       bool
}

// CreateOperator makes an operator, or finds the one that already has the
// address: idempotent, so the e2e fixtures can run before every run. There
// is no operator signup endpoint and there should never be one; the first
// operator comes from the console, which needs shell access.
func CreateOperator(ctx context.Context, svc *platform.Services, in CreateOperatorInput) (id string, created bool, err error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if !strings.Contains(email, "@") {
		return "", false, fmt.Errorf("manager: %q is not an address", in.Email)
	}
	if len(in.Password) < MinPasswordLength {
		return "", false, fmt.Errorf("manager: an operator password must be at least %d characters", MinPasswordLength)
	}
	repo := newRepository(svc)
	hash := repo.crypto.HashForLookup(email)
	if existing, err := repo.byEmailHash(ctx, hash); err == nil {
		return existing.ID, false, nil
	} else if !errors.Is(err, errNotFound) {
		return "", false, err
	}
	verifier, err := svc.Passwords.Hash(in.Password)
	if err != nil {
		return "", false, err
	}
	var name *string
	if n := strings.TrimSpace(in.DisplayName); n != "" {
		name = &n
	}
	o := newOperator(email, hash, verifier, name, in.Super)
	if err := repo.insert(ctx, o); err != nil {
		return "", false, err
	}
	return o.ID, true, nil
}
