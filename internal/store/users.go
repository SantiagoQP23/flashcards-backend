package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// User is a row of users. PasswordHash is empty for Google-only accounts.
type User struct {
	ID              uuid.UUID
	Email           string
	PasswordHash    string
	GoogleSub       string
	DisplayName     string
	Timezone        string
	Locale          string
	PreferredAccent string
	DailyNewLimit   int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

const userColumns = `id, email, coalesce(password_hash, ''), coalesce(google_sub, ''), coalesce(display_name, ''),
	timezone, locale, preferred_accent, daily_new_limit, created_at, updated_at`

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.GoogleSub, &u.DisplayName,
		&u.Timezone, &u.Locale, &u.PreferredAccent, &u.DailyNewLimit, &u.CreatedAt, &u.UpdatedAt)
	return u, notFound(err)
}

// CreateUser inserts a user with a password; ErrConflict means the email is taken.
func CreateUser(ctx context.Context, db DB, email, passwordHash, displayName, timezone, locale string) (User, error) {
	u, err := scanUser(db.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, display_name, timezone, locale)
		VALUES ($1, $2, nullif($3, ''), $4, $5)
		RETURNING `+userColumns, email, passwordHash, displayName, timezone, locale))
	if isUniqueViolation(err) {
		return User{}, ErrConflict
	}
	return u, err
}

// GetUserByEmail looks a user up by email, case-insensitively.
func GetUserByEmail(ctx context.Context, db DB, email string) (User, error) {
	return scanUser(db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE email = $1`, email))
}
