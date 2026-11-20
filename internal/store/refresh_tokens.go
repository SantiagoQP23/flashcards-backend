package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// RefreshToken is a row of refresh_tokens. Tokens of one sign-in share a
// family: each refresh rotates to a new token in the same family.
type RefreshToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	FamilyID  uuid.UUID
	ExpiresAt time.Time
	RevokedAt *time.Time
	// RotatedAt is set when the token was exchanged for a new one.
	RotatedAt *time.Time
}

func CreateRefreshToken(ctx context.Context, db DB, userID, familyID uuid.UUID, hash []byte, expiresAt time.Time) error {
	_, err := db.Exec(ctx,
		`INSERT INTO refresh_tokens (user_id, family_id, token_hash, expires_at) VALUES ($1, $2, $3, $4)`,
		userID, familyID, hash, expiresAt)
	return err
}

// GetRefreshTokenForUpdate locks the token row for rotation.
func GetRefreshTokenForUpdate(ctx context.Context, db DB, hash []byte) (RefreshToken, error) {
	var t RefreshToken
	err := db.QueryRow(ctx,
		`SELECT id, user_id, family_id, expires_at, revoked_at, rotated_at
		 FROM refresh_tokens WHERE token_hash = $1 FOR UPDATE`, hash).
		Scan(&t.ID, &t.UserID, &t.FamilyID, &t.ExpiresAt, &t.RevokedAt, &t.RotatedAt)
	return t, notFound(err)
}

// RotateRefreshToken revokes a token because it was exchanged for a new one.
func RotateRefreshToken(ctx context.Context, db DB, id uuid.UUID, at time.Time) error {
	_, err := db.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = $2, rotated_at = $2 WHERE id = $1 AND revoked_at IS NULL`, id, at)
	return err
}

// FamilyHasLiveToken reports whether a family still has an unrevoked,
// unexpired token (it wasn't signed out or revoked as a whole).
func FamilyHasLiveToken(ctx context.Context, db DB, familyID uuid.UUID, now time.Time) (bool, error) {
	var ok bool
	err := db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM refresh_tokens WHERE family_id = $1 AND revoked_at IS NULL AND expires_at > $2)`,
		familyID, now).Scan(&ok)
	return ok, err
}

func RevokeRefreshFamily(ctx context.Context, db DB, familyID uuid.UUID) error {
	_, err := db.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = now() WHERE family_id = $1 AND revoked_at IS NULL`, familyID)
	return err
}

func RevokeAllRefreshTokens(ctx context.Context, db DB, userID uuid.UUID) error {
	_, err := db.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	return err
}
