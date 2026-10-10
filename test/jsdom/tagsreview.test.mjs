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

// A proposal with a table in it (ADR-121): the Table, then each row and its cells directly after it, each
// naming its parent. One cell is empty.
const tableProposal = () => ({
  elements: [
    { id: 0, role: 'P', page: 1, text: 'Above the table', list: -1, parent: -1, rect: [72, 730, 300, 745], pageBox: BOX },
    { id: 1, role: 'Table', page: 1, text: 'Name Qty Apple', list: -1, parent: -1, rect: [100, 640, 400, 700], pageBox: BOX },
    { id: 2, role: 'TR', page: 1, text: 'Name Qty', list: -1, parent: 1, rect: [100, 670, 400, 700], pageBox: BOX },
    { id: 3, role: 'TH', page: 1, text: 'Name', list: -1, parent: 2, rect: [100, 670, 250, 700], pageBox: BOX },
    { id: 4, role: 'TH', page: 1, text: 'Qty', list: -1, parent: 2, rect: [250, 670, 400, 700], pageBox: BOX },
    { id: 5, role: 'TR', page: 1, text: 'Apple', list: -1, parent: 1, rect: [100, 640, 400, 670], pageBox: BOX },
    { id: 6, role: 'TD', page: 1, text: 'Apple', list: -1, parent: 5, rect: [100, 640, 250, 670], pageBox: BOX },
    { id: 7, role: 'TD', page: 1, text: '', list: -1, parent: 5, rect: [250, 640, 400, 670], pageBox: BOX },
    { id: 8, role: 'P', page: 1, text: 'Below the table', list: -1, parent: -1, rect: [72, 600, 300, 615], pageBox: BOX },
  ],
  unsupported: [],
  noText: [],
});
// A proposal with two figures in it (ADR-122): a Figure has no text, and its rect is its image's box.
const figureProposal = () => ({
  elements: [
    { id: 0, role: 'P', page: 1, text: 'Above the picture', list: -1, parent: -1, rect: [72, 730, 300, 745], pageBox: BOX },
    { id: 1, role: 'Figure', page: 1, text: '', list: -1, parent: -1, rect: [100, 500, 300, 700], pageBox: BOX },
    { id: 2, role: 'P', page: 1, text: 'Between the pictures', list: -1, parent: -1, rect: [72, 470, 300, 485], pageBox: BOX },
    { id: 3, role: 'Figure', page: 2, text: '', list: -1, parent: -1, rect: [100, 300, 300, 450], pageBox: BOX },
  ],
  unsupported: [],
  noText: [],
});
let nextProposal = proposal;

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
    '/api/tags/propose': () => nextProposal(),
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

// ── A proposed table (ADR-121) ───────────────────────────────────────────────
// The server refuses a review that parts a table, retypes its rows or ignores one cell. This proves the
// card cannot BUILD such a review: the controls that would are not there, and a move takes the table whole.
test('a table is shown nested, and each of its rows offers only what a reviewer may do to it', async () => {
  nextProposal = tableProposal;
  await propose();
  assert.equal(rows().length, 9, 'a row per proposed element, the table\'s rows and cells included');
  const depth = (id) => (rowFor(id).classList.contains('tags-sub2') ? 2 : rowFor(id).classList.contains('tags-sub1') ? 1 : 0);
  assert.deepEqual([0, 1, 2, 3, 4, 5, 6, 7, 8].map(depth), [0, 0, 1, 2, 2, 1, 2, 2, 0], 'rows and cells are not indented under their table');

  // The table has one choice — it is a table, or it is not one and its cells are paragraphs — and its rows
  // say what they are and offer none.
  assert.deepEqual([...rowFor(1).querySelectorAll('option')].map((o) => o.value), ['Table', 'P'], 'the table does not offer "not a table"');
  assert.equal(rowFor(1).querySelector('select').value, 'Table', 'a proposed table does not start as a table');
  for (const [id, name] of [[2, 'Table row'], [5, 'Table row']]) {
    assert.equal(rowFor(id).querySelector('select'), null, `element ${id} offers a choice of type`);
    assert.equal(rowFor(id).querySelector('.tags-kind').textContent, name, `element ${id} does not say what it is`);
  }
  // A cell is a header cell or a data cell and nothing else; everything else keeps the usual choices.
  for (const id of [3, 4, 6, 7]) {
    assert.deepEqual([...rowFor(id).querySelectorAll('option')].map((o) => o.value), ['TH', 'TD'], `cell ${id} offers other types`);
  }
  assert.deepEqual([3, 4, 6, 7].map((id) => rowFor(id).querySelector('select').value), ['TH', 'TH', 'TD', 'TD'], 'a cell does not show its proposed type');
  assert.equal([...rowFor(0).querySelectorAll('option')].some((o) => o.value === 'TH' || o.value === 'Table'), false, 'a paragraph can be made part of a table');
  assert.match(rowFor(7).querySelector('.tags-text').textContent, /empty cell/, 'an empty cell is a row with no words');

  // Ignore and the moves are the table's, never a row's or a cell's.
  for (const id of [2, 3, 4, 5, 6, 7]) {
    assert.equal(rowFor(id).querySelector('input[type="checkbox"], .tags-up, .tags-down'), null, `element ${id} can be ignored or moved by itself`);
    assert.ok(rowFor(id).querySelector('.tags-show'), `element ${id} cannot be shown on the page`);
  }
  assert.ok(rowFor(1).querySelector('input[type="checkbox"]') && rowFor(1).querySelector('.tags-up') && rowFor(1).querySelector('.tags-down'),
    'the table cannot be ignored or moved');
  for (const row of rows()) {
    for (const c of row.querySelectorAll('select, input, button')) {
      assert.notEqual(c.tabIndex, -1, `a ${c.tagName} in row ${row.dataset.id} is taken out of the tab order`);
      assert.ok(c.getAttribute('aria-label') || c.closest('label'), `a ${c.tagName} in row ${row.dataset.id} has no accessible name`);
    }
  }
});

