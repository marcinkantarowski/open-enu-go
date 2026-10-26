package internal

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"time"
)

// Standard Webhooks signing (standardwebhooks.com). A published scheme rather
// than one invented here, because the receiver is somebody else's code: an
// off-the-shelf library should verify these without reading our docs.
//
// The signed string is `{id}.{timestamp}.{payload}` - all three, deliberately:
// the id makes a replay identifiable as a replay; the timestamp lets a
// receiver reject anything old, which is what actually stops a captured
// request being replayed tomorrow; signing the payload alone would let
// either of the other two be swapped.

// toleranceSeconds is what a receiver should allow for clock skew, and
// what our own verifier allows.
const toleranceSeconds = 300

// sign is the base64 HMAC-SHA256 over the signed string.
func sign(messageID string, timestamp int64, payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%s.%d.", messageID, timestamp)
	mac.Write(payload)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// headers are the three a receiver reads.
func headers(messageID string, timestamp int64, payload []byte, secret string) map[string]string {
	return map[string]string{
		"webhook-id":        messageID,
		"webhook-timestamp": fmt.Sprintf("%d", timestamp),
		"webhook-signature": "v1," + sign(messageID, timestamp, payload, secret),
	}
}

// verify checks a signature the way a receiver should: inside the window,
// and in constant time - a fast reject on the first differing byte leaks
// the signature one character at a time. Exists so the tests check what a
// third party computes rather than what we computed a moment ago.
func verify(header, messageID string, timestamp int64, payload []byte, secret string, now time.Time) bool {
	if d := now.Unix() - timestamp; d > toleranceSeconds || d < -toleranceSeconds {
		return false
	}
	expected := "v1," + sign(messageID, timestamp, payload, secret)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(header)) == 1
}
