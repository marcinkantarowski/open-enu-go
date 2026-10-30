package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/open-enu/kernel/audit"
	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/cache"
	"github.com/open-enu/kernel/crypto"
	"github.com/open-enu/kernel/event"
	"github.com/open-enu/kernel/flags"
	"github.com/open-enu/kernel/i18n"
	"github.com/open-enu/kernel/mail"
	"github.com/open-enu/kernel/module"
	"github.com/open-enu/kernel/notification"
	"github.com/open-enu/kernel/outbox"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/progress"
	"github.com/open-enu/kernel/realtime"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/setup"
	"github.com/open-enu/kernel/storage"
)

// newServices composes the platform from the environment - the one place
// a backing service is chosen. Postgres through DATABASE_URL (the
// application role: NOSUPERUSER, no BYPASSRLS), Redis through REDIS_ADDR
// when set and the memory cache otherwise, local storage under STORAGE_ROOT
// with signed download URLs, the audit trail in Postgres, progress persisted
// in Postgres and pushed to the hub, mail from MAILER_DSN, and the hub
// itself: Mercure when MERCURE_URL is set (MERCURE_PUBLISHER_JWT_SECRET required),
// otherwise a publisher that logs each update. The second half comes from
// the registry once the pool exists: the Notification module's notifier
// (a logging one until then), every module's tenant setup steps, and the
// Settings module's flag store, over which the flag resolver is rebuilt so
// per-tenant overrides replace the in-memory ones it started with.
//
// Every secret is required. A default here would be a default in production.
//
// APP_SECRET is one value with two consumers, each signing with its own
// HKDF sub-key: a download URL's signature is never a token's, and the other
// way round. Changing an info string invalidates what that consumer signed.
const (
	secretInfoTokens  = "open-enu:secret:jwt:v1"
	secretInfoStorage = "open-enu:secret:storage-url:v1"
)

// storageSigner is kept for the download route (api.go), which verifies
// what the Storage's TemporaryURL signed.
var storageSigner *storage.Signer

