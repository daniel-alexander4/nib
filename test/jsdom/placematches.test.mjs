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
  // A short stamp's ink is not on any box: it keeps its reach.
  assert.deepEqual(ys(matchesInMap(page([{ ...t, short: true }]), [re('provided')])[0]), [131, 152]);
});

test('print is boxed by its font\'s own ink where the map carries it, a point clear, and never past the box its reach drew', () => {
  // 10pt type whose reach is 200..212.5 (baseline 210): the font says its glyphs stop at 202.7 and 212.1.
  const t = run(100, 200, 'the secret word', { ink: [202.7 / H, 212.1 / H] });
  t.rect[3] = 212.5 / H;
  const got = matchesInMap(page([t]), [re('secret')]);
  // Up: the ink less a whole point — the line above's tails, which the reach's 199.5 took, are left alone.
  // Down: a point past the ink would be 213.1, past the 213 the reach drew; ink inside the reach never draws a
  // larger box than the reach did.
  assert.deepEqual(ys(got[0]), [201.7, 213]);
  assert.deepEqual(pts(got[0]).filter((_, i) => i % 2 === 0), [123.25, 160.75], 'across, nothing changes');
  // The estimate is still matched against the reach: one drawn high, its centre above the ink, is replaced.
  assert.deepEqual(ys(got[0].hold), [199.5, 213]);
  const est = [120 / W, 196 / H, 164 / W, 205 / H]; // centre 200.5: inside the reach, above the ink's box
  assert.deepEqual(placeMatches([est], got), { boxes: got, exact: 1, kept: 0 });
  // Without ink, the reach — as every print box was drawn before.
  const { ink, ...bare } = t;
  assert.deepEqual(ys(matchesInMap(page([bare]), [re('secret')])[0]), [199.5, 213]);
  // A tail deeper than the reach (an Arabic letter's, 0.45 of a size): the box grows to it, and a point past.
  const deep = { ...t, ink: [202.7 / H, 214.5 / H] };
  assert.deepEqual(ys(matchesInMap(page([deep]), [re('secret')])[0]), [201.7, 215.5]);
  // Ink a hair inside the reach's top: a point clear of it would be past the reach's box, so the reach's stands.
  assert.deepEqual(ys(matchesInMap(page([{ ...t, ink: [200.2 / H, 212.1 / H] }]), [re('secret')])[0]), [199.5, 213]);
  // And ink above the reach likewise.
  const tall = { ...t, ink: [198 / H, 212.1 / H] };
  assert.deepEqual(ys(matchesInMap(page([tall]), [re('secret')])[0]), [197, 213]);
  // A match over two runs, one with ink and one without, is as tall as the taller needs.
  const a = run(100, 200, 'sec', { ink: [202.7 / H, 212.1 / H] }), b = run(118, 200, 'ret');
  a.rect[3] = b.rect[3] = 212.5 / H;
  assert.deepEqual(ys(matchesInMap(page([a, b]), [re('secret')])[0]), [199.5, 213]);
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

// A right-to-left word as Nib stamps it since ADR-097: set last letter first and marked, so the map hands it over in
// reading order with its cuts running from the RIGHT — letter i is 6pt wide leftward from x + 6·(n − i).
const reversed = (x, y, str, extra = {}) => {
  const r = run(x, y, str, { hidden: true, reversed: true, ...extra });
  r.cuts.reverse();
  return r;
};

test('part of a Hebrew OCR word set in reverse is boxed where its letters are, which is from the right', () => {
  const word = 'שלום'; // at 100..124: ש at 118..124, ם at 100..106
  const map = page([reversed(100, 200, word), reversed(160, 200, 'עולם')]);
  assert.deepEqual(xs(matchesInMap(map, [re(word.slice(0, 2))])[0]), [111.25, 124.75], 'the first two letters are the right half');
  assert.deepEqual(xs(matchesInMap(map, [re(word.slice(2))])[0]), [99.25, 112.75], 'the last two are the left half');
  assert.deepEqual(xs(matchesInMap(map, [re(word.slice(1, 3))])[0]), [105.25, 118.75], 'and the middle two the middle');
  assert.deepEqual(xs(matchesInMap(map, [re(word)])[0]), [99.25, 124.75]);
});

test('…and only that: an older layer, a short stamp, another tool\'s layer and an Arabic word are still taken whole', () => {
  const heb = 'שלום', ara = 'محمد';
  const whole = (t, part, why) => {
    const [x0, x1] = xs(matchesInMap(page([t]), [re(part)])[0]);
    assert.ok(x0 <= 100 && x1 >= 124, `${why}: the box runs ${x0}..${x1} and the word is at 100..124 — a letter asked for may be showing`);
  };
  for (const part of [heb.slice(0, 2), heb.slice(2)]) {
    whole(fitted(100, 200, heb), part, 'a word stamped in reading order (before ADR-097, or by another tool)');
    whole(reversed(100, 200, heb, { short: true }), part, 'a reversed word that could not be fitted');
  }
  // Arabic letters are stamped apart and printed joined: the boundaries between them are not the page's.
  for (const part of [ara.slice(0, 2), ara.slice(2)]) whole(reversed(100, 200, ara), part, 'an Arabic word, reversed and fitted');
  // One Arabic-script letter in a run is enough.
  whole(reversed(100, 200, 'שלוم'), 'של', 'a run with one Arabic letter in it');
});

test('a match only the map found is added, and with no map reading the estimates stand', () => {
  assert.equal(placeMatches([], [[0.2, 0.25, 0.26, 0.27]]).boxes.length, 1);
  const est = [[0.17, 0.245, 0.29, 0.275]];
  assert.deepEqual(placeMatches(est, []), { boxes: est, exact: 0, kept: 1 });
});

// ── the accuracy harness's `scan` column (test/accuracy/scanscore.mjs) ───────
// Not product code: the instrument that says whether a search-redaction over an OCR'd scan leaves ink showing. Held
// here because its figures are quoted as facts, and a scorer nobody has made fail can only be believed.
const scan = await import(new URL('../../test/accuracy/scanscore.mjs', import.meta.url));
// The engine's word as the harness holds it: a box in points from the top-left.
const word = (text, x0, y0, x1, y1) => ({ text, box: [x0, y0, x1, y1] });
const frac = (x0, y0, x1, y1) => [x0 / W, y0 / H, x1 / W, y1 / H];

test('scan column: the engine\'s boxes are turned top-left, and a page whose words do not line up with its layer is not scored', () => {
  const truth = [{ page: 1, text: 'Smith', rect: [100, 560, 130, 570] }, { page: 2, text: 'other', rect: [100, 560, 130, 570] },
    { page: 1, text: 'empty', rect: [50, 50, 50, 60] }, { page: 1, text: 'norect' }];
  const m = page([run(100, 222, 'Smith', { hidden: true })]);
  const words = scan.scanWords(truth, 1, m);
  assert.deepEqual(words, [{ text: 'Smith', box: [100, 222, 130, 232] }]);
  // The run is 30 wide over ink 30 wide; a short stamp of 21.6 reads 0.72.
  assert.deepEqual(scan.scanLayer(words, m), { engineWords: 1, lineUp: 1, widthOverInk: [1, 1, 1], ok: true });
  const short = page([{ ...run(100, 222, 'Smith', { hidden: true }), rect: [100 / W, 222 / H, 121.6 / W, 234 / H] }]);
  assert.deepEqual(scan.scanLayer(words, short).widthOverInk, [0.72, 0.72, 0.72]);
  // The same words over a layer that is somewhere else, over print, and over nothing: not lined up.
  assert.equal(scan.scanLayer(words, page([run(300, 222, 'Smith', { hidden: true })])).ok, false);
  assert.equal(scan.scanLayer(words, page([run(100, 400, 'Smith', { hidden: true })])).ok, false);
  assert.equal(scan.scanLayer(words, page([run(100, 222, 'Smyth', { hidden: true })])).ok, false);
  assert.equal(scan.scanLayer(words, page([run(100, 222, 'Smith')])).ok, false, 'print was taken for the OCR layer');
  assert.equal(scan.scanLayer([], m).ok, false);
  // Half is enough; less is not.
  const four = ['aaaa', 'bbbb', 'cccc', 'dddd'].map((t, i) => word(t, 100, 222 + 20 * i, 124, 232 + 20 * i));
  assert.equal(scan.scanLayer(four, page([run(100, 222, 'aaaa', { hidden: true }), run(100, 242, 'bbbb', { hidden: true })])).ok, true);
  assert.equal(scan.scanLayer(four, page([run(100, 222, 'aaaa', { hidden: true })])).ok, false);
});

test('scan column: the words searched are whole, four letters or more, and on the page once as the search counts', () => {
  const texts = ['Smith', 'smithy', 'Jones', 'tax', 'A-1234', 'Wages', 'wages', 'Total', 'Form1040'];
  const words = texts.map((t, i) => word(t, 100, 100 + 20 * i, 160, 110 + 20 * i));
  const m = page(texts.map((t, i) => run(100, 100 + 20 * i, t, { hidden: true })));
  // 'Smith' is inside 'smithy'; 'Wages' is there twice; 'tax' is short; 'A-1234' is not one word.
  assert.deepEqual(scan.scanPicks(words, m, 10).map((w) => w.text), ['smithy', 'Jones', 'Total', 'Form1040']);
  // Once among the engine's words but twice in the layer (another tool's words under it): not searched.
  const twice = page([...m.text, run(300, 100, 'jones again', { hidden: true })]);
  assert.deepEqual(scan.scanPicks(words, twice, 10).map((w) => w.text), ['smithy', 'Total', 'Form1040']);
  // Twice among the engine's words and once in the layer (a word the stamp dropped): which box is its is not known.
  assert.deepEqual(scan.scanPicks([...words, word('Total', 300, 100, 330, 110)], m, 10).map((w) => w.text), ['smithy', 'Jones', 'Form1040']);
  // PRINT that repeats the word is not the layer's, and does not take it out.
  assert.equal(scan.scanPicks(words, page([...m.text, run(300, 100, 'Jones')]), 10).length, 4);
  // A limit takes them spread down the page, not the first few.
  assert.deepEqual(scan.scanPicks(words, m, 2).map((w) => w.text), ['smithy', 'Total']);
});

test('scan column: ink outside the box is measured on each side, and a word with no box over it is missed', () => {
  const w = word('Smith', 100, 222, 130, 232), m = page([]);
  const got = (...boxes) => scan.scoreScanWord(w, boxes.map((b) => frac(...b)), m, [w]);
  assert.deepEqual(got([99, 221, 131, 233]), { word: 'Smith', boxes: 1, inkOutside: 0, left: -1, right: -1, above: -1, below: -1, pastRight: 1, tall: 1.2, reaches: 0, specks: 0 });
  assert.equal(got([99, 221, 126, 233]).inkOutside, 4, 'the end of the word shows');
  assert.equal(got([99, 221, 126, 233]).right, 4);
  assert.equal(got([103, 221, 131, 233]).left, 3);
  assert.equal(got([103, 221, 131, 233]).inkOutside, 3, 'the start of the word shows');
  assert.equal(got([99, 224, 131, 233]).above, 2);
  assert.equal(got([99, 224, 131, 233]).inkOutside, 2, 'the top of the word shows');
  assert.equal(got([99, 221, 131, 230.5]).below, 1.5);
  assert.equal(got([99, 221, 131, 230.5]).inkOutside, 1.5);
  // Two boxes that cover it between them: nothing shows, and the count says there were two.
  const two = got([99, 221, 115, 233], [114, 221, 131, 233]);
  assert.deepEqual([two.boxes, two.inkOutside], [2, 0]);
  // No box, and a box that is somewhere else (beside it, and on the next line).
  assert.deepEqual(got(), { word: 'Smith', missed: true, boxes: 0 });
  assert.deepEqual(got([140, 221, 170, 233]), { word: 'Smith', missed: true, boxes: 1 });
  assert.deepEqual(got([99, 240, 131, 252]), { word: 'Smith', missed: true, boxes: 1 });
  // A second box elsewhere does not widen the one over the word.
  assert.equal(got([99, 221, 131, 233], [300, 221, 330, 233]).pastRight, 1);
});

test('scan column: a box reaches another line only where a real word of another line is under it', () => {
  const w = word('Smith', 100, 222, 130, 232), m = page([]);
  const above = word('Above', 100, 208, 130, 218), beside = word('next', 133, 222, 160, 232), far = word('Far', 300, 208, 330, 218);
  const got = (box, others) => scan.scoreScanWord(w, [frac(...box)], m, [w, ...others]);
  // The line above ends at 218: a box from 221 clears it, one from 215 is 3pt into it.
  assert.equal(got([99, 221, 131, 233], [above, beside, far]).reaches, 0);
  assert.equal(got([99, 215, 131, 233], [above, beside, far]).reaches, 1);
  // 0.3pt into it is the same edge; a little more is not.
  assert.equal(got([99, 217.7, 131, 233], [above]).reaches, 0);
  assert.equal(got([99, 217.6, 131, 233], [above]).reaches, 1);
  // The neighbour on the word's own line is not another line, however far the box runs into it.
  assert.equal(got([99, 221, 150, 233], [beside]).reaches, 0);
  // The three the probe counted, each a speck of OCR noise in or on the word's own box: none is another line.
  const dot = word('.', 110, 226, 111, 226.3), slash = word('/', 120, 229, 121.1, 229.3), za = word('za', 128.5, 221.5, 131, 231.5);
  assert.deepEqual([got([99, 221, 131, 233], [dot, slash, za]).reaches, got([99, 221, 131, 233], [dot, slash, za]).specks], [0, 0]);
  // A word whose middle is inside this one's height is on its line even when this one's middle is not inside its:
  // the probe asked one way only.
  assert.equal(got([99, 221, 131, 233], [word('x', 105, 229.5, 112, 231.5)]).reaches, 0);
  // A speck on the line above that the box does reach is counted, apart from words.
  const speck = word('\'', 110, 216, 111, 216.8), thin = word('|', 110, 208, 111, 218);
  assert.deepEqual([got([99, 215, 131, 233], [speck]).reaches, got([99, 215, 131, 233], [speck]).specks], [0, 1]);
  assert.deepEqual([got([99, 215, 131, 233], [thin]).reaches, got([99, 215, 131, 233], [thin]).specks], [0, 1]);
  const flat = word('_', 105, 216, 115, 216.8); // wide enough, and too flat to be a word
  assert.deepEqual([got([99, 215, 131, 233], [flat]).reaches, got([99, 215, 131, 233], [flat]).specks], [0, 1]);
  // …and one way round the other: a tall mark this word's middle is inside, whose own middle is below the word.
  assert.equal(got([99, 221, 131, 233], [word('[', 105, 226, 112, 260)]).reaches, 0);
  assert.deepEqual([got([99, 215, 131, 233], [above, speck]).reaches, got([99, 215, 131, 233], [above, speck]).specks], [1, 1]);
});

test('scan column: the summary counts what must be zero, and a corpus with no scan has no column', () => {
  assert.equal(scan.scanSummary([{ doc: 'form.pdf', page: 1 }], 0), null);
  assert.deepEqual(scan.scanReport([{ doc: 'form.pdf', page: 1 }]), []);
  const layer = { engineWords: 4, lineUp: 4, widthOverInk: [0.9, 1, 1], ok: true };
  const rows = [
    { doc: 'scan.pdf', page: 1, scan: { layer, words: [
      { word: 'a', boxes: 1, inkOutside: 0, pastRight: 0.7, tall: 1.1, reaches: 0, specks: 0 },
      { word: 'b', boxes: 2, inkOutside: 0.3, pastRight: 0.8, tall: 1.2, reaches: 1, specks: 2 },
      { word: 'c', boxes: 1, inkOutside: 2.5, pastRight: -2.5, tall: 1.3, reaches: 0, specks: 1 },
      { word: 'd', missed: true, boxes: 0 }] } },
    { doc: 'scan.pdf', page: 2, scan: { layer: { ...layer, widthOverInk: [null, null, null], ok: false }, error: 'does not line up' } },
    { doc: 'form.pdf', page: 1 },
  ];
  assert.deepEqual(scan.scanSummary(rows, 12.4), { pages: 1, words: 4, missed: 1, notOneBox: 1, inkOutside: 1, maxInkOutsidePt: 2.5, reachesAnotherLine: 1,
    reachesOnlySpecks: 1, boxPastInkRightMedPt: 0.7, boxOverInkHeightMed: 1.2, layerWidthOverInkMed: 1, notScored: ['scan.pdf p2: does not line up'], seconds: 12 });
  const lines = scan.scanReport(rows);
  assert.equal(lines.length, 4);
  assert.match(lines[2], /scan\.pdf\s+p1\s+\| 4\/1\/1\/1\/1\/1 \| 0\.7 \| 1$/);
  assert.match(lines[3], /p2\s+\| not scored: does not line up$/);
});
