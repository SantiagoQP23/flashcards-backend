DATABASE_URL ?= postgresql://flashcards:flashcards@localhost:5432/flashcards

.PHONY: run migrate generate test lint check

run: ## Start the API on :3000
	DATABASE_URL=$(DATABASE_URL) go run ./cmd/api

migrate: ## Apply pending migrations
	DATABASE_URL=$(DATABASE_URL) go run ./cmd/migrate

generate: ## Regenerate server code from api/openapi.yaml and queries from internal/db/queries
	go generate ./...

test: ## Unit tests, plus database tests when DATABASE_URL is set
	DATABASE_URL=$(DATABASE_URL) go test ./...

lint:
	golangci-lint run ./...

check: generate lint test ## Everything CI runs
	git diff --exit-code -- internal/httpapi/api.gen.go internal/db
