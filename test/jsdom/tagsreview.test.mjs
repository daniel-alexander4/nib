// The autotagger's review — `PLAN-accessibility.md` P08.S06c.
//
// ── What only this tier can see ──────────────────────────────────────────────
// The server tests prove a review is applied and refused correctly. This proves the review a PERSON
// builds is the review that is sent: that retyping, ignoring and moving an element are exactly what
// the commit carries, that every control is an ordinary keyboard-reachable one, and that a refusal
// leaves the review open with the server's reason instead of losing the work.
//
// What it cannot see: a real page's geometry (the stub viewer has no rendered text), so the outline's
// POSITION is tier 3's; this asserts only that focusing a row draws one on the element's page.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const OPEN = {
  id: 'tags:1', name: 'doc.pdf', path: '/tmp/nib-harness/doc.pdf',
  canSave: true, canUndo: false, canRedo: false, signature: { state: 'unsigned' },
};
const BOX = [0, 0, 612, 792];
const proposal = () => ({
  elements: [
    { id: 0, role: 'H1', page: 1, text: 'A title', list: -1, rect: [72, 700, 300, 720], pageBox: BOX },
    { id: 1, role: 'P', page: 1, text: 'An opening paragraph', list: -1, rect: [72, 650, 540, 690], pageBox: BOX },
    { id: 2, role: 'LI', page: 1, text: '• first item', marker: '•', list: 0, rect: [72, 620, 200, 640], pageBox: BOX },
    { id: 3, role: 'P', page: 1, text: 'A closing paragraph', list: -1, rect: [72, 590, 200, 610], pageBox: BOX },
    { id: 4, role: 'P', page: 2, text: 'A paragraph on the second page', list: -1, rect: [72, 700, 300, 720], pageBox: BOX },
  ],
  unsupported: [{ page: 2, reason: 'text sits side by side on one baseline' }],
  noText: [3],
});

let commitBody = null;
let commitReply = null;
// What the server holds. Empty at boot — a boot that finds documents restores them as views — and set
// to the open document before the refusal, because a 409 makes the client reconcile its tabs against
// this list, and the server really does still hold a document it refused to tag.
let held = { docs: [], activeId: '' };

const { document: doc, settle, calls } = await boot({
  routes: {
    '/api/docs': () => held,
    '/api/open': OPEN,
    '/api/scan': { hidden: [] },
    '/api/tags/propose': proposal,
    '/api/tags/commit': (opts) => {
      commitBody = JSON.parse(opts.body);
      return commitReply ? commitReply() : { ...OPEN, canUndo: true };
    },
  },
});

async function openDoc() {
  setNextDocument({ numPages: 3 });
  doc.getElementById('pathInput').value = '/tmp/nib-harness/doc.pdf';
  doc.getElementById('openGo').click();
  await settle();
}

async function propose() {
  doc.getElementById('tagsBtn').click();
  await settle();
  await settle();
}

const rows = () => [...doc.querySelectorAll('#tagsList .tags-row')];
const rowFor = (id) => rows().find((r) => r.dataset.id === String(id));

test('the button proposes, and the review opens with one row per element and the page notes', async () => {
  await openDoc();
  await propose();
  assert.ok(calls.some((c) => c.url.includes('/api/tags/propose')), 'the button did not ask the server for a proposal');
  assert.equal(doc.getElementById('tagsModal').hidden, false, 'the review did not open');
  assert.equal(rows().length, 5, 'a row per proposed element');
  assert.deepEqual(rows().map((r) => r.querySelector('select').value), ['H1', 'P', 'LI', 'P', 'P'], 'each row shows its proposed role');
  const summary = doc.getElementById('tagsSummary').textContent;
  assert.match(summary, /Nothing is written until you commit/, `the summary does not say nothing is written yet: "${summary}"`);
  assert.match(summary, /Page 2: text sits side by side/, 'the unsupported page is not named');
  assert.match(summary, /page 3/, 'the page with no text is not named');
});

