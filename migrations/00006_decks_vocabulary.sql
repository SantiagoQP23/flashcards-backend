-- +goose Up
-- Decks and saved words (M4).

-- A deleted entry must not block saving the same meaning again, so the
-- one-card-per-meaning rule only covers live entries.
ALTER TABLE vocabulary_entries DROP CONSTRAINT vocabulary_entries_user_id_definition_id_key;
CREATE UNIQUE INDEX vocabulary_entries_user_definition_live_idx
    ON vocabulary_entries (user_id, definition_id) WHERE deleted_at IS NULL;
CREATE INDEX vocabulary_entries_user_created_idx ON vocabulary_entries (user_id, created_at DESC, id DESC);

-- Words and meanings a user types in themselves are private to that user
-- (owner_id set). Shared dictionary rows keep owner_id NULL, and dictionary
-- lookups must only read those. A private meaning can belong to a shared
-- word (the user's own meaning of "run") or to a private word the
-- dictionary doesn't know.
ALTER TABLE dictionary_words ADD COLUMN owner_id uuid REFERENCES users (id) ON DELETE CASCADE;
ALTER TABLE definitions ADD COLUMN owner_id uuid REFERENCES users (id) ON DELETE CASCADE;
ALTER TABLE dictionary_words DROP CONSTRAINT dictionary_words_language_lemma_key;
CREATE UNIQUE INDEX dictionary_words_shared_lemma_idx ON dictionary_words (language, lemma) WHERE owner_id IS NULL;
CREATE UNIQUE INDEX dictionary_words_owner_lemma_idx ON dictionary_words (owner_id, language, lemma) WHERE owner_id IS NOT NULL;
CREATE INDEX definitions_owner_id_idx ON definitions (owner_id) WHERE owner_id IS NOT NULL;

-- Deck names are unique per user, ignoring case, among live decks.
CREATE UNIQUE INDEX decks_user_name_live_idx ON decks (user_id, lower(name)) WHERE deleted_at IS NULL;

-- The starter deck: every new account gets these meanings in a "Daily
-- Vocabulary" deck. The words are written by us (source intenli-starter), so
-- they need no attribution.
CREATE TABLE starter_deck_words (
    position      integer PRIMARY KEY,
    definition_id uuid NOT NULL REFERENCES definitions (id)
);

CREATE TEMP TABLE starter_seed (
    position       integer,
    lemma          text,
    phonetic_us    text,
    phonetic_uk    text,
    part_of_speech text,
    text           text,
    example        text,
    synonyms       text[]
) ON COMMIT DROP;

INSERT INTO starter_seed VALUES
    (1,  'achieve',     '/əˈtʃiːv/',      '/əˈtʃiːv/',      'verb',      'To succeed in doing or getting something after making an effort.', 'She achieved her goal of running a marathon.', ARRAY['accomplish', 'reach']),
    (2,  'afford',      '/əˈfɔːrd/',      '/əˈfɔːd/',       'verb',      'To have enough money or time to pay for or do something.', 'We can''t afford a new car this year.', ARRAY[]::text[]),
    (3,  'appointment', '/əˈpɔɪntmənt/',  '/əˈpɔɪntmənt/',  'noun',      'An arrangement to meet someone at a particular time.', 'I have a dentist appointment at three.', ARRAY['meeting']),
    (4,  'available',   '/əˈveɪləbəl/',   '/əˈveɪləbəl/',   'adjective', 'Able to be used, bought or contacted.', 'Is this seat available?', ARRAY['free']),
    (5,  'aware',       '/əˈwer/',        '/əˈweə/',        'adjective', 'Knowing that something exists or is happening.', 'Are you aware of the risks?', ARRAY['conscious']),
    (6,  'borrow',      '/ˈbɑːroʊ/',      '/ˈbɒrəʊ/',       'verb',      'To take something from someone and give it back later.', 'Can I borrow your pen?', ARRAY[]::text[]),
    (7,  'challenge',   '/ˈtʃælɪndʒ/',    '/ˈtʃælɪndʒ/',    'noun',      'Something difficult that tests your ability.', 'Learning a new language is a real challenge.', ARRAY['test']),
    (8,  'convenient',  '/kənˈviːniənt/', '/kənˈviːniənt/', 'adjective', 'Easy, useful or suitable for your plans.', 'Is Friday convenient for you?', ARRAY['handy', 'suitable']),
    (9,  'deadline',    '/ˈdedlaɪn/',     '/ˈdedlaɪn/',     'noun',      'The time or date by which something must be done.', 'The deadline for the report is Monday.', ARRAY[]::text[]),
    (10, 'eventually',  '/ɪˈventʃuəli/',  '/ɪˈventʃuəli/',  'adverb',    'In the end, after a long time or after many problems.', 'He eventually found a job he liked.', ARRAY['finally']),
    (11, 'improve',     '/ɪmˈpruːv/',     '/ɪmˈpruːv/',     'verb',      'To become better, or to make something better.', 'Your English has improved a lot.', ARRAY['get better']),
    (12, 'actually',    '/ˈæktʃuəli/',    '/ˈæktʃuəli/',    'adverb',    'In fact; used to say what is really true.', 'Actually, I''ve never been to London.', ARRAY['really', 'in fact']),
    (13, 'meanwhile',   '/ˈmiːnwaɪl/',    '/ˈmiːnwaɪl/',    'adverb',    'At the same time, while something else is happening.', 'Dinner is almost ready. Meanwhile, set the table.', ARRAY[]::text[]),
    (14, 'owe',         '/oʊ/',           '/əʊ/',           'verb',      'To need to pay or give back money to someone.', 'I owe you ten dollars.', ARRAY[]::text[]),
    (15, 'reliable',    '/rɪˈlaɪəbəl/',   '/rɪˈlaɪəbəl/',   'adjective', 'Able to be trusted to work well or to do what is expected.', 'We need a reliable car for the trip.', ARRAY['dependable', 'trustworthy']),
    (16, 'remind',      '/rɪˈmaɪnd/',     '/rɪˈmaɪnd/',     'verb',      'To make someone remember something.', 'Remind me to call my mother tonight.', ARRAY[]::text[]),
    (17, 'schedule',    '/ˈskedʒuːl/',    '/ˈʃedjuːl/',     'noun',      'A plan that lists when things will happen.', 'What''s your schedule for tomorrow?', ARRAY['timetable', 'plan']),
    (18, 'struggle',    '/ˈstrʌɡəl/',     '/ˈstrʌɡəl/',     'verb',      'To try very hard to do something that is difficult.', 'She struggled to understand the instructions.', ARRAY[]::text[]),
    (19, 'suggest',     '/səɡˈdʒest/',    '/səˈdʒest/',     'verb',      'To give an idea or a plan for someone to think about.', 'I suggest we leave early to avoid traffic.', ARRAY['propose', 'recommend']),
    (20, 'worth',       '/wɝːθ/',         '/wɜːθ/',         'adjective', 'Deserving to be done, seen or bought.', 'The museum is worth visiting.', ARRAY[]::text[]);

INSERT INTO dictionary_words (lemma, phonetic_us, phonetic_uk, source)
SELECT lemma, phonetic_us, phonetic_uk, 'intenli-starter' FROM starter_seed
ON CONFLICT (language, lemma) WHERE owner_id IS NULL DO NOTHING;

-- Only words we just wrote get our meaning; a word already cached from a
-- provider keeps its own and the starter deck uses its first meaning.
INSERT INTO definitions (word_id, part_of_speech, text, example, synonyms, position)
SELECT w.id, s.part_of_speech, s.text, s.example, s.synonyms, 0
FROM starter_seed s JOIN dictionary_words w ON w.language = 'en' AND w.lemma = s.lemma AND w.owner_id IS NULL
WHERE w.source = 'intenli-starter';

INSERT INTO starter_deck_words (position, definition_id)
SELECT s.position, d.id
FROM starter_seed s
JOIN dictionary_words w ON w.language = 'en' AND w.lemma = s.lemma AND w.owner_id IS NULL
JOIN LATERAL (
    SELECT id FROM definitions WHERE word_id = w.id AND owner_id IS NULL ORDER BY position, id LIMIT 1
) d ON true;

-- +goose Down
DROP TABLE starter_deck_words;
DROP INDEX decks_user_name_live_idx;
DROP INDEX vocabulary_entries_user_created_idx;
DROP INDEX vocabulary_entries_user_definition_live_idx;
DROP INDEX definitions_owner_id_idx;
DROP INDEX dictionary_words_owner_lemma_idx;
DROP INDEX dictionary_words_shared_lemma_idx;
-- Rolling back drops the words people typed in themselves.
DELETE FROM vocabulary_entries
WHERE definition_id IN (SELECT id FROM definitions WHERE owner_id IS NOT NULL)
   OR word_id IN (SELECT id FROM dictionary_words WHERE owner_id IS NOT NULL);
DELETE FROM definitions WHERE owner_id IS NOT NULL;
DELETE FROM dictionary_words WHERE owner_id IS NOT NULL;
ALTER TABLE dictionary_words ADD CONSTRAINT dictionary_words_language_lemma_key UNIQUE (language, lemma);
ALTER TABLE definitions DROP COLUMN owner_id;
ALTER TABLE dictionary_words DROP COLUMN owner_id;
ALTER TABLE vocabulary_entries ADD CONSTRAINT vocabulary_entries_user_id_definition_id_key UNIQUE (user_id, definition_id);
-- The starter words stay in dictionary_words: saved entries may point at them.
