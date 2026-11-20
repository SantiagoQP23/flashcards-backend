package db

import (
	"context"
	"os"
	"testing"
)

// Runs against the database in DATABASE_URL (CI provides one); skipped when it is not set.
func TestMigrateAndQuery(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	// Applying twice is a no-op.
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := New(tx)
	var id [16]byte
	if err := tx.QueryRow(ctx, `INSERT INTO users (email) VALUES ('test@example.com') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	user, err := q.GetUserByEmail(ctx, "test@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if user.Timezone != "UTC" || user.DailyNewLimit != 20 || user.PreferredAccent != AccentUS {
		t.Fatalf("defaults not applied: %+v", user)
	}
}
