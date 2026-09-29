import { describe, expect, it } from 'vitest';
import { translate } from './i18n.js';
import { manualUploadOutcome, summarizeUploadActivity, uploadStatusText } from './uploadActivity.js';

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

  it('shows only controlled Jianguoyun safety diagnostics in both languages', () => {
    const record = { status: 'failed', provider: 'jianguoyun', failure: 'unsafe_condition', safetyCheck: 'move_first', httpStatus: 405 };
    expect(uploadStatusText(record, (key) => translate('zh-CN', key))).toContain('首次 WebDAV 移动失败 (HTTP 405)');
    expect(uploadStatusText(record, (key) => translate('en', key))).toContain('first WebDAV move failed (HTTP 405)');
    expect(uploadStatusText({ ...record, safetyCheck: 'private-account@example.invalid', httpStatus: 999 }, (key) => translate('zh-CN', key)))
      .toBe(translate('zh-CN', 'cloud.activity.jianguoyun.unsafe_condition'));
  });

  it('does not present a failed readback as a completed backup', () => {
    const record = { status: 'failed', provider: 'jianguoyun', failure: 'integrity' };
    expect(uploadStatusText(record, (key) => translate('zh-CN', key))).toContain('上传未确认成功');
    expect(uploadStatusText(record, (key) => translate('en', key))).toContain('upload is not confirmed');
  });
});
