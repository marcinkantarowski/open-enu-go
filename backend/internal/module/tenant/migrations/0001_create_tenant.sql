-- +goose Up
-- The workspaces. Deliberately NOT under row-level security: this
-- is the row a scope names, read before any scope exists (login) and by
-- operators across all of them.
CREATE TABLE tenant (
    id             UUID         NOT NULL PRIMARY KEY,
    slug           VARCHAR(64)  NOT NULL UNIQUE,
    name           VARCHAR(200) NOT NULL,
    status         VARCHAR(16)  NOT NULL DEFAULT 'pending',
    plan           VARCHAR(40)  NOT NULL DEFAULT 'free',
    default_locale VARCHAR(5)   NOT NULL DEFAULT 'en',
    version        INT          NOT NULL DEFAULT 1,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    activated_at   TIMESTAMPTZ
);
CREATE INDEX idx_tenant_status_created ON tenant (status, created_at);

-- +goose Down
DROP TABLE tenant;
