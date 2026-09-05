// /pending 333 — the file underneath an open document changed, and the app has to say so.
//
// ── What this tier can reach ─────────────────────────────────────────────────
// The banner is a DOM state driven by a field on the server's reply, so jsdom sees all
// of it: which message is shown, which button is offered, and — the part that matters —
// which button is NOT. It also sees the precedence rule between the two conditions the
// one banner now carries.
//
// ── What it cannot, and who covers it ────────────────────────────────────────
// That the server ever SETS diskChanged: that is tier 1 (internal/server/diskstate_test.go),
// which rewrites a real file under a real open document. And the round trip of the two
// together — a real binary, a real file rewritten on disk, a real browser — is tier 3.
// This tier stubs the reply, so a server that never sets the field would leave every
// assertion here green. That is the ceiling, and it is why the tier-1 rows exist.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const DOC = '/tmp/nib-harness/doc.pdf';

// The open reply is mutated per test, so one boot can drive both the changed and the
// unchanged case without a second process.
// The id is not decoration: recheckDisk pins its request to the document (ADR-004) and
// declines to ask about one it cannot name, so a reply without an id would make the
// background re-check silently do nothing — and every assertion about it pass for the
// wrong reason. The real server sends one on every document reply.
let openReply = {
  id: 'd1', name: 'doc.pdf', path: DOC, canSave: true,
  signature: { state: 'unsigned' }, canUndo: false, canRedo: false,
};

const h = await boot({
  routes: {
    '/api/open': () => openReply,
    '/api/doc': () => openReply,
    // Undo reloads the view that asked, which is the only ordinary path that puts a
    // fresh render onto a view that already holds a document. See the precedence test.
    '/api/undo': () => openReply,
    // The remedy. The server has re-read the file, so its answer is the same document
    // with the flag DOWN — which is what lets these tests tell a reload that happened
    // from one that was merely attempted.
    '/api/reload': () => ({ ...openReply, diskChanged: false }),
  },
});
const { document: doc, settle } = h;

async function openDocument() {
  setNextDocument({ numPages: 2, outline: null });
  doc.getElementById('pathInput').value = DOC;
  doc.getElementById('openGo').click();
  await settle();
}

const banner = () => doc.getElementById('staleBanner');
const msg = () => doc.getElementById('staleMsg').textContent;
const retry = () => doc.getElementById('staleRetry');
const reload = () => doc.getElementById('staleReload');

test('an unchanged file raises no banner', async () => {
  openReply = { ...openReply, diskChanged: false };
  await openDocument();
  assert.equal(banner().hidden, true,
    'the banner is up over a document whose file has not changed — a warning that shows on every document is one the user learns to ignore');
});

test('a changed file names the file, the copy on screen, and what saving would cost', async () => {
  openReply = { ...openReply, diskChanged: true };
  await openDocument();

  assert.equal(banner().hidden, false,
    'the file changed on disk and nothing is shown — this is /pending 333: the user has no way to learn what she is looking at is not the file');

  const text = msg();
  assert.match(text, /changed on disk/i,
    'the message does not say the file changed on disk');
  assert.match(text, /saving would replace/i,
    'the message does not say what saving would cost — the banner is dismissible-adjacent and the warning has to be carried while she is reading it, not after');

  // Each of these would be a FALSE STATEMENT in a case the banner can fire, which is
  // the defect shape this repo keeps finding. Nib cannot know who wrote the file (it
  // may have been the user's own `nib … -w`), and mtime can go backwards, so "newer"
  // is not safe either.
  assert.doesNotMatch(text, /another program|someone else|somebody/i,
    'the message attributes the change to a party Nib cannot identify');
  assert.doesNotMatch(text, /newer/i,
    'the message claims the file on disk is "newer" — restoring a backup over it makes that false while "changed" stays true');
  assert.doesNotMatch(text, /cache|in-memory|memory|mtime|hash/i,
    'the message asks a non-technical user to reason about implementation');
});

