// The View group: two layout modes and the zoom modes (Dan, 2026-09-10).
//
// ── Why this is a View GROUP and not a View menu ─────────────────────────────
// Dan asked for a View menu. Nib has no menu bar: `index.html`'s header says "the settings gear is
// the only chrome dropdown left", and that gear went when Settings became a mode (ADR-025). The
// only `.menu` left is `.modemenu`, a width-swap of the mode tabs. So the controls join the
// toolbar group that already held Fit width — one route, per the drift rule ADR-009 exists for.
//
// ── What only this tier can see ──────────────────────────────────────────────
// The Go tests prove the layout is stored as absence-means-default and comes back over
// `/api/status`. They cannot see which element the class lands on, whether it lands on views that
// are NOT on screen, or whether a document opened afterwards inherits it — and that last pair is
// the ADR-002 seam, where one `PDFViewer` per document is hidden rather than destroyed, so a view
// off screen when the mode changes is exactly the one that comes back wrong.
//
// jsdom resolves no layout, so nothing here asserts that pages LOOK joined; that is the class
// being present and a CSS rule tier 3 can measure.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const DOC = '/tmp/nib-harness/viewmodes.pdf';
let openPath = DOC;
let saved = null;

const h = await boot({
  routes: {
    '/api/settings': (opts) => { saved = JSON.parse((opts && opts.body) || '{}'); return { status: 'ok' }; },
    // A DIFFERENT path per call, because the second open is the whole point: the first reuses
    // the empty boot view (installOpened's own branch) and only a second document makes a second
    // view — which is the population every "reaches every view" assertion below needs.
    '/api/open': () => ({
      name: openPath.split('/').pop(), path: openPath, canSave: true,
      signature: { state: 'unsigned' }, canUndo: false, canRedo: false,
    }),
    '/api/close': () => ({}),
  },
});
const { document: doc, settle } = h;

async function openDocument(path) {
  openPath = path;
  setNextDocument({ numPages: 3, outline: null });
  doc.getElementById('pathInput').value = path;
  doc.getElementById('openGo').click();
  await settle();
}

const pageStacks = () => [...doc.querySelectorAll('.viewerContainer .pdfViewer')];
const joined = () => pageStacks().map((el) => el.classList.contains('nibJoined'));

test('the View group holds both layouts and every zoom control, in one labelled group', () => {
  const group = doc.querySelector('.tbgroup[data-label="View"]');
  assert.ok(group, 'there is no View group — the controls went somewhere without a label, and '
    + 'ADR-015 keeps group labels out of the bar, so an unlabelled control has no name at all');
  for (const id of ['viewStandardBtn', 'viewContinuousBtn', 'zoomOutBtn', 'fitBtn', 'fitPageBtn',
    'actualSizeBtn', 'zoomInBtn']) {
    assert.ok(group.querySelector('#' + id), `${id} is not in the View group`);
  }
  // The thing Dan asked for that must NOT have been built: a second navigation surface.
  const menus = [...doc.querySelectorAll('#menubar .menu')].filter((m) => !m.classList.contains('modemenu'));
  assert.equal(menus.length, 0,
    'a chrome dropdown was added to the menubar. Nib deleted its last one when Settings became a '
    + 'mode, and the Settings tab\'s own comment calls a second route to one surface "the drift '
    + 'ADR-009 exists to stop"');
});

test('the standard layout is the boot default, and it is the pressed one', () => {
  assert.equal(doc.getElementById('viewStandardBtn').getAttribute('aria-pressed'), 'true',
    'nothing is pressed at boot, so a screen reader cannot say which of a radio pair is on');
  assert.equal(doc.getElementById('viewContinuousBtn').getAttribute('aria-pressed'), 'false');
  assert.deepEqual(joined(), [false],
    'the boot view is already joined — continuous is meant to be entered deliberately, and the '
    + 'standard layout is what every existing document renders as today');
});

test('switching to continuous marks the pages and saves the choice', async () => {
  saved = null;
  doc.getElementById('viewContinuousBtn').click();
  await settle();
  assert.deepEqual(joined(), [true], 'the page stack did not get the joining class');
  assert.ok(saved, 'switching layout saved nothing — the choice would not survive a restart');
  assert.equal(saved.viewLayout, 'continuous', `the request carried ${JSON.stringify(saved.viewLayout)}`);
  assert.equal(doc.getElementById('viewContinuousBtn').getAttribute('aria-pressed'), 'true');
  assert.equal(doc.getElementById('viewStandardBtn').getAttribute('aria-pressed'), 'false',
    'both halves of the radio pair read as pressed');
});

