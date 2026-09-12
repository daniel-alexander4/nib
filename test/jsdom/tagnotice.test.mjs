// The lost-accessibility-structure notice (ADR-031, law 2's `dropped-with-notice`).
//
// ── What only this tier can see ──────────────────────────────────────────────
// The Go tests prove the commit door NOTICES — that the flag is set exactly when a claim disappears,
// that it is sticky across later edits, and that it reaches the wire. They cannot see whether
// anything is drawn, whether it survives the next edit on screen, or whether dismissing it works.
//
// ── Why a persistent banner and not a toast, asserted rather than commented ──
// `index.html`'s own note about the other persistent notice says it: *"`toast` cannot carry them: it
// clears itself after 2500 ms."* A user who lost their document's accessibility structure has to
// still be able to see that at the moment they save, which may be ten minutes later. So the test
// that matters most here is the one where a SECOND edit arrives and the banner is still up.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const DOC = '/tmp/nib-harness/tagged.pdf';
let dropped = false;

const h = await boot({
  routes: {
    '/api/open': () => ({
      name: 'tagged.pdf', path: DOC, canSave: true,
      signature: { state: 'unsigned' }, canUndo: false, canRedo: false,
      taggingDropped: dropped,
    }),
    // The route a page rotation actually takes — driven through the real button below, because a
    // bare fetch would not re-render anything and would prove nothing about the banner.
    '/api/pages': () => ({
      name: 'tagged.pdf', path: DOC, canSave: true,
      signature: { state: 'unsigned' }, canUndo: true, canRedo: false,
      taggingDropped: dropped,
    }),
  },
});
const { document: doc, settle } = h;

const notice = () => doc.getElementById('tagNotice');
const shown = () => { const n = notice(); return !!n && !n.hidden; };

// anEdit drives a real page rotation through the real control, so what is asserted is what the app
// does on an ordinary edit rather than what a hand-made fetch does.
async function anEdit() {
  doc.getElementById('rotateLeftBtn').click();
  await settle();
}

async function openDocument() {
  setNextDocument({ numPages: 2, outline: null });
  doc.getElementById('pathInput').value = DOC;
  doc.getElementById('openGo').click();
  await settle();
}

test('a document that never lost tagging shows nothing', async () => {
  dropped = false;
  await openDocument();
  assert.equal(shown(), false,
    'the lost-accessibility banner is up on a document that never had tagging — it would fire on '
    + 'nearly every edit in the product and teach the user to ignore the one time it matters');
});

test('losing the structure raises a persistent banner, not a toast', async () => {
  dropped = true;
  await openDocument();
  assert.equal(shown(), true,
    'nothing is shown after an edit removed the document accessibility structure. The file is '
    + 'honest — it no longer claims to be tagged — and the person who could re-make it is told '
    + 'nothing, which is the whole of law 2\'s `with-notice` half');
  const n = notice();
  assert.equal(n.getAttribute('role'), 'status',
    'the banner carries no role, so a screen-reader user — the person this notice is most for — '
    + 'is not told at all');
  assert.equal(n.getAttribute('aria-live'), 'polite',
    'the banner is not a live region, or is assertive; assertive is spent on the signature-loss '
    + 'notice and interrupting for this would devalue that one');
  const text = doc.getElementById('tagNoticeText').textContent;
  assert.match(text, /screen reader/i,
    `the banner says ${JSON.stringify(text)} — "tagging" alone means nothing to the small-practice `
    + 'user this product is for, so it has to name what was lost and what it was for');
  assert.match(text, /re-make|source/i, 'the banner does not say the one thing the user can do about it');
});

test('it survives the next edit, which is the whole reason it is not a toast', async () => {
  // A toast would be gone 2.5 seconds later; the tagging is still gone ten minutes later, and the
  // moment this has to be on screen is the SAVE. The server keeps the flag sticky per document —
  // this asserts the client does not drop it on the next response.
  assert.equal(shown(), true, 'setup: the banner is not up, so surviving an edit proves nothing');
  await anEdit();
  assert.equal(shown(), true,
    'an ordinary later edit took the banner down. The tagging is still gone, and the user stops '
    + 'being told before the moment they save');
});

test('dismissing it is remembered, so the user is not nagged', async () => {
  assert.equal(shown(), true, 'setup: nothing to dismiss');
  doc.getElementById('tagNoticeDismiss').click();
  await settle();
  assert.equal(shown(), false, 'Dismiss did not take the banner down');
  await anEdit();
  assert.equal(shown(), false,
    'the banner came back after a dismissal — a user who read it and decided they do not care is '
    + 'nagged for the rest of the session');
});
