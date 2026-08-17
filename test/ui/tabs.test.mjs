// P06.S01 — tier 3, and the home of P05's carried acceptance clause.
//
// **Why this file exists at tier 3 and not tier 2.** Both halves of the clause are
// about LAYOUT: a view that loads while hidden reports `clientWidth` 0, so
// `fitWidestWidth` silently no-ops and the view has no scale until activation re-fits
// it; and a device-pixel-ratio change while a view is hidden reaches only the active
// viewer, so the hidden one's canvases stay at the old resolution. jsdom has no layout
// — every `clientWidth` is 0 there, which is the very condition under test rather than
// an environment that can observe it — and no device pixel ratio to change.
//
// **Why it exists now.** The clause was written for P05.S04 and recorded `not
// exercised` through the whole of P05, because the only path that created a second
// view was a co-signature arrival needing a live pinned peer. P06.S01 makes Open add,
// so every Open after the first takes the same hidden-load-then-activate path with no
// peer involved. This is the slice that owes the test, and the plan says so in as many
// words: a clause that carries twice is how `not exercised` becomes permanent.
//
// **The dpr half was pre-flighted before this file was written**, because it is the
// half that might not have been drivable at all: measured
// `{"before":1,"after":2,"changed":true}` via CDP `Emulation.setDeviceMetricsOverride`,
// so the override does reach the page's `devicePixelRatio`. Had it not, this half would
// have recorded `not exercised` WITH that probe as its evidence rather than as a silent
// skip.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const A = writeFixture('tab-a.pdf', { pages: 3, label: 'doc A page' });
const B = writeFixture('tab-b.pdf', { pages: 5, label: 'doc B page' });

const h = await launch();
const page = h.page;

// The browser closes in an `after` hook, not at the end of the last test — the shape
// both sibling tier-3 files already use. Closing inside a test leaks the browser
// whenever an earlier assertion throws, and `node --test` then never exits: the run
// hangs instead of failing, which reads as an infrastructure problem rather than as
// the assertion failure it is. Observed here on the first run of this file.
after(() => h.browser.close());

// switched waits for the visible view to be the document with `pages` pages. The page
// count is the discriminator because the two fixtures differ in length — a switch that
// did nothing leaves the old count, so this is a real condition and not a sleep.
const switched = (pages) => page.waitForFunction(
  (n) => document.querySelector('.pageCount').textContent === `/ ${n}`
    && document.querySelectorAll('.viewerContainer:not([hidden]) .page').length > 0,
  pages,
);

// The visible view's geometry: the one container that is not hidden, and the first page
// inside it. Scoped to `:not([hidden])` for the reason harness.mjs spells out — with
// several containers in the wrap, a bare selector measures whichever one the document
// ordered first, which need not be the one on screen.
const geometry = () => page.evaluate(() => {
  const c = document.querySelector('.viewerContainer:not([hidden])');
  const p = c?.querySelector('.page');
  const canvas = p?.querySelector('canvas');
  return {
    containerWidth: c?.clientWidth ?? 0,
    pageWidth: p?.offsetWidth ?? 0,
    canvasBacking: canvas?.width ?? 0,
    canvasCss: canvas?.clientWidth ?? 0,
    dpr: devicePixelRatio,
    scrollTop: c?.scrollTop ?? 0,
    pageNumber: Number(document.querySelector('.pageNum')?.value ?? 0),
    containers: document.querySelectorAll('.viewerContainer').length,
    visible: document.querySelectorAll('.viewerContainer:not([hidden])').length,
  };
});

// Tabs are addressed by SELECTOR, never by a held handle. syncTabs rebuilds the strip
// wholesale on every change (see its comment for why eight buttons is not worth
// diffing), so a handle captured before a re-render points at a detached node and
// Playwright refuses the click — observed here as "Element is not attached to the DOM".
// Nothing about that harms a user; it is the automation cost of the rebuild, and paying
// it in the test is cheaper than a diffing render in the app.
const tabSel = (n) => `#tabstrip .tab:nth-child(${n})`;
const tabCount = () => page.$$eval('#tabstrip .tab', (els) => els.length);
const activeTabName = () => page.$eval('#tabstrip .tab.active', (el) => el.textContent);

