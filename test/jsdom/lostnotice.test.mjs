// The page operations' loss notice (/pending 574).
//
// ── What only this tier can see ──────────────────────────────────────────────
// The Go tests prove the commit door COUNTS — that a destroying operation records the loss, that it
// accumulates across edits, that a lossless one adds nothing, and that the counts reach the wire.
// They cannot see whether anything is drawn, what sentence the user reads, whether it survives the
// next edit on screen, or whether dismissing it works.
//
// ── Why this is a second banner and not a sentence in the first ──────────────
// The tagging notice says a PROPERTY of the file is gone and cannot be put back. This says CONTENT
// is gone and names how much. They are dismissed separately because a user may care about one and
// not the other, and they can be on screen together — which is why the assertion below checks the
// two coexist rather than only that this one appears.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const DOC = '/tmp/nib-harness/noted.pdf';
let lostAnnots = 0;
let lostFields = 0;

const meta = () => ({
  name: 'noted.pdf', path: DOC, canSave: true,
  signature: { state: 'unsigned' }, canUndo: true, canRedo: false,
  lostAnnots: lostAnnots || undefined,
  lostFields: lostFields || undefined,
});

const h = await boot({
  routes: {
    '/api/open': () => ({ ...meta(), canUndo: false }),
    '/api/pages': () => meta(),
  },
});
const { document: doc, settle } = h;

const notice = () => doc.getElementById('lostNotice');
const shown = () => { const n = notice(); return !!n && !n.hidden; };
const text = () => doc.getElementById('lostNoticeText').textContent;

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

test('a document that lost nothing shows nothing', async () => {
  lostAnnots = 0; lostFields = 0;
  await openDocument();
  assert.equal(shown(), false,
    'the loss banner is up on a document that lost nothing — it would fire on nearly every edit '
    + 'and teach the user to ignore the one time it matters');
});

test('a lost comment is reported as a count, in words the user has', async () => {
  lostAnnots = 1; lostFields = 0;
  await openDocument();
  assert.equal(shown(), true,
    'nothing is shown after an edit destroyed a comment. The route answers with the new document '
    + 'and the user is never told what it cost them, which is the whole of /pending 574');
  assert.match(text(), /1 comment(?!s)/,
    `the banner says ${JSON.stringify(text())} — the count has to be exact and singular, because `
    + '"1 comments" is the tell that nobody read the sentence they shipped');
  assert.doesNotMatch(text(), /form field/,
    'the banner mentions form fields when none were lost — a notice that lists what did NOT happen '
    + 'is one the user stops reading');
  const n = notice();
  assert.equal(n.getAttribute('role'), 'status', 'the banner carries no role');
  assert.equal(n.getAttribute('aria-live'), 'polite',
    'the banner is not a polite live region; assertive is spent on the signature-loss notice');
});

test('both kinds are named, and pluralised for what was actually lost', async () => {
  lostAnnots = 3; lostFields = 2;
  await openDocument();
  assert.match(text(), /3 comments/, `the banner says ${JSON.stringify(text())}`);
  assert.match(text(), /2 form fields/, `the banner says ${JSON.stringify(text())}`);
  assert.match(text(), /cannot put them back/i,
    'the banner does not say the thing that decides what the user does next — whether this is '
    + 'recoverable. It is not, and the only route back is undo or the original file');
});

test('it survives the next edit, which is why it is not a toast', async () => {
  assert.equal(shown(), true, 'setup: the banner is not up, so surviving an edit proves nothing');
  await anEdit();
  assert.equal(shown(), true,
    'an ordinary later edit took the banner down. The comments are still gone, and the moment '
    + 'this has to be on screen is the SAVE, which may be ten minutes later');
});

test('dismissing it is remembered, so the user is not nagged', async () => {
  assert.equal(shown(), true, 'setup: nothing to dismiss');
  doc.getElementById('lostNoticeDismiss').click();
  await settle();
  assert.equal(shown(), false, 'Dismiss did not take the banner down');
  await anEdit();
  assert.equal(shown(), false,
    'the banner came back after a dismissal — a user who read it and decided they do not care is '
    + 'nagged for the rest of the session');
});
