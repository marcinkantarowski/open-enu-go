package internal

import (
	"context"
	"net/http"
	"time"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/command"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/platform"

	"github.com/open-enu/app/internal/module/tenant/contract"
)

// Mount registers the four handlers on the bus. No routes: in the tenant
// realm a workspace is created by registering and changed by an operator.
func Mount(_ *http.ServeMux, svc *platform.Services) {
	h := handlers{repo: newRepository(svc), outbox: svc.Outbox, cache: svc.Cache, setup: svc.Setup, locale: svc.I18n.Default()}
	command.Register(svc.Bus, h.create)
	command.Register(svc.Bus, h.activate)
	command.Register(svc.Bus, h.suspend)
	command.Register(svc.Bus, h.delete)
}

// LocalePreference is the tenant's default locale for a request made inside
// one - below a person's own choice (Identity's preference runs first),
// above Accept-Language. Cached with Describe, so it costs a request nothing.
func LocalePreference(svc *platform.Services) httpx.LocalePreference {
	reader := contract.NewReader(svc)
	return func(r *http.Request) string {
		p, ok := auth.From(r.Context())
		if !ok || p.TenantID == "" {
			return ""
		}
		info, found, err := reader.Describe(r.Context(), p.TenantID)
		if err != nil || !found {
			return ""
		}
		return info.DefaultLocale
	}
}

// PurgeUnverified removes tenants pending for more than `days`, through the
// Delete command so each is audited and announced; dryRun only lists them.
func PurgeUnverified(ctx context.Context, svc *platform.Services, days int, dryRun bool) ([]contract.Info, error) {
	stale, err := newRepository(svc).stalePending(ctx, time.Now().Add(-time.Duration(days)*24*time.Hour))
	if err != nil || dryRun {
		return stale, err
	}
	p := contract.Provisioner{Bus: svc.Bus}
	for _, t := range stale {
		if err := p.Delete(ctx, t.ID); err != nil {
			return stale, err
		}
	}
	return stale, nil
}
