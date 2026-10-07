// A search-redaction match that wraps a line (ADR-095): found by both readings of a page, one box on each line.
//
// ── What this tier can reach ─────────────────────────────────────────────────
// wrappedMatches and rowPieces are pure. scanTextMatches is driven END TO END — the function's own text taken from
// app.js, over a pdf.js page and a page map this file makes — because the defect was in neither reading alone: each
// searched a row at a time, so the two together found nothing, and nothing said so. Before the change the first test
// below read `[]` for a name in plain sight, with a map and without one.
//
// ── What it cannot ───────────────────────────────────────────────────────────
// What pdf.js makes of a real page's lines (one item a line, or several), and whether a real two-column page's gutter
// is wider than WRAP_GAP. That is tier 3 and build/accuracy.sh; neither has a wrapped-match case yet.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

// buildTextRows measures characters on a canvas; here every character is 8 wide, so an item's own width decides.
globalThis.document ||= { createElement: () => ({ getContext: () => ({ measureText: () => ({ width: 8 }) }) }) };
const detect = await import(new URL('../../web/detect.js', import.meta.url));
const { matchesInMap, placeMatches, buildTextRows, wrappedMatches, rowPieces } = detect;
const APP = fs.readFileSync(new URL('../../web/app.js', import.meta.url), 'utf8');

const W = 612, H = 792;
const re = (s, flags = 'gi') => new RegExp(s, flags);

// ── a page, as pdf.js and as the map have it ─────────────────────────────────
// A line of 10pt text in a 6pt fixed pitch whose top is `y` points down the page, starting at `x`.
const item = (x, y, str) => ({ str, transform: [10, 0, 0, 10, x, H - (y + 10)], width: 6 * str.length, height: 10 });
const run = (x, y, str, extra = {}) => {
  const chars = [...str];
  return { rect: [x / W, y / H, (x + 6 * chars.length) / W, (y + 12) / H], text: str, size: 10 / H,
    chars, cuts: chars.map((_, i) => (x + 6 * i) / W).concat((x + 6 * chars.length) / W), ...extra };
};
const mapOf = (text) => ({ page: 1, width: W, height: H, text, shapes: [], widgets: [], noText: false });

// scanTextMatches itself, with everything it reaches for handed in.
const at = APP.indexOf('async function scanTextMatches(');
const scanText = APP.slice(at, APP.indexOf('\n}\n', at) + 2);
const pdfjsLib = { Util: { transform: (a, b) => [a[0] * b[0] + a[2] * b[1], a[1] * b[0] + a[3] * b[1], a[0] * b[2] + a[2] * b[3],
  a[1] * b[2] + a[3] * b[3], a[0] * b[4] + a[2] * b[5] + a[4], a[1] * b[4] + a[3] * b[5] + a[5]] } };
const viewport = { transform: [1, 0, 0, -1, 0, H], width: W, height: H,
  convertToPdfPoint: (x, y) => [x, H - y], convertToViewportPoint: (x, y) => [x, H - y] };
const LINE = { item }; // which kind of item a line is made into (see scan8)
async function scan(lines, patterns, map) {
  const owner = { pdfDocument: { numPages: 1, getPage: async () => ({ getViewport: () => viewport, getTextContent: async () => ({ items: lines.map((l) => LINE.item(...l)) }) }) } };
  const fn = new Function('pdfjsLib', 'buildTextRows', 'pageMap', 'placeMatches', 'matchesInMap', 'wrappedMatches', 'rowPieces', 'rowBaselines', 'view',
    scanText + '\nreturn scanTextMatches;');
  const marks = await fn(pdfjsLib, buildTextRows, async () => map, placeMatches, matchesInMap, wrappedMatches, rowPieces, detect.rowBaselines, owner)(patterns, owner);
  // In points, top-left, to a tenth; down the page then across.
  const boxes = marks.map((m) => [m.fx * W, m.fy * H, (m.fx + m.fw) * W, (m.fy + m.fh) * H].map((v) => Math.round(v * 10) / 10));
  return { boxes: boxes.sort((a, b) => a[1] - b[1] || a[0] - b[0]), exact: marks.exact || 0 };
}
const inside = (box, x, y) => x >= box[0] && x <= box[2] && y >= box[1] && y <= box[3];

// "…pay John" ends a line at x 226; "Smith the sum" begins the next at 100.
const LINES = [[100, 200, 'We agree to pay John'], [100, 214, 'Smith the sum due.']];
const JOHN = [208, 206], SMITH = [115, 220], PAY = [193, 206], THE = [145, 220]; // a point inside each word

