import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import en from '../locales/en.js';
import zhCN from '../locales/zh-CN.js';

const view = readFileSync(new URL('../views/CloudBackup.svelte', import.meta.url), 'utf8');

describe('protected cloud credentials in the settings UI', () => {
  it('indicates configured WebDAV and OAuth secrets without prefilling the fields', () => {
    expect(view).toContain('config.passwordConfigured ? $t(\'cloud.fields.secretConfigured\')');
    expect(view).toContain('config.customClientSecretsConfigured?.[config.provider]');
    expect(view).toContain('type="password" autocomplete="new-password" bind:value={config.password}');
    expect(view).toContain('type="password" autocomplete="new-password" bind:value={ownAppSecret}');
  });

  it('requires an explicit action to remove saved custom headers', () => {
    expect(view).toContain("{ cloudSync: { clearHeaders: true } }");
    expect(view).toContain("config.headersConfigured ? $t('cloud.fields.secretConfigured')");
    expect(view).toContain("$t('cloud.fields.removeHeaders')");
  });

  it('has matching Chinese and English status labels', () => {
    for (const key of ['cloud.fields.secretConfigured', 'cloud.fields.removeHeaders', 'cloud.toast.headersRemoved']) {
      expect(en[key]).toBeTruthy();
      expect(zhCN[key]).toMatch(/\p{Script=Han}/u);
    }
  });
});
