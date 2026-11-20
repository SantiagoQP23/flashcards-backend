// Command migrate applies pending database migrations (migrations/*.sql) to DATABASE_URL.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/SantiagoQP23/flashcards-backend/internal/db"
)

func main() {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		slog.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		slog.Error("connect", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		slog.Error("migrate", "error", err)
		os.Exit(1)
	}
	slog.Info("migrations applied")
}
