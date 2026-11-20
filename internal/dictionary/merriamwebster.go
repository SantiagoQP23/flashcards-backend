package dictionary

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// MerriamWebster is Merriam-Webster's Learner's Dictionary API
// (dictionaryapi.com), the paid fallback for when Free Dictionary is down.
// It is written for people learning English, gives IPA and a US recording,
// and needs an API key.
type MerriamWebster struct {
	BaseURL string // default https://www.dictionaryapi.com
	Key     string
	Client  *http.Client
}

// NewMerriamWebster returns the provider, or nil when key is empty (no
// fallback configured).
func NewMerriamWebster(key string, timeout time.Duration) *MerriamWebster {
	if key == "" {
		return nil
	}
	return &MerriamWebster{BaseURL: "https://www.dictionaryapi.com", Key: key, Client: newHTTPClient(timeout)}
}

func (p *MerriamWebster) Lookup(ctx context.Context, word string) (Entry, error) {
	u := p.BaseURL + "/api/v3/references/learners/json/" + url.PathEscape(word) + "?key=" + url.QueryEscape(p.Key)
	body, status, err := get(ctx, p.Client, u)
	if err != nil {
		return Entry{}, err
	}
	if status != http.StatusOK {
		return Entry{}, fmt.Errorf("%w: merriam-webster answered %d", ErrUnavailable, status)
	}
	return ParseMerriamWebster(word, body)
}

type mwEntry struct {
	Meta struct {
		ID    string   `json:"id"`
		Stems []string `json:"stems"`
	} `json:"meta"`
	HWI struct {
		Prs []mwPron `json:"prs"`
	} `json:"hwi"`
	FL       string            `json:"fl"`
	Def      []json.RawMessage `json:"def"`
	Shortdef []string          `json:"shortdef"`
}

type mwPron struct {
	IPA   string `json:"ipa"`
	Label string `json:"l"`
	Sound struct {
		Audio string `json:"audio"`
	} `json:"sound"`
}

// ParseMerriamWebster normalises a Learner's answer for word. When the
// dictionary doesn't know a word it answers 200 with spelling suggestions
// (an array of strings), which is ErrNotFound here. A bad key answers plain
// text, which is ErrUnavailable.
func ParseMerriamWebster(word string, body []byte) (Entry, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(body, &items); err != nil {
		return Entry{}, fmt.Errorf("%w: merriam-webster JSON: %v", ErrUnavailable, err)
	}
	var entries []mwEntry
	for _, it := range items {
		var e mwEntry
		if json.Unmarshal(it, &e) == nil && e.Meta.ID != "" {
			entries = append(entries, e)
		}
	}
	// The answer also lists nearby headwords ("resilience" for
	// "resilient"). Keep the word's own entries, or, for an inflected form
	// ("ran"), the entries that list it among their forms.
	var mine []mwEntry
	for _, e := range entries {
		if strings.EqualFold(headword(e.Meta.ID), word) {
			mine = append(mine, e)
		}
	}
	if len(mine) == 0 {
		for _, e := range entries {
			for _, s := range e.Meta.Stems {
				if strings.EqualFold(s, word) {
					mine = append(mine, e)
					break
				}
			}
		}
	}

	out := Entry{
		Source:      SourceMerriamWebster,
		SourceURLs:  []string{"https://www.learnersdictionary.com/definition/" + url.PathEscape(word)},
		Audio:       []Audio{},
		Definitions: []Definition{},
	}
	for _, e := range mine {
		for _, pr := range e.HWI.Prs {
			ipa := strings.TrimSpace(pr.IPA)
			british := strings.Contains(strings.ToLower(pr.Label), "brit")
			if ipa != "" && british && out.PhoneticUK == "" {
				out.PhoneticUK = "/" + ipa + "/"
			}
			if ipa != "" && !british && out.PhoneticUS == "" {
				out.PhoneticUS = "/" + ipa + "/"
			}
			if pr.Sound.Audio != "" && !british && len(out.Audio) == 0 {
				out.Audio = append(out.Audio, Audio{Accent: "US", URL: mwAudioURL(pr.Sound.Audio)})
			}
		}
		pos := normalizePartOfSpeech(e.FL)
		senses := mwSenses(e.Def)
		if len(senses) == 0 {
			for _, s := range e.Shortdef {
				senses = append(senses, Definition{Text: s})
			}
		}
		for _, d := range senses {
			d.Text = strings.TrimSpace(d.Text)
			if d.Text == "" || len(out.Definitions) >= maxDefinitions {
				continue
			}
			d.PartOfSpeech = pos
			d.Synonyms, d.Antonyms = []string{}, []string{}
			out.Definitions = append(out.Definitions, d)
		}
	}
	if len(out.Definitions) == 0 {
		return Entry{}, ErrNotFound
	}
	if out.PhoneticUK == "" {
		out.PhoneticUK = out.PhoneticUS
	}
	return out, nil
}

