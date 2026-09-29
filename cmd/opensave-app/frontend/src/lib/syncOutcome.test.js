import { describe, expect, it } from 'vitest';
import { peerRequiringSavePath } from './syncOutcome.js';

describe('peerRequiringSavePath', () => {
  it('identifies the device that needs its save folder set', () => {
    expect(peerRequiringSavePath({ results: {
      ready: { status: 'updated', peerName: 'Desktop' },
      guest: { status: 'path_mapping_required', peerName: 'Windows VM' }
    } })).toBe('Windows VM');
  });

  it('uses the peer id when no name is available', () => {
    expect(peerRequiringSavePath({ results: {
      guest: { status: 'path_mapping_required' }
    } })).toBe('guest');
  });

  it('does not mistake a completed sync or unrelated error for a path choice', () => {
    expect(peerRequiringSavePath({ results: {
      ready: { status: 'in_sync' }, failed: { status: 'error' }
    } })).toBe('');
    expect(peerRequiringSavePath(null)).toBe('');
  });
});
