// P01's end-to-end acceptance, against the real binary in a real browser.
//
// These are the flows a human drove by hand while P01 was being built. This is
// the last tier: anything it cannot exercise is a standing gap to be filed, not
// delegated onward, so each `not exercised` here would be a real admission.
//
// Two clauses land here specifically because tier 2 could not reach them:
//   * the thumbnail grid (jsdom has no canvas, so its grid is empty either way);
//   * the overlayFields / overlayHistory edit signals (they need a placed
//     overlay, which needs layout).
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const h = await launch();
const { page } = h;
after(() => h.browser.close());

const DOC = writeFixture('doc.pdf', { pages: 3, label: 'doc A page' });
const OTHER = writeFixture('other.pdf', { pages: 5, label: 'doc B page' });
const BIG = writeFixture('big.pdf', { pages: 80, label: 'big page' });
// 2000 pages, the LAST of them twice as wide as the rest. Both numbers are load-bearing
// and the test below says why: the page count is what makes the load slow enough to act
// inside, and the odd page out is what makes both edges of the load window visible in
// the DOM.
const MIXED = writeFixture('mixed.pdf', { pages: 2000, label: 'mixed page', widePage: 2000 });

const chrome = () => page.evaluate(() => ({
  wrap: document.getElementById('viewerWrap').className,
  empty: document.getElementById('empty').textContent,
  badge: document.getElementById('sigBadge').textContent,
  badgeClass: document.getElementById('sigBadge').className,
  saveDisabled: document.getElementById('saveBtn').disabled,
  saveTitle: document.getElementById('saveBtn').title,
  closeDisabled: document.getElementById('closeBtn').disabled,
  pageCount: document.querySelector('.pageCount').textContent,
  pageNum: document.querySelector('.pageNum').value,
  thumbs: document.querySelector('.thumbgrid:not([hidden])')?.children.length ?? 0,
  // CONTENT, not children: since P05.S05 each view's outline is a `.outlinelist` wrapper
  // inside the shared `#outline` panel, so `children.length` counts open views.
  outline: document.querySelectorAll('#outline .outline-edit, #outline a').length,
}));
const closeDoc = () => h.closeDocument(); // File mode first — see harness.mjs

// The clause tier 2 had to skip. Here the grid genuinely populates, so an empty
// grid after a Close is EARNED — under jsdom it is empty either way, which is a
// structural zero and would have been a green nobody paid for.
test('the empty state matches launch, thumbnail grid included', async () => {
  await h.openDocument(DOC, 3);
  await page.waitForFunction(() => document.querySelector('.thumbgrid:not([hidden])')?.children.length === 3);

  const open = await chrome();
  assert.equal(open.wrap, 'has-doc');
  assert.equal(open.pageCount, '/ 3');
  assert.equal(open.thumbs, 3, 'the grid must POPULATE before its emptiness means anything');
  // counts().pages carries a careful argument about being scoped to the VISIBLE
  // container (with two views open, a bare selector sums 3 + 5 = 8 and reads as a page
  // count) — and until the P05 graduation pass it had no reader at all: a correctness
  // argument protecting a value nobody consulted. Read here, where a document of known
  // length is already open, so the argument is defended by an assertion rather than by
  // a comment.
  assert.equal((await h.counts()).pages, 3, 'the visible viewer must hold one .page per page of the document');
  assert.equal(open.saveDisabled, false);
  assert.ok(open.saveTitle.includes(DOC));

  h.dialogs.length = 0;
  await closeDoc();

  const shut = await chrome();
  assert.equal(shut.wrap, '');
  assert.equal(shut.empty, 'Open a PDF to begin.');
  assert.equal(shut.badge, 'no document');
  assert.equal(shut.badgeClass, 'badge badge-none');
  assert.equal(shut.saveDisabled, true);
  assert.equal(shut.saveTitle, 'Save (overwrites the original)');
  assert.equal(shut.closeDisabled, true);
  assert.equal(shut.pageCount, '/ 0');
  assert.equal(shut.pageNum, '1');
  assert.equal(shut.thumbs, 0);
  // The grid must still EXIST, emptied. `?? 0` reports 0 for a missing grid too, so a
  // teardown that removed the active view's container instead of clearing it would read as
  // "empty" and pass the line above — and the reopen test that follows checks pageCount but
  // never thumbs, so nothing else would catch it.
  assert.notEqual(await page.$('.thumbgrid'), null,
    'the active view kept no grid after a close — a reopen would render into a detached node');
  assert.equal(shut.outline, 0);
  assert.equal(shut.wrap, '');
  // A clean document: no prompt. Evidence only because the prompt is driven below.
  assert.equal(h.dialogs.length, 0, 'a freshly opened document must not prompt');
});