test('a phrase that wraps a line is found end to end: one box on each line, with the page map and without it', async () => {
  for (const [name, map] of [['no map: the estimate alone', null], ['the map', mapOf(LINES.map((l) => run(...l)))]]) {
    const got = await scan(LINES, [re('john smith')], map);
    assert.equal(got.boxes.length, 2, `${name}: a name split over a line end was marked with ${got.boxes.length} boxes — it would not be redacted`);
    assert.ok(inside(got.boxes[0], ...JOHN) && inside(got.boxes[1], ...SMITH), `${name}: the boxes are not over the two words: ${JSON.stringify(got.boxes)}`);
  }
});

test('with the map, a wrapped match is boxed by its glyphs on each line and takes no neighbour', async () => {
  const got = await scan(LINES, [re('john smith')], mapOf(LINES.map((l) => run(...l))));
  // "John" is glyphs 16..19 of line one (196..220), "Smith" glyphs 0..4 of line two (100..130).
  assert.deepEqual(got.boxes, [[195.3, 199.5, 220.8, 212.5], [99.3, 213.5, 130.8, 226.5]]);
  assert.equal(got.exact, 2);
  assert.ok(!got.boxes.some((b) => inside(b, ...PAY) || inside(b, ...THE)));
});

test('a phrase on one line is still one box, and a word on each of two lines is not a phrase', async () => {
  const map = mapOf(LINES.map((l) => run(...l)));
  assert.equal((await scan(LINES, [re('pay john')], map)).boxes.length, 1);
  const est = (await scan(LINES, [re('pay john')], null)).boxes;
  assert.equal(est.length, 1);
  assert.ok(inside(est[0], ...PAY) && inside(est[0], ...JOHN), `the estimate is not over both words: ${JSON.stringify(est)}`);
  // "agree" is mid-line and "Smith" starts the next: not adjacent in reading order.
  assert.equal((await scan(LINES, [re('agree smith')], map)).boxes.length, 0);
  // "John" ends line one, "the" is mid-line two.
  assert.equal((await scan(LINES, [re('john the')], null)).boxes.length, 0);
});

// ── a row is not a line ──────────────────────────────────────────────────────
// 8pt type in two columns six points out of step: buildTextRows makes ONE row of the two lines, on the first's baseline.
const item8 = (x, base, str) => ({ str, transform: [8, 0, 0, 8, x, H - base], width: 4.8 * str.length, height: 8 });
const run8 = (x, base, str) => {
  const chars = [...str];
  return { rect: [x / W, (base - 8) / H, (x + 4.8 * chars.length) / W, (base + 2) / H], text: str, size: 8 / H,
    chars, cuts: chars.map((_, i) => (x + 4.8 * i) / W).concat((x + 4.8 * chars.length) / W) };
};
async function scan8(lines, patterns, withMap) {
  const saved = LINE.item;
  LINE.item = item8;
  try { return await scan(lines, patterns, withMap ? mapOf(lines.map((l) => run8(...l))) : null); } finally { LINE.item = saved; }
}
const STEP = [[100, 208, 'the first column'], [330, 214, 'holds a secret word']]; // baselines 208 and 214

test('the estimate is drawn on the baseline of the item the match is in, not on its row\'s first item\'s', async () => {
  assert.equal(buildTextRows(STEP.map((l) => { const t = item8(...l); return { str: t.str, x: l[0], y: l[1], w: t.width, h: 8 }; })).length, 1, 'the fixture is not one row: nothing here is tested');
  // "secret word" is in the lower item, the row's last characters. Its letters stand from 206 to 214, descenders below.
  const [low] = (await scan8(STEP, [re('secret word')], false)).boxes;
  assert.ok(low[1] <= 206 && low[3] >= 216, `the box stops short of the word it is for (206–216): ${JSON.stringify(low)} — the feet of its letters would show`);
  assert.deepEqual([low[1], low[3]], [204.8, 217.6]);
  // A match in the upper item is where it always was.
  const [high] = (await scan8(STEP, [re('first')], false)).boxes;
  assert.deepEqual([high[1], high[3]], [198.8, 211.6]);
  // A match over both items of the row (the estimate's row string runs them together) reaches both baselines.
  const [both] = (await scan8(STEP, [re('columnholds')], false)).boxes;
  assert.deepEqual([both[1], both[3]], [198.8, 217.6]);
});

test('…so the map\'s box holds its centre and replaces it: one box, and the line above is not taken', async () => {
  const got = await scan8(STEP, [re('secret')], true);
  assert.equal(got.boxes.length, 1, `the estimate was kept beside the exact box: ${JSON.stringify(got.boxes)}`);
  assert.deepEqual(got.boxes[0], [367.7, 205.5, 398, 216.5]);
});

