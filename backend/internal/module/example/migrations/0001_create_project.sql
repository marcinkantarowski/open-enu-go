-- +goose Up
-- The reference module's one entity. Each column beyond the name shows one
-- platform concern: an encrypted reference, open-ended JSONB attributes, a
-- version for optimistic locking.
CREATE TABLE example_project (
    tenant_id        UUID         NOT NULL,
    id               UUID         NOT NULL PRIMARY KEY,
    name             VARCHAR(200) NOT NULL,
    client_reference TEXT         DEFAULT NULL,  -- encrypted: v1:iv:ct:tag under the tenant's key
    description      TEXT         DEFAULT NULL,
    attributes       JSONB        NOT NULL DEFAULT '{}'::jsonb,
    status           VARCHAR(16)  NOT NULL DEFAULT 'active',
    version          INT          NOT NULL DEFAULT 1,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX idx_example_project_tenant ON example_project (tenant_id);

-- Isolation is the policy's, not the query's (ADR-0009). Every tenant-scoped
-- table ends its migration with these three statements - scope.Policy
-- writes exactly this text; it is inlined here because goose reads SQL, not Go.
ALTER TABLE example_project ENABLE ROW LEVEL SECURITY;
ALTER TABLE example_project FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON example_project
    USING      (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid OR current_setting('app.bypass', true) = 'on')
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid OR current_setting('app.bypass', true) = 'on');

-- +goose Down
DROP TABLE example_project;
