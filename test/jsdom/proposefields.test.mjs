// proposeFields (ADR-089): the fields a page draws for itself, read from the page map — and mergeProposals, which
// puts that reading together with the picture's.
//
// ── What this tier can reach ─────────────────────────────────────────────────
// The whole rule. It is a pure function over rectangles, so a form is stated here as numbers — its lines, its
// labels, its squares — and where each field must land is checked as numbers.
//
// ── What it cannot ───────────────────────────────────────────────────────────
// Whether real forms are drawn the way these are. That is build/accuracy.sh, on real documents; the shapes below are
// the ones it found there (a row's top edge drawn one piece per cell, an upright drawn one piece per row, a heavy
// rule drawn as a filled bar, "-" printed inside a phone number's blank).
import test from 'node:test';
import assert from 'node:assert/strict';
import { proposeFields, mergeProposals } from '../../web/detect.js';

// A letter page: one point is 1/612 across and 1/792 down.
const X = (pt) => pt / 612, Y = (pt) => pt / 792;
const rect = (x0, y0, x1, y1) => [X(x0), Y(y0), X(x1), Y(y1)];
const page = (over = {}) => ({ page: 1, width: 612, height: 792, text: [], shapes: [], widgets: [], noText: false, ...over });
const h = (x0, x1, y) => ({ kind: 'h', rect: rect(x0, y - 0.25, x1, y + 0.25) });
const v = (x, y0, y1) => ({ kind: 'v', rect: rect(x - 0.25, y0, x + 0.25, y1) });
const box = (x0, y0, x1, y1, filled = false) => ({ kind: 'box', rect: rect(x0, y0, x1, y1), filled });
const white = (x0, y0, x1, y1) => ({ kind: 'white', rect: rect(x0, y0, x1, y1) });
const label = (x0, y0, x1, y1, text = 'Label', extra = {}) => ({ rect: rect(x0, y0, x1, y1), text, size: Y(y1 - y0), ...extra });
const pts = (f) => [f.rect[0] * 612, f.rect[1] * 792, f.rect[2] * 612, f.rect[3] * 792].map((n) => Math.round(n * 10) / 10);
const about = (got, want, slack = 1.6) => got.every((n, i) => Math.abs(n - want[i]) <= slack);
const sole = (fields, kind) => {
  assert.equal(fields.length, 1, `expected one field, got ${JSON.stringify(fields.map((f) => [f.kind, f.from, pts(f)]))}`);
  assert.equal(fields[0].kind, kind);
  return pts(fields[0]);
};

// One row of a table, 100..400 across and 100..124 down, drawn the way the corpus's forms draw it.
const rowOf = (...ups) => [h(100, 400, 100), h(100, 400, 124), ...ups.map((x) => v(x, 100, 124))];

test('a cell with its label in the top is a field below the label, edge to edge', () => {
  const got = sole(proposeFields(page({ shapes: rowOf(100, 400), text: [label(104, 101, 130, 112, 'City:')] })), 'text');
  assert.ok(about(got, [101, 111, 399, 123]), `the field is ${got}`);
});

test('uprights between two lines cut the row into its cells, and an empty row with none is a line to write on', () => {
  const cells = proposeFields(page({ shapes: rowOf(100, 250, 400), text: [label(104, 101, 130, 112), label(254, 101, 280, 112)] }));
  assert.deepEqual(cells.map((f) => f.kind), ['text', 'text']);
  const [a, b] = cells.map(pts).sort((p, q) => p[0] - q[0]);
  assert.ok(about(a, [101, 111, 249, 123]) && about(b, [251, 111, 399, 123]), `cells ${a} and ${b}`);
  // No upright and nothing printed: the lower line is written on, one line's height above it.
  const bare = sole(proposeFields(page({ shapes: [h(100, 400, 100), h(100, 400, 140)] })).filter((f) => f.rect[1] > Y(110)), 'text');
  assert.ok(about(bare, [101, 124, 399, 139]), `the field on a bare line is ${bare}`);
});

test('a row whose edges are drawn in pieces, and an upright drawn one piece per row, are still one grid', () => {
  // The top and bottom edges are one piece per cell; the cell on the right is two rows deep, its upright in two pieces.
  const shapes = [
    h(100, 249.8, 100), h(250.2, 400, 100),
    h(100, 249.8, 124),                      // closes the left cell only
    h(100, 249.8, 148), h(250.2, 400, 148),
    v(250, 100.2, 124), v(250, 124, 148),
  ];
  const fields = proposeFields(page({ shapes, text: [label(254, 101, 300, 112, 'Mailing address:')] }));
  const deep = fields.map(pts).find((p) => p[0] > 250);
  assert.ok(deep && about(deep, [251, 111, 399, 147]), `the two-row cell came out as ${JSON.stringify(fields.map(pts))}`);
  assert.equal(fields.filter((f) => f.rect[0] < X(250)).length, 2, 'the two cells on the left');
});

