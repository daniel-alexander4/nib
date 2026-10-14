// ADR-169 — a window that did NOT press the power button closes too.
//
// Quit is pressed in one window and ends the process under all of them. The server tells every window on the
// stream each already holds (`event: quit`); this is the page's half, in a window that was only told.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

const h = await boot({});
const { window, document } = h;
let closes = 0;
window.close = () => { closes++; };

test('a window told that Nib is quitting says so, closes its stream and asks the browser to close it', async () => {
  // Something to lose, so the prompt's standing down is seen to happen.
  h.pushWindowEvent('armed', JSON.stringify({ armed: true }));
  const armed = new window.Event('beforeunload', { cancelable: true });
  window.dispatchEvent(armed);
  assert.equal(armed.defaultPrevented, true, 'setup: the close prompt is not armed');
  assert.equal(document.getElementById('launchOverlay').hidden, true, 'setup: the screen is already up');

  assert.ok(h.pushWindowEvent('quit', '{}'), 'nothing listens for the stream\'s quit event');
  await h.settle();

  assert.equal(closes, 1, 'the page did not ask the browser to close the window');
  assert.equal(document.getElementById('launchOverlay').hidden, false);
  assert.match(document.getElementById('launchText').textContent, /You quit Nib/);
  assert.equal(h.windowStreamsClosed(), true, 'the stream was left to reconnect to a process that is going');
  assert.equal(h.calls.filter((c) => c.url.includes('/api/quit')).length, 0, 'a window that was told sent a quit of its own');
  const e = new window.Event('beforeunload', { cancelable: true });
  window.dispatchEvent(e);
  assert.equal(e.defaultPrevented, false, 'the browser\'s close prompt would stop the window closing');
});

test('told twice, it leaves once', async () => {
  h.pushWindowEvent('quit', '{}');
  await h.settle();
  assert.equal(closes, 1);
});
