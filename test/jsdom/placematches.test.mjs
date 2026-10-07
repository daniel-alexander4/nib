// matchesInMap and placeMatches (ADR-090): a search-redaction box placed from the page's own glyph boundaries, and
// the rule for when it may replace the estimated one.
//
// ── What this tier can reach ─────────────────────────────────────────────────
// The whole rule: both are pure functions over a map and rectangles. In particular the one property redaction cannot
// get wrong — a match the map did not place keeps its estimated box, whatever its neighbours did.
//
// ── What it cannot ───────────────────────────────────────────────────────────
// Whether the map's glyph boundaries are where the ink is. That is pagemap_test.go for the arithmetic and
// build/accuracy.sh for real documents.
import test from 'node:test';
import assert from 'node:assert/strict';
import { matchesInMap, placeMatches } from '../../web/detect.js';

const W = 612, H = 792;
// A run set in a 6pt-wide fixed pitch, 12pt tall, starting at x: every boundary is known exactly.
const run = (x, y, str, extra = {}) => {
  const chars = [...str];
  return { rect: [x / W, y / H, (x + 6 * chars.length) / W, (y + 12) / H], text: str, size: 10 / H,
    chars, cuts: chars.map((_, i) => (x + 6 * i) / W).concat((x + 6 * chars.length) / W), ...extra };
};
const page = (text) => ({ page: 1, width: W, height: H, text, shapes: [], widgets: [], noText: false });
const re = (s, flags = 'gi') => new RegExp(s, flags);
const pts = (r) => [r[0] * W, r[1] * H, r[2] * W, r[3] * H].map((n) => Math.round(n * 100) / 100);

test('a match is boxed from its first glyph\'s left edge to its last glyph\'s right, and takes no neighbour', () => {
  //            x: 100   ...   "secret" starts at glyph 4 → 124, ends at 160
  const got = matchesInMap(page([run(100, 200, 'the secret word')]), [re('secret')]);
  assert.equal(got.length, 1);
  assert.deepEqual(pts(got[0]), [123.25, 199.5, 160.75, 212.5]);
  // The glyphs either side — the "e" of "the" (centre 115) and the "w" of "word" (centre 169) — are outside it.
  assert.ok(got[0][0] * W > 115 && got[0][2] * W < 169);
});

test('every occurrence is found, case folded by the pattern, and a pattern is reusable', () => {
  const p = re('ab');
  const map = page([run(100, 200, 'ab AB xab')]);
  assert.equal(matchesInMap(map, [p]).length, 3);
  // A global RegExp remembers where it stopped. One left part-way — by the estimate's own pass over another row —
  // must still be run from the start of this one.
  p.lastIndex = 5;
  assert.equal(matchesInMap(map, [p]).length, 3, 'a RegExp left part-way through was not run from the start');
});

test('a match across two runs of one line is one box; a real gap between runs is a space', () => {
  // "123-45-" and "6789" abut: one number.
  const joined = matchesInMap(page([run(100, 200, '123-45-'), run(142, 200, '6789')]), [re('\\b\\d{3}-\\d{2}-\\d{4}\\b', 'g')]);
  assert.equal(joined.length, 1);
  assert.deepEqual(pts(joined[0]).filter((_, i) => i % 2 === 0), [99.25, 166.75]);
  // "SSN" then a word gap then the number: the gap is a space, so \b holds and "SSN123" does not appear.
  const map = page([run(100, 200, 'SSN'), run(124, 200, '123-45-6789')]);
  assert.equal(matchesInMap(map, [re('\\b\\d{3}-\\d{2}-\\d{4}\\b', 'g')]).length, 1);
  assert.equal(matchesInMap(map, [re('SSN123')]).length, 0);
  assert.equal(matchesInMap(map, [re('SSN 123')]).length, 1);
});

test('runs on different lines are not joined, and a phrase\'s inner space takes no ink but stays inside the box', () => {
  assert.equal(matchesInMap(page([run(100, 200, 'sec'), run(118, 230, 'ret')]), [re('secret')]).length, 0);
  const got = matchesInMap(page([run(100, 200, 'one two')]), [re('one two')]);
  assert.deepEqual(pts(got[0]).filter((_, i) => i % 2 === 0), [99.25, 142.75]);
});

test('a ligature is one glyph: a match inside it takes the whole glyph', () => {
  const t = run(100, 200, 'office');
  t.chars = ['o', 'ffi', 'c', 'e']; t.cuts = [100, 106, 118, 124, 130].map((x) => x / W);
  const got = matchesInMap(page([t]), [re('fic')]);
  assert.deepEqual(pts(got[0]).filter((_, i) => i % 2 === 0), [105.25, 124.75]);
});

test('a run without glyph boundaries is not placed from the map', () => {
  const bare = run(100, 200, 'secret'); delete bare.cuts;
  assert.deepEqual(matchesInMap(page([bare]), [re('secret')]), []);
  assert.deepEqual(matchesInMap(null, [re('secret')]), []);
});

