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
let xfa = '';
let unoptimized = false;
let unreadLists = [];
let unreadWhat = '';
let refuseEdits = false;

const meta = () => ({
  id: 'test-epoch:1', name: 'noted.pdf', path: DOC, canSave: true,
  signature: { state: 'unsigned' }, canUndo: true, canRedo: false,
  lostAnnots: lostAnnots || undefined,
  lostFields: lostFields || undefined,
  xfa: xfa || undefined,
  unoptimized: unoptimized || undefined,
  unreadLists: unreadLists.length ? unreadLists : undefined,
  unreadWhat: unreadWhat || undefined,
});

// The server's refusal of a change over a Web Capture record (`httpError`, /pending 889 part 2).
const refusal = () => new Response(JSON.stringify({
  error: 'this document holds ' + unreadWhat + ', which Nib could not read in full and cannot carry through a change — so nothing was changed',
  cause: 'name-tree', removable: 'webcapture',
}), { status: 422, headers: { 'Content-Type': 'application/json' } });

const h = await boot({
  routes: {
    '/api/open': () => ({ ...meta(), canUndo: false }),
    '/api/pages': () => (refuseEdits ? refusal() : meta()),
    // The removal answers a sanitizeResponse: the document, ok, and what is left.
    '/api/sanitize': () => { unreadLists = []; unreadWhat = ''; refuseEdits = false; return { ...meta(), ok: true, residual: { findings: [] } }; },
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

// ── The XFA form notice (ADR-145) ────────────────────────────────────────────
// The Go tests prove the server SAYS which case a document is (`xfa` on its answer) and refuses what it must.
// Only this tier sees whether the user is told, in which words, and whether a dismissal of one sentence hides
// the next one. In this file because it is the third banner of the same channel — one boot per file.
const xfaShown = () => { const n = doc.getElementById('xfaNotice'); return !!n && !n.hidden; };
const xfaText = () => doc.getElementById('xfaNoticeText').textContent;

test('a document with no XFA form says nothing about one', async () => {
  lostAnnots = 0; lostFields = 0; xfa = '';
  await openDocument();
  assert.equal(xfaShown(), false, 'the XFA notice is up on a document that carries no XFA form');
});

test('a static XFA form is announced at the open, with what a change will do', async () => {
  xfa = 'static';
  await openDocument();
  assert.equal(xfaShown(), true,
    'a document carrying an XFA form opened with no notice — the first the user hears of it is never');
  assert.match(xfaText(), /does not show or fill XFA forms/, `the notice says ${JSON.stringify(xfaText())}`);
  assert.match(xfaText(), /any change you make here removes it/, `the notice says ${JSON.stringify(xfaText())}`);
  const n = doc.getElementById('xfaNotice');
  assert.equal(n.getAttribute('role'), 'status', 'the notice carries no role');
  assert.equal(n.getAttribute('aria-live'), 'polite', 'the notice is not a polite live region');
  assert.equal(n.closest('dialog'), null, 'the notice is in a dialog — it is a page-level banner');
});

test('a dismissed "a change removes it" does not hide "a change removed it"', async () => {
  doc.getElementById('xfaNoticeDismiss').click();
  await settle();
  assert.equal(xfaShown(), false, 'Dismiss did not take the notice down');
  await anEdit();
  assert.equal(xfaShown(), false, 'the same notice came back after a dismissal');
  xfa = 'removed';
  await anEdit();
  assert.equal(xfaShown(), true,
    'the edit removed the XFA form and nothing said so: dismissing the warning silenced the report');
  assert.match(xfaText(), /a change removed it/, `the notice says ${JSON.stringify(xfaText())}`);
  await anEdit();
  assert.equal(xfaShown(), true, 'a later edit took the notice down; the form is still gone');
});

test('a dynamic XFA form is announced as a placeholder Nib will not change', async () => {
  xfa = 'dynamic';
  await openDocument();
  assert.equal(xfaShown(), true, 'a dynamic XFA document opened with no notice');
  assert.match(xfaText(), /only a placeholder/, `the notice says ${JSON.stringify(xfaText())}`);
  assert.match(xfaText(), /will not change this document/, `the notice says ${JSON.stringify(xfaText())}`);
});

test('a state the window does not know shows nothing rather than an empty box', async () => {
  xfa = 'constructor';
  await openDocument();
  assert.equal(xfaShown(), false, 'an unknown xfa value drew a notice');
});

// ── A document Nib does not compact (/pending 766 part 7) ────────────────────
// The Go tests prove the server SAYS it (`unoptimized` on the answer, from the read that skipped the pass). Only
// this tier sees whether the user is told, in which words, and that it is said once and not on every answer.
const toastText = () => doc.getElementById('toast')?.textContent || '';

test('a document Nib compacts is told nothing about size', async () => {
  lostAnnots = 0; lostFields = 0; xfa = ''; unoptimized = false;
  await openDocument();
  assert.doesNotMatch(toastText(), /compact/, `an ordinary document was told: ${JSON.stringify(toastText())}`);
});

test('a document Nib cannot compact says so once, at the first answer that says it', async () => {
  unoptimized = true;
  await openDocument();
  assert.match(toastText(), /cannot compact this document/,
    `the answer says the document is not compacted and the user was told ${JSON.stringify(toastText())}`);
  assert.match(toastText(), /may be larger than it needs to be/, `it does not say what that costs: ${toastText()}`);
  assert.match(toastText(), /Nothing in it is lost/, `it does not say nothing is lost: ${toastText()}`);
  // Once: the next answer for the same document says it again on the wire and must not on the screen.
  doc.getElementById('toast').textContent = '';
  await anEdit();
  assert.doesNotMatch(toastText(), /compact/, 'the same document was told a second time, on its next answer');
});

// ── A list Nib could not read whole (ADR-164, /pending 889 part 2) ────────────
// The Go tests prove the server SAYS which lists (`unreadLists`, `unreadWhat`), refuses a change naming the record
// (`removable`), and removes it on `method=webcapture`. Only this tier sees whether the user is told at the open,
// whether the removal is offered only where it can be made, and whether a refusal brings back a dismissed offer.
const listShown = () => { const n = doc.getElementById('listNotice'); return !!n && !n.hidden; };
const listText = () => doc.getElementById('listNoticeText').textContent;
const removeShown = () => !doc.getElementById('listNoticeRemove').hidden;
const WEB = 'a record of the web pages it was captured from';

test('a document with every list read says nothing about one', async () => {
  lostAnnots = 0; lostFields = 0; xfa = ''; unoptimized = false; unreadLists = []; unreadWhat = '';
  await openDocument();
  assert.equal(listShown(), false, 'the unread-list notice is up on a document Nib read whole');
});

test('a list Nib cannot carry is announced at the open, with no removal Nib cannot make', async () => {
  unreadLists = ['EmbeddedFiles']; unreadWhat = 'its attachments';
  await openDocument();
  assert.equal(listShown(), true, 'a document every change of which will be refused opened with no notice');
  assert.match(listText(), /holds its attachments, which Nib could not read in full/, `the notice says ${JSON.stringify(listText())}`);
  assert.match(listText(), /will not change this document\.$/, `the notice says ${JSON.stringify(listText())}`);
  assert.equal(removeShown(), false, 'Remove it is offered for attachments — it removes only a web capture record');
});

test('a web capture record is announced with its removal, and the removal clears it', async () => {
  unreadLists = ['URLS']; unreadWhat = WEB; refuseEdits = true;
  await openDocument();
  assert.equal(listShown(), true, 'a web capture record Nib could not read opened with no notice');
  assert.match(listText(), new RegExp('holds ' + WEB), `the notice says ${JSON.stringify(listText())}`);
  assert.match(listText(), /until it is removed\.$/, `the notice does not say the way out: ${JSON.stringify(listText())}`);
  assert.equal(removeShown(), true, 'the notice does not offer the removal');
  const n = doc.getElementById('listNotice');
  assert.equal(n.getAttribute('role'), 'status', 'the notice carries no role');
  assert.equal(n.closest('dialog'), null, 'the notice is in a dialog — it is a page-level banner');

  // A refused change, after a dismissal, brings the offer back: the dismissal was of the warning.
  doc.getElementById('listNoticeDismiss').click();
  await settle();
  assert.equal(listShown(), false, 'Dismiss did not take the notice down');
  await anEdit();
  assert.equal(listShown(), true, 'a change refused over the record left its removal dismissed');

  const before = h.calls.length;
  doc.getElementById('listNoticeRemove').click();
  await settle();
  const posted = h.calls.slice(before).find((c) => c.url.includes('/api/sanitize'));
  assert.ok(posted, 'Remove it posted nothing');
  assert.match(posted.url, /method=webcapture/, `Remove it posted ${posted.url}`);
  assert.equal(posted.method, 'POST', 'the removal was not a POST');
  assert.equal(listShown(), false, 'the notice is still up after the record was removed');
});

