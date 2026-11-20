-- +goose Up
-- Sign-in for M1. There is no email check yet, so a password account's email
-- is unproven until the same address signs in with Google.
ALTER TABLE users ADD COLUMN email_verified boolean NOT NULL DEFAULT false;

-- Refresh tokens rotate: each refresh revokes the old token and issues a new
-- one in the same family. Reusing a rotated token revokes the whole family.
-- The table has never been written to, so its columns can change freely.
ALTER TABLE refresh_tokens
    ADD COLUMN family_id uuid NOT NULL,
    ADD COLUMN rotated_at timestamptz,
    ALTER COLUMN token_hash TYPE bytea USING decode(token_hash, 'hex');
CREATE INDEX refresh_tokens_family_id_idx ON refresh_tokens (family_id);

-- +goose Down
DROP INDEX refresh_tokens_family_id_idx;
ALTER TABLE refresh_tokens
    DROP COLUMN family_id,
    DROP COLUMN rotated_at,
    ALTER COLUMN token_hash TYPE text USING encode(token_hash, 'hex');
ALTER TABLE users DROP COLUMN email_verified;
