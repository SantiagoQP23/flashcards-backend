package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// User is a row of users. PasswordHash is empty for Google-only accounts and
// GoogleSub is empty until the user signs in with Google.
type User struct {
	ID              uuid.UUID
	Email           string
	EmailVerified   bool
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

const userColumns = `id, email, email_verified, coalesce(password_hash, ''), coalesce(google_sub, ''),
	coalesce(display_name, ''), timezone, locale, preferred_accent, daily_new_limit, created_at, updated_at`

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.EmailVerified, &u.PasswordHash, &u.GoogleSub, &u.DisplayName,
		&u.Timezone, &u.Locale, &u.PreferredAccent, &u.DailyNewLimit, &u.CreatedAt, &u.UpdatedAt)
	return u, notFound(err)
}

// NewUser is what sign-up knows about a new account. PasswordHash is empty
// for Google sign-up, which sets GoogleSub and EmailVerified instead.
type NewUser struct {
	Email         string
	EmailVerified bool
	PasswordHash  string
	GoogleSub     string
	DisplayName   string
	Timezone      string
	Locale        string
}

// CreateUser inserts a user; ErrConflict means the email (or Google
// account) is taken.
func CreateUser(ctx context.Context, db DB, n NewUser) (User, error) {
	u, err := scanUser(db.QueryRow(ctx, `
		INSERT INTO users (email, email_verified, password_hash, google_sub, display_name, timezone, locale)
		VALUES ($1, $2, nullif($3, ''), nullif($4, ''), nullif($5, ''), $6, $7)
		RETURNING `+userColumns,
		n.Email, n.EmailVerified, n.PasswordHash, n.GoogleSub, n.DisplayName, n.Timezone, n.Locale))
	if isUniqueViolation(err) {
		return User{}, ErrConflict
	}
	return u, err
}

func GetUserByID(ctx context.Context, db DB, id uuid.UUID) (User, error) {
	return scanUser(db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id))
}

// GetUserByEmail looks a user up by email, case-insensitively.
func GetUserByEmail(ctx context.Context, db DB, email string) (User, error) {
	return scanUser(db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE email = $1`, email))
}

// GetUserByEmailForUpdate locks the user row, for linking Google to it.
func GetUserByEmailForUpdate(ctx context.Context, db DB, email string) (User, error) {
	return scanUser(db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE email = $1 FOR UPDATE`, email))
}

func GetUserByGoogleSub(ctx context.Context, db DB, sub string) (User, error) {
	return scanUser(db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE google_sub = $1`, sub))
}

// LinkGoogle attaches a Google account to a user and marks the email as
// verified. dropPassword removes the password, for accounts whose email was
// never proven: whoever set that password may not own the address.
// ErrConflict means the Google account is linked to another user.
func LinkGoogle(ctx context.Context, db DB, id uuid.UUID, sub string, dropPassword bool) (User, error) {
	u, err := scanUser(db.QueryRow(ctx, `
		UPDATE users SET google_sub = $2, email_verified = true,
			password_hash = CASE WHEN $3 THEN NULL ELSE password_hash END,
			updated_at = now()
		WHERE id = $1
		RETURNING `+userColumns, id, sub, dropPassword))
	if isUniqueViolation(err) {
		return User{}, ErrConflict
	}
	return u, err
}

// UserUpdate holds the profile fields PATCH /v1/me can change; nil leaves a
// field as it is.
type UserUpdate struct {
	DisplayName     *string
	Timezone        *string
	Locale          *string
	PreferredAccent *string
	DailyNewLimit   *int
}

func UpdateUser(ctx context.Context, db DB, id uuid.UUID, u UserUpdate) (User, error) {
	return scanUser(db.QueryRow(ctx, `
		UPDATE users SET
			display_name     = CASE WHEN $2::text IS NULL THEN display_name ELSE nullif($2, '') END,
			timezone         = coalesce($3, timezone),
			locale           = coalesce($4, locale),
			preferred_accent = coalesce($5::accent, preferred_accent),
			daily_new_limit  = coalesce($6, daily_new_limit),
			updated_at       = now()
		WHERE id = $1
		RETURNING `+userColumns,
		id, u.DisplayName, u.Timezone, u.Locale, u.PreferredAccent, u.DailyNewLimit))
}

// DeleteUser removes the user and, through ON DELETE CASCADE, everything
// they own.
func DeleteUser(ctx context.Context, db DB, id uuid.UUID) error {
	tag, err := db.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
