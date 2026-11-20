package httpapi

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

func (r resp) data() []map[string]any {
	items, _ := r.Body["data"].([]any)
	out := make([]map[string]any, len(items))
	for i, it := range items {
		out[i], _ = it.(map[string]any)
	}
	return out
}

func obj(v any, key string) map[string]any {
	m, _ := v.(map[string]any)[key].(map[string]any)
	return m
}

// signUp registers a user and returns its access token.
func (ta *testAPI) signUp(email, lang string) string {
	ta.t.Helper()
	reg := ta.expect(ta.do("POST", "/v1/auth/register", "", map[string]any{
		"email": email, "password": "correct horse",
	}, "Accept-Language", lang), http.StatusCreated)
	return reg.str("access_token")
}

func TestStarterDeck(t *testing.T) {
	ta := newTestAPI(t, fakeGoogle{})
	es := ta.signUp("es@example.com", "es-EC")
	en := ta.signUp("en@example.com", "en-US")

	decks := ta.expect(ta.do("GET", "/v1/decks", es, nil), http.StatusOK).data()
	if len(decks) != 1 || decks[0]["name"] != "Vocabulario diario" ||
		decks[0]["entry_count"] != float64(20) || decks[0]["due_count"] != float64(20) {
		t.Fatalf("es decks = %v", decks)
	}
	enDecks := ta.expect(ta.do("GET", "/v1/decks", en, nil), http.StatusOK).data()
	if len(enDecks) != 1 || enDecks[0]["name"] != "Daily Vocabulary" {
		t.Fatalf("en decks = %v", enDecks)
	}

	// Newest first, in the starter order: "achieve" is position 1.
	page := ta.expect(ta.do("GET", "/v1/vocabulary?limit=5", es, nil), http.StatusOK)
	items := page.data()
	if len(items) != 5 || obj(items[0], "word")["text"] != "achieve" || obj(items[1], "word")["text"] != "afford" {
		t.Fatalf("first page = %v", page.Raw)
	}
	e := items[0]
	def, card := obj(e, "definition"), obj(e, "flashcard")
	if def["text"] == "" || def["is_custom"] != false || def["part_of_speech"] != "verb" ||
		card["state"] != "NEW" || obj(e, "word")["phonetic_us"] != "/əˈtʃiːv/" {
		t.Fatalf("entry = %v", e)
	}

	// Walk every page.
	seen := 0
	cursor := ""
	for i := 0; i < 10; i++ {
		p := ta.expect(ta.do("GET", "/v1/vocabulary?limit=7&cursor="+url.QueryEscape(cursor), es, nil), http.StatusOK)
		seen += len(p.data())
		next, _ := p.Body["next_cursor"].(string)
		if next == "" {
			break
		}
		cursor = next
	}
	if seen != 20 {
		t.Fatalf("paged through %d entries, want 20", seen)
	}

	// Each user gets their own entries over the same shared words.
	got := ta.expect(ta.do("GET", "/v1/vocabulary?q=CHIEV", en, nil), http.StatusOK).data()
	if len(got) != 1 || obj(got[0], "word")["text"] != "achieve" {
		t.Fatalf("search = %v", got)
	}
	if n := len(ta.expect(ta.do("GET", "/v1/vocabulary?q=%25", en, nil), http.StatusOK).data()); n != 0 {
		t.Fatalf("%% should match literally, got %d", n)
	}
}