test('reopening after a close works normally', async () => {
  await h.openDocument(OTHER, 5);
  const c = await chrome();
  assert.equal(c.wrap, 'has-doc');
  assert.equal(c.pageCount, '/ 5', 'a different document must actually have opened');
  const undo = await page.evaluate(async () => (await (await fetch('/api/doc')).json()).canUndo);
  assert.equal(undo, false, 'the undo ring must not survive a close');
  h.dialogs.length = 0;
  await closeDoc();
});

// S02's first delegation, discharged. Driven on its own: no server history, no
// pdf.js annotation edits — only a placed overlay.
test('the close prompt fires from a placed overlay alone', async () => {
  await h.openDocument(DOC, 3);
  const before = await h.counts();
  assert.equal(before.overlays, 0, 'setup: no overlays yet');

  await h.placeMarker('date');
  const after = await h.counts();
  assert.equal(after.markers, 1, 'setup: a marker must actually be placed');

  h.dialogs.length = 0;
  h.answerDialogs(false); // cancel — this doubles as the cancel-is-safe check
  await closeDoc();
  assert.equal(h.dialogs.length, 1, 'a document with a placed overlay must prompt');
  // "unsaved" since v1.108.7. It read "since the last save" while the app could only
  // answer "since it was opened" — a save cleared none of the four signals — so the
  // hedge was the honest wording then and is the inaccurate one now.
  assert.match(h.dialogs[0], /unsaved/);

  // Cancel left everything alone.
  const kept = await chrome();
  assert.equal(kept.wrap, 'has-doc', 'cancelling must not close the document');
  assert.equal((await h.counts()).markers, 1, 'cancelling must not discard the overlay');
  const pdfStatus = await page.evaluate(async () => (await fetch('/api/pdf')).status);
  assert.equal(pdfStatus, 200, 'the server must still hold the document after a cancel');

  h.answerDialogs(true);
  await closeDoc();
  assert.equal((await chrome()).wrap, '', 'confirming must close it');
});

// S02's second delegation. The care this needs: placing THEN deleting must leave
// overlayHistory non-empty with no fields left — otherwise it is really the test
// above wearing a different name.
test('the close prompt fires from overlay history alone, with no overlays left', async () => {
  await h.openDocument(DOC, 3);
  await h.placeMarker('date');
  assert.equal((await h.counts()).markers, 1, 'setup: placed');

  await h.deleteMarker();
  const gone = await h.counts();
  assert.equal(gone.overlays, 0, 'the overlay must be gone — otherwise this is the previous test');

  h.dialogs.length = 0;
  await closeDoc();
  assert.equal(h.dialogs.length, 1,
    'an overlay edit history must prompt even with no overlays remaining');
});