// ── the rule itself ──────────────────────────────────────────────────────────
const piece = (row, x0, s) => ({ row, x0, x1: x0 + 6 * s.length, s });
const texts = (ms) => ms.map((parts) => parts.map((p) => p.piece.s.slice(p.from, p.to)));

test('a line end is a space; a space that pdf.js left at the end of a line or the start of the next is not a second one', () => {
  assert.deepEqual(texts(wrappedMatches([piece(0, 100, 'pay John'), piece(1, 100, 'Smith now')], [re('john smith')])), [['John', 'Smith']]);
  assert.deepEqual(texts(wrappedMatches([piece(0, 100, 'pay John  '), piece(1, 100, ' Smith now')], [re('john smith')])), [['John', 'Smith']]);
  // …and it is a space, not nothing: "JohnSmith" is not on this page.
  assert.deepEqual(wrappedMatches([piece(0, 100, 'pay John'), piece(1, 100, 'Smith now')], [re('johnsmith')]), []);
  // A piece of nothing but spaces is not a line.
  assert.deepEqual(texts(wrappedMatches([piece(0, 100, 'pay John'), piece(1, 100, '   '), piece(2, 100, 'Smith')], [re('john smith')])), [['John', 'Smith']]);
});

test('a match inside one piece is the row\'s own and is not found again; a pattern is run from its start', () => {
  const p = re('john');
  p.lastIndex = 7;
  assert.deepEqual(wrappedMatches([piece(0, 100, 'pay John'), piece(1, 100, 'John Smith')], [p]), []);
  const q = re('john smith');
  q.lastIndex = 7;
  assert.equal(wrappedMatches([piece(0, 100, 'pay John'), piece(1, 100, 'Smith')], [q]).length, 1, 'a RegExp left part-way through was not run from the start');
});

test('a line that ends in a hyphen is also read joined: with the hyphen for a number, without it for a broken word', () => {
  const ssn = re('\\b\\d{3}-\\d{2}-\\d{4}\\b', 'g');
  assert.deepEqual(texts(wrappedMatches([piece(0, 100, 'SSN 123-45-'), piece(1, 100, '6789 on file')], [ssn])), [['123-45-', '6789']]);
  assert.deepEqual(texts(wrappedMatches([piece(0, 100, 'strictly confi-'), piece(1, 100, 'dential terms')], [re('confidential')])), [['confi', 'dential']]);
  // A soft hyphen (U+00AD) — what a typesetter leaves at a break — is a hyphen.
  assert.deepEqual(texts(wrappedMatches([piece(0, 100, 'confi' + String.fromCharCode(0xAD)), piece(1, 100, 'dential')], [re('confidential')])), [['confi', 'dential']]);
  // A hyphen is not dropped anywhere but a line's end, and a line with none is not joined.
  assert.deepEqual(wrappedMatches([piece(0, 100, 'co-nfi'), piece(1, 100, 'dential')], [re('confidential')]), []);
  assert.deepEqual(wrappedMatches([piece(0, 100, 'strictly confi'), piece(1, 100, 'dential terms')], [re('confidential')]), []);
  // The hyphenated line still ends in a space for a phrase: "well-" / "known" is not it, but "a -" / "b" is "- b".
  assert.deepEqual(texts(wrappedMatches([piece(0, 100, 'item -'), piece(1, 100, 'b')], [re('- b')])), [['-', 'b']]);
  assert.deepEqual(wrappedMatches([piece(0, 100, 'a-b confi'), piece(1, 100, 'dential')], [re('confidential')]), [], 'a hyphen inside a line joined its end to the next');
  // A match that two of the readings find is one match.
  assert.deepEqual(texts(wrappedMatches([piece(0, 100, 'item -'), piece(1, 100, 'b')], [re('-\\s?b')])), [['-', 'b']]);
});

test('text wraps from the end of its own column: across a gutter is not the next line, and both columns are tried', () => {
  // Two columns, two lines. Column one: "…pay John" / "Smith the…"; column two: "…see Mary" / "Jones for…".
  const cols = [piece(0, 50, 'will pay John'), piece(0, 330, 'and see Mary'), piece(1, 50, 'Smith the sum'), piece(1, 330, 'Jones for it')];
  assert.deepEqual(texts(wrappedMatches(cols, [re('john smith')])), [['John', 'Smith']]);
  assert.deepEqual(texts(wrappedMatches(cols, [re('mary jones')])), [['Mary', 'Jones']]);
  // The end of column two's line does not run on into what is to its RIGHT on the next row, and the end of column
  // one's line does not run on into column two's next line.
  assert.deepEqual(wrappedMatches([piece(0, 50, 'pay John'), piece(1, 330, 'Smith')], [re('john smith')]), []);
  // It carries on in the NEXT row down that has anything under it — not the one after.
  assert.deepEqual(wrappedMatches([piece(0, 50, 'pay John'), piece(1, 50, 'and'), piece(2, 50, 'Smith')], [re('john smith')]), []);
  assert.deepEqual(wrappedMatches([piece(1, 50, 'Smith'), piece(0, 50, 'pay John')], [re('smith pay')]), [], 'text ran on UP the page');
});

