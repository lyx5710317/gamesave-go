import { describe, expect, it } from 'vitest';
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
});