// P01.S03's G2, and the row whose instrument already misled once: a grid
// child-count cannot tell "the fix worked" from "the race never happened", so the
// in-flight count is captured AT CLOSE TIME and asserted strictly between.
test('closing mid-thumbnail-build leaves no orphan thumbnail', async () => {
  await h.openDocument(BIG, 80);
  const total = await page.evaluate(() => Number(document.querySelector('.pageCount').textContent.replace('/ ', '')));
  assert.equal(total, 80);

  // Wait for the build to be demonstrably underway but nowhere near done.
  await page.waitForFunction(() => document.querySelector('.thumbgrid:not([hidden])')?.children.length >= 3);
  const atClose = (await h.counts()).thumbs;

  h.dialogs.length = 0;
  await closeDoc();
  await page.waitForTimeout(2500); // well past any in-flight page render

  assert.ok(atClose > 0 && atClose < total,
    `NOT EXERCISED: the build was not in flight at close time (${atClose} of ${total})`);
  assert.equal((await h.counts()).thumbs, 0, 'no thumbnail may survive the close');
});

test('a failed close tears nothing down', async () => {
  await h.openDocument(DOC, 3);
  // The stimulus counter, and without it this test proves nothing: EVERY assertion
  // below is already true before the click. Ask the question — what would this have
  // missed if the close had done nothing at all? — and the answer was "nothing, it
  // would still be green". A close that never fired, a control that never armed, or
  // a settle that returned too early all read as "a failed close tore nothing down".
  let refusals = 0;
  await page.route('**/api/close', (route) => {
    refusals++;
    return route.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"injected"}' });
  });

  h.dialogs.length = 0;
  await closeDoc();

  assert.ok(refusals > 0,
    'NOT EXERCISED: /api/close was never called, so the failure this test injects never happened');

  const kept = await chrome();
  assert.equal(kept.wrap, 'has-doc', 'a failed close must not tear down the client');
  assert.equal(kept.pageCount, '/ 3');
  assert.equal(kept.closeDisabled, false, 'the control must stay usable');
  const status = await page.evaluate(async () => (await fetch('/api/pdf')).status);
  assert.equal(status, 200, 'the server must still be serving the document');

  await page.unroute('**/api/close');
  await closeDoc();
});

// The visible view's widths. Scoped to `:not([hidden])` for the reason harness.mjs
// spells out: with several containers in the wrap a bare selector measures whichever
// one the document ordered first, which need not be the one on screen.
const widths = () => page.evaluate(() => {
  const c = document.querySelector('.viewerContainer:not([hidden])');
  const pages = c ? c.querySelectorAll('.page') : [];
  return {
    container: c?.clientWidth ?? 0,
    first: pages[0]?.offsetWidth ?? 0,
    last: pages[pages.length - 1]?.offsetWidth ?? 0,
    count: pages.length,
  };
});

test('a zoom set while the document is still loading is not thrown away', async () => {
  // **The load door onto the defect P06's exit criterion names at the switch door.**
  // `pagesinit` fits page one immediately so there is no 100%-then-fit flash, and
  // `pagesloaded` refines that to the WIDEST page once every page view is populated.
  // The refine was unconditional — so a zoom made in between, on a document long enough
  // that "in between" is a real interval, was overwritten the moment the load finished.
  // Silently, and with the user's hand still on the button.
  //
  // This is also the mechanism behind tabs.test.mjs's intermittently-flaky zoom test:
  // there the refine landed after the zoom and before the switch, and the switch then
  // had nothing to preserve.
  await h.openDocument(MIXED, 2000);

  // ── the test is only meaningful inside the load window, so it reads whether it is ──
  // openDocument returns at `pagesinit`, where pdf.js has sized every page div from
  // PAGE ONE's viewport — so page 1 fills the container and the wide last page is still
  // reported portrait. Both are checked: either one failing means the document finished
  // loading before the zoom, and nothing below could have failed.
  const opening = await widths();
  assert.equal(opening.count, 2000, `setup: ${opening.count} page divs, want 2000`);
  assert.ok(opening.first / opening.container > 0.9,
    `setup: page 1 is ${opening.first}px in a ${opening.container}px container — the widest-page refine has ALREADY run, so the load window closed before this test acted and the assertion at the end cannot fail`);
  assert.ok(opening.last < opening.first * 1.5,
    `setup: the last page is already ${opening.last}px against page 1's ${opening.first}px, so it has resolved its own size and the document is loaded — same reason, the window has closed`);

  // The user zooms, inside the window.
  await page.click('#zoomInBtn');
  await page.click('#zoomInBtn');
  const zoomed = (await widths()).first;
  assert.ok(zoomed > opening.first * 1.1,
    `setup: zooming moved page 1 from ${opening.first}px to ${zoomed}px, which is not a change this test can see`);

  // ── then the load finishes ────────────────────────────────────────────────────────
  // The wide page is the LAST one, so its div reaching its true width means every page
  // resolved — which is the condition `pagesloaded` fires on. The settle after it is a
  // bound, not a tuning: the handler runs synchronously off that same promise chain, so
  // if the zoom is going to be overwritten it has been overwritten already.
  await page.waitForFunction(() => {
    const pages = document.querySelectorAll('.viewerContainer:not([hidden]) .page');
    const last = pages[pages.length - 1];
    return last && last.offsetWidth > pages[0].offsetWidth * 1.5;
  });
  await page.waitForTimeout(300);

  const after = (await widths()).first;
  assert.ok(Math.abs(after - zoomed) < 2,
    `page 1 is ${after}px, not the ${zoomed}px the user zoomed it to: the fit that lands when the document finishes loading overwrote a scale the user had already chosen`);

  await closeDoc();
});