test('a label beside the blank narrows the field, and separators printed in the blank do not', () => {
  const beside = sole(proposeFields(page({ shapes: rowOf(100, 400), text: [label(104, 106, 170, 118, 'From month/year:')] })), 'text');
  assert.ok(beside[0] > 170 && beside[0] < 173 && Math.abs(beside[2] - 399) < 1, `beside a label: ${beside}`);
  const phone = sole(proposeFields(page({ shapes: rowOf(100, 400), text: [label(104, 101, 160, 112, 'Phone number:'), label(150, 112, 153, 123, '-'), label(190, 112, 193, 123, '-')] })), 'text');
  assert.ok(about(phone, [101, 111, 399, 123]), `the "-" of a phone number cut its blank: ${phone}`);
});

test('a cell with two blanks in it gets two fields', () => {
  const fields = proposeFields(page({ shapes: rowOf(100, 400), text: [label(230, 108, 245, 120, 'ext')] }));
  assert.equal(fields.length, 2, JSON.stringify(fields.map(pts)));
});

test('a cell full of print proposes nothing, and neither does a shaded panel', () => {
  assert.deepEqual(proposeFields(page({ shapes: rowOf(100, 400), text: [label(102, 106, 398, 118, 'This row is a sentence.')] })), []);
  assert.deepEqual(proposeFields(page({ shapes: [...rowOf(100, 400), box(100, 100, 400, 124, true)] })), []);
});

test('a small empty square is a checkbox, and a cell holding one gets no text field', () => {
  const fields = proposeFields(page({ shapes: [...rowOf(100, 400), box(110, 107, 120, 117)], text: [label(124, 106, 150, 118, 'Yes')] }));
  assert.deepEqual(fields.map((f) => [f.kind, f.from]), [['check', 'square']]);
  // A square with a character in it is a drawn tick or a bullet, not an empty box.
  assert.deepEqual(proposeFields(page({ shapes: [box(110, 107, 120, 117)], text: [label(111, 107, 119, 117, 'X')] })), []);
});

test('a line that closes no row is written on above it, beside its label — and a heavy bar is not written on', () => {
  const got = sole(proposeFields(page({ shapes: [h(100, 400, 300)], text: [label(100, 288, 140, 300, 'Signed')] })), 'text');
  assert.ok(got[0] > 140 && Math.abs(got[2] - 400) < 1 && Math.abs(got[1] - 285) < 1, `on a bare line: ${got}`);
  // The same line drawn 3pt thick, as a filled bar: a section divider.
  assert.deepEqual(proposeFields(page({ shapes: [box(100, 298.5, 400, 301.5, true)] })), []);
});

test('underscores are a blank, and a box drawn as a character is a checkbox', () => {
  const chars = [...'Name: ______ ☐'];
  const cuts = chars.map((_, i) => X(100 + 6 * i)); cuts.push(X(100 + 6 * chars.length));
  const fields = proposeFields(page({ text: [label(100, 200, 100 + 6 * chars.length, 212, chars.join(''), { chars, cuts })] }));
  assert.deepEqual(fields.map((f) => [f.kind, f.from]).sort(), [['check', 'glyph'], ['text', 'underscores']]);
  const blank = pts(fields.find((f) => f.kind === 'text'));
  assert.ok(about(blank, [136, 200, 172, 212], 0.1), `the blank is ${blank}, want the six underscores`);
  // Three underscores is a rule in a sentence, not a blank.
  const c3 = [...'a ___ b'], k3 = c3.map((_, i) => X(100 + 6 * i)); k3.push(X(100 + 6 * c3.length));
  assert.deepEqual(proposeFields(page({ text: [label(100, 200, 142, 212, c3.join(''), { chars: c3, cuts: k3 })] })), []);
});

test('nothing is proposed where the document already has a field, on a page with no text, or from hidden text', () => {
  const shapes = rowOf(100, 400);
  assert.deepEqual(proposeFields(page({ shapes, widgets: [{ kind: 'text', name: 'city', rect: rect(100, 104, 400, 124) }] })), []);
  assert.deepEqual(proposeFields(page({ shapes, noText: true })), [], 'a page whose text is outlines: a label cannot be told from a blank');
  assert.deepEqual(proposeFields(null), []);
  // An OCR layer over a cell is not print: the cell is still blank, edge to edge.
  const got = sole(proposeFields(page({ shapes: [...shapes], text: [label(102, 106, 398, 118, 'ocr words', { hidden: true })] })), 'text');
  assert.ok(Math.abs(got[0] - 101) < 1 && Math.abs(got[2] - 399) < 1);
});

