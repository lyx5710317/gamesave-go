import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { translate } from './i18n.js';
import { PRIMARY_NAV, SETTINGS_OWNED_VIEWS, primaryNavIsActive } from './navigation.js';

describe('Windows-first primary navigation', () => {
  it('focuses on the four everyday workflows in their intended order', () => {
    expect(PRIMARY_NAV.map((item) => item.id)).toEqual(['home', 'cloud', 'activity', 'settings']);
  });

  it('keeps device sync and the changelog out of the primary navigation', () => {
    const ids = PRIMARY_NAV.map((item) => item.id);
    expect(ids).not.toContain('devices');
    expect(ids).not.toContain('changelog');
    expect(SETTINGS_OWNED_VIEWS).toEqual(new Set(['settings', 'devices', 'changelog']));
  });

  it('keeps Settings highlighted while an advanced destination is open', () => {
    expect(primaryNavIsActive('devices', 'settings')).toBe(true);
    expect(primaryNavIsActive('changelog', 'settings')).toBe(true);
    expect(primaryNavIsActive('cloud', 'settings')).toBe(false);
    expect(primaryNavIsActive('cloud', 'cloud')).toBe(true);
  });

  it('offers device sync directly before adding a folder on Home', () => {
    const home = readFileSync(new URL('../views/Home.svelte', import.meta.url), 'utf8');
    const deviceButton = '<button class="btn primary" on:click={() => navigate(\'devices\')}>+ {$t(\'home.addDevice\')}</button>';
    const folderButton = '<button class="btn primary" on:click={() => (showAdd = !showAdd)}>+ {$t(\'home.trackFolder\')}</button>';
    expect(home).toContain(deviceButton);
    expect(home).toContain(folderButton);
    expect(home.indexOf(deviceButton)).toBeLessThan(home.indexOf(folderButton));
    expect(translate('zh-CN', 'home.addDevice')).toBe('添加设备');
    expect(translate('en', 'home.addDevice')).toBe('Add device');
  });

  it('does not offer upstream sync-component builds as GameSave Go updates', () => {
    const devices = readFileSync(new URL('../views/Devices.svelte', import.meta.url), 'utf8');
    expect(devices).not.toContain('peer.hasNewerBuild');
    expect(devices).not.toContain('peer.appVersion');
    expect(devices).not.toContain('installFromPeer');
    expect(translate('zh-CN', 'settings.updates.source')).toContain('GameSave Go');
    expect(translate('en', 'settings.updates.source')).toContain('GameSave Go');
  });
});
