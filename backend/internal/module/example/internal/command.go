package internal

import "time"

// The writes, as data. Each is dispatched through the bus, which opens the
// scoped transaction, calls the handler in handler.go and records the audit
// entry whether the handler succeeded or not.

// createProject - `example.project.created`. No subject: the row does not
// exist when the entry is opened; the id arrives in the `after` snapshot.
type createProject struct {
	ID              string // "" = a new v7 uuid; fixtures and seeds name theirs
	Name            string
	Description     *string
	ClientReference *string
	Attributes      map[string]any
}

func (createProject) AuditAction() string    { return "example.project.created" }
func (createProject) AuditSubjectID() string { return "" }

// renameProject - `example.project.renamed`. Locked is whether the client
// sent If-Match; a stale IfMatch is a 409, not a silent last-write-wins.
type renameProject struct {
	ID      string
	Name    string
	IfMatch int
	Locked  bool
}

func (renameProject) AuditAction() string      { return "example.project.renamed" }
func (c renameProject) AuditSubjectID() string { return c.ID }

// archiveProjects - `example.projects.archive_requested`: the synchronous
// half, which starts the progress and enqueues the job. A bulk action has no
// single subject; `after` holds the job id, which ties the entry to the work.
type archiveProjects struct{}

func (archiveProjects) AuditAction() string    { return "example.projects.archive_requested" }
func (archiveProjects) AuditSubjectID() string { return "" }

// archiveActive - `example.projects.archived`: the job's write, one
// transaction for the whole batch so a failure halfway leaves no
// half-archived tenant.
type archiveActive struct{ JobID string }

func (archiveActive) AuditAction() string    { return "example.projects.archived" }
func (archiveActive) AuditSubjectID() string { return "" }

// sweepArchived - `example.projects.swept`: the clock's write (task.go), one
// tenant at a time, removing what was archived before `Before`. `before`
// holds the ids that left; there is no `after`.
type sweepArchived struct{ Before time.Time }

func (sweepArchived) AuditAction() string    { return "example.projects.swept" }
func (sweepArchived) AuditSubjectID() string { return "" }
