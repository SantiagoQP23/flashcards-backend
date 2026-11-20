# flashcards-backend

Backend of Intenli Flashcards in Go. The build plan (milestones, data model, API contracts) is the "Intenli Flashcards Build Plan" doc; the mobile app is `SantiagoQP23/flashcards-app`. Setup and conventions follow `SantiagoQP23/intenli-money-backend`.

- Router: chi with hand-written handlers in `internal/httpapi`. Use `decodeJSON`, `writeJSON`, `writeError` and `internalError`; errors are `{"error":{"code","message"}}`, JSON is snake_case, lists are `{"data":[...]}`.
- Database: PostgreSQL via pgx/v5. SQL lives in `internal/store` (hand-written, one file per table group, `InTx` for transactions). Schema changes are new goose files in `migrations/` (never edit an applied one).
- The README's API table is the contract with the app, which keeps its own types in `src/api/`. Update both in the same change when a route or response shape changes.
- Config comes from env vars (`internal/config`, documented in the README and `.env.example`).
- Before pushing: `make lint test-db` (needs `make db-up`, or set `TEST_DATABASE_URL`).
