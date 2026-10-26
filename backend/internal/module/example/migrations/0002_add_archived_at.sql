-- +goose Up
-- `status` says a project was archived; retention (the daily sweep) needs
-- to know WHEN. Null while active.
ALTER TABLE example_project ADD COLUMN archived_at TIMESTAMPTZ DEFAULT NULL;

-- +goose Down
ALTER TABLE example_project DROP COLUMN archived_at;
