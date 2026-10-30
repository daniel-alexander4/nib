// A check box whose two widgets carry different on-states — an Acrobat Yes/No pair — keeps the
// answer the user ticked when the window saves (/pending 893, ADR-172).
//
// pdf.js shows such a field as two boxes and saves each widget in turn; every widget it saves sets the
// field's /V, and only the FIRST to be saved is kept. The box the user did not tick saves /V Off, so a
// "No" whose widget comes second was written as no answer at all — its /AS right, so the window reopened
// it ticked and nothing looked wrong, while every reader of /V (pdfcpu's export, `nib export-xfdf`, a
// mail-merge, the other party's software) read nothing. Measured on real forms: 20 of 180 such answers
// in five documents (`build/formrt.sh`).
//
// The value is read back through pdfcpu's export of the saved file, which reads /V: asking pdf.js would
// ask the thing under test, and it reads each widget from its /AS.
//
// jsdom cannot see any of this: tier 2 stubs the viewer, and the save is pdf.js's worker.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch, shutdown } from './harness.mjs';
import { writeRawFixture } from './fixtures.mjs';

// One page, one field "Answer" (/FT /Btn, no radio flag) answered Yes, with two widgets: the Yes box
// first in /Kids and /Annots, then the No box — the order in which ticking No used to lose it.
function checkPairPDF() {
  const box = (s) => `<< /Type /XObject /Subtype /Form /BBox [0 0 12 12] /Length ${s.length} >>\nstream\n${s}\nendstream`;
  const on = '0 g 2 2 8 8 re f';
  const off = '0 G 0.5 0.5 11 11 re S';
  const objs = [
    '<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [5 0 R] >> >>',
    '<< /Type /Pages /Kids [3 0 R] /Count 1 >>',
    '<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R /Annots [6 0 R 7 0 R] >>',
    '<< /Length 0 >>\nstream\n\nendstream',
    '<< /FT /Btn /T (Answer) /V /Yes /Kids [6 0 R 7 0 R] >>',
    '<< /Type /Annot /Subtype /Widget /Parent 5 0 R /P 3 0 R /Rect [40 90 52 102] /F 4 /AS /Yes /AP << /N << /Yes 8 0 R /Off 9 0 R >> >> >>',
    '<< /Type /Annot /Subtype /Widget /Parent 5 0 R /P 3 0 R /Rect [140 90 152 102] /F 4 /AS /Off /AP << /N << /No 10 0 R /Off 11 0 R >> >> >>',
    box(on), box(off), box(on), box(off),
  ];
  let out = '%PDF-1.7\n';
  const offsets = [];
  objs.forEach((o, i) => {
    offsets.push(out.length);
    out += `${i + 1} 0 obj\n${o}\nendobj\n`;
  });
  const xref = out.length;
  out += `xref\n0 ${objs.length + 1}\n0000000000 65535 f \n`;
  for (const o of offsets) out += `${String(o).padStart(10, '0')} 00000 n \n`;
  out += `trailer\n<< /Size ${objs.length + 1} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF\n`;
  return Buffer.from(out, 'latin1');
}

const DOC = writeRawFixture('checkpair.pdf', checkPairPDF());
const BOXES = '.viewerContainer:not([hidden]) .annotationLayer input[type="checkbox"]';

const h = await launch();
const { page } = h;
after(() => shutdown(h));

// What pdfcpu reads as the field's value in the document now open. v0.16.2 reads this pair, which has no Radio
// flag, as ONE check box (ADR-174 §4, D5; ADR-177 adds the answer's name as `state`): a boolean, true when /V names any on-state — a Yes and a No both read
// `ticked`, where v0.13 counted the widgets and listed a radio group naming the state. So this asks the half that
// still matters: that an answer is STORED. The bug it pins wrote /V Off, which reads `unticked`; WHICH box
// answered is the window's /AS, and is asserted below from the boxes themselves.
const storedAnswer = () => page.evaluate(async () => {
  const r = await nibFetch('/api/form-data');
  if (!r.ok) return `HTTP ${r.status}`;
  const form = ((await r.json()).forms || [])[0] || {};
  const box = (form.checkbox || []).find((g) => g.name === 'Answer');
  return box ? (box.value ? 'ticked' : 'unticked') : 'not listed';
});

test('ticking the second box of a Yes/No pair is the answer the saved file stores', async () => {
  await h.openDocument(DOC, 1);
  await page.waitForFunction((sel) => document.querySelectorAll(sel).length === 2, BOXES);
  assert.equal(await storedAnswer(), 'ticked', 'setup: the fixture does not open answered Yes, so nothing below is about the change');

  await page.locator(BOXES).nth(1).click();
  // The stimulus: the window shows No ticked and Yes cleared before anything is saved.
  assert.deepEqual(await page.$$eval(BOXES, (els) => els.map((e) => e.checked)), [false, true],
    'setup: clicking the No box did not tick it and clear Yes, so the save below is not of a changed answer');

  await h.mode('file');
  await page.click('#saveBtn');
  await page.waitForFunction(() => document.getElementById('toast')?.textContent === 'Saved');
  await h.closeDocument();
  await h.openDocument(DOC, 1);
  await page.waitForFunction((sel) => document.querySelectorAll(sel).length === 2, BOXES);

  assert.equal(await storedAnswer(), 'ticked',
    'the saved file stores no answer (/V Off) although No was ticked — pdf.js let the box the user did not tick write /V Off');
  assert.deepEqual(await page.$$eval(BOXES, (els) => els.map((e) => e.checked)), [false, true],
    'the reopened window does not show No ticked');
});

// The other order, and the half a looser fix breaks: the ticked box is now saved FIRST and the unticked
// No box after it, so a fix that let any later widget write /V would store Off here.
test('ticking the first box back is the answer the saved file stores', async () => {
  await page.locator(BOXES).nth(0).click();
  assert.deepEqual(await page.$$eval(BOXES, (els) => els.map((e) => e.checked)), [true, false],
    'setup: clicking the Yes box did not tick it and clear No');
  await h.mode('file');
  await page.click('#saveBtn');
  await page.waitForFunction(() => document.getElementById('toast')?.textContent === 'Saved');
  await h.closeDocument();
  await h.openDocument(DOC, 1);
  await page.waitForFunction((sel) => document.querySelectorAll(sel).length === 2, BOXES);
  assert.equal(await storedAnswer(), 'ticked',
    'the saved file stores no answer although Yes was ticked — the No box, saved after it, overwrote /V');
});

test('this file leaves the shared server as it found it', async () => {
  await h.closeAll();
});
