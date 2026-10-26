package module

import (
	"fmt"
	"sort"
	"strings"
)

// topo orders by dependency, deterministically (ties broken by name), and
// reports a cycle by naming it.
func topo(byName map[string]Module) ([]Module, error) {
	names := make([]string, 0, len(byName))
	for n := range byName {
		names = append(names, n)
	}
	sort.Strings(names)

	const (
		white = iota
		grey
		black
	)
	state := map[string]int{}
	var out []Module
	var visit func(n string, path []string) error
	visit = func(n string, path []string) error {
		switch state[n] {
		case black:
			return nil
		case grey:
			return fmt.Errorf("module dependency cycle: %s -> %s", strings.Join(path, " -> "), n)
		}
		state[n] = grey
		deps := append([]string(nil), byName[n].Manifest.Depends...)
		sort.Strings(deps)
		for _, d := range deps {
			if err := visit(d, append(path, n)); err != nil {
				return err
			}
		}
		state[n] = black
		out = append(out, byName[n])
		return nil
	}
	for _, n := range names {
		if err := visit(n, nil); err != nil {
			return nil, err
		}
	}
	return out, nil
}
