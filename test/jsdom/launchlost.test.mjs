// A reloaded window keeps its token; a window whose token dies mid-run says so (ADR-053/054).
//
// The boot here is the RELOAD path: the token is already in sessionStorage and the URL has no key.
// Then Nib restarts under the page (a pinned `NIB_ADDR` port): every call answers 403 "no session".
// Before this the app kept looking usable while each action toasted a refusal. And a 403 that is NOT
// the session — a bad origin, a switched-off feature — must stay the caller's (R7-6).
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot, BOOT_ROUTES } from './boot.mjs';

let answer = 'ok';
let nibGone = false;
const h = await boot({
  routes: {
    // A Nib that has quit answers nothing: fetch rejects with a TypeError, as a browser's does.
    '/api/status': () => {
      if (nibGone) throw new TypeError('Failed to fetch');
      return BOOT_ROUTES['/api/status'];
    },
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

// /pending 731 (7): the window stream's error asks `checkSession`, which swallowed a fetch that got
// no answer at all — so a window whose Nib had quit or crashed never said so. A drop Nib DOES answer
// after (a reconnect blip) must leave the screen down; that half is the control.
test('a window stream drop with Nib still answering leaves the screen down', async () => {
  assert.ok(h.failWindowStream(), 'setup: the page set no onerror on its window stream');
  await h.settle();
  assert.equal(h.document.getElementById('launchOverlay').hidden, true,
    'a stream blip Nib answered after put up "Open Nib again"');
});

test('a window stream drop with Nib gone says Nib could not be reached', async () => {
  nibGone = true;
  assert.ok(h.failWindowStream(), 'setup: the page set no onerror on its window stream');
  await h.settle();
  nibGone = false;
  assert.equal(h.document.getElementById('launchOverlay').hidden, false,
    'the stream dropped and nothing answered, and the window still looks usable');
  assert.match(h.document.getElementById('launchText').textContent, /Could not reach Nib/);
});

test('a 403 "no session" puts up the screen, with the words for a lost connection', async () => {
  answer = 'no session';
  await write();
  assert.equal(h.document.getElementById('launchOverlay').hidden, false);
  assert.match(h.document.getElementById('launchText').textContent, /lost its connection/);
});
