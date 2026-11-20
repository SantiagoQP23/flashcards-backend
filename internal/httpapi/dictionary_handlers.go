package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SantiagoQP23/flashcards-backend/internal/dictionary"
	"github.com/SantiagoQP23/flashcards-backend/internal/store"
)

const (
	defaultHistoryLimit = 20
	maxHistoryLimit     = 50
)

// cleanLookup is cleanWord for a search: it also needs a letter in it.
func cleanLookup(raw string) (string, error) {
	word, err := cleanWord(raw)
	if err != nil {
		return "", err
	}
	if !strings.ContainsFunc(word, unicode.IsLetter) {
		return "", badRequestCode("invalid_word", "word must contain a letter")
	}
	return word, nil
}

// GET /v1/dictionary/words/{word} looks a word up: from the cache when we
// have it, otherwise from the provider (and then cached for everyone). Every
// lookup goes into the user's search history.
func (a *api) handleLookupWord(w http.ResponseWriter, r *http.Request) {
	raw, err := url.PathUnescape(chi.URLParam(r, "word"))
	if err != nil {
		err = badRequestCode("invalid_word", "word is not valid")
	}
	var lemma string
	if err == nil {
		lemma, err = cleanLookup(raw)
	}
	if writeBadRequest(w, err) {
		return
	}
	ctx := r.Context()
	userID := userIDFrom(ctx)

	wordID, err := a.findOrFetchWord(ctx, lemma)
	switch {
	case errors.Is(err, dictionary.ErrNotFound):
		a.recordSearch(ctx, userID, lemma, nil)
		writeError(w, http.StatusNotFound, "word_not_found", "the dictionary doesn't have this word")
		return
	case errors.Is(err, dictionary.ErrUnavailable):
		writeError(w, http.StatusServiceUnavailable, "dictionary_unavailable", "the dictionary isn't answering, try again later")
		return
	case err != nil:
		internalError(w, r, err)
		return
	}

	view, err := store.GetWordView(ctx, a.db, userID, wordID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	a.recordSearch(ctx, userID, lemma, &wordID)
	writeJSON(w, http.StatusOK, toWordDTO(view))
}

// findOrFetchWord returns the cached word's id, asking the provider when it
// isn't cached (or is a starter word with only our one meaning). Lookups of
// the same word at the same time share one provider call.
func (a *api) findOrFetchWord(ctx context.Context, lemma string) (uuid.UUID, error) {
	cached, err := store.FindWord(ctx, a.db, lemma)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return uuid.Nil, err
	}
	found := err == nil
	if found && cached.Source != store.SourceStarter {
		return cached.ID, nil
	}
	if a.dictionary == nil {
		if found {
			return cached.ID, nil
		}
		return uuid.Nil, dictionary.ErrUnavailable
	}

	v, err, _ := a.lookups.Do(lemma, func() (any, error) {
		entry, err := a.dictionary.Lookup(ctx, lemma)
		if err != nil {
			return uuid.Nil, err
		}
		var id uuid.UUID
		err = store.InTx(ctx, a.pool, func(tx pgx.Tx) error {
			var err error
			id, err = store.SaveProviderWord(ctx, tx, lemma, entry)
			return err
		})
		return id, err
	})
	if err != nil {
		if found && (errors.Is(err, dictionary.ErrNotFound) || errors.Is(err, dictionary.ErrUnavailable)) {
			// The starter word still has its own meaning to show.
			return cached.ID, nil
		}
		if errors.Is(err, dictionary.ErrUnavailable) {
			slog.WarnContext(ctx, "dictionary lookup failed", "word", lemma, "err", err)
		}
		return uuid.Nil, err
	}
	return v.(uuid.UUID), nil
}

// recordSearch adds to the history; a failure there doesn't fail the search.
func (a *api) recordSearch(ctx context.Context, userID uuid.UUID, query string, wordID *uuid.UUID) {
	err := store.InTx(ctx, a.pool, func(tx pgx.Tx) error {
		return store.AddSearch(ctx, tx, userID, query, wordID)
	})
	if err != nil {
		slog.ErrorContext(ctx, "record search", "err", err)
	}
}