func newServices(ctx context.Context, log *slog.Logger, reg *module.Registry) (*platform.Services, error) {
	locales := strings.Split(envOr("APP_LOCALES", "en,pl"), ",")
	cat, err := i18n.NewCatalogue(envOr("APP_DEFAULT_LOCALE", "en"), locales...)
	if err != nil {
		return nil, err
	}
	if err := reg.LoadLocales(cat); err != nil {
		return nil, err
	}
	keys, err := crypto.NewDerivedKeys(os.Getenv("APP_ENCRYPTION_KEY"))
	if err != nil {
		return nil, err
	}
	secret := os.Getenv("APP_SECRET")
	if len(secret) < 32 {
		return nil, fmt.Errorf("APP_SECRET must be set and at least 32 characters; `make env` generates it")
	}
	resolver, err := flags.NewResolver(&flags.MemoryStore{}, reg.Flags()...)
	if err != nil {
		return nil, err
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, fmt.Errorf("DATABASE_URL must be set - the application role's DSN, never the owner's")
	}
	pool, err := scope.Open(ctx, dsn)
	if err != nil {
		return nil, err
	}
	// The policies hold only for a role Postgres holds to them; a superuser
	// or BYPASSRLS DSN would serve every tenant every row, silently.
	if err := scope.VerifyApplicationRole(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	var backend cache.Cache = &cache.Memory{}
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		r, err := cache.NewRedis(addr, os.Getenv("REDIS_PASSWORD"), envOr("PROJECT_SLUG", "open-enu-go")+":")
		if err != nil {
			return nil, err
		}
		backend = r
	}
	signer := &storage.Signer{Secret: crypto.DeriveKey([]byte(secret), secretInfoStorage), BaseURL: envOr("API_URL", "http://localhost:8080")}
	storageSigner = signer
	// The hub: Mercure when the stack names one, otherwise a log line per
	// update. An unsigned hub is refused here, at start, rather than
	// discovered when the first update is accepted from anyone.
	var hub event.Publisher = event.LogPublisher{Log: log}
	if u := os.Getenv("MERCURE_URL"); u != "" {
		// The publisher's key, not the subscriber's (MERCURE_JWT_SECRET, read
		// by realtime.Tokens): a subscriber key that leaks must not publish.
		mercureSecret := os.Getenv("MERCURE_PUBLISHER_JWT_SECRET")
		if len(mercureSecret) < 32 {
			return nil, fmt.Errorf("MERCURE_URL is set but MERCURE_PUBLISHER_JWT_SECRET is missing or shorter than 32 characters; `make env` generates it")
		}
		// The hub trusts the API's public URL as the issuer and its own
		// public URL as the audience (MERCURE_TRUSTED_ISSUERS and
		// `resource_identifier` in compose), although this request stays inside
		// the network.
		api := envOr("API_URL", "http://localhost:8080")
		hub = realtime.Hub{URL: u, Secret: []byte(mercureSecret), Issuer: api, Audience: api + realtime.CookiePath}
	}
	mailer, err := mail.FromDSN(os.Getenv("MAILER_DSN"), os.Getenv("MAILER_FROM"), os.Getenv("MAILER_REPLY_TO"), log)
	if err != nil {
		return nil, err
	}
	apiURL := envOr("API_URL", "http://localhost:8080")
	maxUploadMB, err := strconv.Atoi(envOr("STORAGE_MAX_UPLOAD_MB", "25"))
	if err != nil || maxUploadMB < 1 {
		return nil, fmt.Errorf("STORAGE_MAX_UPLOAD_MB must be a whole number of megabytes, at least 1 (got %q)", os.Getenv("STORAGE_MAX_UPLOAD_MB"))
	}
	svc := &platform.Services{
		Log:            log,
		I18n:           cat,
		Flags:          resolver,
		Cache:          &cache.Tenant{Backend: backend},
		Storage:        &storage.Local{Root: envOr("STORAGE_ROOT", "var/storage"), Signer: signer},
		MaxUploadBytes: int64(maxUploadMB) << 20,
		// Persisted and pushed: the Progress module reads the rows, the
		// browser follows the topic (kernel/progress.Postgres).
		Progress:        progress.Postgres{Store: progress.Store{Pool: pool}, Hub: hub, Log: log},
		Events:          &event.Broadcaster{Hub: hub, Log: log},
		Audit:           audit.Slog{Log: log},
		Actor:           auth.Actor{},
		Crypto:          crypto.NewEncryptor(keys),
		Signer:          auth.Signer{Secret: crypto.DeriveKey([]byte(secret), secretInfoTokens)},
		Passwords:       crypto.PasswordHasher{},
		Mail:            mailer,
		URLs:            platform.URLs{App: os.Getenv("APP_URL"), Manager: os.Getenv("MANAGER_URL"), API: apiURL},
		Permissions:     reg.Permissions(),
		FlagDefinitions: reg.Flags(),
		// The registry's, like Permissions: what every module declared
		// searchable and what every module may notify about.
		Searchable:              reg.Searchable(),
		NotificationDefinitions: reg.Notifications(),
		// Logged until a module stores them; the Notification module
		// replaces this below through Module.Notifier.
		Notifier: notification.Log{Log: log},
	}
	if err := platform.WithDatabase(svc, pool, outbox.Options{}, true); err != nil {
		return nil, err
	}
	// The notifier after the database half, for the same reason as Setup:
	// the Notification module's keeps the pool.
	if n, err := reg.Notifier(svc); err != nil {
		return nil, err
	} else if n != nil {
		svc.Notifier = n
	}
	// Built after the database half: a module's setup constructor keeps the
	// pool. Tenant's create handler runs it inside the new tenant's scope.
	svc.Setup = setup.NewRunner(log, reg.SetupSteps(svc)...)
	// Likewise the flag store: Settings keeps overrides in a table under the
	// pool, so the resolver is rebuilt over it once the pool exists.
	store, err := reg.FlagStore(svc)
	if err != nil {
		return nil, err
	}
	if store != nil {
		if svc.Flags, err = flags.NewResolver(store, reg.Flags()...); err != nil {
			return nil, err
		}
	}
	return svc, svc.Validate()
}
