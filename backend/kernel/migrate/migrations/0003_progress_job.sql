-- +goose Up
-- +goose StatementBegin
-- Persisted progress (kernel/progress.Postgres). Written by the kernel's
-- reporter from any job, read per tenant by the Progress module. The id is
-- the 26-character id progress.NewID mints.
CREATE TABLE progress_job (
    id             varchar(32)  PRIMARY KEY,
    tenant_id      uuid         NOT NULL,
    kind           varchar(64)  NOT NULL,
    label          varchar(200),
    total          integer      NOT NULL DEFAULT 0,
    done           integer      NOT NULL DEFAULT 0,
    status         varchar(16)  NOT NULL DEFAULT 'running',
    result         jsonb        NOT NULL DEFAULT '{}'::jsonb,
    failure_reason text,
    started_at     timestamptz  NOT NULL DEFAULT now(),
    finished_at    timestamptz
);
CREATE INDEX idx_progress_job_tenant ON progress_job (tenant_id, started_at);
ALTER TABLE progress_job ENABLE ROW LEVEL SECURITY;
ALTER TABLE progress_job FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON progress_job
    USING      (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid OR current_setting('app.bypass', true) = 'on')
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid OR current_setting('app.bypass', true) = 'on');
-- +goose StatementEnd

-- +goose Down
DROP TABLE progress_job;
