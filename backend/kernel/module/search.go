package module

import (
	"fmt"
	"strings"

	"github.com/open-enu/kernel/search"
)

// Searchable returns every module's search declarations, in module order:
// what the search endpoint may answer for, and with which permission.
func (r *Registry) Searchable() []search.Declaration {
	var out []search.Declaration
	for _, m := range r.ordered {
		out = append(out, m.Search...)
	}
	return out
}

// SearchOwner returns the module that declared an entity type.
func (r *Registry) SearchOwner(entityType string) (string, bool) {
	o, ok := r.search[entityType]
	return o, ok
}

// declareSearch runs after the module's permissions are declared, so the
// permission check is against this module's own.
func (r *Registry) declareSearch(m Module) error {
	n := m.Manifest.Name
	for _, d := range m.Search {
		if !strings.HasPrefix(d.EntityType, n+".") || len(d.EntityType) == len(n)+1 {
			return fmt.Errorf("module %q declares searchable type %q - a type is prefixed with its owner: %q", n, d.EntityType, n+".<entity>")
		}
		if owner, ok := r.owner[d.Permission]; !ok || owner != n {
			return fmt.Errorf("module %q gates searchable type %q with %q, which it does not declare - an excerpt is content, and only the module that owns the data can say who may read it", n, d.EntityType, d.Permission)
		}
		if other, taken := r.search[d.EntityType]; taken {
			return fmt.Errorf("searchable type %q is declared by both %q and %q", d.EntityType, other, n)
		}
		r.search[d.EntityType] = n
	}
	return nil
}
