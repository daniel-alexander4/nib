// What the compare tells the user about a document nobody edited — /pending 282.
//
// ── Why this is separate from compare-hash.test.mjs ──────────────────────────
// That file measures the HASH: how far apart two pages are. This one measures what the hash is
// FOR. The user never sees a Hamming distance; they see `alignStat`'s "+added −removed", which is
// the LCS in `alignPages` reading the hash through `DHASH_T`. /pending 277 established that no
// reduction separates the populations by a constant, and that the monotone LCS is what actually
// recovers the alignment — so the threshold's effect is only visible here.
//
// It drives the product's own `alignPages`, exported for this reason. A private LCS would be the
// failure compare-hash.test.mjs records twice: once where a copy kept every assertion green against
// a product that had changed, and once where the copy AGREED with a product defect.
//
// ── What the measurement found, and what it overturned ───────────────────────
// /pending 282 recorded that `DHASH_T = 12` makes the tool report "+3 −3" on an unedited document,
// with per-page distances `13 14 13 11 8 11 11 8`, and asked for the threshold to be raised.
// Re-measured on the corrected fixtures (v1.117.152 replaced `pageField`'s aliasing closed form
// with a per-page PRNG), that does not reproduce: an ordinary rescan sits at 0–10 bits and T=12
// reports +0 −0. The item's own caveat said the threshold result "used `pageField` and stands" —
// `pageField` is exactly what changed.
//
// What IS real is at the other end, and no threshold fixes it. Under a hard rescan the same-page
// population reaches 28 while the nearest different-page pair is 30, and under a brutal one the two
// INVERT — same-page 46, different-page 32. There is no constant that separates them, which is
// /pending 277's finding arriving through the consumer. That is the case for a different hash, not
// a different number, and it is filed as such.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import { launch } from './harness.mjs';
import { makeScanPDF, writeRawFixture } from './fixtures.mjs';

const N = 8;
const pages = Array.from({ length: N }, (_, i) => i);
const A = writeRawFixture('align-a.pdf', makeScanPDF(pages));

// Nothing is edited in any of these. The only difference is how badly the second pass scanned.
const REGIMES = [
  ['gentle', { noise: 6, ramp: 0.04 }],
  ['moderate', { noise: 10, ramp: -0.12, shiftX: 3, shiftY: 2, rot: 1.5 }],
  ['hard', { noise: 14, ramp: -0.22, shiftX: 6, shiftY: 4, rot: 4, contrast: 0.70 }],
];

const h = await launch();
const { page } = h;
after(async () => { await h.browser.close(); });

const bytesA = [...fs.readFileSync(A)];
const regimeBytes = REGIMES.map(([name, deg], i) =>
  [name, [...fs.readFileSync(writeRawFixture(`align-b${i}.pdf`, makeScanPDF(pages, deg)))]]);

const out = await page.evaluate(async ([ba, regimes]) => {
  const app = await import('/app.js');
  const hashes = async (arr) => {
    const doc = await window.pdfjsLib.getDocument({ data: new Uint8Array(arr) }).promise;
    const hs = [];
    for (let i = 1; i <= doc.numPages; i++) {
      const pg = await doc.getPage(i);
      const vp = pg.getViewport({ scale: 1 });
      const c = document.createElement('canvas');
      c.width = Math.ceil(vp.width); c.height = Math.ceil(vp.height);
      await pg.render({ canvasContext: c.getContext('2d'), viewport: vp }).promise;
      hs.push(app.dhashFromGrid(app.gridMeans(c)));
    }
    return hs;
  };
  const ha = await hashes(ba);
  const near = (x, y) => app.hamming(x, y) <= app.DHASH_T;
  const results = [];
  for (const [name, bb] of regimes) {
    const hb = await hashes(bb);
    let nearestDiff = 999;
    for (let i = 0; i < ha.length; i++) {
      for (let j = 0; j < hb.length; j++) {
        if (i !== j) nearestDiff = Math.min(nearestDiff, app.hamming(ha[i], hb[j]));
      }
    }
    const steps = app.alignPages(ha, hb, near);
    results.push({
      name,
      dists: ha.map((x, i) => app.hamming(x, hb[i])),
      nearestDiff,
      aligned: steps.filter((s) => s.a !== null && s.b !== null).length,
      added: steps.filter((s) => s.a === null).length,
      removed: steps.filter((s) => s.b === null).length,
    });
  }
  return { results, shipped: app.DHASH_T, n: ha.length };
}, [bytesA, regimeBytes]);

