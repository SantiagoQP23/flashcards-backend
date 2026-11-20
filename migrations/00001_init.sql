-- +goose Up
-- Intenli Flashcards data model.
-- Shared dictionary data (dictionary_words, definitions, word_audio) is written once and reused by every user.
-- Everything a user owns hangs off users and is soft-deleted so sync can tell devices what disappeared.

CREATE TYPE accent AS ENUM ('US', 'UK');
CREATE TYPE audio_source AS ENUM ('PROVIDER', 'KOKORO');
CREATE TYPE card_state AS ENUM ('NEW', 'LEARNING', 'REVIEW', 'RELEARNING');
CREATE TYPE rating AS ENUM ('AGAIN', 'HARD', 'GOOD', 'EASY');

CREATE TABLE users (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email            text NOT NULL UNIQUE,
    password_hash    text,                          -- null for Google-only accounts
    google_sub       text UNIQUE,                   -- Google account id
    display_name     text,
    timezone         text NOT NULL DEFAULT 'UTC',   -- IANA name, used for "today" and streaks
    preferred_accent accent NOT NULL DEFAULT 'US',
    daily_new_limit  integer NOT NULL DEFAULT 20,
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

CREATE TABLE dictionary_words (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    language    text NOT NULL DEFAULT 'en',
    lemma       text NOT NULL,                      -- lowercased, trimmed search key
    phonetic_us text,
    phonetic_uk text,
    source      text NOT NULL,                      -- "free-dictionary", ...
    source_urls text[] NOT NULL DEFAULT '{}',
    fetched_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (language, lemma)
);

CREATE TABLE definitions (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    word_id        uuid NOT NULL REFERENCES dictionary_words (id) ON DELETE CASCADE,
    part_of_speech text NOT NULL,                   -- noun, verb, adjective...
    text           text NOT NULL,
    example        text,
    synonyms       text[] NOT NULL DEFAULT '{}',
    antonyms       text[] NOT NULL DEFAULT '{}',
    position       integer NOT NULL                 -- order from the provider
);
CREATE INDEX definitions_word_id_idx ON definitions (word_id);

CREATE TABLE word_audio (
    id      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    word_id uuid NOT NULL REFERENCES dictionary_words (id) ON DELETE CASCADE,
    accent  accent NOT NULL,
    source  audio_source NOT NULL,
    url     text NOT NULL,
    voice   text,
    UNIQUE (word_id, accent, source)
);

CREATE TABLE search_history (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    query       text NOT NULL,
    word_id     uuid REFERENCES dictionary_words (id) ON DELETE SET NULL, -- null when not found
    searched_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX search_history_user_searched_idx ON search_history (user_id, searched_at);

CREATE TABLE decks (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE INDEX decks_user_id_idx ON decks (user_id);

CREATE TABLE vocabulary_entries (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    deck_id       uuid NOT NULL REFERENCES decks (id),
    word_id       uuid NOT NULL REFERENCES dictionary_words (id),
    definition_id uuid NOT NULL REFERENCES definitions (id), -- the meaning the user chose to learn
    note          text,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    deleted_at    timestamptz,
    UNIQUE (user_id, definition_id)                 -- one card per meaning per user
);
CREATE INDEX vocabulary_entries_user_deck_idx ON vocabulary_entries (user_id, deck_id);

CREATE TABLE flashcards (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entry_id       uuid NOT NULL UNIQUE REFERENCES vocabulary_entries (id) ON DELETE CASCADE,
    user_id        uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE, -- denormalised for the due query
    state          card_state NOT NULL DEFAULT 'NEW',
    due            timestamptz NOT NULL DEFAULT now(),
    stability      double precision NOT NULL DEFAULT 0,
    difficulty     double precision NOT NULL DEFAULT 0,
    elapsed_days   integer NOT NULL DEFAULT 0,
    scheduled_days integer NOT NULL DEFAULT 0,
    reps           integer NOT NULL DEFAULT 0,
    lapses         integer NOT NULL DEFAULT 0,
    last_review    timestamptz
);
CREATE INDEX flashcards_user_due_idx ON flashcards (user_id, due);

CREATE TABLE reviews (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    client_review_id uuid NOT NULL UNIQUE,          -- idempotency key from the phone
    flashcard_id     uuid NOT NULL REFERENCES flashcards (id) ON DELETE CASCADE,
    user_id          uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    rating           rating NOT NULL,
    state_before     card_state NOT NULL,
    reviewed_at      timestamptz NOT NULL,          -- client clock, clamped server-side
    duration_ms      integer,
    scheduled_days   integer NOT NULL,
    due_after        timestamptz NOT NULL
);
CREATE INDEX reviews_user_reviewed_idx ON reviews (user_id, reviewed_at);

-- +goose Down
DROP TABLE reviews;
DROP TABLE flashcards;
DROP TABLE vocabulary_entries;
DROP TABLE decks;
DROP TABLE search_history;
DROP TABLE word_audio;
DROP TABLE definitions;
DROP TABLE dictionary_words;
DROP TABLE refresh_tokens;
DROP TABLE users;
DROP TYPE rating;
DROP TYPE card_state;
DROP TYPE audio_source;
DROP TYPE accent;
