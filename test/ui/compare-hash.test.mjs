// What the perceptual page-compare hash can and cannot tell apart, measured — /pending 8.
//
// ── Why this exists, and why it is tier 3 ────────────────────────────────────
// The item was parked for months on Dan supplying two genuinely scanned revisions. That gate is
// false: `pageDHash` renders a page, reduces it to a 9x8 grid of cell means, and emits a symbol per
// adjacent pair. Sensor noise, paper texture and halftoning are destroyed before the algorithm
// reads a byte, so what reaches `hamming` is gross tonal structure and the illumination envelope —
// both exactly reproducible by a generated document.
//
// ── What this file's first run found, and what it now guards ─────────────────
// It was written against a plain dHash (one strict `>` per pair, 64 bits) and measured three
// defects in it (/pending 276): an illumination gradient's SIGN decided the whole result, because
// a strict comparison files all 22 tied pairs under "darker"; the margin was INVERTED, a different
// page landing 10 bits away and the same page under a reverse gradient 21; and a BLANK sheet
// paired with every content page, so a dropped or double-fed page aligned silently. The product
// now compares log-ratios and emits a trit against an adaptive band. Every assertion below is a
// row of that measurement, kept so the next change to `pageDHash` has to face them.
//
// Tier 2 cannot host it and says so: `test/jsdom/boot.mjs` names "no canvas … the compare pixel
// map" as its ceiling. There is nothing for the reduction to draw into.
//
// ── What this measures, and the seam that used to be here ────────────────────
// The render is the app's and so is everything after it: this file supplies a fixture and a page
// canvas, and `gridMeans`, `dhashFromGrid` and `hamming` all come out of `web/app.js` in page
// context. That was not the original shape. This file first reimplemented the reduction and the
// encoding, declaring the seam — "where a future change to `pageDHash` could drift from what is
// asserted here" — and the seam was load-bearing twice over. Once because a private copy of the
// encoding would keep every assertion green against a product that no longer encoded that way. And
// once, worse, because the copy AGREED with a product defect: both sides called
// `drawImage(src, 0, 0, 9, 8)` and both point-sampled, so the test confirmed the mistake back to
// the product and only a probe printing the raw grid found it. What is left unasserted is the
// rasterise call itself, which needs a pdf.js document handle.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './harness.mjs';
import { makeScanPDF, writeRawFixture, pagesDiffer, sparseField } from './fixtures.mjs';

const A = writeRawFixture('hash-a.pdf', makeScanPDF([0, 1, 2, 3]));
const NEG = writeRawFixture('hash-neg.pdf', makeScanPDF([0, 1, 2, 3], { invert: true }));
const NOISE = writeRawFixture('hash-noise.pdf', makeScanPDF([0, 1, 2, 3], { noise: 12 }));
const RAMP_UP = writeRawFixture('hash-rampup.pdf', makeScanPDF([0, 1, 2, 3], { ramp: 0.10 }));
const RAMP_DN = writeRawFixture('hash-rampdn.pdf', makeScanPDF([0, 1, 2, 3], { ramp: -0.10 }));
const OTHER = writeRawFixture('hash-other.pdf', makeScanPDF([5, 6, 7, 8]));
const PALE = writeRawFixture('hash-pale.pdf', makeScanPDF([0, 1, 2, 3], { contrast: 0.35 }));
const SKEW = writeRawFixture('hash-skew.pdf', makeScanPDF([0, 1, 2, 3], { rot: 2 }));
// A blank sheet: a dropped page, a double feed, or a separator.
const BLANK = writeRawFixture('hash-blank.pdf', makeScanPDF([0, 1, 2, 3], { contrast: 0 }));
// Two sparse text pages — paper with a heading and eight lines, differing only in line lengths.
// This is the hard case, and the one real documents actually present.
const SP0 = writeRawFixture('hash-sparse0.pdf', makeScanPDF([0], {}, { field: sparseField }));
const SP1 = writeRawFixture('hash-sparse1.pdf', makeScanPDF([1], {}, { field: sparseField }));

const h = await launch();
const { page } = h;

// The threshold comes from the product too. A hand-kept copy is a rule with two doors (ADR-009)
// and the drift would be invisible: `DHASH_T` could move in `web/app.js` and every assertion here
// would keep passing against the old value.
const DHASH_T = await page.evaluate(async () => (await import('/app.js')).DHASH_T);

