// Package search is full-text search on Postgres, with no extra service to
// run: one search_index table (kernel migration 0002) holds a tsvector per
// indexed record under a GIN index. Adequate well past the point where a
// project can afford a search cluster, and the interface means swapping in
// something else later is one type.
//
// The table is under the same RLS policy as every scoped table. Index and
// Remove name the tenant explicitly - a job re-indexing after an import has
// no request scope - and the policy's WITH CHECK refuses a row whose tenant
// disagrees with the transaction's scope, so an explicit tenant can never
// smuggle a record across. Search reads under the request's scope: no tenant,
// no hits.
package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/open-enu/kernel/scope"
)

// Hit is one result. Excerpt is a ts_headline of the indexed text - content,
// which is why every searchable entity type needs a permission: the
// Declaration the registry checks and the Search module's endpoint consults.
type Hit struct {
	EntityType string
	EntityID   string
	Score      float32
	Excerpt    string
}

// Declaration is what a module publishes about an entity type it indexes:
// the type's name and the permission that gates its excerpts. The module
// system collects them (module.Module.Search) and the search endpoint
// consults them before it answers - an excerpt is content, so a type with
// no permission is a type nobody may search.
type Declaration struct {
	EntityType string
	Permission string
}

// Indexer writes and reads the index. Config is the text search
// configuration ("simple" by default: no stemming, so it works for every
// locale at once).
type Indexer struct {
	Config string
}

func (x Indexer) config() string {
	if x.Config == "" {
		return "simple"
	}
	return x.Config
}

// Index upserts the record's text. Empty text removes the record: a row that
// lost its indexed text must not linger as a hit that matches nothing.
func (x Indexer) Index(ctx context.Context, tx *scope.Tx, entityType, entityID string, fields []string, tenantID string) error {
	var parts []string
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			parts = append(parts, f)
		}
	}
	text := strings.Join(parts, " ")
	if text == "" {
		return x.Remove(ctx, tx, entityType, entityID, tenantID)
	}
	_, err := tx.Exec(ctx, `INSERT INTO search_index (tenant_id, entity_type, entity_id, content, document, indexed_at)
		VALUES ($1, $2, $3, $4, to_tsvector($5::regconfig, $4), now())
		ON CONFLICT (tenant_id, entity_type, entity_id) DO UPDATE
		SET content = EXCLUDED.content, document = EXCLUDED.document, indexed_at = EXCLUDED.indexed_at`,
		tenantID, entityType, entityID, text, x.config())
	if err != nil {
		return fmt.Errorf("search: index: %w", err)
	}
	return nil
}

// Remove deletes the record from the index.
func (x Indexer) Remove(ctx context.Context, tx *scope.Tx, entityType, entityID, tenantID string) error {
	if _, err := tx.Exec(ctx, "DELETE FROM search_index WHERE tenant_id = $1 AND entity_type = $2 AND entity_id = $3", tenantID, entityType, entityID); err != nil {
		return fmt.Errorf("search: remove: %w", err)
	}
	return nil
}

// Search runs a websearch-syntax query ("quoted phrase", -excluded, or) over
// the transaction's scope, optionally limited to entity types.
func (x Indexer) Search(ctx context.Context, tx *scope.Tx, query string, entityTypes []string, limit int) ([]Hit, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	sql := `SELECT entity_type, entity_id,
			ts_rank(document, websearch_to_tsquery($1::regconfig, $2)) AS score,
			ts_headline($1::regconfig, content, websearch_to_tsquery($1::regconfig, $2), 'MaxWords=24, MinWords=8') AS excerpt
		FROM search_index
		WHERE document @@ websearch_to_tsquery($1::regconfig, $2)`
	args := []any{x.config(), query}
	if len(entityTypes) > 0 {
		sql += " AND entity_type = ANY($3)"
		args = append(args, entityTypes)
	}
	sql += fmt.Sprintf(" ORDER BY score DESC, entity_id LIMIT %d", limit)
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer rows.Close()
	var hits []Hit
	for rows.Next() {
		var h Hit
		if err := rows.Scan(&h.EntityType, &h.EntityID, &h.Score, &h.Excerpt); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}
