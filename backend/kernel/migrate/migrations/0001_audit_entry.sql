-- +goose Up
-- +goose StatementBegin
-- The audit trail (kernel/audit). Written by the kernel under bypass
-- (entries from the manager realm have no tenant); read per tenant by the
-- Audit module, and across tenants by operators through scope.Unscoped.
CREATE TABLE audit_entry (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    action          varchar(100) NOT NULL,
    tenant_id       varchar(64),
    actor_id        varchar(64),
    on_behalf_of_id varchar(64),
    subject_id      varchar(64),
    request_id      varchar(64),
    succeeded       boolean NOT NULL,
    failure_reason  text,
    before          jsonb,
    after           jsonb,
    context         jsonb NOT NULL DEFAULT '{}'::jsonb,
    recorded_at     timestamp(0) without time zone NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_tenant_time ON audit_entry (tenant_id, recorded_at);
CREATE INDEX idx_audit_action ON audit_entry (action);
CREATE INDEX idx_audit_actor ON audit_entry (actor_id);
CREATE INDEX idx_audit_subject ON audit_entry (subject_id);
ALTER TABLE audit_entry ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_entry FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON audit_entry
    USING (tenant_id = current_setting('app.tenant_id', true) OR current_setting('app.bypass', true) = 'on')
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true) OR current_setting('app.bypass', true) = 'on');
-- +goose StatementEnd

-- +goose Down
DROP TABLE audit_entry;
