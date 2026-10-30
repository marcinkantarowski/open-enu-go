package i18n

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// Catalogue holds every message for every enabled locale: the kernel's own
// plus each module's i18n/messages.<locale>.json, loaded in registry order.
//
// Keys are flat and namespaced by module (`example.project.created`), the
// values are plain messages with `%name%` placeholders, substituted as they
// are - no plural or ICU syntax.
type Catalogue struct {
	def     string
	enabled []string
	msgs    map[string]map[string]string // locale -> key -> message
	owner   map[string]string            // key -> origin, for the duplicate error
}

// NewCatalogue takes the default locale and the enabled ones; the default
// must be enabled.
func NewCatalogue(def string, enabled ...string) (*Catalogue, error) {
	c := &Catalogue{def: def, enabled: enabled, msgs: map[string]map[string]string{}, owner: map[string]string{}}
	for _, l := range enabled {
		c.msgs[l] = map[string]string{}
	}
	if _, ok := c.msgs[def]; !ok {
		return nil, fmt.Errorf("i18n: default locale %q is not among the enabled locales %v", def, enabled)
	}
	return c, nil
}

// Default returns the default locale.
func (c *Catalogue) Default() string { return c.def }

// Enabled returns the enabled locales, in the order given.
func (c *Catalogue) Enabled() []string { return append([]string(nil), c.enabled...) }

// Load reads messages.<locale>.json for every enabled locale from fsys
// (nil is "no catalogue"), after checking that every locale has the same
// keys. A catalogue for a locale that is not enabled is a language this
// deployment does not offer, and is left out. A key already loaded from
// another origin is an error: two modules translating the same key is a
// namespace violation, not a merge.
func (c *Catalogue) Load(origin string, fsys fs.FS) error {
	if fsys == nil {
		return nil
	}
	files, err := CheckParity(fsys)
	if err != nil {
		return fmt.Errorf("%s: %w", origin, err)
	}
	for locale, msgs := range files {
		if _, enabled := c.msgs[locale]; !enabled {
			continue
		}
		for k, v := range msgs {
			if other, dup := c.owner[k]; dup && other != origin {
				return fmt.Errorf("%s: key %q is already defined by %s - keys are namespaced by module", origin, k, other)
			}
			c.owner[k] = origin
			c.msgs[locale][k] = v
		}
	}
	for _, l := range c.enabled {
		if _, has := files[l]; !has && len(files) > 0 {
			return fmt.Errorf("%s: messages.%s.json is missing - every enabled locale ships a catalogue", origin, l)
		}
	}
	return nil
}

// Has reports whether a key exists for a locale.
func (c *Catalogue) Has(locale string, key Key) bool {
	_, ok := c.msgs[locale][string(key)]
	return ok
}

// Translate resolves a key for a locale, falling back to the default locale
// and then to the key itself - a raw key in the output is visible, which is
// what the parity check exists to prevent in the first place. Placeholders
// are `%name%`, replaced from params.
func (c *Catalogue) Translate(locale string, key Key, params map[string]string) string {
	msg, ok := c.msgs[locale][string(key)]
	if !ok {
		msg, ok = c.msgs[c.def][string(key)]
	}
	if !ok {
		return string(key)
	}
	if len(params) == 0 {
		return msg
	}
	pairs := make([]string, 0, 2*len(params))
	for k, v := range params {
		pairs = append(pairs, "%"+k+"%", v)
	}
	return strings.NewReplacer(pairs...).Replace(msg)
}

// TranslateCtx resolves a key in the context's locale (WithLocale), or the
// default. Its signature is httpx.Translator's, so a server installs it with
// httpx.WithTranslator(ctx, catalogue.TranslateCtx).
func (c *Catalogue) TranslateCtx(ctx context.Context, key Key) string {
	return c.Translate(Locale(ctx), key, nil)
}

// CheckParity reads every messages.<locale>.json in fsys and returns them,
// or an error naming the first key present in one locale and absent from
// another. Values must be strings: a nested object is a key that was meant
// to be dotted.
func CheckParity(fsys fs.FS) (map[string]map[string]string, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("i18n: %w", err)
	}
	files := map[string]map[string]string{}
	var locales []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "messages.") || path.Ext(name) != ".json" {
			continue
		}
		locale := strings.TrimSuffix(strings.TrimPrefix(name, "messages."), ".json")
		msgs, err := readFlat(fsys, name)
		if err != nil {
			return nil, err
		}
		files[locale] = msgs
		locales = append(locales, locale)
	}
	sort.Strings(locales)
	for _, a := range locales {
		for k := range files[a] {
			for _, b := range locales {
				if _, ok := files[b][k]; !ok {
					return nil, fmt.Errorf("i18n: key %q exists in messages.%s.json and not in messages.%s.json - add it, or the %s reader sees a raw key", k, a, b, b)
				}
			}
		}
	}
	return files, nil
}

func readFlat(fsys fs.FS, name string) (map[string]string, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, fmt.Errorf("i18n: %w", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("i18n: %s: %w", name, err)
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("i18n: %s: key %q is not a string - catalogues are flat, dotted keys, never nested objects", name, k)
		}
		if !Key(k).Valid() {
			return nil, errors.New("i18n: " + name + ": key " + fmt.Sprintf("%q", k) + " is not dotted lowercase")
		}
		out[k] = s
	}
	return out, nil
}
