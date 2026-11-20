import { validateEnv } from './env.js';

describe('validateEnv', () => {
  it('applies defaults and coerces the port', () => {
    const env = validateEnv({ DATABASE_URL: 'postgresql://u:p@localhost:5432/db', PORT: '4000' });
    expect(env).toEqual({
      NODE_ENV: 'development',
      PORT: 4000,
      DATABASE_URL: 'postgresql://u:p@localhost:5432/db',
    });
  });

  it('rejects a missing DATABASE_URL with a readable message', () => {
    expect(() => validateEnv({})).toThrow(/DATABASE_URL/);
  });
});
