package internal

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/crypto"
)

// Statuses, roles and token purposes as the tables store them, and the
// lifetimes of each token.
const (
	userUnverified = "unverified"
	userActive     = "active"
	userDisabled   = "disabled"

	membershipActive    = "active"
	membershipSuspended = "suspended"

	roleOwner  = "owner"
	roleAdmin  = "admin"
	roleMember = "member"

	purposeVerifyEmail   = "verify_email"
	purposeResetPassword = "reset_password"

	verifyTTL  = 24 * time.Hour
	resetTTL   = time.Hour
	refreshTTL = 30 * 24 * time.Hour
	accessTTL  = 15 * time.Minute
)

// user is the row decrypted. Email and display name are ciphertext in the
// table under the global key - a person belongs to several tenants, so no
// tenant's key may own them; email_hash is what a login looks up.
type user struct {
	ID          string
	Email       string
	EmailHash   string
	Password    string // the verifier, never the password
	DisplayName *string
	Status      string
	Locale      *string
	AvatarKey   *string
	Version     int
	CreatedAt   time.Time
	LastLoginAt *time.Time
}

func (u user) active() bool { return u.Status == userActive }

// Snapshot is what the trail keeps: no address, no name, no verifier. The
// trail is read by more people than the record.
func (u user) Snapshot() map[string]any {
	return map[string]any{"id": u.ID, "status": u.Status, "locale": u.Locale, "version": u.Version, "hasDisplayName": u.DisplayName != nil}
}

// view is the wire shape, for its owner.
func (u user) view() SessionUser {
	return SessionUser{
		Id: u.ID, Email: u.Email, DisplayName: u.DisplayName, Status: SessionUserStatus(u.Status), Locale: u.Locale,
		HasAvatar: u.AvatarKey != nil, Version: u.Version, CreatedAt: u.CreatedAt,
	}
}

func newUser(email, verifier string, displayName, locale *string, status string) user {
	return user{
		ID: uuid.Must(uuid.NewV7()).String(), Email: email, EmailHash: emailHash(email), Password: verifier,
		DisplayName: displayName, Status: status, Locale: locale, Version: 1, CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
}

// normalizeEmail is the one spelling of an address: trimmed, lower-cased.
func normalizeEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func emailHash(email string) string { return crypto.HashForLookup(normalizeEmail(email)) }

type membership struct {
	ID        string
	UserID    string
	TenantID  string
	Role      string
	Status    string
	CreatedAt time.Time
}

func (m membership) active() bool { return m.Status == membershipActive }

// rank orders roles for the default workspace at login: where the person
// has the most authority.
func (m membership) rank() int {
	switch m.Role {
	case roleOwner:
		return 3
	case roleAdmin:
		return 2
	case roleMember:
		return 1
	}
	return 0
}

func (m membership) Snapshot() map[string]any {
	return map[string]any{"id": m.ID, "userId": m.UserID, "tenantId": m.TenantID, "role": m.Role, "status": m.Status}
}

func newMembership(userID, tenantID, role string) membership {
	return membership{ID: uuid.Must(uuid.NewV7()).String(), UserID: userID, TenantID: tenantID, Role: role, Status: membershipActive, CreatedAt: time.Now().UTC().Truncate(time.Microsecond)}
}

func validRole(r string) bool { return r == roleOwner || r == roleAdmin || r == roleMember }

// refreshToken is one link in a family: each use mints the next and marks
// this one used. A used token presented again is theft or a replay, and the
// whole family is revoked.
type refreshToken struct {
	ID        string
	TokenHash string
	UserID    string
	Family    string
	TenantID  *string
	ExpiresAt time.Time
	UsedAt    *time.Time
	Revoked   bool
	CreatedAt time.Time
}

// securityToken is a one-shot link: verification, reset, invitation. Only
// its hash is stored; the raw value lives in exactly one email.
type securityToken struct {
	ID         string
	TokenHash  string
	Purpose    string
	UserID     *string
	TenantID   *string
	Payload    map[string]any
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	CreatedAt  time.Time
}

// newRawToken is 32 random bytes, URL-safe. hashToken is what the table
// holds (SHA-256 hex, the kernel's spelling).
func newRawToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("identity: crypto/rand: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func hashToken(raw string) string { return auth.HashKey(raw) }
