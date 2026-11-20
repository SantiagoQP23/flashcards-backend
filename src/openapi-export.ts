// Writes openapi.json at the repo root. The app generates its typed client from this file.
// Run with `pnpm openapi`; CI fails if the committed file is out of date.
import { writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { NestFactory } from '@nestjs/core';
import { AppModule } from './app.module.js';
import { buildOpenApiDocument, configureApp } from './bootstrap.js';

process.env.DATABASE_URL ??= 'postgresql://unused@localhost:5432/unused';
const app = await NestFactory.create(AppModule, { logger: false });
configureApp(app);
const doc = buildOpenApiDocument(app);
writeFileSync(join(import.meta.dirname, '..', 'openapi.json'), `${JSON.stringify(doc, null, 2)}\n`);
await app.close();
