// Crop, driven through the real button, the real drag and the real reload.
//
// `PLAN-ua-coverage.md` P02.S05 changed what a crop IS: it used to rebuild each target page
// through pdfcpu's CutPage into display orientation at the origin, and now it moves the page's
// boxes in the page's own coordinates and touches nothing else (ADR-047). The Go side proves
// the geometry against poppler at every rotation; what nothing drove was the client's fractions
// arriving at the new server shape and pdf.js drawing the result the user asked for.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch, shutdown } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const DOC = writeFixture('crop.pdf', { pages: 3, label: 'crop page' });

const h = await launch();
const page = h.page;

after(async () => {
  try {
    for (let i = 0; i < 8 && await page.$eval('#viewerWrap', (el) => el.className) === 'has-doc'; i++) {
      await h.closeDocument();
    }
  } catch { /* the assertion that already failed is the one worth reporting */ }
  await shutdown(h);
});

const pageBoxes = () => page.$$eval('.viewerContainer:not([hidden]) .page', (els) =>
  els.map((el) => { const r = el.getBoundingClientRect(); return { left: r.x, top: r.y, w: r.width, h: r.height }; }));

test('cropping one page to its left half makes that page half as wide and leaves the others alone', async () => {
  await h.openDocument(DOC, 3);
  await h.topOfDocument();
  await h.mode('edit');
  await h.group('Size & Number Pages');
  await page.click('#cropBtn');

  const before = await pageBoxes();
  assert.equal(before.length, 3, 'setup: the fixture did not lay out three pages');
  const p = before[0];
  assert.ok(p.h > 100 && p.top < 600,
    `setup: page 1 is not on screen to draw on (top ${p.top}, height ${p.h})`);

  // The LEFT half, from a little inside the top edge to a little inside the visible bottom.
  const y0 = p.top + 10, y1 = Math.min(p.top + p.h - 10, 700);
  await page.mouse.move(p.left + 2, y0);
  await page.mouse.down();
  await page.mouse.move(p.left + p.w / 2, y1, { steps: 8 });
  await page.mouse.up();
  await page.waitForSelector('#cropModal:not([hidden])');
  await page.uncheck('#cropAllPages');

  // Mark the page elements the crop will replace: the reload builds new ones, so "no marked page
  // is left" is the transition, and it does not depend on the RESULT — a wait on "page 1 got
  // narrower" would time out on exactly the defect this test exists to catch.
  await page.$$eval('.viewerContainer:not([hidden]) .page', (els) => els.forEach((el) => { el.dataset.precrop = '1'; }));
  await page.click('#cropGo');
  await page.waitForFunction(() => /cropped to the box/.test(document.getElementById('toast')?.textContent ?? ''),
    null, { timeout: 15000 });
  await page.waitForFunction(() => {
    const els = [...document.querySelectorAll('.viewerContainer:not([hidden]) .page')];
    return els.length === 3 && els.every((el) => !el.dataset.precrop && el.getBoundingClientRect().width > 0);
  }, null, { timeout: 15000 });

  // pdf.js sizes a page it has not fetched yet from page 1's viewport, so visit every page before
  // measuring: otherwise page 3 reads as the size of the cropped page 1 and not its own.
  for (const n of [3, 2, 1]) await h.gotoPage(n);
  await page.waitForFunction(() => [...document.querySelectorAll('.viewerContainer:not([hidden]) .page')]
    .every((el) => el.querySelector('canvas, .canvasWrapper')), null, { timeout: 15000 });

  const after = await pageBoxes();
  assert.equal(after.length, 3, `the crop changed the page count to ${after.length}`);
  // pdf.js lays pages out at one scale, so width ratios are page-size ratios.
  const ratio = after[0].w / after[1].w;
  assert.ok(Math.abs(ratio - 0.5) < 0.03,
    `page 1 is ${after[0].w.toFixed(0)}px wide against page 2's ${after[1].w.toFixed(0)}px (ratio ${ratio.toFixed(3)}); a left-half crop is 0.5`);
  assert.ok(Math.abs(after[1].w - after[2].w) < 1 && Math.abs(after[1].h - after[2].h) < 1,
    'pages 2 and 3 differ in size after cropping page 1 alone — the crop reached pages outside the selection');
});
