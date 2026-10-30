package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/open-enu/kernel"
	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/module"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/realtime"
	"github.com/open-enu/kernel/storage"
)

func init() {
	subcommands["api"] = command{summary: "serve the API (-addr :8080)", run: runAPI}
}

func runAPI(args []string) int {
	fs := flag.NewFlagSet("api", flag.ContinueOnError)
	addr := fs.String("addr", envOr("API_ADDR", ":8080"), "listen address")
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

	handler, err := newHandler(reg, svc, log)
	if err != nil {
		log.Error("request pipeline refused to start", "error", err)
		return 1
	}
	srv := &httpx.Server{Addr: *addr, Handler: handler, Log: log}
	if err := srv.Run(ctx); err != nil {
		log.Error("server stopped", "error", err)
		return 1
	}
	return 0
}

// newHandler is the whole request pipeline: every module's routes behind the
// kernel's middleware, the health probes beside them, and the envelope for
// anything unmatched.
func newHandler(reg *module.Registry, svc *platform.Services, log *slog.Logger) (http.Handler, error) {
	mux := newMux()
	health := &httpx.Health{Stage: envOr("APP_STAGE", "dev"), Kernel: kernel.Version, Token: os.Getenv("HEALTH_TOKEN"), Checks: map[string]httpx.Check{}}
	if svc.DB != nil {
		health.Checks["postgres"] = func(ctx context.Context) error { return svc.DB.Ping(ctx) }
	}
	health.Mount(mux)
	reg.Mount(mux, svc)
	// The browser's way into SSE: a cookie scoped to the caller's tenant
	// topics, issued by the kernel (kernel/realtime). Secure unless the API
	// itself is served over plain http, which only a test does.
	apiURL := envOr("API_URL", "http://localhost:8080")
	mux.Handle("POST /api/realtime/token", realtime.Tokens{
		Secret:   []byte(os.Getenv("MERCURE_JWT_SECRET")),
		Issuer:   apiURL,
		Audience: apiURL + realtime.CookiePath,
		Secure:   strings.HasPrefix(apiURL, "https://"),
	})
	// Signed downloads: the URL an attachment carries, verified by the same
	// signer that minted it (kernel/storage.Download).
	if storageSigner != nil {
		storage.Download{Storage: svc.Storage, Signer: storageSigner}.Mount(mux)
	}
	mux.Handle("/", httpx.NotFoundHandler())
	// The signer verifies what Identity issues; API keys resolve through the
	// one module that declares Keys (ApiKey), NoKeys otherwise.
	keys, err := reg.Keys(svc)
	if err != nil {
		return nil, err
	}
	authn := auth.Middleware(svc.Signer, keys)
	// CORS: the two browser apps, by exact origin (ADR-0021). Both
	// derive from DOMAIN; unset (a test) means no browser is expected.
	cors := httpx.CORS(os.Getenv("APP_URL"), os.Getenv("MANAGER_URL"))
	// Locale twice, on purpose: once from Accept-Language so the auth
	// middleware's own 401 is translated, and again after it with the
	// modules' preferences, which need the principal (a person's stored
	// choice, a tenant's default) and override the header.
	//
	// Every body is capped at httpx.DefaultMaxBody (a 413 in the envelope
	// beyond it); a module's upload route raises its own with
	// httpx.AllowBody. A malformed If-Match is a 400 here, so no handler
	// reads it as "no lock".
	return httpx.Chain(mux, httpx.RequestID, cors, httpx.AccessLog(log), httpx.Recover,
		httpx.Locale(svc.I18n), httpx.MaxBody(httpx.DefaultMaxBody), httpx.ValidIfMatch,
		authn, httpx.Locale(svc.I18n, reg.LocalePreferences(svc)...)), nil
}

func newMux() *http.ServeMux { return http.NewServeMux() }

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// modulesCmd prints what discovery resolved - the first thing to run when a
// route is missing: if the module is not in this table, no amount of handler
// debugging will help.
func init() {
	subcommands["modules"] = command{summary: "show what module discovery resolved", run: runModules}
}

func runModules([]string) int {
	reg, err := registry()
	if err != nil {
		fmt.Fprintln(os.Stderr, "openenu modules:", err)
		return 1
	}
	all := reg.All()
	if len(all) == 0 {
		fmt.Println("no modules - a directory under internal/module/ with a module.yaml becomes one (make module NAME=x)")
		return 0
	}
	fmt.Printf("%-14s %-8s %-7s %-24s %s\n", "MODULE", "ROUTES", "TABLES", "DEPENDS", "PERMISSIONS")
	for _, m := range all {
		fmt.Printf("%-14s %-8s %-7s %-24s %s\n",
			m.Manifest.Name, yesNo(m.Mount != nil), yesNo(m.Migrations != nil),
			join(m.Manifest.Depends), join(m.Permissions))
	}
	return 0
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "-"
}

func join(s []string) string {
	if len(s) == 0 {
		return "-"
	}
	out := s[0]
	for _, x := range s[1:] {
		out += ", " + x
	}
	return out
}
