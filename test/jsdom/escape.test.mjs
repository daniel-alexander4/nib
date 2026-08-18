// Escape closes what is open — the dialogs and the first-run intro.
//
// Before v1.108.16 the ONLY thing in web/app.js bound to Escape was closeMenu(): none of
// the 37 *Modal dialogs closed on it, and the first-run intro overlay had no close control
// of any kind — a keyboard-only or screen-reader user had no announced way past it to the
// setup form beneath. (That overlay is gone as of v1.109.3, folded into the setup card;
// what remains here is the dialog half, which is the larger one.)
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

// The first-run overlay this file used to test is GONE, and that is the stronger fix.
//
// v1.108.16 gave it a button and an Escape handler, because it had no close control of any
// kind and trapped a keyboard-only or screen-reader user on first run. v1.109.3 folded its
// content into the setup card instead: there is no second overlay to dismiss, so there is
// nothing to be trapped by and no Escape special-case to keep working.
//
// Kept as an assertion rather than deleted with its subject. "Escape dismisses the intro"
// stopped being a claim about anything the moment the intro stopped existing, and a test
// quietly removed alongside the thing it tested is how a property gets lost — the fold was
// supposed to REMOVE the trap, and this is what says it did rather than that someone
// deleted the check.
test('there is no separate first-run overlay left to trap anyone', () => {
  assert.equal(doc.getElementById('introOverlay'), null,
    'the first-run intro overlay is back as a separate stacked overlay — it sat at z-index 200 over the setup form and swallowed every click aimed at it');
  assert.ok(doc.getElementById('introBlock'),
    'the first-run explanation is gone entirely — it was folded INTO the setup card, not deleted, and a user asked to choose an SSH key needs to be told what it is for');
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
