import type { Response } from 'express';
import type { PrismaService } from '../prisma/prisma.service.js';
import { HealthController } from './health.controller.js';

describe('HealthController', () => {
  const res = () => {
    const status = vi.fn();
    return { status, response: { status } as unknown as Response };
  };

  it('reports ok when the database answers', async () => {
    const prisma = {
      $queryRaw: vi.fn().mockResolvedValue([{ '?column?': 1 }]),
    } as unknown as PrismaService;
    const { status, response } = res();
    await expect(new HealthController(prisma).check(response)).resolves.toEqual({
      status: 'ok',
      database: 'up',
    });
    expect(status).not.toHaveBeenCalled();
  });

  it('returns 503 when the database is down', async () => {
    const prisma = { $queryRaw: vi.fn().mockRejectedValue(new Error('down')) } as unknown as PrismaService;
    const { status, response } = res();
    await expect(new HealthController(prisma).check(response)).resolves.toEqual({
      status: 'degraded',
      database: 'down',
    });
    expect(status).toHaveBeenCalledWith(503);
  });
});
