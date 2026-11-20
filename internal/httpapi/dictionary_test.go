package httpapi

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/SantiagoQP23/flashcards-backend/internal/dictionary"
)

type fakeDictionary struct {
	mu      sync.Mutex
	entries map[string]dictionary.Entry
	down    map[string]bool
	calls   map[string]int
}

func (f *fakeDictionary) Lookup(_ context.Context, word string) (dictionary.Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[word]++
	if f.down[word] {
		return dictionary.Entry{}, dictionary.ErrUnavailable
	}
	e, ok := f.entries[word]
	if !ok {
		return dictionary.Entry{}, dictionary.ErrNotFound
	}
	return e, nil
}

func newFakeDictionary() *fakeDictionary {
	return &fakeDictionary{
		calls: map[string]int{},
		down:  map[string]bool{"outage": true, "afford": true},
		entries: map[string]dictionary.Entry{
			"resilient": {
				PhoneticUS: "/ɹɪˈzɪli.ənt/", PhoneticUK: "/rɪˈzɪlɪənt/",
				Audio: []dictionary.Audio{{Accent: "US", URL: "https://example.com/resilient-us.mp3"}},
				Definitions: []dictionary.Definition{
					{PartOfSpeech: "adjective", Text: "Able to recover quickly.", Example: "Children are resilient.", Synonyms: []string{"tough"}},
					{PartOfSpeech: "noun", Text: "A made-up noun meaning."},
					{PartOfSpeech: "adjective", Text: "Returning to its shape."},
				},
				Source: dictionary.SourceFreeDictionary, SourceURLs: []string{"https://en.wiktionary.org/wiki/resilient"},
			},
			"give up": {
				Definitions: []dictionary.Definition{{PartOfSpeech: "phrase", Text: "To stop trying."}},
				Source:      dictionary.SourceFreeDictionary,
			},
			"achieve": {
				PhoneticUS: "/əˈtʃiv/",
				Audio:      []dictionary.Audio{{Accent: "UK", URL: "https://example.com/achieve-uk.mp3"}},
				Definitions: []dictionary.Definition{
					{PartOfSpeech: "verb", Text: "To carry out successfully."},
					{PartOfSpeech: "verb", Text: "To obtain, or gain as the result of exertion."},
				},
				Source: dictionary.SourceFreeDictionary,
			},
		},
	}
}

func newDictionaryTestAPI(t *testing.T) (*testAPI, *fakeDictionary) {
	fake := newFakeDictionary()
	ta := newTestAPI(t, fakeGoogle{}, func(d *Deps) { d.Dictionary = fake })
	ctx := context.Background()
	reset := func() {
		// Shared dictionary rows outlive DELETE FROM users: drop what the
		// provider wrote and put the starter words back as migration 00006 left them.
		for _, q := range []string{
			`DELETE FROM users`,
			`DELETE FROM dictionary_words WHERE owner_id IS NULL AND source <> 'intenli-starter' AND lemma IN ('resilient', 'give up')`,
			`DELETE FROM definitions WHERE word_id IN (SELECT id FROM dictionary_words WHERE lemma = 'achieve' AND owner_id IS NULL) AND position > 0`,
			`DELETE FROM word_audio WHERE word_id IN (SELECT id FROM dictionary_words WHERE lemma = 'achieve' AND owner_id IS NULL)`,
			`UPDATE dictionary_words SET source = 'intenli-starter', source_urls = '{}', phonetic_us = '/əˈtʃiːv/' WHERE lemma = 'achieve' AND owner_id IS NULL`,
		} {
			if _, err := ta.pool.Exec(ctx, q); err != nil {
				t.Fatal(err)
			}
		}
	}
	reset()
	t.Cleanup(reset)
	return ta, fake
}

func definitions(word resp) []map[string]any {
	var out []map[string]any
	meanings, _ := word.Body["meanings"].([]any)
	for _, m := range meanings {
		defs, _ := m.(map[string]any)["definitions"].([]any)
		for _, d := range defs {
			out = append(out, d.(map[string]any))
		}
	}
	return out
}

