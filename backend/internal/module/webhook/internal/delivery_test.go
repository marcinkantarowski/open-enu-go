package internal

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/open-enu/kernel/command"
	"github.com/open-enu/kernel/i18n"
	"github.com/open-enu/kernel/migrate"
	"github.com/open-enu/kernel/notification"
	"github.com/open-enu/kernel/outbox"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/testdb"
)

// The delivery path, end to end against real Postgres and a real HTTPS
// receiver: an event is published, the publisher records a delivery, the
// worker signs and posts it, and the receiver verifies the signature the way
// a third party would - from the spec, not from our signer.
//
// In package internal rather than webhook_test for one reason: the worker's
// HTTP client must trust the test receiver's certificate, and that client is
// the one thing Mount does not take from the platform.

const (
	tenantA = "11111111-1111-1111-1111-111111111111"
	tenantB = "22222222-2222-2222-2222-222222222222"
)

// receiver is a third party's endpoint: it records what arrived and answers
// with whatever status the test set.
type receiver struct {
	mu     sync.Mutex
	status int
	got    []received
	srv    *httptest.Server
}

type received struct {
	headers http.Header
	body    []byte
}

func newReceiver(t *testing.T) *receiver {
	r := &receiver{status: http.StatusOK}
	r.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.got = append(r.got, received{headers: req.Header.Clone(), body: body})
		status := r.status
		r.mu.Unlock()
		w.WriteHeader(status)
		_, _ = io.WriteString(w, strings.Repeat("x", 3000)) // longer than the log keeps
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *receiver) fail()    { r.mu.Lock(); r.status = http.StatusInternalServerError; r.mu.Unlock() }
func (r *receiver) recover() { r.mu.Lock(); r.status = http.StatusOK; r.mu.Unlock() }
func (r *receiver) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.got)
}

// demoEvent is a domain event from some module.
type demoEvent struct{ subject string }

func (demoEvent) EventName() string          { return "demo.thing.created" }
func (e demoEvent) SubjectID() string        { return e.subject }
func (demoEvent) OccurredAt() time.Time      { return time.Unix(1700000000, 0).UTC() }
func (demoEvent) Payload() map[string]any    { return map[string]any{"name": "Thing"} }
func (demoEvent) RequiresPermission() string { return "" }

type rig struct {
	t        *testing.T
	ctx      context.Context
	svc      *platform.Services
	repo     repository
	deliver  deliverer
	notified *notification.Recorder
}

// policy is the deliverer's real client, allowed onto the loopback where
// the test receiver listens and trusting its certificate.
func (r *receiver) policy() outbound {
	return outbound{allowPrivate: true, tls: r.srv.Client().Transport.(*http.Transport).TLSClientConfig}
}

func newRig(t *testing.T, o outbound) *rig {
	t.Helper()
	db := testdb.Postgres(t)
	ctx := context.Background()
	if _, err := migrate.Run(ctx, db.Admin, nil, migrate.Source{Name: "webhook", FS: migrationsFS(t)}); err != nil {
		t.Fatal(err)
	}
	svc := platform.InMemory(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := platform.WithDatabase(svc, &scope.Pool{Pool: db.App}, outbox.Options{MaxWorkers: 2, PollInterval: 100 * time.Millisecond}, false); err != nil {
		t.Fatal(err)
	}
	mount(http.NewServeMux(), svc, o)
	return &rig{t: t, ctx: ctx, svc: svc, repo: newRepository(svc), deliver: newDeliverer(svc, o), notified: svc.Notifier.(*notification.Recorder)}
}

// endpoint registers one through the bus, in the tenant's scope.
func (r *rig) endpoint(tenant, url string, events ...string) created {
	r.t.Helper()
	c, err := command.Dispatch[created](scope.WithTenant(r.ctx, tenant), r.svc.Bus, createEndpoint{URL: url, Events: events})
	if err != nil {
		r.t.Fatal(err)
	}
	return c
}

// publish raises the event as a handler would, in the tenant's scope.
func (r *rig) publish(tenant string, ev demoEvent) {
	r.t.Helper()
	err := r.svc.DB.Run(scope.WithTenant(r.ctx, tenant), func(ctx context.Context, tx *scope.Tx) error {
		return r.svc.Outbox.Publish(ctx, tx, ev)
	})
	if err != nil {
		r.t.Fatal(err)
	}
}

// work runs the outbox until n jobs have finished.
func (r *rig) work(n int) {
	r.t.Helper()
	done, cancel := r.svc.Outbox.Completed()
	defer cancel()
	if err := r.svc.Outbox.Start(r.ctx); err != nil {
		r.t.Fatal(err)
	}
	defer r.svc.Outbox.Stop(r.ctx)
	for i := 0; i < n; i++ {
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			r.t.Fatalf("job %d of %d never finished", i+1, n)
		}
	}
}

