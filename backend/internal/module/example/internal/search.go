package internal

import "github.com/open-enu/kernel/search"

// entityType is the index's name for a project; a hit carries it back so
// the caller knows what to open.
const entityType = "example.project"

// Searchable declares what of this module is findable and who may find it.
// The permission is required, not optional: search returns an excerpt, an
// excerpt is content, and a hit the caller may not read is a disclosure. It
// is applied before the query runs, never to the results.
var Searchable = []search.Declaration{{EntityType: entityType, Permission: "example.view"}}

// indexed is the text a project is found by. `clientReference` is
// deliberately absent: it is encrypted at rest and the index stores
// plaintext, so indexing it would move the secret into a table with no
// access control of its own.
func indexed(p project) []string {
	desc := ""
	if p.Description != nil {
		desc = *p.Description
	}
	return []string{p.Name, desc}
}
