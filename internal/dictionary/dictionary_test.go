package dictionary

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// The fixtures in testdata follow the providers' documented answer shapes,
// with entries cut down to a few meanings.

func fixture(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", path))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseFreeDictionary(t *testing.T) {
	tests := []struct {
		file   string
		check  func(t *testing.T, e Entry)
		hasErr error
	}{
		{file: "resilient.json", check: func(t *testing.T, e Entry) {
			if e.PhoneticUS != "/ɹɪˈzɪli.ənt/" || e.PhoneticUK != "/ɹɪˈzɪli.ənt/" {
				t.Errorf("phonetics = %q %q", e.PhoneticUS, e.PhoneticUK)
			}
			if len(e.Audio) != 1 || e.Audio[0].Accent != "US" || !strings.HasSuffix(e.Audio[0].URL, "resilient-us.mp3") {
				t.Errorf("audio = %v", e.Audio)
			}
			if len(e.Definitions) != 2 || e.Definitions[1].Example != "Children are often very resilient." {
				t.Fatalf("definitions = %+v", e.Definitions)
			}
			// Part-of-speech synonyms go on the first meaning, without duplicates.
			if !reflect.DeepEqual(e.Definitions[0].Synonyms, []string{"elastic", "tough"}) ||
				!reflect.DeepEqual(e.Definitions[0].Antonyms, []string{"brittle"}) ||
				len(e.Definitions[1].Synonyms) != 0 {
				t.Errorf("synonyms = %v / %v", e.Definitions[0].Synonyms, e.Definitions[1].Synonyms)
			}
			if e.Source != SourceFreeDictionary || !reflect.DeepEqual(e.SourceURLs, []string{"https://en.wiktionary.org/wiki/resilient"}) {
				t.Errorf("source = %q %v", e.Source, e.SourceURLs)
			}
		}},
		{file: "hello.json", check: func(t *testing.T, e Entry) {
			// The UK recording has its own IPA; the US one has none, so the
			// first IPA stands in. The Australian recording is skipped.
			if e.PhoneticUK != "/həˈləʊ/" || e.PhoneticUS != "/həˈləʊ/" {
				t.Errorf("phonetics = %q %q", e.PhoneticUS, e.PhoneticUK)
			}
			if len(e.Audio) != 1 || e.Audio[0].Accent != "UK" {
				t.Errorf("audio = %v", e.Audio)
			}
			got := []string{}
			for _, d := range e.Definitions {
				got = append(got, d.PartOfSpeech)
			}
			if !reflect.DeepEqual(got, []string{"noun", "verb", "interjection", "interjection"}) {
				t.Errorf("parts of speech = %v", got)
			}
		}},
		{file: "bank.json", check: func(t *testing.T, e Entry) {
			// Two etymologies, kept in order.
			if len(e.Definitions) != 3 || e.Definitions[2].Text != "An edge of river, lake, or other watercourse." {
				t.Errorf("definitions = %+v", e.Definitions)
			}
			if e.PhoneticUS != "/bæŋk/" || e.PhoneticUK != "/baŋk/" || len(e.Audio) != 2 ||
				e.Audio[0].Accent != "US" || e.Audio[1].Accent != "UK" {
				t.Errorf("phonetics = %q %q %v", e.PhoneticUS, e.PhoneticUK, e.Audio)
			}
			if len(e.SourceURLs) != 1 {
				t.Errorf("source urls = %v", e.SourceURLs)
			}
		}},
		{file: "serendipity.json", check: func(t *testing.T, e Entry) {
			if e.PhoneticUS != "" || e.PhoneticUK != "" || len(e.Audio) != 0 || e.Audio == nil {
				t.Errorf("want no phonetics and an empty audio list: %+v", e)
			}
		}},
		{file: "colour.json", check: func(t *testing.T, e Entry) {
			if len(e.Audio) != 1 || e.Audio[0].Accent != "UK" ||
				e.Audio[0].URL != "https://ssl.gstatic.com/dictionary/static/sounds/20200429/colour--_gb_1.mp3" {
				t.Errorf("audio = %v", e.Audio)
			}
		}},
		{file: "give-up.json", check: func(t *testing.T, e Entry) {
			d := e.Definitions[0]
			if d.PartOfSpeech != "phrase" || d.Example != "Don't give up now." || d.Synonyms[0] != "quit" {
				t.Errorf("definition = %+v", d)
			}
		}},
		{file: "cheers.json", check: func(t *testing.T, e Entry) {
			if e.Definitions[0].PartOfSpeech != "interjection" || e.Definitions[1].PartOfSpeech != "" {
				t.Errorf("parts of speech = %q %q", e.Definitions[0].PartOfSpeech, e.Definitions[1].PartOfSpeech)
			}
			if len(e.Audio) != 0 {
				t.Errorf("audio = %v", e.Audio)
			}
		}},
		{file: "empty-meanings.json", hasErr: ErrNotFound},
		{file: "not-found.json", hasErr: ErrUnavailable},
		{file: "html-error.txt", hasErr: ErrUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			e, err := ParseFreeDictionary(fixture(t, "freedictionary/"+tt.file))
			if tt.hasErr != nil {
				if !errors.Is(err, tt.hasErr) {
					t.Fatalf("err = %v, want %v", err, tt.hasErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, e)
		})
	}
}

func TestParseMerriamWebster(t *testing.T) {
	e, err := ParseMerriamWebster("resilient", fixture(t, "merriamwebster/resilient.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Definitions) != 2 {
		t.Fatalf("definitions = %+v", e.Definitions)
	}
	d := e.Definitions[0]
	if d.PartOfSpeech != "adjective" || d.Text != "able to become strong, healthy, or successful again after something bad happens" ||
		d.Example != "Children are often very resilient." {
		t.Errorf("first = %+v", d)
	}
	if e.Definitions[1].Text != "able to return to an original shape after being pulled, stretched, pressed, bent, etc." {
		t.Errorf("cross reference not removed: %q", e.Definitions[1].Text)
	}
	if e.PhoneticUS != "/rɪˈzɪljənt/" || e.PhoneticUK != "/rɪˈzɪljənt/" {
		t.Errorf("phonetics = %q %q", e.PhoneticUS, e.PhoneticUK)
	}
	if len(e.Audio) != 1 || e.Audio[0].URL != "https://media.merriam-webster.com/audio/prons/en/us/mp3/r/resili02.mp3" {
		t.Errorf("audio = %v", e.Audio)
	}
	if e.Source != SourceMerriamWebster || e.SourceURLs[0] != "https://www.learnersdictionary.com/definition/resilient" {
		t.Errorf("source = %q %v", e.Source, e.SourceURLs)
	}

	// An inflected form finds its headword through the stems; nested sense
	// sequences are read in order.
	ran, err := ParseMerriamWebster("ran", fixture(t, "merriamwebster/ran.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ran.Definitions) != 2 || ran.Definitions[1].Text != "to go quickly" || ran.Definitions[1].Example != "She ran to the store." {
		t.Errorf("ran = %+v", ran.Definitions)
	}

	if _, err := ParseMerriamWebster("resilent", fixture(t, "merriamwebster/suggestions.json")); !errors.Is(err, ErrNotFound) {
		t.Errorf("suggestions err = %v", err)
	}
	if _, err := ParseMerriamWebster("x", fixture(t, "merriamwebster/bad-key.txt")); !errors.Is(err, ErrUnavailable) {
		t.Errorf("bad key err = %v", err)
	}
}

func TestMWAudioURL(t *testing.T) {
	for name, want := range map[string]string{
		"bixen01": "bix/bixen01", "ggmo0001": "gg/ggmo0001", "3d000001": "number/3d000001", "hello001": "h/hello001",
	} {
		if got := mwAudioURL(name); got != "https://media.merriam-webster.com/audio/prons/en/us/mp3/"+want+".mp3" {
			t.Errorf("%s: %s", name, got)
		}
	}
}

func TestFreeDictionaryHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/api/v2/entries/en/resilient":
			_, _ = w.Write(fixture(t, "freedictionary/resilient.json"))
		case "/api/v2/entries/en/give%20up":
			_, _ = w.Write(fixture(t, "freedictionary/give-up.json"))
		case "/api/v2/entries/en/qwzx":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write(fixture(t, "freedictionary/not-found.json"))
		case "/api/v2/entries/en/slow":
			time.Sleep(200 * time.Millisecond)
		default:
			w.WriteHeader(http.StatusBadGateway)
		}
	}))
	defer srv.Close()
	p := &FreeDictionary{BaseURL: srv.URL, Client: newHTTPClient(100 * time.Millisecond)}
	ctx := context.Background()

	if e, err := p.Lookup(ctx, "resilient"); err != nil || len(e.Definitions) != 2 {
		t.Errorf("resilient: %v %v", e, err)
	}
	if _, err := p.Lookup(ctx, "give up"); err != nil {
		t.Errorf("phrase: %v", err)
	}
	if _, err := p.Lookup(ctx, "qwzx"); !errors.Is(err, ErrNotFound) {
		t.Errorf("404 err = %v", err)
	}
	if _, err := p.Lookup(ctx, "down"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("502 err = %v", err)
	}
	if _, err := p.Lookup(ctx, "slow"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("timeout err = %v", err)
	}
}

