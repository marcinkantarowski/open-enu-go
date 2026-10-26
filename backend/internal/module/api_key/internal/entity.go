package internal

import (
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/google/uuid"

	"github.com/open-enu/kernel/auth"
)

// prefixLength is how much of the secret the list shows, to tell keys apart.
const prefixLength = 12

// apiKey is the row. The secret is never in it: KeyHash is what the
// resolver looks up, KeyPrefix what a person recognises.
type apiKey struct {
	ID          string
	TenantID    string
	Name        string
	KeyHash     string
	KeyPrefix   string
	Permissions []string
	Revoked     bool
	ExpiresAt   *time.Time
	LastUsedAt  *time.Time
	CreatedAt   time.Time
}

func (k apiKey) usable(now time.Time) bool {
	return !k.Revoked && (k.ExpiresAt == nil || now.Before(*k.ExpiresAt))
}

// Snapshot for the trail: no hash. The prefix is already public to the
// tenant's admins.
func (k apiKey) Snapshot() map[string]any {
	return map[string]any{"id": k.ID, "name": k.Name, "prefix": k.KeyPrefix, "permissions": k.Permissions, "revoked": k.Revoked, "expiresAt": k.ExpiresAt}
}

func (k apiKey) view() ApiKey {
	return ApiKey{Id: k.ID, Name: k.Name, Prefix: k.KeyPrefix, Permissions: k.Permissions, Revoked: k.Revoked, ExpiresAt: k.ExpiresAt, LastUsedAt: k.LastUsedAt, CreatedAt: k.CreatedAt}
}

// newSecret mints `sk_` + 32 random bytes, URL-safe: the one time the
// secret exists in plaintext is the creating response.
func newSecret() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("api_key: crypto/rand: " + err.Error())
	}
	return auth.KeyPrefix + base64.RawURLEncoding.EncodeToString(b[:])
}

func newKey(tenantID, name string, permissions []string, expires *time.Time) (apiKey, string) {
	secret := newSecret()
	return apiKey{
		ID: uuid.Must(uuid.NewV7()).String(), TenantID: tenantID, Name: name, KeyHash: auth.HashKey(secret), KeyPrefix: secret[:prefixLength],
		Permissions: permissions, ExpiresAt: expires, CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}, secret
}
