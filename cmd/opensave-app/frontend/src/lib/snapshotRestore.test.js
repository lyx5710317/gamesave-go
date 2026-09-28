import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { fileRestoreFailureKey, restorePreflightFailureKey, snapshotComment } from './snapshotRestore.js';
import { translate } from './i18n.js';

describe('single-file restore safety diagnostics', () => {
  it('maps whole-restore preflight failures in both languages without private errors', () => {
    for (const category of ['archive', 'location', 'safety', 'changed']) {
      const key = restorePreflightFailureKey({ code: `restore_${category}`, message: 'synthetic-private-path' });
      expect(key).toBe(`game.restoreFailure.${category}`);
      for (const language of ['zh-CN', 'en']) {
        expect(translate(language, key)).not.toBe(key);
        expect(translate(language, key)).not.toContain('synthetic-private-path');
      }
    }
    for (const code of [undefined, '__proto__', 'toString', 'restore_unknown']) expect(restorePreflightFailureKey({code})).toBeNull();
    for (const view of ['GameDetail', 'CloudBackup']) {
      expect(readFileSync(new URL(`../views/${view}.svelte`, import.meta.url), 'utf8')).toContain('restorePreflightFailureKey(e)');
    }
    expect(readFileSync(new URL('../views/CloudBackup.svelte', import.meta.url), 'utf8')).toContain('(res.results || []).map(restorePreflightFailureKey)');
  });
  it('localizes generated safety comments without rewriting user comments', () => {
    for (const [comment, key] of [
      ['Pre-rollback safety restore point (before restoring snap_123)', 'game.safetyRestoreComment'],
      ['Safety snapshot before single-file restore from snap_123', 'game.safetyFileRestoreComment']
    ]) {
      for (const language of ['zh-CN', 'en']) {
        const t = (key, args) => translate(language, key, args);
        expect(snapshotComment({ comment, isSystemAuto: true }, t)).toBe(t(key, { id: 'snap_123' }));
        expect(snapshotComment({ comment, isSystemAuto: false }, t)).toBe(comment);
      }
    }
    expect(snapshotComment({ comment: 'synthetic custom comment', isSystemAuto: true }, () => 'changed')).toBe('synthetic custom comment');
  });
  it('maps only fixed codes to bilingual actionable instructions', () => {
    for (const category of ['location', 'target', 'archive', 'safety', 'changed', 'publish']) {
      const key = fileRestoreFailureKey({ code: `file_restore_${category}`, message: 'synthetic-private-path' });
      expect(key).toBe(`game.fileRestoreFailure.${category}`);
      for (const language of ['zh-CN', 'en']) {
        const text = translate(language, key);
        expect(text).not.toBe(key);
        expect(text).not.toContain('synthetic-private-path');
      }
    }
    for (const code of [undefined, 'toString', '__proto__', 'file_restore_unknown']) {
      expect(fileRestoreFailureKey({ code })).toBeNull();
    }
    expect(fileRestoreFailureKey(null)).toBeNull();
  });

  it('explains that the local snapshot list does not download a cloud snapshot', () => {
    const markup = readFileSync(new URL('../views/GameDetail.svelte', import.meta.url), 'utf8');
    expect(markup).toContain('fileRestoreFailureKey(e)');
    expect(markup).toContain("$t('game.localSnapshotsHint')");
    expect(translate('zh-CN', 'game.localSnapshotsHint')).toContain('不会自动下载');
    expect(translate('en', 'game.localSnapshotsHint')).not.toBe('game.localSnapshotsHint');
  });
});
