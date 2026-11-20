package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	// ErrDeckNotFound means the deck given for an entry isn't a live deck of
	// the user.
	ErrDeckNotFound = errors.New("deck not found")
	// ErrDefinitionNotFound means the meaning isn't one of the word's, or is
	// another user's.
	ErrDefinitionNotFound = errors.New("definition not found")
	// ErrDefinitionNotEditable means the entry's meaning comes from the
	// dictionary, so the user can't rewrite it.
	ErrDefinitionNotEditable = errors.New("definition not editable")
)

// Entry is a saved word: the meaning the user chose, the deck it's in and
// its flashcard.
type Entry struct {
	ID         uuid.UUID
	DeckID     uuid.UUID
	Note       string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Word       EntryWord
	Definition EntryDefinition
	Flashcard  EntryFlashcard
}

type EntryWord struct {
	ID         uuid.UUID
	Lemma      string
	PhoneticUS string
	PhoneticUK string
}

type EntryDefinition struct {
	ID           uuid.UUID
	PartOfSpeech string
	Text         string
	Example      string
	Synonyms     []string
	// Custom is true for a meaning the user wrote themselves.
	Custom bool
}

type EntryFlashcard struct {
	ID    uuid.UUID
	State string
	Due   time.Time
}

const entrySelect = `
	SELECT e.id, e.deck_id, coalesce(e.note, ''), e.created_at, e.updated_at,
		w.id, w.lemma, coalesce(w.phonetic_us, ''), coalesce(w.phonetic_uk, ''),
		def.id, def.part_of_speech, def.text, coalesce(def.example, ''), def.synonyms, def.owner_id IS NOT NULL,
		f.id, f.state, f.due
	FROM vocabulary_entries e
	JOIN dictionary_words w ON w.id = e.word_id
	JOIN definitions def ON def.id = e.definition_id
	JOIN flashcards f ON f.entry_id = e.id
	WHERE e.user_id = $1 AND e.deleted_at IS NULL`

func scanEntry(row interface{ Scan(...any) error }) (Entry, error) {
	var e Entry
	err := row.Scan(&e.ID, &e.DeckID, &e.Note, &e.CreatedAt, &e.UpdatedAt,
		&e.Word.ID, &e.Word.Lemma, &e.Word.PhoneticUS, &e.Word.PhoneticUK,
		&e.Definition.ID, &e.Definition.PartOfSpeech, &e.Definition.Text, &e.Definition.Example, &e.Definition.Synonyms, &e.Definition.Custom,
		&e.Flashcard.ID, &e.Flashcard.State, &e.Flashcard.Due)
	return e, notFound(err)
}

// EntryCursor is where a page of entries continues: after the entry with
// this created_at and id, newest first.
type EntryCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// EntryFilter narrows ListEntries. Query matches anywhere in the word,
// ignoring case.
type EntryFilter struct {
	DeckID *uuid.UUID
	Query  string
	After  *EntryCursor
	Limit  int
}

