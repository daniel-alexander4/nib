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
// The document's revision as the server would report it (ADR-146), and whether the next page
// operation is refused as built on an older one.
let serverRev = 0;
let refuseNext = false;
const metaNow = () => ({
  id: 'test-epoch:1', name: 'insert.pdf', path: DOC, canSave: true,
  signature: { state: 'unsigned' }, canUndo: serverRev > 0, canRedo: false, rev: serverRev || undefined,
});

const h = await boot({
  routes: {
    '/api/open': () => ({
      id: 'test-epoch:1', name: 'insert.pdf', path: DOC, canSave: true,
      signature: { state: 'unsigned' }, canUndo: false, canRedo: false,
    }),
    '/api/pages': (opts) => {
      const form = opts.body;
      posted.push({ op: form.get('op'), page: form.get('page'), side: form.get('side'), baseRev: form.get('baseRev') });
      if (refuseNext) {
        refuseNext = false;
        return new Response(JSON.stringify({ error: 'this document changed while the operation was running' }),
          { status: 409, headers: { 'Content-Type': 'application/json' } });
      }
      serverRev++;
      return metaNow();
    },
    '/api/save': () => { serverRev++; return metaNow(); },
    '/api/docs': () => ({ docs: [metaNow()], activeId: 'test-epoch:1' }),
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

// ── The revision a page operation names (ADR-146, /pending 792) ──────────────
// The Go tests prove the server refuses a stale `baseRev`. They cannot see that the window sends one,
// that it is the revision of the bytes the view HOLDS, or that a refused view can ever succeed again.
test('a page operation names the revision of the bytes its view holds', async () => {
  const first = await lastPost(() => doc.getElementById('insertBlankBtn').click());
  const held = serverRev - 1; // what the view held when it posted; the answer moved the server on
  assert.equal(first.baseRev, String(held), 'the page operation did not send the revision its view loaded');
  const second = await lastPost(() => doc.getElementById('insertBlankBtn').click());
  assert.equal(second.baseRev, String(held + 1), 'the next operation did not name the revision the last answer brought');
});

test('a save leaves the view at the saved revision, so the next page operation is not refused', async () => {
  doc.getElementById('saveBtn').click();
  await settle();
  const after = await lastPost(() => doc.getElementById('insertBlankBtn').click());
  assert.equal(after.baseRev, String(serverRev - 1), 'after a save the view still names the revision before it');
});

test('a refused operation reloads a view the document has moved past, and the next one goes through', async () => {
  serverRev += 3; // another window edited the document
  refuseNext = true;
  const refused = await lastPost(() => doc.getElementById('insertBlankBtn').click());
  assert.notEqual(refused.baseRev, String(serverRev), 'the fixture is wrong: the view was not stale');
  const toastText = doc.getElementById('toast').textContent;
  assert.match(toastText, /reloaded/, `the refusal did not say the document was reloaded: ${toastText}`);
  const again = await lastPost(() => doc.getElementById('insertBlankBtn').click());
  assert.equal(again.baseRev, String(serverRev - 1), 'the reloaded view still names the old revision — "run it again" is refused for ever');
});
