package setup

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type spy struct {
	name string
	log  *[]string
	fail bool
}

func (s spy) OnTenantCreated(_ context.Context, id string) error {
	*s.log = append(*s.log, s.name+":create:"+id)
	if s.fail {
		return errors.New("no disk")
	}
	return nil
}

func (s spy) SeedExamples(_ context.Context, id string) error {
	*s.log = append(*s.log, s.name+":seed:"+id)
	return nil
}

func TestRunnerKeepsOrderAndStopsAtTheFirstFailure(t *testing.T) {
	var log []string
	r := NewRunner(nil, Step{"tenant", spy{"tenant", &log, false}}, Step{"identity", spy{"identity", &log, true}}, Step{"example", spy{"example", &log, false}})
	if got := strings.Join(r.Modules(), ","); got != "tenant,identity,example" {
		t.Fatal(got)
	}
	err := r.OnTenantCreated(context.Background(), "t1")
	if err == nil || !strings.Contains(err.Error(), `module "identity"`) || !strings.Contains(err.Error(), "no disk") {
		t.Fatalf("the error names the module and the cause: %v", err)
	}
	if got := strings.Join(log, " "); got != "tenant:create:t1 identity:create:t1" {
		t.Fatalf("stopped at the failure, in order: %s", got)
	}
	log = nil
	if err := r.SeedExamples(context.Background(), "t1"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(log, " "); got != "tenant:seed:t1 identity:seed:t1 example:seed:t1" {
		t.Fatal(got)
	}
}