// ListEntries returns the user's live entries, newest first.
func ListEntries(ctx context.Context, db DB, userID uuid.UUID, f EntryFilter) ([]Entry, error) {
	var afterAt *time.Time
	var afterID *uuid.UUID
	if f.After != nil {
		afterAt, afterID = &f.After.CreatedAt, &f.After.ID
	}
	rows, err := db.Query(ctx, entrySelect+`
		AND ($2::uuid IS NULL OR e.deck_id = $2)
		AND ($3 = '' OR w.lemma ILIKE '%' || $3 || '%' ESCAPE '\')
		AND ($4::timestamptz IS NULL OR (e.created_at, e.id) < ($4, $5::uuid))
		ORDER BY e.created_at DESC, e.id DESC
		LIMIT $6`,
		userID, f.DeckID, escapeLike(f.Query), afterAt, afterID, f.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []Entry{}
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// GetEntry returns one live entry of the user, or ErrNotFound.
func GetEntry(ctx context.Context, db DB, userID, id uuid.UUID) (Entry, error) {
	return scanEntry(db.QueryRow(ctx, entrySelect+` AND e.id = $2`, userID, id))
}

// definitionOfWord reports whether the meaning belongs to the word and is
// either shared or the user's own.
func definitionOfWord(ctx context.Context, db DB, userID, definitionID, wordID uuid.UUID) (bool, error) {
	var ok bool
	err := db.QueryRow(ctx, `
		SELECT true FROM definitions
		WHERE id = $1 AND word_id = $2 AND (owner_id IS NULL OR owner_id = $3)`,
		definitionID, wordID, userID).Scan(&ok)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return ok, err
}

// NewEntry is a meaning to save. Note may be empty.
type NewEntry struct {
	WordID       uuid.UUID
	DefinitionID uuid.UUID
	DeckID       uuid.UUID
	Note         string
}

// CreateEntry saves a meaning in a deck with a new flashcard. Errors:
// ErrDeckNotFound, ErrDefinitionNotFound, and ErrConflict when the user has
// already saved that meaning. Run it in a transaction.
func CreateEntry(ctx context.Context, db DB, userID uuid.UUID, n NewEntry) (Entry, error) {
	if ok, err := deckIsLive(ctx, db, userID, n.DeckID); err != nil || !ok {
		return Entry{}, orErr(err, ErrDeckNotFound)
	}
	if ok, err := definitionOfWord(ctx, db, userID, n.DefinitionID, n.WordID); err != nil || !ok {
		return Entry{}, orErr(err, ErrDefinitionNotFound)
	}
	var id uuid.UUID
	err := db.QueryRow(ctx, `
		INSERT INTO vocabulary_entries (user_id, deck_id, word_id, definition_id, note)
		VALUES ($1, $2, $3, $4, nullif($5, ''))
		RETURNING id`, userID, n.DeckID, n.WordID, n.DefinitionID, n.Note).Scan(&id)
	if isUniqueViolation(err) {
		return Entry{}, ErrConflict
	}
	if err != nil {
		return Entry{}, err
	}
	if _, err := db.Exec(ctx, `INSERT INTO flashcards (entry_id, user_id) VALUES ($1, $2)`, id, userID); err != nil {
		return Entry{}, err
	}
	return GetEntry(ctx, db, userID, id)
}

// Meaning is a meaning the user writes themselves.
type Meaning struct {
	PartOfSpeech string
	Text         string
	Example      string
}

// CustomEntry is a word saved by hand: the user types the word and their
// own meaning. Lemma is already trimmed and lowercased.
type CustomEntry struct {
	Lemma   string
	Meaning Meaning
	DeckID  uuid.UUID
	Note    string
}

// CreateCustomEntry saves a word the user typed in. The meaning is private
// to the user; it hangs off the shared dictionary word when there is one and
// off a private word otherwise. Errors: ErrDeckNotFound. Run it in a
// transaction.
func CreateCustomEntry(ctx context.Context, db DB, userID uuid.UUID, c CustomEntry) (Entry, error) {
	if ok, err := deckIsLive(ctx, db, userID, c.DeckID); err != nil || !ok {
		return Entry{}, orErr(err, ErrDeckNotFound)
	}
	var wordID uuid.UUID
	err := db.QueryRow(ctx, `
		SELECT id FROM dictionary_words
		WHERE language = 'en' AND lemma = $1 AND owner_id IS NULL`, c.Lemma).Scan(&wordID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = db.QueryRow(ctx, `
			INSERT INTO dictionary_words (lemma, source, owner_id) VALUES ($1, 'user', $2)
			ON CONFLICT (owner_id, language, lemma) WHERE owner_id IS NOT NULL
				DO UPDATE SET fetched_at = dictionary_words.fetched_at
			RETURNING id`, c.Lemma, userID).Scan(&wordID)
	}
	if err != nil {
		return Entry{}, err
	}
	var definitionID uuid.UUID
	err = db.QueryRow(ctx, `
		INSERT INTO definitions (word_id, owner_id, part_of_speech, text, example, position)
		VALUES ($1, $2, $3, $4, nullif($5, ''),
			(SELECT coalesce(max(position), 0) + 1 FROM definitions WHERE word_id = $1))
		RETURNING id`, wordID, userID, c.Meaning.PartOfSpeech, c.Meaning.Text, c.Meaning.Example).
		Scan(&definitionID)
	if err != nil {
		return Entry{}, err
	}
	return CreateEntry(ctx, db, userID, NewEntry{
		WordID: wordID, DefinitionID: definitionID, DeckID: c.DeckID, Note: c.Note,
	})
}

// MeaningEdit changes the user's own meaning; nil leaves a field as it is
// and an empty Example clears it.
type MeaningEdit struct {
	PartOfSpeech *string
	Text         *string
	Example      *string
}

// EntryUpdate holds what PATCH /v1/vocabulary/{id} can change; nil leaves a
// field as it is and an empty Note clears it.
type EntryUpdate struct {
	DeckID       *uuid.UUID
	DefinitionID *uuid.UUID
	Note         *string
	// Meaning rewrites the entry's meaning; only for a meaning the user
	// wrote (ErrDefinitionNotEditable otherwise).
	Meaning *MeaningEdit
}

// UpdateEntry moves an entry to another deck, changes its meaning (to
// another meaning of the same word, which starts its flashcard over) or
// edits its note. Errors: ErrNotFound, ErrDeckNotFound,
// ErrDefinitionNotFound, and ErrConflict when the new meaning is already
// saved. Run it in a transaction.
func UpdateEntry(ctx context.Context, db DB, userID, id uuid.UUID, u EntryUpdate) (Entry, error) {
	var wordID, definitionID uuid.UUID
	err := db.QueryRow(ctx, `
		SELECT word_id, definition_id FROM vocabulary_entries
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
		FOR UPDATE`, id, userID).Scan(&wordID, &definitionID)
	if err != nil {
		return Entry{}, notFound(err)
	}
	if u.DeckID != nil {
		if ok, err := deckIsLive(ctx, db, userID, *u.DeckID); err != nil || !ok {
			return Entry{}, orErr(err, ErrDeckNotFound)
		}
	}
	newMeaning := u.DefinitionID != nil && *u.DefinitionID != definitionID
	if newMeaning {
		if ok, err := definitionOfWord(ctx, db, userID, *u.DefinitionID, wordID); err != nil || !ok {
			return Entry{}, orErr(err, ErrDefinitionNotFound)
		}
	}
	_, err = db.Exec(ctx, `
		UPDATE vocabulary_entries SET
			deck_id       = coalesce($3, deck_id),
			definition_id = coalesce($4, definition_id),
			note          = CASE WHEN $5::text IS NULL THEN note ELSE nullif($5, '') END,
			updated_at    = now()
		WHERE id = $1 AND user_id = $2`, id, userID, u.DeckID, u.DefinitionID, u.Note)
	if isUniqueViolation(err) {
		return Entry{}, ErrConflict
	}
	if err != nil {
		return Entry{}, err
	}
	if u.Meaning != nil {
		tag, err := db.Exec(ctx, `
			UPDATE definitions SET
				part_of_speech = coalesce($3, part_of_speech),
				text           = coalesce($4, text),
				example        = CASE WHEN $5::text IS NULL THEN example ELSE nullif($5, '') END
			WHERE id = (SELECT definition_id FROM vocabulary_entries WHERE id = $1) AND owner_id = $2`,
			id, userID, u.Meaning.PartOfSpeech, u.Meaning.Text, u.Meaning.Example)
		if err != nil {
			return Entry{}, err
		}
		if tag.RowsAffected() == 0 {
			return Entry{}, ErrDefinitionNotEditable
		}
	}
	if newMeaning {
		// A different meaning is a different thing to remember.
		if _, err := db.Exec(ctx, `
			UPDATE flashcards SET state = 'NEW', due = now(), stability = 0, difficulty = 0,
				elapsed_days = 0, scheduled_days = 0, reps = 0, lapses = 0, last_review = NULL
			WHERE entry_id = $1`, id); err != nil {
			return Entry{}, err
		}
	}
	return GetEntry(ctx, db, userID, id)
}

// DeleteEntry soft-deletes an entry; its flashcard leaves the due queue
// with it. ErrNotFound means no such live entry.
func DeleteEntry(ctx context.Context, db DB, userID, id uuid.UUID) error {
	tag, err := db.Exec(ctx, `
		UPDATE vocabulary_entries SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// orErr returns err when it is set and otherwise the fallback.
func orErr(err, fallback error) error {
	if err != nil {
		return err
	}
	return fallback
}
