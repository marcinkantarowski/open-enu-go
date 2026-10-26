package inventory

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// typeNames lists the types one file declares, in order of name. A missing
// file is a module without that concern, not an error.
func typeNames(path string) ([]string, error) {
	f, err := parseFile(path)
	if err != nil || f == nil {
		return nil, err
	}
	var out []string
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, s := range gd.Specs {
			out = append(out, s.(*ast.TypeSpec).Name.Name)
		}
	}
	sort.Strings(out)
	return out, nil
}

// exportedNames lists what a contract/ directory exports: types, functions,
// variables and constants - the surface other modules may import.
func exportedNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parseFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && d.Name.IsExported() {
					out = append(out, d.Name.Name+"()")
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						if s.Name.IsExported() {
							out = append(out, s.Name.Name)
						}
					case *ast.ValueSpec:
						for _, n := range s.Names {
							if n.IsExported() {
								out = append(out, n.Name)
							}
						}
					}
				}
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

func parseFile(path string) (*ast.File, error) {
	src, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return parser.ParseFile(token.NewFileSet(), path, src, parser.SkipObjectResolution)
}

var createTable = regexp.MustCompile(`(?i)CREATE TABLE (?:IF NOT EXISTS )?"?([a-z_][a-z0-9_]*)"?`)

// tables lists what the module's migrations create. Read as text: a
// migration is SQL, and the names are what a schema change has to find.
func tables(migrations fs.FS) ([]string, error) {
	if migrations == nil {
		return nil, nil
	}
	seen := map[string]bool{}
	err := fs.WalkDir(migrations, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".sql") {
			return err
		}
		data, err := fs.ReadFile(migrations, path)
		if err != nil {
			return err
		}
		for _, m := range createTable.FindAllSubmatch(data, -1) {
			seen[strings.ToLower(string(m[1]))] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	var out []string
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out, nil
}