// A SHORT stamp — an OCR word as Nib wrote it before ADR-092: its own hidden run, starting where the scanned word
// starts and narrower than it. `fitted` is a hidden run that already spans its word (ADR-092, or another tool's layer).
const ocr = (x, y, str) => run(x, y, str, { hidden: true, short: true });
const fitted = (x, y, str) => run(x, y, str, { hidden: true });
const xs = (r) => [r[0] * W, r[2] * W].map((n) => Math.round(n * 100) / 100);
const ys = (r) => [r[1] * H, r[3] * H].map((n) => Math.round(n * 100) / 100);

test('a hidden word is boxed out to the next hidden word on its line, because the scanned word is wider than its stamp', () => {
  // "secret" is stamped 36pt wide at 100; the next word starts at 160. The scanned word ends somewhere before that.
  const got = matchesInMap(page([ocr(100, 200, 'secret'), ocr(160, 200, 'word')]), [re('secret')]);
  assert.equal(got.length, 1);
  assert.deepEqual(xs(got[0]), [99.25, 160.25], 'the box stops half a point short of the next word, plus its own pad');
  // And it is taller than the stamp: the stamp sat above the bottom of the ink. A quarter of the 10pt size each way.
  assert.deepEqual(ys(got[0]), [197, 215]);
});

test('the stretch is never past the next word, never more than 2.5 of the stamp, and never a shrink', () => {
  // The next word is a column away: 2.5 x 36 = 90, not the 300 to the next run.
  assert.deepEqual(xs(matchesInMap(page([ocr(100, 200, 'secret'), ocr(400, 200, 'word')]), [re('secret')])[0]), [99.25, 190.75]);
  // The last word on its line has no next word to stop at: the same ceiling.
  assert.deepEqual(xs(matchesInMap(page([ocr(100, 200, 'secret')]), [re('secret')])[0]), [99.25, 190.75]);
  // A layer whose glyphs already reach the next word — another tool's, stretched when it was written — has no room,
  // and is read exactly as it is.
  assert.deepEqual(xs(matchesInMap(page([ocr(100, 200, 'secret'), ocr(136.5, 200, 'word')]), [re('secret')])[0]), [99.25, 136.75]);
  // ...and one whose next word starts INSIDE it is not shrunk to fit.
  assert.deepEqual(xs(matchesInMap(page([ocr(100, 200, 'secret'), ocr(130, 200, 'word')]), [re('secret')])[0]), [99.25, 136.75]);
});

test('part of a hidden word is stretched in proportion, and a word on another line is not "the next word"', () => {
  // Stretch 60/36: "cre" is glyphs 2..4, 112..130 unstretched → 100 + 12 x 59.5/36 .. 100 + 30 x 59.5/36.
  const got = matchesInMap(page([ocr(100, 200, 'secret'), ocr(160, 200, 'word')]), [re('cre')]);
  assert.deepEqual(xs(got[0]), [119.08, 150.33]);
  // The run at 160 is a line lower: it bounds nothing here.
  assert.deepEqual(xs(matchesInMap(page([ocr(100, 200, 'secret'), ocr(160, 230, 'word')]), [re('secret')])[0]), [99.25, 190.75]);
});

test('print and an OCR layer are two layers: neither joins the other\'s line, and print is never stretched', () => {
  // Visible "sec" directly followed by hidden "ret" on one line is not the word "secret".
  assert.equal(matchesInMap(page([run(100, 200, 'sec'), ocr(118, 200, 'ret')]), [re('secret')]).length, 0);
  // A hidden run to the right does not stretch print, and print does not bound a hidden run.
  assert.deepEqual(xs(matchesInMap(page([run(100, 200, 'secret'), ocr(160, 200, 'word')]), [re('secret')])[0]), [99.25, 136.75]);
  assert.deepEqual(xs(matchesInMap(page([ocr(100, 200, 'secret'), run(160, 200, 'word')]), [re('secret')])[0]), [99.25, 190.75]);
  assert.deepEqual(ys(matchesInMap(page([run(100, 200, 'secret')]), [re('secret')])[0]), [199.5, 212.5], 'print took the hidden layer\'s extra height');
});

test('a hidden word that is not a short stamp is read as it is written: not stretched, not made taller', () => {
  // ADR-092: a fitted word already spans its scanned word, and its size is the line's. The next word is 24pt away
  // and a column is further still: neither is reached for, and the box is print's height.
  const got = matchesInMap(page([fitted(100, 200, 'secret'), fitted(160, 200, 'word')]), [re('secret')]);
  assert.deepEqual(xs(got[0]), [99.25, 136.75], 'a fitted word was stretched toward the next one');
  assert.deepEqual(ys(got[0]), [199.5, 212.5], 'a fitted word was made taller: on a real layer that reaches the lines either side');
  assert.deepEqual(xs(matchesInMap(page([fitted(100, 200, 'secret')]), [re('secret')])[0]), [99.25, 136.75], 'the last word on a line took the ceiling');
  // It is still the OCR layer, not print: it does not join print's line.
  assert.equal(matchesInMap(page([run(100, 200, 'sec'), fitted(118, 200, 'ret')]), [re('secret')]).length, 0);
  // One line may hold both kinds — a word that could not be fitted beside ones that were. Each is read as what it is,
  // and a fitted word still bounds the short stamp before it.
  const mixed = page([ocr(100, 200, 'secret'), fitted(160, 200, 'word')]);
  assert.deepEqual(xs(matchesInMap(mixed, [re('secret')])[0]), [99.25, 160.25]);
  assert.deepEqual(ys(matchesInMap(mixed, [re('secret')])[0]), [197, 215]);
  assert.deepEqual(xs(matchesInMap(mixed, [re('word')])[0]), [159.25, 184.75]);
  assert.deepEqual(ys(matchesInMap(mixed, [re('word')])[0]), [199.5, 212.5]);
});

