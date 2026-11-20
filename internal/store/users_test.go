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
	if _, err := pool.Exec(ctx, `TRUNCATE users CASCADE`); err != nil {
		t.Fatal(err)
	}

	u, err := CreateUser(ctx, pool, "Ana@Example.com", "hash", "Ana", "America/Guayaquil", "es")
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

	if _, err := CreateUser(ctx, pool, "ANA@example.com", "hash", "", "UTC", "en"); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate email: err = %v, want ErrConflict", err)
	}
	if _, err := GetUserByEmail(ctx, pool, "nobody@example.com"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing user: err = %v, want ErrNotFound", err)
	}
}
