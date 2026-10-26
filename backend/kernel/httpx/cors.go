package httpx

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// CORS allows the browser clients at exactly the given origins and no other.
//
// Exact origins, never a wildcard: authentication rides on a cookie (the
// refresh token, the Mercure subscriber token), so Allow-Credentials is on,
// and a browser refuses "*" beside it. The headers the request asks for are
// echoed - the allowlist is the origin, not the header. ETag and the request
// id are exposed so the conflict bar and an error report can read them.
//
// A request from an unlisted origin is served without CORS headers, which is
// the browser refusing it; a preflight from one is answered 403 so the
// failure is visible in the network tab rather than looking like a timeout.
// A request with no Origin header (curl, a server) is untouched.
func CORS(origins ...string) Middleware {
	allowed := make([]string, 0, len(origins))
	for _, o := range origins {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" {
			allowed = append(allowed, o)
		}
	}
	maxAge := strconv.Itoa(int((10 * time.Minute).Seconds()))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}
			preflight := r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != ""
			h := w.Header()
			h.Add("Vary", "Origin")
			if !slices.Contains(allowed, origin) {
				if preflight {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Credentials", "true")
			h.Set("Access-Control-Expose-Headers", "ETag, Location, "+RequestIDHeader)
			if !preflight {
				next.ServeHTTP(w, r)
				return
			}
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			if want := r.Header.Get("Access-Control-Request-Headers"); want != "" {
				h.Set("Access-Control-Allow-Headers", want)
			}
			h.Set("Access-Control-Max-Age", maxAge)
			w.WriteHeader(http.StatusNoContent)
		})
	}
}
