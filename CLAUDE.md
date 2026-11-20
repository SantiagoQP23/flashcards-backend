# flashcards-backend

Backend of Intenli Flashcards. The build plan (milestones, data model, API contracts) is the "Intenli Flashcards Build Plan" doc; the mobile app is `SantiagoQP23/flashcards-app`.

- NestJS 12 is ESM only and so is this project: relative imports end in `.js`, use `import.meta.dirname` instead of `__dirname`, and tests run on Vitest (`vi.fn()`).
- Validation uses Nest's built-in `StandardSchemaValidationPipe` with zod schemas (`@Body({ schema })`); document responses with `@ApiOkResponse({ standardSchema })`. Don't add class-validator or nestjs-zod.
- Prisma 7: the client is generated into `src/generated/prisma` (git-ignored) and connects through `@prisma/adapter-pg`. The database URL comes from `prisma.config.ts`, not the schema.
- After changing routes or schemas run `pnpm openapi` and commit `openapi.json`; the app's client is generated from it.
- Before pushing: `pnpm format:check && pnpm lint && pnpm typecheck && pnpm test && pnpm test:e2e`.
