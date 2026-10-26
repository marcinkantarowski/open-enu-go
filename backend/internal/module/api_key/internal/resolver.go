package internal

import (
	"context"
	"log/slog"
	"time"

	"github.com/open-enu/kernel/auth"

	tenantc "github.com/open-enu/app/internal/module/tenant/contract"
)

// keys is the kernel's KeyResolver backed by the table: the middleware
// hands it the hash of a bearer `sk_...` and gets the principal - the key
// itself, not a person, with exactly the key's permissions. Unknown,
// revoked, expired and a suspended tenant all answer "no key" alike; the
// middleware turns that into the one 401.
type keys struct {
	repo    repository
	tenants tenantc.Reader
	log     *slog.Logger
}

var _ auth.KeyResolver = keys{}

func (k keys) Resolve(ctx context.Context, hash string) (auth.Principal, bool, error) {
	key, found, err := k.repo.resolve(ctx, hash)
	if err != nil || !found || !key.usable(time.Now()) {
		return auth.Principal{}, false, err
	}
	t, ok, err := k.tenants.Describe(ctx, key.TenantID)
	if err != nil || !ok || !t.Active() {
		return auth.Principal{}, false, err
	}
	if err := k.repo.touch(ctx, key); err != nil && k.log != nil {
		k.log.WarnContext(ctx, "api_key.touch_failed", "key", key.ID, "error", err.Error())
	}
	return auth.Principal{Subject: key.ID, TenantID: key.TenantID, Realm: auth.RealmTenant, Permissions: key.Permissions, APIKey: true}, true, nil
}
