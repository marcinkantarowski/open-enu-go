package crypto

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
)

// PasswordHasher derives a verifier from a password: argon2id, written in
// the PHC string format - `$argon2id$v=19$m=65536,t=4,p=1$<salt>$<hash>` -
// that the reference implementation and most languages' libraries read and
// write, so a verifier can move between systems (ADR-0022). The encoded form
// carries its own parameters, so the cost can change and every existing
// verifier still checks; NeedsRehash says which ones to replace on the next
// successful login.
type PasswordHasher struct {
	// Cost is the argon2id work factor; the zero value is
	// DefaultPasswordCost. A test platform sets argon2's floor - the cost is
	// the point in production and only a cost in a suite that logs in two
	// hundred times.
	Cost PasswordCost
}

// PasswordCost is argon2id's three parameters.
type PasswordCost struct {
	MemoryKiB uint32
	Time      uint32
	Threads   uint8
}

// DefaultPasswordCost is 64 MiB, four passes, one lane - above OWASP's
// argon2id minimum.
var DefaultPasswordCost = PasswordCost{MemoryKiB: 64 * 1024, Time: 4, Threads: 1}

const (
	argonPrefix = "$argon2id$"
	saltLen     = 16
	keyLen      = 32
)

// hashing bounds how many derivations run at once. Each argon2id takes 64
// MiB for as long as it runs, and with one lane it is one core's work, so
// more at once than there are cores only adds memory: a burst of logins
// queues here instead of allocating a gigabyte.
var hashing = make(chan struct{}, runtime.GOMAXPROCS(0))

func (h PasswordHasher) cost() PasswordCost {
	if h.Cost == (PasswordCost{}) {
		return DefaultPasswordCost
	}
	return h.Cost
}

// Hash returns the encoded verifier for a password.
func (h PasswordHasher) Hash(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	c := h.cost()
	key := derive(func() []byte { return argon2.IDKey([]byte(password), salt, c.Time, c.MemoryKiB, c.Threads, keyLen) })
	enc := base64.RawStdEncoding
	return fmt.Sprintf("%sv=%d$m=%d,t=%d,p=%d$%s$%s", argonPrefix, argon2.Version, c.MemoryKiB, c.Time, c.Threads, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// Verify reports whether password matches the encoded verifier. A
// malformed verifier is simply false: the caller answers "wrong
// credentials" either way, and must.
func (h PasswordHasher) Verify(encoded, password string) bool {
	c, salt, want, err := parseArgon(encoded)
	if err != nil {
		return false
	}
	got := derive(func() []byte {
		return argon2.IDKey([]byte(password), salt, c.Time, c.MemoryKiB, c.Threads, uint32(len(want)))
	})
	return subtle.ConstantTimeCompare(got, want) == 1
}

// NeedsRehash is true when the verifier was not made by this hasher as it
// is now: another cost, or nothing readable.
func (h PasswordHasher) NeedsRehash(encoded string) bool {
	c, _, _, err := parseArgon(encoded)
	return err != nil || c != h.cost()
}

// Spend does the work of one verification and throws it away, for a login
// whose account does not exist. Answering at once would say exactly that: at
// argon2id's cost the difference is tens of milliseconds, and measuring it
// needs nothing but a stopwatch.
func (h PasswordHasher) Spend(password string) {
	h.Verify(nothing(h.cost()), password)
}

var nothings sync.Map // PasswordCost -> a verifier of no password anyone has

func nothing(c PasswordCost) string {
	if v, ok := nothings.Load(c); ok {
		return v.(string)
	}
	v, err := PasswordHasher{Cost: c}.Hash("")
	if err != nil {
		v = "" // no randomness on this host; Verify of "" is quick, and still false
	}
	nothings.Store(c, v)
	return v
}

func derive(f func() []byte) []byte {
	hashing <- struct{}{}
	defer func() { <-hashing }()
	return f()
}

// parseArgon reads the PHC form. The parameters are bounded: a verifier is
// read from the database, and one claiming a terabyte of memory must not be
// able to take the process down with it.
func parseArgon(encoded string) (c PasswordCost, salt, hash []byte, err error) {
	parts := strings.Split(encoded, "$") // "", "argon2id", "v=19", "m=..,t=..,p=..", salt, hash
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v="+strconv.Itoa(argon2.Version) {
		return c, nil, nil, errors.New("crypto: not an argon2id verifier")
	}
	var m, t, p uint64
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return c, nil, nil, fmt.Errorf("crypto: argon2id parameters %q", parts[3])
	}
	if m < 8 || m > 1<<20 || t < 1 || t > 64 || p < 1 || p > 64 || m < 8*p {
		return c, nil, nil, fmt.Errorf("crypto: argon2id parameters out of range: %q", parts[3])
	}
	enc := base64.RawStdEncoding
	if salt, err = enc.DecodeString(parts[4]); err != nil {
		return c, nil, nil, err
	}
	if hash, err = enc.DecodeString(parts[5]); err != nil || len(hash) < 16 {
		return c, nil, nil, errors.New("crypto: argon2id hash")
	}
	return PasswordCost{MemoryKiB: uint32(m), Time: uint32(t), Threads: uint8(p)}, salt, hash, nil
}
