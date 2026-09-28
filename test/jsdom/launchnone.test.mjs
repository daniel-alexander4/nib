// A window with no launch key and no session says so, and does nothing else (ADR-053).
//
// This is the tab opened by hand at Nib's address in a browser that never traded a key. The server
// answers it 403 everywhere that matters; what the page owes is to say what works — open Nib again
// — rather than draw a first-run wizard or an empty app whose every action fails.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

const h = await boot({
  routes: {
    '/api/launch': () => new Response('{"error":"no session"}', { status: 403 }),
  },
});

test('it shows the open-Nib-again screen', () => {
  assert.equal(h.document.getElementById('launchOverlay').hidden, false);
  assert.equal(h.document.getElementById('authOverlay').hidden, true,
    'the unlock wizard is up too — it would offer actions this page cannot perform');
});

test('it asks with its cookie and never trades a key it does not have', () => {
  const launch = h.calls.filter((c) => c.url.endsWith('/api/launch'));
  assert.deepEqual(launch.map((c) => c.method), ['GET']);
});

test('it does not go on to boot the app', () => {
  assert.ok(!h.calls.some((c) => c.url.endsWith('/api/status')),
    `status was fetched: ${h.calls.map((c) => c.url).join(', ')}`);
});