test('a changed file offers reload and withholds retry', async () => {
  openReply = { ...openReply, diskChanged: true };
  await openDocument();

  assert.equal(reload().hidden, false,
    'no reload button — the item\'s own words were "a hard reload didn\'t update it", so an affordance that re-reads the file IS the fix');
  assert.equal(retry().hidden, true,
    'Try again is offered for a disk change. It re-runs the same fetch, re-renders the SAME in-memory bytes and then clears the banner on success — so the warning would vanish while the file was still different, at exactly the moment before the user saves');
});

test('a render failure outranks a disk change, and takes its own button back', async () => {
  // Driven through UNDO, not a second open, and the difference is not cosmetic: an open
  // installs a NEW view, and markStale deliberately refuses a view with no document yet
  // ("not what the server holds" is meaningless when nothing is on screen). So the two
  // conditions can only ever coexist on a view that already holds a document, and undo
  // is the ordinary path that reloads the one you are looking at.
  openReply = { ...openReply, diskChanged: true, canUndo: true };
  await openDocument();
  assert.equal(reload().hidden, false, 'precondition: the disk-change banner is up');
  assert.equal(doc.getElementById('undoBtn').disabled, false,
    'precondition: undo is available, otherwise the click below does nothing and every assertion after it passes for the wrong reason');

  // Fail the reload with the document still reporting diskChanged. A document that
  // cannot be displayed at all is the more urgent fact and owns the retry.
  setNextDocument({ fail: true });
  doc.getElementById('undoBtn').click();
  await settle();

  assert.equal(banner().hidden, false, 'the render failed and no banner is up');
  assert.match(msg(), /could not be displayed/i,
    'a render failure is being reported with the disk-change message — the more urgent fact lost to the less urgent one');
  assert.equal(retry().hidden, false, 'the render failure did not get its retry button back');
  assert.equal(reload().hidden, true,
    'reload is still offered over a document that failed to render — it would re-open a file whose problem is not that it is stale');
});

// This is the reported scenario itself, and it went uncovered until a mutation pass
// found it: the file is rewritten while Nib is in the BACKGROUND — the user is in a
// terminal, or another application — so nothing reloads the document and the banner
// would otherwise wait for her next operation. In the case /pending 333 was filed for,
// her next operation would have been the Save that overwrote the newer file.
test('coming back to the window reloads a clean document by itself', async () => {
  // A fresh id per test, and it is not decoration. installOpened finds the view it just
  // filled with `views.find(x => x.docMeta.id === meta.id)`, so with one id reused across
  // tests the dirty-clear lands on the FIRST view still holding it and the new view keeps
  // the `dirty` that every arrival sets — which would make this test assert that the
  // automatic path declines, for a reason that cannot occur in production. Document ids
  // are monotonic and never reused there (ADR-001).
  openReply = { ...openReply, id: 'd-auto', diskChanged: false, canUndo: false };
  await openDocument();
  assert.equal(banner().hidden, true, 'precondition: nothing is wrong yet');

  // The file changes while the window is in the background. No open, no undo, no
  // operation of any kind — only the server's answer to /api/doc changes.
  openReply = { ...openReply, diskChanged: true };
  setNextDocument({ numPages: 2, outline: null }); // the reload has to have something to render
  const from = h.calls.length;
  h.window.dispatchEvent(new h.window.Event('focus'));
  await settle();

  const reloads = h.calls.slice(from).filter((c) => c.url.startsWith('/api/reload'));
  assert.equal(reloads.length, 1,
    'the user came back to a document with no unsaved work whose file had changed, and Nib left her looking at the stale copy with a banner to read — the remedy is safe here and taking it is the whole feature');
  assert.equal(reloads[0].method, 'POST', 'the reload was not a POST');
  // The route the client fires WITHOUT the user asking is the one that must never fall
  // back to "whichever document is active" (ADR-004): she may have switched tabs.
  assert.ok(reloads[0].headers['X-Nib-Doc'],
    'the automatic reload went out unpinned, so the server would reload the file underneath whichever document happened to be active');
  assert.equal(banner().hidden, true,
    'the document was reloaded and the banner is still up — it now describes a state that no longer exists');
});

