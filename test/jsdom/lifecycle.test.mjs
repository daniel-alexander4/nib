// P01's client acceptance, converted from a live drive into assertions.
//
// Every clause P01 owed on the client was verified by a human driving a browser.
// That is real evidence and it expires the moment the session ends — nothing
// re-checks it on the next commit. This file is the conversion. Each test names
// the P01 acceptance line it discharges, so the mapping can be checked rather
// than assumed.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument, lastDocument } from './stub-pdfjs.mjs';

const DOC = '/tmp/nib-harness/doc.pdf';
const docResponse = (over = {}) => ({
  name: 'doc.pdf', path: DOC, canSave: true,
  signature: { state: 'unsigned' }, canUndo: false, canRedo: false, ...over,
});

let openResponse = docResponse();
const h = await boot({
  routes: {
    '/api/open': () => openResponse,
    '/api/close': { name: '', path: '', canSave: false, signature: { state: '' }, canUndo: false, canRedo: false },
  },
});
const { document, settle } = h;

// openDocument drives the real Open dialog rather than calling into app.js — the
// module's functions are not exported, and going through the UI is the path a
// user takes anyway.
async function openDocument({ numPages = 3, outline = null } = {}) {
  setNextDocument({ numPages, outline });
  document.getElementById('pathInput').value = DOC;
  document.getElementById('openGo').click();
  await settle();
}

async function closeDocument() {
  document.getElementById('closeBtn').click();
  await settle();
}

const armRedact = () => document.getElementById('redactBtn').click();
const redactLit = () => document.getElementById('redactBtn').classList.contains('active');
// Re-derived for P05.S03, not loosened. The crosshair used to be written on
// #viewerContainer; that element is now one per open document and built in JS, so the
// cursor moved to the stable #viewerWrap — which is also where the drawing tools now
// listen. Reading it off the wrap keeps this asserting the same user-visible fact.
// Over the page itself this is identical: the container is inset:0 over the wrap, and
// `.textLayer :is(span, br) { cursor: text }` in the vendored sheet overrode the
// inherited crosshair before the move and still does. It is NOT identical everywhere —
// #signBanner is a child of the wrap rather than of the container, so it now inherits the
// crosshair on its padding and label where it previously showed an arrow. Cosmetic, and
// invisible at this tier (jsdom computes no inherited cursor); recorded so the next
// reader does not take "identical" as a claim about the whole wrap.
const cursor = () => document.getElementById('viewerWrap').style.cursor;

test('opening a document reaches the app (the stimulus for everything below)', async () => {
  await openDocument();
  // Asserted before any teardown claim: if the open path never ran, every
  // "cleared after close" assertion below would pass against a document that
  // never existed.
  assert.equal(document.getElementById('viewerWrap').className, 'has-doc');
  assert.equal(document.querySelector('.pageCount').textContent, '/ 3');
});

// P01.S03 acceptance: "Arming the redact tool and then opening a *different*
// document leaves no lit button and no crosshair — the pre-existing bug,
// asserted directly."
test('the shared reset clears an armed mode on the OPEN path', async () => {
  armRedact();
  assert.equal(redactLit(), true, 'setup: redact must actually arm, or the clear below proves nothing');
  assert.equal(cursor(), 'crosshair');

  await openDocument({ numPages: 5 });

  assert.equal(document.querySelector('.pageCount').textContent, '/ 5', 'a different document must actually have opened');
  assert.equal(redactLit(), false, 'opening a document must clear an armed redact mode');
  assert.equal(cursor(), '', 'opening a document must clear the crosshair');
});

// Same clause, other half: the reset is shared, so close must agree with open.
test('the shared reset clears an armed mode on the CLOSE path', async () => {
  await openDocument();
  armRedact();
  assert.equal(redactLit(), true, 'setup: redact must actually arm');

  await closeDocument();

  assert.equal(redactLit(), false);
  assert.equal(cursor(), '');
});

