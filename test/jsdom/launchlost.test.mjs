// A page whose session dies mid-run says so (ADR-053).
//
// A restart on a pinned port (`NIB_ADDR`) leaves the page holding a cookie the new process never
// issued, and every call answers 403 "no session". Before this the app kept looking usable while
// each action toasted a refusal; the page owes the same screen a tab with no session gets.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

let dead = false;
const h = await boot({
  routes: {
    '/api/settings': () => (dead
      ? new Response('{"error":"no session"}', { status: 403 })
      : {}),
  },
});

test('the page boots with a session (the setup this assertion needs)', () => {
  assert.equal(h.document.getElementById('launchOverlay').hidden, true);
});

test('a 403 "no session" puts up the open-Nib-again screen', async () => {
  dead = true;
  const chk = h.document.getElementById('autoUpdateChk');
  chk.checked = !chk.checked;
  chk.dispatchEvent(new h.window.Event('change'));
  await h.settle();
  assert.ok(h.calls.some((c) => c.url.endsWith('/api/settings') && c.method === 'POST'), 'no write was made');
  assert.equal(h.document.getElementById('launchOverlay').hidden, false);
});
