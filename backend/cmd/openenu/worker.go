package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/open-enu/kernel/heartbeat"
	"github.com/open-enu/kernel/platform"
)

func init() {
	subcommands["worker"] = command{summary: "work the outbox: events, broadcasts, background jobs", run: runWorker}
}

// startRetry is how long the worker waits between attempts to start while the
// database is not ready for it.
const startRetry = 5 * time.Second

// startOutbox starts the outbox, retrying until it does or the context ends.
// The worker comes up with the stack, before `openenu migrate` has created
// River's tables, and a worker that exits on that is one that a developer
// restarts by hand after every `make builddev` - or forgets to, and the job
// they are waiting for never runs. The condition waited on is the real one:
// Start succeeding. Returns false only when the context ended first.
func startOutbox(ctx context.Context, svc *platform.Services, log *slog.Logger) bool {
	for attempt := 1; ; attempt++ {
		err := svc.Outbox.Start(ctx)
		if err == nil {
			return true
		}
		log.Warn("worker cannot start yet - retrying", "attempt", attempt, "retry_in", startRetry.String(), "error", err.Error(),
			"hint", "the schema is usually not migrated yet: make migrate")
		select {
		case <-ctx.Done():
			log.Error("worker never started", "error", err.Error())
			return false
		case <-time.After(startRetry):
		}
	}
}

// runWorker is the second process of the stack. Same platform as `api`,
// started the other way round: the outbox works jobs here and only inserts
// them there. One binary, two roles, so a job and a handler can never
// disagree about a type.
func runWorker(args []string) int {
	fs := flag.NewFlagSet("worker", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	log := newLogger(os.Stdout, nil)
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
	// Mounting registers each module's command handlers and subscribers on
	// the bus and the outbox; the mux is thrown away.
	reg.Mount(newMux(), svc)
	if !startOutbox(ctx, svc, log) {
		return 1
	}
	log.Info("worker started", "heartbeat", heartbeat.Path())
	// Alive is: River still running and the database answering. Either
	// failing stops the beat and the container's healthcheck sees it.
	go heartbeat.Run(ctx, heartbeat.Path(), heartbeat.Interval, func(ctx context.Context) error {
		select {
		case <-svc.Outbox.Stopped():
			return errors.New("the outbox has stopped")
		default:
		}
		return svc.DB.Ping(ctx)
	}, log)
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := svc.Outbox.Stop(shutdown); err != nil {
		log.Error("worker stopped uncleanly", "error", err)
		return 1
	}
	log.Info("worker stopped")
	return 0
}
