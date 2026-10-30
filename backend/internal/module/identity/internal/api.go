package internal

import (
	"context"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/command"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/i18n"
)

const (
	keyEmailInvalid      i18n.Key = "identity.error.email_invalid"
	keyPasswordTooShort  i18n.Key = "identity.error.password_too_short"
	keyTenantName        i18n.Key = "identity.error.tenant_name_required"
	keyLocaleUnsupported i18n.Key = "identity.error.locale_unsupported"
	keyNotAPerson        i18n.Key = "identity.error.not_a_person"
)

// api implements the StrictServerInterface genapi derives from
// ../openapi.json. Thin: validation, a command or a session call, a
// response. Reads go through the repository.
type api struct {
	bus      *command.Bus
	repo     repository
	sessions sessions
	locales  []string
	guard    auth.Guard
}

var _ StrictServerInterface = api{}

//impersonation:allowed public sign-up, no principal yet - nobody to impersonate
func (a api) PostAuthRegister(ctx context.Context, req PostAuthRegisterRequestObject) (PostAuthRegisterResponseObject, error) {
	b := req.Body
	if !validEmail(string(b.Email)) {
		return nil, httpx.BadRequest(keyEmailInvalid)
	}
	if !validPassword(b.Password) {
		return nil, httpx.BadRequest(keyPasswordTooShort)
	}
	if name := strings.TrimSpace(b.TenantName); name == "" || utf8.RuneCountInString(name) > 200 {
		return nil, httpx.BadRequest(keyTenantName)
	}
	if b.Locale != nil && !a.supported(*b.Locale) {
		return nil, httpx.BadRequest(keyLocaleUnsupported)
	}
	if _, err := command.Dispatch[registered](ctx, a.bus, registerUser{Email: string(b.Email), Password: b.Password, TenantName: b.TenantName, DisplayName: cleanOpt(b.DisplayName), Locale: b.Locale}); err != nil {
		return nil, err
	}
	return PostAuthRegister202JSONResponse{Status: RegisteredStatusUnverified}, nil
}

//impersonation:allowed public, the mailed token is the credential - there is no session to impersonate
func (a api) PostAuthVerify(ctx context.Context, req PostAuthVerifyRequestObject) (PostAuthVerifyResponseObject, error) {
	u, err := command.Dispatch[user](ctx, a.bus, verifyEmail{Token: req.Body.Token})
	if err != nil {
		return nil, err
	}
	return PostAuthVerify200JSONResponse{Status: VerifiedStatusVerified, UserId: u.ID}, nil
}

func (a api) PostAuthLogin(ctx context.Context, req PostAuthLoginRequestObject) (PostAuthLoginResponseObject, error) {
	s, c, err := a.sessions.login(ctx, req.Body.Email, req.Body.Password, req.Body.TenantId)
	if err != nil {
		return nil, err
	}
	return sessionResponse{Session: s, cookie: c}, nil
}

func (a api) PostAuthRefresh(ctx context.Context, req PostAuthRefreshRequestObject) (PostAuthRefreshResponseObject, error) {
	// No cookie is not a failed credential, it is no credential: every first
	// visit asks, and a 401 here is a red line in the console of a browser
	// that did nothing wrong. A cookie that IS sent and does not verify is
	// still the 401 below.
	if req.Params.OpenEnuRefresh == nil || *req.Params.OpenEnuRefresh == "" {
		return PostAuthRefresh204Response{}, nil
	}
	s, c, err := a.sessions.refresh(ctx, req.Params.OpenEnuRefresh)
	if err != nil {
		return nil, err
	}
	return sessionResponse{Session: s, cookie: c}, nil
}

func (a api) PostAuthSwitchTenant(ctx context.Context, req PostAuthSwitchTenantRequestObject) (PostAuthSwitchTenantResponseObject, error) {
	s, c, err := a.sessions.switchTenant(ctx, req.Params.OpenEnuRefresh, req.Body.TenantId)
	if err != nil {
		return nil, err
	}
	return sessionResponse{Session: s, cookie: c}, nil
}

func (a api) PostAuthLogout(ctx context.Context, req PostAuthLogoutRequestObject) (PostAuthLogoutResponseObject, error) {
	return loggedOutResponse{cookie: a.sessions.logout(ctx, req.Params.OpenEnuRefresh)}, nil
}

//impersonation:allowed public, no principal - the same answer for every address (ADR-0023)
func (a api) PostAuthForgotPassword(ctx context.Context, req PostAuthForgotPasswordRequestObject) (PostAuthForgotPasswordResponseObject, error) {
	// The same body whether the address is known or not; the handler sends
	// a mail only when it is.
	if _, err := a.bus.Dispatch(ctx, requestPasswordReset{Email: req.Body.Email}); err != nil {
		return nil, err
	}
	return PostAuthForgotPassword200JSONResponse{Status: SentStatusSent}, nil
}

