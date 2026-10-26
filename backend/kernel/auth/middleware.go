package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/i18n"
	"github.com/open-enu/kernel/scope"
)

// KeyPrefix marks an API key on the wire.
const KeyPrefix = "sk_"

// KeyImpersonationDenied is the 403 an impersonated session gets on a
// guarded action.
const KeyImpersonationDenied i18n.Key = "kernel.error.impersonation_denied"

// KeyResolver finds the principal behind an API key, by the hex SHA-256 of
// the full key - the table holds hashes, never keys. The ApiKey module
// provides the real one; tests use MemoryKeys.
type KeyResolver interface {
	Resolve(ctx context.Context, hash string) (Principal, bool, error)
}

// NoKeys resolves nothing: the process before the ApiKey module exists.
type NoKeys struct{}

// Resolve implements KeyResolver.
func (NoKeys) Resolve(context.Context, string) (Principal, bool, error) {
	return Principal{}, false, nil
}

// MemoryKeys is a KeyResolver over a map of hash -> principal, for tests.
type MemoryKeys map[string]Principal

// Add registers key under its hash and returns the key.
func (m MemoryKeys) Add(key string, p Principal) string {
	p.APIKey = true
	m[HashKey(key)] = p
	return key
}

// Resolve implements KeyResolver.
func (m MemoryKeys) Resolve(_ context.Context, hash string) (Principal, bool, error) {
	p, ok := m[hash]
	return p, ok, nil
}

// HashKey is how a key is stored and looked up.
func HashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// Middleware authenticates `Authorization: Bearer <token>`. A key
// (`sk_...`) goes to the resolver, anything else to the signer. A valid
// credential puts the Principal on the context and enters its tenant's
// scope; a missing header passes through anonymous (Require answers 401
// where it matters); a bad credential is 401 here, before any handler.
func Middleware(signer Signer, keys KeyResolver) httpx.Middleware {
	if keys == nil {
		keys = NoKeys{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := strings.TrimSpace(r.Header.Get("Authorization"))
			if raw == "" {
				next.ServeHTTP(w, r)
				return
			}
			token, ok := strings.CutPrefix(raw, "Bearer ")
			if !ok {
				httpx.WriteError(w, r, httpx.Unauthenticated(httpx.KeyUnauthenticated))
				return
			}
			p, err := authenticate(r.Context(), signer, keys, strings.TrimSpace(token))
			if err != nil {
				// A credential that does not verify is the caller's 401. A
				// resolver that could not answer (the database is down) is
				// a 500: logged with its cause, and not mistaken for a bad
				// key by whoever is reading the logs during the outage.
				var e *httpx.Error
				switch {
				case errors.Is(err, ErrInvalidToken), errors.As(err, &e):
					httpx.WriteError(w, r, httpx.Unauthenticated(httpx.KeyUnauthenticated).WithCause(err))
				default:
					httpx.WriteError(w, r, httpx.Internal(fmt.Errorf("auth: resolving credential: %w", err)))
				}
				return
			}
			ctx := WithPrincipal(r.Context(), p)
			if p.TenantID != "" {
				ctx = scope.WithTenant(ctx, p.TenantID)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func authenticate(ctx context.Context, signer Signer, keys KeyResolver, token string) (Principal, error) {
	if strings.HasPrefix(token, KeyPrefix) {
		p, ok, err := keys.Resolve(ctx, HashKey(token))
		if err != nil {
			return Principal{}, err
		}
		if !ok {
			return Principal{}, ErrInvalidToken
		}
		return p, nil
	}
	c, err := signer.Verify(token)
	if err != nil {
		return Principal{}, err
	}
	return c.principal(), nil
}
