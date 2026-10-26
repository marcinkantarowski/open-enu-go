-- +goose Up
-- A tenant's value for a declared flag. There is no setting table: a flag is
-- declared in code and has no row until a tenant's value diverges from it.
CREATE TABLE setting_override (
    id         UUID         NOT NULL PRIMARY KEY,
    tenant_id  UUID         NOT NULL,
    identifier VARCHAR(120) NOT NULL,
    value      JSONB        NOT NULL,   -- boxed: {"value": ...}
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, identifier)
);

ALTER TABLE setting_override ENABLE ROW LEVEL SECURITY;
ALTER TABLE setting_override FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON setting_override
    USING      (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid OR current_setting('app.bypass', true) = 'on')
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid OR current_setting('app.bypass', true) = 'on');

-- +goose Down
DROP TABLE setting_override;
