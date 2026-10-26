package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/open-enu/kernel/module"
	"github.com/open-enu/kernel/platform"
)

// One task panicking or failing does not stop the next: the scheduler's
// whole value is that it keeps going.
func TestRunTasksSurvivesAFailingTask(t *testing.T) {
	var out bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&out, nil))
	svc := platform.InMemory(log)
	ran := 0
	tasks := []module.Task{
		{Name: "a.panics", Cadence: module.Daily, Run: func(context.Context, *platform.Services) error { panic("boom") }},
		{Name: "a.fails", Cadence: module.Daily, Run: func(context.Context, *platform.Services) error { return errors.New("no") }},
		{Name: "a.works", Cadence: module.Daily, Run: func(context.Context, *platform.Services) error { ran++; return nil }},
	}
	runTasks(context.Background(), svc, tasks, log)
	if ran != 1 {
		t.Fatalf("the healthy task ran %d times", ran)
	}
	s := out.String()
	for _, want := range []string{`"task":"a.panics"`, `panic: boom`, `"task":"a.fails"`, `"error":"no"`, `"task":"a.works"`, `"msg":"task ran"`} {
		if !strings.Contains(s, want) {
			t.Fatalf("log lacks %s:\n%s", want, s)
		}
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	ran = 0
	runTasks(cancelled, svc, tasks[2:], log)
	if ran != 0 {
		t.Fatal("a cancelled context must stop the loop before the next task")
	}

	if _, err := envSeconds("SCHEDULER_TICK_SECONDS", 60); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SCHEDULER_TICK_SECONDS", "0")
	if _, err := envSeconds("SCHEDULER_TICK_SECONDS", 60); err == nil {
		t.Fatal("zero seconds must be refused")
	}
}
