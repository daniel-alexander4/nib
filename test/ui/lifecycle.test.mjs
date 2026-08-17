// P01's end-to-end acceptance, against the real binary in a real browser.
//
// These are the flows a human drove by hand while P01 was being built. This is
// the last tier: anything it cannot exercise is a standing gap to be filed, not
// delegated onward, so each `not exercised` here would be a real admission.
//
// Two clauses land here specifically because tier 2 could not reach them:
//   * the thumbnail grid (jsdom has no canvas, so its grid is empty either way);
//   * the overlayFields / overlayHistory edit signals (they need a placed
//     overlay, which needs layout).
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const h = await launch();
const { page } = h;
after(() => h.browser.close());

const DOC = writeFixture('doc.pdf', { pages: 3, label: 'doc A page' });
const OTHER = writeFixture('other.pdf', { pages: 5, label: 'doc B page' });
const BIG = writeFixture('big.pdf', { pages: 80, label: 'big page' });

const chrome = () => page.evaluate(() => ({
  wrap: document.getElementById('viewerWrap').className,
  empty: document.getElementById('empty').textContent,
  badge: document.getElementById('sigBadge').textContent,
  badgeClass: document.getElementById('sigBadge').className,
  saveDisabled: document.getElementById('saveBtn').disabled,
  saveTitle: document.getElementById('saveBtn').title,
  closeDisabled: document.getElementById('closeBtn').disabled,
  pageCount: document.querySelector('.pageCount').textContent,
  pageNum: document.querySelector('.pageNum').value,
  thumbs: document.querySelector('.thumbgrid:not([hidden])')?.children.length ?? 0,
  // CONTENT, not children: since P05.S05 each view's outline is a `.outlinelist` wrapper
  // inside the shared `#outline` panel, so `children.length` counts open views.
  outline: document.querySelectorAll('#outline .outline-edit, #outline a').length,
}));
const closeDoc = () => h.closeDocument(); // File mode first — see harness.mjs

// The clause tier 2 had to skip. Here the grid genuinely populates, so an empty
// grid after a Close is EARNED — under jsdom it is empty either way, which is a
// structural zero and would have been a green nobody paid for.
test('the empty state matches launch, thumbnail grid included', async () => {
  await h.openDocument(DOC, 3);
  await page.waitForFunction(() => document.querySelector('.thumbgrid:not([hidden])')?.children.length === 3);

  const open = await chrome();
  assert.equal(open.wrap, 'has-doc');
  assert.equal(open.pageCount, '/ 3');
  assert.equal(open.thumbs, 3, 'the grid must POPULATE before its emptiness means anything');
  assert.equal(open.saveDisabled, false);
  assert.ok(open.saveTitle.includes(DOC));

  h.dialogs.length = 0;
  await closeDoc();

  const shut = await chrome();
  assert.equal(shut.wrap, '');
  assert.equal(shut.empty, 'Open a PDF to begin.');
  assert.equal(shut.badge, 'no document');
  assert.equal(shut.badgeClass, 'badge badge-none');
  assert.equal(shut.saveDisabled, true);
  assert.equal(shut.saveTitle, 'Save (overwrites the original)');
  assert.equal(shut.closeDisabled, true);
  assert.equal(shut.pageCount, '/ 0');
  assert.equal(shut.pageNum, '1');
  assert.equal(shut.thumbs, 0);
  // The grid must still EXIST, emptied. `?? 0` reports 0 for a missing grid too, so a
  // teardown that removed the active view's container instead of clearing it would read as
  // "empty" and pass the line above — and the reopen test that follows checks pageCount but
  // never thumbs, so nothing else would catch it.
  assert.notEqual(await page.$('.thumbgrid'), null,
    'the active view kept no grid after a close — a reopen would render into a detached node');
  assert.equal(shut.outline, 0);
  assert.equal(shut.wrap, '');
  // A clean document: no prompt. Evidence only because the prompt is driven below.
  assert.equal(h.dialogs.length, 0, 'a freshly opened document must not prompt');
});

test('reopening after a close works normally', async () => {
  await h.openDocument(OTHER, 5);
  const c = await chrome();
  assert.equal(c.wrap, 'has-doc');
  assert.equal(c.pageCount, '/ 5', 'a different document must actually have opened');
  const undo = await page.evaluate(async () => (await (await fetch('/api/doc')).json()).canUndo);
  assert.equal(undo, false, 'the undo ring must not survive a close');
  h.dialogs.length = 0;
  await closeDoc();
});

