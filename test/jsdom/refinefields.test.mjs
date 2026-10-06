// refineFields (ADR-088): a field proposed from a picture of the page, corrected against where the page's text,
// ruled lines and existing fields really are.
//
// ── What this tier can reach ─────────────────────────────────────────────────
// The whole rule. It is a pure function over two lists of rectangles, so every case — a label above the blank, a
// label beside it, an underlined sentence, a rule through the middle, a field the document already has — is stated
// here as numbers and checked as numbers.
//
// ── What it cannot ───────────────────────────────────────────────────────────
// Whether real documents come out better. That is build/accuracy.sh, on real documents.
import test from 'node:test';
import assert from 'node:assert/strict';
import { refineFields } from '../../web/detect.js';

// A letter page: one point is 1/612 across and 1/792 down.
const X = (pt) => pt / 612, Y = (pt) => pt / 792;
const rect = (x0, y0, x1, y1) => [X(x0), Y(y0), X(x1), Y(y1)];
const page = (over = {}) => ({ page: 1, width: 612, height: 792, text: [], shapes: [], widgets: [], noText: false, ...over });
const label = (x0, y0, x1, y1, extra = {}) => ({ rect: rect(x0, y0, x1, y1), text: 'label', size: Y(y1 - y0), ...extra });
const near = (a, b) => Math.abs(a - b) < 0.0005;
const one = (r) => { assert.equal(r.fields.length, 1, `expected one field, got ${JSON.stringify(r.fields)}`); return r.fields[0].rect; };

test('a label printed in the top of a cell pushes the field below it, and the field keeps the cell\'s width', () => {
  // The cell is 100..300 across and 100..130 down; "Last" is printed small in its top-left corner.
  const got = one(refineFields([{ kind: 'text', rect: rect(100, 100, 300, 130) }], page({ text: [label(103, 102, 125, 111)] })));
  assert.ok(near(got[0], X(100)) && near(got[2], X(300)), `the field was narrowed to ${got}: a label ABOVE the blank does not take its width`);
  assert.ok(got[1] > Y(111) && got[1] < Y(113.5), `the field's top is ${got[1] * 792}pt, want just under the label at 111pt`);
  assert.ok(near(got[3], Y(130)));
});

test('a label beside the blank narrows the field to the blank', () => {
  // "Name:" sits on the same line as the blank, at its left.
  const got = one(refineFields([{ kind: 'text', rect: rect(100, 100, 400, 115) }], page({ text: [label(102, 102, 140, 114)] })));
  assert.ok(got[0] > X(140) && got[0] < X(143), `the field starts at ${got[0] * 612}pt, want just right of the label's 140pt`);
  assert.ok(near(got[2], X(400)) && near(got[1], Y(100)) && near(got[3], Y(115)));
});

test('text across the whole of it leaves no blank: an underlined sentence is not a field', () => {
  const r = refineFields([{ kind: 'text', rect: rect(100, 100, 400, 115) }], page({ text: [label(98, 101, 402, 114)] }));
  assert.deepEqual(r.fields, []);
  assert.equal(r.dropped, 1, 'a proposal was dropped and not counted');
});

test('the line above only grazing the field does not block it', () => {
  // A 10pt line whose descenders reach 2pt into the top of the field.
  const got = one(refineFields([{ kind: 'text', rect: rect(100, 100, 400, 115) }], page({ text: [label(100, 89.5, 400, 102)] })));
  assert.ok(near(got[0], X(100)) && near(got[2], X(400)), `text that only touches the top took the field's width: ${got}`);
  assert.ok(got[1] >= Y(102), 'the field still overlaps the line above');
});

test('a short field the line above grazes is neither moved nor blocked', () => {
  // 10pt tall: moving its top below the descenders would leave under a line's height, so it stays — and 2pt of a
  // 12.5pt line is a graze, not text standing in the blank.
  const got = one(refineFields([{ kind: 'text', rect: rect(100, 100, 400, 110) }], page({ text: [label(100, 89.5, 400, 102)] })));
  assert.ok(near(got[0], X(100)) && near(got[2], X(400)) && near(got[1], Y(100)), `the field became ${got}`);
});

