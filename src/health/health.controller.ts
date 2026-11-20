import { Controller, Get, HttpStatus, Res } from '@nestjs/common';
import { ApiOkResponse, ApiServiceUnavailableResponse, ApiTags } from '@nestjs/swagger';
import type { Response } from 'express';
import { PrismaService } from '../prisma/prisma.service.js';
import { type Health, healthSchema } from './health.dto.js';

@ApiTags('health')
@Controller('health')
export class HealthController {
  constructor(private readonly prisma: PrismaService) {}

  @Get()
  @ApiOkResponse({ standardSchema: healthSchema })
  @ApiServiceUnavailableResponse({ standardSchema: healthSchema })
  async check(@Res({ passthrough: true }) res: Response): Promise<Health> {
    try {
      await this.prisma.$queryRaw`SELECT 1`;
      return { status: 'ok', database: 'up' };
    } catch {
      res.status(HttpStatus.SERVICE_UNAVAILABLE);
      return { status: 'degraded', database: 'down' };
    }
  }
}
