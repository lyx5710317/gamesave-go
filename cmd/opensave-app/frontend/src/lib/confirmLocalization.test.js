import { afterEach, describe, expect, it } from 'vitest';
import { get } from 'svelte/store';
import { readFileSync } from 'node:fs';
import { askConfirm, answerConfirm, confirmRequest } from './stores.js';
import { locale, t, translate } from './i18n.js';

const initialLocale = get(locale);
afterEach(() => {
  answerConfirm(false);
  locale.set(initialLocale);
});

describe('shared confirmation localization', () => {
  it('leaves omitted labels to the reactive dialog instead of English defaults', async () => {
    locale.set('zh-CN');
    const result = askConfirm('test');
    const request = get(confirmRequest);
    expect(request.title).toBeNull();
    expect(request.confirmText).toBeNull();
    expect(request.cancelText).toBeNull();
    answerConfirm(false);
    expect(await result).toBe(false);
  });

  it('retains caller-supplied titles, actions and alternative cancellation labels', async () => {
    const result = askConfirm('test', { title: '验证', confirmText: '验证', cancelText: '保留云端', danger: true });
    expect(get(confirmRequest)).toMatchObject({ title: '验证', confirmText: '验证', cancelText: '保留云端', danger: true });
    answerConfirm(true);
    expect(await result).toBe(true);
  });

  it('provides complete defaults in both languages and reads the active language', () => {
    for (const language of ['en', 'zh-CN']) {
      for (const key of ['dialog.title', 'dialog.confirm', 'dialog.cancel']) {
        expect(translate(language, key)).not.toBe(key);
      }
    }
    locale.set('zh-CN');
    expect(get(t)('dialog.cancel')).toBe('取消');
    locale.set('en');
    expect(get(t)('dialog.cancel')).toBe('Cancel');
  });

  it('uses translated defaults for shared verification and password-removal dialogs', () => {
    const dialog = readFileSync(new URL('../components/ConfirmDialog.svelte', import.meta.url), 'utf8');
    expect(dialog).toContain("$confirmRequest.title ?? $t('dialog.title')");
    expect(dialog).toContain("$confirmRequest.confirmText ?? $t('dialog.confirm')");
    expect(dialog).toContain("$confirmRequest.cancelText ?? $t('dialog.cancel')");
    const cloud = readFileSync(new URL('../views/CloudBackup.svelte', import.meta.url), 'utf8');
    expect(cloud).toContain("askConfirm($t('cloud.verify.confirm')");
    expect(cloud).toContain("askConfirm($t('cloud.jianguoyun.disconnectConfirm')");
  });
});
