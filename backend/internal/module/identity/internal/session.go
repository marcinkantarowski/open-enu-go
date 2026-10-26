package internal

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/crypto"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/i18n"
	"github.com/open-enu/kernel/platform"

	tenantc "github.com/open-enu/app/internal/module/tenant/contract"
)

const (
	keyInvalidCredentials i18n.Key = "identity.error.invalid_credentials"
	keyNotAMember         i18n.Key = "identity.error.not_a_member"

	// RefreshCookie is the cookie's name; its path keeps it off every other
	// route (ADR-0018).
	RefreshCookie = "open_enu_refresh"
	refreshPath   = "/api/auth"
)

// sessions issues and rotates. Infrastructure writes, outside the bus: a
// login is stamped (last_login_at) and a rotation is a row, but neither is
// an audit entry.
type sessions struct {
	repo    repository
	tenants tenantc.Reader
	signer  auth.Signer
	hasher  crypto.PasswordHasher
	perms   resolver
	secure  bool
}

// login is ONE failure for no such person, wrong password and not active;
// distinguishing them makes the endpoint an address oracle. The same answer
// in the same time: the password is checked before whether the account is
// active, and an address nobody has spends a verification too.
func (s sessions) login(ctx context.Context, email, password string, tenantID *string) (Session, *http.Cookie, error) {
	u, err := s.repo.userByEmailHash(ctx, emailHash(email))
	if err != nil {
		s.hasher.Spend(password)
		return Session{}, nil, httpx.Unauthenticated(keyInvalidCredentials)
	}
	if !s.hasher.Verify(u.Password, password) || !u.active() {
		return Session{}, nil, httpx.Unauthenticated(keyInvalidCredentials)
	}
	if err := s.repo.recordLogin(ctx, u.ID, s.upgraded(u.Password, password)); err != nil {
		return Session{}, nil, err
	}
	return s.open(ctx, u, tenantID, uuid.NewString(), false)
}

// refresh rotates: the presented token is consumed, the next one in the
// family is minted, the access token is reissued for the same workspace.
func (s sessions) refresh(ctx context.Context, raw *string) (Session, *http.Cookie, error) {
	u, t, err := s.consume(ctx, raw)
	if err != nil {
		return Session{}, nil, err
	}
	return s.open(ctx, u, t.TenantID, t.Family, false)
}

// switchTenant is a refresh aimed at another workspace the person belongs
// to; 401 when they do not.
func (s sessions) switchTenant(ctx context.Context, raw *string, tenantID string) (Session, *http.Cookie, error) {
	u, t, err := s.consume(ctx, raw)
	if err != nil {
		return Session{}, nil, err
	}
	return s.open(ctx, u, &tenantID, t.Family, true)
}

// logout revokes every token of the person behind the cookie. A cookie
// nobody recognises is cleared all the same: a logout that appears to fail
// is worse than one that quietly works.
func (s sessions) logout(ctx context.Context, raw *string) *http.Cookie {
	if u, _, err := s.consume(ctx, raw); err == nil {
		_ = s.repo.revokeAllRefreshOf(ctx, u.ID)
	}
	return s.cookie("", -1)
}

func (s sessions) consume(ctx context.Context, raw *string) (user, refreshToken, error) {
	if raw == nil || *raw == "" {
		return user{}, refreshToken{}, httpx.Unauthenticated(keyInvalidCredentials)
	}
	t, err := s.repo.consumeRefresh(ctx, *raw)
	if err != nil {
		if errors.Is(err, errInvalidToken) {
			return user{}, refreshToken{}, httpx.Unauthenticated(keyInvalidCredentials)
		}
		return user{}, refreshToken{}, err
	}
	u, err := s.repo.findUser(ctx, t.UserID)
	if err != nil || !u.active() {
		return user{}, refreshToken{}, httpx.Unauthenticated(keyInvalidCredentials)
	}
	return u, t, nil
}

