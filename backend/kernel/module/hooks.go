package module

import (
	"fmt"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/flags"
	"github.com/open-enu/kernel/gdpr"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/i18n"
	"github.com/open-enu/kernel/logctx"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/setup"
)

// Flags returns every declared flag, in module order - what a Resolver is
// built from and what the operator console lists.
func (r *Registry) Flags() []flags.Definition {
	var out []flags.Definition
	for _, m := range r.ordered {
		out = append(out, m.Flags...)
	}
	return out
}

// FlagOwner returns the module that declared a flag.
func (r *Registry) FlagOwner(identifier string) (string, bool) {
	o, ok := r.flags[identifier]
	return o, ok
}

// SetupSteps returns the modules' tenant setup hooks in dependency order,
// ready for setup.NewRunner.
func (r *Registry) SetupSteps(svc *platform.Services) []setup.Step {
	var out []setup.Step
	for _, m := range r.ordered {
		if m.Setup != nil {
			out = append(out, setup.Step{Module: m.Manifest.Name, Setup: m.Setup(svc)})
		}
	}
	return out
}

// GdprSubjects returns the modules' GDPR subjects in dependency order,
// ready for gdpr.NewWalker (which erases in reverse).
func (r *Registry) GdprSubjects(svc *platform.Services) []gdpr.Registered {
	var out []gdpr.Registered
	for _, m := range r.ordered {
		if m.Gdpr != nil {
			out = append(out, gdpr.Registered{Module: m.Manifest.Name, Subject: m.Gdpr(svc)})
		}
	}
	return out
}

// LoadLocales loads the kernel's catalogue and then every module's into
// cat, in dependency order. A key two modules both define is an error here
// even though parity passed per module.
func (r *Registry) LoadLocales(cat *i18n.Catalogue) error {
	if err := cat.Load("kernel", i18n.KernelLocales()); err != nil {
		return err
	}
	for _, m := range r.ordered {
		if err := cat.Load("module "+m.Manifest.Name, m.Locales); err != nil {
			return err
		}
	}
	return nil
}

// Keys returns the one module's key resolver, or auth.NoKeys when no module
// provides one. Two providers is a configuration error, named.
func (r *Registry) Keys(svc *platform.Services) (auth.KeyResolver, error) {
	var out auth.KeyResolver = auth.NoKeys{}
	owner := ""
	for _, m := range r.ordered {
		if m.Keys == nil {
			continue
		}
		if owner != "" {
			return nil, fmt.Errorf("modules %q and %q both resolve API keys - exactly one may", owner, m.Manifest.Name)
		}
		owner, out = m.Manifest.Name, m.Keys(svc)
	}
	return out, nil
}

// FlagStore returns the one module's override store, or nil when no module
// provides one.
func (r *Registry) FlagStore(svc *platform.Services) (flags.Store, error) {
	var out flags.Store
	owner := ""
	for _, m := range r.ordered {
		if m.FlagStore == nil {
			continue
		}
		if owner != "" {
			return nil, fmt.Errorf("modules %q and %q both store flag overrides - exactly one may", owner, m.Manifest.Name)
		}
		owner, out = m.Manifest.Name, m.FlagStore(svc)
	}
	return out, nil
}

// LogFields is every module's log fields, in module order, for the
// process's logger (logctx.Handler) after the kernel's own.
func (r *Registry) LogFields() []logctx.Field {
	var out []logctx.Field
	for _, m := range r.ordered {
		out = append(out, m.LogFields...)
	}
	return out
}

// LocalePreferences returns every module's preference ready for
// httpx.Locale, a module's before those of the modules it depends on: the
// dependent knows more about the request, so a person's stored choice
// (Identity, which depends on Tenant) answers before the workspace's
// default - which is always set, and would otherwise answer every time.
func (r *Registry) LocalePreferences(svc *platform.Services) []httpx.LocalePreference {
	var out []httpx.LocalePreference
	for i := len(r.ordered) - 1; i >= 0; i-- {
		if m := r.ordered[i]; m.Locale != nil {
			out = append(out, m.Locale(svc))
		}
	}
	return out
}