after(async () => {
  try {
    for (let i = 0; i < 8 && await page.$eval('#viewerWrap', (el) => el.className) === 'has-doc'; i++) {
      await h.closeDocument();
    }
  } catch { /* the assertion that already failed is the one worth reporting */ }
  await h.browser.close();
});

// grid renders a fixture's first page in the app and hands the canvas to the product's own
// `gridMeans`. Only ONE page canvas is rendered at a time, so this always measures the first — an
// earlier draft asked for page 2 and silently got page 1 through a `|| all[0]` fallback.
async function grid(file, pages = 4) {
  await h.openDocument(file, pages);
  await h.topOfDocument();
  await page.waitForFunction(() => {
    const c = document.querySelector('.viewerContainer:not([hidden]) .page canvas');
    return c && c.width > 0;
  }, null, { timeout: 20000 });
  const got = await page.evaluate(async () => {
    const all = document.querySelectorAll('.viewerContainer:not([hidden]) .page canvas');
    const src = all[0];
    if (!src) return null;
    // The PRODUCT's reduction, not a copy. It used to be a copy — `drawImage(src, 0, 0, 9, 8)`,
    // the same line `pageDHash` had — and that agreement is exactly what hid the defect: both
    // sides point-sampled, so a page of text reduced to 63 untouched paper cells and 9 untouched
    // ink cells and the test happily confirmed the product's own mistake back to it.
    return (await import('/app.js')).gridMeans(src);
  });
  await h.closeDocument();
  assert.ok(got, 'no page canvas was rendered, so this measured nothing');
  return got;
}

// dist hands two 9x8 grids to the page and asks `web/app.js` for the distance between them,
// using ITS `dhashFromGrid` and ITS `hamming`. Nothing about the encoding is reimplemented here.
async function dist(a, b) {
  const d = await page.evaluate(async ([x, y]) => {
    const app = await import('/app.js');
    if (typeof app.dhashFromGrid !== 'function' || typeof app.hamming !== 'function') return null;
    return app.hamming(app.dhashFromGrid(x), app.dhashFromGrid(y));
  }, [a, b]);
  assert.ok(d !== null,
    'web/app.js did not export dhashFromGrid and hamming — this file measured nothing about the product');
  return d;
}
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

  const d = await dist(a, n);
  assert.ok(d > DHASH_T,
    `a page and its photographic negative are ${d} bits apart against a threshold of ${DHASH_T} — the compare would pair them as the same page`);
});

test('the hash survives scanner noise', async () => {
  const a = await grid(A), z = await grid(NOISE);
  const d = await dist(a, z);

  // The stimulus: noise has to have MOVED something, or "it survived" is true of a fixture that
  // was never degraded.
  assert.ok(ties(a) !== ties(z) || d > 0,
    'the noisy fixture reduced to exactly the same grid as the clean one, so this row is not measuring resilience to anything');

  assert.ok(d <= DHASH_T,
    `the same page with sensor-grade noise is ${d} bits from its clean render against a threshold of ${DHASH_T} — two scans of one page would read as different pages`);
});

test('an illumination gradient does not decide the result, and its sign does not decide it either', async () => {
  const a = await grid(A), up = await grid(RAMP_UP), dn = await grid(RAMP_DN);

  // SETUP: the ramp has to have MOVED the render, or "the gradient is tolerated" is a claim about
  // a fixture that was never degraded. It moves cell VALUES even where it moves no bit.
  assert.notDeepEqual(a, up, 'the +10% ramp produced an identical grid to the clean render, so nothing below is measuring tolerance to a gradient');
  assert.notDeepEqual(a, dn, 'the -10% ramp produced an identical grid to the clean render, so nothing below is measuring tolerance to a gradient');

  const du = await dist(a, up), dd = await dist(a, dn);

  // The defect this replaced: brightening left-to-right moved 0 bits and darkening moved 21, out
  // of a threshold of 12. Both are the same page under the same lamp, tilted the other way.
  assert.ok(du <= DHASH_T && dd <= DHASH_T,
    `the same page under a +10% illumination gradient is ${du} bits away and under a -10% gradient ${dd}, against a threshold of ${DHASH_T} — a scan of one page under an uneven lamp does not match itself`);

  // And the two must not be wildly asymmetric: a scheme that tolerates one sign and not the other
  // passes the line above by luck of which fixture was named first.
  assert.ok(Math.abs(du - dd) <= DHASH_T,
    `the two gradient signs are ${du} and ${dd} bits from the clean render — the SIGN of the illumination is deciding the result, which is the defect this file was written to catch`);
});

