-- +goose Up
-- The feed: one row per recipient, storing the TYPE and its ARGUMENTS -
-- never a rendered sentence (ADR-0008).
CREATE TABLE notification (
    tenant_id  UUID         NOT NULL,
    id         UUID         NOT NULL PRIMARY KEY,
    user_id    UUID         NOT NULL,
    type       VARCHAR(120) NOT NULL,
    context    JSONB        NOT NULL DEFAULT '{}'::jsonb,
    read_at    TIMESTAMPTZ  DEFAULT NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX idx_notification_tenant ON notification (tenant_id);
CREATE INDEX idx_notification_user ON notification (user_id, created_at DESC);

ALTER TABLE notification ENABLE ROW LEVEL SECURITY;
ALTER TABLE notification FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON notification
    USING      (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid OR current_setting('app.bypass', true) = 'on')
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid OR current_setting('app.bypass', true) = 'on');

-- +goose Down
DROP TABLE notification;
