package crypto

import (
	"errors"
	"strings"
	"testing"
)

const (
	master = "0123456789abcdef0123456789abcdef"
	tenant = "11111111-2222-3333-4444-555555555555"
	// A fixed test vector (HKDF-SHA256 + AES-256-GCM, computed independently
	// of this package) with the IV 00..0b and the plaintext "hello, world".
	vectorPayloadTenant = "v1:AAECAwQFBgcICQoL:kkVG7YfrxjxM5d/K:HuAosQ6/T76vqwPIzhAOVg=="
	vectorPayloadGlobal = "v1:AAECAwQFBgcICQoL:xGx9ZxS71Pd4LHE7:sA37apR1XC/mLeZfjL67iA=="
	vectorKeyTenant     = "620377ec0ef2f3666f769a54c419f71e498fabbd96ce76a70fb43feeb19b292c"
)

func enc(t *testing.T) *Encryptor {
	t.Helper()
	keys, err := NewDerivedKeys(master)
	if err != nil {
		t.Fatal(err)
	}
	return NewEncryptor(keys)
}

func TestDerivationMatchesVector(t *testing.T) {
	keys, _ := NewDerivedKeys(master)
	k, err := keys.KeyFor(tenant)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.ToLower(hexOf(k)); got != vectorKeyTenant {
		t.Fatalf("derived key differs from the vector:\n got %s\nwant %s", got, vectorKeyTenant)
	}
	if _, err := NewDerivedKeys("short"); err == nil || !strings.Contains(err.Error(), "make env") {
		t.Fatalf("short master must be refused with the fix named, got %v", err)
	}
}

func TestDecryptsTheVector(t *testing.T) {
	e := enc(t)
	for _, c := range []struct{ payload, tenant string }{{vectorPayloadTenant, tenant}, {vectorPayloadGlobal, ""}} {
		got, err := e.Decrypt(c.payload, c.tenant)
		if err != nil || got != "hello, world" {
			t.Fatalf("tenant %q: got %q, %v", c.tenant, got, err)
		}
	}
}

func TestRoundTripAndFailures(t *testing.T) {
	e := enc(t)
	p1, err := e.Encrypt("secret", tenant)
	if err != nil {
		t.Fatal(err)
	}
	p2, _ := e.Encrypt("secret", tenant)
	if p1 == p2 {
		t.Fatal("two encryptions of the same value must differ (random IV)")
	}
	if !strings.HasPrefix(p1, "v1:") || strings.Count(p1, ":") != 3 {
		t.Fatalf("format v1:iv:ct:tag, got %s", p1)
	}
	if got, err := e.Decrypt(p1, tenant); err != nil || got != "secret" {
		t.Fatalf("round trip: %q %v", got, err)
	}
	if _, err := e.Decrypt(p1, "other-tenant"); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("another tenant's key must fail with ErrDecrypt, got %v", err)
	}
	if _, err := e.Decrypt("plain text", tenant); !errors.Is(err, ErrNotEncrypted) {
		t.Fatalf("plaintext must be ErrNotEncrypted, got %v", err)
	}
	parts := strings.Split(p1, ":")
	parts[2] = parts[2][:len(parts[2])-2] + "AA"
	if _, err := e.Decrypt(strings.Join(parts, ":"), tenant); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("tampered ciphertext must be ErrDecrypt, got %v", err)
	}
}

// The lookup hash is HMAC-SHA256 under HKDF(master, info
// "open-enu:lookup:v1"), both computed independently of this package: a
// change here orphans every *_hash column, deliberately or not at all.
func TestHashForLookupMatchesVector(t *testing.T) {
	const wantKey = "fb0c37997846daf4a1e5007dae5a12d2b1f90007690b519e27942996388f32f3"
	const want = "c3bedb083d55bdc7fd2a4ce522a0d56dee0fe0802ffb678eb81bf4831f8cda07"
	keys, _ := NewDerivedKeys(master)
	if k, _ := keys.LookupKey(); hexOf(k) != wantKey {
		t.Fatalf("lookup key differs from the vector: %s", hexOf(k))
	}
	e := enc(t)
	if got := e.HashForLookup("  Alice@Example.COM \n"); got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	if e.HashForLookup("a") == e.HashForLookup("b") {
		t.Fatal("distinct values must hash differently")
	}
	other, _ := NewDerivedKeys("another-master-key-another-master-key")
	if NewEncryptor(other).HashForLookup("alice@example.com") == want {
		t.Fatal("the hash must depend on the master key, or a dump is reversible by dictionary")
	}
}

// A provider without its own lookup key still gets a keyed hash.
func TestHashForLookupUnderAPlainKeyProvider(t *testing.T) {
	e := NewEncryptor(fixedKey{})
	if e.HashForLookup("x") == "" || e.HashForLookup("x") != e.HashForLookup(" X ") {
		t.Fatal("normalised and keyed")
	}
}

type fixedKey struct{}

func (fixedKey) KeyFor(string) ([]byte, error) { return make([]byte, keyBytes), nil }

func TestDeriveKeyMatchesVectorAndSeparatesPurposes(t *testing.T) {
	const want = "c87f781130f8785a833db13fc7980d04ddcce2292890847424dbb7b250bfaccf"
	if got := hexOf(DeriveKey([]byte(master), "open-enu:secret:jwt:v1")); got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	if hexOf(DeriveKey([]byte(master), "a")) == hexOf(DeriveKey([]byte(master), "b")) {
		t.Fatal("two purposes must get two keys")
	}
}

func hexOf(b []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, c := range b {
		out = append(out, digits[c>>4], digits[c&15])
	}
	return string(out)
}
