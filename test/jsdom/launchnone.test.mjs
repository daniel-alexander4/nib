// A window with no launch key and no token says so, and does nothing else (ADR-053/054).
//
// This is the tab opened by hand at Nib's address, in a browser tab that never traded a key. The
// server answers it 403 everywhere; what the page owes is to say what works — open Nib again —
// rather than draw a wizard or an empty app whose every action fails, and to ask the server nothing.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

const h = await boot({ token: null });

test('it shows the open-Nib-again screen, with the words for a window that never had a token', () => {
  assert.equal(h.document.getElementById('launchOverlay').hidden, false);
  assert.match(h.document.getElementById('launchText').textContent, /not connected to Nib/);
  assert.doesNotMatch(h.document.getElementById('launchText').textContent, /still there/,
    'it promises documents survived — it cannot know that (R7-2)');
  assert.equal(h.document.getElementById('authOverlay').hidden, true,
    'the unlock wizard is up too — it would offer actions this page cannot perform');
});

test('it asks the server nothing and opens no window stream', () => {
  assert.deepEqual(h.calls.filter((c) => c.url.includes('/api/')).map((c) => c.url), []);
  assert.deepEqual(h.windowStreamURLs(), []);
});

test('everything behind it is inert, and the screen is described to a screen reader', () => {
  const overlay = h.document.getElementById('launchOverlay');
  for (const el of h.document.body.children) {
    if (el !== overlay) assert.equal(el.inert, true, `${el.id || el.tagName} is still reachable behind the screen`);
  }
  assert.equal(overlay.getAttribute('aria-describedby'), 'launchText');
});
