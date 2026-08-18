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
// 2000 pages, the LAST of them twice as wide as the rest. Both numbers are load-bearing
// and the test below says why: the page count is what makes the load slow enough to act
// inside, and the odd page out is what makes both edges of the load window visible in
// the DOM.
const MIXED = writeFixture('mixed.pdf', { pages: 2000, label: 'mixed page', widePage: 2000 });

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
  // counts().pages carries a careful argument about being scoped to the VISIBLE
  // container (with two views open, a bare selector sums 3 + 5 = 8 and reads as a page
  // count) — and until the P05 graduation pass it had no reader at all: a correctness
  // argument protecting a value nobody consulted. Read here, where a document of known
  // length is already open, so the argument is defended by an assertion rather than by
  // a comment.
  assert.equal((await h.counts()).pages, 3, 'the visible viewer must hold one .page per page of the document');
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
  // "unsaved" since v1.108.7. It read "since the last save" while the app could only
  // answer "since it was opened" — a save cleared none of the four signals — so the
  // hedge was the honest wording then and is the inaccurate one now.
  assert.match(h.dialogs[0], /unsaved/);

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

// The visible view's widths. Scoped to `:not([hidden])` for the reason harness.mjs
// spells out: with several containers in the wrap a bare selector measures whichever
// one the document ordered first, which need not be the one on screen.
const widths = () => page.evaluate(() => {
  const c = document.querySelector('.viewerContainer:not([hidden])');
  const pages = c ? c.querySelectorAll('.page') : [];
  return {
    container: c?.clientWidth ?? 0,
    first: pages[0]?.offsetWidth ?? 0,
    last: pages[pages.length - 1]?.offsetWidth ?? 0,
    count: pages.length,
  };
});

test('a zoom set while the document is still loading is not thrown away', async () => {
  // **The load door onto the defect P06's exit criterion names at the switch door.**
  // `pagesinit` fits page one immediately so there is no 100%-then-fit flash, and
  // `pagesloaded` refines that to the WIDEST page once every page view is populated.
  // The refine was unconditional — so a zoom made in between, on a document long enough
  // that "in between" is a real interval, was overwritten the moment the load finished.
  // Silently, and with the user's hand still on the button.
  //
  // This is also the mechanism behind tabs.test.mjs's intermittently-flaky zoom test:
  // there the refine landed after the zoom and before the switch, and the switch then
  // had nothing to preserve.
  await h.openDocument(MIXED, 2000);

  // ── the test is only meaningful inside the load window, so it reads whether it is ──
  // openDocument returns at `pagesinit`, where pdf.js has sized every page div from
  // PAGE ONE's viewport — so page 1 fills the container and the wide last page is still
  // reported portrait. Both are checked: either one failing means the document finished
  // loading before the zoom, and nothing below could have failed.
  const opening = await widths();
  assert.equal(opening.count, 2000, `setup: ${opening.count} page divs, want 2000`);
  assert.ok(opening.first / opening.container > 0.9,
    `setup: page 1 is ${opening.first}px in a ${opening.container}px container — the widest-page refine has ALREADY run, so the load window closed before this test acted and the assertion at the end cannot fail`);
  assert.ok(opening.last < opening.first * 1.5,
    `setup: the last page is already ${opening.last}px against page 1's ${opening.first}px, so it has resolved its own size and the document is loaded — same reason, the window has closed`);

  // The user zooms, inside the window.
  await page.click('#zoomInBtn');
  await page.click('#zoomInBtn');
  const zoomed = (await widths()).first;
  assert.ok(zoomed > opening.first * 1.1,
    `setup: zooming moved page 1 from ${opening.first}px to ${zoomed}px, which is not a change this test can see`);

  // ── then the load finishes ────────────────────────────────────────────────────────
  // The wide page is the LAST one, so its div reaching its true width means every page
  // resolved — which is the condition `pagesloaded` fires on. The settle after it is a
  // bound, not a tuning: the handler runs synchronously off that same promise chain, so
  // if the zoom is going to be overwritten it has been overwritten already.
  await page.waitForFunction(() => {
    const pages = document.querySelectorAll('.viewerContainer:not([hidden]) .page');
    const last = pages[pages.length - 1];
    return last && last.offsetWidth > pages[0].offsetWidth * 1.5;
  });
  await page.waitForTimeout(300);

  const after = (await widths()).first;
  assert.ok(Math.abs(after - zoomed) < 2,
    `page 1 is ${after}px, not the ${zoomed}px the user zoomed it to: the fit that lands when the document finishes loading overwrote a scale the user had already chosen`);

  await closeDoc();
});