// S02's first delegation, discharged. Driven on its own: no server history, no
// pdf.js annotation edits — only a placed overlay.
test('the close prompt fires from a placed overlay alone', async () => {
  await h.openDocument(DOC, 3);
  const before = await h.counts();
  assert.equal(before.overlays, 0, 'setup: no overlays yet');

  await h.placeMarker('date');
  const after = await h.counts();
  assert.equal(after.markers, 1, 'setup: a marker must actually be placed');

  h.dialogs.length = 0;
  h.answerDialogs(false); // cancel — this doubles as the cancel-is-safe check
  await closeDoc();
  assert.equal(h.dialogs.length, 1, 'a document with a placed overlay must prompt');
  assert.match(h.dialogs[0], /since the last save/);

  // Cancel left everything alone.
  const kept = await chrome();
  assert.equal(kept.wrap, 'has-doc', 'cancelling must not close the document');
  assert.equal((await h.counts()).markers, 1, 'cancelling must not discard the overlay');
  const pdfStatus = await page.evaluate(async () => (await fetch('/api/pdf')).status);
  assert.equal(pdfStatus, 200, 'the server must still hold the document after a cancel');

  h.answerDialogs(true);
  await closeDoc();
  assert.equal((await chrome()).wrap, '', 'confirming must close it');
});

// S02's second delegation. The care this needs: placing THEN deleting must leave
// overlayHistory non-empty with no fields left — otherwise it is really the test
// above wearing a different name.
test('the close prompt fires from overlay history alone, with no overlays left', async () => {
  await h.openDocument(DOC, 3);
  await h.placeMarker('date');
  assert.equal((await h.counts()).markers, 1, 'setup: placed');

  await h.deleteMarker();
  const gone = await h.counts();
  assert.equal(gone.overlays, 0, 'the overlay must be gone — otherwise this is the previous test');

  h.dialogs.length = 0;
  await closeDoc();
  assert.equal(h.dialogs.length, 1,
    'an overlay edit history must prompt even with no overlays remaining');
});

// P01.S03's G2, and the row whose instrument already misled once: a grid
// child-count cannot tell "the fix worked" from "the race never happened", so the
// in-flight count is captured AT CLOSE TIME and asserted strictly between.
test('closing mid-thumbnail-build leaves no orphan thumbnail', async () => {
  await h.openDocument(BIG, 80);
  const total = await page.evaluate(() => Number(document.querySelector('.pageCount').textContent.replace('/ ', '')));
  assert.equal(total, 80);

  // Wait for the build to be demonstrably underway but nowhere near done.
  await page.waitForFunction(() => document.querySelector('.thumbgrid:not([hidden])')?.children.length >= 3);
  const atClose = (await h.counts()).thumbs;

  h.dialogs.length = 0;
  await closeDoc();
  await page.waitForTimeout(2500); // well past any in-flight page render

  assert.ok(atClose > 0 && atClose < total,
    `NOT EXERCISED: the build was not in flight at close time (${atClose} of ${total})`);
  assert.equal((await h.counts()).thumbs, 0, 'no thumbnail may survive the close');
});

test('a failed close tears nothing down', async () => {
  await h.openDocument(DOC, 3);
  // The stimulus counter, and without it this test proves nothing: EVERY assertion
  // below is already true before the click. Ask the question — what would this have
  // missed if the close had done nothing at all? — and the answer was "nothing, it
  // would still be green". A close that never fired, a control that never armed, or
  // a settle that returned too early all read as "a failed close tore nothing down".
  let refusals = 0;
  await page.route('**/api/close', (route) => {
    refusals++;
    return route.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"injected"}' });
  });

  h.dialogs.length = 0;
  await closeDoc();

  assert.ok(refusals > 0,
    'NOT EXERCISED: /api/close was never called, so the failure this test injects never happened');

  const kept = await chrome();
  assert.equal(kept.wrap, 'has-doc', 'a failed close must not tear down the client');
  assert.equal(kept.pageCount, '/ 3');
  assert.equal(kept.closeDisabled, false, 'the control must stay usable');
  const status = await page.evaluate(async () => (await fetch('/api/pdf')).status);
  assert.equal(status, 200, 'the server must still be serving the document');

  await page.unroute('**/api/close');
  await closeDoc();
});