test('the margin is not inverted — a different page is further away than the same page degraded', async () => {
  const a = await grid(A), other = await grid(OTHER), dn = await grid(RAMP_DN);

  // SETUP: assert the FIXTURE distinguishes these pages before claiming the hash does. Two earlier
  // drafts of `pageField` aliased — `idx * 7 % 7` for every page, then a period of 7 in `idx` —
  // and each produced a confident "different pages align" that was the fixture, not the hash.
  // The sparse family gets the same check, and it never had one. `pagesDiffer` was written
  // for exactly this class of fixture aliasing and then hard-wired to `pageField`, so the
  // fixture that most needs it — the sparse one, which by design carries the least into a
  // 9x8 reduction — was never covered. Measured before this: `sparseField` page 7 was
  // byte-identical to page 0 and page 8 to page 1, and every page was a cyclic rotation of
  // one multiset of line lengths. Asserted over EVERY pair, because an alias between any
  // two is what makes a "different page" number below mean nothing.
  for (let a = 0; a < 9; a++) {
    for (let b = a + 1; b < 9; b++) {
      const d = pagesDiffer(a, b, { field: sparseField });
      assert.ok(d > 4,
        `sparse pages ${a} and ${b} differ in only ${d} of 72 reduced cells — these two fixture pages are effectively the same page, so any distance measured between them says nothing about the hash`);
    }
  }

  assert.ok(pagesDiffer(0, 5) > 20,
    `page 0 and page 5 differ in only ${pagesDiffer(0, 5)} of 72 reduced cells — the fixture is not drawing two distinguishable pages, so a small distance below would say nothing about the hash`);

  const diff = await dist(a, other);
  const same = await dist(a, dn);

  assert.ok(diff > DHASH_T,
    `a genuinely different page is ${diff} bits away against a threshold of ${DHASH_T} — the compare would pair two different pages`);

  // The inversion itself: no threshold can exist while a different page is nearer than the same
  // page degraded. This is the assertion that made retuning unavailable in the old encoding.
  assert.ok(diff > same,
    `a different page is ${diff} bits away and the SAME page under a reverse gradient is ${same} — the margin is inverted, and no threshold separates them`);
});

test('a blank sheet does not pair with a page that has content on it', async () => {
  const a = await grid(A), blank = await grid(BLANK);

  // SETUP: the blank fixture must actually be blank, and the content one must not be. Assert both,
  // because either failing makes the distance below true for the wrong reason.
  assert.equal(new Set(blank).size, 1,
    `the "blank" fixture reduced to ${new Set(blank).size} distinct cell values — it is not blank, so this measures nothing about blank pages`);
  assert.ok(new Set(a).size > 3,
    'the content fixture reduced to fewer than 4 distinct cell values — it is effectively blank too');

  const d = await dist(a, blank);

  // The defect: a blank hashes to all-zeroes under a strict `>`, and so does every tied pair on a
  // real page, so every content page in the fixture set sat 6-11 bits from blank — inside the
  // threshold. A dropped page or a double feed aligned against real content and said nothing.
  assert.ok(d > DHASH_T,
    `a blank sheet is ${d} bits from a page with content against a threshold of ${DHASH_T} — a dropped or double-fed page would align silently against real content`);
});

test('the hash survives a skewed feed', async () => {
  const a = await grid(A), sk = await grid(SKEW);

  assert.notDeepEqual(a, sk,
    'the rotated fixture reduced to the same grid as the clean one, so this row is not measuring tolerance to skew');

  const d = await dist(a, sk);
  assert.ok(d <= DHASH_T,
    `the same page fed 2° askew is ${d} bits from its straight render against a threshold of ${DHASH_T} — a sheet that went through the feeder crooked would not match itself`);
});

