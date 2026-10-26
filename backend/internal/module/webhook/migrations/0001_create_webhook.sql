-- +goose Up
-- A tenant's webhook endpoints and the log of their deliveries, as two
-- scoped tables.
CREATE TABLE webhook_endpoint (
    tenant_id  UUID         NOT NULL,
    id         UUID         NOT NULL PRIMARY KEY,
    url        VARCHAR(500) NOT NULL,
    events     JSONB        NOT NULL DEFAULT '[]'::jsonb,
    secret     TEXT         NOT NULL,            -- encrypted under the tenant's key; shown once at creation
    active     BOOLEAN      NOT NULL DEFAULT true,
    version    INT          NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX idx_webhook_endpoint_tenant ON webhook_endpoint (tenant_id);

CREATE TABLE webhook_delivery (
    tenant_id       UUID         NOT NULL,
    id              UUID         NOT NULL PRIMARY KEY,
    endpoint_id     UUID         NOT NULL,
    event_name      VARCHAR(120) NOT NULL,
    payload         JSONB        NOT NULL,
    status          VARCHAR(16)  NOT NULL DEFAULT 'pending',
    attempts        INT          NOT NULL DEFAULT 0,
    response_code   INT          DEFAULT NULL,
    response_body   TEXT         DEFAULT NULL,   -- truncated to 2000 characters
    last_attempt_at TIMESTAMPTZ  DEFAULT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX idx_webhook_delivery_tenant ON webhook_delivery (tenant_id, created_at DESC);
CREATE INDEX idx_webhook_delivery_endpoint ON webhook_delivery (endpoint_id);

ALTER TABLE webhook_endpoint ENABLE ROW LEVEL SECURITY;
ALTER TABLE webhook_endpoint FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON webhook_endpoint
    USING      (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid OR current_setting('app.bypass', true) = 'on')
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid OR current_setting('app.bypass', true) = 'on');
ALTER TABLE webhook_delivery ENABLE ROW LEVEL SECURITY;
ALTER TABLE webhook_delivery FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON webhook_delivery
    USING      (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid OR current_setting('app.bypass', true) = 'on')
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid OR current_setting('app.bypass', true) = 'on');

-- +goose Down
DROP TABLE webhook_delivery;
DROP TABLE webhook_endpoint;
