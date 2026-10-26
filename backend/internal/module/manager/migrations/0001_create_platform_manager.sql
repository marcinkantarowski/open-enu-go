-- +goose Up
-- Platform operators, in a table of their own, separate from users.
-- No tenant and no policy - an operator belongs to the platform.
CREATE TABLE platform_manager (
    id            UUID        NOT NULL PRIMARY KEY,
    email         TEXT        NOT NULL,            -- encrypted
    email_hash    VARCHAR(64) NOT NULL UNIQUE,     -- the lookup
    password      TEXT        NOT NULL,            -- the verifier
    display_name  TEXT,                            -- encrypted
    roles         JSONB       NOT NULL DEFAULT '["ROLE_PLATFORM_MANAGER"]'::jsonb,
    active        BOOLEAN     NOT NULL DEFAULT true,
    version       INTEGER     NOT NULL DEFAULT 1,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at TIMESTAMPTZ
);

-- +goose Down
DROP TABLE platform_manager;
