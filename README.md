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
| `internal/auth` | Passwords (argon2id), access tokens (JWT), refresh tokens, Google ID token check |
| `internal/database` | pgx connection pool |
| `internal/store` | Hand-written SQL, one file per table group (`InTx`, `ErrNotFound`, `ErrConflict`); tests clear data with `DELETE FROM users`, not `TRUNCATE ... CASCADE`, which would also wipe the shared dictionary |
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
| `JWT_SECRET` | dev-only value in development; **required** in production | 32+ characters; signs access tokens |
| `ACCESS_TOKEN_TTL` / `REFRESH_TOKEN_TTL` | `15m` / `720h` | |
| `GOOGLE_CLIENT_IDS` | (empty: Google sign-in off) | Comma-separated OAuth client IDs whose ID tokens are accepted |
| `AUTH_RATE_LIMIT` | `20` | Requests per minute per IP on `/v1/auth` (`0` disables) |

## API

JSON fields are snake_case, and lists are `{"data": [...]}`.

Errors are `{"error": {"code", "message"}}`. The `code` is the contract: the app shows its own translated text for it, and `message` is English for developers. Bad input is `invalid_request`; form errors a user can cause get specific codes.

Languages: Spanish (`es`, the default) and English (`en`), stored per user in `users.locale`.

Signed-in routes take `Authorization: Bearer <access_token>`. Without one the answer is `401 unauthorized`; with an expired or bad one it is `401 invalid_token`, and the app refreshes and retries once.

| Route | Who | What |
| --- | --- | --- |
| `GET /healthz` | anyone | `{"status":"ok"}` while the process is up |
| `GET /readyz` | anyone | `{"status":"ready"}`, or `503 database_unavailable` |
| `POST /v1/auth/register` | anyone | `{email, password, display_name?, timezone?}` → `201` session. `locale` comes from `Accept-Language`. Errors: `invalid_email`, `invalid_password` (8 to 128 characters), `email_taken` (409) |
| `POST /v1/auth/login` | anyone | `{email, password}` → session, or `401 invalid_credentials` |
| `POST /v1/auth/google` | anyone | `{id_token, timezone?}` (Google ID token from the app) → session. Errors: `invalid_google_token`, `email_not_verified` (401), `google_not_configured` (501), `retry` (409) |
| `POST /v1/auth/refresh` | anyone | `{refresh_token}` → new session; the old refresh token stops working. Error: `401 invalid_refresh_token` (sign in again) |
| `POST /v1/auth/logout` | anyone | `{refresh_token}` → `204`; signs out that device |
| `GET /v1/me` | signed in | user |
| `PATCH /v1/me` | signed in | `{display_name?, timezone?, locale?, preferred_accent?, daily_new_limit?}` → user. `""` clears `display_name` |
| `DELETE /v1/me` | signed in | `204`; deletes the account and everything in it |
| `GET /v1/decks` | signed in | `{data: [deck]}`, oldest first |
| `POST /v1/decks` | signed in | `{name}` → `201` deck. Errors: `invalid_deck_name` (1 to 60 characters), `deck_name_taken` (409, ignores case) |
| `PATCH /v1/decks/{id}` | signed in | `{name}` → deck. Errors as above, plus `404 deck_not_found` |
| `DELETE /v1/decks/{id}` | signed in | `204`; deletes the deck and every saved word in it. `404 deck_not_found` |
| `GET /v1/vocabulary` | signed in | `?deck_id&q&cursor&limit` → `{data: [entry], next_cursor}`, newest first. `q` matches anywhere in the word, `limit` is 1 to 100 (default 50), and `next_cursor` is `null` on the last page |
| `POST /v1/vocabulary` | signed in | Save a meaning: `{deck_id, note?}` plus either `{word_id, definition_id}` (a dictionary meaning) or `{word, definition: {text, example?, part_of_speech?}}` (typed in by hand) → `201` entry. Errors: `deck_not_found` (404), `definition_not_found` (404), `already_saved` (409), `invalid_word` (1 to 100 characters), `invalid_definition` (text 1 to 500 characters) |
| `PATCH /v1/vocabulary/{id}` | signed in | `{deck_id?, definition_id?, note?, definition?: {text?, example?, part_of_speech?}}` → entry. `deck_id` moves it, `definition_id` picks another meaning of the same word (its flashcard starts over), `definition` rewrites a typed-in meaning (`409 definition_not_editable` for dictionary meanings), `""` clears `note` or `example`. Errors: `entry_not_found` (404) and the ones above |
| `DELETE /v1/vocabulary/{id}` | signed in | `204`; the word leaves its deck and the review queue, and its meaning can be saved again. `404 entry_not_found` |

