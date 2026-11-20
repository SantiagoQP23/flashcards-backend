-- +goose Up
-- Each user's decks, saved words and spaced-repetition state. Soft-deleted rows
-- (deleted_at) let sync tell other devices what disappeared.
CREATE TYPE card_state AS ENUM ('NEW', 'LEARNING', 'REVIEW', 'RELEARNING');
CREATE TYPE rating AS ENUM ('AGAIN', 'HARD', 'GOOD', 'EASY');

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
DROP TYPE rating;
DROP TYPE card_state;
