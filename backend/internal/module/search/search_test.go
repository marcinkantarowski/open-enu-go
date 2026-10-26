package search_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/flags"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/migrate"
	"github.com/open-enu/kernel/module"
	"github.com/open-enu/kernel/outbox"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/testdb"

	"github.com/open-enu/app/internal/module/example"
	"github.com/open-enu/app/internal/module/search"
)

// The endpoint over a module that declared a searchable type - Example,
// which indexes its projects in its own handlers. Through real HTTP against
// real Postgres, as a person with and without the type's permission.

const (
	tenantA = "11111111-1111-1111-1111-111111111111"
	tenantB = "22222222-2222-2222-2222-222222222222"
)

type stack struct {
	t      *testing.T
	h      http.Handler
	signer auth.Signer
}

func newStack(t *testing.T) *stack {
	t.Helper()
	db := testdb.Postgres(t)
	ctx := context.Background()
	reg, err := module.NewRegistry(example.Module, search.Module)
	if err != nil {
		t.Fatal(err)
	}
	var sources []migrate.Source
	for _, m := range reg.All() {
		sources = append(sources, migrate.Source{Name: m.Manifest.Name, FS: m.Migrations})
	}
	if _, err := migrate.Run(ctx, db.Admin, nil, sources...); err != nil {
		t.Fatal(err)
	}
	svc := platform.InMemory(slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc.Actor = auth.Actor{}
	svc.Searchable = reg.Searchable()
	if svc.Flags, err = flags.NewResolver(&flags.MemoryStore{}, reg.Flags()...); err != nil {
		t.Fatal(err)
	}
	if err := platform.WithDatabase(svc, &scope.Pool{Pool: db.App}, outbox.Options{MaxWorkers: 1, PollInterval: time.Second}, false); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	reg.Mount(mux, svc)
	signer := auth.Signer{Secret: []byte(platform.TestKey)}
	s := &stack{t: t, signer: signer, h: httpx.Chain(mux, httpx.RequestID, httpx.Recover, auth.Middleware(signer, auth.MemoryKeys{}))}
	// Two projects in A through the module's own API, so they are indexed
	// the way real ones are; one in B with the same word.
	s.create(tenantA, "Apollo launch plan")
	s.create(tenantA, "Budget review")
	s.create(tenantB, "Apollo in another workspace")
	return s
}

func (s *stack) token(tenant string, perms ...string) string {
	tok, err := s.signer.Sign(auth.Claims{Subject: "user-1", TenantID: tenant, Audience: auth.RealmTenant, Permissions: perms}, time.Hour)
	if err != nil {
		s.t.Fatal(err)
	}
	return tok
}

func (s *stack) create(tenant, name string) {
	s.t.Helper()
	req := httptest.NewRequest("POST", "/api/projects", strings.NewReader(`{"name":`+strconvQuote(name)+`}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.token(tenant, "example.view", "example.manage"))
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	if rec.Code != 201 {
		s.t.Fatalf("create %q: %d %s", name, rec.Code, rec.Body.String())
	}
}

func strconvQuote(v string) string { b, _ := json.Marshal(v); return string(b) }

type result struct {
	code  int
	raw   string
	Items []map[string]any `json:"items"`
	Meta  struct {
		Query string `json:"query"`
		Total int    `json:"total"`
	} `json:"meta"`
}

func (s *stack) search(authz, q string) result {
	s.t.Helper()
	req := httptest.NewRequest("GET", "/api/search?q="+q, nil)
	if authz != "" {
		req.Header.Set("Authorization", "Bearer "+authz)
	}
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	r := result{code: rec.Code, raw: rec.Body.String()}
	_ = json.Unmarshal(rec.Body.Bytes(), &r)
	return r
}

func TestAHitIsATypeAnIdAScoreAndAnExcerptFromTheCallersTenantOnly(t *testing.T) {
	s := newStack(t)
	r := s.search(s.token(tenantA, "example.view"), "apollo")
	if r.code != 200 || r.Meta.Total != 1 || len(r.Items) != 1 || r.Meta.Query != "apollo" {
		t.Fatalf("%d %s", r.code, r.raw)
	}
	hit := r.Items[0]
	if hit["type"] != "example.project" || hit["id"] == "" || hit["score"].(float64) <= 0 || !strings.Contains(hit["excerpt"].(string), "Apollo") {
		t.Fatalf("hit: %v", hit)
	}
	// Tenant B finds its own Apollo and not A's two.
	if r := s.search(s.token(tenantB, "example.view"), "apollo"); r.Meta.Total != 1 || !strings.Contains(r.Items[0]["excerpt"].(string), "another") {
		t.Fatalf("tenant B: %s", r.raw)
	}
	if r := s.search(s.token(tenantA, "example.view"), "budget"); r.Meta.Total != 1 {
		t.Fatalf("budget: %s", r.raw)
	}
}

func TestWithoutTheTypesPermissionThereAreNoHitsBecauseAnExcerptIsContent(t *testing.T) {
	s := newStack(t)
	r := s.search(s.token(tenantA, "audit.view"), "apollo")
	if r.code != 200 || r.Meta.Total != 0 || len(r.Items) != 0 {
		t.Fatalf("a caller without example.view got hits: %d %s", r.code, r.raw)
	}
	if r := s.search("", "apollo"); r.code != 401 {
		t.Fatalf("anonymous: %d", r.code)
	}
}

func TestAnEmptyQueryIsAnEmptyResultNotEverything(t *testing.T) {
	s := newStack(t)
	for _, q := range []string{"", "%20%20"} {
		r := s.search(s.token(tenantA, "example.view"), q)
		if r.code != 200 || r.Meta.Total != 0 || r.Items == nil || len(r.Items) != 0 || r.Meta.Query != "" {
			t.Fatalf("q=%q: %d %s", q, r.code, r.raw)
		}
	}
}
