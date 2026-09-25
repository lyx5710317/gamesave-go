import { expect, it } from 'vitest';
import { readFileSync } from 'node:fs';

it('allows another update attempt after an asynchronous install failure', () => {
  const app = readFileSync(new URL('../App.svelte', import.meta.url), 'utf8');
  expect(app).toContain("$: if ($appUpdate?.state === 'error') installStarted = false;");
});