test('the picture adds only where the map proposed nothing', () => {
  const fromMap = [{ kind: 'text', rect: rect(100, 100, 300, 120), from: 'cell' }];
  const merged = mergeProposals(fromMap, [
    { kind: 'text', rect: rect(280, 105, 296, 120) },  // a sliver inside the map's field
    { kind: 'check', rect: rect(400, 100, 412, 112) }, // somewhere the map proposed nothing
  ]);
  assert.deepEqual(merged.map((f) => f.kind), ['text', 'check']);
  assert.equal(merged[0].from, 'cell', 'the map\'s own field was replaced');
  assert.equal(mergeProposals([], [{ kind: 'text', rect: rect(1, 1, 2, 2) }]).length, 1, 'with no map reading the picture stands');
});

test('a top edge drawn in pieces that do not meet at an upright is still one edge', () => {
  // The edge breaks at 250; the uprights stand at 100, 300 and 400. The cells are the uprights', not the pieces'.
  const shapes = [h(100, 249.8, 100), h(250.2, 400, 100), h(100, 400, 124), v(100, 100, 124), v(300, 100, 124), v(400, 100, 124)];
  const got = proposeFields(page({ shapes })).map(pts).sort((p, q) => p[0] - q[0]);
  assert.equal(got.length, 2, JSON.stringify(got));
  assert.ok(about(got[0], [101, 101, 299, 123]) && about(got[1], [301, 101, 399, 123]), JSON.stringify(got));
});

test('a stray short line under a cell does not close it', () => {
  // A 10pt tick under the label, then the row's real bottom edge.
  const shapes = [h(100, 400, 100), h(104, 114, 112), h(100, 400, 124), v(100, 100, 124), v(400, 100, 124)];
  const got = sole(proposeFields(page({ shapes })), 'text');
  assert.ok(about(got, [101, 101, 399, 123]), `the cell is ${got}`);
});

test('two lines with no upright between them are a row only when they are one width and close together', () => {
  // Far apart: two lines to write on.
  const far = proposeFields(page({ shapes: [h(100, 400, 200), h(100, 400, 400)] }));
  assert.deepEqual(far.map((f) => Math.round(pts(f)[3])).sort(), [200, 400], JSON.stringify(far.map(pts)));
  // Close, and of different widths: a signature line over a date line, not a table.
  const uneven = proposeFields(page({ shapes: [h(100, 400, 200), h(100, 300, 230)] }));
  assert.deepEqual(uneven.map((f) => Math.round(pts(f)[3])).sort(), [200, 230], JSON.stringify(uneven.map(pts)));
});

test('print standing in the room above a bare line leaves no blank there: an underlined sentence is not a field', () => {
  // The sentence's box reaches 6pt into the 15pt above the line. Under a cell's rule that would be a label to step below.
  assert.deepEqual(proposeFields(page({ shapes: [h(100, 400, 300)], text: [label(100, 281, 400, 291, 'A sentence that is underlined.')] })), []);
});

// ── white grounds (ADR-096) ──────────────────────────────────────────────────
// The shapes are an IRS 1040's as a form designer wrote it: a tint over the page, a white rectangle under each blank
// (72 x 12 for an amount, 8 x 8 for a tick box, 24 deep with the label in its top), neighbours sharing an edge.
const shapesOf = (fields) => fields.map((f) => [f.kind, f.from, pts(f)]);

test('a white ground on a tinted panel is a field: the whole of it, or below the label printed in its top', () => {
  const panel = box(50, 50, 560, 700, true);
  const got = sole(proposeFields(page({ shapes: [panel, white(504, 450, 576, 462)] })), 'text');
  assert.ok(about(got, [504, 450, 576, 462], 0.1), `the field is ${got}, want the ground itself`);

  const labelled = proposeFields(page({ shapes: [panel, white(100, 200, 316, 224)], text: [label(104, 201, 160, 210, 'First name')] }));
  const f = sole(labelled, 'text');
  assert.ok(about(f, [100, 209, 316, 224]), `the field is ${f}, want it below the label`);
  assert.equal(labelled[0].from, 'ground');

  assert.equal(sole(proposeFields(page({ shapes: [panel, white(97, 206, 105, 214)] })), 'check').join(), '97,206,105,214');

  // The same blank drawn with lines inside the panel is still refused: a ground is the one thing read over a shade.
  assert.deepEqual(proposeFields(page({ shapes: [panel, ...rowOf(100, 400)] })), []);
});