test('an estimated box is replaced only by an exact box that holds its centre', () => {
  const exact = [[0.20, 0.25, 0.26, 0.27]];
  const over = [0.17, 0.245, 0.29, 0.275];        // the estimate for the same match: wider, same centre
  const r = placeMatches([over], exact);
  assert.deepEqual(r.boxes, exact);
  assert.deepEqual([r.exact, r.kept], [1, 0]);
});

test('a match the map did not place keeps its estimate, even when its wide box overlaps a placed neighbour', () => {
  // Two matches side by side. The map placed the left one only. The right one's estimate over-reaches into the left
  // one's box — and must still be kept: dropping it would leave the right match unredacted.
  const exact = [[0.20, 0.25, 0.26, 0.27]];
  const left = [0.17, 0.245, 0.29, 0.275], right = [0.25, 0.245, 0.37, 0.275];
  const r = placeMatches([left, right], exact);
  assert.deepEqual(r.boxes, [exact[0], right]);
  assert.deepEqual([r.exact, r.kept], [1, 1]);
  // And on another line entirely.
  assert.equal(placeMatches([[0.17, 0.445, 0.29, 0.475]], exact).kept, 1);
});

test('a fitted OCR word is boxed by its own ink, and still replaces the estimate pdf.js drew above it', () => {
  // "provided" on a real scan (points, top-left): its reach 134.0..149.0, its ink 141.4..148.1, and the estimate —
  // from pdf.js's span, which sits high under the fit's matrix — 133.1..144.4, whose centre is ABOVE the ink.
  const t = run(235.4, 134, 'provided', { hidden: true, ink: [141.4 / H, 148.1 / H] });
  t.rect[3] = 149 / H;
  const got = matchesInMap(page([t]), [re('provided')]);
  assert.deepEqual(ys(got[0]), [140.9, 148.6], 'the ink, plus the half-point pad');
  const est = [232.0 / W, 133.1 / H, 267.3 / W, 144.4 / H];
  assert.deepEqual(placeMatches([est], got), { boxes: got, exact: 1, kept: 0 }, 'the estimate is replaced, not kept beside it');
  // The line above's estimate is not this word's: its centre is outside the reach too.
  const above = [232.0 / W, 121.6 / H, 267.3 / W, 132.9 / H];
  assert.equal(placeMatches([above], got).kept, 1);
  // A short stamp's ink is not on any box, and print has none: both keep their reach.
  assert.deepEqual(ys(matchesInMap(page([{ ...t, short: true }]), [re('provided')])[0]), [131, 152]);
  assert.deepEqual(ys(matchesInMap(page([{ ...t, hidden: false }]), [re('provided')])[0]), [133.5, 149.5]);
});

test('part of a right-to-left OCR word takes the whole word: its letters are stamped from the left and scanned from the right', () => {
  // "שלום" at 100..124. Its first two letters are stamped at 100..112 — and are on the paper at 112..124.
  const word = 'שלום';
  // Both ends: the first letters are stamped at the left, the last at the right, and neither is where it is scanned.
  for (const [make, part] of [fitted, ocr].flatMap((m) => [[m, word.slice(0, 2)], [m, word.slice(2)]])) {
    const got = matchesInMap(page([make(100, 200, word), make(160, 200, 'word')]), [re(part)]);
    assert.equal(got.length, 1);
    const [x0, x1] = xs(got[0]);
    assert.ok(x0 <= 100 && x1 >= 124, `the box runs ${x0}..${x1}, and the word is at 100..124`);
    assert.ok(x1 <= 160.25, `and it stops at the next word, which starts at 160: ${x1}`); // a short stamp's own reach, plus the pad
  }
  // The same letters in PRINT are where the page set them, and a left-to-right OCR word is where its glyphs are.
  assert.deepEqual(xs(matchesInMap(page([run(100, 200, word)]), [re(word.slice(0, 2))])[0]), [99.25, 112.75]);
  assert.deepEqual(xs(matchesInMap(page([fitted(100, 200, 'secret')]), [re('se')])[0]), [99.25, 112.75]);
});

test('a match only the map found is added, and with no map reading the estimates stand', () => {
  assert.equal(placeMatches([], [[0.2, 0.25, 0.26, 0.27]]).boxes.length, 1);
  const est = [[0.17, 0.245, 0.29, 0.275]];
  assert.deepEqual(placeMatches(est, []), { boxes: est, exact: 0, kept: 1 });
});
