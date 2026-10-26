package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DownloadPath is where the kernel's download handler is mounted; a signed
// URL is BaseURL + DownloadPath + key + the signature query.
const DownloadPath = "/files/"

// ErrBadSignature is returned by Verify for an edited or expired URL.
var ErrBadSignature = errors.New("storage: signature invalid or expired")

// Signer turns a key into a time-limited URL for the local backend. The
// signature covers the key AND the expiry, so the URL cannot be edited to
// reach another file or to live longer, and it stops working on its own.
type Signer struct {
	Secret  []byte
	BaseURL string // scheme and host, no trailing slash
	Now     func() time.Time
}

func (s Signer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s Signer) mac(key string, expires int64) string {
	h := hmac.New(sha256.New, s.Secret)
	h.Write([]byte(key))
	h.Write([]byte{'\n'})
	h.Write([]byte(strconv.FormatInt(expires, 10)))
	return hex.EncodeToString(h.Sum(nil))
}

// Sign returns the URL for key, valid for ttl.
func (s Signer) Sign(key string, ttl time.Duration) string {
	exp := s.now().Add(ttl).Unix()
	q := url.Values{"expires": {strconv.FormatInt(exp, 10)}, "sig": {s.mac(key, exp)}}
	return strings.TrimSuffix(s.BaseURL, "/") + DownloadPath + key + "?" + q.Encode()
}

// Verify checks a key, expiry and signature as they arrived in a request.
func (s Signer) Verify(key, expires, sig string) error {
	exp, err := strconv.ParseInt(expires, 10, 64)
	if err != nil || !s.now().Before(time.Unix(exp, 0)) {
		return ErrBadSignature
	}
	if !hmac.Equal([]byte(sig), []byte(s.mac(key, exp))) {
		return ErrBadSignature
	}
	return nil
}
