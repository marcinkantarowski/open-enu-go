package main

import (
	"strings"
	"testing"
)

// What an agent is offered, and what it is deliberately not. The list is
// generated, so it cannot go stale - but it can go WRONG, and both ways are
// security-shaped: offering what a scoped key can never call (an agent
// burning its turns on guaranteed 401s), or what it should never call at
// all. Against the real registry, the real fragments and the generated ACL,
// with nothing mocked: the rule under test is "these three sources agree".
func TestMCPCatalogueOffersExactlyThePermissionGuardedOperations(t *testing.T) {
	reg, err := registry()
	if err != nil {
		t.Fatal(err)
	}
	tools, err := catalogue(reg).Tools()
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) == 0 {
		t.Fatal("no tools")
	}
	byName := map[string]bool{}
	for _, tool := range tools {
		byName[tool.Name] = true
		if _, ok := reg.OwnerOf(tool.Permission); !ok {
			t.Errorf("%s requires %q, which no module declares", tool.Name, tool.Permission)
		}
		if !strings.Contains(tool.Description, `requires the "`+tool.Permission+`" permission`) {
			t.Errorf("%s: the description must say what the key needs: %s", tool.Name, tool.Description)
		}
		// The operator realm refuses an api_key token on its aud claim, and
		// the auth endpoints establish browser sessions a key has no use for.
		// A segment, not a prefix: /api/authorization is a module's own.
		if under(tool.Path, "/api/manager") || under(tool.Path, "/api/auth") {
			t.Errorf("%s is offered but a key can never call %s", tool.Name, tool.Path)
		}
	}
	// The reference module's ordinary permission-guarded POST: if this one
	// disappears, the generation has broken.
	if !byName["post_example_project_create"] || !byName["get_example_project_show"] {
		t.Fatalf("the Example operations are missing from %d tools", len(tools))
	}
	for _, tool := range tools {
		if tool.Name == "get_example_project_show" {
			if req, _ := tool.InputSchema["required"].([]string); len(req) != 1 || req[0] != "id" {
				t.Fatalf("show requires %v, want [id]", tool.InputSchema["required"])
			}
		}
	}
}

// under is whether path is base or below it, by whole segments.
func under(path, base string) bool { return path == base || strings.HasPrefix(path, base+"/") }
