package security_test

import (
	"net/http"
	"testing"
)

// The browser apps are the only origins (ADR-0021). Credentials
// ride on the cookie, so a wildcard is not available; every origin is exact.

func preflight(s *stack, origin string) reply {
	return s.send(request{method: http.MethodOptions, path: "/api/auth/login", headers: map[string]string{
		"Origin": origin, "Access-Control-Request-Method": "POST", "Access-Control-Request-Headers": "content-type, authorization, if-match",
	}})
}

func TestTheTwoAppOriginsMayCallTheApi(t *testing.T) {
	s := newStack(t)
	for _, origin := range []string{appOrigin, managerOrigin} {
		r := preflight(s, origin)
		if r.code != 204 || r.header.Get("Access-Control-Allow-Origin") != origin || r.header.Get("Access-Control-Allow-Credentials") != "true" {
			t.Fatalf("%s: %d %v", origin, r.code, r.header)
		}
		actual := s.send(request{method: "POST", path: "/api/auth/login", payload: map[string]any{"email": "x@example.test", "password": "y"}, headers: map[string]string{"Origin": origin}})
		if actual.header.Get("Access-Control-Allow-Origin") != origin || actual.header.Get("Vary") == "" {
			t.Fatalf("%s actual request: %v", origin, actual.header)
		}
	}
}

func TestAThirdOrLookalikeOriginIsRefused(t *testing.T) {
	s := newStack(t)
	for _, origin := range []string{"https://evil.test", "https://app.test.evil.test", "https://app.test:8443", "http://app.test", "https://xapp.test", "null"} {
		r := preflight(s, origin)
		if r.code != 403 || r.header.Get("Access-Control-Allow-Origin") != "" {
			t.Fatalf("%s preflight: %d %q", origin, r.code, r.header.Get("Access-Control-Allow-Origin"))
		}
		actual := s.send(request{method: "GET", path: "/api/audit", headers: map[string]string{"Origin": origin}})
		if actual.header.Get("Access-Control-Allow-Origin") != "" {
			t.Fatalf("%s actual: allowed", origin)
		}
	}
	if r := s.header(preflight(s, "*")); r != "" {
		t.Fatalf("a literal wildcard origin was allowed: %q", r)
	}
}

func (s *stack) header(r reply) string { return r.header.Get("Access-Control-Allow-Origin") }

func TestTheAllowedHeadersCoverOptimisticLockingAndTheClientCanReadTheVersionAndTheRequestId(t *testing.T) {
	s := newStack(t)
	r := preflight(s, appOrigin)
	mustContain(t, r.header.Get("Access-Control-Allow-Headers"), "if-match", "If-Match may be sent")
	mustContain(t, r.header.Get("Access-Control-Allow-Headers"), "authorization", "Authorization may be sent")
	mustContain(t, r.header.Get("Access-Control-Allow-Methods"), "PATCH", "PATCH is allowed")
	exposed := r.header.Get("Access-Control-Expose-Headers")
	mustContain(t, exposed, "ETag", "the version is readable")
	mustContain(t, exposed, "X-Request-Id", "the request id is readable")
	if id := s.do("GET", "/api/audit", "", nil).header.Get("X-Request-Id"); len(id) < 16 {
		t.Fatalf("no request id on a response: %q", id)
	}
}
