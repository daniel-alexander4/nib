// A file that was not a PDF becomes one, and the window says so once (ADR-185, S02b; D6, D12, W28, W31, Y12).
//
// ── What this tier can reach ─────────────────────────────────────────────────
// What the page does with `importedFrom` on an opening route's answer and with a hand-off event that carries
// `importedFrom` or a refusal `reason`: one polite-region sentence, coalesced for a batch, never repeated for the
// same document, and a refusal that says why. And that the Import… entry carries the new label.
//
// ── What it cannot ───────────────────────────────────────────────────────────
// A real conversion (Go's tests drive every route) and what a screen reader announces from the region.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const doc1 = { id: 'e:1', name: 'report.pdf', path: '', canSave: false, signature: { state: 'unsigned' }, canUndo: false, canRedo: false, importedFrom: 'report.docx' };

setNextDocument({ numPages: 1, outline: null });
const h = await boot({
  routes: {
    '/api/docs': () => ({ docs: [], activeId: '' }),
    '/api/lastopen': () => [],
    '/api/upload': () => doc1,
  },
});
const { document: doc, settle } = h;
const box = () => doc.getElementById('importNotice');
const text = () => doc.getElementById('importNoticeText').textContent;
const wait = (ms) => new Promise((r) => setTimeout(r, ms));

test('the notice is a polite status region that is not the session alert and starts hidden', async () => {
  await settle();
  assert.equal(box().hidden, true, 'setup: the notice is up before anything was imported');
  assert.equal(box().getAttribute('role'), 'status');
  assert.equal(box().getAttribute('aria-live'), 'polite');
  assert.notEqual(box().id, 'sessionNotice');
});

test('Import… folded into Open… (ADR-215) and the group is still Open & Import', () => {
  assert.equal(doc.getElementById('officeOpenBtn'), null, 'an Import… button is back beside Open…');
  assert.equal(doc.getElementById('openMenuItem').textContent.trim(), 'Open…');
  assert.equal(doc.querySelector('.tbgroup[data-label="Open & Import"]') !== null, true, 'the Open group still has its old name');
});

test('a hand-off that converted names the file once, however many times the stream replays it', async () => {
  h.pushWindowEvent('handoff', JSON.stringify({ result: 'opened', importedFrom: 'letter.docx', seq: 1 }));
  h.pushWindowEvent('handoff', JSON.stringify({ result: 'opened', importedFrom: 'letter.docx', seq: 1 }));
  await settle(); await wait(500); await settle();
  assert.equal(box().hidden, false, 'a converted hand-off said nothing');
  assert.equal(text(), 'Imported letter.docx as a PDF. Save writes a PDF; use Export for other formats.');
  doc.getElementById('importNoticeDismiss').onclick();
  assert.equal(box().hidden, true, 'Dismiss left the notice up');
});

test('the same file handed off again later is announced again - it is another event', async () => {
  h.pushWindowEvent('handoff', JSON.stringify({ result: 'opened', importedFrom: 'letter.docx', seq: 2 }));
  await settle(); await wait(500); await settle();
  assert.equal(box().hidden, false, 'a second hand-off of the same file name was swallowed as a replay (/pending 940)');
  assert.match(text(), /Imported letter\.docx as a PDF/);
  doc.getElementById('importNoticeDismiss').onclick();
  assert.equal(box().hidden, true);
});

test('several imports landing together are one sentence', async () => {
  for (const n of ['a.docx', 'b.xlsx', 'c.odt']) h.pushWindowEvent('handoff', JSON.stringify({ result: 'opened', importedFrom: n }));
  await settle(); await wait(500); await settle();
  assert.equal(text(), 'Imported 3 files as PDFs. Save writes a PDF; use Export for other formats.');
});

test('a refused hand-off says why in the import function\'s own words', async () => {
  h.pushWindowEvent('handoff', JSON.stringify({ result: 'refused', reason: 'Nib could not find LibreOffice, so it cannot convert this document.' }));
  await settle();
  assert.match((doc.getElementById('toast') || {}).textContent || '', /LibreOffice/, 'the refusal reason did not reach the window');
});

test('the Show Export action is wired and the notice moves no focus', () => {
  assert.equal(typeof doc.getElementById('importNoticeExport').onclick, 'function');
  assert.notEqual(doc.activeElement && doc.activeElement.id, 'importNotice');
});
