import { describe, expect, it } from 'vitest';
import { manualUploadOutcome, summarizeUploadActivity } from './uploadActivity.js';

describe('cloud upload activity', () => {
  it('separates running, succeeded, and failed transfers', () => {
    expect(summarizeUploadActivity([
      { status: 'running' }, { status: 'succeeded' }, { status: 'failed' }, { status: 'failed' }
    ])).toEqual({ running: 1, succeeded: 1, failed: 2 });
  });

  it('does not infer success from an absent response', () => {
    expect(summarizeUploadActivity(null)).toEqual({ running: 0, succeeded: 0, failed: 0 });
  });

  it('never presents a partial failure as already current', () => {
    expect(manualUploadOutcome({ uploaded: 0, skipped: 3, failed: 1 })).toBe('failed');
    expect(manualUploadOutcome({ uploaded: 0, skipped: 0, conflicts: 1, failed: 0 })).toBe('conflict');
    expect(manualUploadOutcome({ uploaded: 1, skipped: 0, conflicts: 1, failed: 1 })).toBe('conflict');
    expect(manualUploadOutcome({ uploaded: 0, skipped: 3, failed: 0 })).toBe('already-current');
    expect(manualUploadOutcome({ uploaded: 2, skipped: 1, failed: 0 })).toBe('uploaded');
  });
});
