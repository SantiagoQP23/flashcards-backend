package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/SantiagoQP23/flashcards-backend/internal/store"
)

const maxDeckNameLength = 60

// starterDeckName is the starter deck's name in each app language.
var starterDeckName = map[string]string{
	"es": "Vocabulario diario",
	"en": "Daily Vocabulary",
}

func cleanDeckName(raw string) (string, error) {
	name := strings.Join(strings.Fields(raw), " ")
	if name == "" || len([]rune(name)) > maxDeckNameLength {
		return "", badRequestCode("invalid_deck_name", "name must be 1 to %d characters", maxDeckNameLength)
	}
	return name, nil
}

// GET /v1/decks
func (a *api) handleListDecks(w http.ResponseWriter, r *http.Request) {
	decks, err := store.ListDecks(r.Context(), a.db, userIDFrom(r.Context()), a.now())
	if err != nil {
		internalError(w, r, err)
		return
	}
	out := make([]deckDTO, len(decks))
	for i, d := range decks {
		out[i] = toDeckDTO(d)
	}
	writeJSON(w, http.StatusOK, listDTO[deckDTO]{Data: out})
}

// POST /v1/decks
func (a *api) handleCreateDeck(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	err := decodeJSON(w, r, &req)
	if err == nil {
		req.Name, err = cleanDeckName(req.Name)
	}
	if writeBadRequest(w, err) {
		return
	}
	deck, err := store.CreateDeck(r.Context(), a.db, userIDFrom(r.Context()), req.Name)
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "deck_name_taken", "you already have a deck with this name")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toDeckDTO(deck))
}

// PATCH /v1/decks/{id}
func (a *api) handleRenameDeck(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(r, "id")
	if !ok {
		writeDeckNotFound(w)
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	err := decodeJSON(w, r, &req)
	if err == nil {
		req.Name, err = cleanDeckName(req.Name)
	}
	if writeBadRequest(w, err) {
		return
	}
	userID := userIDFrom(r.Context())
	err = store.RenameDeck(r.Context(), a.db, userID, id, req.Name)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeDeckNotFound(w)
		return
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "deck_name_taken", "you already have a deck with this name")
		return
	case err != nil:
		internalError(w, r, err)
		return
	}
	deck, err := store.GetDeck(r.Context(), a.db, userID, id, a.now())
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toDeckDTO(deck))
}

// DELETE /v1/decks/{id} soft-deletes the deck and every saved word in it.
func (a *api) handleDeleteDeck(w http.ResponseWriter, r *http.Request) {
	id, ok := uuidParam(r, "id")
	if !ok {
		writeDeckNotFound(w)
		return
	}
	err := store.InTx(r.Context(), a.pool, func(tx pgx.Tx) error {
		return store.DeleteDeck(r.Context(), tx, userIDFrom(r.Context()), id)
	})
	if errors.Is(err, store.ErrNotFound) {
		writeDeckNotFound(w)
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeDeckNotFound(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, "deck_not_found", "deck not found")
}
