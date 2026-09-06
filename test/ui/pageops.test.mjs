// M7's page-op thumbnail buttons, and the double-stamp warning they made necessary.
//
// Two of the ledger's "eyeball it on a real machine" entries and one review finding land
// in the same file because they are the same surface: operations that rewrite the open
// document and reload it underneath the viewer. The Go side of each is fully tested; what
// was never driven is the button, the reload, and what the user sees afterwards.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const h = await launch();
const { page } = h;
after(() => h.browser.close());

const DOC = writeFixture('pageops.pdf', { pages: 3, label: 'pageop page' });

// The visible view's first page, as geometry. Rotation is observable as an ASPECT swap —
// a portrait page becomes wider than it is tall — which is a fact about the rendered
// document rather than about the button having been clicked.
const firstPage = () => page.evaluate(() => {
  const p = document.querySelector('.viewerContainer:not([hidden]) .page');
  return p ? { w: p.offsetWidth, h: p.offsetHeight } : { w: 0, h: 0 };
});
const pageCount = () => page.$eval('.pageCount', (el) => el.textContent);
// Per-thumbnail controls are addressed by title, not by position: `.thumbacts` holds
// ↺ ↻ × in that order and an index would silently follow a reorder.
const thumbBtn = (n, title) => `.thumbgrid:not([hidden]) .thumbwrap:nth-child(${n}) .thumbacts button[title="${title}"]`;

test('rotating one page from its thumbnail turns that page', async () => {
  await h.openDocument(DOC, 3);
  await h.panel('thumbs'); // the mode lands on its Commands panel since v1.121.0
  await page.waitForFunction(() => document.querySelector('.thumbgrid:not([hidden])')?.children.length === 3);

  const before = await firstPage();
  // The stimulus: the fixture is portrait, so "it became landscape" is a change this
  // test can see. A square page would make the assertion below unfalsifiable.
  assert.ok(before.h > before.w,
    `setup: page 1 is ${before.w}×${before.h}, which is not portrait — a rotation would not change its aspect and nothing below could fail`);

  // Keyboard first, and it is an assertion rather than a convenience. These three
  // buttons were `display: none` until :hover, which cannot be focused — so rotate and
  // delete were pointer-only, and per-page rotate exists nowhere else in the UI. They are
  // hidden by opacity now, and this is what would notice if that regressed.
  await page.focus(thumbBtn(1, 'Rotate right'));
  const kb = await page.$eval(thumbBtn(1, 'Rotate right'), (b) => ({
    // getClientRects, not opacity: a `display: none` element reports opacity 1 quite
    // happily, so reading opacity alone is a green that cannot fail. The first draft of
    // this assertion did exactly that and passed with the defect put back.
    laidOut: b.getClientRects().length > 0,
    focused: document.activeElement === b,
    opacity: getComputedStyle(b.closest('.thumbacts')).opacity,
  }));
  assert.ok(kb.laidOut && kb.focused,
    `the rotate button cannot take keyboard focus (laid out: ${kb.laidOut}, focused: ${kb.focused}). It is display:none until :hover, and a display:none element is not focusable — so rotating or deleting a single page is pointer-only, and per-page rotate exists nowhere else in the UI`);
  assert.equal(kb.opacity, '1',
    'the thumbnail actions are focusable but still invisible, so a keyboard-only user cannot see which control they are on');

  await page.hover(`.thumbgrid:not([hidden]) .thumbwrap:nth-child(1)`);
  await page.click(thumbBtn(1, 'Rotate right'));
  await page.waitForFunction((b) => {
    const p = document.querySelector('.viewerContainer:not([hidden]) .page');
    return p && p.offsetWidth > p.offsetHeight && p.offsetWidth !== b.w;
  }, before);

  const rotated = await firstPage();
  assert.ok(rotated.w > rotated.h,
    `page 1 is ${rotated.w}×${rotated.h} after Rotate right — still portrait, so the operation did not reach the rendered document`);
  assert.equal(await pageCount(), '/ 3', 'rotating changed the page count');
});

test('deleting one page from its thumbnail removes it', async () => {
  await page.hover(`.thumbgrid:not([hidden]) .thumbwrap:nth-child(2)`);
  await page.click(thumbBtn(2, 'Delete page'));
  await page.waitForFunction(() => document.querySelector('.pageCount').textContent === '/ 2');

  assert.equal(await pageCount(), '/ 2');
  // The grid is the other half and it is not the same claim: the document can lose a page
  // while the thumbnail strip still shows three, which is precisely the stale-sidebar
  // shape P05.S05 rebuilt the grids to prevent.
  await page.waitForFunction(() => document.querySelector('.thumbgrid:not([hidden])')?.children.length === 2);
});