test('every control in a row is an ordinary one a keyboard can reach', async () => {
  for (const row of rows()) {
    const controls = [...row.querySelectorAll('select, input, button')];
    assert.ok(controls.length >= 5, `row ${row.dataset.id} has ${controls.length} control(s)`);
    for (const c of controls) {
      assert.ok(['SELECT', 'INPUT', 'BUTTON'].includes(c.tagName), `a ${c.tagName} is not a native control`);
      assert.notEqual(c.tabIndex, -1, `a ${c.tagName} in row ${row.dataset.id} is taken out of the tab order`);
      assert.ok(c.getAttribute('aria-label') || c.closest('label'), `a ${c.tagName} in row ${row.dataset.id} has no accessible name`);
    }
  }
  assert.equal(rowFor(0).querySelector('.tags-up').disabled, true, 'the first row can move up');
  assert.equal(rowFor(4).querySelector('.tags-down').disabled, true, 'the last row can move down');
});

// The outline itself needs a rendered page, which the stub viewer does not have (`getPageView` is
// null) — tier 3 asserts it is drawn. What this tier CAN see is that focusing a row takes the viewer
// to the element's page, which is the half a keyboard user needs to find it.
test('focusing a row takes the viewer to its element\'s page', async () => {
  assert.notEqual(doc.querySelector('.pageNum').value, '2', 'setup: the viewer is already on page 2');
  rowFor(4).querySelector('select').dispatchEvent(new doc.defaultView.FocusEvent('focusin', { bubbles: true }));
  await settle();
  assert.equal(doc.querySelector('.pageNum').value, '2', 'focusing the row for a page-2 element did not move the viewer to page 2');
});

test('a retype, an ignore and a move are exactly what the commit sends', async () => {
  const select = rowFor(1).querySelector('select');
  select.value = 'H2';
  select.dispatchEvent(new doc.defaultView.Event('change', { bubbles: true }));
  await settle();
  rowFor(3).querySelector('input[type="checkbox"]').click();
  await settle();
  rowFor(3).querySelector('.tags-up').click();
  await settle();
  assert.deepEqual(rows().map((r) => r.dataset.id), ['0', '1', '3', '2', '4'], 'the move did not reorder the rows');
  assert.ok(rowFor(3).classList.contains('tags-ignored'), 'the ignored row is not shown as ignored');

  doc.getElementById('tagsCommit').click();
  await settle();
  await settle();
  assert.ok(commitBody, 'the commit sent nothing');
  const sent = commitBody.elements;
  assert.deepEqual(sent.map((e) => e.id), [0, 1, 3, 2, 4], 'the commit does not carry the reviewed order');
  assert.deepEqual(sent.map((e) => e.role), ['H1', 'H2', 'P', 'LI', 'P'], 'the commit does not carry the reviewed roles');
  assert.deepEqual(sent.map((e) => e.ignore), [false, false, true, false, false], 'the commit does not carry the ignore');
  assert.deepEqual(sent.map((e) => e.text), ['A title', 'An opening paragraph', 'A closing paragraph', '• first item', 'A paragraph on the second page'],
    'the commit does not echo each element\'s text, so the server cannot tell the review is stale');
  assert.equal(doc.getElementById('tagsModal').hidden, true, 'a successful commit left the review open');
  assert.equal(doc.querySelectorAll('.tag-outline').length, 0, 'closing the review left an outline on the page');
});

// The server refuses a signed document with 409 — the status every commit door answers a refusal with
// (ADR-008) — and apiFetch reconciles tabs on any 409. The document is still held, so the tab must
// survive the reconcile and the review must stay open with the server's sentence.
test('a refused commit keeps the review open and says why', async () => {
  await propose();
  held = { docs: [OPEN], activeId: OPEN.id };
  commitReply = () => new Response(JSON.stringify({ error: 'this document is signed, and adding structure would change the bytes its signatures cover' }),
    { status: 409, headers: { 'Content-Type': 'application/json' } });
  doc.getElementById('tagsCommit').click();
  await settle();
  await settle();
  await settle();
  assert.equal(doc.getElementById('tagsModal').hidden, false, 'a refusal closed the review and lost the work');
  assert.match(doc.getElementById('tagsSummary').textContent, /signed/, 'the refusal does not show the server\'s reason');
  assert.equal(doc.getElementById('tagsCommit').disabled, false, 'after a refusal the commit cannot be tried again');
  assert.ok(doc.querySelectorAll('.viewerContainer').length >= 1, 'the reconcile after the 409 dropped a document the server still holds');
  commitReply = null;
});
