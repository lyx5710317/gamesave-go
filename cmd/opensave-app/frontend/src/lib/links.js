// Every destination the app can send someone to outside itself, in one file.
// A link that leaves the app is one worth being able to audit at a glance
// rather than hunting for a string literal in a view.
//
// All of these are opened with native.openExternal — in the system browser,
// never inside the app window.

export { PRODUCT_REPOSITORY_URL as GITHUB_URL } from './branding.js';

// Use the official search form rather than guessing a trainer page slug.
export function flingTrainerSearchURL(name) {
  const query = typeof name === 'string' ? name.trim() : '';
  const english = /[a-z]/i.test(query) && !/[^\p{Script=Latin}\P{L}]/u.test(query);
  return english ? `https://flingtrainer.com/?s=${encodeURIComponent(query)}` : '';
}
