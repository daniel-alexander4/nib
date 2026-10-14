// "Remove form entries on no page" in the hidden-content dialog (/pending 888).
//
// The Go tests prove the removal — what leaves the form, what stays, the 422 on a signed document. What only this
// tier can see is the press: that the button asks before it sends, that "no" sends nothing, that the request names
// the method and the document open at the press, and that on a signed document nothing is sent at all and the
// sentence says why.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const DOC = '/tmp/nib-harness/cut.pdf';
let state = 'unsigned';
const meta = () => ({
  id: 'test-epoch:1', name: 'cut.pdf', path: DOC, canSave: true,
  signature: { state }, canUndo: true, canRedo: false,
});

const h = await boot({
  routes: {
    '/api/open': () => ({ ...meta(), canUndo: false }),
    '/api/sanitize': () => ({ ...meta(), ok: true, residual: { findings: [] } }),
  },
});
const { document: doc, settle } = h;
const asked = h.confirms; // the boot owns confirm(): every question the page asks, in order
// What was sent to the removal route, read from the boot's own record of every request.
const sent = () => h.calls.filter((c) => c.url.split('?')[0].endsWith('/api/sanitize'));

async function openDocument() {
  setNextDocument({ numPages: 1, outline: null });
  doc.getElementById('pathInput').value = DOC;
  doc.getElementById('openGo').click();
  await settle();
}
const press = async () => { doc.getElementById('scanFieldsBtn').click(); await settle(); };

test('the button asks first, and "no" sends nothing', async () => {
  await openDocument();
  h.setConfirmAnswer(false);
  asked.length = 0;
  await press();
  assert.equal(asked.length, 1, 'the removal was not asked about');
  assert.match(asked[0], /on none of this document’s pages/);
  assert.match(asked[0], /calculation/, 'the question does not say what a form may lose in another reader');
  assert.deepEqual(sent(), [], 'a removal the user declined was sent');
});

test('"yes" sends the removal for the document that is open, by its method', async () => {
  h.setConfirmAnswer(true);
  await press();
  assert.equal(sent().length, 1);
  assert.match(sent()[0].url, /\/api\/sanitize\?method=fields$/);
  assert.equal(sent()[0].headers['X-Nib-Doc'], 'test-epoch:1', 'the request does not name the document open at the press (ADR-001)');
});

test('on a signed document nothing is asked and nothing is sent', async () => {
  state = 'valid';
  await openDocument();
  asked.length = 0;
  const before = sent().length;
  await press();
  assert.deepEqual([asked.length, sent().length - before], [0, 0]);
  assert.match(doc.getElementById('toast')?.textContent || doc.body.textContent, /signed, and taking form entries out of it would break the signature/);
});