Auth routes answer `429 rate_limited` past `AUTH_RATE_LIMIT` requests a minute from one IP.

A **session** is `{user, access_token, refresh_token, token_type: "Bearer", expires_in}` (`expires_in` is the access token's lifetime in seconds).

A **user** is `{id, email, display_name, has_password, has_google, timezone, locale, preferred_accent, daily_new_limit, created_at}`. `timezone` is an IANA name (`America/Guayaquil`, default `UTC`), `locale` is `es` or `en`, `preferred_accent` is `US` or `UK`, `daily_new_limit` is 0 to 500 (default 20). `display_name` is `""` when not set.

A **deck** is `{id, name, entry_count, due_count, created_at, updated_at}`. `due_count` counts its words whose flashcard is due now.

An **entry** (a saved word) is:

```json
{
  "id": "…", "deck_id": "…", "note": "", "created_at": "…", "updated_at": "…",
  "word": {"id": "…", "text": "achieve", "phonetic_us": "/əˈtʃiːv/", "phonetic_uk": "/əˈtʃiːv/"},
  "definition": {"id": "…", "part_of_speech": "verb", "text": "…", "example": "…", "synonyms": ["accomplish"], "is_custom": false},
  "flashcard": {"id": "…", "state": "NEW", "due": "…"}
}
```

`part_of_speech` is `""` or one of `noun`, `verb`, `adjective`, `adverb`, `pronoun`, `preposition`, `conjunction`, `interjection`, `phrase`. `is_custom` is true for a meaning the user typed in; only those can be rewritten. `flashcard.state` is `NEW`, `LEARNING`, `REVIEW` or `RELEARNING`.

Routes for search and reviews are added milestone by milestone (see the build plan).

### Decks and saved words

- Every new account (password or Google) starts with a **starter deck**, "Vocabulario diario" or "Daily Vocabulary" by the account's language, holding 20 common words (migration `00006`, written by us, so they need no attribution). Accounts created before that migration don't get it.
- Each saved word sits in exactly one deck; a meaning can be saved once per user. Deleting is soft (`deleted_at`) so devices can sync it later.
- **Words typed in by hand** are private to the user who typed them: their meaning (and the word itself, when the dictionary doesn't have it) has `owner_id` set in `definitions` and `dictionary_words`. Dictionary lookups must only read rows where `owner_id IS NULL`.

### Sign-in rules

- Passwords are hashed with argon2id. Login answers the same way, with the same timing, for an unknown email, a Google-only account and a wrong password.
- Access tokens are HS256 JWTs that last `ACCESS_TOKEN_TTL`. Refresh tokens are random, stored as SHA-256 hashes, and rotate on every refresh. Reusing a rotated token signs out every device of that sign-in (likely theft), except for 60 seconds after rotation, so a retry after a lost response still works.
- Google sign-in finds the user by Google account, then by email (linking the two), and otherwise creates a new user. Emails aren't verified yet, so linking to a password account removes its password and signs out its sessions: whoever set that password never proved they own the address.
- Not built yet: email verification and password reset.

### Google sign-in setup

1. In [Google Cloud Console](https://console.cloud.google.com/apis/credentials), create an OAuth consent screen, then OAuth client IDs of type **Android** (package `com.intenli.flashcards` plus your signing SHA-1) and **Web** (the app asks for an ID token for the web client). Add **iOS** (bundle `com.intenli.flashcards`) when the iOS app comes.
2. Set `GOOGLE_CLIENT_IDS` to all of them, comma-separated.
3. Give the same IDs to the app (see its README).
