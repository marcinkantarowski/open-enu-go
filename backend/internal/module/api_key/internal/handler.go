package internal

import (
	"context"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/i18n"
	"github.com/open-enu/kernel/scope"
)

const (
	keyNameRequired        i18n.Key = "api_key.error.name_required"
	keyPermissionsRequired i18n.Key = "api_key.error.permissions_required"
	keyPermissionUnknown   i18n.Key = "api_key.error.permission_unknown"
	keyPermissionNotHeld   i18n.Key = "api_key.error.permission_not_held"
	keyNoTenant            i18n.Key = "api_key.error.no_tenant"
)

// handlers are the only code that receives *scope.Tx (writeboundary).
type handlers struct {
	repo     repository
	declared []string // every permission every module declares
}

func (h handlers) create(ctx context.Context, tx *scope.Tx, c createKey) (created, error) {
	tenant, ok := scope.TenantID(ctx)
	if !ok {
		return created{}, httpx.BadRequest(keyNoTenant)
	}
	name := strings.TrimSpace(c.Name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return created{}, httpx.BadRequest(keyNameRequired)
	}
	if len(c.Permissions) == 0 {
		return created{}, httpx.BadRequest(keyPermissionsRequired)
	}
	// Only what some module declared: a key for a permission nobody checks
	// is a key that does nothing, granted by a typo.
	perms := slices.Compact(sortedCopy(c.Permissions))
	for _, p := range perms {
		if !slices.Contains(h.declared, p) {
			return created{}, httpx.BadRequest(keyPermissionUnknown).WithDetails(map[string]any{"permission": p})
		}
	}
	// And only what the caller holds. A key with `api_key.manage` could
	// otherwise mint a key with anything, and a credential would outrank
	// the credential that created it. The console has no principal: it is
	// the server's own operator, who could write the row by hand anyway.
	if caller, ok := auth.From(ctx); ok {
		for _, p := range perms {
			if !caller.Has(p) {
				return created{}, httpx.Forbidden(keyPermissionNotHeld).WithDetails(map[string]any{"permission": p})
			}
		}
	}
	var expires *time.Time
	if c.ExpiresInDays != nil && *c.ExpiresInDays > 0 {
		t := time.Now().UTC().Add(time.Duration(*c.ExpiresInDays) * 24 * time.Hour).Truncate(time.Second)
		expires = &t
	}
	k, secret := newKey(tenant, name, perms, expires)
	if err := h.repo.insert(ctx, tx, k); err != nil {
		return created{}, err
	}
	tx.Insert(k)
	return created{key: k, secret: secret}, nil
}

func (h handlers) revoke(ctx context.Context, tx *scope.Tx, c revokeKey) (apiKey, error) {
	old, err := h.repo.get(ctx, tx, c.ID)
	if err != nil {
		return apiKey{}, notFound(err)
	}
	if old.Revoked {
		return old, nil
	}
	updated := old
	updated.Revoked = true
	if err := h.repo.revoke(ctx, tx, old.ID); err != nil {
		return apiKey{}, err
	}
	tx.Update(old, updated)
	return updated, nil
}

func sortedCopy(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}
