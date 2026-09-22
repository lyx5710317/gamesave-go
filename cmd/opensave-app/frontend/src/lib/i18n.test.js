import { describe, expect, it } from 'vitest';
import { get } from 'svelte/store';
import {
  DEFAULT_LOCALE,
  STORAGE_KEY,
  createLocaleStore,
  locale,
  messages,
  resolveLocale,
  t,
  translate
} from './i18n.js';

function memoryStorage(initial = {}) {
  const values = new Map(Object.entries(initial));
  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value)
  };
}

function currentValue(store) {
  let value;
  const unsubscribe = store.subscribe((next) => (value = next));
  unsubscribe();
  return value;
}

describe('translations', () => {
  it('provides English and Simplified Chinese strings', () => {
    expect(translate('en', 'nav.games')).toBe('Games');
    expect(translate('zh-CN', 'nav.games')).toBe('游戏');
  });

  it('keeps both shipped catalogs on the same set of keys', () => {
    expect(Object.keys(messages['zh-CN']).sort()).toEqual(Object.keys(messages.en).sort());
  });

  it('falls back to English before returning the key', () => {
    const catalogs = { en: { present: 'English fallback' }, 'zh-CN': {} };
    expect(translate('zh-CN', 'present', {}, catalogs)).toBe('English fallback');
    expect(translate('zh-CN', 'missing', {}, catalogs)).toBe('missing');
  });

  it('interpolates parameters without erasing missing values', () => {
    const catalogs = { en: { greeting: 'Hello {name}; {missing}' } };
    expect(translate('en', 'greeting', { name: 'Ada' }, catalogs)).toBe('Hello Ada; {missing}');
  });
});

describe('locale selection', () => {
  it('updates the active translator without recreating the app', () => {
    locale.set('en');
    expect(get(t)('nav.home')).toBe('Home');
    locale.set('zh-CN');
    expect(get(t)('nav.home')).toBe('主页');
    locale.set(DEFAULT_LOCALE);
  });

  it('maps Simplified Chinese system locales and defaults everything else to English', () => {
    expect(resolveLocale('zh-Hans-CN')).toBe('zh-CN');
    expect(resolveLocale('zh_SG')).toBe('zh-CN');
    expect(resolveLocale('zh-TW')).toBe(DEFAULT_LOCALE);
    expect(resolveLocale('fr-FR')).toBe(DEFAULT_LOCALE);
  });

  it('applies changes immediately and persists them across store recreation', () => {
    const storage = memoryStorage();
    const documentElement = { lang: '' };
    const first = createLocaleStore({ storage, navigatorLanguage: 'en-US', documentElement });
    first.set('zh-CN');

    expect(currentValue(first)).toBe('zh-CN');
    expect(documentElement.lang).toBe('zh-CN');
    expect(storage.getItem(STORAGE_KEY)).toBe('zh-CN');

    const restarted = createLocaleStore({ storage, navigatorLanguage: 'en-US' });
    expect(currentValue(restarted)).toBe('zh-CN');
  });

  it('ignores unsupported persisted values', () => {
    const storage = memoryStorage({ [STORAGE_KEY]: 'ja-JP' });
    const store = createLocaleStore({ storage, navigatorLanguage: 'zh-CN' });
    expect(currentValue(store)).toBe('zh-CN');
  });
});
