package internal

import (
	"sort"
	"strings"
)

// ownerOnly are the permissions an admin never holds: deleting the
// workspace and minting machine credentials for it.
var ownerOnly = []string{"tenant.delete", "api_key.manage"}

// resolver turns a role into permissions, from what every module declared.
// An undeclared permission is refused rather than granted: a typo in a
// controller then fails closed, loudly, instead of opening an endpoint.
type resolver struct {
	declared map[string]bool
	sorted   []string
}

func newResolver(declared []string) resolver {
	r := resolver{declared: map[string]bool{}}
	for _, p := range declared {
		if !r.declared[p] {
			r.declared[p] = true
			r.sorted = append(r.sorted, p)
		}
	}
	sort.Strings(r.sorted)
	return r
}

// allows is the rule: owner everything but `.review`, admin everything but
// ownerOnly, a plain member what reads (`.view`) and the everyday work a module
// names `.use`. Anything else that changes state needs a role someone granted.
// A `.review` is a check by someone other than the workspace's owner, so the
// owner never holds one: checking your own work is no check.
func (r resolver) allows(role, permission string) bool {
	if !r.declared[permission] {
		return false
	}
	switch role {
	case roleOwner:
		return !strings.HasSuffix(permission, ".review")
	case roleAdmin:
		for _, p := range ownerOnly {
			if p == permission {
				return false
			}
		}
		return true
	case roleMember:
		return strings.HasSuffix(permission, ".view") || strings.HasSuffix(permission, ".use")
	}
	return false
}

// forRole is the sorted list a token carries.
func (r resolver) forRole(role string) []string {
	out := []string{}
	for _, p := range r.sorted {
		if r.allows(role, p) {
			out = append(out, p)
		}
	}
	return out
}
