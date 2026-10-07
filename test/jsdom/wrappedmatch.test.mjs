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

// ── an item read right to left ───────────────────────────────────────────────
// pdf.js hands a Hebrew item over in reading order and says `dir: 'rtl'`: its first character is at its RIGHT.
test('the estimate for part of a right-to-left item is drawn at the end its letters are at', async () => {
  const word = 'שלוםעולם'; // 8 letters, 100..148 across: the first four are the right half, 124..148
  const rtl = (x, y, str) => ({ ...item(x, y, str), dir: 'rtl' });
  const saved = LINE.item;
  LINE.item = rtl;
  try {
    const [first] = (await scan([[100, 200, word]], [re('שלום')], null)).boxes;
    assert.ok(first[0] > 112 && first[2] >= 148, `the first four letters are at 124..148 and the box is ${first[0]}..${first[2]}`);
    const [last] = (await scan([[100, 200, word]], [re('עולם')], null)).boxes;
    assert.ok(last[0] <= 100 && last[2] < 136, `the last four letters are at 100..124 and the box is ${last[0]}..${last[2]}`);
    // With the map's exact box for a word set in reverse (ADR-097), the estimate is replaced and not kept beside it.
    const rev = run(100, 200, word, { hidden: true, reversed: true });
    rev.cuts.reverse();
    const got = await scan([[100, 200, word]], [re('שלום')], mapOf([rev]));
    assert.deepEqual(got.boxes, [[123.3, 199.5, 148.8, 212.5]]);
  } finally { LINE.item = saved; }
  // A word of such an item still runs from its left edge to its right, for the field detectors that read `words`.
  const [w] = buildTextRows([{ str: word, x: 100, y: 210, w: 48, h: 10, dir: 'rtl' }])[0].words;
  assert.deepEqual([w.x0, w.x1], [100, 148]);
  // An item read left to right is laid out as it always was.
  const [ltr] = (await scan([[100, 200, 'abcdefgh']], [re('abcd')], null)).boxes;
  assert.ok(ltr[0] <= 100 && ltr[2] < 136, JSON.stringify(ltr));
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

// ── the cost of the rule, and its budget (ADR-100) ───────────────────────────
// wrappedMatches as it was first written (v1.189.6, ADR-095), kept here as the ORACLE: the function was rewritten for
// cost — each chain read every piece on the page to find its next row, and built its text a character at a time — and
// this is a redaction path, so "the same matches, in the same order" is the whole acceptance. Measured on one machine,
// five patterns, tables of lines × stretches a line, first version → this one: 60 × 12, 172 → 113 ms; 120 × 12,
// 311 → 200 ms; 200 × 20 with the budget lifted, 2,177 → 1,283 ms; the densest corpus page, 56 → 19 ms.
function asFirstWritten(pieces, patterns) {
  const LINE_HYPHEN = /[-­‐‑]$/, WRAP_LINES = 3;
  pieces = pieces.filter((p) => p.s.trim());
  const out = [], seen = new Set();
  const carriesOn = (a) => {
    const later = pieces.filter((b) => b.row > a.row && b.x0 < a.x1);
    const row = Math.min(...later.map((b) => b.row));
    return later.filter((b) => b.row === row);
  };
  const search = (chain) => {
    for (const mode of ['space', 'joined', 'dehyphenated']) {
      let s = '', hyphened = false;
      const at = [];
      chain.forEach((p, n) => {
        const last = n === chain.length - 1;
        const from = n ? p.s.length - p.s.trimStart().length : 0;
        let to = last ? p.s.length : p.s.trimEnd().length;
        const hyphen = !last && mode !== 'space' && LINE_HYPHEN.test(p.s.slice(0, to));
        if (hyphen && mode === 'dehyphenated') to--;
        for (let i = from; i < to; i++) { s += p.s[i]; at.push({ n, i }); }
        if (hyphen) hyphened = true; else if (!last) { s += ' '; at.push(null); }
      });
      if (mode !== 'space' && !hyphened) continue;
      for (const pattern of patterns) {
        pattern.lastIndex = 0;
        let m;
        while ((m = pattern.exec(s))) {
          if (!m[0].length) { pattern.lastIndex++; continue; }
          const parts = chain.map((piece) => ({ piece, from: Infinity, to: -Infinity }));
          for (let k = m.index; k < m.index + m[0].length; k++) {
            const g = at[k];
            if (!g || !s[k].trim()) continue;
            parts[g.n].from = Math.min(parts[g.n].from, g.i); parts[g.n].to = Math.max(parts[g.n].to, g.i + 1);
          }
          if (parts.some((p) => !(p.to > p.from))) continue;
          const key = parts.map((p) => [pieces.indexOf(p.piece), p.from, p.to].join(':')).join('|');
          if (seen.has(key)) continue;
          seen.add(key);
          out.push(parts);
        }
      }
    }
  };
  const walk = (chain) => {
    if (chain.length > 1) search(chain);
    if (chain.length < WRAP_LINES) for (const b of carriesOn(chain[chain.length - 1])) walk([...chain, b]);
  };
  for (const a of pieces) walk([a]);
  return out;
}

// The dialog's own patterns (app.js PII_PATTERNS) and two phrases, plus six that lean on what a rewrite could get
// wrong: a match that may be empty, one that reads past its own end, one that swallows the spaces at a line end, one
// loose enough to match across almost any line end, one that BEGINS with a space on its first line, and one that
// needs the spaces pdf.js left at the end of its last.
const SEARCHES = () => [re('john smith'), re('confidential'), /\b\d{3}[-.\s]?\d{2}[-.\s]?\d{4}\b/g, /\b[\w.+-]+@[\w-]+\.[\w.-]+\b/g,
  /(?:\+?1[-.\s]?)?\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4}\b/g, /\b(?:\d[ -]?){13,19}\b/g, /\d*/g, /smith(?=\s+the)/gi, /\s*total\s*/gi, /[a-z]+-?\s?[a-z]+/gi,
  /\s(john|total)\s+(smith|b)/gi, /(the|b|smith) john\s\s/gi];
const CELLS = ['pay John', 'Smith the sum', 'SSN 123-45-', '6789 on file', 'confi-', 'dential', 'confi­', 'co-nfi', ' Total ', '  ', 'a@b.', 'co.uk x',
  '4111 1111 1111', '1111 ok', '555-', '867-5309', '-', 'b', 'John  ', '  Smith', 'the', 'Smith', '1,234.00', 'x‐', '(555) 010-', '0199'];
// A page of pieces from a seed: rows down the page with gaps in their numbering, stretches at random places across
// — some overlapping, some out of order in the list — and now and then the same piece listed twice.
function pageOf(seed, lines, across) {
  let r = seed;
  const rnd = (n) => (r = (Math.imul(r, 1103515245) + 12345) & 0x7fffffff) % n;
  const ps = [];
  for (let l = 0, row = 0; l < lines; l++, row += 1 + (rnd(5) === 0 ? rnd(3) : 0)) {
    for (let c = rnd(across) ? 0 : 1, n = 1 + rnd(across); c < n; c++) {
      const s = CELLS[rnd(CELLS.length)], x0 = c * 90 + rnd(60) - 20;
      ps.push({ row, x0, x1: x0 + 6 * s.length, s });
      if (rnd(40) === 0) ps.push(ps[rnd(ps.length)]);
    }
  }
  if (seed % 3 === 0) ps.reverse();
  if (seed % 7 === 0) ps.sort((a, b) => a.x0 - b.x0);
  return ps;
}
const shape = (ps, ms) => ms.map((parts) => parts.map((p) => [ps.indexOf(p.piece), p.from, p.to].join(':')).join('|'));

test('the rewrite for cost finds exactly what the first version found, in the same order, on generated pages', () => {
  let matches = 0, threeLines = 0;
  for (let seed = 1; seed <= 400; seed++) {
    const ps = pageOf(seed, 3 + seed % 14, 1 + seed % 9);
    const want = asFirstWritten(ps, SEARCHES()), got = wrappedMatches(ps, SEARCHES());
    assert.deepEqual(shape(ps, got), shape(ps, want), `seed ${seed}`);
    assert.equal(got.skipped, undefined, `seed ${seed}: a small page was past the budget`);
    matches += want.length; threeLines += want.filter((p) => p.length === 3).length;
  }
  // Alone, because on the pages above a looser pattern finds the same two words first: the last line of a chain keeps
  // the spaces at its end, for a pattern that asks for them.
  const spaced = [piece(0, 100, 'the'), piece(1, 100, 'John  ')];
  assert.deepEqual(texts(wrappedMatches(spaced, [/the john\s\s/gi])), [['the', 'John']]);
  assert.deepEqual(texts(asFirstWritten(spaced, [/the john\s\s/gi])), [['the', 'John']]);
  // The oracle is only worth its agreement if the pages make it find things, three-line chains among them.
  assert.ok(matches > 5000 && threeLines > 300, `the generated pages found ${matches} matches, ${threeLines} over three lines`);
});

// A table of `lines` lines × `across` cells, every cell the same text.
const grid = (lines, across, s) => Array.from({ length: lines * across }, (_, i) => ({ row: Math.floor(i / across), x0: (i % across) * 100, x1: (i % across) * 100 + 80, s }));

test('past the budget the three-line chains are left out, then all of them — and the result says so each time', () => {
  // A cell carries on into the cells at or left of it on the next line: 210 chains of two lines for each pair of
  // lines of 20 cells, 1,540 of three for each three lines.
  // 30 × 20: 6,090 chains of two lines and 43,120 of three — inside the budget (200,000). Everything is searched.
  const all = wrappedMatches(grid(30, 20, 'b c a'), [re('a b c a b')]);
  assert.equal(all.skipped, undefined);
  assert.equal(all.length, 43120, 'the count of three-line chains this test rests on');
  // 120 × 20: 24,990 of two lines, 181,720 of three: together past the budget. Two lines are searched, three are not.
  const two = wrappedMatches(grid(120, 20, 'b c a'), [re('a b c a b'), re('c a b')]);
  assert.equal(two.skipped, true, 'a search that left chains out did not say so');
  assert.equal(two.length, 24990, 'every two-line chain is still searched');
  assert.ok(two.every((parts) => parts.length === 2));
  // 45 × 100: 222,200 chains of two lines alone. None is searched; a match on ONE line is the row's own search's.
  const vast = wrappedMatches(grid(45, 100, 'Smith x John'), [re('john smith')]);
  assert.equal(vast.skipped, true);
  assert.equal(vast.length, 0);
  // The map's reading says so too, through matchesInMap: a page of 45 lines × 100 runs a gutter apart.
  const text = [];
  for (let l = 0; l < 45; l++) for (let c = 0; c < 100; c++) text.push(run(c * 40, 100 + 14 * l, 'ab'));
  assert.equal(matchesInMap(mapOf(text), [re('ab ab')]).skipped, true);
  assert.equal(matchesInMap(mapOf(text.map((t) => ({ ...t, hidden: true }))), [re('ab ab')]).skipped, true, 'an OCR layer past the budget did not say so');
  assert.equal(matchesInMap(mapOf(LINES.map((l) => run(...l))), [re('john smith')]).skipped, undefined);
});

test('a search names the page it could not look across the line ends of, from either reading, and the dialog says it', async () => {
  // 45 lines of 100 two-letter cells, as pdf.js has them and as the map has them.
  const dense = [], runs = [];
  for (let l = 0; l < 45; l++) for (let c = 0; c < 100; c++) { dense.push([c * 40, 100 + 14 * l, 'ab']); runs.push(run(c * 40, 100 + 14 * l, 'ab')); }
  const skipped = async (lines, map) => {
    const owner = { pdfDocument: { numPages: 1, getPage: async () => ({ getViewport: () => viewport, getTextContent: async () => ({ items: lines.map((l) => item(...l)) }) }) } };
    const fn = new Function('pdfjsLib', 'buildTextRows', 'pageMap', 'placeMatches', 'matchesInMap', 'wrappedMatches', 'rowPieces', 'rowBaselines', 'view', scanText + '\nreturn scanTextMatches;');
    return (await fn(pdfjsLib, buildTextRows, async () => map, placeMatches, matchesInMap, wrappedMatches, rowPieces, detect.rowBaselines, owner)([re('john smith')], owner)).skipped;
  };
  assert.deepEqual(await skipped(dense, null), [1], 'the estimate\'s reading was past the budget and the page was not named');
  assert.deepEqual(await skipped(LINES, mapOf(runs)), [1], 'the map\'s reading was past the budget and the page was not named');
  assert.equal(await skipped(LINES, mapOf(LINES.map((l) => run(...l)))), undefined);
  // The words, and both places the dialog can end: with marks (the toast) and with none (its status line).
  const from = APP.indexOf('function wrapNote(');
  const wrapNote = new Function(APP.slice(from, APP.indexOf('\n}\n', from) + 2) + '\nreturn wrapNote;')();
  assert.equal(wrapNote(undefined), '');
  assert.equal(wrapNote([]), '');
  assert.equal(wrapNote([3]), ' Page 3: check by eye for a name or number split across lines — this page is too dense to search across line ends.');
  assert.equal(wrapNote([1, 2, 3, 4, 5, 6, 7, 8, 9, 10]), ' Pages 1, 2, 3, 4, 5, 6, 7, 8 and 2 more: check by eye for a name or number split across lines — these pages are too dense to search across line ends.');
  const find = APP.slice(APP.indexOf('els.rtFind.onclick'), from);
  assert.match(find, /No matches found[^\n]*wrapNote\(marks\.skipped\)/, 'a search that found nothing does not say which pages it could not fully search');
  assert.match(find, /toast\([^\n]*wrapNote\(marks\.skipped\)/, 'a search that marked matches does not say which pages it could not fully search');
});
