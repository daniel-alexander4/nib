// A converted document's "its PDF isn't saved" mark clears when its PDF is saved, and only then (/pending 940 #4).
//
// ── What this tier can reach ─────────────────────────────────────────────────
// The wording Quit gives for an edited view that was converted, before and after a Save As of its PDF, and that an
// export to another format leaves it. The write itself is stubbed (`/api/write`).
//
// ── What it cannot ───────────────────────────────────────────────────────────
// A real file on disk, and a real conversion (Go's tests drive every route).
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const h = await boot({
  routes: {
    '/api/listdir': { path: '/home/someone/nib', parent: '/home/someone', dirs: [], files: [] },
    // canUndo true seeds the view dirty, so Quit has something to name.
    '/api/open': () => ({ id: 'is:1', name: 'photo.pdf', path: '', canSave: false, signature: { state: 'unsigned' }, canUndo: true, canRedo: false, importedFrom: 'photo.png' }),
    '/api/export/document': () => new Response('MD', { status: 200, headers: { 'X-Nib-Export': '{"source":"tree"}' } }),
    '/api/write': () => ({ path: '/home/someone/nib/x' }),
  },
});
const { document: doc, settle } = h;
globalThis.FormData = class { #f = new Map(); append(k, v) { if (!this.#f.has(k)) this.#f.set(k, v); } get(k) { return this.#f.has(k) ? this.#f.get(k) : null; } };
const $ = (id) => doc.getElementById(id);

// Quit's confirm is answered "no", so nothing quits; what is read is the sentence.
async function quitSentence() {
  h.confirms.length = 0;
  h.setConfirmAnswer(false);
  $('quitBtn').click();
  await settle();
  h.setConfirmAnswer(true);
  return h.confirms.at(-1) || '';
}

setNextDocument({ numPages: 1, text: 'Name Amount' });
$('pathInput').value = '/tmp/nib-harness/photo.png';
$('openGo').click();
await settle();

test('an edited conversion is named as converted, with its PDF unsaved', async () => {
  assert.match(await quitSentence(), /“photo\.png” was converted and its PDF isn't saved/);
});

test('exporting it to another format does not save its PDF', async () => {
  $('exportDocBtn').click();
  await settle();
  assert.equal($('saveAsModal').hidden, false, 'stimulus: the document export opened no save dialog');
  $('saveAsGo').click();
  await settle();
  assert.equal($('saveAsModal').hidden, true, 'stimulus: the export did not write');
  assert.match(await quitSentence(), /was converted and its PDF isn't saved/, 'an export to Markdown cleared the mark');
});

test('Save As of its PDF clears the mark', async () => {
  $('saveAsBtn').click();
  await settle();
  assert.equal($('saveAsModal').hidden, false, 'stimulus: Save As opened no dialog');
  $('saveAsGo').click();
  await settle();
  assert.equal($('saveAsModal').hidden, true, 'stimulus: the Save As did not write');
  const s = await quitSentence();
  assert.doesNotMatch(s, /was converted/, 'the mark outlived the save of its PDF');
  assert.match(s, /photo\.pdf|unsaved changes/, 'stimulus: Quit stopped naming the edited view at all');
});
