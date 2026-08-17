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
    // Needed by the Compare test below, which has to reach the branch of installOpened
    // that reuses the empty view — the only open path that does NOT go through
    // activateView, and therefore the only one where D11's open half is observable.
    '/api/close': () => ({}),
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

// ── P05.S05 — the sidebars ──────────────────────────────────────────────────
//
// Same decoy technique as the sweeps above, and the same fidelity rule: the decoy is built
// to the shape newView() produces, with a real `.thumbwrap` carrying a `dataset.page`,
// because a decoy that differs from the real thing tests the difference and not the rule.

test('each view builds its own sidebar containers into the shared panels', () => {
  const thumbs = doc.getElementById('thumbs');
  const outline = doc.getElementById('outline');

  assert.equal(doc.getElementById('thumbGrid'), null,
    'a static #thumbGrid is back — with several views that id is no longer unique');
  assert.equal(thumbs.querySelectorAll('.thumbgrid').length, 1, 'the boot view built no grid');
  assert.equal(outline.querySelectorAll('.outlinelist').length, 1, 'the boot view built no outline list');

  // #outline must stay ONE id: the tab machinery resolves panels by getElementById and the
  // link styling is a `#outline a` descendant selector.
  assert.ok(outline.classList.contains('panel'), '#outline stopped being the panel node');
});

test('a build aimed at one view cannot clear another view grid', async () => {
  // **Re-aimed at P06.S01, and the decoy is gone.** This test used to open the same
  // document twice, which under the old semantics REBUILT the active view's grid, and
  // it needed a hand-built decoy grid to stand in for the second view that could not
  // exist. Open now ADDS, so the second open builds a real second view with a real
  // grid of its own — the stimulus the decoy was imitating. Two things improve at
  // once: the population is the app's own rather than the test's, and the shape can no
  // longer drift from what newView() produces, which is exactly how the first version
  // of this test passed 42/42 against a document-wide sweep.
  await openDocument();
  const thumbs = doc.getElementById('thumbs');
  const firstGrid = thumbs.querySelector('.thumbgrid:not([hidden])');
  assert.ok(firstGrid, 'setup: the first view has no visible grid');

  // A sentinel in the FIRST view's grid. It is what makes the survival assertion mean
  // "the second build did not reach in here" rather than "this grid was always empty".
  const sentinel = doc.createElement('div');
  sentinel.className = 'thumbwrap';
  sentinel.dataset.page = '99';
  firstGrid.appendChild(sentinel);
  const gridsBefore = thumbs.querySelectorAll('.thumbgrid').length;

  await openDocument({ numPages: 2 });

  // The positive half: a SECOND view exists and built its own grid. Without this the
  // sentinel surviving proves nothing — an open that failed outright would pass it.
  const gridsAfter = thumbs.querySelectorAll('.thumbgrid').length;
  assert.equal(gridsAfter, gridsBefore + 1,
    `opening a second document produced ${gridsAfter - gridsBefore} new thumbnail grids, want 1 — the open did not add a view`);
  const secondGrid = thumbs.querySelector('.thumbgrid:not([hidden])');
  assert.notEqual(secondGrid, firstGrid, 'the visible grid is still the first view\'s — the new view was not activated');
  // jsdom renders no canvas, so exactly one .thumbwrap lands before the render rejects.
  assert.equal(secondGrid.querySelectorAll('.thumbwrap').length, 1,
    'the second view built no thumbnail, so its grid is empty for a reason unrelated to scoping');

  // The negative half: the first view's grid is untouched, sentinel included.
  assert.equal(sentinel.isConnected, true,
    'opening a second document cleared the FIRST view\'s thumbnail grid — the sidebars are not view-scoped');
  assert.equal(firstGrid.hidden, true, 'the first view\'s grid was left visible alongside the second');
});