// record is what the publisher does for one endpoint, without River in the
// loop: the row and its queued job, which the test then runs by hand. The
// publisher itself is covered by the first test.
func (r *rig) record(tenant, endpointID string) string {
	r.t.Helper()
	d := newDelivery(tenant, endpointID, "demo.thing.created", map[string]any{"event": "demo.thing.created", "subjectId": "thing-1", "data": map[string]any{"name": "Thing"}})
	if err := r.repo.record(scope.WithTenant(r.ctx, tenant), d); err != nil {
		r.t.Fatal(err)
	}
	return d.ID
}

func (r *rig) deliveries(tenant string) []delivery {
	r.t.Helper()
	items, err := r.repo.recent(scope.WithTenant(r.ctx, tenant), "", 50)
	if err != nil {
		r.t.Fatal(err)
	}
	return items
}

func TestASubscribedEventArrivesSignedAndTheReceiverCanVerifyIt(t *testing.T) {
	rec := newReceiver(t)
	r := newRig(t, rec.policy())
	ep := r.endpoint(tenantA, rec.srv.URL+"/hook", "demo.thing.created")
	r.endpoint(tenantA, rec.srv.URL+"/other", "demo.thing.deleted") // subscribed to something else
	r.endpoint(tenantB, rec.srv.URL+"/b", "demo.thing.created")     // another tenant
	before := time.Now().Unix()
	r.publish(tenantA, demoEvent{subject: "thing-1"})
	r.work(2) // the event, then the one delivery it produced

	if rec.count() != 1 {
		t.Fatalf("receiver got %d requests, want exactly one: the subscribed endpoint of the raising tenant", rec.count())
	}
	got := rec.got[0]
	// The payload is the event, shaped for a third party.
	var payload map[string]any
	if err := json.Unmarshal(got.body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["event"] != "demo.thing.created" || payload["subjectId"] != "thing-1" || payload["occurredAt"] != "2023-11-14T22:13:20Z" || payload["data"].(map[string]any)["name"] != "Thing" {
		t.Fatalf("payload: %s", got.body)
	}
	// Verified as the receiver would, from the spec and the secret it was
	// handed at creation - not with our signer.
	id, tsRaw, sig := got.headers.Get("webhook-id"), got.headers.Get("webhook-timestamp"), got.headers.Get("webhook-signature")
	ts, err := strconv.ParseInt(tsRaw, 10, 64)
	if err != nil || ts < before || ts > time.Now().Unix() {
		t.Fatalf("timestamp %q is not now", tsRaw)
	}
	mac := hmac.New(sha256.New, []byte(ep.secret))
	mac.Write([]byte(id + "." + tsRaw + "."))
	mac.Write(got.body)
	if want := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil)); sig != want {
		t.Fatalf("signature %q, receiver computed %q", sig, want)
	}
	if !verify(sig, id, ts, got.body, ep.secret, time.Now()) {
		t.Fatal("our own verifier refuses what we sent")
	}
	// The log says delivered, and the id in the header is the log's row.
	ds := r.deliveries(tenantA)
	if len(ds) != 1 || ds[0].ID != id || ds[0].Status != statusDelivered || ds[0].Attempts != 1 || *ds[0].ResponseCode != 200 {
		t.Fatalf("log: %+v", ds)
	}
	if len(r.deliveries(tenantB)) != 0 {
		t.Fatal("tenant B has a delivery for tenant A's event")
	}
}

