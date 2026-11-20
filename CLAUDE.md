# flashcards-backend

Backend of Intenli Flashcards in Go. The build plan (milestones, data model, API contracts) is the "Intenli Flashcards Build Plan" doc; the mobile app is `SantiagoQP23/flashcards-app`.

- Router: chi. Handlers implement the oapi-codegen `StrictServerInterface` generated from `api/openapi.yaml` (spec first: change the spec, `make generate`, then implement).
- Database: PostgreSQL via pgx/v5. Queries are written in `internal/db/queries/*.sql` and generated with sqlc; schema changes are new goose files in `migrations/` (never edit an applied one).
- Generators run via `go tool` (see the `tool` block in go.mod). Commit generated files; CI fails if they are stale.
- The app regenerates its TypeScript client from `api/openapi.yaml`, so keep response shapes stable or update the app in the same change.
- Before pushing: `make generate lint test` (tests that need Postgres run when `DATABASE_URL` is set).
