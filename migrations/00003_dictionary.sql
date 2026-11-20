-- +goose Up
-- Shared dictionary data: written once from the provider and reused by every user.
CREATE TYPE audio_source AS ENUM ('PROVIDER', 'KOKORO');

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

-- +goose Down
DROP TABLE search_history;
DROP TABLE word_audio;
DROP TABLE definitions;
DROP TABLE dictionary_words;
DROP TYPE audio_source;
