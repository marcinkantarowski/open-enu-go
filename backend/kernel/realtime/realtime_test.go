package realtime

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/scope"
)

var secret = []byte("a-mercure-secret-of-sufficient-length")

const (
	issuer   = "https://api.example.test"
	audience = "https://api.example.test/.well-known/mercure"
)

// claims verifies the HS256 signature and the `at+jwt` type Mercure 1.x
// requires, and decodes the payload.
func claims(t *testing.T, token string) map[string]any {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("not a JWT: %q", token)
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if parts[2] != want {
		t.Fatal("signature does not verify under the hub's secret")
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || string(header) != `{"alg":"HS256","typ":"at+jwt"}` {
		t.Fatalf("header = %s - Mercure 1.x refuses a token that is not an at+jwt", header)
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var c map[string]any
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	if c["iss"] != issuer || c["aud"] != audience {
		t.Fatalf("iss/aud = %v / %v - the hub refuses any other pair", c["iss"], c["aud"])
	}
	return c
}

// grants is the one Mercure authorization detail a token carries, as JSON.
func grants(t *testing.T, c map[string]any) string {
	t.Helper()
	details, _ := c["authorization_details"].([]any)
	if len(details) != 1 {
		t.Fatalf("authorization_details = %v", c["authorization_details"])
	}
	out, _ := json.Marshal(details[0])
	return string(out)
}

func TestHubPublishesAPrivateUpdateWithAPublisherToken(t *testing.T) {
	var got *http.Request
	var form map[string][]string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		got, form = r, r.PostForm
		w.WriteHeader(200)
	}))
	defer hub.Close()

	h := Hub{URL: hub.URL + "/.well-known/mercure", Secret: secret, Issuer: issuer, Audience: audience}
	before := time.Now().Unix()
	if err := h.Publish(context.Background(), "/tenants/t1/events", []byte(`{"event":"x"}`)); err != nil {
		t.Fatal(err)
	}
	if form["topic"][0] != "/tenants/t1/events" || form["data"][0] != `{"event":"x"}` || form["private"][0] != "on" {
		t.Fatalf("form = %v", form)
	}
	c := claims(t, strings.TrimPrefix(got.Header.Get("Authorization"), "Bearer "))
	if g := grants(t, c); g != `{"actions":["publish"],"topics":[{"match":"*"}],"type":"https://mercure.rocks/authorization-detail"}` {
		t.Fatalf("publisher grant = %s", g)
	}
	if exp := int64(c["exp"].(float64)); exp < before+60 || exp > time.Now().Unix()+60 {
		t.Fatalf("exp = %d - a publisher token lives a minute", exp)
	}

	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "nope", 401) }))
	defer down.Close()
	if err := (Hub{URL: down.URL, Secret: secret, Issuer: issuer, Audience: audience}).Publish(context.Background(), "/t", nil); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("a refused publish must be an error naming the status: %v", err)
	}
	// Without the hub's issuer and audience nothing is sent: the hub would
	// refuse the token, and the reason would be lost in a bare 401.
	got = nil
	if err := (Hub{URL: hub.URL, Secret: secret}).Publish(context.Background(), "/t", nil); err == nil || got != nil {
		t.Fatalf("an unconfigured hub published: err=%v", err)
	}
}

func TestTokensScopeTheCookieToTheCallersTenant(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	h := Tokens{Secret: secret, Issuer: issuer, Audience: audience, Secure: true, Now: func() time.Time { return now }}

	ctx := scope.WithTenant(auth.WithPrincipal(context.Background(), auth.Principal{Subject: "u1", TenantID: "t1"}), "t1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/realtime/token", nil).WithContext(ctx))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"topic":"/tenants/t1/events"`) {
		t.Fatalf("= %d %s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %v", cookies)
	}
	ck := cookies[0]
	if ck.Name != CookieName || ck.Path != CookiePath || !ck.HttpOnly || !ck.Secure || ck.SameSite != http.SameSiteStrictMode || ck.Domain != "" {
		t.Fatalf("cookie attributes: %+v", ck)
	}
	if !ck.Expires.Equal(now.Add(time.Hour)) {
		t.Fatalf("expires = %v", ck.Expires)
	}
	c := claims(t, ck.Value)
	want := `{"actions":["subscribe"],"topics":[{"match":"/tenants/t1/events"},{"match":"/tenants/t1/progress/:id","match_type":"urlpattern"}],"type":"https://mercure.rocks/authorization-detail"}`
	if g := grants(t, c); g != want {
		t.Fatalf("subscriber grant = %s - it must name this tenant's topics and nothing else", g)
	}
	if int64(c["exp"].(float64)) != now.Add(time.Hour).Unix() || c["sub"] != "u1" {
		t.Fatalf("exp/sub = %v / %v", c["exp"], c["sub"])
	}

	// No principal: 401. A principal with no tenant: 403.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/realtime/token", nil))
	if rec.Code != 401 || !strings.Contains(rec.Body.String(), "unauthenticated") {
		t.Fatalf("anonymous = %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/realtime/token", nil).WithContext(auth.WithPrincipal(context.Background(), auth.Principal{Subject: "op", Realm: "manager"})))
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), "forbidden") {
		t.Fatalf("no tenant = %d %s", rec.Code, rec.Body.String())
	}

	// An empty secret never issues a token.
	rec = httptest.NewRecorder()
	Tokens{}.ServeHTTP(rec, httptest.NewRequest("POST", "/api/realtime/token", nil).WithContext(ctx))
	if rec.Code != 500 || len(rec.Result().Cookies()) != 0 {
		t.Fatalf("empty secret = %d with %d cookie(s)", rec.Code, len(rec.Result().Cookies()))
	}
	// Nor does one with no issuer or audience: the hub would refuse it.
	rec = httptest.NewRecorder()
	Tokens{Secret: secret}.ServeHTTP(rec, httptest.NewRequest("POST", "/api/realtime/token", nil).WithContext(ctx))
	if rec.Code != 500 || len(rec.Result().Cookies()) != 0 {
		t.Fatalf("no issuer = %d with %d cookie(s)", rec.Code, len(rec.Result().Cookies()))
	}
}
