import { derived, writable } from 'svelte/store';
import en from '../locales/en.js';
import zhCN from '../locales/zh-CN.js';

export const DEFAULT_LOCALE = 'en';
export const SUPPORTED_LOCALES = ['en', 'zh-CN'];
// Reserved so future work can add catalogs without changing persisted values.
export const PLANNED_LOCALES = ['zh-TW', 'ja-JP'];
export const STORAGE_KEY = 'opensave.locale';

export const messages = { en, 'zh-CN': zhCN };

export function resolveLocale(candidate = '') {
  const normalized = String(candidate).trim().replaceAll('_', '-').toLowerCase();
  if (
    normalized === 'zh-cn' ||
    normalized === 'zh-sg' ||
    normalized === 'zh-hans' ||
    normalized.startsWith('zh-hans-')
  ) {
    return 'zh-CN';
  }
  return DEFAULT_LOCALE;
}

export function interpolate(message, params = {}) {
  return message.replace(/\{([A-Za-z0-9_]+)\}/g, (match, key) =>
    Object.prototype.hasOwnProperty.call(params, key) ? String(params[key]) : match
  );
}

export function translate(language, key, params = {}, catalogs = messages) {
  const message = catalogs[language]?.[key] ?? catalogs[DEFAULT_LOCALE]?.[key];
  return message === undefined ? key : interpolate(message, params);
}

function getBrowserStorage() {
  try {
    return typeof window === 'undefined' ? null : window.localStorage;
  } catch {
    return null;
  }
}

function getBrowserLanguage() {
  if (typeof navigator === 'undefined') return '';
  return navigator.languages?.[0] ?? navigator.language ?? '';
}

function storedLocale(storage) {
  try {
    const saved = storage?.getItem(STORAGE_KEY);
    return SUPPORTED_LOCALES.includes(saved) ? saved : null;
  } catch {
    return null;
  }
}

export function createLocaleStore({
  storage = getBrowserStorage(),
  navigatorLanguage = getBrowserLanguage(),
  documentElement = typeof document === 'undefined' ? null : document.documentElement
} = {}) {
  let current = storedLocale(storage) ?? resolveLocale(navigatorLanguage);
  const store = writable(current);

  function apply(value, persist = true) {
    current = SUPPORTED_LOCALES.includes(value) ? value : DEFAULT_LOCALE;
    store.set(current);
    if (documentElement) documentElement.lang = current;
    if (persist) {
      try {
        storage?.setItem(STORAGE_KEY, current);
      } catch {
        // Language switching must still work if storage is unavailable.
      }
    }
  }

  apply(current, false);
  return {
    subscribe: store.subscribe,
    set: apply,
    update(updater) {
      apply(updater(current));
    }
  };
}

export const locale = createLocaleStore();
export const t = derived(locale, (language) => (key, params = {}) =>
  translate(language, key, params)
);
