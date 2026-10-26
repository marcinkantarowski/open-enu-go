// Package module is the module system: what a module declares, and the
// registry that validates and orders the declarations.
//
// A module is a Go package under backend/internal/module/<name> that exports
// exactly one thing the kernel looks at - `var Module module.Module`. The
// generated registry (cmd/openenu/module_registry_gen.go, written by
// tools/genregistry from the module.yaml files on disk) imports every module
// package and hands the values to NewRegistry. Nothing central is edited by
// hand: a directory with a manifest is a module, and a manifest the generator
// has not seen fails `make arch`.
package module

import (
	"fmt"
	"io/fs"
	"net/http"
	"sort"
	"strings"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/flags"
	"github.com/open-enu/kernel/gdpr"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/i18n"
	"github.com/open-enu/kernel/logctx"
	"github.com/open-enu/kernel/notification"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/search"
	"github.com/open-enu/kernel/setup"
)

// Module is the declaration. Every field is optional except Manifest; a
// module that only declares permissions is as valid as one that mounts
// twenty routes.
type Module struct {
	Manifest Manifest

	// Permissions this module defines, each prefixed `<name>.`: they are
	// global, so the registry refuses a string two modules both claim.
	// A permission ending in `.view` or `.use` is what a plain member holds;
	// anything else needs the admin or owner role (the Identity module's
	// resolver).
	Permissions []string

	// Mount registers the module's HTTP handlers on the server's mux, keeping
	// whatever it needs from the platform. Nil when the module has no API.
	// The generated strict server for the module's openapi.json fragment is
	// what normally lives behind this.
	Mount func(mux *http.ServeMux, svc *platform.Services)

	// Migrations is the module's migrations/ directory (goose SQL). Nil when
	// it owns no tables.
	Migrations fs.FS

	// Locales is the module's i18n/ directory: messages.<lang>.json. The
	// registry refuses a key present in one locale and absent from another.
	Locales fs.FS

	// OpenAPI is the module's spec fragment - its own paths and components,
	// merged into backend/openapi.json by tools/genspec.
	OpenAPI []byte

	// Flags this module reads, each prefixed `<name>.` like a permission. A
	// read of a key not declared here is refused by the flagdeclared
	// analyzer; a declared one can be seen and switched by an operator.
	Flags []flags.Definition

	// Setup runs when a tenant is created and when examples are seeded, in
	// dependency order. Nil when the module needs nothing in place.
	Setup func(svc *platform.Services) setup.TenantSetup

	// Gdpr answers export and erasure for a person. Nil when the module holds
	// nothing about one. Nothing checks that claim against the module's
	// tables: a module with a user_id column and a nil Gdpr is simply
	// skipped by the walker.
	Gdpr func(svc *platform.Services) gdpr.Subject

	// Tasks run on a clock in `openenu scheduler` (task.go). Empty for most
	// modules.
	Tasks []Task

	// Search declares the entity types this module indexes and the
	// permission each needs (search.go). The registry refuses a type not
	// prefixed with the module's name or gated by a permission the module
	// does not declare.
	Search []search.Declaration

	// Keys resolves an API key's hash to its principal for the auth
	// middleware. The ApiKey module provides it; the registry refuses two.
	Keys func(svc *platform.Services) auth.KeyResolver

	// Locale contributes a locale preference to the Locale middleware - a
	// person's stored choice (Identity), a tenant's default (Tenant) -
	// consulted before Accept-Language, a module's before those of the
	// modules it depends on.
	Locale func(svc *platform.Services) httpx.LocalePreference

	// FlagStore holds per-tenant flag overrides for the flags resolver. The
	// Settings module provides it; the registry refuses two. Without one,
	// every flag resolves to its declared default.
	FlagStore func(svc *platform.Services) flags.Store

	// Notifications this module raises (notification.go); each is prefixed
	// `<name>.` and its keys must be in this module's Locales. Notifier is
	// the one module that stores and delivers them - without it they are
	// logged (notification.Log).
	Notifications []notification.Definition
	Notifier      func(svc *platform.Services) notification.Notifier

	// LogFields add what this module knows to every log line logged with a
	// context, after the kernel's request, tenant, caller and command
	// (kernel/logctx). Each reads the context and nothing else: it runs on
	// every line.
	LogFields []logctx.Field
}

// MustSub returns the sub-tree of an embedded FS, for `//go:embed i18n` and
// `//go:embed migrations` in a module.go. The directory is spelled in the
// embed directive two lines above, so a mismatch is a build-time typo and a
// panic at init is the right response.
func MustSub(f fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(f, dir)
	if err != nil {
		panic("module: " + err.Error())
	}
	return sub
}