func TestTheFifthFailureClosesTheRowAndTellsTheTenant(t *testing.T) {
	rec := newReceiver(t)
	rec.fail()
	r := newRig(t, rec.policy())
	ep := r.endpoint(tenantA, rec.srv.URL+"/hook", "demo.thing.created")
	r.record(tenantA, ep.endpoint.ID)
	ds := r.deliveries(tenantA)
	if len(ds) != 1 || ds[0].Status != statusPending || ds[0].Attempts != 0 {
		t.Fatalf("after publish: %+v", ds)
	}
	// Each attempt as River would run it - without waiting for its backoff.
	ctx := scope.WithTenant(r.ctx, tenantA)
	for i := 1; i <= maxAttempts; i++ {
		if err := r.deliver.run(ctx, deliverJob{DeliveryID: ds[0].ID}); err == nil {
			t.Fatalf("attempt %d: no error, so River would not retry", i)
		}
		d := r.deliveries(tenantA)[0]
		want := statusPending
		if i == maxAttempts {
			want = statusFailed
		}
		if d.Attempts != i || d.Status != want || *d.ResponseCode != 500 || len(*d.ResponseBody) != responseBodyLimit {
			t.Fatalf("attempt %d: %+v", i, d)
		}
		if n := len(r.notified.Sent()); (i < maxAttempts && n != 0) || (i == maxAttempts && n != 1) {
			t.Fatalf("attempt %d: %d notifications", i, n)
		}
	}
	sent := r.notified.Sent()[0]
	if sent.To != "tenant:"+tenantA || sent.Type != NotificationDeliveryFailed || sent.Context["attempts"] != 5 || sent.Context["url"] != ep.endpoint.URL || sent.Context["event"] != "demo.thing.created" {
		t.Fatalf("notification: %+v", sent)
	}
	// A sixth run, if River ever made one, still works and is still an error.
	if err := r.deliver.run(ctx, deliverJob{DeliveryID: ds[0].ID}); err == nil {
		t.Fatal("sixth attempt silently succeeded")
	}
}

func TestADeactivatedEndpointAndAGoneDeliveryAreNoOps(t *testing.T) {
	rec := newReceiver(t)
	r := newRig(t, rec.policy())
	ep := r.endpoint(tenantA, rec.srv.URL+"/hook", "demo.thing.created")
	id := r.record(tenantA, ep.endpoint.ID)
	ctx := scope.WithTenant(r.ctx, tenantA)
	if _, err := command.Dispatch[endpoint](ctx, r.svc.Bus, deactivateEndpoint{ID: ep.endpoint.ID}); err != nil {
		t.Fatal(err)
	}
	// Deactivated while queued: nothing sent, nothing retried, the row stays
	// as it was - an error here would make River try five times.
	if err := r.deliver.run(ctx, deliverJob{DeliveryID: id}); err != nil || rec.count() != 0 {
		t.Fatalf("deactivated: err=%v requests=%d", err, rec.count())
	}
	if d := r.deliveries(tenantA)[0]; d.Attempts != 0 || d.Status != statusPending {
		t.Fatalf("row changed: %+v", d)
	}
	if err := r.deliver.run(ctx, deliverJob{DeliveryID: "0192f000-0000-7000-8000-0000000000ff"}); err != nil {
		t.Fatalf("a gone delivery: %v", err)
	}
	// And nothing is published for an event nobody subscribed to.
	r.publish(tenantA, demoEvent{subject: "thing-2"})
	r.work(1)
	if len(r.deliveries(tenantA)) != 1 {
		t.Fatal("a delivery was recorded for a deactivated endpoint")
	}
}

func TestRedeliverResetsTheSameRowAndQueuesItAgain(t *testing.T) {
	rec := newReceiver(t)
	rec.fail()
	r := newRig(t, rec.policy())
	ep := r.endpoint(tenantA, rec.srv.URL+"/hook", "demo.thing.created")
	id := r.record(tenantA, ep.endpoint.ID)
	ctx := scope.WithTenant(r.ctx, tenantA)
	_ = r.deliver.run(ctx, deliverJob{DeliveryID: id})
	// The receiver recovers; the tenant asks for the same delivery again.
	rec.recover()
	d, err := command.Dispatch[delivery](ctx, r.svc.Bus, redeliver{DeliveryID: id})
	if err != nil || d.ID != id || d.Status != statusPending || d.Attempts != 0 || d.ResponseCode != nil {
		t.Fatalf("redeliver: %+v %v", d, err)
	}
	// Two jobs are now queued: the original, which River has not yet run,
	// and the redelivery. Both run against the same row and
	// the recovered receiver; the log counts every attempt, and stays one row.
	r.work(2)
	ds := r.deliveries(tenantA)
	if len(ds) != 1 || ds[0].ID != id || ds[0].Status != statusDelivered || ds[0].Attempts != 2 || rec.count() != 3 {
		t.Fatalf("after redeliver: %+v, %d requests", ds, rec.count())
	}
}