test('a view CONSTRUCTED while continuous is on arrives continuous', async () => {
  // **This is the half `applyViewLayout` cannot do**, and it is why the class is also applied in
  // `newView`: that function walks the views that EXIST when the user switches, so a document
  // opened afterwards would arrive on the default and the setting would look like it had come
  // undone on the second file.
  //
  // **Two opens, and the count is asserted**, because the first REUSES the empty boot view
  // (installOpened's own branch) — that view already carries the class from the switch above, so
  // one open tests nothing about construction. Only the second builds a fresh page stack.
  await openDocument(DOC);
  await openDocument('/tmp/nib-harness/second.pdf');
  const stacks = pageStacks().length;
  assert.ok(stacks >= 2,
    `only ${stacks} page stack(s) exist, so no view was CONSTRUCTED during this test and the `
    + 'assertion below is about the boot view being re-marked rather than about a new one');
  assert.ok(joined().every(Boolean),
    `${joined().filter((m) => !m).length} of ${stacks} page stacks are not joined — a document `
    + 'opened after the switch arrived on the default layout');
});

test('switching back reaches a view that is NOT on screen', async () => {
  // ADR-002 hides an inactive view rather than destroying it, so the view off screen when the mode
  // changes is exactly the one that comes back wrong. Two stacks exist by now (above), and the
  // floor is re-asserted rather than assumed: a population that collapsed between tests would make
  // "every view" quantify over one again, silently.
  const stacks = pageStacks().length;
  assert.ok(stacks >= 2, `only ${stacks} page stack(s) — "every view" cannot fail with one`);

  saved = null;
  doc.getElementById('viewStandardBtn').click();
  await settle();
  assert.ok(joined().every((m) => m === false),
    'a view kept the joining class after switching back to standard — `applyViewLayout` reached '
    + 'the active view only, and the hidden one comes back on the layout the user turned off');
  assert.equal(saved && saved.viewLayout, 'pages',
    `returning to the default sent ${JSON.stringify(saved && saved.viewLayout)}; the server stores `
    + 'that as absence, which is how the default and "never set" stay one state');
});

test('presentation is a third layout, and it is never saved', async () => {
  // **Not persisted, unlike the other two**, and the assertion is on the REQUEST: presentation is
  // something you are doing now, not how you like to read. An app that reopened full screen next
  // week because of a meeting today would be wrong in a way its user cannot diagnose.
  saved = null;
  doc.getElementById('viewPresentBtn').click();
  await settle();
  assert.equal(doc.getElementById('viewPresentBtn').getAttribute('aria-pressed'), 'true',
    'the Present button does not read as pressed while presenting');
  assert.equal(doc.getElementById('viewStandardBtn').getAttribute('aria-pressed'), 'false',
    'two of a three-way radio read as pressed at once');
  assert.equal(saved, null,
    `entering presentation saved ${JSON.stringify(saved)} — it would come back on the next launch`);
  assert.ok(doc.body.classList.contains('presenting'),
    'body.presenting is not set, so the chrome that presentation exists to hide is still there');
  // **And Full screen reads FALSE here, which is the point of keeping the two axes apart.** jsdom
  // implements no Fullscreen API, so the request presentation makes cannot succeed — and the
  // button must report the window's actual state rather than inferring it from the layout. A
  // control that said "full screen" because we asked for it would be lying on every machine where
  // the request is refused, which is exactly this one.
  assert.equal(doc.getElementById('fullScreenBtn').getAttribute('aria-pressed'), 'false',
    'the Full screen button reads as pressed because presentation ASKED for full screen, not '
    + 'because the window is in it — `document.fullscreenElement` is the only thing that knows');
});

test('leaving presentation restores the previous chrome and DOES save', async () => {
  saved = null;
  doc.getElementById('viewStandardBtn').click();
  await settle();
  assert.equal(doc.body.classList.contains('presenting'), false,
    'the presenting class survived leaving presentation, so the menubar and toolbar stay hidden '
    + 'with no control left on screen to bring them back');
  assert.equal(saved && saved.viewLayout, 'pages',
    'returning to a real layout did not save it');
});

test('Full screen is a separate control from Present', () => {
  // pdf.js conflates them — its presentation mode IS the Fullscreen API — which is why Escape is
  // unpredictable in most viewers. Two controls, and the full-screen one is a toggle rather than a
  // fourth radio, because a window state is orthogonal to a page layout.
  const fs = doc.getElementById('fullScreenBtn');
  assert.ok(fs, 'there is no Full screen control, so the only way to fill the display is to present');
  assert.equal(fs.getAttribute('aria-pressed'), 'false',
    'Full screen reads as pressed when the document is not full screen — it reflects '
    + '`document.fullscreenElement`, which is the only thing that knows');
  assert.equal(doc.getElementById('viewStandardBtn').getAttribute('aria-pressed'), 'true',
    'the layout radio moved when the full-screen toggle was read; the two axes are not independent');
});