test('a table moves whole, and a neighbour moves past the whole of it', async () => {
  rowFor(1).querySelector('.tags-down').click();
  await settle();
  assert.deepEqual(rows().map((r) => r.dataset.id), ['0', '8', '1', '2', '3', '4', '5', '6', '7'], 'moving the table down did not take its rows and cells');
  assert.equal(rowFor(1).querySelector('.tags-down').disabled, true, 'a table whose last cell is the last row can still move down');
  rowFor(1).querySelector('.tags-up').click();
  await settle();
  assert.deepEqual(rows().map((r) => r.dataset.id), ['0', '1', '2', '3', '4', '5', '6', '7', '8'], 'moving the table up one place did not pass the paragraph whole');
  assert.equal(doc.activeElement, rowFor(1).querySelector('.tags-up'), 'the move lost the keyboard\'s place');
  rowFor(1).querySelector('.tags-up').click();
  await settle();
  assert.deepEqual(rows().map((r) => r.dataset.id), ['1', '2', '3', '4', '5', '6', '7', '0', '8'], 'moving the table up did not take its rows and cells');
  assert.equal(rowFor(1).querySelector('.tags-up').disabled, true, 'the first element can move up');
  // The paragraph after the table moves past all of it, not into it.
  rowFor(0).querySelector('.tags-up').click();
  await settle();
  assert.deepEqual(rows().map((r) => r.dataset.id), ['0', '1', '2', '3', '4', '5', '6', '7', '8'], 'a paragraph moved up into the table');
  rowFor(0).querySelector('.tags-down').click();
  await settle();
  assert.deepEqual(rows().map((r) => r.dataset.id), ['1', '2', '3', '4', '5', '6', '7', '0', '8'], 'a paragraph moved down into the table');
});

test('a table said not to be one shows its cells as paragraphs and sends the table as P', async () => {
  const pick = rowFor(1).querySelector('select');
  pick.value = 'P';
  pick.dispatchEvent(new doc.defaultView.Event('change', { bubbles: true }));
  await settle();
  assert.equal(doc.activeElement, rowFor(1).querySelector('select'), 'declining the table lost the keyboard\'s place');
  assert.equal(rowFor(1).querySelector('select').value, 'P', 'the table\'s row no longer offers the choice it just took');
  // Its cells offer nothing now: one with text will be a paragraph, the empty one and the rows nothing.
  assert.deepEqual([2, 3, 4, 5, 6, 7].map((id) => rowFor(id).querySelector('select')), [null, null, null, null, null, null], 'a cell of a declined table still offers header or data');
  assert.deepEqual([2, 3, 7].map((id) => rowFor(id).querySelector('.tags-kind').textContent), ['—', 'Paragraph', '—'], 'the rows do not say what a declined table\'s parts become');
  commitBody = null;
  commitReply = null;
  doc.getElementById('tagsCommit').click();
  await settle();
  await settle();
  const sentRole = Object.fromEntries(commitBody.elements.map((e) => [e.id, e.role]));
  assert.deepEqual([0, 1, 2, 3, 4, 5, 6, 7, 8].map((id) => sentRole[id]), ['P', 'P', 'TR', 'TH', 'TH', 'TR', 'TD', 'TD', 'P'], 'the commit does not send the table as P with its rows and cells as proposed');
  assert.deepEqual(commitBody.elements.map((e) => e.ignore), Array(9).fill(false), 'declining a table ignored something');
  // And back: the choice is still there, and the cells get theirs again.
  const again = rowFor(1).querySelector('select');
  again.value = 'Table';
  again.dispatchEvent(new doc.defaultView.Event('change', { bubbles: true }));
  await settle();
  assert.deepEqual([...rowFor(3).querySelectorAll('option')].map((o) => o.value), ['TH', 'TD'], 'a table taken back does not give its cells their choice again');
});

