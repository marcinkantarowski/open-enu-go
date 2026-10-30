package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/open-enu/kernel/heartbeat"
	"github.com/open-enu/kernel/module"
	"github.com/open-enu/kernel/platform"
)

func init() {
	subcommands["scheduler"] = command{summary: "run every module's clock tasks (module.Task)", run: runScheduler}
}

// runScheduler is the third process of the stack. A loop and not cron: cron
// in a container needs its own daemon, its own log plumbing and its own copy
// of the environment, and reports failures to a mailbox nobody reads. This
// writes to stdout, where every other container's output already goes.
//
// Two cadences. EveryMinute tasks run each SCHEDULER_TICK_SECONDS (60); Daily
// tasks run at start and then every SCHEDULER_INTERVAL_SECONDS (86400 - the
// dev stack sets an hour, so they can be watched in a working session). Tasks
// run one after another, so a slow tick delays the next rather than
// overlapping it.
//
// The loop touches HEARTBEAT_FILE (/tmp/openenu-heartbeat) every 30 s and
// after every run, for `openenu health -heartbeat`.
func runScheduler(args []string) int {
	fs := flag.NewFlagSet("scheduler", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	log := newLogger(os.Stdout, nil)
	tick, err := envSeconds("SCHEDULER_TICK_SECONDS", 60)
	if err != nil {
		log.Error("scheduler refused to start", "error", err)
		return 2
	}
	interval, err := envSeconds("SCHEDULER_INTERVAL_SECONDS", 86400)
	if err != nil {
		log.Error("scheduler refused to start", "error", err)
		return 2
	}
	reg, err := registry()
	if err != nil {
		log.Error("module registry refused to start", "error", err)
		return 1
	}
	log = newLogger(os.Stdout, nil, reg.LogFields()...)
	slog.SetDefault(log)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	svc, err := newServices(ctx, log, reg)
	if err != nil {
		log.Error("platform refused to start", "error", err)
		return 1
	}
	defer svc.DB.Close()
	// A task dispatches commands like any caller; mounting registers the
	// handlers on the bus. The mux is thrown away.
	reg.Mount(newMux(), svc)

	minute, daily := reg.Tasks(module.EveryMinute), reg.Tasks(module.Daily)
	log.Info("scheduler started", "minute_tasks", len(minute), "daily_tasks", len(daily),
		"tick_seconds", int(tick.Seconds()), "interval_seconds", int(interval.Seconds()))

	// The heartbeat is beaten by this loop itself, between tasks: a task that
	// hangs stops it, and the container's healthcheck sees the scheduler is
	// no longer turning (kernel/heartbeat).
	beatPath := heartbeat.Path()
	beat := func() {
		if err := heartbeat.Touch(beatPath); err != nil {
			log.Error("heartbeat not written", "error", err.Error())
		}
	}
	beat()
	runTasks(ctx, svc, daily, log)
	beat()
	beats := time.NewTicker(heartbeat.Interval)
	defer beats.Stop()
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	housekeeping := time.NewTicker(interval)
	defer housekeeping.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info("scheduler stopped")
			return 0
		case <-beats.C:
			beat()
		case <-ticker.C:
			runTasks(ctx, svc, minute, log)
			beat()
		case <-housekeeping.C:
			runTasks(ctx, svc, daily, log)
			beat()
		}
	}
}

// runTasks runs each task once, recovering a panic so one failing task
// never stops the others - and never the scheduler, which would then be
// silently doing nothing.
func runTasks(ctx context.Context, svc *platform.Services, tasks []module.Task, log *slog.Logger) {
	for _, t := range tasks {
		if ctx.Err() != nil {
			return
		}
		started := time.Now()
		err := runOne(ctx, svc, t)
		ms := time.Since(started).Milliseconds()
		if err != nil {
			log.Error("task failed", "task", t.Name, "cadence", string(t.Cadence), "duration_ms", ms, "error", err.Error())
			continue
		}
		log.Info("task ran", "task", t.Name, "cadence", string(t.Cadence), "duration_ms", ms)
	}
}

func runOne(ctx context.Context, svc *platform.Services, t module.Task) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("panic: %v", p)
		}
	}()
	return t.Run(ctx, svc)
}

func envSeconds(key string, def int) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return time.Duration(def) * time.Second, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%s must be a positive number of seconds, got %q", key, v)
	}
	return time.Duration(n) * time.Second, nil
}