func TestLookupWord(t *testing.T) {
	ta, fake := newDictionaryTestAPI(t)
	ana := ta.signUp("ana@example.com", "es")
	ben := ta.signUp("ben@example.com", "en")

	ta.expectErr(ta.do("GET", "/v1/dictionary/words/resilient", "", nil), http.StatusUnauthorized, "unauthorized")

	word := ta.expect(ta.do("GET", "/v1/dictionary/words/%20Resilient%20", ana, nil), http.StatusOK)
	if word.str("word") != "resilient" || word.str("id") == "" {
		t.Fatalf("word = %s", word.Raw)
	}
	if ph := obj(word.Body, "phonetics"); ph["us"] != "/ɹɪˈzɪli.ənt/" || ph["uk"] != "/rɪˈzɪlɪənt/" {
		t.Errorf("phonetics = %v", ph)
	}
	audio, _ := word.Body["audio"].([]any)
	if len(audio) != 1 || audio[0].(map[string]any)["accent"] != "US" || audio[0].(map[string]any)["source"] != "PROVIDER" {
		t.Errorf("audio = %v", audio)
	}
	// Grouped by part of speech, in the order each first appears.
	meanings, _ := word.Body["meanings"].([]any)
	if len(meanings) != 2 || meanings[0].(map[string]any)["part_of_speech"] != "adjective" ||
		len(meanings[0].(map[string]any)["definitions"].([]any)) != 2 {
		t.Fatalf("meanings = %v", meanings)
	}
	first := definitions(word)[0]
	if first["text"] != "Able to recover quickly." || first["example"] != "Children are resilient." ||
		first["is_custom"] != false || first["saved"] != nil || first["synonyms"].([]any)[0] != "tough" {
		t.Errorf("first meaning = %v", first)
	}
	if at := obj(word.Body, "attribution"); at["source"] != "Wiktionary via Free Dictionary API" || at["license"] != "CC BY-SA 3.0" {
		t.Errorf("attribution = %v", at)
	}

	// Cached: the provider isn't asked again, by anyone.
	ta.expect(ta.do("GET", "/v1/dictionary/words/resilient", ben, nil), http.StatusOK)
	if fake.calls["resilient"] != 1 {
		t.Errorf("provider calls = %d, want 1", fake.calls["resilient"])
	}

	// Saving a meaning marks it as saved for that user only.
	deckID := ta.expect(ta.do("GET", "/v1/decks", ana, nil), http.StatusOK).data()[0]["id"]
	entry := ta.expect(ta.do("POST", "/v1/vocabulary", ana, map[string]any{
		"deck_id": deckID, "word_id": word.str("id"), "definition_id": first["id"],
	}), http.StatusCreated)
	// A meaning Ana writes herself shows up for her, after the dictionary's.
	ta.expect(ta.do("POST", "/v1/vocabulary", ana, map[string]any{
		"deck_id": deckID, "word": "Resilient", "definition": map[string]any{"text": "Mine", "part_of_speech": "noun"},
	}), http.StatusCreated)

	again := definitions(ta.expect(ta.do("GET", "/v1/dictionary/words/resilient", ana, nil), http.StatusOK))
	saved, _ := again[0]["saved"].(map[string]any)
	if saved["entry_id"] != entry.str("id") || saved["deck_id"] != deckID {
		t.Errorf("saved = %v", again[0])
	}
	if len(again) != 4 || again[3]["text"] != "Mine" || again[3]["is_custom"] != true {
		t.Errorf("own meaning = %v", again)
	}
	forBen := definitions(ta.expect(ta.do("GET", "/v1/dictionary/words/resilient", ben, nil), http.StatusOK))
	if len(forBen) != 3 || forBen[0]["saved"] != nil {
		t.Errorf("ben sees = %v", forBen)
	}

	// Phrases, unknown words, outages and junk.
	if w := ta.expect(ta.do("GET", "/v1/dictionary/words/Give%20%20up", ana, nil), http.StatusOK); w.str("word") != "give up" {
		t.Errorf("phrase = %s", w.Raw)
	}
	ta.expectErr(ta.do("GET", "/v1/dictionary/words/qwzx", ana, nil), http.StatusNotFound, "word_not_found")
	ta.expectErr(ta.do("GET", "/v1/dictionary/words/outage", ana, nil), http.StatusServiceUnavailable, "dictionary_unavailable")
	ta.expectErr(ta.do("GET", "/v1/dictionary/words/1234", ana, nil), http.StatusBadRequest, "invalid_word")
}

