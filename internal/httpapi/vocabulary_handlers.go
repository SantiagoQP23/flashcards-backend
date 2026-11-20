package httpapi

import (
	"encoding/base64"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SantiagoQP23/flashcards-backend/internal/store"
)

const (
	defaultVocabularyLimit = 50
	maxVocabularyLimit     = 100
	maxWordLength          = 100
	maxMeaningLength       = 500
	maxNoteLength          = 500
)

// partsOfSpeech are the values a typed-in meaning may use ("" is allowed
// too). They match what dictionary providers return.
var partsOfSpeech = []string{
	"noun", "verb", "adjective", "adverb", "pronoun", "preposition",
	"conjunction", "interjection", "phrase",
}

// cleanWord trims, lowercases and collapses spaces in a typed word or
// phrase ("Give  Up" -> "give up").
func cleanWord(raw string) (string, error) {
	word := strings.ToLower(strings.Join(strings.Fields(raw), " "))
	if word == "" || len([]rune(word)) > maxWordLength {
		return "", badRequestCode("invalid_word", "word must be 1 to %d characters", maxWordLength)
	}
	return word, nil
}

func cleanMeaningText(raw string) (string, error) {
	text := strings.TrimSpace(raw)
	if text == "" || len([]rune(text)) > maxMeaningLength {
		return "", badRequestCode("invalid_definition", "definition text must be 1 to %d characters", maxMeaningLength)
	}
	return text, nil
}

func cleanExample(raw string) (string, error) {
	ex := strings.TrimSpace(raw)
	if len([]rune(ex)) > maxMeaningLength {
		return "", badRequest("example must be at most %d characters", maxMeaningLength)
	}
	return ex, nil
}

func cleanPartOfSpeech(raw string) (string, error) {
	pos := strings.ToLower(strings.TrimSpace(raw))
	if pos != "" && !slices.Contains(partsOfSpeech, pos) {
		return "", badRequest("part_of_speech must be one of %s", strings.Join(partsOfSpeech, ", "))
	}
	return pos, nil
}

func cleanNote(raw string) (string, error) {
	note := strings.TrimSpace(raw)
	if len([]rune(note)) > maxNoteLength {
		return "", badRequest("note must be at most %d characters", maxNoteLength)
	}
	return note, nil
}

// Cursors are opaque to the app: base64url of "<created_at>|<id>".
func encodeCursor(e store.Entry) string {
	raw := e.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + e.ID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(s string) (*store.EntryCursor, error) {
	invalid := badRequest("cursor is not valid")
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, invalid
	}
	at, id, ok := strings.Cut(string(raw), "|")
	if !ok {
		return nil, invalid
	}
	createdAt, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return nil, invalid
	}
	entryID, err := uuid.Parse(id)
	if err != nil {
		return nil, invalid
	}
	return &store.EntryCursor{CreatedAt: createdAt, ID: entryID}, nil
}

// GET /v1/vocabulary?deck_id&q&cursor&limit
func (a *api) handleListVocabulary(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.EntryFilter{Query: strings.ToLower(strings.TrimSpace(q.Get("q"))), Limit: defaultVocabularyLimit}
	var err error
	if v := q.Get("deck_id"); v != "" {
		id, perr := uuid.Parse(v)
		if perr != nil {
			err = badRequest("deck_id must be a UUID")
		}
		f.DeckID = &id
	}
	if v := q.Get("limit"); err == nil && v != "" {
		n, perr := strconv.Atoi(v)
		if perr != nil || n < 1 || n > maxVocabularyLimit {
			err = badRequest("limit must be 1 to %d", maxVocabularyLimit)
		}
		f.Limit = n
	}
	if v := q.Get("cursor"); err == nil && v != "" {
		f.After, err = decodeCursor(v)
	}
	if writeBadRequest(w, err) {
		return
	}

	limit := f.Limit
	f.Limit++ // one extra row tells us whether there is a next page
	entries, err := store.ListEntries(r.Context(), a.db, userIDFrom(r.Context()), f)
	if err != nil {
		internalError(w, r, err)
		return
	}
	page := entryPageDTO{Data: []entryDTO{}}
	if len(entries) > limit {
		entries = entries[:limit]
		next := encodeCursor(entries[limit-1])
		page.NextCursor = &next
	}
	for _, e := range entries {
		page.Data = append(page.Data, toEntryDTO(e))
	}
	writeJSON(w, http.StatusOK, page)
}

type meaningRequest struct {
	PartOfSpeech *string `json:"part_of_speech"`
	Text         *string `json:"text"`
	Example      *string `json:"example"`
}

