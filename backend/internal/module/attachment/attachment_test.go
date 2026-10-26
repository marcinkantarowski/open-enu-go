package attachment_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/open-enu/kernel/audit"
	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/migrate"
	"github.com/open-enu/kernel/outbox"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/storage"
	"github.com/open-enu/kernel/testdb"

	"github.com/open-enu/app/internal/module/attachment"
)

const (
	tenantA = "11111111-1111-1111-1111-111111111111"
	tenantB = "22222222-2222-2222-2222-222222222222"
)

type stack struct {
	t    *testing.T
	ctx  context.Context
	db   *testdb.DB
	svc  *platform.Services
	h    http.Handler
	keys auth.MemoryKeys
}

func newStack(t *testing.T) *stack {
	t.Helper()
	db := testdb.Postgres(t)
	ctx := context.Background()
	if _, err := migrate.Run(ctx, db.Admin, nil, migrate.Source{Name: "attachment", FS: attachment.Module.Migrations}); err != nil {
		t.Fatal(err)
	}
	svc := platform.InMemory(slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc.Actor = auth.Actor{}
	if err := platform.WithDatabase(svc, &scope.Pool{Pool: db.App}, outbox.Options{MaxWorkers: 1, PollInterval: time.Second}, false); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	attachment.Module.Mount(mux, svc)
	// The kernel's download route, as the process mounts it, with the same
	// signer the storage signs with.
	storage.Download{Storage: svc.Storage, Signer: svc.Storage.(*storage.Memory).Signer}.Mount(mux)
	keys := auth.MemoryKeys{}
	return &stack{t: t, ctx: ctx, db: db, svc: svc, keys: keys, h: httpx.Chain(mux, httpx.RequestID, httpx.Recover, auth.Middleware(svc.Signer, keys))}
}

func (s *stack) token(tenant string, perms ...string) string {
	tok, _ := s.svc.Signer.Sign(auth.Claims{Subject: "u", TenantID: tenant, Audience: auth.RealmTenant, Permissions: perms}, time.Hour)
	return tok
}

type reply struct {
	code int
	body map[string]any
	raw  []byte
}

func (r reply) str(k string) string { v, _ := r.body[k].(string); return v }

func (s *stack) upload(authz string, fields map[string]string, filename string, content []byte) reply {
	s.t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	if filename != "" {
		part, _ := w.CreateFormFile("file", filename)
		part.Write(content)
	}
	w.Close()
	req := httptest.NewRequest("POST", "/api/attachments", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if authz != "" {
		req.Header.Set("Authorization", "Bearer "+authz)
	}
	return s.serve(req)
}

func (s *stack) get(url, authz string) reply {
	req := httptest.NewRequest("GET", url, nil)
	if authz != "" {
		req.Header.Set("Authorization", "Bearer "+authz)
	}
	return s.serve(req)
}

func (s *stack) serve(req *http.Request) reply {
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	r := reply{code: rec.Code, raw: rec.Body.Bytes()}
	_ = json.Unmarshal(r.raw, &r.body)
	return r
}

func TestAnUploadIsStoredUnderTheTenantTypedByItsBytesAndServedBySignedURL(t *testing.T) {
	s := newStack(t)
	owner := s.token(tenantA, "attachment.view", "attachment.manage")
	// Named .png and declared nothing: the bytes are text, so the record says text.
	r := s.upload(owner, map[string]string{"ownerType": "invoice", "ownerId": "inv-1"}, "notes.png", []byte("hello, this is plain text\n"))
	if r.code != 201 || !strings.HasPrefix(r.str("contentType"), "text/plain") || r.str("filename") != "notes.png" || r.body["size"] != float64(26) || r.str("ownerId") != "inv-1" {
		t.Fatalf("upload: %d %s", r.code, r.raw)
	}
	url := r.str("url")
	if !strings.Contains(url, storage.DownloadPath+tenantA+"/attachment/") || !strings.Contains(url, "sig=") || !strings.Contains(url, "expires=") {
		t.Fatalf("url: %s", url)
	}
	// The link downloads, as an attachment, without a bearer token.
	dl := s.get(strings.TrimPrefix(url, "http://test"), "")
	if dl.code != 200 || string(dl.raw) != "hello, this is plain text\n" {
		t.Fatalf("download: %d %s", dl.code, dl.raw)
	}
	// The record, with a fresh link, as a key holding only view.
	key := s.keys.Add("sk_attachment-test", auth.Principal{Subject: "k", TenantID: tenantA, Realm: auth.RealmTenant, Permissions: []string{"attachment.view"}})
	g := s.get("/api/attachments/"+r.str("id"), key)
	if g.code != 200 || g.str("id") != r.str("id") || !strings.Contains(g.str("url"), "sig=") {
		t.Fatalf("show: %d %s", g.code, g.raw)
	}
	if u := s.upload(key, nil, "x.txt", []byte("x")); u.code != 403 {
		t.Fatalf("a view-only key uploading: %d", u.code)
	}
	if g := s.get("/api/attachments/"+r.str("id"), s.token(tenantB, "attachment.view")); g.code != 404 {
		t.Fatalf("another tenant's record: %d", g.code)
	}
	if g := s.get("/api/attachments/"+r.str("id"), ""); g.code != 401 {
		t.Fatalf("anonymous: %d", g.code)
	}
	if e := entries(s, "attachment.uploaded"); len(e) != 1 || e[0].SubjectID != "inv-1" || e[0].TenantID != tenantA || e[0].After["storageKey"] == nil {
		t.Fatalf("audit: %+v", e)
	}
}

func TestRefusedUploads(t *testing.T) {
	s := newStack(t)
	owner := s.token(tenantA, "attachment.manage")
	if r := s.upload(owner, map[string]string{"ownerType": "x"}, "", nil); r.code != 400 || !strings.Contains(string(r.raw), "file_required") {
		t.Fatalf("no file: %d %s", r.code, r.raw)
	}
	attachment.SetMaxBytes(10)
	defer attachment.SetMaxBytes(25 << 20)
	if r := s.upload(owner, nil, "big.bin", bytes.Repeat([]byte{1}, 11)); r.code != 400 || !strings.Contains(string(r.raw), "maxBytes") {
		t.Fatalf("too large: %d %s", r.code, r.raw)
	}
	if r := s.upload(owner, nil, "ok.bin", bytes.Repeat([]byte{1}, 10)); r.code != 201 || !strings.HasPrefix(r.str("contentType"), "application/octet-stream") {
		t.Fatalf("at the limit: %d %s", r.code, r.raw)
	}
	if r := s.upload(s.token(tenantA, "attachment.view"), nil, "x.txt", []byte("x")); r.code != 403 {
		t.Fatalf("without manage: %d", r.code)
	}
}

func TestTheSweepRemovesUnclaimedFilesBlobFirst(t *testing.T) {
	s := newStack(t)
	owner := s.token(tenantA, "attachment.view", "attachment.manage")
	claimed := s.upload(owner, map[string]string{"ownerType": "invoice", "ownerId": "inv-1"}, "keep.txt", []byte("keep"))
	loose := s.upload(owner, nil, "loose.txt", []byte("loose"))
	fresh := s.upload(owner, nil, "fresh.txt", []byte("fresh"))
	for _, id := range []string{claimed.str("id"), loose.str("id")} {
		if _, err := s.db.Admin.Exec(s.ctx, `UPDATE attachment SET created_at = now() - interval '2 days' WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}
	}
	would, err := attachment.SweepOrphans(s.ctx, s.svc, 24, true)
	if err != nil || len(would) != 1 || would[0] != loose.str("id") {
		t.Fatalf("dry run: %v %v", would, err)
	}
	if _, err := attachment.SweepOrphans(s.ctx, s.svc, 24, false); err != nil {
		t.Fatal(err)
	}
	if g := s.get("/api/attachments/"+loose.str("id"), owner); g.code != 404 {
		t.Fatalf("the orphan survived: %d", g.code)
	}
	if dl := s.get(strings.TrimPrefix(loose.str("url"), "http://test"), ""); dl.code != 404 {
		t.Fatalf("the orphan's blob survived: %d", dl.code)
	}
	for _, kept := range []reply{claimed, fresh} {
		if g := s.get("/api/attachments/"+kept.str("id"), owner); g.code != 200 {
			t.Fatalf("%s was swept: %d", kept.str("filename"), g.code)
		}
	}
	if e := entries(s, "attachment.deleted"); len(e) != 1 || e[0].TenantID != tenantA || e[0].SubjectID != loose.str("id") {
		t.Fatalf("audit: %+v", e)
	}
}

func entries(s *stack, action string) (out []audit.Entry) {
	for _, e := range s.svc.Audit.(*audit.Recorder).Entries() {
		if e.Action == action {
			out = append(out, e)
		}
	}
	return out
}
