// Package contract is the part of example other modules may import: its
// one event. Everything else is internal/ and the compiler refuses it.
package contract

import "time"

// ProjectCreated is raised when a project is created, in the same
// transaction as the row (the outbox). It is a broadcast: every browser of
// the tenant holding `example.view` receives it on the tenant's topic.
type ProjectCreated struct {
	ID   string
	Name string
	At   time.Time
}

// EventName is the dotted name subscribers use.
func (ProjectCreated) EventName() string { return "example.project.created" }

// SubjectID is the project.
func (e ProjectCreated) SubjectID() string { return e.ID }

// OccurredAt is when.
func (e ProjectCreated) OccurredAt() time.Time { return e.At }

// Payload is deliberately not the whole record: the encrypted client
// reference and the attributes stay server-side.
func (e ProjectCreated) Payload() map[string]any { return map[string]any{"id": e.ID, "name": e.Name} }

// RequiresPermission makes it a broadcast, to holders of example.view.
func (ProjectCreated) RequiresPermission() string { return "example.view" }