const at = (name) => out.results.find((r) => r.name === name);

test('the compare reports no edits on a document nobody edited', () => {
  // STIMULUS: the pages really differ from each other, or "aligned 8/8" is what a fixture that
  // paints eight identical pages would produce too. `pageField` aliased twice before v1.117.152 and
  // both times the result was the fixture rather than the hash.
  for (const r of out.results) {
    assert.ok(r.nearestDiff > 0,
      `[${r.name}] two DIFFERENT pages hash identically, so this fixture cannot tell alignment from luck`);
  }
  for (const name of ['gentle', 'moderate']) {
    const r = at(name);
    assert.equal(r.aligned, out.n,
      `[${name}] a rescan of an unedited ${out.n}-page document aligned ${r.aligned}/${out.n} at ` +
      `DHASH_T=${out.shipped} — same-page distances ${r.dists.join(' ')}, nearest different-page ` +
      `pair ${r.nearestDiff}. The user is told pages were added and removed from a document they ` +
      `did not touch.`);
    assert.ok(r.added === 0 && r.removed === 0,
      `[${name}] the compare reports +${r.added} -${r.removed} on an unedited document`);
  }
});

test('the shipped threshold sits inside the separating band, where one exists', () => {
  for (const name of ['gentle', 'moderate']) {
    const r = at(name);
    const worst = Math.max(...r.dists);
    assert.ok(out.shipped >= worst,
      `[${name}] DHASH_T=${out.shipped} is BELOW the worst same-page distance ${worst}, so a page ` +
      `of an unedited document falls outside its own threshold`);
    assert.ok(out.shipped < r.nearestDiff,
      `[${name}] DHASH_T=${out.shipped} is at or above the nearest different-page pair ` +
      `${r.nearestDiff}, so two different pages would align as one`);
  }
});

// **The measured limit, and it is a property of the CONSTANT rather than of any one rescan.**
//
// Each regime has a band of thresholds that work: at or above its worst same-page distance, and
// below its nearest different-page pair. Measured, those bands are roughly [2,18) gentle,
// [10,22) moderate, [28,30) hard — and the first and last do not overlap. **There is no single
// number that aligns a hard rescan without also pairing different pages on an ordinary one.**
//
// That is /pending 277's "no reduction separates the populations by a constant", reached through
// the consumer instead of the hash, and it is why raising `DHASH_T` is not the fix: 12 is correct
// for the ordinary case and nothing is correct for both. The case for a different hash lives here.
//
// **The first version of this assertion was wrong and this file caught it.** It claimed the hard
// regime had NO separating threshold, from a probe that swept T only to 26 — the band is [28,30),
// just outside what was measured. A limit asserted from a range too narrow to see past is the same
// vacuous shape as a threshold tuned on the wrong fixture, which is what this whole item was.
test('no single threshold serves both an ordinary rescan and a hard one', () => {
  const band = (r) => [Math.max(...r.dists), r.nearestDiff]; // [lo, hi): lo works, hi does not
  const g = band(at('gentle')), hard = band(at('hard'));
  assert.ok(hard[0] >= g[1],
    `the hard rescan's workable thresholds [${hard[0]},${hard[1]}) now OVERLAP the gentle ` +
    `regime's [${g[0]},${g[1]}) — a single constant would serve both, so this limit is gone and ` +
    `DHASH_T should be re-derived from the overlap. Good news; re-read /pending 301.`);
  // And the shipped constant is the ordinary case's, which is the deliberate choice: an advisory
  // compare that is right about ordinary rescans and wrong about hard ones beats one that pairs
  // different pages on every ordinary document.
  assert.ok(out.shipped >= g[0] && out.shipped < g[1],
    `DHASH_T=${out.shipped} is outside the ordinary rescan's own band [${g[0]},${g[1]})`);
});
