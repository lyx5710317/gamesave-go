import { describe, expect, it } from 'vitest';
import { summarizeLocalPreview, summarizeRemoteInventory, summarizeRemoteVault, summarizeJoinOverlap } from './localPreview.js';

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

  it('rejects duplicate game groups, duplicate snapshot names, and malformed entries', () => {
    expect(summarizeRemoteInventory([
      { gameId: 'one', snapshots: [{ name: 'a.zip' }] },
      { gameId: 'one', snapshots: [{ name: 'b.zip' }] }
    ])).toBeNull();
    expect(summarizeRemoteInventory([
      { gameId: 'one', snapshots: [{ name: 'a.zip' }] },
      { gameId: 'two', snapshots: [{ name: 'a.zip' }] }
    ])).toBeNull();
    expect(summarizeRemoteInventory([{ gameId: 'one', snapshots: [{}] }])).toBeNull();
  });
});

describe('display-only first-join overlap warning', () => {
  const remote = summarizeRemoteInventory([
    { gameId: 'shared', snapshots: [{ name: 'shared.zip' }] },
    { gameId: 'remote', snapshots: [{ name: 'remote.zip' }] }
  ]);

  it('flags shared game IDs without inferring equality or a sync direction', () => {
    expect(summarizeJoinOverlap({
      complete: true,
      library: { games: [
        { gameId: 'shared', name: 'Local title', latestAt: '2099-01-01' },
        { gameId: 'local', name: 'Remote title' }
      ] }
    }, remote)).toEqual({ overlapping: [{ gameId: 'shared', name: 'Local title' }] });
  });

  it('does not match names across different IDs or treat a missing inventory as empty', () => {
    expect(summarizeJoinOverlap({ complete: true, library: { games: [
      { gameId: 'different', name: 'shared' }
    ] } }, remote)).toEqual({ overlapping: [] });
    expect(summarizeJoinOverlap({ complete: true, library: { games: [] } }, null)).toBeNull();
  });

  it('stops the overlap summary for incomplete or duplicated local scans', () => {
    expect(summarizeJoinOverlap({ complete: false, library: { games: [] } }, remote)).toBeNull();
    expect(summarizeJoinOverlap({ complete: true, library: { games: [
      { gameId: 'same' }, { gameId: 'same' }
    ] } }, remote)).toBeNull();
  });
});

describe('read-only remote vault discovery', () => {
  it('accepts only a bounded validated summary, never a write capability', () => {
    expect(summarizeRemoteVault({ status: 'valid', revision: 2, deviceCount: 1, vaultId: 'private' }))
      .toEqual({ status: 'valid', revision: 2, deviceCount: 1 });
    expect(summarizeRemoteVault({ status: 'valid', revision: 0, deviceCount: 1 }))
      .toEqual({ status: 'unavailable' });
  });

  it('keeps missing, invalid, unsupported, and failed checks distinct', () => {
    for (const status of ['missing', 'invalid', 'upgrade-required', 'unsupported']) {
      expect(summarizeRemoteVault({ status })).toEqual({ status });
    }
    expect(summarizeRemoteVault(null)).toEqual({ status: 'unavailable' });
    expect(summarizeRemoteVault({ status: 'joined' })).toEqual({ status: 'unavailable' });
  });
});