test('opening a second document adds a tab, and clicking one switches to it', async () => {
  // P06.S01's central claim, driven rather than scanned. The strip is client state, so
  // this tier is the right home for it: no server round-trip decides what it shows.
  const strip = doc.getElementById('tabstrip');
  assert.ok(strip, 'there is no #tabstrip in index.html');

  // The state before, asserted rather than assumed — the app has been opening documents
  // in the tests above, so "two tabs" here has to be measured against what is actually
  // open rather than against a fresh boot.
  const before = strip.querySelectorAll('.tab').length;
  const containersBefore = wrap.querySelectorAll('.viewerContainer').length;

  await openDocument({ numPages: 4 });

  const tabs = [...strip.querySelectorAll('.tab')];
  assert.equal(tabs.length, before + 1, `the strip shows ${tabs.length} tabs, want ${before + 1} — an open did not add one`);
  assert.equal(strip.hidden, false, 'the strip is hidden with several documents open');
  assert.equal(wrap.querySelectorAll('.viewerContainer').length, containersBefore + 1,
    'no container was built for the new document');

  // Exactly one tab is active, and it is the last one — the just-opened document.
  const active = tabs.filter((t) => t.classList.contains('active'));
  assert.equal(active.length, 1, `${active.length} tabs are marked active, want exactly 1`);
  assert.equal(active[0], tabs[tabs.length - 1], 'the just-opened document is not the active tab');
  assert.equal(active[0].getAttribute('aria-selected'), 'true',
    'the active tab does not say so to assistive tech — the accent bar is not readable');

  // The switch. The stimulus assertion first: the target must not already be active, or
  // "clicking it activated it" is true before the click.
  const target = tabs[0];
  assert.equal(target.classList.contains('active'), false, 'setup: the tab being clicked is already the active one');
  const targetName = target.textContent;
  target.click();
  await settle();

  const after = [...strip.querySelectorAll('.tab')];
  const nowActive = after.filter((t) => t.classList.contains('active'));
  assert.equal(nowActive.length, 1, 'clicking a tab left more than one active');
  assert.equal(nowActive[0].textContent, targetName, 'clicking a tab did not activate that document');
  // One visible container, always — the outgoing view is hidden, never destroyed (ADR-002).
  const visible = [...wrap.querySelectorAll('.viewerContainer')].filter((c) => !c.hidden);
  assert.equal(visible.length, 1, `${visible.length} containers are visible, want exactly 1`);
  assert.equal(wrap.querySelectorAll('.viewerContainer').length, after.length,
    'a container was destroyed on a switch — ADR-002 hides inactive views, it does not tear them down');
});

// ── P05.S06 — the undo stack, and why there is no behavioural test here ─────
//
// The acceptance clause asks for "a guard asserts an overlay-edit undo recorded on one view
// cannot be drained through another — red without the fix". There is no such test in this
// file, deliberately, and the absence is the honest answer rather than an omission:
//
//   * No second view is creatable in any TEST TIER. Note the precision: a second view IS
//     creatable in production — pollRecv -> openArrivalInNewView, live since P05.S04 — so
//     the cross-view hazards this phase closes are reachable today by anyone doing a live
//     co-sign, not latent until P06. What P06 adds is a way to create one WITHOUT a pinned
//     peer, which is what a test would need. An earlier draft of this comment said "no
//     second view is creatable below P06" full stop, which is false and made the residual
//     risk read as unreachable.
//   * No overlay can be PLACED at this tier at all. Every getBoundingClientRect is 0 under
//     jsdom, so pageAt never resolves a page and no drawing tool can complete a gesture —
//     which means the stack cannot even be made non-empty here. This bullet alone is
//     sufficient; the first is context.
//
// A first draft of this test asserted that the undo button starts disabled. It passed
// identically against a shared stack, so it tested the per-view property not at all — a
// green that would have read as coverage for the clause it was written under. It was
// deleted rather than kept.
//
// What covers the clause instead is in view.test.mjs: the binding is in the per-view scan,
// the three field helpers are asserted to take an owner and to contain no active-view read,
// every recordOverlayEdit call is asserted to pass one, and the clear is asserted to sit
// above clearOverlays' shared-chrome return. All four are red-fixtured. The BEHAVIOUR is
// recorded `not exercised` and carried to P06.

// D11's open-path half, which the switch path has had since P05.S04.
//
// `closeCmpDoc` had four call sites and `setDocumentFromServer` was not among them, so
// opening a different document left the Compare modal on screen showing a cached text
// diff of the PREVIOUS document against the picked file, while the viewer behind it
// showed something else. Reachable by ordinary use, and that is not a guess:
// #compareModal is not in the fixed-overlay group in style.css, so it does not scrim the
// toolbar and Open… stays clickable while it is open.
//
// The fix lives in resetSharedDocState — the shared reset whose entire job is stopping
// the open and close paths from disagreeing, and which covered the drawing modes while
// Compare was never in it.
test('opening a document closes a Compare left open against the previous one', async () => {
  const modal = doc.getElementById('compareModal');
  assert.ok(modal, 'there is no #compareModal — this guard no longer covers anything');

  // **The open must be the one that REUSES the empty view.** A first draft opened a
  // second document instead, which goes through activateView — and activateView has
  // called closeDocBoundModals since P05.S04, so the modal closed for a reason that had
  // nothing to do with the fix under test. Deleting the fix left the test green, which
  // is how it was caught: a red-proof, not a review. The open path is only
  // distinguishable from the switch path on the branch that installs into the view that
  // is already there.
  await openDocument();
  // Close ALL — this needs to collapse to the single empty view, and #closeBtn is
  // close-VIEW once several are open (P06.S02).
  const all = doc.getElementById('closeAllBtn');
  (all && !all.hidden ? all : doc.getElementById('closeBtn')).click();
  await settle();
  assert.equal(win.document.querySelectorAll('.viewerContainer').length, 1,
    'setup: the close did not collapse to a single view, so the open below will take the switch path and prove nothing');

  // Open Compare, and assert it opened — with the modal already hidden, "it is hidden
  // after an open" is true before the open and reads a state nothing produced.
  modal.hidden = false;
  assert.equal(modal.hidden, false, 'setup: the Compare modal did not open, so closing it proves nothing');

  await openDocument({ numPages: 2 });

  assert.equal(modal.hidden, true,
    'opening a document left Compare on screen — it is now showing a cached diff of a document that is no longer in front of it');
});