func TestAHostnameResolvingToAPrivateAddressIsNeverDialled(t *testing.T) {
	rec := newReceiver(t)
	_, port, _ := strings.Cut(strings.TrimPrefix(rec.srv.URL, "https://"), ":")
	// The receiver is on the loopback; the name says nothing about that,
	// the resolver does. Production policy: no private addresses.
	var answer []netip.Addr
	o := rec.policy()
	o.allowPrivate = false
	o.resolve = func(context.Context, string) ([]netip.Addr, error) { return answer, nil }
	r := newRig(t, o)
	ep := r.endpoint(tenantA, "https://hooks.example.test:"+port+"/hook", "demo.thing.created")
	for _, answer = range [][]netip.Addr{
		{netip.MustParseAddr("127.0.0.1")},
		{netip.MustParseAddr("10.1.2.3")},
		{netip.MustParseAddr("169.254.169.254")},
		{netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("127.0.0.1")}, // one public answer is not enough
		{netip.MustParseAddr("::ffff:127.0.0.1")},
	} {
		id := r.record(tenantA, ep.endpoint.ID)
		if err := r.deliver.run(scope.WithTenant(r.ctx, tenantA), deliverJob{DeliveryID: id}); err == nil {
			t.Fatalf("%v: delivered", answer)
		}
		d, err := r.repo.findDelivery(scope.WithTenant(r.ctx, tenantA), id)
		if err != nil {
			t.Fatal(err)
		}
		if rec.count() != 0 || d.ResponseCode != nil || d.ResponseBody == nil || *d.ResponseBody != errAddressRefused.Error() {
			t.Fatalf("%v: %d requests reached the receiver, log %+v", answer, rec.count(), d)
		}
	}
}

func TestARedirectIsNotFollowedAndItsBodyIsNotKept(t *testing.T) {
	target := newReceiver(t) // where the redirect points: must never be reached
	redirector := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Location", target.srv.URL+"/latest/meta-data/")
		w.WriteHeader(http.StatusFound)
		_, _ = io.WriteString(w, "secret-looking redirect body")
	}))
	t.Cleanup(redirector.Close)
	o := target.policy()
	o.tls = redirector.Client().Transport.(*http.Transport).TLSClientConfig
	r := newRig(t, o)
	ep := r.endpoint(tenantA, redirector.URL+"/hook", "demo.thing.created")
	id := r.record(tenantA, ep.endpoint.ID)
	if err := r.deliver.run(scope.WithTenant(r.ctx, tenantA), deliverJob{DeliveryID: id}); err == nil {
		t.Fatal("a redirect counted as a delivery")
	}
	d := r.deliveries(tenantA)[0]
	if target.count() != 0 || d.Status != statusPending || d.Attempts != 1 || d.ResponseBody == nil || *d.ResponseBody != errRedirectRefused.Error() {
		t.Fatalf("redirect: target reached %d times, log %+v", target.count(), d)
	}
}

func TestTheURLRuleRefusesInternalAddressesUnlessPrivateIsAllowed(t *testing.T) {
	strict, dev := outbound{}, outbound{allowPrivate: true}
	for raw, want := range map[string]i18n.Key{
		"https://hooks.example.test/x":      "",
		"https://hooks.example.test:8443/x": "",
		"https://93.184.216.34/x":           "",
		"http://hooks.example.test/x":       keyHTTPSRequired,
		"https:///x":                        keyHTTPSRequired,
		"https://localhost/x":               keyAddressRefused,
		"https://api.localhost./x":          keyAddressRefused,
		"https://127.0.0.1/x":               keyAddressRefused,
		"https://10.0.0.5/x":                keyAddressRefused,
		"https://172.16.0.1/x":              keyAddressRefused,
		"https://192.168.1.1/x":             keyAddressRefused,
		"https://169.254.169.254/latest/":   keyAddressRefused,
		"https://100.64.0.1/x":              keyAddressRefused,
		"https://[::1]/x":                   keyAddressRefused,
		"https://[fd00::1]/x":               keyAddressRefused,
		"https://[fe80::1]/x":               keyAddressRefused,
		"https://[::ffff:10.0.0.1]/x":       keyAddressRefused,
		"https://0.0.0.0/x":                 keyAddressRefused,
		"https://224.0.0.1/x":               keyAddressRefused,
		"https://hooks.example.test:5432/x": keyAddressRefused,
		"https://hooks.example.test:6379/x": keyAddressRefused,
	} {
		if got := strict.checkURL(raw); got != want {
			t.Errorf("%s: %q, want %q", raw, got, want)
		}
	}
	// The dev stack's switch lifts the address rules, never https or ports.
	if dev.checkURL("https://10.0.0.5/x") != "" || dev.checkURL("http://10.0.0.5/x") != keyHTTPSRequired || dev.checkURL("https://10.0.0.5:5432/x") != keyAddressRefused {
		t.Fatal("allowPrivate lifted the wrong rules")
	}
}