// P01 exit criterion: "/api/pdf 404s and the UI shows 'Open a PDF to begin.'
// after Close" (client half), plus P01.S03's "The empty state is byte-identical
// to launch."
test('closeDocument restores the launch chrome', async () => {
  await openDocument();
  // The transition, not the end state: every value below is ALSO true at launch,
  // so each is asserted in its opened form first.
  assert.equal(document.getElementById('viewerWrap').className, 'has-doc');
  assert.equal(document.getElementById('saveBtn').disabled, false);
  assert.ok(document.getElementById('saveBtn').title.includes(DOC),
    'the Save title should carry the document path while one is open');
  assert.ok(document.getElementById('outline').children.length > 0,
    'the outline sidebar should have content while a document is open');

  await closeDocument();

  assert.equal(document.getElementById('viewerWrap').className, '');
  assert.equal(document.getElementById('empty').textContent, 'Open a PDF to begin.');
  assert.equal(document.getElementById('saveBtn').disabled, true);
  // The literal markup string — app.js overwrites this on open, so a paraphrase
  // would not catch a teardown that forgot to put it back.
  assert.equal(document.getElementById('saveBtn').title, 'Save (overwrites the original)');
  assert.equal(document.getElementById('sigBadge').textContent, 'no document');
  assert.equal(document.getElementById('sigBadge').className, 'badge badge-none');
  assert.equal(document.querySelector('.pageCount').textContent, '/ 0');
  assert.equal(document.querySelector('.pageNum').value, '1');
  assert.equal(document.getElementById('outline').children.length, 0);
  assert.equal(document.getElementById('closeBtn').disabled, true);
  // NOTE: the thumbnail grid is deliberately NOT asserted here. jsdom has no
  // canvas, so buildThumbnails always fails into its caller's .catch and the grid
  // is empty at this tier whether or not the teardown clears it — a green that
  // would be structural, not earned. Tier 3 (P02.S04) owns that half.
});

// P01.S04 acceptance, and the reason the helper is named hasEditsSinceOpen: each
// signal is driven ON ITS OWN, so a signal that never fires cannot hide behind
// the other three.
//
// Two of the four are reachable at this tier and two are not, and that is stated
// rather than quietly skipped:
//   * docMeta.canUndo         — driven below (the server reports it)
//   * annotationStorage.size  — driven below (via the document stub)
//   * overlayFields           — needs a placed overlay, which needs layout and a
//     canvas to detect against. NOT EXERCISED here; tier 3 (P02.S04) owns it.
//   * overlayHistory          — same, for the same reason.
test('the prompt fires from the server-history signal alone', async (t) => {
  // Restored in an after-hook, not at the end of the body: if an assertion below
  // throws, a body-level restore never runs and every later test inherits a
  // document that reports undo history — one failure becoming three.
  t.after(() => { openResponse = docResponse(); });
  openResponse = docResponse({ canUndo: true }); // what the server returns after a page op
  await openDocument();
  h.confirms.length = 0;

  document.getElementById('closeBtn').click();
  await settle();

  assert.equal(h.confirms.length, 1,
    'a document with server-side undo history must prompt before closing');
  // The wording is part of the contract: the three signals mean "edited since
  // open", not "unsaved", so the prompt must not claim more than that.
  assert.match(h.confirms.at(-1), /since the last save/);
});

test('the prompt fires from the pdf.js annotation-storage signal alone', async () => {
  await openDocument();               // clean: canUndo false, no overlays
  h.confirms.length = 0;
  assert.ok(lastDocument, 'setup: the stub must have produced a document');
  assert.equal(lastDocument.annotationStorage.size, 0, 'setup: storage starts empty');

  lastDocument.annotationStorage.set('field-1', { value: 'typed' }); // a form fill

  document.getElementById('closeBtn').click();
  await settle();
  assert.equal(h.confirms.length, 1,
    'a document with pdf.js annotation edits must prompt before closing');
});

test('a clean document closes without prompting', async () => {
  await openDocument();
  h.confirms.length = 0;
  await closeDocument();
  // The dangerous zero: "no prompt" is also what a broken signal read produces.
  // It is only evidence because the two tests above already showed it firing.
  assert.equal(h.confirms.length, 0, 'a freshly opened document has no edits and must not prompt');
  assert.equal(document.getElementById('viewerWrap').className, '');
});
