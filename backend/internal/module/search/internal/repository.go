package internal

import (
	"context"

	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
	ksearch "github.com/open-enu/kernel/search"
)

// maxResults is a screen's worth. A search box is not an export.
const maxResults = 20

// repository runs the one read, under the request's scope: no tenant, no
// hits - the index table is under the same policy as every scoped table.
type repository struct {
	db    *scope.Pool
	index ksearch.Indexer
}

func newRepository(svc *platform.Services) repository {
	return repository{db: svc.DB, index: svc.Search}
}

func (r repository) search(ctx context.Context, query string, types []string) (hits []ksearch.Hit, err error) {
	err = r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		hits, err = r.index.Search(ctx, tx, query, types, maxResults)
		return err
	})
	return hits, err
}
