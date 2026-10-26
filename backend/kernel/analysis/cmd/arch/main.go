// Command arch runs every architecture guardrail over a package pattern:
//
//	cd backend && go run github.com/open-enu/kernel/analysis/cmd/arch ./...
//
// It is a go/analysis multichecker, so it accepts the same flags as go vet
// and exits non-zero on any finding. `make arch` and `make check` run it;
// `make selftest` plants one violation per analyzer and expects it to fail.
// A rule missing from this list is a rule that is not enforced.
package main

import (
	"golang.org/x/tools/go/analysis/multichecker"

	"github.com/open-enu/kernel/analysis/flagdeclared"
	"github.com/open-enu/kernel/analysis/platformservice"
	"github.com/open-enu/kernel/analysis/rawsql"
	"github.com/open-enu/kernel/analysis/rawstring"
	"github.com/open-enu/kernel/analysis/tableowner"
	"github.com/open-enu/kernel/analysis/textlen"
	"github.com/open-enu/kernel/analysis/thincontroller"
	"github.com/open-enu/kernel/analysis/writeboundary"
)

func main() {
	multichecker.Main(
		thincontroller.Analyzer,
		rawsql.Analyzer,
		platformservice.Analyzer,
		rawstring.Analyzer,
		flagdeclared.Analyzer,
		writeboundary.Analyzer,
		textlen.Analyzer,
		tableowner.Analyzer,
	)
}