// ── Who owns Ctrl+Z ─────────────────────────────────────────────────────────
//
// Placing a note focuses its `textarea.note-text`, and the shortcut handler yielded to any
// typing target — so the keystroke went to a field with an EMPTY native undo stack and the note
// survived it. Measured in Chromium before the fix: 1 overlay before Ctrl+Z, 1 after, and the
// same keystroke removing it the instant the note was blurred. The Undo button was enabled the
// whole time, so nothing in the UI said the shortcut had been swallowed.
//
// **The second half is the regression this must not cause**, and it is why the rule is "has an
// undo of its own" rather than "is empty": type into a note, clear it, and Ctrl+Z must still be
// the field's, not the document's. A field is marked the first time it receives input.
//
// Tier 3 because every step needs layout: a note is placed by clicking a rendered page, and what
// is asserted is which element has FOCUS when a real key event arrives.
test('Ctrl+Z removes a just-placed note, and leaves a typed one to the field', async () => {
  await h.openDocument(DOC, 3);
  await h.topOfDocument();
  await h.mode('markup');
  await h.group('Annotate & Draw');

  const armNote = async () => {
    const armed = await page.evaluate(() => document.getElementById('noteBtn').classList.contains('active'));
    if (!armed) await page.click('#noteBtn');
    await page.waitForFunction(() => document.getElementById('noteBtn').classList.contains('active'));
  };
  const placeNote = async (fx, fy) => {
    await armNote();
    const box = await page.locator('.viewerContainer:not([hidden]) .page').first().boundingBox();
    await page.mouse.click(box.x + box.width * fx, box.y + box.height * fy);
    await page.waitForSelector('.viewerContainer:not([hidden]) .ovl-note');
  };
  const notes = () => page.evaluate(() => document.querySelectorAll('.viewerContainer:not([hidden]) .ovl-note').length);

  await placeNote(0.4, 0.3);
  assert.equal(await notes(), 1, 'setup: no note was placed, so nothing below is being measured');
  // The defect lived in this precondition: the field must actually hold focus, or the yield
  // being tested never happens and the assertion after it passes for the wrong reason.
  assert.equal(await page.evaluate(() => document.activeElement?.className), 'note-text',
    'setup: placing a note no longer focuses its textarea — this test exercises the yield to a focused field and there is none');

  await page.keyboard.press('Control+z');
  await page.waitForTimeout(600);
  assert.equal(await notes(), 0,
    'Ctrl+Z did nothing to a note that was just placed. Focus is in the note\'s own empty textarea, whose native undo stack has nothing in it — so yielding the keystroke there loses it entirely');

  // Now the other direction, on a note that HAS been typed into.
  await placeNote(0.55, 0.5);
  await page.keyboard.type('hello');
  await page.waitForFunction(() => document.activeElement?.value === 'hello');
  assert.equal(await page.evaluate(() => document.activeElement?.dataset?.nibTyped), '1',
    'setup: typing did not mark the field, so the branch below is not the one under test');

  await page.keyboard.press('Control+z');
  await page.waitForTimeout(600);
  assert.equal(await notes(), 1,
    'Ctrl+Z removed a note the user had typed into. A field that has received input owns its own undo, and taking it means the keystroke deletes the note instead of the last word');

  h.answerDialogs(true);
  await closeDoc();
});

