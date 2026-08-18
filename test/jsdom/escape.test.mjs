// Escape closes what is open — the dialogs and the first-run intro.
//
// Before v1.108.16 the ONLY thing in web/app.js bound to Escape was closeMenu(): none of
// the 37 *Modal dialogs closed on it, and the intro overlay had no close control of any
// kind. Its only stated exit was "Click anywhere outside this box to continue", in 12px
// muted text — so a keyboard-only or screen-reader user on first run had no announced way
// past it to the setup form beneath, on every platform, on every new install.
//
// **The dialog test asserts the CLEANUP, not just the hiding.** Escape clicks the
// dialog's own cancel rather than hiding the element, because hiding would skip what each
// cancel does — dropping a pending export's bytes, disarming a live listener, tearing
// down a second pdf.js document. A test that only checked `hidden` would pass against the
// cheap wrong implementation, which is the one someone would naturally write.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

const h = await boot({ routes: {} });
const { document: doc, settle } = h;

const esc = () => {
  doc.dispatchEvent(new h.window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
};

test('Escape dismisses the first-run intro overlay', async () => {
  const intro = doc.getElementById('introOverlay');
  assert.ok(intro, 'there is no #introOverlay in index.html');
  assert.ok(doc.getElementById('introGo'), 'the intro card has no button — its only exit is prose again');

  intro.hidden = false; // the state a first run puts it in
  esc();
  await settle();
  assert.equal(intro.hidden, true,
    'Escape did not dismiss the first-run intro. It sits at z-index 200 over the setup form and swallows every click aimed at what is beneath, so a user who does not discover the backdrop click is stuck on first run.');
});

test('Escape closes an open dialog by clicking its cancel, so the cancel\'s cleanup runs', async () => {
  const modal = doc.getElementById('saveAsModal');
  const cancel = doc.getElementById('saveAsCancel');
  assert.ok(modal && cancel, 'setup: the Save As dialog is not in index.html under the expected ids');

  // A spy on the cancel, because "the modal is hidden" is true for BOTH the right
  // implementation and the wrong one. saveAsCancel is what drops the pending export's
  // bytes; an Escape that merely hid the dialog would leave them held.
  let cancelled = 0;
  cancel.addEventListener('click', () => { cancelled++; });

  modal.hidden = false;
  esc();
  await settle();

  assert.equal(modal.hidden, true, 'Escape left the dialog open');
  assert.equal(cancelled, 1,
    'Escape hid the dialog without going through its Cancel, so the cleanup that button performs did not run — for this dialog that means the pending export\'s bytes and chosen name stay held');
});

test('every dialog has a control Escape can reach', () => {
  // The mechanism is a convention — Escape looks for a button whose id ends in Cancel or
  // Close INSIDE the open dialog — and a convention with an exception is a dialog that
  // traps the user. Asserted across all of them rather than trusting the two driven
  // above, because the next dialog added is the one that will not have it.
  const modals = [...doc.querySelectorAll('div[id$="Modal"]')];
  assert.ok(modals.length >= 30, `only ${modals.length} dialogs found — this scan is not reading index.html`);
  const trapped = modals
    .filter((m) => !m.querySelector('button[id$="Cancel"], button[id$="Close"]'))
    .map((m) => m.id);
  assert.deepEqual(trapped, [],
    `these dialogs have no Cancel/Close button, so the Escape handler has nothing to click and they cannot be dismissed from the keyboard: ${trapped.join(', ')}`);
});
