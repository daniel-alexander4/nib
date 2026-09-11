// Read aloud (`/pending 408`).
//
// ── What this feature is NOT, asserted rather than merely commented ──────────
// It is not tagged-PDF accessibility. A screen reader needs a tag tree; this builds none, and
// `/pending 29` stays open. The entry is explicit that saying otherwise would be the over-claim
// ADR-013's language rule exists to prevent — so the test that matters most here is the one about
// a page with no text layer, where the honest answer names OCR instead of failing silently.
//
// ── Why jsdom can hold all of it ─────────────────────────────────────────────
// `speechSynthesis` does not exist in jsdom, and `app.js` reads it at call time for exactly that
// reason — a browser without it, an embedded view without it, and this harness are the same case.
// So the Web Speech API is stubbed here and what is asserted is what Nib ASKS it to say, which is
// the whole of the logic. Actual speech is the browser's and is nobody's test.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const DOC = '/tmp/nib-harness/readaloud.pdf';
const h = await boot({
  routes: {
    '/api/open': () => ({
      name: 'readaloud.pdf', path: DOC, canSave: true,
      signature: { state: 'unsigned' }, canUndo: false, canRedo: false,
    }),
  },
});
const { document: doc, window: win, settle } = h;

// The stub records what was said and how often it was cancelled, which is the whole observable.
const spoken = [];
let cancels = 0;
class FakeUtterance {
  constructor(text) { this.text = text; }
}
const fakeSpeech = {
  speak(u) { spoken.push(u.text); if (u.onend) fakeSpeech._end = u.onend; },
  cancel() { cancels += 1; },
};
win.speechSynthesis = fakeSpeech;
globalThis.speechSynthesis = fakeSpeech;
win.SpeechSynthesisUtterance = FakeUtterance;
globalThis.SpeechSynthesisUtterance = FakeUtterance;

async function openDocument(opts) {
  setNextDocument(opts);
  doc.getElementById('pathInput').value = DOC;
  doc.getElementById('openGo').click();
  await settle();
}

const btn = () => doc.getElementById('readAloudBtn');
const toastText = () => (doc.getElementById('toast') || {}).textContent || '';

test('the control exists and starts idle', () => {
  assert.ok(btn(), 'there is no Read aloud control');
  assert.equal(btn().getAttribute('aria-pressed'), 'false');
  assert.equal(btn().textContent, 'Read aloud');
});

test('a page with text is read, and only that page', async () => {
  // **Three DIFFERENT pages**, because "only that page" cannot fail against a document whose pages
  // all say the same thing: reading the whole file and reading page one produce the same utterance.
  // Measured — the whole-document mutation stayed green until this fixture differed per page.
  await openDocument({ numPages: 3, outline: null,
    text: ['the first page says this', 'the second page says that', 'the third page says something else'] });
  spoken.length = 0;
  btn().click();
  await settle();
  assert.equal(spoken.length, 1,
    `${spoken.length} utterances were queued for one page — the unit is the page on screen, and a `
    + 'document read start to finish cannot be followed or resumed');
  assert.match(spoken[0], /first page/,
    `what was read was ${JSON.stringify(spoken[0])}, which is not this page's text`);
  assert.doesNotMatch(spoken[0], /second page|third page/,
    `the utterance carried other pages too: ${JSON.stringify(spoken[0])}. A document read start to `
    + 'finish cannot be followed, cannot be resumed, and has no relationship to what is on screen');
  assert.equal(btn().getAttribute('aria-pressed'), 'true',
    'the button does not report that it is reading, so the only way to find out is to listen');
  assert.equal(btn().textContent, 'Stop reading',
    'the label still offers to start while it is already reading');
});

test('pressing again stops, through the one door', async () => {
  cancels = 0;
  btn().click();
  await settle();
  assert.ok(cancels >= 1, 'stopping did not cancel the speech queue, so the voice carries on');
  assert.equal(btn().getAttribute('aria-pressed'), 'false');
  assert.equal(btn().textContent, 'Read aloud');
});

test('turning the page stops reading', async () => {
  spoken.length = 0;
  btn().click();
  await settle();
  assert.equal(btn().getAttribute('aria-pressed'), 'true', 'setup: it is not reading, so the '
    + 'assertion below would pass on a build where turning the page does nothing');
  cancels = 0;
  // **Driven through the page-number field, which is the control a user actually uses.** The stub
  // viewer's `currentPageNumber` is a plain property and fires nothing, so the app's own handler is
  // what has to carry this — and going through the field means the test exercises the route that
  // exists rather than an event dispatched by hand into a bus.
  const field = doc.querySelector('.pageNum');
  field.value = '2';
  field.dispatchEvent(new win.Event('change'));
  await settle();
  assert.equal(btn().getAttribute('aria-pressed'), 'false',
    'the voice reads on after the page turned — it is now describing a page the user cannot see, '
    + 'which is the one thing a per-page reader must not do');
  assert.ok(cancels >= 1, 'the flag cleared but the queue was never cancelled, so it keeps talking');
});

test('a page with no text layer says so, and names OCR', async () => {
  await openDocument({ numPages: 1, outline: null, text: '' });
  spoken.length = 0;
  btn().click();
  await settle();
  assert.equal(spoken.length, 0,
    'a scanned page with no text layer queued an utterance — of what?');
  assert.equal(btn().getAttribute('aria-pressed'), 'false',
    'the button reports that it is reading a page with nothing to read');
  assert.match(toastText(), /scan|OCR/i,
    `a page with no text said ${JSON.stringify(toastText())}. Silence is indistinguishable from a `
    + 'broken feature, and the remedy — OCR it first — is the one thing the user can act on');
});
