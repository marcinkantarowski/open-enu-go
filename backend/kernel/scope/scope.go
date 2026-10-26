// Package scope is tenant isolation: which tenant a request acts for, and
// (tx.go) the transaction that carries it into Postgres as
// `SET LOCAL app.tenant_id`, where row-level security reads it (ADR-0009).
//
// This file is the context half - the part every other kernel package needs
// and none of them needs a database for. The cache prefixes keys with it, the
// storage prefixes paths with it, the broadcaster picks the topic from it,
// and the audit entry records it. Nothing here opens a connection.
//
// A context with no tenant is a context in which scoped reads return nothing:
// the RLS policy evaluates `current_setting('app.tenant_id', true)` to null
// and matches no row. Fail closed is the only acceptable default - "no tenant"
// must never mean "every tenant".
package scope

import "context"

type tenantKey struct{}

// WithTenant returns a context acting for one tenant. The auth middleware
// calls it from the principal's claims; a worker calls it from the job's
// stamp; a test calls it directly.
func WithTenant(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantKey{}, tenantID)
}

// TenantID returns the tenant the context acts for, and false when there is
// none - the manager realm, a system job, or a test that forgot.
func TenantID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(tenantKey{}).(string)
	return id, ok && id != ""
}

// TenantOrGlobal returns the tenant id, or "global" for an unscoped context.
// The word is the cache's and the storage's prefix for cross-tenant data and
// the key provider's salt for the global encryption key - one spelling, here.
func TenantOrGlobal(ctx context.Context) string {
	if id, ok := TenantID(ctx); ok {
		return id
	}
	return "global"
}
