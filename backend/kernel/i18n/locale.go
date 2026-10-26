package i18n

import (
	"context"
	"embed"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

type localeKey struct{}

// WithLocale returns a context whose responses speak locale.
func WithLocale(ctx context.Context, locale string) context.Context {
	return context.WithValue(ctx, localeKey{}, locale)
}

// Locale returns the context's locale, or "" when none was resolved.
func Locale(ctx context.Context) string {
	l, _ := ctx.Value(localeKey{}).(string)
	return l
}

// Negotiate picks a locale from an Accept-Language header, narrowed to the
// locales that ship: honouring a requested locale that has no catalogue
// produces a page of raw keys, which is worse than confidently serving the
// default. Quality values order the candidates; `pl-PL` matches `pl`.
//
// This is the lowest-priority signal. A user's stored preference (Identity)
// and a tenant's default (Tenant) win over whatever the browser happens to
// send: they are the httpx.LocalePreference providers a module declares as
// Module.Locale, and the locale middleware consults them - a module's before
// those of the modules it depends on - before falling back to this.
func Negotiate(acceptLanguage string, enabled []string, def string) string {
	type cand struct {
		tag string
		q   float64
		pos int
	}
	var cands []cand
	for i, part := range strings.Split(acceptLanguage, ",") {
		tag, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" {
			continue
		}
		q := 1.0
		for _, p := range strings.Split(params, ";") {
			if k, v, ok := strings.Cut(strings.TrimSpace(p), "="); ok && strings.TrimSpace(k) == "q" {
				if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
					q = f
				}
			}
		}
		if q > 0 {
			cands = append(cands, cand{tag, q, i})
		}
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].q != cands[j].q {
			return cands[i].q > cands[j].q
		}
		return cands[i].pos < cands[j].pos
	})
	for _, c := range cands {
		for _, e := range enabled {
			if c.tag == e || strings.HasPrefix(c.tag, e+"-") || c.tag == "*" {
				return e
			}
		}
	}
	return def
}

//go:embed messages.*.json
var kernelLocales embed.FS

// KernelLocales is the kernel's own catalogue - the error envelope messages
// - loaded first, under the origin "kernel".
func KernelLocales() fs.FS { return kernelLocales }
