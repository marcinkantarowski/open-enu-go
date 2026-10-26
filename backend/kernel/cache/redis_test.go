package cache

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/testdb"
)

// The Redis backend answers the same questions the memory one does: the
// tenant wrapper never serves one tenant's entry to another, tags
// invalidate, TTL expires.
func TestRedisBackendKeepsTheContract(t *testing.T) {
	r, err := NewRedis(testdb.Redis(t), "", "test:"+rand.Text()[:8]+":")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	ctx := context.Background()
	c := &Tenant{Backend: r}
	a, b := scope.WithTenant(ctx, "a"), scope.WithTenant(ctx, "b")
	compute, calls := counter()

	v1, _ := c.Get(a, "demo", "totals", 0, compute)
	v2, _ := c.Get(a, "demo", "totals", 0, compute)
	v3, _ := c.Get(b, "demo", "totals", 0, compute)
	if *calls != 2 || v1[0] != 1 || v2[0] != 1 || v3[0] != 2 {
		t.Fatalf("tenants separate: calls=%d %v %v %v", *calls, v1, v2, v3)
	}
	if err := c.InvalidateTenant(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := r.Get(ctx, "a.demo.totals"); ok {
		t.Fatal("a's entry dropped")
	}
	if _, ok, _ := r.Get(ctx, "b.demo.totals"); !ok {
		t.Fatal("b's entry kept")
	}
	if err := c.InvalidateModule(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := r.Get(ctx, "b.demo.totals"); ok {
		t.Fatal("module invalidation drops every tenant's")
	}

	if err := r.Set(ctx, "ttl", []byte("x"), 150*time.Millisecond, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := r.Get(ctx, "ttl"); !ok {
		t.Fatal("live within ttl")
	}
	time.Sleep(250 * time.Millisecond)
	if _, ok, _ := r.Get(ctx, "ttl"); ok {
		t.Fatal("expired")
	}
	_ = r.Set(ctx, "d", []byte{0, 1, 2}, 0, nil)
	if v, ok, _ := r.Get(ctx, "d"); !ok || len(v) != 3 || v[0] != 0 {
		t.Fatalf("binary values survive: %v %v", v, ok)
	}
	_ = r.Delete(ctx, "d")
	if _, ok, _ := r.Get(ctx, "d"); ok {
		t.Fatal("deleted")
	}
	if err := r.InvalidateTags(ctx, "never.set"); err != nil {
		t.Fatalf("an unknown tag is a no-op: %v", err)
	}
}

// A tag's set must not outlive every key in it, and must not expire while a
// key in it still lives - or an invalidation misses that key.
func TestATagLivesExactlyAsLongAsItsLongestLivedKey(t *testing.T) {
	r, err := NewRedis(testdb.Redis(t), "", "test:"+rand.Text()[:8]+":")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	ctx := context.Background()
	pttl := func(tag string) int64 {
		t.Helper()
		v, err := r.c.Do(ctx, r.c.B().Pttl().Key(r.t(tag)).Build()).AsInt64()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}

	_ = r.Set(ctx, "short", []byte("x"), time.Minute, []string{"expiring"})
	if got := pttl("expiring"); got <= 0 || got > time.Minute.Milliseconds() {
		t.Fatalf("a new tag takes its first key's TTL, PTTL = %d", got)
	}
	_ = r.Set(ctx, "long", []byte("x"), time.Hour, []string{"expiring"})
	if got := pttl("expiring"); got <= time.Minute.Milliseconds() {
		t.Fatalf("a longer-lived key extends the tag, PTTL = %d", got)
	}
	_ = r.Set(ctx, "short2", []byte("x"), time.Second, []string{"expiring"})
	if got := pttl("expiring"); got <= time.Minute.Milliseconds() {
		t.Fatalf("a shorter-lived key never shortens the tag, PTTL = %d", got)
	}

	_ = r.Set(ctx, "forever", []byte("x"), 0, []string{"mixed"})
	_ = r.Set(ctx, "brief", []byte("x"), time.Minute, []string{"mixed"})
	if got := pttl("mixed"); got != -1 {
		t.Fatalf("a tag holding a key with no expiry never expires, PTTL = %d", got)
	}
	if err := r.InvalidateTags(ctx, "mixed"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := r.Get(ctx, "forever"); ok {
		t.Fatal("the permanent key was reached through its tag")
	}
}
