// Insert before, insert after — /pending 483.
//
// ── What only this tier can see ──────────────────────────────────────────────
// The Go tests prove each door puts the page on the side it is sent. They cannot see that a button
// sends the side its label names, so this file clicks each real control and reads the form that reached
// /api/pages. A button wired to the wrong side passes every Go test and inserts on the wrong side.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const DOC = '/tmp/nib-harness/insert.pdf';
const posted = [];

const h = await boot({
  routes: {
    '/api/open': () => ({
      name: 'insert.pdf', path: DOC, canSave: true,
      signature: { state: 'unsigned' }, canUndo: false, canRedo: false,
    }),
    '/api/pages': (opts) => {
      const form = opts.body;
      posted.push({ op: form.get('op'), page: form.get('page'), side: form.get('side') });
      return {
        name: 'insert.pdf', path: DOC, canSave: true,
        signature: { state: 'unsigned' }, canUndo: true, canRedo: false,
      };
    },
  },
});
const { document: doc, window: win, settle } = h;

setNextDocument({ numPages: 3, outline: null });
doc.getElementById('pathInput').value = DOC;
doc.getElementById('openGo').click();
await settle();

async function lastPost(click) {
  posted.length = 0;
  await click();
  await settle();
  assert.equal(posted.length, 1, `one /api/pages request expected, saw ${posted.length}`);
  return posted[0];
}

function choosePdf(inputId) {
  const input = doc.getElementById(inputId);
  const file = new win.File(['%PDF-1.4'], 'other.pdf', { type: 'application/pdf' });
  Object.defineProperty(input, 'files', { value: [file], configurable: true });
  input.dispatchEvent(new win.Event('change'));
}

test('the blank-page buttons send the side each one names', async () => {
  const before = await lastPost(() => doc.getElementById('insertBlankBeforeBtn').click());
  assert.deepEqual([before.op, before.side], ['insertblank', 'before'], 'Blank page before did not send side=before');
  const after = await lastPost(() => doc.getElementById('insertBlankBtn').click());
  assert.deepEqual([after.op, after.side], ['insertblank', 'after'], 'Blank page after did not send side=after');
});

test('the insert-PDF buttons send the side each one names, through the one file input', async () => {
  const before = await lastPost(() => { doc.getElementById('insertPdfBtn').click(); choosePdf('insertPdfInput'); });
  assert.deepEqual([before.op, before.side], ['insertpdf', 'before'], 'Insert PDF before… did not send side=before');
  const after = await lastPost(() => { doc.getElementById('insertPdfAfterBtn').click(); choosePdf('insertPdfInput'); });
  assert.deepEqual([after.op, after.side], ['insertpdf', 'after'], 'Insert PDF after… did not send side=after');
  // The side is the button's, not left over from the previous click.
  const again = await lastPost(() => { doc.getElementById('insertPdfBtn').click(); choosePdf('insertPdfInput'); });
  assert.equal(again.side, 'before', 'Insert PDF before… after an Insert PDF after… still sent the old side');
});
