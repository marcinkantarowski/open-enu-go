package scope

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type bypassKey struct{}

// Unscoped runs fn in a context whose transactions see every tenant's rows
// (`SET LOCAL app.bypass = 'on'`, which the policy accepts). The reason is a
// string literal, non-empty, and the rawsql analyzer refuses anything else
// outside a repository.go: `grep Unscoped` must be a complete review of every
// place isolation is lifted.
//
// Nothing of the request's tenant survives inside: a write made here must
// name its tenant explicitly, which is the point - the ambient scope is gone
// and the code has to say what it means.
func Unscoped(ctx context.Context, reason string, fn func(ctx context.Context) error) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("scope: Unscoped needs a reason - say why this read may cross tenants")
	}
	// The request's tenant is cleared, not only overridden by the bypass:
	// code inside that asked scope.TenantID would otherwise get the outer
	// tenant back and write it onto rows that belong to another.
	return fn(context.WithValue(context.WithValue(ctx, tenantKey{}, ""), bypassKey{}, reason))
}

func isBypass(ctx context.Context) bool {
	r, _ := ctx.Value(bypassKey{}).(string)
	return r != ""
}

// Policy returns the SQL that puts a table under row-level security: the
// policy from ADR-0009, enabled and FORCED so the table's owner is not
// exempt either. column is the tenant column; kind is its SQL type ("uuid"
// or "text"), so the comparison is on the column's own type and its index
// stays usable.
//
// Every migration that creates a tenant-scoped table ends with this. A
// table without it is readable by every tenant, and nothing else notices.
func Policy(table, column, kind string) string {
	setting := "current_setting('app.tenant_id', true)"
	if kind != "text" {
		setting = "NULLIF(" + setting + ", '')::" + kind
	}
	using := fmt.Sprintf("(%s = %s OR current_setting('app.bypass', true) = 'on')", column, setting)
	return fmt.Sprintf(`ALTER TABLE %[1]s ENABLE ROW LEVEL SECURITY;
ALTER TABLE %[1]s FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON %[1]s USING %[2]s WITH CHECK %[2]s;`, table, using)
}