test('a retyped cell and an ignored table are what the commit sends', async () => {
  const select = rowFor(6).querySelector('select');
  select.value = 'TH';
  select.dispatchEvent(new doc.defaultView.Event('change', { bubbles: true }));
  await settle();
  assert.equal(doc.activeElement, rowFor(6).querySelector('select'), 'retyping a cell lost the keyboard\'s place');
  assert.match(doc.getElementById('tagsSummary').textContent, /9 element\(s\) proposed, 9 kept/, 'the summary miscounts a table');
  rowFor(1).querySelector('input[type="checkbox"]').click();
  await settle();
  // Ignoring the table ignores what is under it: shown on every row of it, and counted.
  for (const id of [1, 2, 3, 4, 5, 6, 7]) assert.ok(rowFor(id).classList.contains('tags-ignored'), `element ${id} of an ignored table is not shown as ignored`);
  assert.equal(rowFor(0).classList.contains('tags-ignored'), false, 'ignoring the table ignored its neighbour');
  assert.match(doc.getElementById('tagsSummary').textContent, /9 element\(s\) proposed, 2 kept/, 'an ignored table\'s rows and cells are counted as kept');

  commitBody = null;
  commitReply = null;
  doc.getElementById('tagsCommit').click();
  await settle();
  await settle();
  assert.ok(commitBody, 'the commit sent nothing');
  const sent = commitBody.elements;
  assert.deepEqual(sent.map((e) => e.id), [1, 2, 3, 4, 5, 6, 7, 0, 8], 'the commit does not carry the reviewed order');
  assert.deepEqual(sent.map((e) => e.role), ['Table', 'TR', 'TH', 'TH', 'TR', 'TH', 'TD', 'P', 'P'], 'the commit does not carry the table\'s roles');
  // Only the table carries the ignore: the server refuses a row or a cell ignored by itself.
  assert.deepEqual(sent.map((e) => e.ignore), [true, false, false, false, false, false, false, false, false], 'the ignore is not the table\'s alone');
  assert.deepEqual(Object.keys(sent[0]).sort(), ['alt', 'id', 'ignore', 'role', 'text'], 'the review\'s shape changed');
  assert.deepEqual(sent.map((e) => e.alt), Array(9).fill(''), 'an element that is not a figure was sent with a description');
  assert.equal(sent[6].text, '', 'an empty cell does not echo its empty text');
  nextProposal = proposal;
});

// ── A proposed figure (ADR-122) ──────────────────────────────────────────────
// The server refuses a kept figure with no description, and a figure made anything else. This proves the
// card asks for the description where the figure is, never offers another type, keeps what was typed through
// every re-render, and sends it.
const type = (input, value) => {
  input.value = value;
  input.dispatchEvent(new doc.defaultView.Event('input', { bubbles: true }));
};

test('a figure says its type, offers no other, and asks for a description by name', async () => {
  nextProposal = figureProposal;
  await propose();
  assert.equal(rows().length, 4, 'a row per proposed element, the figures included');
  for (const id of [1, 3]) {
    const row = rowFor(id);
    assert.equal(row.querySelector('select'), null, `figure ${id} offers a choice of type`);
    assert.equal(row.querySelector('.tags-kind').textContent, 'Figure', `figure ${id} does not say what it is`);
    const alt = row.querySelector('input.tags-alt');
    assert.ok(alt, `figure ${id} has no field for its description`);
    assert.equal(alt.type, 'text');
    assert.match(alt.getAttribute('aria-label'), new RegExp(`Description of the picture on page ${id === 1 ? 1 : 2}`), `figure ${id}'s field does not name what it is for`);
    assert.equal(alt.value, '', 'a proposed figure arrives with a description nobody wrote');
    assert.ok(row.querySelector('input[type="checkbox"]') && row.querySelector('.tags-up') && row.querySelector('.tags-down') && row.querySelector('.tags-show'),
      `figure ${id} cannot be ignored, moved or shown`);
    assert.match(row.querySelector('.tags-text').textContent, /a picture/, 'a figure\'s row does not say it is a picture');
    for (const c of row.querySelectorAll('select, input, button')) {
      assert.notEqual(c.tabIndex, -1, `a ${c.tagName} in figure ${id}'s row is taken out of the tab order`);
      assert.ok(c.getAttribute('aria-label') || c.closest('label'), `a ${c.tagName} in figure ${id}'s row has no accessible name`);
    }
  }
  for (const id of [0, 2]) {
    assert.equal(rowFor(id).querySelector('.tags-alt'), null, `paragraph ${id} asks for a description`);
    assert.equal([...rowFor(id).querySelectorAll('option')].some((o) => o.value === 'Figure'), false, `paragraph ${id} can be made a figure`);
  }
  const summary = doc.getElementById('tagsSummary').textContent;
  assert.match(summary, /2 figure\(s\) still need a description/, `the summary does not count the figures with no description: "${summary}"`);
  assert.equal(doc.getElementById('tagsCommit').disabled, false, 'a figure with no description disables the commit, so the server\'s reason is never seen');
});

