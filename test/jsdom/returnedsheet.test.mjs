// PLAN-returned-document P03.S01: the command for a document that came back, and the sheet it opens.
//
// The sheet stands in place of the viewer (D12), about ONE document captured at open (ADR-001). These drive the real
// app: both entry points, focus in and back to whichever control opened it, Escape, a tab change closing it, and the
// ceremony sheet and this one never sharing the screen.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

let opened = 0;
const h = await boot({
  routes: {
    '/api/open': () => ({
      id: `test-epoch:${++opened}`, name: `doc${opened}.pdf`, path: `/tmp/nib-harness/doc${opened}.pdf`, canSave: true,
      signature: { state: 'valid', signers: [{ name: 'Alice', valid: true, fingerprint: 'a'.repeat(64) }] },
      canUndo: false, canRedo: false,
    }),
    // Shaped like handleClose: docResponse(nil), the empty document.
    '/api/close': { name: '', path: '', canSave: false, signature: { state: '' }, canUndo: false, canRedo: false },
  },
});
const { document: doc, settle } = h;
const $ = (id) => doc.getElementById(id);

async function openDoc() {
  setNextDocument({ numPages: 1 });
  $('pathInput').value = '/tmp/nib-harness/doc.pdf';
  $('openGo').click();
  await settle();
}
const sheetShown = () => !$('returnedSheet').hidden && $('viewerWrap').hidden;
const sheetGone = () => $('returnedSheet').hidden && !$('viewerWrap').hidden;

test('the command waits for a document (DOC_REQUIRED), and so does its twin', () => {
  assert.equal($('returnedBtn').disabled, true, 'the command is live with nothing open — the sheet would have no subject');
  assert.equal(doc.querySelector('[data-forward="returnedBtn"]').disabled, true,
    'the twin looks live while the command it forwards to is dead, so a click does nothing');
});

test('the sheet is a named region', () => {
  assert.equal($('returnedSheet').getAttribute('role'), 'region',
    'aria-labelledby on a div with no role names nothing a screen reader announces');
  assert.equal($('returnedSheet').getAttribute('aria-labelledby'), 'returnedHeading');
});

test('the command lives in Sign & Timestamp, with ONE twin in Send & Receive that forwards to it', async () => {
  const card = $('returnedBtn').closest('.tbgroup');
  assert.equal(card && card.dataset.label, 'Sign & Timestamp');
  const twins = doc.querySelectorAll('[data-forward="returnedBtn"]');
  assert.equal(twins.length, 1, 'want exactly one alias, not a second declaration');
  assert.equal(twins[0].closest('.tbgroup').dataset.label, 'Send & Receive');
  await openDoc();
  assert.equal($('returnedBtn').disabled, false, 'stimulus: a document is open, so the command is live');
});

test('the twin opens the sheet in place of the viewer, focus moves in, and "Back" returns it to the twin', async () => {
  const twin = doc.querySelector('[data-forward="returnedBtn"]');
  twin.focus();
  twin.click();
  await settle();
  assert.ok(sheetShown(), 'the sheet did not stand in place of the viewer');
  assert.equal(doc.activeElement, $('returnedHeading'), 'focus did not move into the sheet');
  assert.match($('returnedDoc').textContent, /doc\d\.pdf/, 'the sheet does not name its document');
  $('returnedClose').click();
  assert.ok(sheetGone(), '"Back to the document" did not put the viewer back');
  assert.equal(doc.activeElement, twin, 'focus did not return to the control that opened the sheet');
});

test('Escape leaves the sheet and focus returns to the command', async () => {
  $('returnedBtn').focus();
  $('returnedBtn').click();
  assert.ok(sheetShown());
  $('returnedHeading').dispatchEvent(new doc.defaultView.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  assert.ok(sheetGone(), 'Escape did not leave the sheet');
  assert.equal(doc.activeElement, $('returnedBtn'));
});

test('opening another document closes the sheet rather than describe the wrong one (ADR-001)', async () => {
  $('returnedBtn').click();
  assert.ok(sheetShown(), 'stimulus: the sheet is open on the first document');
  await openDoc();
  assert.ok(sheetGone(), 'the sheet stayed open over a different document');
});

test('the ceremony sheet and this one never share the screen', async () => {
  $('returnedBtn').click();
  assert.ok(sheetShown());
  $('ceremonyConveneBtn').click();
  await settle();
  assert.equal($('ceremonySheet').hidden, false, 'stimulus: the ceremony sheet did not open');
  assert.equal($('returnedSheet').hidden, true, 'the returned sheet stayed under the ceremony sheet');
  $('returnedBtn').click();
  assert.equal($('ceremonySheet').hidden, true, 'the ceremony sheet stayed over the returned sheet');
  assert.ok(sheetShown());
  $('returnedClose').click();
});

test('closing every document closes the sheet with them', async () => {
  doc.defaultView.confirm = () => true;
  $('returnedBtn').click();
  assert.ok(sheetShown(), 'stimulus: the sheet is open');
  $('closeAllBtn').click();
  await settle();
  assert.equal($('returnedSheet').hidden, true, 'the sheet outlived the documents it was about');
});

test('Escape with focus on <body> still leaves, and focus returns to the twin that opened it', async () => {
  await openDoc();
  const twin = doc.querySelector('[data-forward="returnedBtn"]');
  twin.focus();
  twin.click();
  assert.ok(sheetShown(), 'stimulus: the sheet is open');
  doc.activeElement.blur(); // a click on the sheet's plain text leaves focus on <body>
  assert.equal(doc.activeElement, doc.body, 'stimulus: focus is outside the sheet');
  doc.dispatchEvent(new doc.defaultView.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  assert.ok(sheetGone(), 'Escape did nothing once focus left the sheet');
  assert.equal(doc.activeElement, twin, 'focus did not return to the twin — the opener was not captured');
});

test('a mode change takes the sheet down: the new mode acts on the viewer, and its opener is now hidden', async () => {
  $('returnedBtn').click();
  assert.ok(sheetShown(), 'stimulus: the sheet is open');
  doc.querySelector('.modetab[data-tab="markup"]').click();
  await settle();
  assert.ok(sheetGone(), 'the sheet stayed over the viewer after a mode change');
  doc.querySelector('.modetab[data-tab="secure"]').click();
  await settle();
});

test('arming a tool takes the sheet down, so the tool never acts on a hidden viewer', async () => {
  $('returnedBtn').click();
  assert.ok(sheetShown(), 'stimulus: the sheet is open');
  $('redactBtn').click();
  await settle();
  assert.ok(sheetGone(), 'a tool armed under the sheet, acting on the hidden viewer');
  $('redactBtn').click(); // put it down again
  await settle();
});
