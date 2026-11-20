package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SantiagoQP23/flashcards-backend/internal/dictionary"
)

// SourceStarter marks the words migration 00006 wrote for the starter deck.
// They hold one meaning each, so a search fills them in from a provider.
const SourceStarter = "intenli-starter"

// Word is a shared dictionary word (owner_id IS NULL).
type Word struct {
	ID         uuid.UUID
	Lemma      string
	PhoneticUS string
	PhoneticUK string
	Source     string
	SourceURLs []string
}

// FindWord returns the shared word for a lemma, or ErrNotFound.
func FindWord(ctx context.Context, db DB, lemma string) (Word, error) {
	var w Word
	err := db.QueryRow(ctx, `
		SELECT id, lemma, coalesce(phonetic_us, ''), coalesce(phonetic_uk, ''), source, source_urls
		FROM dictionary_words
		WHERE language = 'en' AND lemma = $1 AND owner_id IS NULL`, lemma).
		Scan(&w.ID, &w.Lemma, &w.PhoneticUS, &w.PhoneticUK, &w.Source, &w.SourceURLs)
	return w, notFound(err)
}

// SaveProviderWord stores a provider's answer for lemma in the shared
// dictionary and returns the word's id. A starter word gets the provider's
// meanings added after its own (saved entries point at it, so it stays), and
// its phonetics and source replaced. When another request stored the word
// first, it is left as that request wrote it. Run it in a transaction.
func SaveProviderWord(ctx context.Context, db DB, lemma string, e dictionary.Entry) (uuid.UUID, error) {
	var id uuid.UUID
	var source string
	err := db.QueryRow(ctx, `
		SELECT id, source FROM dictionary_words
		WHERE language = 'en' AND lemma = $1 AND owner_id IS NULL
		FOR UPDATE`, lemma).Scan(&id, &source)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		err = db.QueryRow(ctx, `
			INSERT INTO dictionary_words (lemma, phonetic_us, phonetic_uk, source, source_urls)
			VALUES ($1, nullif($2, ''), nullif($3, ''), $4, $5)
			ON CONFLICT (language, lemma) WHERE owner_id IS NULL DO NOTHING
			RETURNING id`, lemma, e.PhoneticUS, e.PhoneticUK, e.Source, nonNil(e.SourceURLs)).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			// Stored by a request that finished first.
			w, err := FindWord(ctx, db, lemma)
			return w.ID, err
		}
		if err != nil {
			return uuid.Nil, err
		}
	case err != nil:
		return uuid.Nil, err
	case source != SourceStarter:
		return id, nil
	default:
		if _, err := db.Exec(ctx, `
			UPDATE dictionary_words SET
				phonetic_us = coalesce(nullif($2, ''), phonetic_us),
				phonetic_uk = coalesce(nullif($3, ''), phonetic_uk),
				source = $4, source_urls = $5, fetched_at = now()
			WHERE id = $1`, id, e.PhoneticUS, e.PhoneticUK, e.Source, nonNil(e.SourceURLs)); err != nil {
			return uuid.Nil, err
		}
	}

	var start int
	if err := db.QueryRow(ctx, `SELECT coalesce(max(position), 0) FROM definitions WHERE word_id = $1`, id).
		Scan(&start); err != nil {
		return uuid.Nil, err
	}
	for i, d := range e.Definitions {
		if _, err := db.Exec(ctx, `
			INSERT INTO definitions (word_id, part_of_speech, text, example, synonyms, antonyms, position)
			VALUES ($1, $2, $3, nullif($4, ''), $5, $6, $7)`,
			id, d.PartOfSpeech, d.Text, d.Example, nonNil(d.Synonyms), nonNil(d.Antonyms), start+i+1); err != nil {
			return uuid.Nil, err
		}
	}
	for _, a := range e.Audio {
		if _, err := db.Exec(ctx, `
			INSERT INTO word_audio (word_id, accent, source, url) VALUES ($1, $2, 'PROVIDER', $3)
			ON CONFLICT (word_id, accent, source) DO UPDATE SET url = excluded.url`,
			id, a.Accent, a.URL); err != nil {
			return uuid.Nil, err
		}
	}
	return id, nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// WordView is a word as one user sees it: the shared meanings, then the
// user's own, each with the entry it is saved as, if any.
type WordView struct {
	Word
	Audio       []WordAudio
	Definitions []WordDefinition
}