// headword strips the homograph number: "run:1" -> "run".
func headword(id string) string {
	h, _, _ := strings.Cut(id, ":")
	return h
}

// mwAudioURL builds the recording's URL from its file name, following
// Merriam-Webster's rule for the folder.
func mwAudioURL(name string) string {
	var dir string
	switch {
	case strings.HasPrefix(name, "bix"):
		dir = "bix"
	case strings.HasPrefix(name, "gg"):
		dir = "gg"
	case name != "" && !unicode.IsLetter(rune(name[0])):
		dir = "number"
	default:
		dir = name[:1]
	}
	return "https://media.merriam-webster.com/audio/prons/en/us/mp3/" + dir + "/" + url.PathEscape(name) + ".mp3"
}

// mwSenses walks the "def" sense sequences and returns each sense's text and
// first example, in order.
func mwSenses(defs []json.RawMessage) []Definition {
	var out []Definition
	for _, raw := range defs {
		var d struct {
			Sseq [][][]json.RawMessage `json:"sseq"`
		}
		if json.Unmarshal(raw, &d) != nil {
			continue
		}
		for _, seq := range d.Sseq {
			for _, item := range seq {
				out = append(out, mwSenseItem(item)...)
			}
		}
	}
	return out
}

// mwSenseItem reads one ["sense", {...}], ["bs", {"sense": {...}}] or
// ["pseq", [...]] item.
func mwSenseItem(item []json.RawMessage) []Definition {
	if len(item) != 2 {
		return nil
	}
	var kind string
	if json.Unmarshal(item[0], &kind) != nil {
		return nil
	}
	switch kind {
	case "sense":
		if d, ok := mwSense(item[1]); ok {
			return []Definition{d}
		}
	case "bs":
		var bs struct {
			Sense json.RawMessage `json:"sense"`
		}
		if json.Unmarshal(item[1], &bs) == nil {
			if d, ok := mwSense(bs.Sense); ok {
				return []Definition{d}
			}
		}
	case "pseq":
		var seq [][]json.RawMessage
		if json.Unmarshal(item[1], &seq) != nil {
			return nil
		}
		var out []Definition
		for _, it := range seq {
			out = append(out, mwSenseItem(it)...)
		}
		return out
	}
	return nil
}

func mwSense(raw json.RawMessage) (Definition, bool) {
	var s struct {
		DT [][]json.RawMessage `json:"dt"`
	}
	if json.Unmarshal(raw, &s) != nil {
		return Definition{}, false
	}
	var d Definition
	for _, part := range s.DT {
		if len(part) != 2 {
			continue
		}
		var kind string
		if json.Unmarshal(part[0], &kind) != nil {
			continue
		}
		switch kind {
		case "text":
			var t string
			if json.Unmarshal(part[1], &t) == nil && d.Text == "" {
				d.Text = mwText(t)
			}
		case "vis":
			var vis []struct {
				T string `json:"t"`
			}
			if json.Unmarshal(part[1], &vis) == nil && len(vis) > 0 && d.Example == "" {
				d.Example = mwText(vis[0].T)
			}
		}
	}
	return d, d.Text != ""
}

var (
	mwCrossRef = regexp.MustCompile(`\{dx\}.*?\{/dx\}`)
	mwLink     = regexp.MustCompile(`\{(?:a_link|d_link|i_link|et_link|mat|sx|dxt)\|([^|}]*)[^}]*\}`)
	mwTag      = regexp.MustCompile(`\{[^{}]*\}`)
	mwSpaces   = regexp.MustCompile(`\s+`)
)

// mwText turns Merriam-Webster's markup ("{bc}able to {it}recover{/it}")
// into plain text.
func mwText(s string) string {
	s = mwCrossRef.ReplaceAllString(s, "")
	s = mwLink.ReplaceAllString(s, "$1")
	s = strings.NewReplacer("{ldquo}", "“", "{rdquo}", "”").Replace(s)
	s = mwTag.ReplaceAllString(s, "")
	s = mwSpaces.ReplaceAllString(s, " ")
	return strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(s), ":"))
}