func TestDecksCRUD(t *testing.T) {
	ta := newTestAPI(t, fakeGoogle{})
	tok := ta.signUp("ana@example.com", "en")
	other := ta.signUp("bob@example.com", "en")

	work := ta.expect(ta.do("POST", "/v1/decks", tok, map[string]any{"name": "  Work   words "}), http.StatusCreated)
	if work.str("name") != "Work words" || work.Body["entry_count"] != float64(0) {
		t.Fatalf("deck = %v", work.Body)
	}
	id := work.str("id")
	ta.expectErr(ta.do("POST", "/v1/decks", tok, map[string]any{"name": "WORK WORDS"}), http.StatusConflict, "deck_name_taken")
	ta.expectErr(ta.do("POST", "/v1/decks", tok, map[string]any{"name": "  "}), http.StatusBadRequest, "invalid_deck_name")
	ta.expect(ta.do("POST", "/v1/decks", other, map[string]any{"name": "Work words"}), http.StatusCreated)

	renamed := ta.expect(ta.do("PATCH", "/v1/decks/"+id, tok, map[string]any{"name": "Office"}), http.StatusOK)
	if renamed.str("name") != "Office" {
		t.Fatalf("renamed = %v", renamed.Body)
	}
	ta.expectErr(ta.do("PATCH", "/v1/decks/"+id, tok, map[string]any{"name": "Daily Vocabulary"}), http.StatusConflict, "deck_name_taken")
	ta.expectErr(ta.do("PATCH", "/v1/decks/"+id, other, map[string]any{"name": "Mine"}), http.StatusNotFound, "deck_not_found")
	ta.expectErr(ta.do("DELETE", "/v1/decks/"+id, other, nil), http.StatusNotFound, "deck_not_found")
	ta.expectErr(ta.do("DELETE", "/v1/decks/nope", tok, nil), http.StatusNotFound, "deck_not_found")

	// Deleting a deck takes its words with it, and its name is free again.
	starter := ta.expect(ta.do("GET", "/v1/decks", tok, nil), http.StatusOK).data()[0]["id"].(string)
	ta.expect(ta.do("DELETE", "/v1/decks/"+starter, tok, nil), http.StatusNoContent)
	ta.expectErr(ta.do("DELETE", "/v1/decks/"+starter, tok, nil), http.StatusNotFound, "deck_not_found")
	decks := ta.expect(ta.do("GET", "/v1/decks", tok, nil), http.StatusOK).data()
	if len(decks) != 1 || decks[0]["id"] != id {
		t.Fatalf("decks = %v", decks)
	}
	if n := len(ta.expect(ta.do("GET", "/v1/vocabulary", tok, nil), http.StatusOK).data()); n != 0 {
		t.Fatalf("entries after deleting the deck = %d", n)
	}
	ta.expect(ta.do("POST", "/v1/decks", tok, map[string]any{"name": "Daily Vocabulary"}), http.StatusCreated)

	ta.expectErr(ta.do("GET", "/v1/decks", "", nil), http.StatusUnauthorized, "unauthorized")
}

func TestSaveMoveAndRemove(t *testing.T) {
	ta := newTestAPI(t, fakeGoogle{})
	tok := ta.signUp("ana@example.com", "en")
	other := ta.signUp("bob@example.com", "en")
	starter := ta.expect(ta.do("GET", "/v1/decks", tok, nil), http.StatusOK).data()[0]["id"].(string)
	work := ta.expect(ta.do("POST", "/v1/decks", tok, map[string]any{"name": "Work"}), http.StatusCreated).str("id")

	first := ta.expect(ta.do("GET", "/v1/vocabulary?limit=1", tok, nil), http.StatusOK).data()[0]
	entryID := first["id"].(string)
	wordID := obj(first, "word")["id"].(string)
	defID := obj(first, "definition")["id"].(string)

	// The same meaning can't be saved twice, even in another deck.
	ta.expectErr(ta.do("POST", "/v1/vocabulary", tok, map[string]any{
		"word_id": wordID, "definition_id": defID, "deck_id": work,
	}), http.StatusConflict, "already_saved")
	ta.expectErr(ta.do("POST", "/v1/vocabulary", tok, map[string]any{
		"word_id": wordID, "definition_id": defID, "deck_id": "00000000-0000-0000-0000-000000000001",
	}), http.StatusNotFound, "deck_not_found")
	ta.expectErr(ta.do("POST", "/v1/vocabulary", tok, map[string]any{"word_id": wordID, "deck_id": work}),
		http.StatusBadRequest, "invalid_request")

	// Move to another deck and add a note.
	moved := ta.expect(ta.do("PATCH", "/v1/vocabulary/"+entryID, tok, map[string]any{
		"deck_id": work, "note": "from the starter deck",
	}), http.StatusOK)
	if moved.str("deck_id") != work || moved.str("note") != "from the starter deck" {
		t.Fatalf("moved = %v", moved.Body)
	}
	inWork := ta.expect(ta.do("GET", "/v1/vocabulary?deck_id="+work, tok, nil), http.StatusOK).data()
	if len(inWork) != 1 {
		t.Fatalf("work deck has %d entries", len(inWork))
	}
	if n := len(ta.expect(ta.do("GET", "/v1/vocabulary?deck_id="+starter, tok, nil), http.StatusOK).data()); n != 19 {
		t.Fatalf("starter deck has %d entries", n)
	}
	ta.expectErr(ta.do("PATCH", "/v1/vocabulary/"+entryID, other, map[string]any{"note": "x"}), http.StatusNotFound, "entry_not_found")
	ta.expectErr(ta.do("PATCH", "/v1/vocabulary/"+entryID, tok, map[string]any{
		"definition": map[string]any{"text": "mine now"},
	}), http.StatusConflict, "definition_not_editable")

	// Remove, then the meaning can be saved again.
	ta.expect(ta.do("DELETE", "/v1/vocabulary/"+entryID, tok, nil), http.StatusNoContent)
	ta.expectErr(ta.do("DELETE", "/v1/vocabulary/"+entryID, tok, nil), http.StatusNotFound, "entry_not_found")
	again := ta.expect(ta.do("POST", "/v1/vocabulary", tok, map[string]any{
		"word_id": wordID, "definition_id": defID, "deck_id": work,
	}), http.StatusCreated)
	if obj(again.Body, "flashcard")["state"] != "NEW" || obj(again.Body, "word")["text"] != "achieve" {
		t.Fatalf("saved again = %v", again.Body)
	}
}

