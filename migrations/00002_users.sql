-- +goose Up
CREATE TYPE accent AS ENUM ('US', 'UK');

CREATE TABLE users (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email            citext NOT NULL UNIQUE,
    password_hash    text,                          -- null for Google-only accounts
    google_sub       text UNIQUE,                   -- Google account id
    display_name     text,
    timezone         text NOT NULL DEFAULT 'UTC',   -- IANA name, used for "today" and streaks
    locale           text NOT NULL DEFAULT 'es' CHECK (locale IN ('es', 'en')),
    preferred_accent accent NOT NULL DEFAULT 'US',
    daily_new_limit  integer NOT NULL DEFAULT 20 CHECK (daily_new_limit BETWEEN 0 AND 500),
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE refresh_tokens (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash text NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX refresh_tokens_user_id_idx ON refresh_tokens (user_id);

-- +goose Down
DROP TABLE refresh_tokens;
DROP TABLE users;
DROP TYPE accent;