type WordAudio struct {
	Accent string
	URL    string
	Source string
}

type WordDefinition struct {
	ID           uuid.UUID
	PartOfSpeech string
	Text         string
	Example      string
	Synonyms     []string
	Custom       bool
	// Saved is set when the user has saved this meaning.
	Saved *SavedRef
}

type SavedRef struct {
	EntryID uuid.UUID
	DeckID  uuid.UUID
}

// GetWordView loads a shared word for a user. ErrNotFound if it's gone.
func GetWordView(ctx context.Context, db DB, userID, wordID uuid.UUID) (WordView, error) {
	var v WordView
	err := db.QueryRow(ctx, `
		SELECT id, lemma, coalesce(phonetic_us, ''), coalesce(phonetic_uk, ''), source, source_urls
		FROM dictionary_words WHERE id = $1 AND owner_id IS NULL`, wordID).
		Scan(&v.ID, &v.Lemma, &v.PhoneticUS, &v.PhoneticUK, &v.Source, &v.SourceURLs)
	if err != nil {
		return WordView{}, notFound(err)
	}

	rows, err := db.Query(ctx, `
		SELECT accent, url, source FROM word_audio WHERE word_id = $1
		ORDER BY accent, source`, wordID)
	if err != nil {
		return WordView{}, err
	}
	v.Audio, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (WordAudio, error) {
		var a WordAudio
		return a, r.Scan(&a.Accent, &a.URL, &a.Source)
	})
	if err != nil {
		return WordView{}, err
	}

	rows, err = db.Query(ctx, `
		SELECT d.id, d.part_of_speech, d.text, coalesce(d.example, ''), d.synonyms, d.owner_id IS NOT NULL,
			e.id, e.deck_id
		FROM definitions d
		LEFT JOIN vocabulary_entries e
			ON e.definition_id = d.id AND e.user_id = $2 AND e.deleted_at IS NULL
		WHERE d.word_id = $1 AND (d.owner_id IS NULL OR d.owner_id = $2)
		ORDER BY d.owner_id IS NOT NULL, d.position, d.id`, wordID, userID)
	if err != nil {
		return WordView{}, err
	}
	v.Definitions, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (WordDefinition, error) {
		var d WordDefinition
		var entryID, deckID *uuid.UUID
		err := r.Scan(&d.ID, &d.PartOfSpeech, &d.Text, &d.Example, &d.Synonyms, &d.Custom, &entryID, &deckID)
		if entryID != nil && deckID != nil {
			d.Saved = &SavedRef{EntryID: *entryID, DeckID: *deckID}
		}
		return d, err
	})
	return v, err
}

// maxSearches is how many recent searches a user keeps.
const maxSearches = 50

// Search is a recent search. WordID is nil when the word wasn't found.
type Search struct {
	Query      string
	WordID     *uuid.UUID
	SearchedAt time.Time
}

// AddSearch records a search, keeping one row per query (the latest) and
// the user's newest maxSearches. Run it in a transaction.
func AddSearch(ctx context.Context, db DB, userID uuid.UUID, query string, wordID *uuid.UUID) error {
	if _, err := db.Exec(ctx, `DELETE FROM search_history WHERE user_id = $1 AND query = $2`, userID, query); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `INSERT INTO search_history (user_id, query, word_id) VALUES ($1, $2, $3)`,
		userID, query, wordID); err != nil {
		return err
	}
	_, err := db.Exec(ctx, `
		DELETE FROM search_history WHERE user_id = $1 AND id NOT IN (
			SELECT id FROM search_history WHERE user_id = $1 ORDER BY searched_at DESC, id DESC LIMIT $2)`,
		userID, maxSearches)
	return err
}

// ListSearches returns the user's recent searches, newest first.
func ListSearches(ctx context.Context, db DB, userID uuid.UUID, limit int) ([]Search, error) {
	rows, err := db.Query(ctx, `
		SELECT query, word_id, searched_at FROM search_history
		WHERE user_id = $1 ORDER BY searched_at DESC, id DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Search, error) {
		var s Search
		return s, r.Scan(&s.Query, &s.WordID, &s.SearchedAt)
	})
}

// ClearSearches deletes the user's search history.
func ClearSearches(ctx context.Context, db DB, userID uuid.UUID) error {
	_, err := db.Exec(ctx, `DELETE FROM search_history WHERE user_id = $1`, userID)
	return err
}
