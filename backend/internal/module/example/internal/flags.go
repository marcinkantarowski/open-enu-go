package internal

import "github.com/open-enu/kernel/flags"

// Flags declares the module's switches. Declared, so an operator can see it
// and turn it off; read with the same literal, so flagdeclared can match
// the two.
var Flags = []flags.Definition{{
	Identifier:  "example.archive",
	Name:        "Bulk archive",
	Description: "Allows archiving every active project at once. Off by default - it is the module's demonstration of a kill switch.",
	// Off, so the route 404s until somebody decides otherwise. A capability
	// that arrives switched on has been decided for you.
	Default:        false,
	TenantEditable: true,
	Category:       "example",
}}