// GET /v1/dictionary/history?limit
func (a *api) handleListHistory(w http.ResponseWriter, r *http.Request) {
	limit := defaultHistoryLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxHistoryLimit {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit must be 1 to "+strconv.Itoa(maxHistoryLimit))
			return
		}
		limit = n
	}
	searches, err := store.ListSearches(r.Context(), a.db, userIDFrom(r.Context()), limit)
	if err != nil {
		internalError(w, r, err)
		return
	}
	out := listDTO[searchDTO]{Data: []searchDTO{}}
	for _, s := range searches {
		out.Data = append(out.Data, searchDTO{Query: s.Query, Found: s.WordID != nil, SearchedAt: s.SearchedAt})
	}
	writeJSON(w, http.StatusOK, out)
}

// DELETE /v1/dictionary/history
func (a *api) handleClearHistory(w http.ResponseWriter, r *http.Request) {
	if err := store.ClearSearches(r.Context(), a.db, userIDFrom(r.Context())); err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type searchDTO struct {
	Query      string    `json:"query"`
	Found      bool      `json:"found"`
	SearchedAt time.Time `json:"searched_at"`
}

type wordDTO struct {
	ID          uuid.UUID         `json:"id"`
	Word        string            `json:"word"`
	Phonetics   phoneticsDTO      `json:"phonetics"`
	Audio       []audioDTO        `json:"audio"`
	Meanings    []meaningGroupDTO `json:"meanings"`
	Attribution attributionDTO    `json:"attribution"`
}

type phoneticsDTO struct {
	US string `json:"us"`
	UK string `json:"uk"`
}

type audioDTO struct {
	Accent string `json:"accent"`
	URL    string `json:"url"`
	Source string `json:"source"`
}

type meaningGroupDTO struct {
	PartOfSpeech string              `json:"part_of_speech"`
	Definitions  []wordDefinitionDTO `json:"definitions"`
}

type wordDefinitionDTO struct {
	ID       uuid.UUID `json:"id"`
	Text     string    `json:"text"`
	Example  string    `json:"example"`
	Synonyms []string  `json:"synonyms"`
	IsCustom bool      `json:"is_custom"`
	Saved    *savedDTO `json:"saved"`
}

type savedDTO struct {
	EntryID uuid.UUID `json:"entry_id"`
	DeckID  uuid.UUID `json:"deck_id"`
}

type attributionDTO struct {
	Source  string   `json:"source"`
	License string   `json:"license"`
	URLs    []string `json:"urls"`
}

func attributionFor(source string, urls []string) attributionDTO {
	a := attributionDTO{URLs: urls}
	if a.URLs == nil {
		a.URLs = []string{}
	}
	switch source {
	case dictionary.SourceFreeDictionary:
		a.Source, a.License = "Wiktionary via Free Dictionary API", "CC BY-SA 3.0"
	case dictionary.SourceMerriamWebster:
		a.Source = "Merriam-Webster's Learner's Dictionary"
	default:
		a.Source = "Intenli"
	}
	return a
}

// toWordDTO groups the meanings by part of speech, in the order each part of
// speech first appears.
func toWordDTO(v store.WordView) wordDTO {
	d := wordDTO{
		ID:          v.ID,
		Word:        v.Lemma,
		Phonetics:   phoneticsDTO{US: v.PhoneticUS, UK: v.PhoneticUK},
		Audio:       []audioDTO{},
		Meanings:    []meaningGroupDTO{},
		Attribution: attributionFor(v.Source, v.SourceURLs),
	}
	for _, a := range v.Audio {
		d.Audio = append(d.Audio, audioDTO{Accent: a.Accent, URL: a.URL, Source: a.Source})
	}
	group := map[string]int{}
	for _, def := range v.Definitions {
		i, ok := group[def.PartOfSpeech]
		if !ok {
			i = len(d.Meanings)
			group[def.PartOfSpeech] = i
			d.Meanings = append(d.Meanings, meaningGroupDTO{PartOfSpeech: def.PartOfSpeech, Definitions: []wordDefinitionDTO{}})
		}
		out := wordDefinitionDTO{
			ID: def.ID, Text: def.Text, Example: def.Example, Synonyms: def.Synonyms, IsCustom: def.Custom,
		}
		if out.Synonyms == nil {
			out.Synonyms = []string{}
		}
		if def.Saved != nil {
			out.Saved = &savedDTO{EntryID: def.Saved.EntryID, DeckID: def.Saved.DeckID}
		}
		d.Meanings[i].Definitions = append(d.Meanings[i].Definitions, out)
	}
	return d
}
