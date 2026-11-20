# Intenli Flashcards API

Go + Chi backend for Intenli Flashcards: search an English word, hear it, save it, and review it with spaced repetition. The mobile app is [flashcards-app](https://github.com/SantiagoQP23/flashcards-app).

## Requirements

- Go (version in `go.mod`)
- Docker (for local PostgreSQL)

## Run locally

```sh
cp .env.example .env
make db-up     # Postgres 16 on localhost:5432 (also creates flashcards_test)
make run       # API on http://localhost:8080, applies migrations in development
```

Check it: `curl localhost:8080/healthz` (process is up) and `curl localhost:8080/readyz` (database reachable). From the Android emulator the API is at `http://10.0.2.2:8080`.

## Commands

| Command | What it does |
| --- | --- |
| `make run` | Start the API |
| `make migrate` | Apply pending migrations and exit (`api migrate` in production) |
| `make test` | Unit tests |
| `make test-db` | Unit tests plus tests against the local test database |
| `make lint` | `go vet` and `gofmt` check |
| `make build` | Build `bin/api` |

## Layout

| Path | What |
| --- | --- |
| `cmd/api` | Entry point: config, logger, migrations, HTTP server with graceful shutdown |
| `internal/config` | Environment variables |
| `internal/database` | pgx connection pool |
| `internal/store` | Hand-written SQL, one file per table group (`InTx`, `ErrNotFound`, `ErrConflict`) |
| `internal/httpapi` | Chi router, middleware, handlers, JSON helpers |
| `migrations` | goose SQL files, embedded in the binary |

## Configuration

| Variable | Default | Notes |
| --- | --- | --- |
| `DATABASE_URL` | (required) | PostgreSQL connection string |
| `APP_ENV` | `development` | `development` or `production` (JSON logs) |
| `HTTP_ADDR` | `:8080` | |
| `AUTO_MIGRATE` | `true` in development, `false` in production | Apply migrations on startup |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `CORS_ALLOWED_ORIGINS` | `http://localhost:8081` in development, none in production | Comma-separated browser origins (Expo web); native apps don't need CORS |

## API

JSON fields are snake_case, and lists are `{"data": [...]}`.

Errors are `{"error": {"code", "message"}}`. The `code` is the contract: the app shows its own translated text for it, and `message` is English for developers. Bad input is `invalid_request`; form errors a user can cause get specific codes.

Languages: Spanish (`es`, the default) and English (`en`), stored per user in `users.locale`.

| Route | Who | What |
| --- | --- | --- |
| `GET /healthz` | anyone | `{"status":"ok"}` while the process is up |
| `GET /readyz` | anyone | `{"status":"ready"}`, or `503 database_unavailable` |

Routes for auth, search, saved words, decks and reviews are added milestone by milestone (see the build plan).