// ── One undo order for one document ─────────────────────────────────────────
//
// Dan: *"ctrl-z works on drawing lines and it works on drawing shapes, but if I draw lines and
// then draw shapes, ctrl-z will not undo the previous set of lines."*
//
// Two client stacks: nib's overlay commands and pdf.js's annotation-editor commands. Ctrl+Z
// drained nib's first and then fell through to the SERVER ring, so the editor stack was reachable
// only while one of pdf.js's own tools happened to be armed — arm a nib tool instead and the
// drawings became unreachable. Measured before the fix: two editor changes on screen, four Ctrl+Z
// presses, nothing undone, and the Undo button reading as disabled throughout.
//
// The property is chronological: the LAST change made is the first undone, whichever stack made
// it. Server operations are deliberately outside this — each one reloads through
// setDocumentFromServer -> clearOverlays, which drops both client stacks, so "client edits, then
// server ops" is already true without bookkeeping.
//
// **A FreeText annotation stands in for the drawn line, and that is a real limitation.** No
// synthetic pointer sequence in this harness produces an ink stroke — tried as a Playwright drag
// and as hand-dispatched PointerEvents, both on the editor layer with it topmost and accepting
// events. FreeText goes through the SAME `addCommands` door on the same manager, which is what
// this test is about; what goes unexercised is ink's own path to that door.
test('Ctrl+Z walks the whole document in the order the changes were made', async () => {
  await h.openDocument(DOC, 3);
  await h.topOfDocument();
  await h.mode('markup');
  await h.group('Annotate & Draw');

  const counts = () => page.evaluate(() => {
    const layer = document.querySelector('.viewerContainer:not([hidden]) .annotationEditorLayer');
    return {
      drawings: layer ? layer.children.length : -1,
      notes: document.querySelectorAll('.viewerContainer:not([hidden]) .ovl-note').length,
    };
  });
  const arm = async (id) => {
    for (let i = 0; i < 3; i++) {
      if (await page.evaluate((b) => document.getElementById(b).classList.contains('active'), id)) return;
      await page.click(`#${id}`);
      await page.waitForTimeout(250);
    }
    throw new Error(`${id} would not arm, so the change it makes is not being made`);
  };

  // FIRST change: a text annotation, which lives in pdf.js's editor stack.
  await arm('textToolBtn');
  const layer = await page.evaluate(() => {
    const b = document.querySelector('.viewerContainer:not([hidden]) .annotationEditorLayer').getBoundingClientRect();
    return { x: b.x, y: b.y };
  });
  await page.mouse.click(layer.x + 150, layer.y + 120);
  await page.waitForTimeout(400);
  await page.keyboard.type('drawn first');
  await page.mouse.click(layer.x + 430, layer.y + 420);   // commit by clicking away
  await page.waitForTimeout(500);
  await page.click('#textToolBtn');                        // disarm
  await page.waitForFunction(() => !document.getElementById('textToolBtn').classList.contains('active'));
  const afterDrawing = await counts();
  assert.equal(afterDrawing.drawings, 1, 'setup: no annotation-editor change was made, so there is no second stack in this test');
  // This used to also assert the Undo BUTTON had noticed the drawing. The button left the toolbar
  // in v1.125.0, so there is no longer a surface that reports undoability — the keystroke is the
  // whole interface, and the ordering below is the whole claim.

  // SECOND change: a note, which lives in nib's overlay stack.
  await arm('noteBtn');
  const pt = await page.evaluate(() => {
    const b = document.querySelector('.viewerContainer:not([hidden]) .page').getBoundingClientRect();
    // Clamped into the viewport: the page is taller than the window, and a click below the fold
    // lands nowhere and silently places nothing.
    return { x: Math.round(b.x + b.width * 0.6), y: Math.round(Math.min(b.y + b.height * 0.3, window.innerHeight - 120)) };
  });
  await page.mouse.click(pt.x, pt.y);
  await page.waitForSelector('.viewerContainer:not([hidden]) .ovl-note');
  await page.evaluate(() => document.activeElement && document.activeElement.blur());
  const afterNote = await counts();
  assert.deepEqual([afterNote.drawings, afterNote.notes], [1, 1],
    'setup: the two changes are not both present, so their ORDER cannot be what is measured below');

  // Newest first: the note, then the drawing.
  await page.evaluate(() => document.body.focus());
  await page.keyboard.press('Control+z');
  await page.waitForTimeout(600);
  const first = await counts();
  assert.deepEqual([first.drawings, first.notes], [1, 0],
    `the first Ctrl+Z did not undo the NOTE, which was the last change made — it left ${first.drawings} drawing(s) and ${first.notes} note(s)`);

  await page.keyboard.press('Control+z');
  await page.waitForTimeout(600);
  const second = await counts();
  assert.deepEqual([second.drawings, second.notes], [0, 0],
    `the second Ctrl+Z did not reach the DRAWING. That is the reported defect: each stack undoes its own changes and the older stack is never reached, so a document holds edits no keystroke can take back — ${second.drawings} drawing(s) left`);

  h.answerDialogs(true);
  await closeDoc();
});

