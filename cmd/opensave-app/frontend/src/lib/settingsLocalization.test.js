import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { parse } from 'svelte/compiler';
import en from '../locales/en.js';
import zhCN from '../locales/zh-CN.js';

const source = (relative) => readFileSync(new URL(relative, import.meta.url), 'utf8');

function visitMarkup(node, callback) {
  if (!node) return;
  if (Array.isArray(node)) {
    node.forEach((child) => visitMarkup(child, callback));
    return;
  }
  callback(node);
  for (const key of ['children', 'else', 'then', 'pending', 'catch']) {
    if (node[key]) visitMarkup(node[key], callback);
  }
}

describe('Settings localization and obsolete links', () => {
  it('has no hard-coded English visible text in Settings markup', () => {
    const markup = parse(source('../views/Settings.svelte')).html;
    const untranslated = [];
    visitMarkup(markup, (node) => {
      if (node.type === 'Text' && /[A-Za-z]{3}/.test(node.data.trim())) {
        untranslated.push(node.data.trim());
      }
      for (const attribute of node.attributes ?? []) {
        if (!['title', 'placeholder', 'aria-label'].includes(attribute.name) || !Array.isArray(attribute.value)) continue;
        const literal = attribute.value.filter((value) => value.type === 'Text').map((value) => value.data).join('');
        if (/[A-Za-z]{3}\s+[A-Za-z]{3}/.test(literal)) untranslated.push(literal);
      }
    });
    expect(untranslated).toEqual([]);
  });

  it('has Chinese text for every Settings key and matching English keys', () => {
    const settingKeys = Object.keys(en).filter((key) => key.startsWith('settings.'));
    expect(settingKeys).toEqual(Object.keys(zhCN).filter((key) => key.startsWith('settings.')));
    expect(settingKeys.filter((key) => !/\p{Script=Han}/u.test(zhCN[key]))).toEqual([]);
  });

  it('keeps translated interpolation fields aligned with English', () => {
    expect(Object.keys(en)).toEqual(Object.keys(zhCN));
    const fields = (value) => [...value.matchAll(/\{([A-Za-z0-9_]+)\}/g)].map((match) => match[1]).sort();
    for (const key of Object.keys(en)) {
      expect(fields(zhCN[key]), key).toEqual(fields(en[key]));
    }
  });

  it('keeps Settings-linked device and relay views free of hard-coded English instructions', () => {
    const untranslated = [];
    for (const file of ['../views/Devices.svelte', '../views/InternetSync.svelte']) {
      visitMarkup(parse(source(file)).html, (node) => {
        if (node.type !== 'Text') return;
        const text = node.data.trim().replaceAll('GameSave Go', '').replaceAll('OPENSAVE_RELAY_URL', '');
        if (/[A-Za-z]{3}/.test(text)) untranslated.push(`${file}: ${text}`);
      });
    }
    expect(untranslated).toEqual([]);
    expect(source('../views/InternetSync.svelte')).not.toContain('github.com/Liquid-co/OpenSave');
  });

  it('localizes game details, conflict choices and their safety explanations', () => {
    const untranslated = [];
    for (const file of [
      '../views/GameDetail.svelte',
      '../components/ConflictModal.svelte',
      '../components/LocationConflictModal.svelte',
      '../components/PairingBanner.svelte',
      '../components/StatusBar.svelte'
    ]) {
      visitMarkup(parse(source(file)).html, (node) => {
        if (node.type === 'Text' && /[A-Za-z]{3}/.test(node.data.trim())) {
          untranslated.push(`${file}: ${node.data.trim()}`);
        }
      });
    }
    expect(untranslated).toEqual([]);
    const keys = Object.keys(en).filter((key) => /^(game|conflict|locationConflict)\./.test(key));
    expect(keys).toEqual(Object.keys(zhCN).filter((key) => /^(game|conflict|locationConflict)\./.test(key)));
    expect(keys.filter((key) => !/\p{Script=Han}/u.test(zhCN[key]))).toEqual([]);
  });

  it('removes upstream promotion from every app surface while keeping project links', () => {
    const surfaces = [
      '../views/Settings.svelte',
      '../views/Changelog.svelte',
      '../components/AboutModal.svelte',
      '../components/WhatsNewModal.svelte',
      './links.js'
    ].map(source).join('\n');
    expect(surfaces).not.toMatch(/DiscordBanner|DISCORD_URL|DONATE_URL|discord\.gg|opensave\.gumroad|support-tab|Upstream Discord/);
    expect(source('./links.js')).toContain('PRODUCT_REPOSITORY_URL');
    expect(Object.values(zhCN).filter((value) => /OpenSave/.test(value))).toEqual([]);
  });
});
