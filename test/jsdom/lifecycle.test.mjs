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
    // A save answers with the document's fresh metadata, and canUndo stays TRUE — the
    // server's handleSave deliberately leaves the undo ring alone, because undo after a
    // save is a feature. That is exactly the response that used to leave the app
    // believing the document was still edited, so the route models it rather than
    // returning a convenient clean one.
    '/api/save': () => ({ ...openResponse, canUndo: true }),
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

// Close ALL, which is what every test in this file means: each asserts the LAUNCH
// state, and the launch state is what you get when nothing is open.
//
// Before P06.S01 an Open replaced, so exactly one document was open however many tests
// had run and #closeBtn was close-all by definition. Views now accumulate across the
// tests in this file, so the control that returns the app to launch is #closeAllBtn
// once it is showing. Pointed at explicitly rather than left to #closeBtn, whose
// meaning now depends on how many documents happen to be open — which would make these
// tests pass or fail on their position in the file.
async function closeDocument() {
  const all = document.getElementById('closeAllBtn');
  (all && !all.hidden ? all : document.getElementById('closeBtn')).click();
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
  // Counted as CONTENT, not children. P05.S05 nested a per-view `.outlinelist` inside
  // `#outline`, so `children.length` is now the number of open views and would read > 0
  // with no document at all — and it was already weak before that: measured with no
  // outline present, `#outline` had 2 children (the Edit button and the empty-state div),
  // so this passed on chrome alone. `.outline-edit` is the honest marker that a build ran.
  assert.ok(document.querySelectorAll('#outline .outline-edit').length > 0,
    'the outline sidebar should have been built while a document is open');
  // The thumbnail grid in its OPENED form, per this test's own convention four lines up.
  // Exactly one lands at this tier — buildThumbnails appends the wrapper before the render
  // that jsdom rejects — and asserting it here is what keeps the post-close zero earned. If
  // the append ever moved after the render, the closing assertion would revert to a
  // structural green in silence, which is the failure the corrected comment below exists to
  // prevent and which this line is what actually prevents.
  assert.equal(document.querySelectorAll('.thumbgrid .thumbwrap').length, 1,
    'the grid must POPULATE before its emptiness means anything');

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
  // "No view has any outline content" — Close is close-all since P05.S04, and since
  // P05.S05 each view's list is a wrapper INSIDE #outline, so the wrappers survive an
  // emptied close by design. What must be zero is what they contain.
  assert.equal(document.querySelectorAll('#outline .outline-edit, #outline a').length, 0);
  assert.equal(document.getElementById('closeBtn').disabled, true);
  // The thumbnail grid IS asserted here now, and the reason the old note gave for not
  // asserting it was measured false. It said jsdom's missing canvas leaves the grid empty
  // "whether or not the teardown clears it". Measured through this harness: the grid holds
  // exactly ONE `.thumbwrap`, because buildThumbnails appends the wrapper BEFORE the
  // render that rejects. So an emptiness assertion here is earned rather than structural —
  // it distinguishes a teardown that clears from one that does not.
  assert.equal(document.querySelectorAll('.thumbgrid .thumbwrap').length, 0,
    'no view may keep a thumbnail after a close-all');
});

// P01.S04 acceptance: each signal is driven ON ITS OWN, so a signal that never fires
// cannot hide behind the other three. (The helper they were written against was named
// hasEditsSinceOpen, because "since it was opened" was all four of them could support;
// v1.108.7 replaced them with a real dirty flag and it is hasUnsavedWork now. The tests
// stand unchanged apart from the prompt wording — each still drives one input to that
// flag on its own, which is what made them worth keeping.)
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
  // The wording is part of the contract, and the contract CHANGED in v1.108.7. It read
  // "any edits made since the last save" because the signals could only support "since
  // it was opened" — a save cleared none of them, so claiming "unsaved" would have been
  // a lie the app could not back. With a real dirty flag it can, and the hedge would now
  // be the inaccuracy. Asserted rather than dropped: this line is what stops the wording
  // drifting back to a claim the signals do not support.
  assert.match(h.confirms.at(-1), /unsaved/);
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

// The whole of the item this file's four signal tests were carried against: the confirm
// fired on every close after every save, because not one of the four signals is cleared
// by saving. Driven with the HARDEST of them — a document the server says still has undo
// history, which is the signal a save provably does not clear — so a pass cannot come
// from the save having quietly made the document look untouched.
test('a save clears the prompt, even though the server still reports undo history', async (t) => {
  t.after(() => { openResponse = docResponse(); });
  openResponse = docResponse({ canUndo: true, canSave: true, path: DOC });
  await openDocument();

  // The stimulus assertion. Without it, a "no prompt" at the end is equally consistent
  // with the save having worked and with the document never having been dirty at all.
  h.confirms.length = 0;
  document.getElementById('closeBtn').click();
  await settle();
  assert.equal(h.confirms.length, 1,
    'setup: this document does not prompt BEFORE the save, so the assertion after it proves nothing');
  await settle();

  await openDocument({ numPages: 3 });   // the cancelled close left it open; re-open clean-slate
  openResponse = docResponse({ canUndo: true, canSave: true, path: DOC });
  document.getElementById('saveBtn').click();
  await settle();

  h.confirms.length = 0;
  document.getElementById('closeBtn').click();
  await settle();
  assert.equal(h.confirms.length, 0,
    'closing straight after a successful save still prompted. Nothing has changed since the save, so the prompt is claiming work is at risk when none is — and a prompt that always fires is one the user learns to dismiss.');
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
