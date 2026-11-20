package dictionary

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// FreeDictionary is https://dictionaryapi.dev, a free API over Wiktionary
// (CC BY-SA). It has no SLA, so answers are cached forever and a paid
// provider covers its outages.
type FreeDictionary struct {
	BaseURL string // default https://api.dictionaryapi.dev
	Client  *http.Client
}

// NewFreeDictionary returns the provider with a per-request timeout.
func NewFreeDictionary(timeout time.Duration) *FreeDictionary {
	return &FreeDictionary{BaseURL: "https://api.dictionaryapi.dev", Client: newHTTPClient(timeout)}
}

func (p *FreeDictionary) Lookup(ctx context.Context, word string) (Entry, error) {
	u := p.BaseURL + "/api/v2/entries/en/" + url.PathEscape(word)
	body, status, err := get(ctx, p.Client, u)
	if err != nil {
		return Entry{}, err
	}
	if status == http.StatusNotFound {
		return Entry{}, ErrNotFound
	}
	if status != http.StatusOK {
		return Entry{}, fmt.Errorf("%w: free dictionary answered %d", ErrUnavailable, status)
	}
	return ParseFreeDictionary(body)
}

// get fetches u and reads at most 2 MB of the body. Network errors and
// timeouts wrap ErrUnavailable, and so does any 5xx or 429.
func get(ctx context.Context, client *http.Client, u string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return nil, 0, fmt.Errorf("%w: read body: %v", ErrUnavailable, err)
	}
	if res.StatusCode >= 500 || res.StatusCode == http.StatusTooManyRequests {
		return nil, res.StatusCode, fmt.Errorf("%w: answered %d", ErrUnavailable, res.StatusCode)
	}
	return body, res.StatusCode, nil
}

type fdEntry struct {
	Word      string `json:"word"`
	Phonetic  string `json:"phonetic"`
	Phonetics []struct {
		Text  string `json:"text"`
		Audio string `json:"audio"`
	} `json:"phonetics"`
	Meanings []struct {
		PartOfSpeech string `json:"partOfSpeech"`
		Definitions  []struct {
			Definition string   `json:"definition"`
			Example    string   `json:"example"`
			Synonyms   []string `json:"synonyms"`
			Antonyms   []string `json:"antonyms"`
		} `json:"definitions"`
		Synonyms []string `json:"synonyms"`
		Antonyms []string `json:"antonyms"`
	} `json:"meanings"`
	SourceURLs []string `json:"sourceUrls"`
}

// maxDefinitions caps a word's meanings ("set" has over a hundred); the
// first ones are the common ones.
const maxDefinitions = 40

// ParseFreeDictionary normalises a 200 answer: an array of entries, one per
// etymology. Meanings keep the provider's order across entries. Synonyms the
// provider lists for a whole part of speech are added to the first meaning
// of that part of speech, which is the one they usually fit.
func ParseFreeDictionary(body []byte) (Entry, error) {
	var raw []fdEntry
	if err := json.Unmarshal(body, &raw); err != nil {
		return Entry{}, fmt.Errorf("%w: free dictionary JSON: %v", ErrUnavailable, err)
	}
	e := Entry{Source: SourceFreeDictionary, Audio: []Audio{}, Definitions: []Definition{}}
	var anyPhonetic string
	audio := map[string]string{}
	for _, r := range raw {
		if anyPhonetic == "" {
			anyPhonetic = strings.TrimSpace(r.Phonetic)
		}
		for _, ph := range r.Phonetics {
			text := strings.TrimSpace(ph.Text)
			if anyPhonetic == "" {
				anyPhonetic = text
			}
			accent := audioAccent(ph.Audio)
			if accent == "" {
				continue
			}
			if audio[accent] == "" {
				audio[accent] = absoluteURL(ph.Audio)
			}
			if accent == "US" && e.PhoneticUS == "" {
				e.PhoneticUS = text
			}
			if accent == "UK" && e.PhoneticUK == "" {
				e.PhoneticUK = text
			}
		}
		for _, m := range r.Meanings {
			pos := normalizePartOfSpeech(m.PartOfSpeech)
			for i, d := range m.Definitions {
				text := strings.TrimSpace(d.Definition)
				if text == "" || len(e.Definitions) >= maxDefinitions {
					continue
				}
				def := Definition{
					PartOfSpeech: pos,
					Text:         text,
					Example:      strings.TrimSpace(d.Example),
					Synonyms:     cleanList(d.Synonyms),
					Antonyms:     cleanList(d.Antonyms),
				}
				if i == 0 {
					def.Synonyms = cleanList(d.Synonyms, m.Synonyms)
					def.Antonyms = cleanList(d.Antonyms, m.Antonyms)
				}
				e.Definitions = append(e.Definitions, def)
			}
		}
		e.SourceURLs = cleanList(e.SourceURLs, r.SourceURLs)
	}
	if len(e.Definitions) == 0 {
		return Entry{}, ErrNotFound
	}
	// An IPA with no recording has no known accent; it's still better than
	// nothing for either one.
	if e.PhoneticUS == "" {
		e.PhoneticUS = anyPhonetic
	}
	if e.PhoneticUK == "" {
		e.PhoneticUK = anyPhonetic
	}
	for _, accent := range []string{"US", "UK"} {
		if audio[accent] != "" {
			e.Audio = append(e.Audio, Audio{Accent: accent, URL: audio[accent]})
		}
	}
	return e, nil
}

// audioAccent reads the accent from a Free Dictionary audio URL:
// ".../hello-us.mp3", ".../hello-uk.mp3", or Google's older
// ".../hello--_gb_1.mp3". Other accents (au, ca) and unknown files give "".
func audioAccent(u string) string {
	name := strings.ToLower(u[strings.LastIndex(u, "/")+1:])
	switch {
	case name == "":
		return ""
	case strings.HasSuffix(name, "-us.mp3"), strings.Contains(name, "_us_"):
		return "US"
	case strings.HasSuffix(name, "-uk.mp3"), strings.Contains(name, "_gb_"):
		return "UK"
	}
	return ""
}

func absoluteURL(u string) string {
	u = strings.TrimSpace(u)
	if strings.HasPrefix(u, "//") {
		return "https:" + u
	}
	return u
}
