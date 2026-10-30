package internal

import "testing"

func TestAReviewIsNeverTheOwners(t *testing.T) {
	r := newResolver([]string{"x.view", "x.use", "x.manage", "x.review"})
	for _, c := range []struct {
		role, permission string
		want             bool
	}{
		{roleOwner, "x.manage", true},
		{roleOwner, "x.review", false},
		{roleAdmin, "x.review", true},
		{roleMember, "x.use", true},
		{roleMember, "x.review", false},
		{roleOwner, "x.undeclared", false},
	} {
		if got := r.allows(c.role, c.permission); got != c.want {
			t.Errorf("%s holding %s: %t, want %t", c.role, c.permission, got, c.want)
		}
	}
}
