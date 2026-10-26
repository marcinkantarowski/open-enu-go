package mcp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const fragment = `{"paths": {
  "/api/projects": {"get": {"operationId": "get_project_index"}, "post": {"operationId": "post_project_create"}},
  "/api/projects/{id}": {"get": {"operationId": "get_project_show"}},
  "/api/auth/login": {"post": {"operationId": "post_auth_login"}},
  "/api/manager/tenants": {"get": {"operationId": "get_manager_tenant_index"}},
  "/api/orphan": {"get": {"operationId": "get_orphan"}}
}}`

func catalogue() Catalogue {
	return Catalogue{
		Fragments: [][]byte{[]byte(fragment)},
		// login has no permission (public); manager needs the operator realm,
		// not a permission; orphan names one no module declares.
		Permissions: map[string]string{"get_project_index": "example.view", "post_project_create": "example.manage", "get_project_show": "example.view", "get_orphan": "ghost.view"},
		Declared:    func(p string) bool { return strings.HasPrefix(p, "example.") },
	}
}

func TestOnlyDeclaredPermissionGuardedOperationsBecomeTools(t *testing.T) {
	tools, err := catalogue().Tools()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range tools {
		names = append(names, tool.Name)
		if !strings.Contains(tool.Description, `requires the "`+tool.Permission+`" permission`) {
			t.Errorf("%s: description does not name the permission: %s", tool.Name, tool.Description)
		}
	}
	if got := strings.Join(names, " "); got != "get_project_index get_project_show post_project_create" {
		t.Fatalf("tools: %s", got)
	}
	show := tools[1].InputSchema
	if req, _ := show["required"].([]string); len(req) != 1 || req[0] != "id" {
		t.Fatalf("show requires %v", show["required"])
	}
	if _, ok := tools[2].InputSchema["properties"].(map[string]any)["body"]; !ok {
		t.Fatal("create has no body argument")
	}
	if _, ok := tools[0].InputSchema["properties"].(map[string]any)["query"]; !ok {
		t.Fatal("index has no query argument")
	}
}

func TestNoFragmentsIsAnErrorNotAnEmptyList(t *testing.T) {
	if _, err := (Catalogue{}).Tools(); err == nil {
		t.Fatal("an empty catalogue should refuse")
	}
}

// The server, end to end over an in-memory transport, against a fake API
// that behaves like the real one: a bearer key in, JSON out, 403 as content.
func TestServerCallsTheAPIAsAClientAndReturnsRefusalsAsContent(t *testing.T) {
	var seen []string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen = append(seen, r.Method+" "+r.URL.RequestURI()+" "+r.Header.Get("Authorization")+" "+string(body))
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			w.WriteHeader(403)
			_, _ = w.Write([]byte(`{"error":{"message":"you lack example.manage"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"p-1"}`))
	}))
	defer api.Close()
	tools, err := catalogue().Tools()
	if err != nil {
		t.Fatal(err)
	}
	srv := Server{Tools: tools, BaseURL: api.URL + "/", Key: "sk_test", Version: "test"}.Build()
	st, ct := sdk.NewInMemoryTransports()
	ctx := context.Background()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	listed, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) != 3 {
		t.Fatalf("listed %d tools", len(listed.Tools))
	}
	res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "get_project_show", Arguments: map[string]any{"id": "p 1", "query": map[string]any{"expand": "all"}}})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || text(res) != `{"id":"p-1"}` {
		t.Fatalf("show: isError=%v %s", res.IsError, text(res))
	}
	res, err = cs.CallTool(ctx, &sdk.CallToolParams{Name: "post_project_create", Arguments: map[string]any{"body": map[string]any{"name": "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(text(res), "you lack example.manage") {
		t.Fatalf("create: isError=%v %s", res.IsError, text(res))
	}
	if len(seen) != 2 || seen[0] != "GET /api/projects/p%201?expand=all Bearer sk_test " || seen[1] != `POST /api/projects Bearer sk_test {"name":"x"}` {
		t.Fatalf("requests: %q", seen)
	}
}

func text(res *sdk.CallToolResult) string {
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*sdk.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}
