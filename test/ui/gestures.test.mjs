// A pointer-capture gesture in flight when the user switches documents.
//
// **Why this file exists at tier 3 and nowhere else.** The defect is made of two things
// neither lower tier has. The first is `setPointerCapture`: it is released when the
// captured element leaves the DOM, not when it is hidden, and an inactive view is
// `display:none` and still in the document — so a stamp or flag being dragged keeps
// receiving `pointermove` after the switch. jsdom implements no `PointerEvent` and no
// `setPointerCapture` at all, so tier 2 cannot even start the gesture. The second is
// LAYOUT: the continuation's damage is computed from `pv.div.clientWidth`, and every
// `clientWidth` at tier 2 is 0 — the arithmetic there produces NaN rather than the
// wrong-document geometry that is the actual bug.
//
// **What it drives.** Two documents open, a flag placed on the second, a drag started
// on that flag, the FIRST document activated mid-drag, and then the pointer moved and
// released. Both halves of the damage are asserted: the flag must not move (the
// continuation read the new document's page geometry), and the newly active document's
// undo stack must not have grown (the release recorded its move onto whichever view was
// active by then).
//
// **Which half is proven red, and which is not.** Against the reintroduced defect the
// position assertion fails, measured: the flag went from 334.29px to 554.29px after the
// switch. The undo-stack assertion has NEVER been observed red and cannot be with this
// stimulus, which is worth saying rather than leaving for someone to assume: a move is
// only recorded when `f.frac` actually changed, so any run that reaches the recording
// has already failed the position check above it. It is kept as the second lock on the
// same door — a future fix that stops the field moving but leaves the release recording
// would pass the first check and fail this one — and NOT as an independently earned
// pass. Read it as a consistency check with a named ceiling.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const A = writeFixture('gest-a.pdf', { pages: 2, label: 'doc A page' });
const B = writeFixture('gest-b.pdf', { pages: 6, label: 'doc B page' });

const h = await launch();
const page = h.page;

// This file sorts FIRST in test/ui, and the tier-3 files share ONE nib process — so
// whatever it leaves open is the starting state for lifecycle.test.mjs, whose first
// assertion is that the app looks like a fresh launch. Leaving two documents and an
// armed flag tool behind failed 13 sibling tests on the first run of this file. The
// cleanup is therefore part of the test, not tidiness: close every document before
// handing the server on. Wrapped, because a cleanup that throws would mask the
// assertion failure that got us here.
after(async () => {
  try {
    // Conditioned on `has-doc`, NOT on the tab count: the strip is empty at one document
    // as well as at zero (syncTabs returns early below two), so a tab-count loop stops
    // with the last document still open — which is exactly how this cleanup failed first
    // time, silently, while looking like it had run.
    for (let i = 0; i < 8 && await page.$eval('#viewerWrap', (el) => el.className) === 'has-doc'; i++) {
      await h.closeDocument();
    }
  } catch { /* the assertion that already failed is the one worth reporting */ }
  await h.browser.close();
});

const tabSel = (n) => `#tabstrip .tab:nth-child(${n})`;
const switched = (pages) => page.waitForFunction(
  (n) => document.querySelector('.pageCount').textContent === `/ ${n}`,
  pages,
);

// The flag's own box, read off the element the drag is holding. Its `style.left` is
// what layoutField writes, so it is the direct readout of `f.frac` — no app internals
// needed, and it moves if and only if the field did.
const flagBox = () => page.evaluate(() => {
  const el = document.querySelector('.ovl-marker');
  return el ? { left: el.style.left, top: el.style.top, present: true } : { present: false };
});

// The active document's undo button. reflectUndoControls reads the ACTIVE view's
// overlayHistory, so an undo command recorded onto the wrong document shows up here as
// a button that became enabled without the user editing that document.
const undoEnabled = () => page.$eval('#undoBtn', (b) => !b.disabled);

test('a drag in flight when the user switches documents neither moves the flag nor records onto the new document', async () => {
  await h.openDocument(A, 2);
  await h.openDocument(B, 6);
  assert.equal(await page.$$eval('#tabstrip .tab', (e) => e.length), 2,
    'setup: two documents are not open, so there is nothing to switch between');

  // Place a flag on B. The Flags panel is available in COLLABORATE mode only
  // (SIDEBAR_FOR) — the app's own navigation model, and switching modes is how a user
  // reaches the tool. Then the Sign flag tool, then a click on the page: the tool arms
  // `markerMode` and the next page click drops a flag there.
  await h.mode('collaborate');
  await page.click('.tab[data-panel="flags"]');
  await page.click('.markers button[data-marker="sign"]');
  const pageBox = await page.$eval('.viewerContainer:not([hidden]) .page',
    (el) => { const r = el.getBoundingClientRect(); return { x: r.x, y: r.y, w: r.width, h: r.height }; });
  await page.mouse.click(pageBox.x + pageBox.w * 0.4, pageBox.y + pageBox.h * 0.4);
  await page.waitForSelector('.viewerContainer:not([hidden]) .ovl-marker');

  const marker = await page.$eval('.viewerContainer:not([hidden]) .ovl-marker',
    (el) => { const r = el.getBoundingClientRect(); return { x: r.x + r.width / 2, y: r.y + r.height / 2 }; });

  // The setup assertions that make the two below meaningful: A is the document we are
  // about to switch to, and its undo button is disabled right now — otherwise "still
  // disabled" at the end proves nothing.
  const before = await flagBox();
  assert.equal(before.present, true, 'setup: no flag was placed, so there is no gesture to start');

  // Start the drag and move once WHILE B is still active — this is the control: the
  // gesture must work normally up to the switch, or the test could pass by the drag
  // never having started.
  await page.mouse.move(marker.x, marker.y);
  await page.mouse.down();
  await page.mouse.move(marker.x + 40, marker.y + 40, { steps: 4 });
  const moved = await flagBox();
  assert.notEqual(moved.left, before.left,
    'setup: the drag did not move the flag at all before the switch, so the abort below is asserting against a gesture that never started');

  // The switch, mid-drag, with the button still down. Driven as an element click rather
  // than through the mouse, which is busy holding the capture.
  await page.$eval(tabSel(1), (el) => el.click());
  await switched(2);
  assert.equal(await undoEnabled(), false,
    'setup: the newly activated document already has an enabled undo button, so the assertion below cannot distinguish a stray command from the starting state');
  const atSwitch = await flagBox();

  // The continuation: pointer still captured by a flag that now belongs to a hidden
  // document, moving over a page that belongs to a different one.
  await page.mouse.move(marker.x + 260, marker.y + 180, { steps: 6 });
  await page.mouse.up();

  const afterMove = await flagBox();
  assert.equal(afterMove.left, atSwitch.left,
    `the flag moved from ${atSwitch.left} to ${afterMove.left} after its document was switched away. setPointerCapture survives display:none, so the drag kept receiving pointermove and laid the field out against the NEWLY ACTIVE document's page geometry — one document's flag positioned by another document's pages.`);
  assert.equal(afterMove.top, atSwitch.top,
    `the flag moved vertically from ${atSwitch.top} to ${afterMove.top} after the switch — same cause as the horizontal check above.`);

  assert.equal(await undoEnabled(), false,
    'the document switched TO has an enabled undo button after a drag that happened on a different document. The gesture\'s pointerup called recordMove with no owner, so the move command landed on whichever view was active at release — Ctrl+Z on this document would now undo an edit made to another one.');
});
