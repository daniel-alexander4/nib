// What the perceptual page-compare hash can and cannot tell apart, measured — /pending 8.
//
// ── Why this exists, and why it is tier 3 ────────────────────────────────────
// The item was parked for months on Dan supplying two genuinely scanned revisions. That gate is
// false: `pageDHash` renders a page, reduces it to a 9x8 grid of cell means, and emits 64 bits of
// "is this cell brighter than the one to its right". Sensor noise, paper texture and halftoning are
// destroyed before the algorithm reads a byte, so what reaches `hamming` is gross tonal structure
// and the illumination envelope — both exactly reproducible by a generated document.
//
// Tier 2 cannot host it and says so: `test/jsdom/boot.mjs` names "no canvas … the compare pixel
// map" as its ceiling. There is nothing for the reduction to draw into.
//
// ── What this measures, and the one thing it does not ────────────────────────
// It reduces the app's OWN rendered page canvas the way `pageDHash` does and compares the bits.
// It does not call `pageDHash` itself: that would need the function exposed on `window`, and a
// test hook shipping in the product is the `toolbarStyle` shape this repo deleted a feature over.
// So the render is the app's and the reduction is this file's — stated plainly, because it is the
// seam where a future change to `pageDHash` could drift from what is asserted here.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './harness.mjs';
import { makeScanPDF, writeRawFixture } from './fixtures.mjs';

const DHASH_T = 12; // web/app.js — the threshold two pages must be within to count as the same

const A = writeRawFixture('hash-a.pdf', makeScanPDF([0, 1, 2, 3]));
const NEG = writeRawFixture('hash-neg.pdf', makeScanPDF([0, 1, 2, 3], { invert: true }));
const NOISE = writeRawFixture('hash-noise.pdf', makeScanPDF([0, 1, 2, 3], { noise: 12 }));

const h = await launch();
const { page } = h;

after(async () => {
  try {
    for (let i = 0; i < 8 && await page.$eval('#viewerWrap', (el) => el.className) === 'has-doc'; i++) {
      await h.closeDocument();
    }
  } catch { /* the assertion that already failed is the one worth reporting */ }
  await h.browser.close();
});

// grid reduces the visible page the way pageDHash does: render, drawImage into 9x8, read luminance.
// Only ONE page canvas is rendered at a time, so this always measures the first — an earlier draft
// asked for page 2 and silently got page 1 through a `|| all[0]` fallback.
async function grid(file) {
  await h.openDocument(file, 4);
  await h.topOfDocument();
  await page.waitForFunction(() => {
    const c = document.querySelector('.viewerContainer:not([hidden]) .page canvas');
    return c && c.width > 0;
  }, null, { timeout: 20000 });
  const got = await page.evaluate(() => {
    const all = document.querySelectorAll('.viewerContainer:not([hidden]) .page canvas');
    const src = all[0];
    if (!src) return null;
    const g = document.createElement('canvas');
    g.width = 9; g.height = 8;
    const ctx = g.getContext('2d');
    ctx.drawImage(src, 0, 0, 9, 8);
    const px = ctx.getImageData(0, 0, 9, 8).data;
    const cells = [];
    for (let i = 0; i < 72; i++) {
      const o = i * 4;
      cells.push(Math.round(0.299 * px[o] + 0.587 * px[o + 1] + 0.114 * px[o + 2]));
    }
    return cells;
  });
  await h.closeDocument();
  assert.ok(got, 'no page canvas was rendered, so this measured nothing');
  return got;
}

const bits = (cells) => {
  const b = [];
  for (let r = 0; r < 8; r++) for (let c = 0; c < 8; c++) b.push(cells[r * 9 + c] > cells[r * 9 + c + 1] ? 1 : 0);
  return b;
};
const hamming = (x, y) => x.reduce((n, v, i) => n + (v !== y[i] ? 1 : 0), 0);
const ties = (cells) => {
  let t = 0;
  for (let r = 0; r < 8; r++) for (let c = 0; c < 8; c++) if (cells[r * 9 + c] === cells[r * 9 + c + 1]) t++;
  return t;
};

test('the hash separates a page from its own photographic negative', async () => {
  const a = await grid(A), n = await grid(NEG);

  // SETUP: the fixture must actually reach the perceptual path and render something with
  // structure. A blank page hashes to all-zeroes and would make every distance below meaningless.
  assert.ok(new Set(a).size > 3,
    `the rendered page reduced to ${new Set(a).size} distinct cell value(s) — it is effectively blank, and every distance measured from it is meaningless`);

  assert.ok(hamming(bits(a), bits(n)) > DHASH_T,
    `a page and its photographic negative are ${hamming(bits(a), bits(n))} bits apart against a threshold of ${DHASH_T} — the compare would pair them as the same page`);
});

test('the hash survives scanner noise', async () => {
  const a = await grid(A), z = await grid(NOISE);
  const d = hamming(bits(a), bits(z));

  // The stimulus: noise has to have MOVED something, or "it survived" is true of a fixture that
  // was never degraded.
  assert.ok(ties(a) !== ties(z) || d > 0,
    'the noisy fixture reduced to exactly the same grid as the clean one, so this row is not measuring resilience to anything');

  assert.ok(d <= DHASH_T,
    `the same page with sensor-grade noise is ${d} bits from its clean render against a threshold of ${DHASH_T} — two scans of one page would read as different pages`);
});
