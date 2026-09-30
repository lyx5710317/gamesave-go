import { describe, expect, it, vi } from 'vitest';

vi.mock('svelte', () => ({ mount: vi.fn(() => ({ mounted: true })) }));
vi.mock('./App.svelte', () => ({ default: function App() {} }));

describe('desktop entry point', () => {
  it('mounts the Svelte 5 component without using the removed class API', async () => {
    const target = {};
    vi.stubGlobal('document', { getElementById: vi.fn(() => target) });
    try {
      const { mount } = await import('svelte');
      const { default: app } = await import('./main.js');
      expect(document.getElementById).toHaveBeenCalledWith('app');
      expect(mount).toHaveBeenCalledWith(expect.any(Function), { target });
      expect(app).toEqual({ mounted: true });
    } finally {
      vi.unstubAllGlobals();
    }
  });
});
