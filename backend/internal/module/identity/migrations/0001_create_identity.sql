-- +goose Up
-- Identity's four tables: users, memberships, refresh tokens and security
-- tokens. None is under row-level security: a person spans tenants and a
-- login has no tenant yet. Rows are reached by user id, memberships by
-- (user, tenant).
CREATE TABLE identity_user (
    id            UUID         NOT NULL PRIMARY KEY,
    email         TEXT         NOT NULL,            -- encrypted under the global key
    email_hash    VARCHAR(64)  NOT NULL UNIQUE,     -- SHA-256 of the normalised address: the login's lookup
    password      TEXT         NOT NULL,            -- the verifier: argon2id, PHC form (ADR-0022)
    display_name  TEXT,                             -- encrypted
    status        VARCHAR(16)  NOT NULL DEFAULT 'unverified',
    locale        VARCHAR(5),
    avatar_key    VARCHAR(255),
    version       INT          NOT NULL DEFAULT 1,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    last_login_at TIMESTAMPTZ
);

CREATE TABLE identity_membership (
    id         UUID        NOT NULL PRIMARY KEY,
    user_id    UUID        NOT NULL REFERENCES identity_user (id) ON DELETE CASCADE,
    tenant_id  UUID        NOT NULL,                -- no FK: cross-module associations are a build failure
    role       VARCHAR(16) NOT NULL,
    status     VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, tenant_id)
);
CREATE INDEX idx_identity_membership_tenant ON identity_membership (tenant_id);

CREATE TABLE identity_refresh_token (
    id         UUID        NOT NULL PRIMARY KEY,
    token_hash VARCHAR(64) NOT NULL UNIQUE,
    user_id    UUID        NOT NULL REFERENCES identity_user (id) ON DELETE CASCADE,
    family     UUID        NOT NULL,
    tenant_id  UUID,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    revoked    BOOLEAN     NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_identity_refresh_user ON identity_refresh_token (user_id);
CREATE INDEX idx_identity_refresh_family ON identity_refresh_token (family);

CREATE TABLE identity_security_token (
    id          UUID        NOT NULL PRIMARY KEY,
    token_hash  VARCHAR(64) NOT NULL UNIQUE,
    purpose     VARCHAR(32) NOT NULL,
    user_id     UUID        REFERENCES identity_user (id) ON DELETE CASCADE,
    tenant_id   UUID,
    payload     JSONB       NOT NULL DEFAULT '{}'::jsonb,
    expires_at  TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE identity_security_token;
DROP TABLE identity_refresh_token;
DROP TABLE identity_membership;
DROP TABLE identity_user;
