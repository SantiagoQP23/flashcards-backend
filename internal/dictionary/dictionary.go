// Package dictionary looks words up in outside dictionaries and turns their
// answers into one shape, so the rest of the API doesn't depend on which
// provider answered.
package dictionary

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

var (
	// ErrNotFound means the provider answered and doesn't know the word.
	ErrNotFound = errors.New("word not found")
	// ErrUnavailable means the provider didn't answer in time, failed (5xx,
	// network) or sent something we can't read.
	ErrUnavailable = errors.New("dictionary unavailable")
)

// DefaultTimeout is how long one provider gets to answer.
const DefaultTimeout = 3 * time.Second

// Provider looks up one English word or phrase. The word is already trimmed
// and lowercased. It returns ErrNotFound or an error wrapping ErrUnavailable.
type Provider interface {
	Lookup(ctx context.Context, word string) (Entry, error)
}

// Entry is a provider's answer, normalised.
type Entry struct {
	PhoneticUS string
	PhoneticUK string
	Audio      []Audio
	// Definitions are in the provider's order, which puts the most common
	// meanings first.
	Definitions []Definition
	// Source names the provider, as stored in dictionary_words.source.
	Source     string
	SourceURLs []string
}

// Audio is a recorded pronunciation.
type Audio struct {
	Accent string // "US" or "UK"
	URL    string
}

// Definition is one meaning.
type Definition struct {
	PartOfSpeech string // "" or one of PartsOfSpeech
	Text         string
	Example      string
	Synonyms     []string
	Antonyms     []string
}

// Sources are the values of Entry.Source.
const (
	SourceFreeDictionary = "free-dictionary"
	SourceMerriamWebster = "merriam-webster-learners"
)

// PartsOfSpeech are the parts of speech the API returns. Anything else a
// provider sends becomes "".
var PartsOfSpeech = []string{
	"noun", "verb", "adjective", "adverb", "pronoun", "preposition",
	"conjunction", "interjection", "phrase",
}

// normalizePartOfSpeech maps a provider's label to one of PartsOfSpeech.
func normalizePartOfSpeech(raw string) string {
	pos := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case pos == "":
		return ""
	case strings.Contains(pos, "phrase"), pos == "idiom", strings.HasPrefix(pos, "phrasal"):
		return "phrase"
	case pos == "exclamation":
		return "interjection"
	}
	for _, p := range PartsOfSpeech {
		if pos == p {
			return p
		}
	}
	// "noun, plural", "transitive verb", "auxiliary verb"...
	for _, p := range PartsOfSpeech {
		if strings.Contains(pos, p) {
			return p
		}
	}
	return ""
}

// WithFallback asks primary first and, when it is unavailable (timeout,
// 5xx, network), asks fallback. A "not found" from primary is the answer: the
// fallback is paid, so it only covers outages. A nil fallback returns primary.
func WithFallback(primary, fallback Provider) Provider {
	if fallback == nil {
		return primary
	}
	return fallbackProvider{primary, fallback}
}

type fallbackProvider struct{ primary, fallback Provider }

func (f fallbackProvider) Lookup(ctx context.Context, word string) (Entry, error) {
	e, err := f.primary.Lookup(ctx, word)
	if errors.Is(err, ErrUnavailable) && ctx.Err() == nil {
		return f.fallback.Lookup(ctx, word)
	}
	return e, err
}

// newHTTPClient is the client providers use: no request runs past timeout.
func newHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &http.Client{Timeout: timeout}
}

// cleanList trims, drops empties and duplicates, keeping order.
func cleanList(items ...[]string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, list := range items {
		for _, s := range list {
			s = strings.TrimSpace(s)
			if s == "" || seen[strings.ToLower(s)] {
				continue
			}
			seen[strings.ToLower(s)] = true
			out = append(out, s)
		}
	}
	return out
}
