package internal

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"testing"
	"time"
)

// A fixed test vector, with the expected value pinned: a change that alters
// these bytes breaks verification for every receiver already integrated.
const (
	vectorSecret    = "whsec_test_secret"
	vectorID        = "msg_01"
	vectorTimestamp = 1700000000
	vectorPayload   = `{"event":"example.project.created"}`
	vectorExpected  = "v1,hRXt/SA8J+qcplkcnKGAoxk7bQb4HuEZfSvU0KLXpvo="
)

func TestTheSignatureMatchesThePinnedVectorAndTheStandard(t *testing.T) {
	h := headers(vectorID, vectorTimestamp, []byte(vectorPayload), vectorSecret)
	if h["webhook-signature"] != vectorExpected || h["webhook-id"] != vectorID || h["webhook-timestamp"] != "1700000000" {
		t.Fatalf("headers: %v", h)
	}
	// Recomputed from the spec rather than from sign(): a test that signs
	// with the code under test and verifies with it proves only that it
	// agrees with itself.
	mac := hmac.New(sha256.New, []byte(vectorSecret))
	mac.Write([]byte(vectorID + ".1700000000." + vectorPayload))
	if "v1,"+base64.StdEncoding.EncodeToString(mac.Sum(nil)) != vectorExpected {
		t.Fatal("the pinned vector disagrees with the spec")
	}
}

func TestChangingAnyPartOfTheSignedStringChangesTheSignature(t *testing.T) {
	base := sign("msg_01", 1700000000, []byte(`{"a":1}`), vectorSecret)
	for name, other := range map[string]string{
		"id":        sign("msg_02", 1700000000, []byte(`{"a":1}`), vectorSecret),
		"timestamp": sign("msg_01", 1700000001, []byte(`{"a":1}`), vectorSecret),
		"payload":   sign("msg_01", 1700000000, []byte(`{"a":2}`), vectorSecret),
		"secret":    sign("msg_01", 1700000000, []byte(`{"a":1}`), "whsec_other"),
	} {
		if other == base {
			t.Fatalf("%s is not part of the signed string", name)
		}
	}
}

func TestAnOldTimestampIsRefusedEvenWithAValidSignature(t *testing.T) {
	now := time.Unix(1700000000, 0)
	stale := now.Unix() - toleranceSeconds - 1
	header := "v1," + sign("msg_01", stale, []byte(`{"a":1}`), vectorSecret)
	// Genuine signature, replayed request. Without the window a captured
	// request stays valid forever.
	if verify(header, "msg_01", stale, []byte(`{"a":1}`), vectorSecret, now) {
		t.Fatal("a stale timestamp was accepted")
	}
	fresh := "v1," + sign("msg_01", now.Unix(), []byte(`{"a":1}`), vectorSecret)
	if !verify(fresh, "msg_01", now.Unix(), []byte(`{"a":1}`), vectorSecret, now) {
		t.Fatal("a fresh signature was refused")
	}
	if verify(fresh, "msg_01", now.Unix(), []byte(`{"a":2}`), vectorSecret, now) {
		t.Fatal("a tampered payload verified")
	}
}
