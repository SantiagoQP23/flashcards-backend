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
