package httpapi

import (
	"time"

	"github.com/google/uuid"

	"github.com/SantiagoQP23/flashcards-backend/internal/store"
)

type userDTO struct {
	ID              uuid.UUID `json:"id"`
	Email           string    `json:"email"`
	DisplayName     string    `json:"display_name"`
	HasPassword     bool      `json:"has_password"`
	HasGoogle       bool      `json:"has_google"`
	Timezone        string    `json:"timezone"`
	Locale          string    `json:"locale"`
	PreferredAccent string    `json:"preferred_accent"`
	DailyNewLimit   int       `json:"daily_new_limit"`
	CreatedAt       time.Time `json:"created_at"`
}

func toUserDTO(u store.User) userDTO {
	return userDTO{
		ID:              u.ID,
		Email:           u.Email,
		DisplayName:     u.DisplayName,
		HasPassword:     u.PasswordHash != "",
		HasGoogle:       u.GoogleSub != "",
		Timezone:        u.Timezone,
		Locale:          u.Locale,
		PreferredAccent: u.PreferredAccent,
		DailyNewLimit:   u.DailyNewLimit,
		CreatedAt:       u.CreatedAt,
	}
}

type sessionDTO struct {
	User         userDTO `json:"user"`
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	TokenType    string  `json:"token_type"`
	ExpiresIn    int     `json:"expires_in"` // access token lifetime, seconds
}

type listDTO[T any] struct {
	Data []T `json:"data"`
}

type deckDTO struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	EntryCount int       `json:"entry_count"`
	DueCount   int       `json:"due_count"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func toDeckDTO(d store.Deck) deckDTO {
	return deckDTO{
		ID:         d.ID,
		Name:       d.Name,
		EntryCount: d.EntryCount,
		DueCount:   d.DueCount,
		CreatedAt:  d.CreatedAt,
		UpdatedAt:  d.UpdatedAt,
	}
}

type entryDTO struct {
	ID        uuid.UUID `json:"id"`
	DeckID    uuid.UUID `json:"deck_id"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Word      struct {
		ID         uuid.UUID `json:"id"`
		Text       string    `json:"text"`
		PhoneticUS string    `json:"phonetic_us"`
		PhoneticUK string    `json:"phonetic_uk"`
	} `json:"word"`
	Definition struct {
		ID           uuid.UUID `json:"id"`
		PartOfSpeech string    `json:"part_of_speech"`
		Text         string    `json:"text"`
		Example      string    `json:"example"`
		Synonyms     []string  `json:"synonyms"`
		IsCustom     bool      `json:"is_custom"`
	} `json:"definition"`
	Flashcard struct {
		ID    uuid.UUID `json:"id"`
		State string    `json:"state"`
		Due   time.Time `json:"due"`
	} `json:"flashcard"`
}

func toEntryDTO(e store.Entry) entryDTO {
	var d entryDTO
	d.ID, d.DeckID, d.Note, d.CreatedAt, d.UpdatedAt = e.ID, e.DeckID, e.Note, e.CreatedAt, e.UpdatedAt
	d.Word.ID, d.Word.Text, d.Word.PhoneticUS, d.Word.PhoneticUK = e.Word.ID, e.Word.Lemma, e.Word.PhoneticUS, e.Word.PhoneticUK
	d.Definition.ID = e.Definition.ID
	d.Definition.PartOfSpeech = e.Definition.PartOfSpeech
	d.Definition.Text = e.Definition.Text
	d.Definition.Example = e.Definition.Example
	d.Definition.Synonyms = e.Definition.Synonyms
	if d.Definition.Synonyms == nil {
		d.Definition.Synonyms = []string{}
	}
	d.Definition.IsCustom = e.Definition.Custom
	d.Flashcard.ID, d.Flashcard.State, d.Flashcard.Due = e.Flashcard.ID, e.Flashcard.State, e.Flashcard.Due
	return d
}

type entryPageDTO struct {
	Data       []entryDTO `json:"data"`
	NextCursor *string    `json:"next_cursor"` // null on the last page
}