test('a pale scan still matches itself — the comparison band is not a constant', async () => {
  const a = await grid(A), pale = await grid(PALE);

  // SETUP: the pale fixture must genuinely have less contrast, or the band never gets tested.
  const spread = (g) => Math.max(...g) - Math.min(...g);
  assert.ok(spread(pale) < spread(a) * 0.6,
    `the "pale" fixture spans ${spread(pale)} grey levels against the clean render's ${spread(a)} — it is not low-contrast, so this says nothing about a fixed band`);

  const d = await dist(a, pale);

  // A FIXED band dies here: at 35% contrast every real difference falls inside it, every
  // comparison is called a tie, and the page stops matching itself. The band is a quantile of the
  // page's own distribution for exactly this reason.
  assert.ok(d <= DHASH_T,
    `a pale scan is ${d} bits from a normal-contrast render of the same page against a threshold of ${DHASH_T} — the comparison band is not adapting to the page's contrast`);
});

test('the threshold this file asserts against is the one the product uses', async () => {
  // ADR-009: a rule gets one door. Every number in this file — the threshold, the encoding, the
  // bit count — is read out of `web/app.js` at run time rather than copied. What that leaves
  // exposed is the READ: a rename or a dropped `export` makes `DHASH_T` undefined, and `undefined`
  // is never greater than anything, so every threshold assertion above would pass silently.
  assert.ok(Number.isFinite(DHASH_T),
    'web/app.js did not export DHASH_T — the constant was renamed or its export dropped, and every threshold assertion in this file has been comparing against undefined');
  assert.ok(DHASH_T > 0 && DHASH_T < 128,
    `DHASH_T is ${DHASH_T}, outside the 0..128 range a 128-bit hash can produce`);

  // And the hash really is 128 bits: a shorter one would make every distance smaller and the
  // threshold quietly generous, without a single assertion above changing.
  const len = await page.evaluate(async () => {
    const app = await import('/app.js');
    return app.dhashFromGrid(new Array(72).fill(0).map((_, i) => (i * 37) % 256)).length;
  });
  assert.equal(len, 16,
    `dhashFromGrid returned ${len} bytes, not 16 — the hash is not ${len * 8}-bit wide and DHASH_T of ${DHASH_T} means something different against it`);
});

// ── The limit, asserted rather than described ───────────────────────────────
// The grill behind /pending 276 predicted a third defect — "every content page pairs with a BLANK
// page" — and measurement in this browser only half agreed. Against `pageField`'s nine strong
// columns a blank sheet was never close. Against `sparseField`, which is what a scanned letter
// actually looks like, the old encoding put a full page of text exactly 12 bits from blank paper
// against a threshold of 12: it did not pair, by one bit, and any slightly emptier page did.
//
// What the same measurement also showed is a limit the fix does NOT remove: four pairs of sparse
// pages, differing only in where their lines end, measure 9, 10, 11 and 12 bits apart — astride
// the threshold of 12, and overlapping the same page at 4° of skew (12) and a 6 px shift (10). No
// constant separates those two populations. A finer grid makes it worse rather than better: at
// 17x16 a 6 px translation of the SAME page moves 199 of 512 bits while the nearest different page
// moves 59, so the margin inverts — a fine grid is more sensitive to registration than to content.
//
// So this pair is asserted as a LIMIT. It is not a bug being tolerated quietly: `alignPages` runs
// an LCS over the hashes, so a spurious near-match only wins if it fits the monotone chain, and
// the fallback exists at all only when neither side has text to fingerprint. When someone improves
// the hash, this assertion fails and makes them come here and rewrite this note.
test('two sparse text pages are closer than the threshold — a measured limit, not a passing grade', async () => {
  const p0 = await grid(SP0, 1), p1 = await grid(SP1, 1);

  // SETUP: the two fixtures must genuinely be different pages, or "they are close" is trivially
  // true and says nothing. Assert it on the pixels, upstream of anything the hash does.
  assert.notDeepEqual(p0, p1,
    'the two sparse fixtures reduced to the same grid, so the distance below is not measuring the hash at all');

  const d = await dist(p0, p1);
  assert.ok(d <= DHASH_T,
    `two sparse text pages are ${d} bits apart against a threshold of ${DHASH_T} — the hash now separates them, which is BETTER than when this was written. Raise the claim: re-measure, tighten this assertion or delete it, and update the note above.`);

  // The half that IS a guarantee: however close two text pages get, neither may reach blank paper.
  const blank = await grid(BLANK);
  const d0 = await dist(p0, blank);
  assert.ok(d0 > DHASH_T,
    `a full page of text is ${d0} bits from blank paper against a threshold of ${DHASH_T} — a dropped or blank-fed sheet would align against a page of content. This is the case the nine-column fixture cannot see, and the one real documents present.`);
});
