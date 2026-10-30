package spec

import (
	"reflect"
	"testing"
)

const fragment = `{"paths": {
  "/api/things/{id}": {
    "parameters": [{"name": "id", "in": "path"}],
    "get": {"operationId": "get_thing_show"},
    "delete": {"operationId": "delete_thing", "x-no-test-required": "destroys the fixture database"}
  },
  "/api/things": {"post": {"operationId": "post_thing_create", "x-mcp": false}}
}}`

func TestOperationsAreSortedAndCarryTheExemption(t *testing.T) {
	ops, err := Operations([]byte(fragment))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, o := range ops {
		got = append(got, o.Pattern()+" "+o.ID)
	}
	want := []string{"POST /api/things post_thing_create", "DELETE /api/things/{id} delete_thing", "GET /api/things/{id} get_thing_show"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	if ops[1].NoTestReason != "destroys the fixture database" || ops[2].NoTestReason != "" {
		t.Fatalf("exemption not read: %+v", ops)
	}
	if !ops[0].NoMCP || ops[1].NoMCP || ops[2].NoMCP {
		t.Fatalf("x-mcp not read: %+v", ops)
	}
	if p := PathParams("/api/t/{tenantId}/u/{userId}/x"); !reflect.DeepEqual(p, []string{"tenantId", "userId"}) {
		t.Fatalf("params %v", p)
	}
}

func TestOperationsRefusesBrokenJSON(t *testing.T) {
	if _, err := Operations([]byte(`{"paths": [}`)); err == nil {
		t.Fatal("no error")
	}
}