test('a view that loaded hidden is re-fitted when it is activated', async () => {
  await h.openDocument(A, 3);
  const first = await geometry();
  // The stimulus, and the control in one: the FIRST document was loaded into a visible
  // container, so it is fitted by the ordinary path. Its ratio is what "fitted" looks
  // like on this viewport, which is what makes the second document's ratio meaningful
  // rather than a number compared against a guess.
  const fittedRatio = first.pageWidth / first.containerWidth;
  assert.ok(fittedRatio > 0.9,
    `setup: the first document is not fitted to width (page ${first.pageWidth} of container ${first.containerWidth}) — nothing below can be compared against it`);

  // The second document loads into a HIDDEN container — newView() hides it because one
  // already exists — and is then activated. That is the exact path the clause names.
  await h.openDocument(B, 5);
  const second = await geometry();

  assert.equal(second.containers, 2, 'the second Open did not add a view');
  assert.equal(second.visible, 1, 'both containers are visible — the outgoing view was not hidden');
  const ratio = second.pageWidth / second.containerWidth;
  assert.ok(ratio > 0.9,
    `the activated view was never re-fitted: its page is ${second.pageWidth}px in a ${second.containerWidth}px container (ratio ${ratio.toFixed(2)}, want > 0.9 like the first document's ${fittedRatio.toFixed(2)}). A view loaded while hidden reports clientWidth 0, so fitWidestWidth no-ops and the scale stays at pdf.js's default.`);
});

// rendered waits for the visible view to have a canvas with real width. openDocument
// waits for `.page` elements, and pdf.js paints the canvas inside them a beat later —
// so measuring straight after an open reads a page with no canvas yet. Caught by this
// file's own setup assertion on its first run, which is what setup assertions are for.
const rendered = () => page.waitForFunction(() => {
  const c = document.querySelector('.viewerContainer:not([hidden]) canvas');
  return c && c.clientWidth > 0 && c.width > 0;
});

test('a view hidden across a dpr change is re-rendered at the new dpr when activated', async () => {
  // Both documents are already open from the test above; B is active and A is hidden.
  await rendered();
  const before = await geometry();
  assert.equal(before.dpr, 1, `setup: dpr is already ${before.dpr}, so raising it proves nothing`);
  // The stimulus for the assertion at the end: the ACTIVE view is rendered at 1:1 now,
  // so a 2:1 backing store later is a change and not the starting state.
  assert.ok(before.canvasBacking > 0 && before.canvasCss > 0, 'setup: the active view has no rendered canvas to measure');
  const beforeRatio = before.canvasBacking / before.canvasCss;
  assert.ok(beforeRatio > 0.9 && beforeRatio < 1.1,
    `setup: the active view's canvas is already ${beforeRatio.toFixed(2)}:1, not 1:1`);

  // Switch to A, so B is the hidden one when the dpr changes.
  await page.click(tabSel(1));
  await switched(3);

  const cdp = await page.context().newCDPSession(page);
  await cdp.send('Emulation.setDeviceMetricsOverride', {
    width: 1280, height: 900, deviceScaleFactor: 2, mobile: false,
  });
  // The override must actually have taken, or the switch below re-renders nothing and
  // the assertion passes against a dpr that never moved.
  await page.waitForFunction(() => devicePixelRatio === 2);

  // Back to B — hidden throughout the dpr change, so nothing has repainted it.
  await page.click(tabSel(await tabCount()));
  await switched(5);
  await rendered();

  // Not named `after` — that is the node:test hook imported above, and shadowing it
  // inside a test is the kind of thing that reads fine until someone adds a hook here.
  const healed = await geometry();
  const ratio = healed.canvasBacking / healed.canvasCss;
  assert.ok(ratio > 1.5,
    `the re-activated view is still rendered at ${ratio.toFixed(2)}:1 after the device pixel ratio went to ${healed.dpr}. lastDpr is one module global and dprChanged refreshes only the ACTIVE viewer, so a dpr change while this view was hidden is recorded and never delivered — its canvases stay at the old resolution, CSS-stretched, permanently soft.`);

  await cdp.send('Emulation.clearDeviceMetricsOverride');
  await page.waitForFunction(() => devicePixelRatio === 1);
});

test('switching tabs preserves the page you were on', async () => {
  // The phase's headline promise, observable for the first time here: switching is
  // user-reachable as of this slice. `activateView` has restored scroll and page since
  // P05.S04 and no tier could drive it.
  const last = await tabCount();
  await page.click(tabSel(1));
  await switched(3);

  // Move A to its last page, and assert it moved — otherwise "the page was preserved"
  // is satisfied by never having left page 1.
  await page.fill('.pageNum', '3');
  await page.press('.pageNum', 'Enter');
  await page.waitForFunction(() => Number(document.querySelector('.pageNum').value) === 3);
  const leftOn = await activeTabName();

  await page.click(tabSel(last));
  await switched(5);
  assert.notEqual(await activeTabName(), leftOn, 'the switch away never happened, so returning proves nothing');

  await page.click(tabSel(1));
  await switched(3);
  const back = await geometry();
  assert.equal(back.pageNumber, 3,
    `returning to a document put it on page ${back.pageNumber}, not the page 3 it was left on — display:none drops scrollTop, so the restore in activateView is the only thing keeping this true`);
});