type stub struct {
	e     Entry
	err   error
	calls int
}

func (s *stub) Lookup(context.Context, string) (Entry, error) { s.calls++; return s.e, s.err }

func TestWithFallback(t *testing.T) {
	ctx := context.Background()
	backup := &stub{e: Entry{Source: "backup"}}

	down := &stub{err: ErrUnavailable}
	if e, err := WithFallback(down, backup).Lookup(ctx, "w"); err != nil || e.Source != "backup" {
		t.Errorf("outage: %v %v", e, err)
	}
	missing := &stub{err: ErrNotFound}
	if _, err := WithFallback(missing, backup).Lookup(ctx, "w"); !errors.Is(err, ErrNotFound) || backup.calls != 1 {
		t.Errorf("not found should not use the fallback: %v, %d calls", err, backup.calls)
	}
	if p := WithFallback(down, nil); p != Provider(down) {
		t.Error("nil fallback should return the primary")
	}
}

func TestNormalizePartOfSpeech(t *testing.T) {
	for raw, want := range map[string]string{
		"Noun": "noun", "transitive verb": "verb", "phrasal verb": "phrase", "idiom": "phrase",
		"exclamation": "interjection", "abbreviation": "", "": "", "noun phrase": "phrase",
	} {
		if got := normalizePartOfSpeech(raw); got != want {
			t.Errorf("%q = %q, want %q", raw, got, want)
		}
	}
}