func TestLookupStarterWord(t *testing.T) {
	ta, fake := newDictionaryTestAPI(t)
	ana := ta.signUp("ana@example.com", "en")

	// "achieve" came with the starter deck holding one meaning; a search
	// adds the provider's meanings after it and keeps it saved.
	word := ta.expect(ta.do("GET", "/v1/dictionary/words/achieve", ana, nil), http.StatusOK)
	defs := definitions(word)
	if len(defs) != 3 || defs[0]["saved"] == nil || defs[1]["text"] != "To carry out successfully." || defs[1]["saved"] != nil {
		t.Fatalf("achieve = %s", word.Raw)
	}
	if ph := obj(word.Body, "phonetics"); ph["us"] != "/əˈtʃiv/" || ph["uk"] != "/əˈtʃiːv/" {
		t.Errorf("phonetics = %v", ph)
	}
	ta.expect(ta.do("GET", "/v1/dictionary/words/achieve", ana, nil), http.StatusOK)
	if fake.calls["achieve"] != 1 {
		t.Errorf("provider calls = %d, want 1", fake.calls["achieve"])
	}

	// When the provider is down, the starter meaning is still there.
	afford := ta.expect(ta.do("GET", "/v1/dictionary/words/afford", ana, nil), http.StatusOK)
	if d := definitions(afford); len(d) != 1 || obj(afford.Body, "attribution")["source"] != "Intenli" {
		t.Errorf("afford = %s", afford.Raw)
	}
}

func TestSearchHistory(t *testing.T) {
	ta, _ := newDictionaryTestAPI(t)
	ana := ta.signUp("ana@example.com", "en")
	ben := ta.signUp("ben@example.com", "en")

	for _, w := range []string{"resilient", "qwzx", "give%20up", "Resilient"} {
		ta.do("GET", "/v1/dictionary/words/"+w, ana, nil)
	}
	ta.expect(ta.do("GET", "/v1/dictionary/words/outage", ana, nil), http.StatusServiceUnavailable)

	hist := ta.expect(ta.do("GET", "/v1/dictionary/history", ana, nil), http.StatusOK).data()
	got := []string{}
	for _, h := range hist {
		got = append(got, h["query"].(string))
	}
	// Newest first, one row per word, outages not recorded.
	if len(got) != 3 || got[0] != "resilient" || got[1] != "give up" || got[2] != "qwzx" {
		t.Fatalf("history = %v", hist)
	}
	if hist[0]["found"] != true || hist[2]["found"] != false || hist[0]["searched_at"] == "" {
		t.Errorf("history = %v", hist)
	}
	if h := ta.expect(ta.do("GET", "/v1/dictionary/history?limit=1", ana, nil), http.StatusOK).data(); len(h) != 1 {
		t.Errorf("limit 1 = %v", h)
	}
	ta.expectErr(ta.do("GET", "/v1/dictionary/history?limit=0", ana, nil), http.StatusBadRequest, "invalid_request")
	if h := ta.expect(ta.do("GET", "/v1/dictionary/history", ben, nil), http.StatusOK).data(); len(h) != 0 {
		t.Errorf("ben's history = %v", h)
	}

	ta.expect(ta.do("DELETE", "/v1/dictionary/history", ana, nil), http.StatusNoContent)
	if h := ta.expect(ta.do("GET", "/v1/dictionary/history", ana, nil), http.StatusOK).data(); len(h) != 0 {
		t.Errorf("after clear = %v", h)
	}
}
