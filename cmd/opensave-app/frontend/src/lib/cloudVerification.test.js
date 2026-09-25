import { describe, expect, it } from 'vitest';
import { translate } from './i18n.js';
import { cloudVerificationFeedback } from './cloudVerification.js';

describe('read-only cloud snapshot verification', () => {
  it('distinguishes identical, different, and absent local archives', () => {
    expect(cloudVerificationFeedback({ sizeBytes: 123, localComparison: 'identical' })).toEqual({ key: 'cloud.verify.identical', tone: 'success' });
    expect(cloudVerificationFeedback({ sizeBytes: 123, localComparison: 'different' })).toEqual({ key: 'cloud.verify.different', tone: 'info' });
    expect(cloudVerificationFeedback({ sizeBytes: 123, localComparison: 'unavailable' })).toEqual({ key: 'cloud.verify.unavailable', tone: 'info' });
  });

  it('never labels an invalid or incomplete result as verified', () => {
    expect(() => cloudVerificationFeedback(null)).toThrow();
    expect(() => cloudVerificationFeedback({ sizeBytes: 0, localComparison: 'identical' })).toThrow();
    expect(() => cloudVerificationFeedback({ sizeBytes: 123, localComparison: 'unknown' })).toThrow();
  });

  it('has complete Chinese and English guidance without a multi-device sync claim', () => {
    for (const key of ['cloud.verify.confirm', 'cloud.verify.identical', 'cloud.verify.different', 'cloud.verify.unavailable', 'cloud.verify.failed']) {
      expect(translate('zh-CN', key)).not.toBe(key);
      expect(translate('en', key)).not.toBe(key);
    }
    expect(translate('zh-CN', 'cloud.verify.identical')).toContain('字节一致');
    expect(translate('en', 'cloud.verify.unavailable')).toContain('no local archive');
  });
});
