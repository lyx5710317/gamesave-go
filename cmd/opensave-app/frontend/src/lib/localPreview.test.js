import { describe, expect, it } from 'vitest';
import { summarizeLocalPreview } from './localPreview.js';

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
