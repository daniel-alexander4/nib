// P05.S03 — the per-view viewer and DOM, driven against the real document.
//
// Its sibling view.test.mjs scans the source; this file boots the app and asserts what
// the app actually builds and what its cleanup actually reaches. The split is forced,
// not stylistic: boot() is once per process (see boot.mjs), and view.test.mjs is a pure
// source scan with no boot at all.
//
// ── What this tier can reach ─────────────────────────────────────────────────
// The DOM the app builds at boot, and — via a decoy container — whether a cleanup sweep
// is view-scoped or document-scoped. That last one is the behavioural half of ADR-002's
// consequence and it is genuinely red without the fix.
//
// ── What it cannot, and who covers it ────────────────────────────────────────
// A real switch. One view exists until P06 gives opens a way to add one, so "inactive
// views hidden, never destroyed" and "the pointer listeners survive a view being hidden"
// are asserted structurally in view.test.mjs and recorded `not exercised` as behaviour.
// jsdom also has no layout, so the hidden-container-reports-zero-width path that
// P05.S04's re-fit exists for is invisible here by construction.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const DOC = '/tmp/nib-harness/doc.pdf';
const h = await boot({
  routes: {
    '/api/open': () => ({
      name: 'doc.pdf', path: DOC, canSave: true,
      signature: { state: 'unsigned' }, canUndo: false, canRedo: false,
    }),
  },
});
const { document: doc, window: win, settle } = h;
const wrap = doc.getElementById('viewerWrap');

// The drawing tools are in DOC_REQUIRED and disabled until something is open, so the
// sweep test cannot arm split-box without this. Driven through the real Open dialog,
// as lifecycle.test.mjs does — app.js exports nothing.
async function openDocument({ numPages = 3 } = {}) {
  setNextDocument({ numPages, outline: null });
  doc.getElementById('pathInput').value = DOC;
  doc.getElementById('openGo').click();
  await settle();
}

test('a view builds its own DOM, and the wrap stays the one stable node', () => {
  // The static markup is gone, because two open documents cannot share an id.
  assert.equal(doc.getElementById('viewerContainer'), null,
    'a static #viewerContainer is back — with several views that id is no longer unique');
  assert.equal(doc.getElementById('viewer'), null, 'a static #viewer is back');

  // …and the boot view built its own.
  const containers = wrap.querySelectorAll('.viewerContainer');
  assert.equal(containers.length, 1, 'the boot view did not build its container');
  assert.equal(containers[0].parentElement, wrap, 'the container is not a child of the stable wrap');
  assert.equal(containers[0].querySelectorAll('.pdfViewer.viewerPages').length, 1,
    'the container has no page stack — PDFViewer requires one and does not create it');
});

test('views are inserted before the launch message, not after it', () => {
  // A structural check, and it is worth being honest about how much it proves. Both
  // elements are absolutely positioned with inset:0 and neither sets a z-index, so DOM
  // order decides paint order — but a container that painted over #empty would still show
  // the message through, because .viewerContainer sets no background and is empty until a
  // document loads, and #empty is display:none the moment `has-doc` lands. So this pins
  // the INSERTION POINT newView() was written to use; it is not evidence about anything a
  // user could see, and the reasoning above would break silently if either element gained
  // a z-index or a background. That is tier 3's to catch, and it did — a probe painting
  // the container bright red left the message fully readable.
  const empty = doc.getElementById('empty');
  const container = wrap.querySelector('.viewerContainer');
  assert.ok(container.compareDocumentPosition(empty) & win.Node.DOCUMENT_POSITION_FOLLOWING,
    'a view container is inserted after #empty — newView() should insert before it');
});

// A second view does not exist until P06, so a decoy container stands in. It must be
// built to the SAME shape newView() produces — .viewerContainer > .pdfViewer.viewerPages
// > .page — and the mark must sit where the app really puts one, inside a page div
// (app.js: `sbHit.pv.div.appendChild(sbDiv)`).
//
// That fidelity is not pedantry; the first version of this test got it wrong and was
// vacuous because of it. With a decoy that had no page stack, and the live mark parked
// on .viewerPages rather than in a .page, a sweep of `document.querySelectorAll(
// '.viewerPages .splitmark')` — document-wide, reaching every open view — passed BOTH
// assertions: it cleared the live mark, and it missed the decoy's only because the decoy
// lacked the element the selector keyed on. A decoy that differs from the real thing
// tests the difference, not the rule.
function plantDecoy(markClass) {
  const decoy = doc.createElement('div');
  decoy.className = 'viewerContainer';
  const pages = doc.createElement('div');
  pages.className = 'pdfViewer viewerPages';
  const page = doc.createElement('div');
  page.className = 'page';
  const mark = doc.createElement('div');
  mark.className = markClass;
  page.appendChild(mark);
  pages.appendChild(page);
  decoy.appendChild(pages);
  wrap.appendChild(decoy);
  return decoy;
}

// …and the same shape in the ACTIVE view. Without a live mark the test passes against a
// sweep that removes NOTHING at all, which is the other half of the same vacuity.
function plantLive(live, markClass) {
  let page = live.querySelector('.page');
  if (!page) {
    page = doc.createElement('div');
    page.className = 'page';
    live.querySelector('.viewerPages').appendChild(page);
  }
  const mark = doc.createElement('div');
  mark.className = markClass;
  page.appendChild(mark);
  return mark;
}

test('the split-box sweep cannot reach another view’s marks', async () => {
  await openDocument();
  const live = wrap.querySelector('.viewerContainer');
  assert.equal(doc.getElementById('splitBoxBtn').disabled, false,
    'setup: the split-box tool is still disabled, so the clicks below would be no-ops and the sweep would never run');

  const decoy = plantDecoy('splitmark');
  plantLive(live, 'splitmark');

  // Arm then disarm the split-box tool: exitSplitBox() -> clearSplitRects().
  doc.getElementById('splitBoxBtn').click();
  doc.getElementById('splitBoxBtn').click();
  await settle();

  assert.equal(live.querySelectorAll('.splitmark').length, 0,
    'the sweep did not clear the active view — so the survival below proves nothing');
  assert.equal(decoy.querySelectorAll('.splitmark').length, 1,
    'the sweep reached another view and removed a mark from a document the user is not looking at');
  decoy.remove();
});

// clearCropRect is the second sweep this slice re-rooted, and it gets its own test rather
// than a comment in the app saying it works "for the same reason". A comment is not a
// check: without this, clearCropRect could regress to `all('.cropmark')` with the whole
// suite green.
test('the crop sweep cannot reach another view’s marks', async () => {
  const live = wrap.querySelector('.viewerContainer');
  assert.equal(doc.getElementById('cropBtn').disabled, false,
    'setup: the crop tool is still disabled, so the clicks below would be no-ops and the sweep would never run');

  const decoy = plantDecoy('cropmark');
  plantLive(live, 'cropmark');

  doc.getElementById('cropBtn').click();
  doc.getElementById('cropBtn').click();
  await settle();

  assert.equal(live.querySelectorAll('.cropmark').length, 0,
    'the sweep did not clear the active view — so the survival below proves nothing');
  assert.equal(decoy.querySelectorAll('.cropmark').length, 1,
    'the sweep reached another view and removed a mark from a document the user is not looking at');
  decoy.remove();
});
