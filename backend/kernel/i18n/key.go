// Package i18n owns translation: the Key type every user-facing string in
// the kernel and the modules must be, so that the rawstring analyzer has a
// type to key on; the Catalogue that loads each module's messages.<lang>.json
// and translates a Key for a locale, refusing a key present in one locale and
// absent from another (CheckParity); and Negotiate, the Accept-Language
// fallback the httpx locale middleware consults last.
package i18n

import (
	"fmt"
	"regexp"
)

// Key names a message in a module's catalogue (i18n/messages.<lang>.json).
// It is never shown to a user as-is - a translator resolves it - which is
// why it is a distinct type and not a string: a function that takes a Key
// cannot be handed a sentence by accident.
//
// Shape: dotted lowercase segments, the first being the owning module
// (`example.project.created`) or `kernel`. Enforced by Valid and by the
// rawstring analyzer at build time.
type Key string

var keyShape = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

// Valid reports whether k has the dotted-lowercase shape.
func (k Key) Valid() bool { return keyShape.MatchString(string(k)) }

// MustKey panics on a malformed key. For package-level declarations, where a
// typo should fail at init rather than render as "example.projct.created".
func MustKey(s string) Key {
	k := Key(s)
	if !k.Valid() {
		panic(fmt.Sprintf("i18n: %q is not a translation key (want dotted lowercase, e.g. example.project.created)", s))
	}
	return k
}
