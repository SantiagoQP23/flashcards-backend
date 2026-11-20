package store

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/SantiagoQP23/flashcards-backend/internal/database"
	"github.com/SantiagoQP23/flashcards-backend/migrations"
)

// Skipped unless TEST_DATABASE_URL is set (CI sets it; locally use `make test-db`).
func TestUsers(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	if err := migrations.Up(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := database.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `DELETE FROM users`); err != nil {
		t.Fatal(err)
	}

	u, err := CreateUser(ctx, pool, NewUser{Email: "Ana@Example.com", PasswordHash: "hash", DisplayName: "Ana", Timezone: "America/Guayaquil", Locale: "es"})
	if err != nil {
		t.Fatal(err)
	}
	if u.DailyNewLimit != 20 || u.PreferredAccent != "US" || u.GoogleSub != "" {
		t.Fatalf("defaults not applied: %+v", u)
	}

	got, err := GetUserByEmail(ctx, pool, "ana@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != u.ID {
		t.Fatalf("lookup is not case-insensitive: got %v, want %v", got.ID, u.ID)
	}

	if _, err := CreateUser(ctx, pool, NewUser{Email: "ANA@example.com", PasswordHash: "hash", Timezone: "UTC", Locale: "en"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate email: err = %v, want ErrConflict", err)
	}
	if _, err := GetUserByEmail(ctx, pool, "nobody@example.com"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing user: err = %v, want ErrNotFound", err)
	}

	// Linking Google can drop an unproven password.
	linked, err := LinkGoogle(ctx, pool, u.ID, "google-123", true)
	if err != nil {
		t.Fatal(err)
	}
	if linked.PasswordHash != "" || !linked.EmailVerified || linked.GoogleSub != "google-123" {
		t.Fatalf("link did not apply: %+v", linked)
	}
	if got, err := GetUserByGoogleSub(ctx, pool, "google-123"); err != nil || got.ID != u.ID {
		t.Fatalf("by google sub: %+v %v", got, err)
	}

	// Partial update: only the given fields change; "" clears the name.
	empty, accent, limit := "", "UK", 5
	updated, err := UpdateUser(ctx, pool, u.ID, UserUpdate{DisplayName: &empty, PreferredAccent: &accent, DailyNewLimit: &limit})
	if err != nil {
		t.Fatal(err)
	}
	if updated.DisplayName != "" || updated.PreferredAccent != "UK" || updated.DailyNewLimit != 5 || updated.Timezone != "America/Guayaquil" {
		t.Fatalf("update: %+v", updated)
	}

	if err := DeleteUser(ctx, pool, u.ID); err != nil {
		t.Fatal(err)
	}
	if err := DeleteUser(ctx, pool, u.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: err = %v, want ErrNotFound", err)
	}
}