// open picks the workspace, signs the access token, mints the refresh
// token and shapes the body. strict is a switch: the named tenant must be
// one of the person's; a login or refresh falls back to the best one.
func (s sessions) open(ctx context.Context, u user, requested *string, family string, strict bool) (Session, *http.Cookie, error) {
	ms, err := s.repo.membershipsOf(ctx, u.ID)
	if err != nil {
		return Session{}, nil, err
	}
	var chosen *membership
	if requested != nil {
		for i := range ms {
			if ms[i].TenantID == *requested && s.enterable(ctx, ms[i]) {
				chosen = &ms[i]
			}
		}
		if chosen == nil && strict {
			return Session{}, nil, httpx.Unauthenticated(keyNotAMember)
		}
	}
	if chosen == nil {
		// Where they have the most authority: one membership never notices,
		// several land predictably.
		for i := range ms {
			if s.enterable(ctx, ms[i]) && (chosen == nil || ms[i].rank() > chosen.rank()) {
				chosen = &ms[i]
			}
		}
	}
	if chosen == nil && len(ms) > 0 {
		// Every workspace they belong to is pending or suspended: the same
		// refusal as a wrong password, so a suspension is not announced.
		return Session{}, nil, httpx.Unauthenticated(keyInvalidCredentials)
	}
	session, err := s.build(ctx, u, ms, chosen, nil)
	if err != nil {
		return Session{}, nil, err
	}
	raw, err := s.repo.issueRefresh(ctx, u.ID, session.TenantId, family)
	if err != nil {
		return Session{}, nil, err
	}
	return session, s.cookie(raw, int(refreshTTL.Seconds())), nil
}

// enterable is an active membership of an active workspace: a suspended
// workspace locks its members out on their next login, refresh or switch.
func (s sessions) enterable(ctx context.Context, m membership) bool {
	if !m.active() {
		return false
	}
	t, found, err := s.tenants.Describe(ctx, m.TenantID)
	return err == nil && found && t.Active()
}

// build is the body every session call returns: the token with the role's
// permissions inside it, the memberships named through Tenant's contract.
// With an actor, the token is an impersonation: `imp` set, `act` naming
// the operator - still `aud: tenant`, so it passes the realm check
// honestly rather than being an exception to it.
func (s sessions) build(ctx context.Context, u user, ms []membership, chosen *membership, actor *auth.ActorInfo) (Session, error) {
	out := Session{ExpiresIn: int(accessTTL.Seconds()), User: u.view(), Memberships: make([]Membership, 0, len(ms)), Permissions: []string{}}
	for _, m := range ms {
		item := Membership{Id: m.ID, TenantId: m.TenantID, Role: MembershipRole(m.Role), Status: MembershipStatus(m.Status)}
		if t, found, err := s.tenants.Describe(ctx, m.TenantID); err != nil {
			return Session{}, err
		} else if found {
			item.TenantName, item.TenantSlug = &t.Name, &t.Slug
		}
		out.Memberships = append(out.Memberships, item)
	}
	claims := auth.Claims{Subject: u.ID, Audience: auth.RealmTenant}
	if actor != nil {
		claims.Impersonated, claims.Actor = true, actor
	}
	if chosen != nil {
		role := SessionRole(chosen.Role)
		out.TenantId, out.Role, out.Permissions = &chosen.TenantID, &role, s.perms.forRole(chosen.Role)
		claims.TenantID, claims.Permissions = chosen.TenantID, out.Permissions
	}
	token, err := s.signer.Sign(claims, accessTTL)
	if err != nil {
		return Session{}, err
	}
	out.Token = token
	return out, nil
}

// NewMinter is contract.NewMinter.
func NewMinter(svc *platform.Services) Minter {
	return sessions{repo: newRepository(svc), tenants: tenantc.NewReader(svc), signer: svc.Signer, hasher: svc.Passwords,
		perms: newResolver(svc.Permissions), secure: svc.URLs.Secure()}
}

