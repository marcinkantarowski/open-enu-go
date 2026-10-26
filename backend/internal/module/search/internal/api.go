package internal

import (
	"context"
	"math"
	"strings"

	"github.com/open-enu/kernel/auth"
)

// declaration is what the controller keeps of a module's search.Declaration:
// the type and the permission that gates its excerpts. Built in mount.go.
type declaration struct{ entityType, permission string }

// api implements the StrictServerInterface genapi derives from
// ../openapi.json. The permission check happens BEFORE the query runs and
// per type, never on the results: a hit the caller may not read is a
// disclosure, and filtering afterwards would still have fetched it.
type api struct {
	repo  repository
	types []declaration
}

var _ StrictServerInterface = api{}

func (a api) GetSearch(ctx context.Context, req GetSearchRequestObject) (GetSearchResponseObject, error) {
	p, err := auth.RequireRealm(ctx, auth.RealmTenant)
	if err != nil {
		return nil, err
	}
	query := ""
	if req.Params.Q != nil {
		query = strings.TrimSpace(*req.Params.Q)
	}
	out := SearchResult{Items: []SearchHit{}, Meta: SearchMeta{Query: query}}
	// An empty query is not an error and not "everything": returning every
	// record for a stray keystroke is how a search box dumps the database.
	if query == "" {
		return GetSearch200JSONResponse(out), nil
	}
	var visible []string
	for _, d := range a.types {
		if p.Has(d.permission) {
			visible = append(visible, d.entityType)
		}
	}
	if len(visible) == 0 {
		return GetSearch200JSONResponse(out), nil
	}
	hits, err := a.repo.search(ctx, query, visible)
	if err != nil {
		return nil, err
	}
	for _, h := range hits {
		out.Items = append(out.Items, SearchHit{Type: h.EntityType, Id: h.EntityID, Score: float32(math.Round(float64(h.Score)*10000) / 10000), Excerpt: h.Excerpt})
	}
	out.Meta.Total = len(out.Items)
	return GetSearch200JSONResponse(out), nil
}
