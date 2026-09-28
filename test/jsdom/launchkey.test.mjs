// A window Nib opened gets its credentials from the launch key in its URL fragment, and from
// nowhere else (ADR-053, `/pending 685`).
//
// The server half — `/api/status` no longer carries the token, and a key trades once — is pinned in
// Go (`launch_test.go`). What only the page can get wrong is here: that it reads the key, sends it
// in the header the server reads, takes the fragment OUT of the address bar before anything else
// can copy it, and writes with the token the trade returned rather than one from anywhere else.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

const h = await boot({
  search: '#k=the-launch-key',
  routes: {
    '/api/settings': {},
    // A distinct token, so a write carrying the default one would be visible.
    '/api/launch': (opts) => (opts.method === 'POST' && opts.headers?.['X-Nib-Launch'] === 'the-launch-key'
      ? { csrf: 'traded-token' }
      : new Response('{"error":"no session"}', { status: 403 })),
  },
});

test('the page trades the fragment key, once, in the X-Nib-Launch header', () => {
  const trades = h.calls.filter((c) => c.url.endsWith('/api/launch') && c.method === 'POST');
  assert.equal(trades.length, 1, `expected one trade, got ${trades.length}`);
  assert.equal(trades[0].headers['X-Nib-Launch'], 'the-launch-key');
});

test('the key is gone from the address bar', () => {
  assert.equal(h.window.location.hash, '', `the fragment is still there: ${h.window.location.href}`);
});

test('a status answer carrying a token is not where the token comes from', () => {
  // The trade is the one source: status is fetched after it, and the page reached ready.
  const iTrade = h.calls.findIndex((c) => c.url.endsWith('/api/launch'));
  const iStatus = h.calls.findIndex((c) => c.url.endsWith('/api/status'));
  assert.ok(iTrade >= 0 && iStatus > iTrade, 'status was asked before the session was taken');
  assert.equal(h.document.getElementById('launchOverlay').hidden, true);
});

test('a write carries the token the trade returned', async () => {
  const chk = h.document.getElementById('autoUpdateChk');
  chk.checked = !chk.checked;
  chk.dispatchEvent(new h.window.Event('change'));
  await h.settle();
  const w = h.calls.filter((c) => c.url.endsWith('/api/settings') && c.method === 'POST').pop();
  assert.ok(w, 'no write was made');
  assert.equal(w.headers['X-CSRF-Token'], 'traded-token');
});