test('a field that is narrowed keeps whatever else it carried', () => {
  const r = refineFields([{ kind: 'text', rect: rect(100, 100, 400, 115), from: 'underline' }], page({ text: [label(102, 102, 140, 114)] }));
  assert.equal(r.fields[0].from, 'underline');
});

test('a vertical rule through a field cuts it in two', () => {
  const r = refineFields([{ kind: 'text', rect: rect(100, 100, 400, 130) }],
    page({ shapes: [{ kind: 'v', rect: rect(249.75, 90, 250.25, 140) }] }));
  assert.equal(r.fields.length, 2, `a field spanning two cells stayed one: ${JSON.stringify(r.fields)}`);
  const [a, b] = r.fields.map((f) => f.rect).sort((p, q) => p[0] - q[0]);
  assert.ok(near(a[0], X(100)) && a[2] < X(249.75) && b[0] > X(250.25) && near(b[2], X(400)));
});

test('a rule at the field\'s own edge, or one that only clips a corner, cuts nothing', () => {
  for (const v of [rect(100.2, 90, 100.7, 140), rect(249.75, 90, 250.25, 105)]) {
    const r = refineFields([{ kind: 'text', rect: rect(100, 100, 400, 130) }], page({ shapes: [{ kind: 'v', rect: v }] }));
    assert.equal(r.fields.length, 1, `cut by a rule at ${v}`);
    assert.ok(near(r.fields[0].rect[0], X(100)) && near(r.fields[0].rect[2], X(400)), `a rule at ${v} moved the field's edge to ${r.fields[0].rect}`);
  }
});

test('nothing is proposed where the document already has a field, whatever its kind', () => {
  const widgets = [{ kind: 'text', name: 'surname', rect: rect(100, 100, 300, 120) }, { kind: 'check', name: 'agree', rect: rect(320, 100, 332, 112) }];
  const r = refineFields([
    { kind: 'text', rect: rect(98, 104, 302, 119) },   // over the real text field
    { kind: 'check', rect: rect(321, 101, 331, 111) }, // over the real checkbox
    { kind: 'text', rect: rect(100, 200, 300, 215) },  // somewhere else
  ], page({ widgets }));
  assert.equal(r.fields.length, 1);
  assert.ok(near(r.fields[0].rect[1], Y(200)), 'the wrong proposal survived');
  assert.equal(r.dropped, 2);
});

test('hidden text — an OCR layer — is not print, and blocks nothing', () => {
  const got = one(refineFields([{ kind: 'text', rect: rect(100, 100, 400, 115) }], page({ text: [label(98, 101, 402, 114, { hidden: true })] })));
  assert.ok(near(got[0], X(100)) && near(got[2], X(400)));
});

test('what is left must still be big enough to type in', () => {
  // Labels leave a 10pt gap between them: narrower than a field.
  const r = refineFields([{ kind: 'text', rect: rect(100, 100, 300, 115) }], page({ text: [label(100, 101, 190, 114), label(203, 101, 300, 114)] }));
  assert.deepEqual(r.fields, []);
});

test('a checkbox and a choice group are left where they were proposed, and carry what they came with', () => {
  const choices = [{ rect: rect(100, 100, 120, 112), word: true }];
  const r = refineFields([{ kind: 'check', rect: rect(50, 50, 62, 62) }, { kind: 'circleone', rect: rect(100, 100, 160, 112), choices }],
    page({ text: [label(48, 48, 64, 64), label(100, 100, 160, 112)] }));
  assert.equal(r.fields.length, 2);
  assert.deepEqual(r.fields[1].choices, choices, 'a choice group lost its choices on the way through');
});
