// Package auth is who is asking: the principal behind a request, how it is
// verified, and what a controller may ask of it.
//
// Two credentials, one shape. A JWT (claims sub, tid, aud as the realm,
// perms, imp + act for impersonation) or an API key
// (`sk_...`, resolved by hash through a KeyResolver the ApiKey module
// provides) both become a Principal on the context, and the tenant it names
// becomes the scope every query runs under. Issuance - login, refresh,
// cookies - is the Identity module's; this package verifies, and signs.
//
// Signing lives here because issuance and verification must agree on one
// algorithm and one key: HS256 over the one APP_SECRET.
// Identity mints with the Signer the platform hands it
// (platform.Services.Signer); a test mints with the same type under a test
// key.
package auth

import (
	"context"
	"slices"

	"github.com/open-enu/kernel/audit"
	"github.com/open-enu/kernel/httpx"
)

// Realms are the audiences a token is minted for (ADR-0019).
const (
	RealmTenant  = "tenant"
	RealmManager = "manager"
)

// Principal is the verified caller.
type Principal struct {
	Subject     string   // the user's or the key's id
	TenantID    string   // the tenant the token was issued for; "" for an operator
	Realm       string   // RealmTenant or RealmManager
	Permissions []string // what the role or the key's scope grants
	// Impersonator is the operator behind an impersonated session, "" for
	// a session that is its own (the `act.sub` claim).
	Impersonator string
	APIKey       bool // true when a key, not a person, is calling
}

// Has reports whether the principal holds perm.
func (p Principal) Has(perm string) bool { return slices.Contains(p.Permissions, perm) }

// Impersonated reports whether somebody else is wearing this face.
func (p Principal) Impersonated() bool { return p.Impersonator != "" }

type principalKey struct{}

// WithPrincipal puts p on the context. The middleware does this; a test may.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// From returns the request's principal, if it was authenticated.
func From(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

// Require is the tenant controller's permission check: 401 with no
// principal or one from another realm, 403 without the permission, nil
// otherwise. The error is already an *httpx.Error, so `return nil, err` is
// the whole handling. Both realms verify against the same key, so this
// realm check IS what keeps an operator token off the tenant API
// (ADR-0019); an impersonation token is minted in the tenant realm and
// passes it honestly.
func Require(ctx context.Context, perm string) (Principal, error) {
	p, err := RequireRealm(ctx, RealmTenant)
	if err != nil {
		return Principal{}, err
	}
	if !p.Has(perm) {
		return p, httpx.Forbidden(httpx.KeyForbidden)
	}
	return p, nil
}

// RequireRealm is the principal of the given realm, or a 401 that does not
// say whether there was no token or a token for the other realm.
func RequireRealm(ctx context.Context, realm string) (Principal, error) {
	p, ok := From(ctx)
	if !ok || p.Realm != realm {
		return Principal{}, httpx.Unauthenticated(httpx.KeyUnauthenticated)
	}
	return p, nil
}

// RequireOperator is the manager realm's check: an operator, or 401.
// Operator authority is the realm itself, not a dotted permission - no
// tenant role can satisfy it, whatever a controller asks for.
func RequireOperator(ctx context.Context) (Principal, error) {
	return RequireRealm(ctx, RealmManager)
}

// Guard refuses an action to an impersonated session (ADR-0020:
// support may look, support may not act in someone else's name) and
// records the refusal in the trail with both identities - "what did the
// operator try while viewing as this user" is the question an incident asks.
type Guard struct{ Audit audit.Logger }

// DenyImpersonated returns a 403 when the principal is impersonated, after
// recording who tried what.
func (g Guard) DenyImpersonated(ctx context.Context, action string) error {
	p, ok := From(ctx)
	if !ok || !p.Impersonated() {
		return nil
	}
	if g.Audit != nil {
		g.Audit.Record(ctx, audit.Entry{
			Action: action, ActorID: p.Subject, OnBehalfOf: p.Impersonator, TenantID: p.TenantID,
			RequestID: httpx.RequestIDFrom(ctx),
		}.Failed("denied under impersonation"))
	}
	return httpx.Forbidden(KeyImpersonationDenied)
}

// Actor answers the audit trail's question from the principal: the subject
// acts, the impersonator is on-behalf-of.
type Actor struct{}

// Actor implements audit.ActorProvider.
func (Actor) Actor(ctx context.Context) audit.Actor {
	p, ok := From(ctx)
	if !ok {
		return audit.Actor{}
	}
	return audit.Actor{ID: p.Subject, OnBehalfOf: p.Impersonator}
}
