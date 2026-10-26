package internal

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/open-enu/app/internal/module/tenant/contract"
)

// PurgeAfterDays is how long a pending tenant waits for its owner to
// verify before the daily task removes it.
const PurgeAfterDays = 7

// tenant is contract.Info with the audit snapshot the trail keeps.
type tenant struct{ contract.Info }

func (t tenant) Snapshot() map[string]any {
	return map[string]any{
		"id": t.ID, "slug": t.Slug, "name": t.Name, "status": t.Status, "plan": t.Plan,
		"defaultLocale": t.DefaultLocale, "version": t.Version, "createdAt": t.CreatedAt.Format(time.RFC3339),
	}
}

// newTenant is a tenant as creation leaves it: pending, free plan, version 1.
func newTenant(c contract.Create, defaultLocale string) tenant {
	id := c.ID
	if id == "" {
		id = uuid.Must(uuid.NewV7()).String()
	}
	slug := c.Slug
	if slug == "" {
		slug = Slugify(c.Name)
	}
	locale := c.DefaultLocale
	if locale == "" {
		locale = defaultLocale
	}
	return tenant{contract.Info{
		ID: id, Slug: slug, Name: strings.TrimSpace(c.Name), Status: contract.StatusPending, Plan: "free",
		DefaultLocale: locale, Version: 1, CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}}
}

var (
	slugShape = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	notSlug   = regexp.MustCompile(`[^a-z0-9]+`)
)

// Slugify lowercases, turns each run of anything else into one hyphen,
// trims hyphens from the ends and cuts to 64 characters. "" when nothing is
// left.
func Slugify(name string) string {
	s := strings.Trim(notSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(s) > 64 {
		s = strings.TrimRight(s[:64], "-")
	}
	return s
}

// ValidSlug is what a slug a caller named must look like.
func ValidSlug(s string) bool { return len(s) <= 64 && slugShape.MatchString(s) }