test('a phrase over three lines is one match in three parts; over four it is not found', () => {
  const three = [piece(0, 100, 'the Right'), piece(1, 100, 'Honourable'), piece(2, 100, 'Member for')];
  assert.deepEqual(texts(wrappedMatches(three, [re('right honourable member')])), [['Right', 'Honourable', 'Member']]);
  const four = [piece(0, 100, 'a'), piece(1, 100, 'b'), piece(2, 100, 'c'), piece(3, 100, 'd')];
  assert.deepEqual(texts(wrappedMatches(four, [re('a b c')])), [['a', 'b', 'c']]);
  assert.deepEqual(wrappedMatches(four, [re('a b c d')]), []);
});

test('rowPieces: a row is one piece unless a gap wider than the text is tall splits it, and it knows where its characters are', () => {
  // One row: "pay " + "John" abutting (pdf.js split a line), then a gutter, then "Mary".
  const items = [{ str: 'pay ', x: 100, y: 210, w: 24, h: 10 }, { str: 'John', x: 124, y: 210, w: 24, h: 10 }, { str: 'Mary', x: 330, y: 210, w: 24, h: 10 },
    { str: 'Smith', x: 100, y: 224, w: 30, h: 10 }];
  const rows = buildTextRows(items);
  const ps = rowPieces(rows);
  assert.deepEqual(ps.map((p) => [p.row, p.x0, p.x1, p.s]), [[0, 100, 148, 'pay John'], [0, 330, 354, 'Mary'], [1, 100, 130, 'Smith']]);
  // row.s is "pay  John Mary": the items with a space between. Each piece's characters point back into it.
  assert.equal(ps[0].k.map((k) => rows[0].s[k]).join(''), 'pay John');
  assert.equal(ps[1].k.map((k) => rows[0].s[k]).join(''), 'Mary');
  // A gap of exactly the text's height is not a gutter.
  const near = buildTextRows([{ str: 'a', x: 100, y: 210, w: 6, h: 10 }, { str: 'b', x: 116, y: 210, w: 6, h: 10 }]);
  assert.equal(rowPieces(near).length, 1);
  // Rows are numbered down the page, whatever order they came in.
  const upside = rowPieces([{ y: 300, items: [{ str: 'low', x: 1, w: 3, h: 10 }] }, { y: 100, items: [{ str: 'high', x: 1, w: 4, h: 10 }] }]);
  assert.deepEqual(upside.map((p) => [p.row, p.s]), [[0, 'high'], [1, 'low']]);
});

test('the map splits a row at a gutter too, and a wrapped match over an OCR layer is boxed as its words are', () => {
  // Two columns in the map: "John" ends column one's line; column two's line is on the same row.
  const text = [run(50, 200, 'will pay John'), run(330, 200, 'and see Mary'), run(50, 214, 'Smith the sum'), run(330, 214, 'Jones for it')];
  const got = matchesInMap(mapOf(text), [re('mary jones')]);
  assert.equal(got.length, 2);
  assert.deepEqual(got.map((b) => Math.round(b[0] * W * 10) / 10), [377.3, 329.3]);
  // The end of column one's line does not run on into column two's NEXT line. (Into column two's line on the same
  // row it does, and always has: a row is searched whole, gutter and all, and that over-covers.)
  assert.equal(matchesInMap(mapOf(text), [re('john jones')]).length, 0, 'the end of column one ran on into column two');
  // A fitted OCR layer: each word its own hidden run with its ink; the wrapped parts are drawn to ink and carry hold.
  const ocr = [run(100, 200, 'John', { hidden: true, ink: [203 / H, 210 / H] }), run(100, 214, 'Smith', { hidden: true, ink: [217 / H, 224 / H] })];
  const boxes = matchesInMap(mapOf(ocr), [re('john smith')]);
  assert.deepEqual(boxes.map((b) => [Math.round(b[1] * H * 10) / 10, Math.round(b[3] * H * 10) / 10]), [[202.5, 210.5], [216.5, 224.5]]);
  assert.ok(boxes.every((b) => b.hold));
  // Print and the OCR layer are still two layers: a word of one does not wrap into a word of the other.
  assert.equal(matchesInMap(mapOf([run(100, 200, 'John'), run(100, 214, 'Smith', { hidden: true })]), [re('john smith')]).length, 0);
});
