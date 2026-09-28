// A reloaded window keeps its token; a window whose token dies mid-run says so (ADR-053/054).
//
// The boot here is the RELOAD path: the token is already in sessionStorage and the URL has no key.
// Then Nib restarts under the page (a pinned `NIB_ADDR` port): every call answers 403 "no session".
// Before this the app kept looking usable while each action toasted a refusal. And a 403 that is NOT
// the session — a bad origin, a switched-off feature — must stay the caller's (R7-6).
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

let answer = 'ok';
const h = await boot({
  routes: {
    '/api/settings': () => (answer === 'ok' ? {}
      : new Response(JSON.stringify({ error: answer }), { status: 403 })),
  },
});

async function write() {
  const chk = h.document.getElementById('autoUpdateChk');
  chk.checked = !chk.checked;
  chk.dispatchEvent(new h.window.Event('change'));
  await h.settle();
}

test('a reload needs no trade: the stored token is used, on every request', () => {
  assert.ok(!h.calls.some((c) => c.url.endsWith('/api/launch')), 'a reload traded a key it does not have');
  const status = h.calls.find((c) => c.url.endsWith('/api/status'));
  assert.ok(status, 'the reloaded page never asked for its status');
  assert.equal(status.headers['X-CSRF-Token'], 'test-csrf');
  assert.equal(h.document.getElementById('launchOverlay').hidden, true);
});

test('a 403 that is not the session leaves the screen down', async () => {
  answer = 'bad origin';
  await write();
  assert.ok(h.calls.some((c) => c.url.endsWith('/api/settings') && c.method === 'POST'), 'no write was made');
  assert.equal(h.document.getElementById('launchOverlay').hidden, true,
    'a refusal that belongs to the caller put up "Open Nib again"');
});

test('a 403 "no session" puts up the screen, with the words for a lost connection', async () => {
  answer = 'no session';
  await write();
  assert.equal(h.document.getElementById('launchOverlay').hidden, false);
  assert.match(h.document.getElementById('launchText').textContent, /lost its connection/);
});
