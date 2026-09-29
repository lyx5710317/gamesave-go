import { describe, expect, it } from 'vitest';
import { JIANGUOYUN_BASE_URL, JIANGUOYUN_REMOTE_FOLDER, recommendJianguoyunForUnset, selectCloudProvider } from './jianguoyun.js';
import { translate } from './i18n.js';

describe('Jianguoyun preset', () => {
  it('uses the official base and a separate application folder', () => {
    expect(JIANGUOYUN_BASE_URL).toBe('https://dav.jianguoyun.com/dav/');
    expect(JIANGUOYUN_BASE_URL + JIANGUOYUN_REMOTE_FOLDER).toBe('https://dav.jianguoyun.com/dav/GameSaveGo/');
  });

  it('never carries another provider password or URL into the preset', () => {
    const selected = selectCloudProvider({ provider: 'webdav', url: 'https://other.example/dav', username: 'old', password: 'secret', passwordConfigured: true }, 'jianguoyun');
    expect(selected).toMatchObject({ provider: 'jianguoyun', url: JIANGUOYUN_BASE_URL, username: '', password: '', passwordConfigured: false });
  });

  it('leaves a selected provider intact and clears credentials when leaving it', () => {
    const current = { provider: 'jianguoyun', url: JIANGUOYUN_BASE_URL, password: '', passwordConfigured: true };
    expect(selectCloudProvider(current, 'jianguoyun')).toBe(current);
    expect(selectCloudProvider(current, 'webdav')).toMatchObject({ provider: 'webdav', url: '', password: '', passwordConfigured: false });
  });

  it('recommends Jianguoyun only for an unset destination', () => {
    expect(recommendJianguoyunForUnset({ provider: 'local', url: '' })).toMatchObject({ provider: 'jianguoyun', url: JIANGUOYUN_BASE_URL });
    const existing = { provider: 'local', url: 'D:/Backups' };
    expect(recommendJianguoyunForUnset(existing)).toBe(existing);
    expect(recommendJianguoyunForUnset({ provider: 'jianguoyun', url: 'https://wrong.invalid', password: 'stale' })).toMatchObject({ url: JIANGUOYUN_BASE_URL, password: '' });
  });

  it('has actionable English and Chinese password, limit, and backup-only copy', () => {
    for (const language of ['en', 'zh-CN']) {
      for (const key of ['cloud.providers.jianguoyun', 'cloud.jianguoyun.appPassword', 'cloud.jianguoyun.passwordHelp', 'cloud.jianguoyun.limits', 'cloud.jianguoyun.backupOnly']) {
        expect(translate(language, key)).not.toBe(key);
      }
    }
    expect(translate('zh-CN', 'cloud.jianguoyun.passwordHelp')).toContain('不要填写坚果云登录密码');
    for (const failure of ['authentication', 'permission', 'quota', 'rate_limit', 'network', 'incomplete_inventory', 'unsafe_condition', 'integrity']) {
      for (const language of ['en', 'zh-CN']) {
        const key = `cloud.activity.jianguoyun.${failure}`;
        expect(translate(language, key)).not.toBe(key);
      }
    }
  });
});