// P06.S02 — the per-tab close.
//
// Two properties, and the second is the one that bites: the × must close the document,
// and it must NOT also switch to it. They fail apart — a × with no stopPropagation
// closes the right document only because the switch ran first and made it active, and
// the moment the neighbour logic differs the user loses the tab they were on. The flag
// × on the pending list is the same defect one surface over: its pointerdown reaches
// the placement handler, so deleting a flag plants another.
test('a tab close button closes that document without switching to it first', async () => {
  const strip = doc.getElementById('tabstrip');
  await openDocument();
  await openDocument({ numPages: 2 });
  await openDocument({ numPages: 4 });

  const before = [...strip.querySelectorAll('.tab')];
  assert.ok(before.length >= 3, `setup: only ${before.length} tabs — nothing below can distinguish closing from switching`);
  assert.equal(strip.querySelector('.tab.active'), before[before.length - 1], 'setup: the last-opened document is not the active tab');
  assert.equal(doc.querySelector('.pageCount').textContent, '/ 4', 'setup: the active document is not the 4-page one');

  // Close the FIRST tab, which is not the active one. If the × also switched, the
  // active tab afterwards would be a different document than the one we started on.
  const target = before[0];
  target.querySelector('.tabclose').click();
  await settle();

  const after = [...strip.querySelectorAll('.tab')];
  assert.equal(after.length, before.length - 1, `closing a tab left ${after.length} tabs, want ${before.length - 1}`);
  assert.ok(!after.includes(target), 'the closed tab is still in the strip');

  // **The document you were reading is still the one you are reading.** This is the
  // assertion the × 's stopPropagation exists for, and it is only observable because
  // closing a background tab does NOT activate it first — an earlier draft activated
  // and then closed, which made this indistinguishable from the defect.
  //
  // Identified by PAGE COUNT, not by tab label: every fixture in this file is called
  // doc.pdf, so a name comparison is 'doc.pdf' vs 'doc.pdf' and cannot fail. The counts
  // are 3, 2 and 4, which actually tell the three documents apart.
  assert.equal(doc.querySelector('.pageCount').textContent, '/ 4',
    'closing a background tab moved the user to a different document — closing tab 1 while reading tab 3 must leave you on tab 3');
  assert.equal(strip.querySelectorAll('.tab.active').length, 1,
    'after closing a background tab there is not exactly one active tab');

  // Exactly one active tab, always — a close that forgot to activate a neighbour leaves
  // none, and one that activated two is a rendering bug the user acts on.
  assert.equal(strip.querySelectorAll('.tab.active').length, 1,
    'after closing a tab there is not exactly one active tab');
  assert.equal(doc.querySelectorAll('.viewerContainer:not([hidden])').length, 1,
    'after closing a tab there is not exactly one visible container');
});

test('closing the last document returns the app to the launch state', async () => {
  const strip = doc.getElementById('tabstrip');
  // Down to one, whatever the tests above left open.
  const all = doc.getElementById('closeAllBtn');
  (all && !all.hidden ? all : doc.getElementById('closeBtn')).click();
  await settle();
  await openDocument();
  assert.equal(strip.hidden, true, 'setup: the strip is showing, so more than one document is open');
  assert.equal(doc.getElementById('viewerWrap').className, 'has-doc', 'setup: nothing is open to close');

  doc.getElementById('closeBtn').click();
  await settle();

  // The launch state, not an emptied view record sitting in a strip.
  assert.equal(doc.getElementById('viewerWrap').className, '', 'closing the last document did not return to the launch state');
  assert.equal(strip.hidden, true, 'the tab strip is showing with nothing open');
  assert.equal(doc.getElementById('closeBtn').disabled, true, 'Close is still enabled with nothing open');
});
