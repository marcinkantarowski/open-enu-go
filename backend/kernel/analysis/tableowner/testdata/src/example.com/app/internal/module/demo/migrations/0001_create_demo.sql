-- +goose Up
CREATE TABLE demo_thing (
    id         UUID PRIMARY KEY,
    parent_id  UUID REFERENCES demo_thing (id),     -- its own table: fine
    account_id UUID REFERENCES other_account (id)   -- another module's: refused
);
-- a comment naming REFERENCES other_account is not a reference
