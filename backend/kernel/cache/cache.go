// Package cache is the only cache modules are allowed to use.
//
// Two problems it exists to make impossible:
//
//  1. Cross-tenant cache poisoning. A key like `invoice_totals` computed for
//     tenant A and served to tenant B is a data leak no query scoping
//     prevents, because the query never runs. Every key here is prefixed
//     with the tenant from the context, so the collision cannot be written.
//  2. Invalidation nobody can perform. Deleting a tenant, or changing
//     something half a dozen modules derive from, is one call rather than a
//     hunt for every key that might be stale: every entry is tagged
//     `tenant.<id>` and `module.<name>`.
//
// Cache is the backend contract - Memory (memory.go) in tests and when no
// REDIS_ADDR is set, Redis (redis.go) in the stack. Tenant
// is what a module holds. A raw Redis client in a module is refused by the
// platformservice analyzer.
package cache

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/open-enu/kernel/scope"
)

// Cache is a byte store with TTL and tags. Keys and tags arrive already
// namespaced; a backend stores them as given.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration, tags []string) error
	Delete(ctx context.Context, key string) error
	InvalidateTags(ctx context.Context, tags ...string) error
}

// Tenant scopes a Cache to the tenant in the context.
type Tenant struct {
	Backend Cache
}

// Get returns the cached bytes for (tenant, module, key), computing and
// storing them on a miss. ttl 0 means no expiry. extraTags are sanitised
// and added beside the module and tenant tags.
func (t *Tenant) Get(ctx context.Context, module, key string, ttl time.Duration, compute func(context.Context) ([]byte, error), extraTags ...string) ([]byte, error) {
	k := t.key(ctx, module, key)
	if v, ok, err := t.Backend.Get(ctx, k); err != nil {
		return nil, err
	} else if ok {
		return v, nil
	}
	v, err := compute(ctx)
	if err != nil {
		return nil, err
	}
	tags := []string{ModuleTag(module)}
	if id, ok := scope.TenantID(ctx); ok {
		tags = append(tags, TenantTag(id))
	}
	for _, x := range extraTags {
		tags = append(tags, SanitiseTag(x))
	}
	return v, t.Backend.Set(ctx, k, v, ttl, tags)
}

// GetJSON is Get for a typed value, JSON-encoded in the cache.
func GetJSON[T any](ctx context.Context, t *Tenant, module, key string, ttl time.Duration, compute func(context.Context) (T, error), extraTags ...string) (T, error) {
	var zero T
	raw, err := t.Get(ctx, module, key, ttl, func(ctx context.Context) ([]byte, error) {
		v, err := compute(ctx)
		if err != nil {
			return nil, err
		}
		return json.Marshal(v)
	}, extraTags...)
	if err != nil {
		return zero, err
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return zero, err
	}
	return out, nil
}

// Delete drops one entry for the current tenant.
func (t *Tenant) Delete(ctx context.Context, module, key string) error {
	return t.Backend.Delete(ctx, t.key(ctx, module, key))
}

// InvalidateModule drops everything a module cached, for every tenant.
func (t *Tenant) InvalidateModule(ctx context.Context, module string) error {
	return t.Backend.InvalidateTags(ctx, ModuleTag(module))
}

// InvalidateTenant drops everything cached for one tenant. Called when a
// tenant is deleted.
func (t *Tenant) InvalidateTenant(ctx context.Context, tenantID string) error {
	return t.Backend.InvalidateTags(ctx, TenantTag(tenantID))
}

// ModuleTag is `module.<name>`.
func ModuleTag(module string) string { return "module." + SanitiseTag(module) }

// TenantTag is `tenant.<id>`.
func TenantTag(tenantID string) string { return "tenant." + SanitiseTag(tenantID) }

var reservedInTag = regexp.MustCompile(`[{}()/\\@:]`)
var notKeyChar = regexp.MustCompile(`[^A-Za-z0-9_.]`)

// SanitiseTag replaces the characters `{}()/\@:` with a dot, so a tag built
// from a tenant id or a caller's string is always one plain segment of the
// key it becomes.
func SanitiseTag(tag string) string { return reservedInTag.ReplaceAllString(tag, ".") }

// key is `<tenant|global>.<module>.<key>`: namespaced by tenant even though
// also tagged by it, because tags drive invalidation and the prefix prevents
// the collision in the first place.
func (t *Tenant) key(ctx context.Context, module, key string) string {
	return strings.Join([]string{scope.TenantOrGlobal(ctx), module, notKeyChar.ReplaceAllString(key, "_")}, ".")
}
