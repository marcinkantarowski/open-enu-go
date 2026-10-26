package module

import (
	"context"
	"fmt"
	"strings"

	"github.com/open-enu/kernel/platform"
)

// Cadence is how often the scheduler calls a Task.
type Cadence string

const (
	// EveryMinute is for a task whose whole job is to notice that a moment
	// has arrived (a due reminder, an expired reservation).
	EveryMinute Cadence = "minute"
	// Daily is housekeeping: purging what nobody verified, sweeping uploads
	// nobody attached. The dev stack runs it hourly so it can be watched.
	Daily Cadence = "daily"
)

// Task is work a module wants done on a clock rather than on a request.
// `openenu scheduler` runs every enabled module's tasks of a cadence, one
// after another, each in its own recovered call; a failure is logged and the
// next task still runs.
//
// A task is idempotent and safe to miss: a restart skips a run rather than
// doubling one, and a tick that takes longer than a minute delays the next
// one instead of overlapping it. So it decides what is DUE from its own data
// ("everything scheduled up to now"), never from "this is the 14:30 run". If
// a piece of work must not be missed, it is a job on the outbox, not a task.
type Task struct {
	// Name is prefixed with the module's name, like a permission or a flag:
	// `attachment.sweep_orphans`. It is what the log says.
	Name    string
	Cadence Cadence
	Run     func(ctx context.Context, svc *platform.Services) error
}

// Tasks returns every enabled module's tasks of one cadence, in module order.
func (r *Registry) Tasks(c Cadence) []Task {
	var out []Task
	for _, m := range r.ordered {
		for _, t := range m.Tasks {
			if t.Cadence == c {
				out = append(out, t)
			}
		}
	}
	return out
}

func (r *Registry) declareTasks(m Module) error {
	n := m.Manifest.Name
	for _, t := range m.Tasks {
		if !strings.HasPrefix(t.Name, n+".") || len(t.Name) == len(n)+1 {
			return fmt.Errorf("module %q declares task %q - a task is prefixed with its owner: %q", n, t.Name, n+".<what>")
		}
		if t.Cadence != EveryMinute && t.Cadence != Daily {
			return fmt.Errorf("module %q: task %q has cadence %q - one of %q or %q", n, t.Name, t.Cadence, EveryMinute, Daily)
		}
		if t.Run == nil {
			return fmt.Errorf("module %q: task %q has no Run", n, t.Name)
		}
	}
	return nil
}
