import { describe, expect, it } from 'vitest';
import { isTemporarilyHiddenProvider, visibleCloudProviders } from './cloudProviderVisibility.js';
import { readFileSync } from 'node:fs';
import { recommendJianguoyunForUnset } from './jianguoyun.js';
import { translate } from './i18n.js';

describe('temporary cloud provider visibility', () => {
  it('hides Baidu, OneDrive, and Dropbox but retains the active choices', () => {
    const providers = ['jianguoyun', 'baidu', 'google_drive', 'onedrive', 'dropbox', 'local', 'webdav', 'webhook']
      .map((id) => ({ id }));
    expect(visibleCloudProviders(providers).map(({ id }) => id)).toEqual([
      'jianguoyun', 'google_drive', 'local', 'webdav', 'webhook'
    ]);
    expect(providers).toHaveLength(8);
    expect(['baidu', 'onedrive', 'dropbox'].every(isTemporarilyHiddenProvider)).toBe(true);
    expect(isTemporarilyHiddenProvider('google_drive')).toBe(false);
  });

  it('keeps a saved hidden-provider configuration intact', () => {
    const saved = { provider: 'onedrive', url: 'unchanged', tokens: { userEmail: 'example.invalid' } };
    expect(recommendJianguoyunForUnset(saved)).toBe(saved);
    expect(saved).toEqual({ provider: 'onedrive', url: 'unchanged', tokens: { userEmail: 'example.invalid' } });
    const page = readFileSync(new URL('../views/CloudBackup.svelte', import.meta.url), 'utf8');
    expect(page).toContain('{#each visibleCloudProviders(providers) as p}');
    expect(page).toContain('{#if isTemporarilyHiddenProvider(config.provider)}');
    expect(page).toContain('{#if !isTemporarilyHiddenProvider(config.provider)}');
    for (const language of ['en', 'zh-CN']) {
      expect(translate(language, 'cloud.hiddenExistingProvider')).not.toBe('cloud.hiddenExistingProvider');
      expect(translate(language, 'cloud.hiddenExistingConnection')).not.toBe('cloud.hiddenExistingConnection');
      expect(translate(language, 'settings.cloud.ownAppHint')).not.toMatch(/OneDrive|Dropbox|百度网盘|Baidu/);
    }
  });
});