func TestSaveWordManually(t *testing.T) {
	ta := newTestAPI(t, fakeGoogle{})
	tok := ta.signUp("ana@example.com", "en")
	other := ta.signUp("bob@example.com", "en")
	deck := ta.expect(ta.do("GET", "/v1/decks", tok, nil), http.StatusOK).data()[0]["id"].(string)
	otherDeck := ta.expect(ta.do("GET", "/v1/decks", other, nil), http.StatusOK).data()[0]["id"].(string)

	save := func(token, deckID, word string, def map[string]any) resp {
		return ta.do("POST", "/v1/vocabulary", token, map[string]any{"deck_id": deckID, "word": word, "definition": def})
	}

	// A word the dictionary doesn't know.
	e := ta.expect(save(tok, deck, "  Give   UP ", map[string]any{
		"text": "To stop trying.", "example": "Don't give up!", "part_of_speech": "Phrase",
	}), http.StatusCreated)
	def := obj(e.Body, "definition")
	if obj(e.Body, "word")["text"] != "give up" || def["text"] != "To stop trying." || def["example"] != "Don't give up!" ||
		def["part_of_speech"] != "phrase" || def["is_custom"] != true || obj(e.Body, "flashcard")["state"] != "NEW" {
		t.Fatalf("custom entry = %v", e.Raw)
	}
	entryID := e.str("id")

	// The user's own meaning of a word the starter deck already has.
	own := ta.expect(save(tok, deck, "achieve", map[string]any{"text": "lograr"}), http.StatusCreated)
	if obj(own.Body, "word")["phonetic_us"] != "/əˈtʃiːv/" || obj(own.Body, "definition")["is_custom"] != true {
		t.Fatalf("own meaning = %v", own.Raw)
	}

	// Another user typing the same word gets their own word and meaning;
	// they can't use Ana's.
	b := ta.expect(save(other, otherDeck, "give up", map[string]any{"text": "rendirse"}), http.StatusCreated)
	if obj(b.Body, "word")["id"] == obj(e.Body, "word")["id"] {
		t.Fatal("private words must not be shared between users")
	}
	ta.expectErr(ta.do("POST", "/v1/vocabulary", other, map[string]any{
		"deck_id": otherDeck, "word_id": obj(e.Body, "word")["id"], "definition_id": def["id"],
	}), http.StatusNotFound, "definition_not_found")

	// Validation.
	ta.expectErr(save(tok, deck, " ", map[string]any{"text": "x"}), http.StatusBadRequest, "invalid_word")
	ta.expectErr(save(tok, deck, "word", map[string]any{"text": " "}), http.StatusBadRequest, "invalid_definition")
	ta.expectErr(save(tok, deck, "word", map[string]any{"example": "only an example"}), http.StatusBadRequest, "invalid_definition")
	ta.expectErr(save(tok, deck, "word", map[string]any{"text": "x", "part_of_speech": "thing"}), http.StatusBadRequest, "invalid_request")
	ta.expectErr(ta.do("POST", "/v1/vocabulary", tok, map[string]any{
		"deck_id": deck, "word": "x", "definition": map[string]any{"text": "y"}, "word_id": obj(e.Body, "word")["id"],
	}), http.StatusBadRequest, "invalid_request")

	// Fix a typo in the meaning, and clear the example.
	fixed := ta.expect(ta.do("PATCH", "/v1/vocabulary/"+entryID, tok, map[string]any{
		"definition": map[string]any{"text": "To stop trying to do something.", "example": ""},
	}), http.StatusOK)
	if d := obj(fixed.Body, "definition"); d["text"] != "To stop trying to do something." || d["example"] != "" || d["part_of_speech"] != "phrase" {
		t.Fatalf("fixed = %v", fixed.Raw)
	}

	// Ana's list has both, newest first, and search finds the phrase.
	list := ta.expect(ta.do("GET", "/v1/vocabulary?q="+url.QueryEscape("give u"), tok, nil), http.StatusOK).data()
	if len(list) != 1 || list[0]["id"] != entryID {
		t.Fatalf("search = %v", list)
	}
	decks := ta.expect(ta.do("GET", "/v1/decks", tok, nil), http.StatusOK).data()
	if decks[0]["entry_count"] != float64(22) {
		t.Fatalf("deck = %v", decks[0])
	}

	// Deleting the account removes the private words too.
	ta.expect(ta.do("DELETE", "/v1/me", tok, nil), http.StatusNoContent)
	var n int
	if err := ta.pool.QueryRow(t.Context(), `SELECT count(*) FROM dictionary_words WHERE id = $1`,
		fmt.Sprint(obj(e.Body, "word")["id"])).Scan(&n); err != nil || n != 0 {
		t.Fatalf("private word left behind: n=%d err=%v", n, err)
	}
}
