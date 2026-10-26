package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Claims are the token's claims: sub, tid (the tenant, absent for an
// operator), aud (the realm), perms, iat and exp, and for impersonation imp
// with act naming the operator. The JSON names are the wire contract every
// app and every issued token depends on.
type Claims struct {
	Subject     string   `json:"sub"`
	TenantID    string   `json:"tid,omitempty"`
	Audience    string   `json:"aud"`
	Permissions []string `json:"perms,omitempty"`
	IssuedAt    int64    `json:"iat"`
	ExpiresAt   int64    `json:"exp"`
	// Impersonation: imp marks the session, act names the operator.
	Impersonated bool       `json:"imp,omitempty"`
	Actor        *ActorInfo `json:"act,omitempty"`
}

// ActorInfo is the `act` claim.
type ActorInfo struct {
	Subject string `json:"sub"`
	Realm   string `json:"realm"`
}

// Signer mints and verifies HS256 tokens under one secret. The kernel's
// own, with the standard library: three base64url segments, HMAC-SHA256.
type Signer struct {
	Secret []byte
	Now    func() time.Time // nil = time.Now
}

var (
	// ErrInvalidToken covers every verification failure. One error, on
	// purpose: a client learns that the token is no good, not which check
	// failed.
	ErrInvalidToken = errors.New("auth: invalid token")
)

const header = `{"alg":"HS256","typ":"JWT"}`

func (s Signer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Sign returns the compact token for c. ExpiresAt and IssuedAt are filled
// when zero (ttl from now).
func (s Signer) Sign(c Claims, ttl time.Duration) (string, error) {
	if len(s.Secret) < 32 {
		return "", errors.New("auth: the signing secret must be at least 32 bytes")
	}
	now := s.now()
	if c.IssuedAt == 0 {
		c.IssuedAt = now.Unix()
	}
	if c.ExpiresAt == 0 {
		c.ExpiresAt = now.Add(ttl).Unix()
	}
	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding
	signing := enc.EncodeToString([]byte(header)) + "." + enc.EncodeToString(payload)
	return signing + "." + enc.EncodeToString(s.mac(signing)), nil
}

// Verify checks the signature and the expiry, and returns the claims.
func (s Signer) Verify(token string) (Claims, error) {
	if len(s.Secret) < 32 {
		return Claims{}, ErrInvalidToken // an unconfigured signer trusts nobody
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, ErrInvalidToken
	}
	enc := base64.RawURLEncoding
	h, err := enc.DecodeString(parts[0])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	var hdr struct{ Alg string }
	if json.Unmarshal(h, &hdr) != nil || hdr.Alg != "HS256" {
		return Claims{}, ErrInvalidToken // alg:none and friends end here
	}
	sig, err := enc.DecodeString(parts[2])
	if err != nil || !hmac.Equal(sig, s.mac(parts[0]+"."+parts[1])) {
		return Claims{}, ErrInvalidToken
	}
	payload, err := enc.DecodeString(parts[1])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	var c Claims
	if json.Unmarshal(payload, &c) != nil || c.Subject == "" || c.Audience == "" {
		return Claims{}, ErrInvalidToken
	}
	// exp is required, not optional: every token this system mints has one,
	// so one without it was minted by something else - and accepting it
	// would make a leaked signing key good forever.
	if c.ExpiresAt == 0 || !s.now().Before(time.Unix(c.ExpiresAt, 0)) {
		return Claims{}, ErrInvalidToken
	}
	if c.Impersonated && (c.Actor == nil || c.Actor.Subject == "") {
		return Claims{}, ErrInvalidToken // imp without act is malformed
	}
	return c, nil
}

func (s Signer) mac(signing string) []byte {
	m := hmac.New(sha256.New, s.Secret)
	m.Write([]byte(signing))
	return m.Sum(nil)
}

// principal turns verified claims into the request's principal.
func (c Claims) principal() Principal {
	p := Principal{Subject: c.Subject, TenantID: c.TenantID, Realm: c.Audience, Permissions: c.Permissions}
	if c.Impersonated && c.Actor != nil {
		p.Impersonator = c.Actor.Subject
	}
	return p
}
