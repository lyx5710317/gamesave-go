import { describe, expect, it } from 'vitest';
import { summarizeLocalPreview, summarizeRemoteInventory } from './localPreview.js';

describe('local cloud join preview', () => {
  it('keeps unmeasured discoveries visible as incomplete', () => {
    expect(summarizeLocalPreview({
      library: { games: [{ gameId: 'one' }] },
      candidates: [{ id: 'measured', measured: true }, { id: 'unknown', measured: false }],
      complete: false
    })).toEqual({ trackedCount: 1, detectedCount: 2, unknownCount: 1, complete: false });
  });

  it('never treats missing scan fields as a completed empty library', () => {
    expect(summarizeLocalPreview(null)).toEqual({
      trackedCount: 0, detectedCount: 0, unknownCount: 0, complete: false
    });
  });

  it('reports a fully measured scan without implying a remote comparison', () => {
    expect(summarizeLocalPreview({ library: { games: [] }, candidates: [], complete: true })).toEqual({
      trackedCount: 0, detectedCount: 0, unknownCount: 0, complete: true
    });
  });
});

describe('read-only remote snapshot inventory', () => {
  it('distinguishes a successfully listed empty destination from an unknown one', () => {
    expect(summarizeRemoteInventory([])).toEqual({ gameCount: 0, snapshotCount: 0, games: [] });
    expect(summarizeRemoteInventory(null)).toBeNull();
    expect(summarizeRemoteInventory([{ gameId: 'one' }])).toBeNull();
  });

  it('counts listed snapshots without claiming a vault comparison or game match', () => {
    expect(summarizeRemoteInventory([
      { gameId: 'one', gameName: 'Game One', snapshots: [{ name: 'a.zip' }, { name: 'b.zip' }] },
      { gameId: 'two', snapshots: [{ name: 'c.zip' }] }
    ])).toEqual({
      gameCount: 2,
      snapshotCount: 3,
      games: [
        { gameId: 'one', name: 'Game One', snapshotCount: 2 },
        { gameId: 'two', name: 'two', snapshotCount: 1 }
      ]
    });
  });
});
