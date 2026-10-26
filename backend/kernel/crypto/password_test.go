package crypto_test

import (
	"strings"
	"testing"
	"time"

	"github.com/open-enu/kernel/crypto"
)

// argon2's floor, as the test platform uses it.
var cheap = crypto.PasswordCost{MemoryKiB: 8, Time: 1, Threads: 1}

func TestPasswordRoundTripSaltAndRehash(t *testing.T) {
	h := crypto.PasswordHasher{Cost: cheap}
	a, err := h.Hash("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := h.Hash("correct horse battery staple")
	if a == b {
		t.Fatal("two hashes of one password are equal - no salt")
	}
	if !strings.HasPrefix(a, "$argon2id$v=19$m=8,t=1,p=1$") || strings.Contains(a, "correct") {
		t.Fatalf("encoded form: %s", a)
	}
	if !h.Verify(a, "correct horse battery staple") || h.Verify(a, "correct horse battery stapl") {
		t.Fatal("verify is wrong")
	}
	for _, bad := range []string{"", "bcrypt$2y$x", "pbkdf2-sha256$abc$x$y", "$argon2id$v=16$m=8,t=1,p=1$AAAA$AAAA", "$argon2id$v=19$m=4294967295,t=1,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"} {
		if h.Verify(bad, "x") {
			t.Fatalf("a malformed verifier verified: %q", bad)
		}
	}
	if h.NeedsRehash(a) {
		t.Fatal("fresh hash needs rehash")
	}
	if !(crypto.PasswordHasher{}).NeedsRehash(a) || !h.NeedsRehash("junk") {
		t.Fatal("rehash not requested for another cost or a broken verifier")
	}
}

// A login for an address nobody has takes as long as one for an address
// somebody has: Spend runs the same derivation, and verifies nothing.
func TestSpendDoesTheWorkOfAVerification(t *testing.T) {
	h := crypto.PasswordHasher{Cost: crypto.PasswordCost{MemoryKiB: 8 * 1024, Time: 2, Threads: 1}}
	real, _ := h.Hash("correct horse battery staple")
	h.Spend("warm") // the first call also makes the verifier it checks against
	timed := func(f func()) time.Duration {
		start := time.Now()
		for range 5 {
			f()
		}
		return time.Since(start)
	}
	verify := timed(func() { h.Verify(real, "wrong password") })
	spend := timed(func() { h.Spend("wrong password") })
	if spend < verify/2 {
		t.Fatalf("Spend took %v, a verification %v - an unknown address would answer faster", spend, verify)
	}
}

// The PHC string format is shared with other implementations. This verifier
// was written by another one, at the default cost, for the password below -
// so verifiers carried in from another system check.
func TestAVerifierWrittenByAnotherImplementationChecks(t *testing.T) {
	const foreign = "$argon2id$v=19$m=65536,t=4,p=1$amcudXRWczU3cjkzYkNaVw$NGBBxXr56eiMhQ0IfgV0Yyg+OiKqS7Xa/juTwiagkJg"
	h := crypto.PasswordHasher{}
	if !h.Verify(foreign, "correct horse battery staple") || h.Verify(foreign, "Correct horse battery staple") {
		t.Fatal("a verifier written elsewhere does not check")
	}
	if h.NeedsRehash(foreign) {
		t.Fatal("a verifier at the default cost needs nothing")
	}
}
