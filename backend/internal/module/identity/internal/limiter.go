package internal

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/open-enu/kernel/httpx"
)

// The per-client limiter (httpx.RateLimit, in Mount) slows one machine; it
// does nothing against a botnet spreading guesses at one address over a
// thousand machines. This one counts per ADDRESS: every login and
// forgot-password naming it, from wherever, shares one bucket.
//
// It answers the same for an address with an account and one without -
// counting starts at the first attempt either way - so it says nothing
// ADR-0023 forbids. The price is that ten attempts at someone's address
// lock their password login for a while; the reset mail is limited by the
// same bucket, and the cost is a wait, not an account.
//
// A login that SUCCEEDS gives its attempt back: the bucket counts guesses,
// and a person who signs in often is not guessing. Forgot-password answers
// the same whatever happens (ADR-0023), so there every attempt counts.
const (
	AccountAttempts = 10
	AccountWindow   = 15 * time.Minute

	// accountBodyLimit bounds what is read to find the address; a body
	// longer than this is not a login and the strict handler refuses it.
	accountBodyLimit = 64 << 10
)

// accountLimiter is a token bucket per address hash, in memory, per
// process - like httpx.RateLimit, and erring the same way: two replicas
// allow twice the attempts.
type accountLimiter struct {
	mu      sync.Mutex
	rate    float64 // tokens per second
	burst   float64
	buckets map[string]*accountBucket
	now     func() time.Time
}

type accountBucket struct {
	tokens float64
	seen   time.Time
}

func newAccountLimiter(attempts int, window time.Duration) *accountLimiter {
	return &accountLimiter{rate: float64(attempts) / window.Seconds(), burst: float64(attempts), buckets: map[string]*accountBucket{}, now: time.Now}
}

func (l *accountLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) >= 100000 {
			for k, old := range l.buckets {
				if now.Sub(old.seen).Seconds()*l.rate >= l.burst {
					delete(l.buckets, k)
				}
			}
		}
		b = &accountBucket{tokens: l.burst, seen: now}
		l.buckets[key] = b
	}
	b.tokens = min(l.burst, b.tokens+now.Sub(b.seen).Seconds()*l.rate)
	b.seen = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// refund returns one attempt to the address's bucket, never above the burst.
func (l *accountLimiter) refund(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if b, ok := l.buckets[key]; ok {
		b.tokens = min(l.burst, b.tokens+1)
	}
}

// perAccount reads the address out of the JSON body, puts the body back
// for the handler, and answers httpx.RateLimit's own 429 when the address
// is out of attempts. A body with no address passes: the handler refuses it.
func (l *accountLimiter) perAccount(next http.Handler) http.Handler { return l.limit(next, false) }

// perLogin is perAccount for the login route: a 2xx gives the attempt back.
func (l *accountLimiter) perLogin(next http.Handler) http.Handler { return l.limit(next, true) }

func (l *accountLimiter) limit(next http.Handler, refundOnSuccess bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Body == nil {
			next.ServeHTTP(w, r)
			return
		}
		head, err := io.ReadAll(io.LimitReader(r.Body, accountBodyLimit))
		if err != nil {
			httpx.WriteError(w, r, httpx.BadRequest(httpx.KeyBadRequest).WithCause(err))
			return
		}
		r.Body = struct {
			io.Reader
			io.Closer
		}{io.MultiReader(bytes.NewReader(head), r.Body), r.Body}
		var in struct {
			Email string `json:"email"`
		}
		if json.Unmarshal(head, &in) != nil || in.Email == "" {
			next.ServeHTTP(w, r)
			return
		}
		key := accountKey(in.Email)
		if !l.allow(key) {
			w.Header().Set("Retry-After", "60")
			httpx.WriteError(w, r, httpx.NewError(http.StatusTooManyRequests, httpx.CodeRateLimited, httpx.KeyRateLimited))
			return
		}
		if !refundOnSuccess {
			next.ServeHTTP(w, r)
			return
		}
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if rec.status >= 200 && rec.status < 300 {
			l.refund(key)
		}
	})
}

// statusRecorder remembers the status the handler wrote.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// accountKey is the normalised address hashed, so the limiter's memory
// holds no address. A plain SHA-256: it is a map key that never leaves the
// process, not a stored lookup column.
func accountKey(email string) string {
	sum := sha256.Sum256([]byte(normalizeEmail(email)))
	return hex.EncodeToString(sum[:])
}
