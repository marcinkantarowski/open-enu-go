package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/open-enu/kernel/platform"

	"github.com/open-enu/app/internal/module/api_key"
	"github.com/open-enu/app/internal/module/attachment"
	"github.com/open-enu/app/internal/module/identity"
	"github.com/open-enu/app/internal/module/manager"
	"github.com/open-enu/app/internal/module/tenant"
)

// The operator's console: thin wrappers over functions each module exports
// from its root package. No logic here - a subcommand parses flags, boots
// the platform the way `api` does, calls one function and prints its answer.

func init() {
	subcommands["user"] = command{summary: "user create --email --password [--tenant --slug --role --name --seed]", run: runUser}
	subcommands["tenant"] = command{summary: "tenant purge-unverified [--days 7 --dry-run]", run: runTenant}
	subcommands["apikey"] = command{summary: "apikey create --tenant <id|slug> --name --permission ... [--days]", run: runApiKey}
	subcommands["manager"] = command{summary: "manager create --email --password [--name --super]", run: runManager}
	subcommands["attachment"] = command{summary: "attachment sweep-orphans [--hours 24 --dry-run]", run: runAttachment}
}

// boot is what every console command shares with `api`: the registry, the
// services and the handlers on the bus. Logs go to stderr so stdout is the
// answer and only the answer.
func boot(ctx context.Context) (*platform.Services, error) {
	reg, err := registry()
	if err != nil {
		return nil, err
	}
	log := newLogger(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}, reg.LogFields()...)
	svc, err := newServices(ctx, log, reg)
	if err != nil {
		return nil, err
	}
	reg.Mount(newMux(), svc)
	return svc, nil
}

func fail(name string, err error) int {
	fmt.Fprintf(os.Stderr, "openenu %s: %v\n", name, err)
	return 1
}

func wantVerb(fs *flag.FlagSet, args []string, verb string) ([]string, bool) {
	if len(args) == 0 || args[0] != verb {
		fmt.Fprintf(os.Stderr, "usage: openenu %s %s [flags]\n", fs.Name(), verb)
		fs.SetOutput(os.Stderr)
		fs.PrintDefaults()
		return nil, false
	}
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return nil, false
	}
	return fs.Args(), true
}

func runUser(args []string) int {
	fs := flag.NewFlagSet("user", flag.ContinueOnError)
	var in identity.CreateUserInput
	fs.StringVar(&in.Email, "email", "", "the address (required)")
	fs.StringVar(&in.Password, "password", "", "the password (required)")
	fs.StringVar(&in.DisplayName, "name", "", "display name (defaults to the address)")
	fs.StringVar(&in.TenantName, "tenant", "", "workspace name, created and activated when its slug is new")
	fs.StringVar(&in.TenantSlug, "slug", "", "workspace slug (defaults to the name, slugified)")
	fs.StringVar(&in.Role, "role", "owner", "owner, admin or member")
	fs.BoolVar(&in.Seed, "seed", false, "run every module's example seed in the workspace")
	if _, ok := wantVerb(fs, args, "create"); !ok {
		return 2
	}
	if in.Email == "" || in.Password == "" || (in.TenantName == "" && in.TenantSlug == "") {
		return fail("user create", fmt.Errorf("--email, --password and --tenant (or --slug) are required"))
	}
	ctx := context.Background()
	svc, err := boot(ctx)
	if err != nil {
		return fail("user create", err)
	}
	defer svc.DB.Close()
	out, err := identity.CreateUser(ctx, svc, in)
	if err != nil {
		return fail("user create", err)
	}
	fmt.Printf("user %s in tenant %s (seeded: %t)\n", out.UserID, out.TenantID, out.Seeded)
	return 0
}