test('a document that already carries a stamp says so before you add another', async () => {
  // **The review finding, driven.** StampPageNumbers bakes onto whatever it is given, so
  // running it twice puts a second set of numbers on top of the first. The document
  // itself is what knows — pdfcpu files every watermark it writes into an optional-content
  // group — and /api/stamps is where the dialog asks.
  await h.mode('edit');
  await page.click('#pageNumBtn');
  await page.waitForFunction(() => !document.getElementById('pageNumModal').hidden);

  // The stimulus, and the whole test rests on it: an unstamped document must NOT warn, or
  // "it warns after stamping" is true of a note that is simply always on.
  await page.waitForFunction(() => document.getElementById('pnStamped').hidden === true);

  await page.click('#pnGo');
  await page.waitForFunction(() => document.getElementById('toast')?.textContent === 'Page numbers added');
  await page.waitForFunction(() => document.getElementById('pageNumModal').hidden);

  await page.click('#pageNumBtn');
  await page.waitForFunction(() => !document.getElementById('pageNumModal').hidden);
  await page.waitForFunction(() => document.getElementById('pnStamped').hidden === false, null, { timeout: 10000 })
    .catch(() => {
      assert.fail('the page-number dialog does not warn on a document it has already stamped: /api/stamps reported no stamp layer, or the dialog never asked. Adding numbers again would silently draw a second set on top of the first');
    });
});

test('a render that fails after the metadata changed says so, and can be retried', async () => {
  // **The P05 finding, driven.** setDocumentFromServer assigns `docMeta` BEFORE awaiting
  // the render, and two failure paths return after it. The naive fix — put the old meta
  // back — is wrong: for an operation reload the server HAS applied the operation, so the
  // new meta names what the server holds and the stale thing is the RENDER. Tearing the
  // view down is worse, because it loses the document over a failed re-render. So the view
  // keeps both and says which is which.
  // The test above leaves its dialog open, over the thumbnails this one has to reach.
  await page.click('#pnCancel');
  await page.waitForFunction(() => document.getElementById('pageNumModal').hidden);
  await h.mode('edit');
  await h.panel('thumbs'); // switching mode lands on that mode's Commands panel since v1.121.0
  const before = await pageCount();
  assert.equal(before, '/ 2', `setup: ${before} pages open, want 2 from the delete above`);
  await page.waitForFunction(() => document.getElementById('staleBanner').hidden === true);

  // Break the render, not the operation: /api/pages still succeeds, so the server really
  // does move on and the client really is left holding pixels of the previous version —
  // which is the exact state the finding is about. Garbage rather than a 500, because a
  // failed FETCH is a different path from a failed PARSE.
  await page.route('**/api/pdf*', (route) =>
    route.fulfill({ status: 200, contentType: 'application/pdf', body: 'not a pdf at all' }));

  await page.hover(`.thumbgrid:not([hidden]) .thumbwrap:nth-child(1)`);
  await page.click(thumbBtn(1, 'Rotate right'));
  await page.waitForFunction(() => document.getElementById('staleBanner').hidden === false, null, { timeout: 15000 })
    .catch(() => { assert.fail('the render failed and nothing on screen said so — the view is showing the previous version under the new document\'s identity, silently'); });

  const msg = await page.$eval('#staleMsg', (el) => el.textContent);
  assert.match(msg, /previous version/,
    `the banner reads "${msg}", which does not tell the user what they are looking at`);
  // The document is STILL THERE. This is the half that makes the design right rather than
  // merely honest: a teardown would have been simpler and would have lost the pages.
  assert.equal(await pageCount(), '/ 2', 'the failed re-render tore the document down');

  // Retry, with the render working again — the banner must clear itself.
  await page.unroute('**/api/pdf*');
  await page.click('#staleRetry');
  await page.waitForFunction(() => document.getElementById('staleBanner').hidden === true, null, { timeout: 15000 })
    .catch(() => { assert.fail('the banner survived a successful retry, so it reports staleness that is over — and the next real one will be ignored'); });
});

test('this file leaves the shared server as it found it', async () => {
  // Tier 3 runs every file against ONE nib process; a document left open here is a
  // document the next file counts. Observe, clean up, THEN assert — an `after` hook does
  // not run when an assertion throws, which is exactly when the cleanup matters.
  const openPages = (await h.counts()).pages;
  await h.closeDocument();
  const left = await page.evaluate(() => document.querySelectorAll('.viewerContainer .page').length);

  assert.ok(openPages > 0, 'setup: no document was open, so this cleanup covered nothing');
  assert.equal(left, 0,
    `${left} page divs survive the close — the next file in this tier will count them as its own`);
});
