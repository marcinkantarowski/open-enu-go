package internal

// suspendTenant - `manager.tenant.suspended`: the operator's decision, in
// the trail against that tenant, before Tenant's own command does the work.
type suspendTenant struct{ ID string }

func (suspendTenant) AuditAction() string      { return "manager.tenant.suspended" }
func (c suspendTenant) AuditSubjectID() string { return c.ID }
func (c suspendTenant) AuditTenantID() string  { return c.ID }

// startImpersonation - `manager.impersonation.started`. Dispatched BEFORE
// the token exists: if minting fails, the attempt is still on record -
// "who tried?" is most of what a trail is asked after an incident.
type startImpersonation struct {
	TenantID   string
	UserID     string
	OperatorID string
	// Super is whether the operator holds roleSuper, read from their row
	// (not their token) by the service. Only a super operator may cross.
	Super bool
}

func (startImpersonation) AuditAction() string      { return "manager.impersonation.started" }
func (c startImpersonation) AuditSubjectID() string { return c.UserID }
func (c startImpersonation) AuditTenantID() string  { return c.TenantID }

// assignConsultant - `manager.consultant.assigned`: the operator's decision, on
// the trail against that tenant, before Identity's own command does the work.
// Super is read as for startImpersonation: this opens a client's data to a person.
type assignConsultant struct {
	TenantID string
	Email    string
	Super    bool
}

func (assignConsultant) AuditAction() string      { return "manager.consultant.assigned" }
func (c assignConsultant) AuditSubjectID() string { return c.TenantID }
func (c assignConsultant) AuditTenantID() string  { return c.TenantID }

// unassignConsultant - `manager.consultant.unassigned`.
type unassignConsultant struct {
	TenantID string
	UserID   string
	Super    bool
}

func (unassignConsultant) AuditAction() string      { return "manager.consultant.unassigned" }
func (c unassignConsultant) AuditSubjectID() string { return c.UserID }
func (c unassignConsultant) AuditTenantID() string  { return c.TenantID }
