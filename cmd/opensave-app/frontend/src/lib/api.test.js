import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from './api.js';

afterEach(() => vi.unstubAllGlobals());

describe('daemon HTTP responses', () => {
  it('rejects a successful HTTP status with a malformed body', async () => {
    vi.stubGlobal('fetch', async () => new Response('{"success":'));
    await expect(api.post('/fixture', {})).rejects.toThrow('invalid JSON');
  });

  it('preserves the status error when the daemon returns a null body', async () => {
    vi.stubGlobal('fetch', async () => new Response('null', { status: 500 }));
    await expect(api.get('/fixture')).rejects.toThrow('GET /fixture failed (500)');
  });

  it('preserves an application error and its code', async () => {
    vi.stubGlobal('fetch', async () => Response.json({ error: 'unsafe save', code: 'restore_safety' }, { status: 409 }));
    await expect(api.get('/fixture')).rejects.toMatchObject({ message: 'unsafe save', code: 'restore_safety' });
  });
});