test('a small white square is a tick box, and the ground it stands in gets no text field', () => {
  const got = proposeFields(page({ shapes: [white(36, 60, 151, 72), white(36, 62, 44, 70)], text: [label(48, 61, 90, 70, 'Single')] }));
  assert.equal(sole(got, 'check').join(), '36,62,44,70');
  // A square too large to tick is typed in, and one that is not square is not ticked.
  assert.equal(sole(proposeFields(page({ shapes: [white(100, 100, 122, 120)] })), 'text').join(), '100,100,122,120');
  assert.deepEqual(proposeFields(page({ shapes: [white(100, 100, 118, 106)] })), [], 'an 18 x 6 sliver is neither a tick box nor room to type');
  assert.equal(sole(proposeFields(page({ shapes: [white(100, 100, 120, 122)] })), 'text').join(), '100,100,120,122', '22 deep is past a tick box');
});

test('print that only touches a tick box does not refuse it, and print standing in it does', () => {
  const tick = white(568, 146, 576, 154);
  assert.equal(proposeFields(page({ shapes: [tick], text: [label(470, 146, 568.7, 154, 'was in')] })).length, 1);
  assert.deepEqual(proposeFields(page({ shapes: [tick], text: [label(470, 146, 573, 154, 'was in')] })), []);
});

test('two grounds sharing an edge are two fields, and a ground the lines already read is not proposed again', () => {
  const pair = proposeFields(page({ shapes: [white(100, 100, 200.2, 112), white(200, 100, 300, 112)] }));
  assert.equal(pair.length, 2, `got ${JSON.stringify(shapesOf(pair))}`);

  const once = proposeFields(page({ shapes: [...rowOf(100, 400), white(100.5, 100.5, 399.5, 123.5)] }));
  assert.ok(about(sole(once, 'text'), [101, 101, 399, 123]));
  assert.equal(once[0].from, 'cell', 'where both see the blank, the lines\' reading stands');
});

test('a ground deeper than a row is a panel and not a blank, a speck is nothing, and a ground inside a ground is the blank', () => {
  assert.deepEqual(proposeFields(page({ shapes: [white(100, 100, 400, 200)] })), []);
  assert.deepEqual(proposeFields(page({ shapes: [white(100, 100, 104, 107)] })), [], '4 across is a speck');
  assert.deepEqual(proposeFields(page({ shapes: [white(100, 100, 107, 104)] })), [], '4 deep is a speck');
  const nested = proposeFields(page({ shapes: [white(100, 100, 400, 130), white(250, 105, 390, 125)] }));
  assert.equal(sole(nested, 'text').join(), '250,105,390,125');
});

test('a ground is not read on a page with no text, nor where the document already has a field', () => {
  assert.deepEqual(proposeFields(page({ shapes: [white(504, 450, 576, 462)], noText: true })), []);
  assert.deepEqual(proposeFields(page({ shapes: [white(504, 450, 576, 462)], widgets: [{ kind: 'text', rect: rect(504, 450, 576, 462) }] })), []);
});

test('boxes stacked in a column are one field each: the gap between two is not a line to write on', () => {
  // An attendance sheet's column: a 12pt box per row, 6pt apart, each outlined on its own.
  const got = proposeFields(page({ shapes: [box(60, 220, 124, 232), box(60, 238, 124, 250), box(60, 256, 124, 268)] }));
  assert.equal(got.length, 3, `got ${JSON.stringify(shapesOf(got))}`);
  got.forEach((f, i) => assert.ok(about(pts(f), [61, 221 + 18 * i, 123, 231 + 18 * i]), `field ${i} is ${pts(f)}`));

  // A short stray line just under a cell's top edge closes nothing: the cell is still one field, edge to edge.
  const stray = proposeFields(page({ shapes: [...rowOf(100, 400), h(200, 210, 103)] }));
  assert.ok(about(sole(stray, 'text'), [101, 101, 399, 123]), `got ${JSON.stringify(shapesOf(stray))}`);

  // A second rule under the left half of a line closes that half only: the right half is still a row to the line below.
  const half = proposeFields(page({ shapes: [h(100, 400, 100), h(100, 250, 103), h(100, 400, 124)] }));
  assert.ok(half.some((f) => about(pts(f), [251, 109, 399, 123])), `got ${JSON.stringify(shapesOf(half))}`);
});
