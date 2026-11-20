package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Deck is a live deck with its counts. EntryCount is the saved words in it
// and DueCount the ones whose flashcard is due at the time asked.
type Deck struct {
	ID         uuid.UUID
	Name       string
	EntryCount int
	DueCount   int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// deckSelect reads decks with their counts; $1 is the user and $2 the time
// "due" is measured against.
const deckSelect = `
	SELECT d.id, d.name,
		count(e.id)::int,
		count(e.id) FILTER (WHERE f.due <= $2)::int,
		d.created_at, d.updated_at
	FROM decks d
	LEFT JOIN vocabulary_entries e ON e.deck_id = d.id AND e.deleted_at IS NULL
	LEFT JOIN flashcards f ON f.entry_id = e.id
	WHERE d.user_id = $1 AND d.deleted_at IS NULL`

func scanDeck(row interface{ Scan(...any) error }) (Deck, error) {
	var d Deck
	err := row.Scan(&d.ID, &d.Name, &d.EntryCount, &d.DueCount, &d.CreatedAt, &d.UpdatedAt)
	return d, notFound(err)
}

// ListDecks returns the user's live decks, oldest first.
func ListDecks(ctx context.Context, db DB, userID uuid.UUID, now time.Time) ([]Deck, error) {
	rows, err := db.Query(ctx, deckSelect+` GROUP BY d.id ORDER BY d.created_at, d.id`, userID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	decks := []Deck{}
	for rows.Next() {
		d, err := scanDeck(rows)
		if err != nil {
			return nil, err
		}
		decks = append(decks, d)
	}
	return decks, rows.Err()
}

// GetDeck returns one live deck of the user, or ErrNotFound.
func GetDeck(ctx context.Context, db DB, userID, id uuid.UUID, now time.Time) (Deck, error) {
	return scanDeck(db.QueryRow(ctx, deckSelect+` AND d.id = $3 GROUP BY d.id`, userID, now, id))
}

// deckIsLive reports whether id is a live deck of the user, locking it so a
// concurrent delete can't strand a new entry in it.
func deckIsLive(ctx context.Context, db DB, userID, id uuid.UUID) (bool, error) {
	var ok bool
	err := db.QueryRow(ctx, `
		SELECT true FROM decks WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL FOR SHARE`,
		id, userID).Scan(&ok)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return ok, err
}

// CreateDeck adds an empty deck. ErrConflict means the user already has a
// deck with that name.
func CreateDeck(ctx context.Context, db DB, userID uuid.UUID, name string) (Deck, error) {
	var d Deck
	err := db.QueryRow(ctx, `
		INSERT INTO decks (user_id, name) VALUES ($1, $2)
		RETURNING id, name, created_at, updated_at`, userID, name).
		Scan(&d.ID, &d.Name, &d.CreatedAt, &d.UpdatedAt)
	if isUniqueViolation(err) {
		return Deck{}, ErrConflict
	}
	return d, err
}

// RenameDeck renames a live deck. ErrNotFound means no such deck,
// ErrConflict that the name is taken.
func RenameDeck(ctx context.Context, db DB, userID, id uuid.UUID, name string) error {
	tag, err := db.Exec(ctx, `
		UPDATE decks SET name = $3, updated_at = now()
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`, id, userID, name)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteDeck soft-deletes a live deck and the entries in it. Run it in a
// transaction.
func DeleteDeck(ctx context.Context, db DB, userID, id uuid.UUID) error {
	tag, err := db.Exec(ctx, `
		UPDATE decks SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	_, err = db.Exec(ctx, `
		UPDATE vocabulary_entries SET deleted_at = now(), updated_at = now()
		WHERE deck_id = $1 AND deleted_at IS NULL`, id)
	return err
}

// CreateStarterDeck gives a new user the starter deck: the meanings in
// starter_deck_words, each with a new flashcard. Run it in the transaction
// that creates the user.
func CreateStarterDeck(ctx context.Context, db DB, userID uuid.UUID, name string) error {
	d, err := CreateDeck(ctx, db, userID, name)
	if err != nil {
		return err
	}
	// Entries get created_at a microsecond apart, earliest position newest,
	// so the newest-first list shows them in the starter order.
	_, err = db.Exec(ctx, `
		WITH e AS (
			INSERT INTO vocabulary_entries (user_id, deck_id, word_id, definition_id, created_at, updated_at)
			SELECT $1, $2, def.word_id, def.id,
				now() - s.position * interval '1 microsecond',
				now() - s.position * interval '1 microsecond'
			FROM starter_deck_words s JOIN definitions def ON def.id = s.definition_id
			RETURNING id
		)
		INSERT INTO flashcards (entry_id, user_id) SELECT id, $1 FROM e`, userID, d.ID)
	return err
}
