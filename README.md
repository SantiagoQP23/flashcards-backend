# flashcards-backend

API for Intenli Flashcards: dictionary lookup, decks, saved vocabulary, spaced-repetition reviews and progress. Go with [chi](https://github.com/go-chi/chi), PostgreSQL through pgx, [sqlc](https://sqlc.dev) queries, [goose](https://github.com/pressly/goose) migrations, and a spec-first OpenAPI contract with [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen).

## Run it locally

```sh
docker compose up -d   # PostgreSQL 16 on localhost:5432
make migrate           # apply migrations/*.sql
make run               # http://localhost:3000/v1/health, spec at /openapi.yaml
```

## How the code is organised

| Path | What lives there |
| --- | --- |
| `api/openapi.yaml` | The API contract. The server interface and the app's TypeScript client are both generated from it. |
| `cmd/api`, `cmd/migrate` | The API server and the migration runner. |
| `internal/httpapi` | chi router, middleware and the handlers implementing the generated `StrictServerInterface`. |
| `internal/db` | pgx pool, migrations runner, and sqlc code generated from `internal/db/queries/*.sql`. |
| `internal/config` | Environment settings (`APP_ENV`, `PORT`, `DATABASE_URL`). |
| `migrations` | goose SQL migrations, embedded in the binary. |

## Adding an endpoint

1. Describe it in `api/openapi.yaml`.
2. Add any SQL to `internal/db/queries/` (and a migration if the schema changes).
3. `make generate`, then implement the new method on `httpapi.Server`. The build fails until you do.
4. `make lint test`, and commit the generated files.

Code generators run through `go tool`, so they need nothing installed beyond Go. Linting uses [golangci-lint](https://golangci-lint.run) v2.