func runTenant(args []string) int {
	fs := flag.NewFlagSet("tenant", flag.ContinueOnError)
	days := fs.Int("days", 7, "remove workspaces still pending after this many days")
	dry := fs.Bool("dry-run", false, "list, do not remove")
	if _, ok := wantVerb(fs, args, "purge-unverified"); !ok {
		return 2
	}
	ctx := context.Background()
	svc, err := boot(ctx)
	if err != nil {
		return fail("tenant purge-unverified", err)
	}
	defer svc.DB.Close()
	gone, err := tenant.PurgeUnverified(ctx, svc, *days, *dry)
	if err != nil {
		return fail("tenant purge-unverified", err)
	}
	verb := "removed"
	if *dry {
		verb = "would remove"
	}
	for _, t := range gone {
		fmt.Printf("%s %s (%s, pending since %s)\n", verb, t.Slug, t.ID, t.CreatedAt.Format("2006-01-02"))
	}
	fmt.Printf("%s %d\n", verb, len(gone))
	return 0
}

type repeated []string

func (r *repeated) String() string     { return strings.Join(*r, ",") }
func (r *repeated) Set(v string) error { *r = append(*r, v); return nil }

func runApiKey(args []string) int {
	fs := flag.NewFlagSet("apikey", flag.ContinueOnError)
	var in api_key.CreateKeyInput
	var perms repeated
	fs.StringVar(&in.Tenant, "tenant", "", "workspace id or slug (required)")
	fs.StringVar(&in.Name, "name", "", "what the key is for (required)")
	fs.Var(&perms, "permission", "a permission the key holds; repeat for more (required)")
	fs.IntVar(&in.Days, "days", 0, "expire after this many days (0: never)")
	if _, ok := wantVerb(fs, args, "create"); !ok {
		return 2
	}
	in.Permissions = perms
	if in.Tenant == "" || in.Name == "" || len(perms) == 0 {
		return fail("apikey create", fmt.Errorf("--tenant, --name and at least one --permission are required"))
	}
	ctx := context.Background()
	svc, err := boot(ctx)
	if err != nil {
		return fail("apikey create", err)
	}
	defer svc.DB.Close()
	secret, id, prefix, err := api_key.CreateKey(ctx, svc, in)
	if err != nil {
		return fail("apikey create", err)
	}
	fmt.Printf("id      %s\nprefix  %s\nsecret  %s\n\nStore the secret now; it is shown once and nothing keeps it.\n", id, prefix, secret)
	return 0
}

func runAttachment(args []string) int {
	fs := flag.NewFlagSet("attachment", flag.ContinueOnError)
	hours := fs.Int("hours", 24, "remove unclaimed uploads older than this many hours")
	dry := fs.Bool("dry-run", false, "list, do not remove")
	if _, ok := wantVerb(fs, args, "sweep-orphans"); !ok {
		return 2
	}
	ctx := context.Background()
	svc, err := boot(ctx)
	if err != nil {
		return fail("attachment sweep-orphans", err)
	}
	defer svc.DB.Close()
	ids, err := attachment.SweepOrphans(ctx, svc, *hours, *dry)
	if err != nil {
		return fail("attachment sweep-orphans", err)
	}
	verb := "removed"
	if *dry {
		verb = "would remove"
	}
	for _, id := range ids {
		fmt.Printf("%s %s\n", verb, id)
	}
	fmt.Printf("%s %d\n", verb, len(ids))
	return 0
}

func runManager(args []string) int {
	fs := flag.NewFlagSet("manager", flag.ContinueOnError)
	var in manager.CreateOperatorInput
	fs.StringVar(&in.Email, "email", "", "the address (required)")
	fs.StringVar(&in.Password, "password", "", "at least 16 characters (required)")
	fs.StringVar(&in.DisplayName, "name", "", "display name")
	fs.BoolVar(&in.Super, "super", false, "may manage other operators")
	if _, ok := wantVerb(fs, args, "create"); !ok {
		return 2
	}
	if in.Email == "" || in.Password == "" {
		return fail("manager create", fmt.Errorf("--email and --password are required"))
	}
	ctx := context.Background()
	svc, err := boot(ctx)
	if err != nil {
		return fail("manager create", err)
	}
	defer svc.DB.Close()
	id, created, err := manager.CreateOperator(ctx, svc, in)
	if err != nil {
		return fail("manager create", err)
	}
	if created {
		fmt.Printf("operator %s created (%s)\n", in.Email, id)
	} else {
		fmt.Printf("operator %s already exists (%s)\n", in.Email, id)
	}
	return 0
}
