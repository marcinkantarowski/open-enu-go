package example_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/open-enu/kernel/audit"
	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/event"
	"github.com/open-enu/kernel/flags"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/migrate"
	"github.com/open-enu/kernel/outbox"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/progress"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/search"
	"github.com/open-enu/kernel/testdb"

	"github.com/open-enu/app/internal/module/example"
)

// Every route, through real HTTP against a real Postgres under row-level
// security, as the application role. The read path runs twice - as a person
// and as an API key - because they authenticate differently, and a module
// that works for one and 403s for the other is the normal failure here.
//
// The module stands alone, without Tenant, Identity, ApiKey or Settings: the
// tenant is a uuid, the tokens are minted with the kernel's own signer, the
// key lives in an in-memory resolver, the flag in an in-memory store. Each of
// those is a kernel contract another module backs with a table in the
// process; none is a mock of the thing under test.

const (
	tenantA = "11111111-1111-1111-1111-111111111111"
	tenantB = "22222222-2222-2222-2222-222222222222"
	unknown = "0192f000-0000-7000-8000-0000000000ff"
)

type stack struct {
	t      *testing.T
	ctx    context.Context
	db     *testdb.DB
	svc    *platform.Services
	store  *flags.MemoryStore
	h      http.Handler
	signer auth.Signer
	key    string
}

func newStack(t *testing.T) *stack {
	t.Helper()
	db := testdb.Postgres(t)
	ctx := context.Background()
	if _, err := migrate.Run(ctx, db.Admin, nil, migrate.Source{Name: "example", FS: example.Module.Migrations}); err != nil {
		t.Fatal(err)
	}
	svc := platform.InMemory(slog.New(slog.NewTextHandler(io.Discard, nil)))
	store := &flags.MemoryStore{}
	resolver, err := flags.NewResolver(store, example.Module.Flags...)
	if err != nil {
		t.Fatal(err)
	}
	svc.Flags, svc.Actor = resolver, auth.Actor{}
	if err := platform.WithDatabase(svc, &scope.Pool{Pool: db.App}, outbox.Options{MaxWorkers: 2, PollInterval: 100 * time.Millisecond}, false); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	example.Module.Mount(mux, svc)
	signer := auth.Signer{Secret: []byte(platform.TestKey)}
	keys := auth.MemoryKeys{}
	key := keys.Add("sk_example-functional-test-key", auth.Principal{Subject: "key-1", TenantID: tenantA, Realm: auth.RealmTenant, Permissions: []string{"example.view"}})
	if err := example.LoadFixtures(ctx, svc, tenantA); err != nil {
		t.Fatal(err)
	}
	return &stack{
		t: t, ctx: ctx, db: db, svc: svc, store: store, signer: signer, key: key,
		h: httpx.Chain(mux, httpx.RequestID, httpx.Recover, auth.Middleware(signer, keys)),
	}
}

func (s *stack) token(tenant string, perms ...string) string {
	tok, err := s.signer.Sign(auth.Claims{Subject: "user-1", TenantID: tenant, Audience: auth.RealmTenant, Permissions: perms}, time.Hour)
	if err != nil {
		s.t.Fatal(err)
	}
	return tok
}

func (s *stack) owner() string { return s.token(tenantA, "example.view", "example.manage") }

func (s *stack) impersonated() string {
	tok, _ := s.signer.Sign(auth.Claims{
		Subject: "user-1", TenantID: tenantA, Audience: auth.RealmTenant, Permissions: []string{"example.view", "example.manage"},
		Impersonated: true, Actor: &auth.ActorInfo{Subject: "op-1", Realm: auth.RealmManager},
	}, time.Hour)
	return tok
}

type reply struct {
	code   int
	body   map[string]any
	header http.Header
}

func (r reply) str(path ...string) string  { v, _ := r.at(path...).(string); return v }
func (r reply) num(path ...string) float64 { v, _ := r.at(path...).(float64); return v }
func (r reply) at(path ...string) any {
	var cur any = r.body
	for _, k := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[k]
	}
	return cur
}

