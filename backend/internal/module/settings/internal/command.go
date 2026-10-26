package internal

// setTenantFlag - `settings.flag.set`: one tenant's override of one declared
// flag, written in that tenant's scope (the caller enters it), audited
// against that tenant even when the caller is an operator outside any.
type setTenantFlag struct {
	TenantID   string
	Identifier string
	Value      any
}

func (setTenantFlag) AuditAction() string      { return "settings.flag.set" }
func (c setTenantFlag) AuditSubjectID() string { return c.Identifier }
func (c setTenantFlag) AuditTenantID() string  { return c.TenantID }
