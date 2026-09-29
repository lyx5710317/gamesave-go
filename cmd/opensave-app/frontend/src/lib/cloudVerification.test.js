import { describe, expect, it } from 'vitest';
import { translate } from './i18n.js';
import { cloudVerificationFeedback, cloudArchiveFailureKey, cloudVerificationFailureKey } from './cloudVerification.js';

describe('read-only cloud snapshot verification', () => {
  it('localizes every supported verification failure without echoing private details', () => {
    const categories = {
      disabled: 'disabled',
      configuration: 'configuration', authentication: 'authentication', permission: 'permission',
      quota: 'quota', rate_limit: 'rateLimit', network: 'network', missing: 'missing',
      incomplete_inventory: 'incompleteInventory', ambiguous: 'ambiguous',
      unsafe_archive: 'unsafeArchive', size_mismatch: 'sizeMismatch', integrity: 'integrity', local_io: 'localIO'
    };
    for (const [code, category] of Object.entries(categories)) {
      const key = cloudVerificationFailureKey({ code: `cloud_verify_${code}`, message: 'synthetic-private-detail' });
      expect(key).toBe(`cloud.verify.failure.${category}`);
      for (const locale of ['zh-CN', 'en']) {
        expect(translate(locale, key)).not.toBe(key);
        expect(translate(locale, key)).not.toContain('synthetic-private-detail');
      }
    }
  });

  it('fails closed for absent, unknown and inherited diagnostic codes', () => {
    for (const code of [undefined, null, 'cloud_verify_failed', 'toString', '__proto__', 'constructor', 'private@example.invalid']) {
      expect(cloudVerificationFailureKey({ code, message: 'private@example.invalid' })).toBe('cloud.verify.failed');
    }
    expect(cloudVerificationFailureKey(null)).toBe('cloud.verify.failed');
  });
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

  it('localizes the controlled unsafe-archive error without displaying entry names', () => {
    const message = 'cloud snapshot archive contains unsafe or conflicting paths; no saves were changed';
    expect(cloudArchiveFailureKey(new Error(message))).toBe('cloud.restore.unsafeArchive');
    expect(cloudArchiveFailureKey(null)).toBeNull();
    expect(cloudArchiveFailureKey(new Error('private-save.sav'))).toBeNull();
    expect(cloudArchiveFailureKey(new Error(`${message}: private-save.sav`))).toBeNull();
    expect(translate('zh-CN', 'cloud.restore.unsafeArchive')).toContain('本机存档没有改变');
    expect(translate('en', 'cloud.restore.unsafeArchive')).toContain('local saves are unchanged');
  });
});
