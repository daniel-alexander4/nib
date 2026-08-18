// A redaction box drawn across a page boundary stays on the page it belongs to.
//
// **This is the worst failure shape this app has, and it looked fine.** Every box tool
// measures against the page the drag STARTED on, and nothing bounded the other end. Page
// divs are `overflow: visible` (pdf_viewer.css) and in continuous scroll the next page sits
// directly below — so a drag from near the bottom of page N carried on into page N+1, and
// the preview, absolutely positioned inside page N's div, painted a black rectangle right
// over page N+1's content.
//
// The user's evidence at Apply time is that rectangle. applyRedact groups marks BY PAGE and
// flattens only the pages named, so page N+1 was never rasterised — the text the user
// watched go black stayed in the output, selectable. Shown coverage that was never going to
// happen, on the one path where being wrong is unrecoverable.
//
// ── Why tier 3 and nowhere else ─────────────────────────────────────────────
// It is entirely a question of geometry: where a page ends, where the pointer went, and
// where the mark was drawn. jsdom has no layout — every getBoundingClientRect() is 0 there
// — so a cross-page drag cannot even be expressed, let alone measured.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const DOC = writeFixture('redact-bounds.pdf', { pages: 3, label: 'redaction page' });

const h = await launch();
const page = h.page;

// Close the document before the browser, so the shared server is handed on empty — the
// tier-3 files run serially against one nib and lifecycle.test.mjs asserts the launch state.
after(async () => {
  try {
    for (let i = 0; i < 8 && await page.$eval('#viewerWrap', (el) => el.className) === 'has-doc'; i++) {
      await h.closeDocument();
    }
  } catch { /* the assertion that already failed is the one worth reporting */ }
  await h.browser.close();
});

test('a redaction drag that crosses onto the next page is bounded by the page it started on', async () => {
  await h.openDocument(DOC, 3);
  await h.topOfDocument();

  await h.mode('secure');
  await page.click('#redactBtn');

  // **Scroll the BOUNDARY into view first.** At fit-width a Letter page is taller than the
  // viewport, so page 1's bottom edge — the thing this test drags across — starts below the
  // fold, and a mouse move to a coordinate off-screen lands nowhere. The first run failed
  // exactly there, and said so, because the assertion below it asks whether the drag
  // registered at all rather than assuming it did.
  await page.evaluate(() => {
    const c = document.querySelector('.viewerContainer:not([hidden])');
    const pages = c.querySelectorAll('.page');
    // Put page 1's bottom edge near the middle of the container.
    c.scrollTop = pages[0].offsetTop + pages[0].offsetHeight - c.clientHeight / 2;
  });
  await page.waitForFunction(() => {
    const c = document.querySelector('.viewerContainer:not([hidden])');
    const p1 = c.querySelectorAll('.page')[0];
    const r = p1.getBoundingClientRect();
    return r.bottom > 0 && r.bottom < window.innerHeight - 40;
  });

  // The geometry this test is about: page 1's box, and page 2 directly beneath it.
  const pages = await page.$$eval('.viewerContainer:not([hidden]) .page', (els) =>
    els.slice(0, 2).map((el) => { const r = el.getBoundingClientRect(); return { top: r.y, bottom: r.y + r.height, left: r.x, width: r.width, height: r.height }; }));
  assert.equal(pages.length, 2, 'setup: fewer than two pages are laid out, so no boundary can be crossed');
  assert.ok(pages[1].top > pages[0].top,
    'setup: the second page is not below the first, so this viewer is not in the continuous scroll this test is about');

  // Drag from inside page 1, near its bottom, DOWN past the boundary into page 2. The
  // stimulus assertion is that the release really is on the other page — without it, a
  // drag that happened to stop short would pass this test having proved nothing.
  const x = pages[0].left + pages[0].width / 2;
  const startY = pages[0].bottom - pages[0].height * 0.08;
  const endY = pages[1].top + pages[1].height * 0.15;
  assert.ok(endY > pages[0].bottom,
    `setup: the drag ends at ${endY}, which is still above page 1's bottom edge (${pages[0].bottom}) — it never crosses the boundary`);

  await page.mouse.move(x, startY);
  await page.mouse.down();
  await page.mouse.move(x + 60, endY, { steps: 8 });
  await page.mouse.up();

  const mark = await page.$eval('.redactmark', (el) => {
    const r = el.getBoundingClientRect();
    return { top: r.y, bottom: r.y + r.height, height: r.height };
  }).catch(() => null);
  assert.ok(mark, 'no redaction mark was recorded at all — the drag did not register, so nothing below is measuring the clamp');
  assert.ok(mark.height > 0, 'the redaction mark has no height');

  // The property. One pixel of slack for sub-pixel layout rounding; the defect this
  // catches overshoots by a whole page fraction, not by a rounding error.
  assert.ok(mark.bottom <= pages[0].bottom + 1,
    `the redaction mark ends at ${mark.bottom.toFixed(1)}px, past page 1's bottom edge at ${pages[0].bottom.toFixed(1)}px — it is painted over page 2. applyRedact flattens only the pages its marks name, so page 2 is never rasterised and every character under that black rectangle survives in the output, selectable. The user's only evidence at Apply time is showing them coverage they will not get.`);
});
