# ADR-0017 - Search is an interface with one Postgres implementation

**Status:** accepted · **Date:** 2026-10-22

## Context
A search abstraction over Meilisearch, Elasticsearch, a vector database or several of them
offers range, and every option is another service to run, configure, back up and keep in
step with the database.

## Decision
`search.Indexer` has one implementation: a `search_index` table holding a `tsvector` per
indexed record under a GIN index (kernel migration 0002), under the same row-level security
as every tenant table. A module declares what it indexes on `Module.Search` - the fields and
the **permission** that may see each entity type, because a result carries an excerpt and an
excerpt is content. The index is written in the same transaction as the record. The Search
module's `GET /api/search` checks the permission per type before it queries.

The database image is `pgvector/pgvector` and `pg_trgm` is installed, so a vector or
trigram driver is a type and a migration if one is ever needed - not an infrastructure
change.

## Consequences
- Full-text search with no extra service, adequate well past the point where a project can
  afford a dedicated search cluster.
- No fuzzy matching or typo tolerance out of the box.
- Swapping in a search service later means implementing one interface.
- An encrypted column is never indexed: the index stores plaintext (ADR-0016).
