// Package gdpr asks every module what it holds about a person, and tells
// every module to forget them.
//
// Export and erasure must reach every module or the answer is wrong, and a
// module that forgets to implement Subject is invisible: nothing in the build
// checks that a module whose tables carry a user_id sets Module.Gdpr. The
// registry collects whatever is declared (module.Registry.GdprSubjects) and
// the walker reaches exactly that, so coverage is a review item, not a
// guarantee - `grep 'Gdpr:' internal/module/*/module.go` is the review.
//
// Erasure runs in REVERSE dependency order - the mirror of setup. A module
// that depends on another may hold references to its rows, so it must let
// go first. Both operations are audited, because "we deleted your data" is a
// claim that has to be evidenced later.
package gdpr

import (
	"context"
	"fmt"
	"time"

	"github.com/open-enu/kernel/audit"
)

// Subject is implemented by a module holding data about a person and
// registered on its Module value.
type Subject interface {
	// ExportFor returns everything held about the person, as plain data:
	// label -> rows.
	ExportFor(ctx context.Context, userID string) (map[string][]map[string]any, error)
	// EraseFor removes or anonymises it and returns the rows affected. Which
	// of the two is the module's call: an audit trail is anonymised because
	// deleting it destroys a record required for other reasons; a draft is
	// deleted. What matters is that the person is no longer identifiable.
	EraseFor(ctx context.Context, userID string) (int, error)
}

// Registered is one module's subject, named.
type Registered struct {
	Module  string
	Subject Subject
}

// Walker runs export and erasure across the registered subjects.
type Walker struct {
	subjects []Registered // dependency order
	audit    audit.Logger
	now      func() time.Time
}

// NewWalker takes the subjects in dependency order (the registry's).
func NewWalker(log audit.Logger, subjects ...Registered) *Walker {
	return &Walker{subjects: subjects, audit: log, now: time.Now}
}

// Export collects every section, keyed `<module>.<label>`. A label two
// modules both use is namespaced by the module; a collision that survives
// that gets a `#n` suffix rather than overwriting - losing a section of a
// subject-access response is not an acceptable failure mode.
func (w *Walker) Export(ctx context.Context, userID string) (map[string][]map[string]any, error) {
	bundle := map[string][]map[string]any{}
	for _, s := range w.subjects {
		sections, err := s.Subject.ExportFor(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("gdpr export: module %q: %w", s.Module, err)
		}
		for label, rows := range sections {
			key := s.Module + "." + label
			unique := key
			for i := 2; ; i++ {
				if _, taken := bundle[unique]; !taken {
					break
				}
				unique = fmt.Sprintf("%s#%d", key, i)
			}
			bundle[unique] = rows
		}
	}
	w.audit.Record(ctx, audit.Entry{
		Action: "gdpr.subject.exported", SubjectID: userID, Succeeded: true,
		Context: map[string]any{"sections": len(bundle)}, At: w.now(),
	})
	return bundle, nil
}

// Erase runs every subject in reverse order and returns module -> rows
// affected. The first failure stops the run: the modules not yet reached
// still hold the data, and the error says which one refused.
func (w *Walker) Erase(ctx context.Context, userID string) (map[string]int, error) {
	affected := map[string]int{}
	total := 0
	for i := len(w.subjects) - 1; i >= 0; i-- {
		s := w.subjects[i]
		n, err := s.Subject.EraseFor(ctx, userID)
		if err != nil {
			return affected, fmt.Errorf("gdpr erase: module %q: %w", s.Module, err)
		}
		affected[s.Module] = n
		total += n
	}
	w.audit.Record(ctx, audit.Entry{
		Action: "gdpr.subject.erased", SubjectID: userID, Succeeded: true,
		Context: map[string]any{"rows": total}, At: w.now(),
	})
	return affected, nil
}
