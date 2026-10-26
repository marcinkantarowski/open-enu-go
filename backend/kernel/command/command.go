// Package command is the write side's vocabulary: a state change as data.
//
// Every write goes through a command and its handler (ADR-0013). The
// interface exists so the bus, the audit trail and the analyzers can all
// recognise a write without anyone registering it anywhere. The bus (bus.go) opens the transaction, hands the handler a
// *scope.Tx, writes the audit entry whether the handler succeeded or not,
// and after the commit runs what the handler queued with tx.AfterCommit -
// cache invalidation, mail - so those side effects never outrun a rollback.
//
// A command is a plain struct: the intent, not the mechanism. Its handler
// does the work. Commands are NOT jobs: a command is synchronous inside a
// transaction because the caller needs to know whether it worked; a job is
// dispatched BY a handler once that transaction committed, for work the
// caller should not wait on.
package command

// Command is a state change, as data.
type Command interface {
	// AuditAction is stable, dotted, present tense: `billing.invoice.void`.
	// The audit trail is queried by it, so renaming one orphans history.
	AuditAction() string

	// AuditSubjectID is the record being changed, or "" when there is none
	// (a creation names the new id; a bulk operation names nothing).
	AuditSubjectID() string
}

// TenantTargeted is a command that acts ON a tenant from outside it.
//
// The audit entry normally takes its tenant from the ambient scope, which is
// right for ordinary traffic. An operator in the manager realm has no scope by
// design, so without this the most consequential entries in the system -
// suspensions, kill switches flipped during an incident - would be written
// with no tenant and vanish from "what happened to this customer?".
//
// Separate from Command because the answer only exists for commands whose
// subject is a tenant, and required to return a value for the same reason:
// an implementation that could answer "" puts the question back.
type TenantTargeted interface {
	Command
	AuditTenantID() string
}