func (s *stack) do(method, path, authz string, payload any, headers map[string]string) reply {
	s.t.Helper()
	var body io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		body = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	if authz != "" {
		req.Header.Set("Authorization", "Bearer "+authz)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	r := reply{code: rec.Code, header: rec.Header()}
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &r.body); err != nil {
			s.t.Fatalf("%s %s: not JSON: %s", method, path, rec.Body.String())
		}
	}
	return r
}

func (s *stack) get(path, authz string) reply { return s.do("GET", path, authz, nil, nil) }
func (s *stack) post(path, authz string, payload any) reply {
	return s.do("POST", path, authz, payload, nil)
}
func (s *stack) patch(id, authz, version, name string) reply {
	var h map[string]string
	if version != "" {
		h = map[string]string{"If-Match": `"` + version + `"`}
	}
	return s.do("PATCH", "/api/projects/"+id, authz, map[string]any{"name": name}, h)
}

// work runs the outbox until n jobs have finished, then stops it.
func (s *stack) work(n int) {
	s.t.Helper()
	done, cancel := s.svc.Outbox.Completed()
	defer cancel()
	if err := s.svc.Outbox.Start(s.ctx); err != nil {
		s.t.Fatal(err)
	}
	defer s.svc.Outbox.Stop(s.ctx)
	for i := 0; i < n; i++ {
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			s.t.Fatalf("job %d of %d never finished", i+1, n)
		}
	}
}

func (s *stack) entries(action string) (out []audit.Entry) {
	for _, e := range s.svc.Audit.(*audit.Recorder).Entries() {
		if e.Action == action {
			out = append(out, e)
		}
	}
	return out
}

