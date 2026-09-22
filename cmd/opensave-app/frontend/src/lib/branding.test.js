import { describe, expect, it } from 'vitest';
import { PRODUCT_NAME, PRODUCT_REPOSITORY_URL } from './branding.js';

describe('visible product identity', () => {
  it('uses the fork brand and repository', () => {
    expect(PRODUCT_NAME).toBe('GameSave Go');
    expect(PRODUCT_REPOSITORY_URL).toBe('https://github.com/lyx5710317/gamesave-go');
  });
});