test('a document with unsaved work is never reloaded by itself', async () => {
  // canUndo at open is what openedDirty reads, so this is a view with work in it.
  openReply = { ...openReply, id: 'd-dirty', diskChanged: false, canUndo: true };
  await openDocument();

  openReply = { ...openReply, diskChanged: true };
  const from = h.calls.length;
  h.window.dispatchEvent(new h.window.Event('focus'));
  await settle();

  assert.equal(h.calls.slice(from).filter((c) => c.url.startsWith('/api/reload')).length, 0,
    'Nib reloaded a document with unsaved work without being asked — a reload replaces the bytes, so this destroys exactly what the banner exists to protect');
  assert.equal(banner().hidden, false,
    'the automatic path correctly declined and then said nothing — this user is the one who most needs the banner');
  assert.equal(reload().hidden, false,
    'the banner is up without the button, so the user who was denied the automatic reload has no way to ask for it');
});

test('the reload button re-reads in place instead of opening a second copy', async () => {
  openReply = { ...openReply, id: 'd-button', diskChanged: true, canUndo: false };
  await openDocument();
  assert.equal(reload().hidden, false, 'precondition: the reload button is offered');

  setNextDocument({ numPages: 2, outline: null });
  const from = h.calls.length;
  reload().click();
  await settle();

  const after = h.calls.slice(from);
  assert.equal(after.filter((c) => c.url.startsWith('/api/reload')).length, 1,
    'the button did not reload');
  // The old button went through /api/open, which built a second view on the same path and
  // closed the first. That reported sameFileOpen on every press — "your file is open in
  // another tab as two separate copies" about a tab closed a line later — and moved the
  // user's document to the end of the strip.
  assert.equal(after.filter((c) => c.url.startsWith('/api/open')).length, 0,
    'Reload from disk opened the file as a SECOND document — the user is told her file is open twice, and her tab moves to the end of the strip');
});

test('a document reloaded by itself is not left looking unsaved', async () => {
  // The reload makes the open copy MATCH the file, so there is nothing unsaved left to
  // lose. Every arrival through setDocumentFromServer sets `dirty` — right for the twenty
  // operations that reach it, wrong here — so without the clear the user is asked to
  // confirm discarding work that a reload she never requested appeared to create.
  openReply = { ...openReply, id: 'd-clean', diskChanged: false, canUndo: false };
  await openDocument();

  openReply = { ...openReply, diskChanged: true };
  setNextDocument({ numPages: 2, outline: null });
  const from = h.calls.length;
  h.window.dispatchEvent(new h.window.Event('focus'));
  await settle();
  assert.equal(h.calls.slice(from).filter((c) => c.url.startsWith('/api/reload')).length, 1,
    'precondition: no reload happened, so the close below is not testing what this test is named for');

  const asked = h.confirms.length;
  doc.getElementById('closeBtn').click();
  await settle();
  assert.equal(h.confirms.length, asked,
    'closing a document that Nib had just reloaded from disk prompted about unsaved changes — the reload left it marked dirty, so the user is asked to discard work that only the reload appeared to create');
});

test('the banner is announced to a screen reader', () => {
  // It appears without any user action, so a screen-reader user was told nothing about
  // either condition. Polite rather than assertive: #sessionNotice reserves assertive
  // for "you are about to lose a signature", and spending it here devalues that.
  assert.equal(banner().getAttribute('role'), 'status',
    'the banner has no role — it appears unannounced');
  assert.equal(banner().getAttribute('aria-live'), 'polite',
    'the banner is not a live region, so its text is never read out');
});
