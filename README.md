# flashcards-backend

API for Intenli Flashcards: dictionary lookup, decks, saved vocabulary, spaced-repetition reviews and progress. NestJS 12 (ESM), PostgreSQL with Prisma 7, zod validation, Vitest.

## Run it locally

```sh
cp .env.example .env
docker compose up -d        # PostgreSQL 16 on localhost:5432
pnpm install                # also generates the Prisma client
pnpm db:migrate             # apply migrations
pnpm dev                    # http://localhost:3000/v1, docs at /docs
```

## Scripts

| Script                                         | What it does                                                                 |
| ---------------------------------------------- | ---------------------------------------------------------------------------- |
| `pnpm dev`                                     | Start with watch mode                                                        |
| `pnpm lint` / `pnpm typecheck` / `pnpm format` | Code checks                                                                  |
| `pnpm test`                                    | Unit tests (`src/**/*.spec.ts`)                                              |
| `pnpm test:e2e`                                | End-to-end tests against the database in `.env` (`test/**/*.e2e-spec.ts`)    |
| `pnpm db:migrate`                              | Create and apply a migration after changing `prisma/schema.prisma`           |
| `pnpm openapi`                                 | Rewrite `openapi.json`, which `flashcards-app` generates its API client from |

## Conventions

- All routes live under `/v1`.
- Request and response shapes are zod schemas. Pass them to `@Body({ schema })`, `@Query({ schema })` or `@Param('id', { schema })` for validation, and to `@ApiOkResponse({ standardSchema })` so they appear in `openapi.json`.
- After changing any route or schema, run `pnpm openapi` and commit the result; CI fails otherwise.
