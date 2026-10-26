package flags

import (
	"context"
	"fmt"
	"sync"

	"github.com/open-enu/kernel/scope"
)

// Store holds per-tenant overrides. The Settings module's table implements
// it (Module.FlagStore; the registry refuses two); MemoryStore does in
// tests. Without a store every flag resolves to its declared default.
type Store interface {
	// Override returns the tenant's value for a flag, and false when the
	// tenant has none.
	Override(ctx context.Context, tenantID, identifier string) (any, bool, error)
}

// Resolver resolves declared flags: the tenant's override when the context
// has a tenant and the store has one, else the declaration's default. An
// undeclared identifier resolves to the call's default - the analyzer is
// what stops that from being silent.
type Resolver struct {
	defs  map[string]Definition
	store Store
}

// NewResolver validates the definitions and refuses a duplicate.
func NewResolver(store Store, defs ...Definition) (*Resolver, error) {
	r := &Resolver{defs: map[string]Definition{}, store: store}
	for _, d := range defs {
		if err := d.Validate(); err != nil {
			return nil, err
		}
		if _, dup := r.defs[d.Identifier]; dup {
			return nil, fmt.Errorf("flag %q is declared twice", d.Identifier)
		}
		r.defs[d.Identifier] = d
	}
	return r, nil
}

// Definitions returns every declared flag, for the operator console and
// `make flags`.
func (r *Resolver) Definitions() []Definition {
	out := make([]Definition, 0, len(r.defs))
	for _, d := range r.defs {
		out = append(out, d)
	}
	return out
}

// value is the raw resolved value and whether the flag is declared.
func (r *Resolver) value(ctx context.Context, id string) (any, bool) {
	d, ok := r.defs[id]
	if !ok {
		return nil, false
	}
	if tenant, has := scope.TenantID(ctx); has && r.store != nil {
		if v, found, err := r.store.Override(ctx, tenant, id); err == nil && found {
			return v, true
		}
	}
	return d.Default, true
}

// Enabled resolves a bool.
func (r *Resolver) Enabled(ctx context.Context, id string, def bool) bool {
	v, ok := r.value(ctx, id)
	if !ok {
		return def
	}
	return asBool(v, def)
}

// String resolves a string.
func (r *Resolver) String(ctx context.Context, id string, def string) string {
	v, ok := r.value(ctx, id)
	if !ok {
		return def
	}
	return asString(v, def)
}

// Int resolves an int.
func (r *Resolver) Int(ctx context.Context, id string, def int) int {
	v, ok := r.value(ctx, id)
	if !ok {
		return def
	}
	return asInt(v, def)
}

// JSON resolves an object.
func (r *Resolver) JSON(ctx context.Context, id string, def map[string]any) map[string]any {
	v, ok := r.value(ctx, id)
	if !ok {
		return def
	}
	return asJSON(v, def)
}

// MemoryStore is a Store in memory.
type MemoryStore struct {
	mu sync.RWMutex
	m  map[string]map[string]any // tenant -> identifier -> value
}

// Set writes an override.
func (s *MemoryStore) Set(tenantID, identifier string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]map[string]any{}
	}
	if s.m[tenantID] == nil {
		s.m[tenantID] = map[string]any{}
	}
	s.m[tenantID][identifier] = value
}

// Unset removes an override.
func (s *MemoryStore) Unset(tenantID, identifier string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t := s.m[tenantID]; t != nil {
		delete(t, identifier)
	}
}

// Override implements Store.
func (s *MemoryStore) Override(_ context.Context, tenantID, identifier string) (any, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.m[tenantID][identifier]
	return v, ok, nil
}
