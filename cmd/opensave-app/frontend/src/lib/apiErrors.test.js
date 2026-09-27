import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from './api.js';

afterEach(() => vi.unstubAllGlobals());

function respond(body, ok = false, status = 502) {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok, status, json: async () => body }));
}

describe('structured API failure codes', () => {
  it('preserves a valid fixed diagnostic code', async () => {
    respond({ error: 'verification failed', code: 'cloud_verify_network' });
    await expect(api.post('/api/cloud/verify/game', {})).rejects.toMatchObject({
      message: 'verification failed', code: 'cloud_verify_network'
    });
  });

  it('does not forward malformed codes', async () => {
    for (const code of [null, 1, 'private@example.invalid', 'CloudFailure', 'x'.repeat(65), { private: true }]) {
      respond({ error: 'verification failed', code });
      const error = await api.get('/api/cloud').catch(e => e);
      expect(error).toBeInstanceOf(Error);
      expect(error.code).toBeUndefined();
    }
  });

  it('retains legacy failure and success responses', async () => {
    respond({ error: 'legacy error' });
    await expect(api.get('/api/cloud')).rejects.toThrow('legacy error');
    respond({});
    await expect(api.get('/api/cloud')).rejects.toThrow('GET /api/cloud failed (502)');
    respond({ status: 'ok' }, true, 200);
    await expect(api.get('/api/cloud')).resolves.toEqual({ status: 'ok' });
  });
});