// Impersonate is the operator's crossing (ADR-0020): the person's
// real session in that workspace, with the operator named in the token and
// NO refresh cookie. The person must be an active member of that tenant;
// a password is not asked for, which is the whole point and the whole risk.
func (s sessions) Impersonate(ctx context.Context, userID, tenantID string, op Operator) (map[string]any, error) {
	u, err := s.repo.findUser(ctx, userID)
	if err != nil {
		return nil, notFound(err)
	}
	ms, err := s.repo.membershipsOf(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	var chosen *membership
	for i := range ms {
		if ms[i].TenantID == tenantID && s.enterable(ctx, ms[i]) {
			chosen = &ms[i]
		}
	}
	if chosen == nil || !u.active() {
		return nil, httpx.BadRequest(keyNotAMember)
	}
	session, err := s.build(ctx, u, ms, chosen, &auth.ActorInfo{Subject: op.ID, Realm: auth.RealmManager})
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(session)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	// The frontend shows a persistent banner while this is set. An
	// impersonated session the user cannot see is the thing that turns
	// support access into a trust problem.
	out["impersonating"] = true
	out["operator"] = map[string]any{"id": op.ID, "displayName": nilIfEmpty(op.DisplayName)}
	return out, nil
}

// Started is contract.Started: a session StartSession opened, waiting to be
// written. Opaque on purpose - the calling module answers with it and does
// not take it apart, so "what a session looks like" stays in this module.
type Started struct {
	session Session
	cookie  *http.Cookie
}

// Write answers the request exactly as a login does: the refresh cookie,
// then the session as JSON with a 200. The calling module's response type
// wraps it in one line:
//
//	func (r ssoDone) VisitPostSsoCallbackResponse(w http.ResponseWriter) error { return r.Write(w) }
func (s Started) Write(w http.ResponseWriter) error {
	return sessionResponse{Session: s.session, cookie: s.cookie}.write(w)
}

// StartSession is an ordinary session for a person another module has
// authenticated - single sign-on, a link sent by mail, credentials carried
// over from a system the project replaced. That module decides WHO the
// person is; this one decides what a session is, with the same open() a
// login uses, so the browser needs no second code path. In the named
// workspace, which must be one they can enter, or their best one when
// tenantID is empty.
//
// An unknown or inactive person, or a workspace they cannot enter, is the
// login's own 401. Refusing guesses is the caller's: it rate-limits its
// endpoint and answers every failure alike, because Identity never sees
// the credential that was checked.
func (s sessions) StartSession(ctx context.Context, userID, tenantID string) (Started, error) {
	if _, err := uuid.Parse(userID); err != nil {
		return Started{}, httpx.Unauthenticated(keyInvalidCredentials)
	}
	u, err := s.repo.findUser(ctx, userID)
	if errors.Is(err, errNotFound) || (err == nil && !u.active()) {
		return Started{}, httpx.Unauthenticated(keyInvalidCredentials)
	}
	if err != nil {
		return Started{}, err
	}
	var requested *string
	if tenantID != "" {
		requested = &tenantID
	}
	session, cookie, err := s.open(ctx, u, requested, uuid.NewString(), true)
	if err != nil {
		return Started{}, err
	}
	// Stamped after the workspace check, so a refusal is not a login.
	if err := s.repo.recordLogin(ctx, u.ID, ""); err != nil {
		return Started{}, err
	}
	return Started{session: session, cookie: cookie}, nil
}

// upgraded is a new verifier for a password that just checked against one
// made at another cost (ADR-0022), "" when the stored one is current - or
// when hashing fails, which leaves the old one working.
func (s sessions) upgraded(stored, password string) string {
	if !s.hasher.NeedsRehash(stored) {
		return ""
	}
	v, err := s.hasher.Hash(password)
	if err != nil {
		return ""
	}
	return v
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// cookie is the refresh token's transport: HttpOnly so a script never reads
// it, Secure whenever the API is served over TLS, SameSite=Strict so no
// other site's request carries it, and scoped to /api/auth so it travels
// with nothing else (ADR-0018). maxAge -1 expires it.
func (s sessions) cookie(value string, maxAge int) *http.Cookie {
	c := &http.Cookie{Name: RefreshCookie, Value: value, Path: refreshPath, HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode, MaxAge: maxAge}
	if maxAge > 0 {
		c.Expires = time.Now().Add(time.Duration(maxAge) * time.Second)
	} else {
		c.Expires = time.Unix(0, 0)
	}
	return c
}
