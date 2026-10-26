-- +goose Up
-- +goose StatementBegin
-- Full-text search (kernel/search): one tsvector per indexed record, GIN
-- indexed. Under the same RLS policy as every
-- scoped table, so a search from tenant A cannot return B's excerpt even if
-- the query forgot the predicate - the policy is the predicate.
CREATE TABLE search_index (
    tenant_id   varchar(64)  NOT NULL,
    entity_type varchar(128) NOT NULL,
    entity_id   varchar(64)  NOT NULL,
    content     text         NOT NULL,
    document    tsvector     NOT NULL,
    indexed_at  timestamptz  NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, entity_type, entity_id)
);
CREATE INDEX idx_search_document ON search_index USING GIN (document);
CREATE INDEX idx_search_tenant ON search_index (tenant_id, entity_type);
ALTER TABLE search_index ENABLE ROW LEVEL SECURITY;
ALTER TABLE search_index FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON search_index
    USING (tenant_id = current_setting('app.tenant_id', true) OR current_setting('app.bypass', true) = 'on')
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true) OR current_setting('app.bypass', true) = 'on');
-- +goose StatementEnd

-- +goose Down
DROP TABLE search_index;
