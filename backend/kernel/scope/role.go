package scope

import (
	"context"
	"errors"
	"fmt"
)

// ErrPrivilegedRole is what VerifyApplicationRole returns for a pool that
// connects as a role Postgres exempts from row-level security.
var ErrPrivilegedRole = errors.New("scope: the application connects as a role that bypasses row-level security")

// VerifyApplicationRole refuses a pool whose role is SUPERUSER or BYPASSRLS.
// Either one makes every policy decoration - every tenant reads every other
// tenant's rows - and nothing else in the code would notice. The process
// calls it once, right after Open, on the DSN it serves requests with;
// `openenu migrate` connects as the owner on purpose and never calls it.
func VerifyApplicationRole(ctx context.Context, p *Pool) error {
	var role string
	var super, bypass bool
	if err := p.QueryRow(ctx, "SELECT current_user, rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user").Scan(&role, &super, &bypass); err != nil {
		return fmt.Errorf("scope: cannot read the application role: %w", err)
	}
	if super || bypass {
		return fmt.Errorf("%w: role %q (superuser=%v, bypassrls=%v); DATABASE_URL must name the NOSUPERUSER NOBYPASSRLS application role, DATABASE_ADMIN_URL the owner",
			ErrPrivilegedRole, role, super, bypass)
	}
	return nil
}
