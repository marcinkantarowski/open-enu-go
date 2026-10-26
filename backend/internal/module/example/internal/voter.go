package internal

// canEdit is the record-level check: the permission says what this ROLE may
// do, this says whether THIS row may be touched now. Nothing about the
// tenant here - the row-level policy already covers that for every scoped
// table, and repeating it would mean two places to fix when the rule
// changes.
func canEdit(p project) bool { return !p.archived() }