test('a description survives typing, a re-render and a move, and the count follows it', async () => {
  const field = rowFor(1).querySelector('.tags-alt');
  field.focus();
  type(field, 'A bar chart of sales (2024)');
  await settle();
  assert.equal(rowFor(1).querySelector('.tags-alt'), field, 'typing rebuilt the field, which loses the caret');
  assert.equal(doc.activeElement, field, 'typing lost the keyboard\'s place');
  assert.match(doc.getElementById('tagsSummary').textContent, /1 figure\(s\) still need a description/, 'the count did not follow the description');
  type(field, '   ');
  await settle();
  assert.match(doc.getElementById('tagsSummary').textContent, /2 figure\(s\) still need a description/, 'a description of spaces counts as one');
  type(field, 'A bar chart of sales (2024)');
  await settle();

  // A re-render: retyping the paragraph above rebuilds every row.
  const select = rowFor(0).querySelector('select');
  select.value = 'H1';
  select.dispatchEvent(new doc.defaultView.Event('change', { bubbles: true }));
  await settle();
  assert.equal(rowFor(1).querySelector('.tags-alt').value, 'A bar chart of sales (2024)', 'a re-render lost the description');
  // A move.
  rowFor(1).querySelector('.tags-down').click();
  await settle();
  assert.deepEqual(rows().map((r) => r.dataset.id), ['0', '2', '1', '3'], 'the figure did not move');
  assert.equal(rowFor(1).querySelector('.tags-alt').value, 'A bar chart of sales (2024)', 'a move lost the description');
  assert.equal(doc.activeElement, rowFor(1).querySelector('.tags-down'), 'the move lost the keyboard\'s place');
  assert.equal(rowFor(3).querySelector('.tags-alt').value, '', 'one figure\'s description appeared on the other');

  // Ignoring the other figure: it no longer needs one, and the keyboard stays on its box.
  rowFor(3).querySelector('input[type="checkbox"]').click();
  await settle();
  assert.equal(doc.activeElement, rowFor(3).querySelector('input[type="checkbox"]'), 'ignoring a figure moved the keyboard to its description');
  assert.equal(rowFor(3).querySelector('.tags-alt').disabled, true, 'an ignored figure still asks for a description');
  assert.doesNotMatch(doc.getElementById('tagsSummary').textContent, /still need a description/, 'an ignored figure is counted as needing a description');
});

test('the commit carries each figure\'s description, and a refusal for a missing one is shown', async () => {
  commitBody = null;
  commitReply = null;
  rowFor(3).querySelector('input[type="checkbox"]').click(); // kept again, with no description
  await settle();
  held = { docs: [OPEN], activeId: OPEN.id };
  commitReply = () => new Response(JSON.stringify({ error: 'a figure needs a description of what it shows, or must be ignored (element 3, page 2)' }),
    { status: 400, headers: { 'Content-Type': 'application/json' } });
  doc.getElementById('tagsCommit').click();
  await settle();
  await settle();
  await settle();
  assert.deepEqual(commitBody.elements.map((e) => [e.id, e.role, e.ignore, e.alt]),
    [[0, 'H1', false, ''], [2, 'P', false, ''], [1, 'Figure', false, 'A bar chart of sales (2024)'], [3, 'Figure', false, '']],
    'the commit does not carry the figures\' descriptions in the reviewed order');
  assert.equal(doc.getElementById('tagsModal').hidden, false, 'a refusal closed the review and lost the descriptions');
  assert.match(doc.getElementById('tagsSummary').textContent, /a figure needs a description of what it shows/, 'the refusal does not show the server\'s sentence');
  assert.equal(rowFor(1).querySelector('.tags-alt').value, 'A bar chart of sales (2024)', 'a refusal lost the description already written');
  assert.equal(doc.getElementById('tagsCommit').disabled, false, 'after a refusal the commit cannot be tried again');

  type(rowFor(3).querySelector('.tags-alt'), 'Ünïcode \\ (parens)');
  commitReply = null;
  commitBody = null;
  doc.getElementById('tagsCommit').click();
  await settle();
  await settle();
  assert.equal(commitBody.elements.find((e) => e.id === 3).alt, 'Ünïcode \\ (parens)', 'the description sent is not the one typed');
  assert.equal(doc.getElementById('tagsModal').hidden, true, 'a successful commit left the review open');
  nextProposal = proposal;
});