// POST /v1/vocabulary saves a meaning in a deck. The meaning is either one
// from the dictionary (word_id + definition_id) or typed in by the user
// (word + definition).
func (a *api) handleCreateVocabulary(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeckID       uuid.UUID       `json:"deck_id"`
		Note         string          `json:"note"`
		WordID       *uuid.UUID      `json:"word_id"`
		DefinitionID *uuid.UUID      `json:"definition_id"`
		Word         *string         `json:"word"`
		Definition   *meaningRequest `json:"definition"`
	}
	err := decodeJSON(w, r, &req)
	fromDictionary := req.WordID != nil || req.DefinitionID != nil
	typed := req.Word != nil || req.Definition != nil
	if err == nil && req.DeckID == uuid.Nil {
		err = badRequest("deck_id is required")
	}
	if err == nil && fromDictionary == typed {
		err = badRequest("send either word_id and definition_id, or word and definition")
	}
	if err == nil && fromDictionary && (req.WordID == nil || req.DefinitionID == nil) {
		err = badRequest("word_id and definition_id are both required")
	}
	if err == nil && typed && (req.Word == nil || req.Definition == nil) {
		err = badRequest("word and definition are both required")
	}
	if err == nil {
		req.Note, err = cleanNote(req.Note)
	}
	var custom store.CustomEntry
	if err == nil && typed {
		custom, err = cleanCustomEntry(*req.Word, *req.Definition)
	}
	if writeBadRequest(w, err) {
		return
	}

	userID := userIDFrom(r.Context())
	var entry store.Entry
	err = store.InTx(r.Context(), a.pool, func(tx pgx.Tx) error {
		var err error
		if typed {
			custom.DeckID, custom.Note = req.DeckID, req.Note
			entry, err = store.CreateCustomEntry(r.Context(), tx, userID, custom)
		} else {
			entry, err = store.CreateEntry(r.Context(), tx, userID, store.NewEntry{
				WordID: *req.WordID, DefinitionID: *req.DefinitionID, DeckID: req.DeckID, Note: req.Note,
			})
		}
		return err
	})
	if writeEntryError(w, r, err) {
		return
	}
	writeJSON(w, http.StatusCreated, toEntryDTO(entry))
}

func cleanCustomEntry(word string, m meaningRequest) (store.CustomEntry, error) {
	var c store.CustomEntry
	var err error
	if c.Lemma, err = cleanWord(word); err != nil {
		return c, err
	}
	if m.Text == nil {
		return c, badRequestCode("invalid_definition", "definition text is required")
	}
	if c.Meaning.Text, err = cleanMeaningText(*m.Text); err != nil {
		return c, err
	}
	if m.Example != nil {
		if c.Meaning.Example, err = cleanExample(*m.Example); err != nil {
			return c, err
		}
	}
	if m.PartOfSpeech != nil {
		if c.Meaning.PartOfSpeech, err = cleanPartOfSpeech(*m.PartOfSpeech); err != nil {
			return c, err
		}
	}
	return c, nil
}

// PATCH /v1/vocabulary/{id} moves a saved word, picks another meaning of
// the same word, edits the note, or rewrites a typed-in meaning.
func (a *api) handleUpdateVocabulary(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(r, "id")
	if !ok {
		writeError(w, http.StatusNotFound, "entry_not_found", "saved word not found")
		return
	}
	var req struct {
		DeckID       *uuid.UUID      `json:"deck_id"`
		DefinitionID *uuid.UUID      `json:"definition_id"`
		Note         *string         `json:"note"`
		Definition   *meaningRequest `json:"definition"`
	}
	err := decodeJSON(w, r, &req)
	if err == nil && req.DeckID == nil && req.DefinitionID == nil && req.Note == nil && req.Definition == nil {
		err = badRequest("at least one field is required")
	}
	if err == nil && req.DefinitionID != nil && req.Definition != nil {
		err = badRequest("send definition_id or definition, not both")
	}
	if err == nil && req.Note != nil {
		var note string
		note, err = cleanNote(*req.Note)
		req.Note = &note
	}
	var meaning *store.MeaningEdit
	if err == nil && req.Definition != nil {
		meaning, err = cleanMeaningEdit(*req.Definition)
	}
	if writeBadRequest(w, err) {
		return
	}

	var entry store.Entry
	err = store.InTx(r.Context(), a.pool, func(tx pgx.Tx) error {
		var err error
		entry, err = store.UpdateEntry(r.Context(), tx, userIDFrom(r.Context()), id, store.EntryUpdate{
			DeckID: req.DeckID, DefinitionID: req.DefinitionID, Note: req.Note, Meaning: meaning,
		})
		return err
	})
	if writeEntryError(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, toEntryDTO(entry))
}

func cleanMeaningEdit(m meaningRequest) (*store.MeaningEdit, error) {
	if m.Text == nil && m.Example == nil && m.PartOfSpeech == nil {
		return nil, badRequest("definition needs at least one field")
	}
	var e store.MeaningEdit
	if m.Text != nil {
		text, err := cleanMeaningText(*m.Text)
		if err != nil {
			return nil, err
		}
		e.Text = &text
	}
	if m.Example != nil {
		ex, err := cleanExample(*m.Example)
		if err != nil {
			return nil, err
		}
		e.Example = &ex
	}
	if m.PartOfSpeech != nil {
		pos, err := cleanPartOfSpeech(*m.PartOfSpeech)
		if err != nil {
			return nil, err
		}
		e.PartOfSpeech = &pos
	}
	return &e, nil
}

// DELETE /v1/vocabulary/{id}
func (a *api) handleDeleteVocabulary(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(r, "id")
	if !ok {
		writeError(w, http.StatusNotFound, "entry_not_found", "saved word not found")
		return
	}
	err := store.DeleteEntry(r.Context(), a.db, userIDFrom(r.Context()), id)
	if writeEntryError(w, r, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeEntryError answers for a vocabulary store error and reports whether
// there was one.
func writeEntryError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "entry_not_found", "saved word not found")
	case errors.Is(err, store.ErrDeckNotFound):
		writeDeckNotFound(w)
	case errors.Is(err, store.ErrDefinitionNotFound):
		writeError(w, http.StatusNotFound, "definition_not_found", "that meaning doesn't belong to this word")
	case errors.Is(err, store.ErrDefinitionNotEditable):
		writeError(w, http.StatusConflict, "definition_not_editable", "only meanings you wrote can be edited")
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "already_saved", "you already saved this meaning")
	default:
		internalError(w, r, err)
	}
	return true
}
