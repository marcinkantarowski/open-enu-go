// Package tableowner keeps each module's tables its own.
//
// A table belongs to the module whose migrations create it. Another module
// reaches that data through the owner's contract/ - never by naming the
// table in SQL, and never with a foreign key to it - so the owner can change
// its schema, and tenant deletion, erasure and the audit trail stay where the
// owner put them (ADR-0004; extension-surfaces.md, *Reaching another
// module*).
//
// Every query is raw SQL, so this rule is what holds the boundary at the
// data layer. Two checks, from each module's own migrations/*.sql:
//
//   - a SQL string literal in a module's package that names a table another
//     module creates (FROM, JOIN, INTO, UPDATE, TABLE, TRUNCATE);
//   - a REFERENCES in a module's migration to a table another module creates,
//     reported on the module's root package.
//
// The kernel's tables (audit_entry, search_index, progress_job, River's) are
// created by no module, so this rule says nothing about them.
package tableowner

import (
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/tools/go/analysis"

	"github.com/open-enu/kernel/analysis/modpath"
)

var Analyzer = &analysis.Analyzer{
	Name: "tableowner",
	Doc:  "a module's SQL and migrations name only its own tables (and the kernel's); another module's data is reached through its contract/",
	Run:  run,
}

var (
	createTable = regexp.MustCompile(`(?i)\bcreate\s+table\s+(?:if\s+not\s+exists\s+)?([a-z_][a-z0-9_]*)`)
	usesTable   = regexp.MustCompile(`(?i)\b(?:from|join|into|update|table|truncate)\s+(?:only\s+)?([a-z_][a-z0-9_]*)`)
	references  = regexp.MustCompile(`(?i)\breferences\s+([a-z_][a-z0-9_]*)`)
)

// schema is what every module's migrations say, read once per module tree.
type schema struct {
	owner map[string]string      // table -> module
	refs  map[string][]reference // module -> its REFERENCES
	err   error
}

type reference struct {
	file, table string
	line        int
}

var (
	mu      sync.Mutex
	schemas = map[string]*schema{}
)

func run(pass *analysis.Pass) (any, error) {
	_, module, rest, ok := modpath.Module(pass.Pkg.Path())
	if !ok || len(pass.Files) == 0 {
		return nil, nil
	}
	dir := moduleTree(pass.Fset.File(pass.Files[0].Pos()).Name())
	if dir == "" {
		return nil, nil
	}
	s := load(dir)
	if s.err != nil {
		return nil, s.err
	}
	for _, f := range pass.Files {
		if strings.HasSuffix(pass.Fset.File(f.Pos()).Name(), "_test.go") {
			continue // a test may read any table to show what happened to it
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			text, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			for _, m := range usesTable.FindAllStringSubmatch(text, -1) {
				table := strings.ToLower(m[1])
				if o, owned := s.owner[table]; owned && o != module {
					pass.Reportf(lit.Pos(), "%s is the %s module's table - reach it through %s/contract, not SQL", table, o, o)
				}
			}
			return true
		})
	}
	if rest == "" { // the module's root package carries its migrations
		for _, r := range s.refs[module] {
			if o := s.owner[r.table]; o != "" && o != module {
				pass.Reportf(pass.Files[0].Package, "%s:%d: REFERENCES %s, the %s module's table - store the id and resolve it through %s/contract; no foreign key crosses a module", r.file, r.line, r.table, o, o)
			}
		}
	}
	return nil, nil
}

// moduleTree returns the directory that holds every module (…/internal/module)
// for a file inside one of them.
func moduleTree(file string) string {
	const marker = string(filepath.Separator) + "internal" + string(filepath.Separator) + "module" + string(filepath.Separator)
	i := strings.LastIndex(file, marker)
	if i < 0 {
		return ""
	}
	return file[:i+len(marker)-1]
}

func load(dir string) *schema {
	mu.Lock()
	defer mu.Unlock()
	if s, ok := schemas[dir]; ok {
		return s
	}
	s := &schema{owner: map[string]string{}, refs: map[string][]reference{}}
	schemas[dir] = s
	files, err := filepath.Glob(filepath.Join(dir, "*", "migrations", "*.sql"))
	if err != nil {
		s.err = err
		return s
	}
	sort.Strings(files)
	for _, f := range files {
		module := filepath.Base(filepath.Dir(filepath.Dir(f)))
		b, err := os.ReadFile(f)
		if err != nil {
			s.err = fmt.Errorf("tableowner: %w", err)
			return s
		}
		for i, line := range strings.Split(string(b), "\n") {
			if c := strings.Index(line, "--"); c >= 0 {
				line = line[:c]
			}
			for _, m := range createTable.FindAllStringSubmatch(line, -1) {
				s.owner[strings.ToLower(m[1])] = module
			}
			for _, m := range references.FindAllStringSubmatch(line, -1) {
				s.refs[module] = append(s.refs[module], reference{file: filepath.Join(module, "migrations", filepath.Base(f)), table: strings.ToLower(m[1]), line: i + 1})
			}
		}
	}
	return s
}
