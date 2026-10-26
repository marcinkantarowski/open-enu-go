// Package platform is the set of kernel services a module is handed when it
// mounts: the one value that replaces a container's autowiring.
//
// A module's Mount receives *Services and keeps what it needs. Nothing here
// is optional at runtime - Validate refuses a nil field - and every field is
// an interface or a kernel type with an in-memory implementation, so a
// module test gets a complete platform from InMemory and tests its rules
// against the real constraints, not a mock (the lesson
// test-contracts-against-the-real-implementation).
//
// The database half - the pool, the command bus, the outbox, the search
// indexer - is attached by WithDatabase, so a module test that needs no
// table runs with InMemory alone and one that does adds testdb.Postgres.
package platform

import (
	"fmt"
	"log/slog"

	"github.com/open-enu/kernel/audit"
	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/cache"
	"github.com/open-enu/kernel/command"
	"github.com/open-enu/kernel/crypto"
	"github.com/open-enu/kernel/event"
	"github.com/open-enu/kernel/flags"
	"github.com/open-enu/kernel/i18n"
	"github.com/open-enu/kernel/mail"
	"github.com/open-enu/kernel/notification"
	"github.com/open-enu/kernel/outbox"
	"github.com/open-enu/kernel/progress"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/search"
	"github.com/open-enu/kernel/setup"
	"github.com/open-enu/kernel/storage"
)

// Services is what every module can rely on.
type Services struct {
	Log     *slog.Logger
	I18n    *i18n.Catalogue
	Flags   flags.Flags
	Cache   *cache.Tenant
	Storage storage.Storage
	// MaxUploadBytes is the largest file the process accepts in one
	// request (STORAGE_MAX_UPLOAD_MB); 0 leaves each module's default.
	MaxUploadBytes int64
	Progress       progress.Reporter
	Events         *event.Broadcaster
	Audit          audit.Logger
	Actor          audit.ActorProvider
	Crypto         *crypto.Encryptor

	// The database half, attached by WithDatabase.
	DB     *scope.Pool    // reads, in repository.go only (writeboundary)
	Bus    *command.Bus   // the one way a write happens
	Outbox *outbox.Outbox // events and jobs, inside the handler's transaction
	Search search.Indexer // the tsvector index

	// Issuing credentials and reaching people. The kernel verifies a token
	// (kernel/auth); Identity issues one with the same Signer.
	Signer    auth.Signer
	Passwords crypto.PasswordHasher
	Mail      mail.Mailer
	// URLs the process is reached at, for links in mail and for whether a
	// cookie may be Secure. Derived from DOMAIN; never hard-coded.
	URLs URLs
	// Setup runs every module's tenant hooks in dependency order; the
	// process builds it from the registry (module.Registry.SetupSteps).
	// Nil in a module test, where no other module is mounted.
	Setup *setup.Runner
	// FlagDefinitions is every flag every module declared, for the Settings
	// module's catalogue and the operator console. Set by the process from
	// the registry, like Permissions.
	FlagDefinitions []flags.Definition
	// Searchable is every entity type every module declared searchable, with
	// the permission that gates it; the Search module's endpoint consults
	// it before answering. The registry's.
	Searchable []search.Declaration
	// Notifier tells a person or a workspace that something happened; the
	// Notification module provides the one that stores and mails, the
	// process starts with notification.Log. NotificationDefinitions is every
	// type every module declared, for the catalogue that translates rows.
	Notifier                notification.Notifier
	NotificationDefinitions []notification.Definition

	// Permissions is every permission every module declares - what a role
	// resolves to and what an API key may be granted. The registry's.
	Permissions []string
}

// URLs is where the three apps and the API answer. Secure reports whether
// the API is served over TLS - the condition for a Secure cookie, which a
// browser refuses to send back over plain http, and only a test uses plain
// http.
type URLs struct {
	App, Manager, API string
}

// Secure is true when the API is reached over https.
func (u URLs) Secure() bool { return len(u.API) >= 8 && u.API[:8] == "https://" }

// Validate refuses a half-built platform: a nil service surfaces as a panic
// on the first request that needs it, which is the wrong time.
func (s *Services) Validate() error {
	missing := func(name string) error { return fmt.Errorf("platform: %s is not configured", name) }
	switch {
	case s.Log == nil:
		return missing("Log")
	case s.I18n == nil:
		return missing("I18n")
	case s.Flags == nil:
		return missing("Flags")
	case s.Cache == nil || s.Cache.Backend == nil:
		return missing("Cache")
	case s.Storage == nil:
		return missing("Storage")
	case s.Progress == nil:
		return missing("Progress")
	case s.Events == nil || s.Events.Hub == nil:
		return missing("Events")
	case s.Audit == nil:
		return missing("Audit")
	case s.Actor == nil:
		return missing("Actor")
	case s.Crypto == nil:
		return missing("Crypto")
	case len(s.Signer.Secret) < 32:
		return missing("Signer (a secret of at least 32 bytes)")
	case s.Mail == nil:
		return missing("Mail")
	case s.Notifier == nil:
		return missing("Notifier")
	case s.DB == nil:
		return missing("DB (platform.WithDatabase)")
	case s.Bus == nil:
		return missing("Bus (platform.WithDatabase)")
	case s.Outbox == nil:
		return missing("Outbox (platform.WithDatabase)")
	}
	return nil
}

// TestKey is the master key InMemory derives from. Tests only.
const TestKey = "test-key-test-key-test-key-test-key-32"

// InMemory is a complete platform with no backing service: the memory cache
// and storage, recorders for progress, events and audit, static flags, the
// kernel's catalogue in en and pl, and an encryptor under TestKey. Fields
// are plain values a test can reach into - the Recorder behind Events.Hub,
// the Recorder behind Audit. Flags is a Static map with no overrides; a test
// that needs them sets Flags to a Resolver over a MemoryStore.
func InMemory(log *slog.Logger) *Services {
	if log == nil {
		log = slog.Default()
	}
	cat, err := i18n.NewCatalogue("en", "en", "pl")
	if err != nil {
		panic(err)
	}
	if err := cat.Load("kernel", i18n.KernelLocales()); err != nil {
		panic(err)
	}
	keys, err := crypto.NewDerivedKeys(TestKey)
	if err != nil {
		panic(err)
	}
	signer := &storage.Signer{Secret: []byte(TestKey), BaseURL: "http://test"}
	return &Services{
		Log:      log,
		I18n:     cat,
		Flags:    flags.Static{},
		Cache:    &cache.Tenant{Backend: &cache.Memory{}},
		Storage:  &storage.Memory{Signer: signer},
		Progress: &progress.Recorder{},
		Events:   &event.Broadcaster{Hub: &event.Recorder{}, Log: log},
		Audit:    &audit.Recorder{},
		Actor:    audit.NoActor{},
		Notifier: &notification.Recorder{},
		Crypto:   crypto.NewEncryptor(keys),
		Signer:   auth.Signer{Secret: []byte(TestKey)},
		// argon2's floor: the format and the rules are what a test checks;
		// the cost is production's (crypto.DefaultPasswordCost).
		Passwords: crypto.PasswordHasher{Cost: crypto.PasswordCost{MemoryKiB: 8, Time: 1, Threads: 1}},
		Mail:      &mail.Recorder{},
		URLs:      URLs{App: "http://app.test", Manager: "http://manager.test", API: "http://test"},
	}
}
