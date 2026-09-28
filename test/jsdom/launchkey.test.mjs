// A window Nib opened gets its token from the launch key in its URL fragment, and from nowhere else
// (ADR-053/054, `/pending 685` and `704`).
//
// The server half — no route but three answers without the token, and a key trades once — is pinned
// in Go (`launch_test.go`). What only the page can get wrong is here: that it takes the key out of
// the address bar BEFORE it sends it anywhere, sends it in the header the server reads, keeps the
// token where a reload finds it, and sends that token — and no other — on every request, including
// the loads the browser makes by itself.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

let hashAtTrade = null;
const h = await boot({
  search: '#k=the-launch-key',
  token: null,
  routes: {
    '/api/settings': {},
    // A status answer carrying a DIFFERENT token, so a page that took its token from status (the
    // defect ADR-053 closed) would be visible in what its writes carry (the P08 phase-close review, R7-4).
    '/api/status': { state: 'ready', csrf: 'status-token', version: 'test', autoUpdate: false, updateCheckLocked: false, ghostscript: false, libreoffice: false, advanced: { ceremony: true, discovery: true, rendezvous: true, timestamp: true } },
    '/api/launch': (opts) => {
      hashAtTrade = globalThis.location.hash;
      return opts.method === 'POST' && opts.headers?.['X-Nib-Launch'] === 'the-launch-key'
        ? { csrf: 'traded-token' }
        : new Response('{"error":"unknown or used launch key"}', { status: 403 });
    },
  },
});

test('the page trades the fragment key, once, in the X-Nib-Launch header', () => {
  const trades = h.calls.filter((c) => c.url.endsWith('/api/launch') && c.method === 'POST');
  assert.equal(trades.length, 1, `expected one trade, got ${trades.length}`);
  assert.equal(trades[0].headers['X-Nib-Launch'], 'the-launch-key');
});

test('the key is out of the address bar BEFORE it is sent anywhere', () => {
  // Read at the moment of the trade, not afterwards (R7-7): a page that stripped it after the
  // awaited trade would leave the final hash empty and pass a check made at the end.
  assert.equal(hashAtTrade, '', `the fragment was still in the address bar when the key was traded: ${hashAtTrade}`);
  assert.equal(h.window.location.hash, '');
});

test('the token is kept where a reload of this tab finds it', () => {
  assert.equal(h.window.sessionStorage.getItem('nib-token'), 'traded-token');
});

test('every request after the trade carries the traded token — reads included, never status\'s', () => {
  const after = h.calls.filter((c) => c.url.includes('/api/') && !c.url.endsWith('/api/launch'));
  assert.ok(after.some((c) => c.url.endsWith('/api/status')), 'setup: the page never asked for its status');
  for (const c of after) {
    assert.equal(c.headers['X-CSRF-Token'], 'traded-token', `${c.method} ${c.url} carried ${c.headers['X-CSRF-Token']}`);
  }
});

test('the window stream carries the token in its URL (an EventSource cannot set a header)', () => {
  const urls = h.windowStreamURLs();
  assert.equal(urls.length, 1, `window streams: ${urls}`);
  assert.match(urls[0], /^\/api\/window\?auth=traded-token$/);
});

test('a write carries the traded token', async () => {
  const chk = h.document.getElementById('autoUpdateChk');
  chk.checked = !chk.checked;
  chk.dispatchEvent(new h.window.Event('change'));
  await h.settle();
  const w = h.calls.filter((c) => c.url.endsWith('/api/settings') && c.method === 'POST').pop();
  assert.ok(w, 'no write was made');
  assert.equal(w.headers['X-CSRF-Token'], 'traded-token');
});
