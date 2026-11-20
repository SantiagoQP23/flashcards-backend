-- +goose Up
-- citext gives case-insensitive emails for the users table.
CREATE EXTENSION IF NOT EXISTS citext;

-- +goose Down
DROP EXTENSION IF EXISTS citext;