// Registry is the validated, dependency-ordered set of enabled modules.
type Registry struct {
	ordered       []Module
	byName        map[string]Module
	owner         map[string]string // permission -> module
	flags         map[string]string // flag identifier -> module
	search        map[string]string // entity type -> module
	notifications map[string]string // notification type -> module
}

// NewRegistry validates the declarations and orders them so that a module
// comes after everything it depends on - the order migrations run in and the
// order setup hooks fire.
//
// Every error says what to change. These run at process start and in
// `openenu modules`, so a broken declaration is found before a request is.
func NewRegistry(mods ...Module) (*Registry, error) {
	r := &Registry{byName: map[string]Module{}, owner: map[string]string{}, flags: map[string]string{}, search: map[string]string{}, notifications: map[string]string{}}

	for _, m := range mods {
		n := m.Manifest.Name
		if !nameShape.MatchString(n) {
			return nil, fmt.Errorf("module %q: name must match ^[a-z][a-z0-9_]*$", n)
		}
		if _, dup := r.byName[n]; dup {
			return nil, fmt.Errorf("module %q is declared twice - two directories cannot share a manifest name", n)
		}
		if !m.Manifest.Enabled {
			continue
		}
		r.byName[n] = m
	}

	// Dependencies: present, enabled, acyclic.
	for n, m := range r.byName {
		for _, d := range m.Manifest.Depends {
			if d == n {
				return nil, fmt.Errorf("module %q depends on itself", n)
			}
			if _, ok := r.byName[d]; !ok {
				return nil, fmt.Errorf("module %q depends on %q, which is not present or not enabled - add it, enable it, or drop the dependency", n, d)
			}
		}
	}
	ordered, err := topo(r.byName)
	if err != nil {
		return nil, err
	}
	r.ordered = ordered

	for _, m := range r.ordered {
		if err := r.declare(m); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// declare validates one module's permissions, flags and locales.
func (r *Registry) declare(m Module) error {
	n := m.Manifest.Name
	for _, p := range m.Permissions {
		if !strings.HasPrefix(p, n+".") || len(p) == len(n)+1 {
			return fmt.Errorf("module %q declares permission %q - a permission is prefixed with its owner: %q", n, p, n+".<action>")
		}
		if other, taken := r.owner[p]; taken {
			return fmt.Errorf("permission %q is declared by both %q and %q - a permission with two owners has no meaning", p, other, n)
		}
		r.owner[p] = n
	}
	for _, f := range m.Flags {
		if err := f.Validate(); err != nil {
			return fmt.Errorf("module %q: %w", n, err)
		}
		if !strings.HasPrefix(f.Identifier, n+".") {
			return fmt.Errorf("module %q declares flag %q - a flag is prefixed with its owner: %q", n, f.Identifier, n+".<feature>")
		}
		if other, taken := r.flags[f.Identifier]; taken {
			return fmt.Errorf("flag %q is declared by both %q and %q", f.Identifier, other, n)
		}
		r.flags[f.Identifier] = n
	}
	if m.Locales != nil {
		if _, err := i18n.CheckParity(m.Locales); err != nil {
			return fmt.Errorf("module %q: %w", n, err)
		}
	}
	if err := r.declareSearch(m); err != nil {
		return err
	}
	if err := r.declareNotifications(m); err != nil {
		return err
	}
	return r.declareTasks(m)
}

// All returns the enabled modules in dependency order.
func (r *Registry) All() []Module { return append([]Module(nil), r.ordered...) }

// Names returns the enabled module names in dependency order.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.ordered))
	for _, m := range r.ordered {
		out = append(out, m.Manifest.Name)
	}
	return out
}

// Get returns an enabled module by name.
func (r *Registry) Get(name string) (Module, bool) {
	m, ok := r.byName[name]
	return m, ok
}

// Permissions returns every declared permission, sorted.
func (r *Registry) Permissions() []string {
	out := make([]string, 0, len(r.owner))
	for p := range r.owner {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// OwnerOf returns the module that declared a permission.
func (r *Registry) OwnerOf(permission string) (string, bool) {
	o, ok := r.owner[permission]
	return o, ok
}

// Mount registers every module's handlers, in dependency order, handing
// each the platform.
func (r *Registry) Mount(mux *http.ServeMux, svc *platform.Services) {
	for _, m := range r.ordered {
		if m.Mount != nil {
			m.Mount(mux, svc)
		}
	}
}
