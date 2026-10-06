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