func (s *stack) countAs(ctx context.Context) int {
	var n int
	pool := &scope.Pool{Pool: s.db.App}
	if err := pool.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM example_project`).Scan(&n)
	}); err != nil {
		s.t.Fatal(err)
	}
	return n
}

// ── as a person ─────────────────────────────────────────────────────────

func TestTheListReturnsTheEnvelopeHidesTheEncryptedColumnAndOnlyThisTenant(t *testing.T) {
	s := newStack(t)
	// Tenant B has one project of its own, created through the API as B.
	if r := s.post("/api/projects", s.token(tenantB, "example.manage"), map[string]any{"name": "Other"}); r.code != 201 {
		t.Fatalf("B create: %d %v", r.code, r.body)
	}
	r := s.get("/api/projects", s.owner())
	items, _ := r.body["items"].([]any)
	if r.code != 200 || r.num("meta", "total") != 2 || r.num("meta", "pages") != 1 || len(items) != 2 {
		t.Fatalf("A list: %d %v", r.code, r.body)
	}
	// What leaves the server is decided per endpoint, not by the entity.
	if _, leaked := items[0].(map[string]any)["clientReference"]; leaked {
		t.Fatal("the encrypted column is in the list shape")
	}
	r = s.get("/api/projects?sort=name&direction=asc", s.owner())
	items, _ = r.body["items"].([]any)
	if r.code != 200 || items[0].(map[string]any)["name"] != "Alpha" {
		t.Fatalf("sorted: %d %v", r.code, r.body)
	}
	r = s.get("/api/projects", s.token(tenantB, "example.view"))
	items, _ = r.body["items"].([]any)
	if r.code != 200 || r.num("meta", "total") != 1 || items[0].(map[string]any)["name"] != "Other" {
		t.Fatalf("B list: %d %v", r.code, r.body)
	}
}

func TestTheEncryptedColumnIsPlaintextOnTheWireAndCiphertextOnDisk(t *testing.T) {
	s := newStack(t)
	r := s.get("/api/projects/"+example.FixtureAlpha, s.owner())
	if r.code != 200 || r.str("clientReference") != "ACME-0001" || r.header.Get("ETag") != `"1"` || r.num("version") != 1 {
		t.Fatalf("show: %d %v %v", r.code, r.body, r.header)
	}
	// Straight to the table as the superuser. Both halves matter:
	// decrypting transparently is useless if it was never encrypted, and
	// encrypting is useless if it cannot be read back.
	var stored string
	if err := s.db.Admin.QueryRow(s.ctx, `SELECT client_reference FROM example_project WHERE id = $1`, example.FixtureAlpha).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored, "v1:") || strings.Contains(stored, "ACME-0001") {
		t.Fatalf("on disk: %q", stored)
	}
}

func TestCreatingAProjectReturns201AndBroadcastsItAfterTheCommit(t *testing.T) {
	s := newStack(t)
	r := s.post("/api/projects", s.owner(), map[string]any{"name": " Gamma ", "attributes": map[string]any{"priority": "low"}})
	if r.code != 201 || r.str("name") != "Gamma" || r.str("attributes", "priority") != "low" || r.num("version") != 1 || r.str("id") == "" {
		t.Fatalf("create: %d %v", r.code, r.body)
	}
	e := s.entries("example.project.created")
	if len(e) != 1 || !e[0].Succeeded || e[0].Before != nil || e[0].After["name"] != "Gamma" || e[0].ActorID != "user-1" || e[0].TenantID != tenantA {
		t.Fatalf("trail: %+v", e)
	}
	if _, kept := e[0].After["clientReference"]; kept {
		t.Fatal("the encrypted column is in the snapshot")
	}
	s.work(1)
	ups := s.svc.Events.Hub.(*event.Recorder).Updates()
	if len(ups) != 1 || ups[0].Topic != event.Topic(tenantA) {
		t.Fatalf("broadcast: %+v", ups)
	}
	var env map[string]any
	_ = json.Unmarshal(ups[0].Data, &env)
	payload, _ := env["payload"].(map[string]any)
	if env["event"] != "example.project.created" || env["requiresPermission"] != "example.view" || payload["name"] != "Gamma" || len(payload) != 2 {
		t.Fatalf("envelope: %s", ups[0].Data)
	}
	if r := s.post("/api/projects", s.owner(), map[string]any{"name": "   "}); r.code != 400 || r.str("error", "code") != "bad_request" {
		t.Fatalf("blank name: %d %v", r.code, r.body)
	}
}

// The spec's maxLength and the VARCHAR(200) column count characters, so the
// check does too: two hundred two-byte letters are a valid name, one more is
// not - and the refusal is the same 400 as an empty one.
func TestARenameWithAStaleVersionIsRefusedWithBothVersions(t *testing.T) {
	s := newStack(t)
	// Without this, two people editing one record is last-write-wins and
	// the first person's change disappears silently.
	if r := s.patch(example.FixtureAlpha, s.owner(), "1", "First writer wins"); r.code != 200 || r.num("version") != 2 {
		t.Fatalf("first: %d %v", r.code, r.body)
	}
	r := s.patch(example.FixtureAlpha, s.owner(), "1", "Second writer must be refused")
	if r.code != 409 || r.str("error", "code") != "conflict" || r.num("error", "details", "yourVersion") != 1 || r.num("error", "details", "currentVersion") != 2 {
		t.Fatalf("stale: %d %v", r.code, r.body)
	}
	// The saved record travels with the conflict, so the UI can offer a
	// real choice instead of "your work is gone".
	if r.str("error", "details", "current", "name") != "First writer wins" {
		t.Fatalf("current: %v", r.body)
	}
	if r := s.get("/api/projects/"+example.FixtureAlpha, s.owner()); r.str("name") != "First writer wins" || r.header.Get("ETag") != `"2"` {
		t.Fatalf("row changed by the refused write: %v", r.body)
	}
	e := s.entries("example.project.renamed")
	if len(e) != 2 || !e[0].Succeeded || e[0].Before["name"] != "Alpha" || e[0].After["name"] != "First writer wins" || e[0].After["version"] != 2 || e[0].SubjectID != example.FixtureAlpha {
		t.Fatalf("trail of the write: %+v", e)
	}
	if e[1].Succeeded || !strings.Contains(e[1].FailureReason, "conflict") {
		t.Fatalf("the refused write is in the trail too: %+v", e[1])
	}
	// No If-Match: the client is not participating in locking, which a job
	// or an internal call may legitimately not.
	if r := s.patch(example.FixtureBeta, s.owner(), "", "Unlocked"); r.code != 200 || r.num("version") != 2 {
		t.Fatalf("unlocked: %d %v", r.code, r.body)
	}
}

func TestAnArchivedProjectIsRefusedByTheVoter(t *testing.T) {
	s := newStack(t)
	if _, err := s.db.Admin.Exec(s.ctx, `UPDATE example_project SET status = 'archived' WHERE id = $1`, example.FixtureAlpha); err != nil {
		t.Fatal(err)
	}
	// The permission allows editing projects; the voter refuses THIS row.
	if r := s.patch(example.FixtureAlpha, s.owner(), "1", "Too late"); r.code != 403 || r.str("error", "code") != "forbidden" {
		t.Fatalf("archived: %d %v", r.code, r.body)
	}
}

func TestAnImpersonatedSessionMayLookButNotRename(t *testing.T) {
	s := newStack(t)
	if r := s.get("/api/projects", s.impersonated()); r.code != 200 {
		t.Fatalf("support may look: %d", r.code)
	}
	if r := s.patch(example.FixtureAlpha, s.impersonated(), "1", "As someone else"); r.code != 403 {
		t.Fatalf("support may not act: %d %v", r.code, r.body)
	}
	e := s.entries("example.project.renamed")
	if len(e) != 1 || e[0].Succeeded || e[0].ActorID != "user-1" || e[0].OnBehalfOf != "op-1" || e[0].TenantID != tenantA {
		t.Fatalf("the trail names both identities: %+v", e)
	}
}

func TestTheArchiveRouteIs404WhileItsFlagIsOffAnd202WhenOn(t *testing.T) {
	s := newStack(t)
	// 404 rather than 403: "not built yet" and "switched off during an
	// incident" must look identical from outside.
	if r := s.post("/api/projects/archive", s.owner(), nil); r.code != 404 || r.str("error", "code") != "not_found" {
		t.Fatalf("flag off: %d %v", r.code, r.body)
	}
	s.store.Set(tenantA, "example.archive", true)
	r := s.post("/api/projects/archive", s.owner(), nil)
	jobID := r.str("jobId")
	// 202 and a job id: the work is accepted, not done. The browser
	// subscribes to this before anything has happened.
	if r.code != 202 || jobID == "" || r.num("total") != 2 {
		t.Fatalf("flag on: %d %v", r.code, r.body)
	}
	if e := s.entries("example.projects.archive_requested"); len(e) != 1 || e[0].After["jobId"] != jobID {
		t.Fatalf("the request names the work it started: %+v", e)
	}
	s.work(1)
	job, _ := s.svc.Progress.(*progress.Recorder).Get(jobID)
	if job.Status != progress.StatusDone || job.Done != 2 || job.Result["archived"] != 2 {
		t.Fatalf("progress: %+v", job)
	}
	list := s.get("/api/projects", s.owner())
	for _, it := range list.body["items"].([]any) {
		if it.(map[string]any)["status"] != "archived" {
			t.Fatalf("not archived: %v", it)
		}
	}
	// The job's write is a command like any other: audited, in the tenant.
	if e := s.entries("example.projects.archived"); len(e) != 1 || e[0].TenantID != tenantA || e[0].After["archived"] != 2 || e[0].ActorID != "" {
		t.Fatalf("the job's trail: %+v", e)
	}
	// The other tenant's flag is still off.
	if r := s.post("/api/projects/archive", s.token(tenantB, "example.manage"), nil); r.code != 404 {
		t.Fatalf("B flag: %d", r.code)
	}
}

func TestAnUnknownProjectIsANotFoundInTheStandardErrorShape(t *testing.T) {
	s := newStack(t)
	for _, id := range []string{unknown, "not-a-uuid"} {
		if r := s.get("/api/projects/"+id, s.owner()); r.code != 404 || r.str("error", "code") != "not_found" {
			t.Fatalf("%s: %d %v", id, r.code, r.body)
		}
		if r := s.patch(id, s.owner(), "1", "x"); r.code != 404 {
			t.Fatalf("rename %s: %d %v", id, r.code, r.body)
		}
	}
}

// ── search and the clock ───────────────────────────────────────────────

// hits runs a search as tenant sees it - the request's scope, no types.
func (s *stack) hits(tenant, query string) []search.Hit {
	s.t.Helper()
	var out []search.Hit
	if err := s.svc.DB.Run(scope.WithTenant(s.ctx, tenant), func(ctx context.Context, tx *scope.Tx) error {
		var err error
		out, err = s.svc.Search.Search(ctx, tx, query, nil, 10)
		return err
	}); err != nil {
		s.t.Fatal(err)
	}
	return out
}

func TestAProjectIsFoundByNameAndDescriptionNotByItsReferenceAndNotOnceArchived(t *testing.T) {
	s := newStack(t)
	r := s.post("/api/projects", s.owner(), map[string]any{"name": "Quarterly close", "description": "the invoice run for Q3", "clientReference": "SECRET-9"})
	if r.code != 201 {
		t.Fatalf("create: %d %v", r.code, r.body)
	}
	id := r.str("id")
	if h := s.hits(tenantA, "invoice"); len(h) != 1 || h[0].EntityType != "example.project" || h[0].EntityID != id || !strings.Contains(h[0].Excerpt, "invoice") {
		t.Fatalf("by description: %+v", h)
	}
	if h := s.hits(tenantA, "quarterly"); len(h) != 1 {
		t.Fatalf("by name: %+v", h)
	}
	// The encrypted column is not in the index: a secret in a table with no
	// access control of its own would be a secret no longer.
	if h := s.hits(tenantA, "SECRET-9"); len(h) != 0 {
		t.Fatalf("the reference is findable: %+v", h)
	}
	if h := s.hits(tenantB, "invoice"); len(h) != 0 {
		t.Fatalf("B finds A's project: %+v", h)
	}
	// Renamed: the index follows the row, in the same transaction.
	if r := s.patch(id, s.owner(), "", "Year end"); r.code != 200 {
		t.Fatalf("rename: %d", r.code)
	}
	if h := s.hits(tenantA, "year"); len(h) != 1 || len(s.hits(tenantA, "quarterly")) != 0 {
		t.Fatalf("after rename: %+v", h)
	}
	// Archived: no longer a hit. The list no longer leads with it either.
	s.store.Set(tenantA, "example.archive", true)
	if r := s.post("/api/projects/archive", s.owner(), nil); r.code != 202 {
		t.Fatalf("archive: %d", r.code)
	}
	s.work(1)
	if h := s.hits(tenantA, "invoice"); len(h) != 0 {
		t.Fatalf("archived and still findable: %+v", h)
	}
}

func TestTheDailySweepRemovesWhatWasArchivedLongEnoughAgoAndOnlyThat(t *testing.T) {
	s := newStack(t)
	if r := s.post("/api/projects", s.token(tenantB, "example.manage"), map[string]any{"name": "Other"}); r.code != 201 {
		t.Fatal(r.code)
	}
	// Alpha and B's project were archived forty days ago, Beta yesterday.
	// Straight to the table as the superuser - a test may backdate.
	for _, step := range []struct {
		where string
		args  []any
		age   string
	}{
		{"id = $1", []any{example.FixtureAlpha}, "40 days"},
		{"id = $1", []any{example.FixtureBeta}, "1 day"},
		{"tenant_id = $1", []any{tenantB}, "40 days"},
	} {
		if _, err := s.db.Admin.Exec(s.ctx, `UPDATE example_project SET status = 'archived', archived_at = now() - interval '`+step.age+`' WHERE `+step.where, step.args...); err != nil {
			t.Fatal(err)
		}
	}
	if err := example.Module.Tasks[0].Run(s.ctx, s.svc); err != nil {
		t.Fatal(err)
	}
	if r := s.get("/api/projects/"+example.FixtureAlpha, s.owner()); r.code != 404 {
		t.Fatalf("Alpha survived the sweep: %d", r.code)
	}
	if r := s.get("/api/projects/"+example.FixtureBeta, s.owner()); r.code != 200 || r.str("status") != "archived" {
		t.Fatalf("Beta, archived yesterday, was swept: %d %v", r.code, r.body)
	}
	if r := s.get("/api/projects", s.token(tenantB, "example.view")); r.num("meta", "total") != 0 {
		t.Fatalf("B's project survived: %v", r.body)
	}
	// One command per tenant, each in that tenant's trail, naming what left.
	e := s.entries("example.projects.swept")
	if len(e) != 2 || e[0].TenantID != tenantA || e[1].TenantID != tenantB || e[0].After != nil {
		t.Fatalf("trail: %+v", e)
	}
	if fmt.Sprint(e[0].Before["ids"]) != "["+example.FixtureAlpha+"]" {
		t.Fatalf("A's entry names what left: %v", e[0].Before)
	}
	// Idempotent: a second run finds nothing due and writes no entry.
	if err := example.Module.Tasks[0].Run(s.ctx, s.svc); err != nil || len(s.entries("example.projects.swept")) != 2 {
		t.Fatalf("second run: %v, %d entries", err, len(s.entries("example.projects.swept")))
	}
}

// ── as an API key, and as nobody ───────────────────────────────────────

func TestAnApiKeyScopedToViewCanListButNotCreate(t *testing.T) {
	s := newStack(t)
	if r := s.get("/api/projects", s.key); r.code != 200 || r.num("meta", "total") != 2 {
		t.Fatalf("key list: %d %v", r.code, r.body)
	}
	// The key was granted example.view and nothing else. A credential that
	// can do more than it was scoped to is the whole risk of having them.
	if r := s.post("/api/projects", s.key, map[string]any{"name": "Nope"}); r.code != 403 || r.str("error", "code") != "forbidden" {
		t.Fatalf("key create: %d %v", r.code, r.body)
	}
	if r := s.get("/api/projects", ""); r.code != 401 || r.str("error", "code") != "unauthenticated" {
		t.Fatalf("anonymous: %d %v", r.code, r.body)
	}
}

// ── the policy itself, on this table ───────────────────────────────────

func TestRowLevelSecurityOnTheRawTable(t *testing.T) {
	s := newStack(t)
	if r := s.post("/api/projects", s.token(tenantB, "example.manage"), map[string]any{"name": "Other"}); r.code != 201 {
		t.Fatal(r.code)
	}
	// No SET LOCAL: fail closed. A: A's two. Bypass: all three. No WHERE
	// tenant_id anywhere in the module - this is the policy at work.
	if n := s.countAs(s.ctx); n != 0 {
		t.Fatalf("no scope sees %d rows", n)
	}
	if n := s.countAs(scope.WithTenant(s.ctx, tenantA)); n != 2 {
		t.Fatalf("A sees %d rows", n)
	}
	var all int
	_ = scope.Unscoped(s.ctx, "test: the operator's view of every tenant", func(ctx context.Context) error {
		all = s.countAs(ctx)
		return nil
	})
	if all != 3 {
		t.Fatalf("bypass sees %d rows", all)
	}
}
