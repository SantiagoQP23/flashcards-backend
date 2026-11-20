import { Logger } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import { NestFactory } from '@nestjs/core';
import { SwaggerModule } from '@nestjs/swagger';
import { AppModule } from './app.module.js';
import { buildOpenApiDocument, configureApp } from './bootstrap.js';
import type { Env } from './config/env.js';

const app = await NestFactory.create(AppModule);
configureApp(app);
const config = app.get<ConfigService<Env, true>>(ConfigService);

if (config.get('NODE_ENV', { infer: true }) !== 'production') {
  SwaggerModule.setup('docs', app, buildOpenApiDocument(app));
}

const port = config.get('PORT', { infer: true });
await app.listen(port);
Logger.log(`API listening on http://localhost:${port}/v1`, 'Bootstrap');
