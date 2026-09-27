import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { cloudReadFailureKey } from './cloudVerification.js';
import { translate } from './i18n.js';
import { recommendJianguoyunForUnset } from './jianguoyun.js';

describe('cloud read failures and the global enable switch', () => {
  it('distinguishes the disabled switch from authentication, network and inventory failures', () => {
    const categories = {
      disabled: 'disabled', configuration: 'configuration', authentication: 'authentication',
      permission: 'permission', quota: 'quota', rate_limit: 'rateLimit', network: 'network',
      missing: 'missing', incomplete_inventory: 'incompleteInventory', ambiguous: 'ambiguous', local_io: 'localIO'
    };
    for (const [code, category] of Object.entries(categories)) {
      const key = cloudReadFailureKey({ code: `cloud_read_${code}`, message: 'synthetic-private-detail' });
      expect(key).toBe(`cloud.read.failure.${category}`);
      for (const locale of ['zh-CN', 'en']) {
        expect(translate(locale, key)).not.toBe(key);
        expect(translate(locale, key)).not.toContain('synthetic-private-detail');
      }
    }
  });

  it('does not reveal raw or unknown provider errors', () => {
    for (const code of [undefined, null, 'cloud_read_failed', 'cloud_verify_network', 'toString', '__proto__', 'private@example.invalid']) {
      expect(cloudReadFailureKey({ code, message: 'private@example.invalid' })).toBe('cloud.read.failure.failed');
    }
    expect(cloudReadFailureKey(null)).toBe('cloud.read.failure.failed');
  });

  it('explains all-cloud access and automatic upload without silently enabling it', () => {
    expect(recommendJianguoyunForUnset({ provider: 'local', url: '', enabled: false }).enabled).toBe(false);
    expect(recommendJianguoyunForUnset({ provider: 'jianguoyun', enabled: false }).enabled).toBe(false);
    for (const locale of ['zh-CN', 'en']) {
      for (const key of ['cloud.enabled.label', 'cloud.enabled.hint', 'cloud.status.disabled', 'settings.cloud.autoMirror', 'settings.cloud.autoMirrorHint']) {
        expect(translate(locale, key)).not.toBe(key);
      }
    }
    expect(translate('zh-CN', 'cloud.enabled.hint')).toContain('自动上传');
    expect(translate('zh-CN', 'cloud.read.failure.disabled')).toContain('无需删除或重填已有密码');
  });

  it('keeps a translated error visible and discards stale actionable inventory on read failure', () => {
    const markup = readFileSync(new URL('../views/CloudBackup.svelte', import.meta.url), 'utf8');
    const browse = markup.slice(markup.indexOf('async function browseCloud()'), markup.indexOf('function openCloudBrowser()'));
    expect(browse).toContain('if (browsing) return;');
    expect(browse).toContain('cloudBrowseFailure = cloudReadFailureKey(e);');
    expect(browse).toContain('cloudGames = null;');
    expect(browse).toContain('detailId = null;');
    expect(markup).toContain('{:else if cloudBrowseFailure}');
    expect(markup).toContain('<p>{$t(cloudBrowseFailure)}</p>');
    expect(markup).toContain('bind:checked={config.enabled}');
    expect(markup).toContain("if (id === cfg.provider && !cfg.enabled) return $t('cloud.status.disabled');");
  });
});