//impersonation:allowed public, the mailed token is the credential - there is no session to impersonate
func (a api) PostAuthResetPassword(ctx context.Context, req PostAuthResetPasswordRequestObject) (PostAuthResetPasswordResponseObject, error) {
	if !validPassword(req.Body.Password) {
		return nil, httpx.BadRequest(keyPasswordTooShort)
	}
	u, err := command.Dispatch[user](ctx, a.bus, resetPassword{Token: req.Body.Token, Password: req.Body.Password})
	if err != nil {
		return nil, err
	}
	return PostAuthResetPassword200JSONResponse{Status: ResetStatusReset, UserId: u.ID}, nil
}

func (a api) GetProfileShow(ctx context.Context, _ GetProfileShowRequestObject) (GetProfileShowResponseObject, error) {
	u, err := a.person(ctx)
	if err != nil {
		return nil, err
	}
	return GetProfileShow200JSONResponse(u.view()), nil
}

func (a api) PatchProfileUpdate(ctx context.Context, req PatchProfileUpdateRequestObject) (PatchProfileUpdateResponseObject, error) {
	p, err := auth.Require(ctx, "identity.profile.manage")
	if err != nil {
		return nil, err
	}
	if p.APIKey {
		return nil, httpx.Forbidden(keyNotAPerson)
	}
	// Support may read a profile; the name and language on it stay the
	// person's to change (ADR-0020).
	if err := a.guard.DenyImpersonated(ctx, "identity.profile.updated"); err != nil {
		return nil, err
	}
	if req.Body.Locale != nil && !a.supported(*req.Body.Locale) {
		return nil, httpx.BadRequest(keyLocaleUnsupported)
	}
	cmd := updateProfile{UserID: p.Subject, Locale: req.Body.Locale}
	if req.Body.DisplayName != nil {
		cmd.SetDisplayName, cmd.DisplayName = true, cleanOpt(req.Body.DisplayName)
	}
	u, err := command.Dispatch[user](ctx, a.bus, cmd)
	if err != nil {
		return nil, err
	}
	return PatchProfileUpdate200JSONResponse(u.view()), nil
}

// person is the caller's own account: any authenticated person of the
// tenant realm, no permission - but a key is not a person and has no
// profile. An operator's token gets the same 401 as no token, not a 404
// that would tell it the realms share a key.
func (a api) person(ctx context.Context) (user, error) {
	p, err := auth.RequireRealm(ctx, auth.RealmTenant)
	if err != nil {
		return user{}, err
	}
	if p.APIKey {
		return user{}, httpx.Forbidden(keyNotAPerson)
	}
	u, err := a.repo.findUser(ctx, p.Subject)
	return u, notFound(err)
}

func (a api) supported(locale string) bool {
	for _, l := range a.locales {
		if l == locale {
			return true
		}
	}
	return false
}

func validEmail(s string) bool {
	s = strings.TrimSpace(s)
	at := strings.LastIndex(s, "@")
	// 254 octets: RFC 5321's limit is on the wire, so this one is in bytes.
	return at > 0 && at < len(s)-3 && strings.Contains(s[at:], ".") && len([]byte(s)) <= 254
}

// validPassword counts characters, as the spec's minLength does: twelve
// Polish letters are a twelve-character password, not a 24-byte one.
func validPassword(s string) bool {
	n := utf8.RuneCountInString(s)
	return n >= 12 && n <= 200
}

// cleanOpt trims, and turns an empty string into nil.
func cleanOpt(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" || utf8.RuneCountInString(v) > 120 {
		return nil
	}
	return &v
}

// sessionResponse is the Session body plus the Set-Cookie the generated
// 200 type has no place for. One type serves login, refresh and switch.
type sessionResponse struct {
	Session
	cookie *http.Cookie
}

func (r sessionResponse) write(w http.ResponseWriter) error {
	http.SetCookie(w, r.cookie)
	return PostAuthLogin200JSONResponse(r.Session).VisitPostAuthLoginResponse(w)
}

func (r sessionResponse) VisitPostAuthLoginResponse(w http.ResponseWriter) error   { return r.write(w) }
func (r sessionResponse) VisitPostAuthRefreshResponse(w http.ResponseWriter) error { return r.write(w) }
func (r sessionResponse) VisitPostAuthSwitchTenantResponse(w http.ResponseWriter) error {
	return r.write(w)
}

type loggedOutResponse struct{ cookie *http.Cookie }

func (r loggedOutResponse) VisitPostAuthLogoutResponse(w http.ResponseWriter) error {
	http.SetCookie(w, r.cookie)
	return PostAuthLogout200JSONResponse{Status: LoggedOutStatusLoggedOut}.VisitPostAuthLogoutResponse(w)
}
