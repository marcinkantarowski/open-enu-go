package httpx

import (
	"net/http"

	"github.com/open-enu/kernel/i18n"
)

// LocalePreference contributes a locale for a request, or "" to defer to
// the next. An extension surface: the kernel knows only Accept-Language; the
// user's stored preference (Identity) and the tenant's default (Tenant) are
// added by the modules that know them, in that priority, never by a module
// reading the header itself.
type LocalePreference func(r *http.Request) string

// Locale resolves the response language - the first preference that names
// an enabled locale, then Accept-Language intersected with what ships, then
// the catalogue's default - and installs it on the context together with
// the catalogue as the error envelope's translator. Content-Language says
// what was chosen.
func Locale(cat *i18n.Catalogue, prefer ...LocalePreference) Middleware {
	enabled := cat.Enabled()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			locale := ""
			for _, p := range prefer {
				if l := p(r); l != "" && isEnabled(l, enabled) {
					locale = l
					break
				}
			}
			if locale == "" {
				locale = i18n.Negotiate(r.Header.Get("Accept-Language"), enabled, cat.Default())
			}
			w.Header().Set("Content-Language", locale)
			ctx := i18n.WithLocale(r.Context(), locale)
			ctx = WithTranslator(ctx, cat.TranslateCtx)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func isEnabled(locale string, enabled []string) bool {
	for _, e := range enabled {
		if e == locale {
			return true
		}
	}
	return false
}