// ── The title in the bar, and whether it needs saving ───────────────────────
//
// The bar answers "which file am I editing, and have I saved it" without being asked. The save
// dot reads `hasUnsavedWork` — the same flag the close prompt asks — so the two cannot disagree,
// which they did on the first attempt: the universal document sink marks every arrival dirty and
// `installOpened` corrects it a line later, so a freshly opened document showed "Unsaved changes"
// until the flag was given one door (`setDirty`).
//
// Tier 3 because the claim is a rendered chrome element following real state through a real open,
// a real server operation and a real save.
test('the toolbar names the open document and says whether it is saved', async () => {
  const state = () => page.evaluate(() => {
    const t = document.getElementById('docTitle');
    return {
      hidden: t.hidden,
      name: document.getElementById('docTitleName').textContent,
      save: document.getElementById('docDirty').getAttribute('aria-label'),
    };
  });

  const before = await state();
  assert.equal(before.hidden, true,
    'setup: the title is showing with no document open, so "it appears on open" proves nothing');

  await h.openDocument(DOC, 3);
  const opened = await state();
  assert.equal(opened.hidden, false, 'no document title appears in the toolbar after opening one');
  assert.match(opened.name, /\.pdf$/, `the toolbar names "${opened.name}" rather than the file that was opened`);
  assert.equal(opened.save, 'Saved',
    'a freshly opened document reads as having unsaved changes. Nothing has been edited — the flag is set by the sink every arrival goes through and corrected a line later, so a display painted at the wrong moment says the opposite of what the close prompt would');

  await h.mode('edit');
  await h.group('Rotate All Pages');
  await page.click('#rotateRightBtn');
  await page.waitForFunction(() => document.getElementById('docDirty').getAttribute('aria-label') === 'Unsaved changes',
    null, { timeout: 20000 });

  await page.click('#saveBtn');
  await page.waitForFunction(() => document.getElementById('toast')?.textContent === 'Saved', null, { timeout: 20000 });
  await page.waitForFunction(() => document.getElementById('docDirty').getAttribute('aria-label') === 'Saved',
    null, { timeout: 20000 });

  h.answerDialogs(true);
  await closeDoc();
  assert.equal((await state()).hidden, true,
    'the title survives the document being closed, naming a file that is no longer open');
});
