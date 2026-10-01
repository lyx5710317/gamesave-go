import { describe, expect, it } from 'vitest';
import { flingTrainerSearchURL } from './links.js';

describe('FLiNG trainer search', () => {
  it.each(['Black Myth: Wukong', '黑神话：悟空', 'Game & DLC + #1/?', 'Game II: Deluxe Edition'])('preserves the game name as a single search parameter: %s', (name) => {
    const url = new URL(flingTrainerSearchURL(`  ${name}  `));
    expect(url.origin).toBe('https://flingtrainer.com');
    expect(url.pathname).toBe('/');
    expect([...url.searchParams]).toEqual([['s', name]]);
    expect(url.hash).toBe('');
  });

  it.each(['', '   ', null, undefined, 42])('disables search for an unusable name: %s', (name) => {
    expect(flingTrainerSearchURL(name)).toBe('');
  });
});
