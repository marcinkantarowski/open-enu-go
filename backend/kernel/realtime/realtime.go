// Package realtime is the Mercure side of broadcasting: the hub the
// Broadcaster publishes to, and the subscriber token a browser needs to open
// an SSE connection. Both are platform decisions - the topic shape, the
// cookie's attributes, "every update is private" - which is why they live in
// the kernel and not in a module.
package realtime

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/event"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/i18n"
	"github.com/open-enu/kernel/scope"
)

// KeyNoTenant is the error for a subscriber token asked for outside a tenant:
// there is no topic anyone is entitled to receive.
const KeyNoTenant i18n.Key = "kernel.error.realtime_no_tenant"

// CookieName is what the hub reads; Mercure 1.x's own default. The
// `__Secure-` prefix makes a browser refuse it without the Secure attribute.
const CookieName = "__Secure-mercure_access_token"

// CookiePath keeps the cookie off every API request: the hub is served under
// this path on the API's host.
const CookiePath = "/.well-known/mercure"

// detailType names Mercure's entry in an OAuth 2.0 `authorization_details`
// claim - the only grant Mercure 1.x reads.
const detailType = "https://mercure.rocks/authorization-detail"

// Hub publishes to a Mercure hub. URL is the container-internal address
// (publishing through the edge would be a round trip for nothing); Secret
// signs the publisher token. Issuer and Audience are the ones the hub trusts
// (MERCURE_TRUSTED_ISSUERS, `resource_identifier`): the API's public URL and
// the hub's public URL - the same pair Tokens signs, although this request
// never leaves the network.
type Hub struct {
	URL      string
	Secret   []byte
	Issuer   string
	Audience string
	Client   *http.Client
}

var _ event.Publisher = Hub{}

// Publish posts one private update. Private always: a public update would
// be readable by anyone who guessed the tenant id.
func (h Hub) Publish(ctx context.Context, topic string, data []byte) error {
	// A minute is enough for one request; the token is minted per publish.
	token, err := accessToken(h.Secret, h.Issuer, h.Audience, "openenu", "publish", []grant{{Match: "*"}}, time.Now(), time.Minute)
	if err != nil {
		return fmt.Errorf("realtime: %w", err)
	}
	form := url.Values{"topic": {topic}, "data": {string(data)}, "private": {"on"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("realtime: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)
	client := h.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("realtime: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return fmt.Errorf("realtime: hub answered %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// Tokens issues the subscriber cookie at POST /api/realtime/token.
//
// EventSource cannot set headers, so the credential has to be a cookie. Its
// subscribe grant names the caller's own tenant topics and nothing else: a
// `{"match": "*"}` grant, the obvious thing to write, would hand every tenant's
// updates to any signed-in user - a leak no query filter can see, because no
// query runs.
type Tokens struct {
	Secret []byte
	// Issuer and Audience as on Hub.
	Issuer   string
	Audience string
	// Secure is false only when the API is served over plain http (a test).
	Secure bool
	TTL    time.Duration
	Now    func() time.Time
}

func (t Tokens) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	principal, ok := auth.From(ctx)
	if !ok {
		httpx.WriteError(w, r, httpx.Unauthenticated(httpx.KeyUnauthenticated))
		return
	}
	tenant, ok := scope.TenantID(ctx)
	if !ok {
		httpx.WriteError(w, r, httpx.Forbidden(KeyNoTenant))
		return
	}
	// An empty secret produces tokens the hub accepts from anyone, which is
	// indistinguishable from working until someone tries it.
	if len(t.Secret) == 0 {
		httpx.WriteError(w, r, httpx.Internal(errors.New("realtime: MERCURE_JWT_SECRET is empty; subscriber tokens would be unsigned")))
		return
	}
	now := time.Now
	if t.Now != nil {
		now = t.Now
	}
	ttl := t.TTL
	if ttl == 0 {
		ttl = time.Hour
	}
	issued := now()
	expires := issued.Add(ttl)
	topic := event.Topic(tenant)
	// A URL Pattern for the jobs: relative, resolved against the hub's
	// `resource_identifier`, as the topics themselves are.
	token, err := accessToken(t.Secret, t.Issuer, t.Audience, principal.Subject, "subscribe",
		[]grant{{Match: topic}, {Match: fmt.Sprintf("/tenants/%s/progress/:id", tenant), MatchType: "urlpattern"}}, issued, ttl)
	if err != nil {
		httpx.WriteError(w, r, httpx.Internal(err))
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:    CookieName,
		Value:   token,
		Path:    CookiePath,
		Expires: expires,
		// Host-only, deliberately: no Domain. Domain=.example.com would also
		// reach every host beneath it - a staging stack included.
		Secure:   t.Secure,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"topic": topic})
}

// grant is one matcher in an authorization detail; MatchType "" is exact.
type grant struct {
	Match     string `json:"match"`
	MatchType string `json:"match_type,omitempty"`
}

// accessToken is the OAuth 2.0 access token Mercure 1.x verifies (RFC 9068:
// `typ: at+jwt`, issuer, audience, expiry), granting one action on topics.
// The hub refuses a token whose `iss` or `aud` it does not expect, so an empty
// one is an error here rather than a 401 nobody can explain.
func accessToken(secret []byte, issuer, audience, subject, action string, topics []grant, now time.Time, ttl time.Duration) (string, error) {
	if issuer == "" || audience == "" {
		return "", errors.New("the hub's issuer and audience are not configured")
	}
	jti := make([]byte, 16)
	_, _ = rand.Read(jti)
	return sign(secret, map[string]any{
		"iss":       issuer,
		"aud":       audience,
		"sub":       subject,
		"client_id": "openenu",
		"iat":       now.Unix(),
		"exp":       now.Add(ttl).Unix(),
		"jti":       hex.EncodeToString(jti),
		"authorization_details": []map[string]any{
			{"type": detailType, "actions": []string{action}, "topics": topics},
		},
	})
}

// sign is HS256 over a claims object - what Mercure verifies. Separate from
// auth.Signer, whose claims are the access token's.
func sign(secret []byte, claims map[string]any) (string, error) {
	if len(secret) == 0 {
		return "", errors.New("empty signing secret")
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding
	signing := enc.EncodeToString([]byte(`{"alg":"HS256","typ":"at+jwt"}`)) + "." + enc.EncodeToString(payload)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signing))
	return signing + "." + enc.EncodeToString(mac.Sum(nil)), nil
}
