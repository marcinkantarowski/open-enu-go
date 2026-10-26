package httpx

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimit is a token bucket per client address: perMinute requests
// sustained, burst extra at once, 429 in the envelope beyond that. It sits
// in front of the public surface - login, register, forgot-password - where
// an unlimited endpoint is an oracle for guessing passwords and addresses,
// and the limiter is what makes "the same 401 for every failure" hold up
// against a client that can try a million times.
//
// The client is the LAST X-Forwarded-For entry when there is one, and the
// connection's address otherwise. The last entry is the one the edge proxy
// appended - the address it actually saw. The first entry is whatever the
// client put in the header itself, and a limiter keyed on that is switched
// off by sending a fresh made-up address with every attempt. In memory, per
// process: two api replicas allow twice the rate, which is the right side
// to err on for a limiter whose job is to slow a guess, not to meter a plan.
func RateLimit(perMinute, burst int) Middleware {
	l := &limiter{rate: float64(perMinute) / 60, burst: float64(burst), buckets: map[string]*bucket{}, now: time.Now}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !l.allow(clientAddr(r)) {
				w.Header().Set("Retry-After", "60")
				WriteError(w, r, NewError(http.StatusTooManyRequests, CodeRateLimited, KeyRateLimited))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type bucket struct {
	tokens float64
	seen   time.Time
}

type limiter struct {
	mu      sync.Mutex
	rate    float64 // tokens per second
	burst   float64
	buckets map[string]*bucket
	now     func() time.Time
}

func (l *limiter) allow(client string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b, ok := l.buckets[client]
	if !ok {
		if len(l.buckets) >= 10000 {
			l.sweep(now)
		}
		b = &bucket{tokens: l.burst, seen: now}
		l.buckets[client] = b
	}
	b.tokens = min(l.burst, b.tokens+now.Sub(b.seen).Seconds()*l.rate)
	b.seen = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// sweep drops clients idle long enough to have a full bucket again; they
// are indistinguishable from a client never seen.
func (l *limiter) sweep(now time.Time) {
	for k, b := range l.buckets {
		if now.Sub(b.seen).Seconds()*l.rate >= l.burst {
			delete(l.buckets, k)
		}
	}
}

func clientAddr(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		last := xff
		if i := strings.LastIndex(xff, ","); i >= 0 {
			last = xff[i+1:]
		}
		if ip := strings.TrimSpace(last); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
