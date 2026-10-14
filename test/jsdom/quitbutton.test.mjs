// ADR-169 — the power button ends the process AND the window.
//
// The window is the user's own browser's, so Nib cannot close it from outside; the page closes itself. What this
// tier can see is everything up to the browser's answer: that the page asks to close, what it shows in case the
// browser refuses, and that the close prompt does not ask a second time. Whether a real browser honours
// `window.close()` is tier 3's (`test/ui/quit.test.mjs`).
//
// One boot, three presses in order — refused by the user, refused by the server, accepted — because quitting is
// for the page's whole life: after the third there is no page left to press on.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot, BOOT_ROUTES } from './boot.mjs';

let answer = 500;
let nibGone = false;
const h = await boot({
  routes: {
    '/api/quit': () => new Response(null, { status: answer }),
    // A Nib that has quit answers nothing: fetch rejects with a TypeError, as a browser's does.
    '/api/status': () => {
      if (nibGone) throw new TypeError('Failed to fetch');
      return BOOT_ROUTES['/api/status'];
    },
  },
});
const { window, document } = h;
let closes = 0;
window.close = () => { closes++; };

const quits = () => h.calls.filter((c) => c.url.includes('/api/quit')).length;
const overlay = () => document.getElementById('launchOverlay');
const closePrompts = () => {
  const e = new window.Event('beforeunload', { cancelable: true });
  window.dispatchEvent(e);
  return e.defaultPrevented;
};

test('setup: something would be lost, so Quit asks and the close prompt is armed', () => {
  assert.ok(h.pushWindowEvent('armed', JSON.stringify({ armed: true, what: 'the lease' })), 'no window stream was listening');
  assert.equal(closePrompts(), true, 'setup: the close prompt is not armed, so "it stands down" below would be true of nothing');
});

test('Cancel in Quit\'s own dialog quits nothing and closes nothing', async () => {
  h.setConfirmAnswer(false);
  const asked = h.confirms.length;
  document.getElementById('quitBtn').click();
  await h.settle();
  assert.equal(h.confirms.length, asked + 1, 'Quit did not ask, with a ceremony armed');
  assert.equal(quits(), 0, 'Quit was sent after the user said no');
  assert.equal(closes, 0, 'the window was asked to close after the user said no');
  assert.equal(overlay().hidden, true);
});

test('a quit the server refuses closes nothing', async () => {
  h.setConfirmAnswer(true);
  document.getElementById('quitBtn').click();
  await h.settle();
  assert.equal(quits(), 1, 'setup: the quit was not sent');
  assert.equal(closes, 0, 'the window closed although Nib did not quit');
  assert.equal(overlay().hidden, true, 'the window says Nib has quit although it has not');
  assert.equal(closePrompts(), true, 'the close prompt stood down although nothing quit');
});

test('an accepted quit says so, asks the browser to close the window, and the close prompt stands down', async () => {
  answer = 204;
  document.getElementById('quitBtn').click();
  await h.settle();
  assert.equal(quits(), 2, 'setup: the quit was not sent');
  assert.equal(closes, 1, 'the page did not ask the browser to close the window');
  assert.equal(overlay().hidden, false, 'nothing is shown for a browser that will not close the window');
  assert.equal(document.getElementById('launchTitle').textContent, 'Nib has quit');
  assert.match(document.getElementById('launchText').textContent, /You quit Nib/);
  assert.equal(closePrompts(), false,
    'the browser\'s own close prompt would follow Quit\'s dialog: the ceremony is still armed as far as this page knows');
});

test('the connection dropping afterwards does not turn the screen into an error', async () => {
  nibGone = true;
  const asked = h.calls.filter((c) => c.url.endsWith('/api/status')).length;
  assert.ok(h.failWindowStream(), 'the stream has no error handler, so nothing below was provoked');
  await h.settle();
  assert.ok(h.calls.filter((c) => c.url.endsWith('/api/status')).length > asked,
    'setup: the dropped stream did not make the page ask whether Nib is there, so the error screen was never at stake');
  assert.match(document.getElementById('launchText').textContent, /You quit Nib/,
    'after a quit the window says it could not reach Nib — which is what it said before this change');
  assert.equal(closes, 1, 'the window was asked to close more than once');
});
