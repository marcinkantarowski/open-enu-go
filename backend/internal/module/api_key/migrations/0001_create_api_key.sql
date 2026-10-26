-- +goose Up
-- Machine credentials: one tenant each, a subset of permissions, and only
-- the secret's hash.
CREATE TABLE api_key (
    id           UUID         NOT NULL PRIMARY KEY,
    tenant_id    UUID         NOT NULL,
    name         VARCHAR(100) NOT NULL,
    key_hash     VARCHAR(64)  NOT NULL UNIQUE,   -- SHA-256 of the secret; the secret is never stored
    key_prefix   VARCHAR(16)  NOT NULL,
    permissions  JSONB        NOT NULL DEFAULT '[]'::jsonb,
    revoked      BOOLEAN      NOT NULL DEFAULT false,
    expires_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX idx_api_key_tenant ON api_key (tenant_id);

ALTER TABLE api_key ENABLE ROW LEVEL SECURITY;
ALTER TABLE api_key FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON api_key
    USING      (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid OR current_setting('app.bypass', true) = 'on')
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid OR current_setting('app.bypass', true) = 'on');

-- +goose Down
DROP TABLE api_key;
