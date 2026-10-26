-- +goose Up
-- Uploaded files: what they are and what they belong to. The bytes are in
-- storage, under storage_key.
CREATE TABLE attachment (
    id           UUID         NOT NULL PRIMARY KEY,
    tenant_id    UUID         NOT NULL,
    storage_key  VARCHAR(255) NOT NULL,
    filename     VARCHAR(255) NOT NULL,
    content_type VARCHAR(127) NOT NULL,
    size         BIGINT       NOT NULL,
    owner_type   VARCHAR(64),
    owner_id     VARCHAR(64),
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX idx_attachment_owner ON attachment (tenant_id, owner_type, owner_id);
CREATE INDEX idx_attachment_unclaimed ON attachment (created_at) WHERE owner_id IS NULL;

ALTER TABLE attachment ENABLE ROW LEVEL SECURITY;
ALTER TABLE attachment FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON attachment
    USING      (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid OR current_setting('app.bypass', true) = 'on')
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid OR current_setting('app.bypass', true) = 'on');

-- +goose Down
DROP TABLE attachment;
