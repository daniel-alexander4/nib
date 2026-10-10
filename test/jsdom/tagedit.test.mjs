// Correcting an existing structure tree — `PLAN-accessibility.md` P09.S06b.
//
// ── What only this tier can see ──────────────────────────────────────────────
// The server tests prove an edit is applied, refused and undone correctly. This proves the edit a PERSON
// makes is the edit that is sent: that each control in the bar sends exactly its one edit for the element
// selected, that the bar says what can and cannot be changed about that element, that after the reload an
// edit causes the person is still where they were — same element, same control — and that a refusal
// keeps them there with the server's reason.
//
// The same for the four controls of ADR-124 — Add tag, Delete this tag, Move into the tag above, Move out
// one level — and where focus lands after a create.
//
// What it cannot see: a real browser's Tab order and a select answering the keyboard, which is tier 3's
// keyboard reader for this panel.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { boot, REPO } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const OPEN = {
  id: 'edit:1', name: 'doc.pdf', path: '/tmp/nib-harness/doc.pdf',
  canSave: true, canUndo: false, canRedo: false, signature: { state: 'unsigned' },
};
const BOX = [0, 0, 612, 792];
const el = (id, parent, standard, text, kids, extra = {}) => ({
  id, parent, kind: standard, standard, page: 1, text, alt: '', hasAlt: false, scope: '', kids, rect: [72, 600, 300, 720], pageBox: BOX, ...extra,
});
const baseTree = () => ({
  tagged: true,
  unaddressable: 2,
  elements: [
    el(4, -1, 'Document', 'A titleAn opening paragraphName', [1, 2, 3, 4, 5]),
    el(5, 0, 'H1', 'A title', []),
    el(6, 0, 'P', 'An opening paragraph', [], { alt: 'kept' }),
    el(7, 0, 'TH', 'Name', [], { scope: 'Column' }),
    el(0, 0, 'P', 'An inline paragraph', []),
    // A table (ADR-119): a header row of two cells and one written inline, and a data cell headed by the first.
    el(8, 0, 'Table', 'QtyPriceTotal3', [6, 9]),
    el(9, 5, 'TR', 'QtyPriceTotal', [7, 8, 11]),
    el(10, 6, 'TH', 'Qty', []),
    el(11, 6, 'TH', 'Price', []),
    el(12, 5, 'TR', '3', [10]),
    el(13, 9, 'TD', '3', [], { colSpan: 2, rowSpan: 1, headers: [7] }),
    el(0, 6, 'TH', 'Total', []),
  ],
});
// The same tree after the paragraph (6) moved above the heading (5): what the server answers after a move.
const movedTree = () => {
  const t = baseTree();
  [t.elements[1], t.elements[2]] = [t.elements[2], t.elements[1]];
  return t;
};
let tree = baseTree();

const edits = [];
let editReply = null;

let removeReply = null;
const { document: doc, window: win, settle, calls, confirms, setConfirmAnswer } = await boot({
  routes: {
    '/api/docs': () => ({ docs: [OPEN], activeId: OPEN.id }),
    '/api/open': OPEN,
    '/api/scan': { hidden: [] },
    '/api/tags/tree': () => tree,
    '/api/tags/remove': () => (removeReply ? removeReply() : { ...OPEN, canUndo: true }),
    '/api/tags/edit': (opts) => {
      edits.push(JSON.parse(opts.body));
      return editReply ? editReply() : { ...OPEN, canUndo: true };
    },
  },
});

const items = () => [...doc.querySelectorAll('#tagTreeList [role="treeitem"]')];
const $ = (id) => doc.getElementById(id);
const treeCalls = () => calls.filter((c) => c.url.includes('/api/tags/tree')).length;

async function select(i) {
  items()[i].focus();
  await settle();
}

// press focuses a control and activates it, the way a keyboard user does — focus first, so the restore
// after the reload has a control to return to.
async function press(id) {
  setNextDocument({ numPages: 2 });
  $(id).focus();
  $(id).click();
  await settle();
  await settle();
  await settle();
}

test('selecting an element shows what can be changed about it, and says when nothing can', async () => {
  setNextDocument({ numPages: 2 });
  $('pathInput').value = OPEN.path;
  $('openGo').click();
  await settle();
  doc.querySelector('.modetab[data-tab="accessibility"]').click(); // ADR-035
  await settle();
  doc.querySelector('.tab[data-panel="tagtree"]').click();
  await settle();
  await settle();
  assert.equal(items().length, 12, 'setup: the tree did not render');
  assert.equal($('tagEditBar').hidden, true, 'the edit bar shows before anything is selected');

  await select(1);
  assert.equal($('tagEditBar').hidden, false, 'selecting an element does not show the edit bar');
  assert.equal($('tagEditType').value, 'H1', 'the type picker does not show the element\'s type');
  assert.equal($('tagEditScopeRow').hidden, true, 'a heading is offered a scope');
  assert.equal($('tagEditUp').disabled, true, 'the first of its siblings can move up');
  assert.equal($('tagEditDown').disabled, false, 'the first of several siblings cannot move down');

  await select(2);
  assert.equal($('tagEditAlt').value, 'kept', 'the alt field does not show the element\'s alt text');
  await select(3);
  assert.equal($('tagEditScopeRow').hidden, false, 'a header cell is not offered a scope');
  assert.equal($('tagEditScope').value, 'Column', 'the scope picker does not show the header cell\'s scope');

  assert.equal($('tagEditColSpanRow').hidden || $('tagEditRowSpanRow').hidden || $('tagEditSpanApply').hidden, false, 'a header cell is not offered its spans');
  assert.equal($('tagEditHeadersRow').hidden && $('tagEditHeadersApply').hidden, true, 'a cell in no table is offered header cells');
  await select(1);
  assert.equal($('tagEditColSpanRow').hidden && $('tagEditRowSpanRow').hidden && $('tagEditSpanApply').hidden, true, 'a heading is offered spans');

  await select(10);
  assert.equal($('tagEditColSpan').value, '2', 'the column span field does not show the cell\'s span');
  assert.equal($('tagEditRowSpan').value, '1', 'a cell that declares no row span does not show the 1 a reader takes');
  assert.equal($('tagEditScopeRow').hidden, true, 'a data cell is offered a scope');
  const boxes = [...$('tagEditHeaders').querySelectorAll('input[type="checkbox"]')];
  assert.deepEqual(boxes.map((b) => [b.value, b.checked, b.parentElement.textContent.trim()]),
    [['10', true, 'Qty'], ['11', false, 'Price']],
    'the header cells offered are not the table\'s own addressable TH cells, with the ones the cell names ticked');
  assert.match(items()[10].textContent, /spans 2 columns/, 'the tree does not say a cell spans columns');
  assert.match(items()[10].textContent, /headed by 1 cell/, 'the tree does not say a cell names its header');
  await select(7);
  assert.deepEqual([...$('tagEditHeaders').querySelectorAll('input')].map((b) => b.value), ['11'], 'a header cell is offered itself as its own header');

  await select(4);
  for (const id of ['tagEditType', 'tagEditTypeApply', 'tagEditAlt', 'tagEditAltApply', 'tagEditUp', 'tagEditDown', 'tagEditArtifact']) {
    assert.equal($(id).disabled, true, `${id} is enabled on an element written inline, which no edit can name`);
  }
  assert.equal($('tagEditInlineWhy').hidden, false, 'the bar does not say why an inline element cannot be edited');
  assert.match($('tagEditInlineWhy').textContent, /inline/, 'the line shown for an inline element does not say it is inline');

  await select(0);
  assert.equal($('tagEditUp').disabled || $('tagEditDown').disabled, true, 'the root, with no siblings, can move');
  assert.equal($('tagEditUp').disabled && $('tagEditDown').disabled, true, 'the root, with no siblings, can move one way');
});

test('each control sends exactly its one edit, for the element selected', async () => {
  edits.length = 0;
  await select(2);
  $('tagEditType').value = 'Figure';
  await press('tagEditTypeApply');
  await select(2);
  $('tagEditAlt').value = 'A chart';
  $('tagEditAlt').focus();
  setNextDocument({ numPages: 2 });
  $('tagEditAlt').dispatchEvent(new win.KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
  await settle();
  await settle();
  await settle();
  await select(2);
  await press('tagEditUp');
  await select(2);
  await press('tagEditDown');
  await select(3);
  $('tagEditScope').value = 'Row';
  await press('tagEditScopeApply');
  await select(10);
  $('tagEditColSpan').value = '3';
  $('tagEditRowSpan').value = '';
  await press('tagEditSpanApply');
  await select(10);
  const [qty, price] = [...$('tagEditHeaders').querySelectorAll('input')];
  qty.checked = false;
  price.checked = true;
  await press('tagEditHeadersApply');
  await select(2);
  await press('tagEditArtifact');
  assert.deepEqual(edits, [
    { edits: [{ element: 6, kind: 'retype', value: 'Figure' }] },
    { edits: [{ element: 6, kind: 'alt', value: 'A chart' }] },
    { edits: [{ element: 6, kind: 'move', parent: 0, index: 0 }] },
    { edits: [{ element: 6, kind: 'move', parent: 0, index: 2 }] },
    { edits: [{ element: 7, kind: 'scope', value: 'Row' }] },
    { edits: [{ element: 13, kind: 'colspan', value: '3' }, { element: 13, kind: 'rowspan', value: '' }] },
    { edits: [{ element: 13, kind: 'headers', headers: [11] }] },
    { edits: [{ element: 6, kind: 'artifact' }] },
  ], 'the edits sent are not the ones the controls describe');
  assert.ok(calls.filter((c) => c.url.includes('/api/tags/edit')).every((c) => {
    const h = c.headers || {};
    return (typeof h.get === 'function' ? h.get('X-Nib-Doc') : h['X-Nib-Doc']) === OPEN.id;
  }), 'an edit went out without naming its document (ADR-004)');
});

test('after an edit the tree is read again, and the person is still on the same element and control', async () => {
  await select(2);
  const before = treeCalls();
  await press('tagEditTypeApply');
  assert.ok(treeCalls() > before, 'the tree was not read again after the edit reloaded the document');
  assert.equal(doc.querySelector('#tagTreeList [aria-selected="true"]')?.dataset.id, '6', 'the edited element is not still selected');
  assert.equal(doc.activeElement?.id, 'tagEditTypeApply', 'focus did not return to the control the person used');
  assert.match($('tagEditStatus').textContent, /Ctrl\+Z/, 'the bar does not say the change can be taken back');
});

test('a refused edit keeps the person where they were and shows the server\'s reason', async () => {
  await select(1);
  const before = treeCalls();
  editReply = () => new Response(JSON.stringify({ error: '"Bogus" is not a standard structure type' }),
    { status: 400, headers: { 'Content-Type': 'application/json' } });
  await press('tagEditTypeApply');
  editReply = null;
  assert.match($('tagEditStatus').textContent, /not a standard structure type/, 'the refusal does not show the server\'s sentence');
  assert.equal(treeCalls(), before, 'a refused edit reloaded the tree');
  assert.equal(doc.querySelector('#tagTreeList [aria-selected="true"]')?.dataset.id, '5', 'a refusal moved the selection');
});

async function reopenPanel() {
  const head = doc.querySelector('.tab[data-panel="tagtree"]');
  if (head.classList.contains('active')) head.click();
  head.click();
  await settle();
  await settle();
}

test('after a move, the moved element stays selected where the tree now puts it', async () => {
  tree = baseTree();
  await reopenPanel();
  await select(2);
  assert.equal(items()[2].dataset.id, '6', 'setup: the paragraph is not third');
  tree = movedTree();
  await press('tagEditUp');
  const selected = items().findIndex((i) => i.getAttribute('aria-selected') === 'true');
  assert.equal(items()[selected]?.dataset.id, '6', 'the selection stayed on the old position instead of following the moved element');
  assert.equal(selected, 1, 'the moved element is not selected at its new position');
  tree = baseTree();
});

test('a tree that re-reads as untagged takes the edit bar away with it', async () => {
  await reopenPanel();
  await select(1);
  assert.equal($('tagEditBar').hidden, false, 'setup: the bar is not showing');
  tree = { tagged: false, unaddressable: 0, elements: [] };
  await reopenPanel();
  assert.equal($('tagEditBar').hidden, true, 'the edit bar still offers changes to an element the tree no longer has');
  assert.equal($('tagTreeRemove').hidden, true, 'a document with no tree is offered the removal of its tags');
  tree = baseTree();
  await reopenPanel();
});

test('a refused edit leaves nothing behind for the next read of the tree to act on', async () => {
  await select(1);
  editReply = () => new Response(JSON.stringify({ error: 'refused' }), { status: 409, headers: { 'Content-Type': 'application/json' } });
  await press('tagEditTypeApply');
  editReply = null;
  await reopenPanel();
  assert.doesNotMatch($('tagEditStatus').textContent, /Ctrl\+Z/, 'a read of the tree after a REFUSED edit announces the change as made');
});

// Removing the whole tree (ADR-120): asked for by name, confirmed, pinned to its document, and a refusal is
// shown where the tree's summary is.
test('Remove all tags asks first, sends nothing when declined, and re-reads the tree when it is done', async () => {
  await reopenPanel();
  assert.equal($('tagTreeRemove').hidden, false, 'a tagged document is not offered the removal of its tags');
  const sent = () => calls.filter((c) => c.url.includes('/api/tags/remove'));
  const asked = confirms.length;
  setConfirmAnswer(false);
  $('tagTreeRemove').click();
  await settle();
  assert.equal(confirms.length, asked + 1, 'removing every tag did not ask first');
  assert.match(confirms[asked], /Ctrl\+Z brings the tags back/, 'the question does not say the removal can be taken back');
  assert.equal(sent().length, 0, 'a declined removal was sent');
  setConfirmAnswer(true);

  removeReply = () => new Response(JSON.stringify({ error: 'this document is signed, and removing its tags would change the bytes its signatures cover' }),
    { status: 409, headers: { 'Content-Type': 'application/json' } });
  const before = treeCalls();
  $('tagTreeRemove').click();
  await settle();
  await settle();
  removeReply = null;
  assert.match($('tagTreeSummary').textContent, /signed/, 'a refused removal does not show the server\'s reason');
  assert.equal(treeCalls(), before, 'a refused removal reloaded the tree');

  tree = { tagged: false, unaddressable: 0, elements: [] };
  setNextDocument({ numPages: 2 });
  $('tagTreeRemove').click();
  await settle();
  await settle();
  await settle();
  const [, done] = sent();
  assert.equal(done.method, 'POST', 'the removal is not a POST');
  const h = done.headers || {};
  assert.equal(typeof h.get === 'function' ? h.get('X-Nib-Doc') : h['X-Nib-Doc'], OPEN.id, 'the removal went out without naming its document (ADR-004)');
  assert.ok(treeCalls() > before, 'the tree was not read again after the removal');
  assert.match($('tagTreeSummary').textContent, /no structure tree/, 'the panel does not say the document is now untagged');
  assert.equal($('tagTreeRemove').hidden, true, 'the removal is still offered once the tags are gone');
  tree = baseTree();
  await reopenPanel();
});

// ── Adding a tag, deleting one, and the two moves that change a parent — ADR-124 ────────────────────────
//
// What the server answers after a create: the tree with one more element, at `at` in the flat order,
// under the element at index `parent` (-1: the top) as that parent's `place`-th kid.
function withNewTag(at, parent, place, id = 20, standard = 'Sect') {
  const t = baseTree();
  const shift = (j) => (j >= at ? j + 1 : j);
  for (const e of t.elements) {
    e.parent = e.parent >= 0 ? shift(e.parent) : e.parent;
    e.kids = e.kids.map(shift);
  }
  t.elements.splice(at, 0, el(id, parent, standard, '', [], { rect: [0, 0, 0, 0], page: 0 }));
  if (parent >= 0) t.elements[parent].kids.splice(place, 0, at);
  return t;
}
const lastEdit = () => edits[edits.length - 1];
const selectedItem = () => doc.querySelector('#tagTreeList [aria-selected="true"]');

test('Add tag creates the chosen type after the selected element, and the new tag takes the selection and the focus', async () => {
  tree = baseTree();
  await reopenPanel();
  assert.equal($('tagNewBar').hidden, false, 'a tagged document is not offered a new tag');
  assert.equal($('tagNewType').value, 'Sect', 'the new-tag picker does not open on a container');
  assert.equal($('tagNewAdd').getAttribute('aria-describedby'), 'tagNewHelp', 'the Add tag button does not carry its help');
  assert.match($('tagNewHelp').textContent, /after the selected one/, 'the help does not say where a new tag goes');

  await select(2); // the paragraph, third of the Document's kids
  edits.length = 0;
  $('tagNewType').value = 'Div';
  tree = withNewTag(3, 0, 2);
  const before = treeCalls();
  await press('tagNewAdd');
  assert.deepEqual(edits, [{ edits: [{ kind: 'create', value: 'Div', parent: 4, index: 2 }] }],
    'the create sent is not "a Div under the Document, directly after the paragraph"');
  assert.ok(treeCalls() > before, 'the tree was not read again after the create');
  assert.equal(selectedItem()?.dataset.id, '20', 'the new tag is not selected');
  assert.equal(doc.activeElement, items()[3], 'focus did not land on the new tag in the tree');
  assert.match($('tagEditStatus').textContent, /Tag added.*Ctrl\+Z/, 'the bar does not say the tag was added and can be taken back');

  // A top-level element selected: the new tag is its next sibling at the top of the tree, which -1 names.
  tree = baseTree();
  await reopenPanel();
  await select(0);
  tree = withNewTag(12, -1, 0);
  await press('tagNewAdd');
  assert.deepEqual(lastEdit(), { edits: [{ kind: 'create', value: 'Div', parent: -1, index: 1 }] },
    'a create beside a top-level element does not name the top of the tree and the place after it');
  assert.equal(selectedItem()?.dataset.id, '20', 'the new top-level tag is not selected');
  assert.equal(doc.activeElement, items()[12], 'focus did not land on the new top-level tag');
});

test('with nothing selected, Add tag puts the new tag last at the top of the tree, and a refusal shows beside the summary', async () => {
  tree = baseTree();
  await reopenPanel();
  assert.equal(selectedItem(), null, 'setup: something is selected after a fresh read');
  editReply = () => new Response(JSON.stringify({ error: 'this document is signed, and correcting its structure would change the bytes its signatures cover' }),
    { status: 409, headers: { 'Content-Type': 'application/json' } });
  const before = treeCalls();
  await press('tagNewAdd');
  editReply = null;
  assert.match($('tagTreeSummary').textContent, /signed/, 'a create refused with nothing selected does not show the server\'s reason');
  assert.equal(treeCalls(), before, 'a refused create reloaded the tree');

  tree = withNewTag(12, -1, 0);
  await press('tagNewAdd');
  assert.deepEqual(lastEdit(), { edits: [{ kind: 'create', value: 'Div', parent: -1 }] },
    'a create with nothing selected does not append at the top of the tree');
  assert.equal(selectedItem()?.dataset.id, '20', 'the new tag is not selected');
  assert.equal(doc.activeElement, items()[12], 'focus did not land on the new tag');
  assert.match($('tagTreeSummary').textContent, /13 element/, 'the summary was not rewritten once the tree was read again');
});

test('Delete this tag sends a delete — never an artifact — and says the content is kept', async () => {
  tree = baseTree();
  await reopenPanel();
  assert.match($('tagEditDelete').textContent, /keep its content/, 'the delete button does not say the content is kept');
  assert.equal($('tagEditDelete').getAttribute('aria-describedby'), 'tagEditDeleteHelp', 'the delete button does not carry its help');
  assert.match($('tagEditDeleteHelp').textContent, /move up to the tag above/, 'the help does not say where the content goes');
  assert.match($('tagEditDeleteHelp').textContent, /Mark as decoration/, 'the help does not tell a delete from Mark as decoration');
  await select(2);
  edits.length = 0;
  await press('tagEditDelete');
  assert.deepEqual(edits, [{ edits: [{ element: 6, kind: 'delete' }] }], 'the edit sent is not a delete of the selected element');

  await select(1);
  editReply = () => new Response(JSON.stringify({ error: 'element 5 is at the top of the structure tree and holds content itself — change its type instead' }),
    { status: 400, headers: { 'Content-Type': 'application/json' } });
  const before = treeCalls();
  await press('tagEditDelete');
  editReply = null;
  assert.match($('tagEditStatus').textContent, /change its type instead/, 'a refused delete does not show the server\'s sentence');
  assert.equal(treeCalls(), before, 'a refused delete reloaded the tree');
  assert.equal(selectedItem()?.dataset.id, '5', 'a refused delete moved the selection');
});

test('Move into the tag above and Move out one level are the move edit, naming the new parent', async () => {
  tree = baseTree();
  await reopenPanel();
  edits.length = 0;
  await select(2); // P, after the H1
  await press('tagEditIn');
  await select(7); // the first TH: in a row, in the table
  await press('tagEditOut');
  await select(6); // the first row: in the table, in the Document
  await press('tagEditOut');
  await select(1); // the H1: in the Document, which is at the top
  await press('tagEditOut');
  assert.deepEqual(edits, [
    { edits: [{ element: 6, kind: 'move', parent: 5 }] },
    { edits: [{ element: 10, kind: 'move', parent: 8, index: 1 }] },
    { edits: [{ element: 9, kind: 'move', parent: 4, index: 5 }] },
    { edits: [{ element: 5, kind: 'move', parent: -1, index: 1 }] },
  ], 'the moves sent are not: last into the previous sibling; next after the parent, under the grandparent or at the top');
});

test('a move that cannot be made is disabled and says why, where a screen reader finds it', async () => {
  tree = baseTree();
  await reopenPanel();
  const why = (id) => ($(`${id}Why`).hidden ? '' : $(`${id}Why`).textContent);
  for (const id of ['tagEditIn', 'tagEditOut']) {
    assert.equal($(id).getAttribute('aria-describedby'), `${id}Why`, `${id} does not name the line that says why it is disabled`);
  }
  await select(2);
  assert.deepEqual([$('tagEditIn').disabled, $('tagEditOut').disabled, why('tagEditIn'), why('tagEditOut')], [false, false, '', ''],
    'an element with a tag above it and a parent cannot move in and out, or a reason is shown for a move that can be made');
  await select(1); // first of its siblings
  assert.equal($('tagEditIn').disabled, true, 'the first of its siblings can move into a tag above it');
  assert.match(why('tagEditIn'), /no tag above/, 'the bar does not say why it cannot move in');
  await select(5); // the Table: the tag above it is written inline
  assert.equal($('tagEditIn').disabled, true, 'an element can be moved into one written inline');
  assert.match(why('tagEditIn'), /written inline/, 'the bar does not say the tag above is inline');
  await select(0); // top level
  assert.equal($('tagEditOut').disabled, true, 'a top-level element can move out');
  assert.match(why('tagEditOut'), /already at the top level/, 'the bar does not say why it cannot move out');
  await select(4); // itself inline: nothing here can name it
  for (const id of ['tagEditIn', 'tagEditOut', 'tagEditDelete']) {
    assert.equal($(id).disabled, true, `${id} is enabled on an element written inline, which no edit can name`);
  }
  assert.deepEqual([why('tagEditIn'), why('tagEditOut')], ['', ''], 'an inline element is given a second reason beside the bar\'s own');
  assert.equal($('tagEditInlineWhy').hidden, false, 'the bar does not say why an inline element cannot be edited');
  assert.match($('tagEditInlineWhy').textContent, /inline/, 'the line shown for an inline element does not say it is inline');

  // A Document holding an INLINE section, which holds a paragraph, which holds a span.
  tree = {
    tagged: true,
    unaddressable: 1,
    elements: [el(4, -1, 'Document', 'x', [1]), el(0, 0, 'Sect', 'x', [2]), el(6, 1, 'P', 'x', [3]), el(7, 2, 'Span', 'x', [])],
  };
  await reopenPanel();
  await select(2);
  assert.equal($('tagEditOut').disabled, true, 'an element can be moved out of a parent written inline');
  assert.match(why('tagEditOut'), /inside one written inline/, 'the bar does not say its parent is inline');
  edits.length = 0;
  await press('tagNewAdd');
  assert.deepEqual(edits, [], 'a create beside an element whose parent is inline was sent, naming a parent with no id');
  assert.match($('tagEditStatus').textContent, /inside one written inline/, 'the bar does not say why a tag cannot be added there');
  await select(3);
  assert.equal($('tagEditOut').disabled, true, 'an element can be moved out into a tag written inline');
  assert.match(why('tagEditOut'), /level above is a tag written inline/, 'the bar does not say the level above is inline');

  tree = { tagged: false, unaddressable: 0, elements: [] };
  await reopenPanel();
  assert.equal($('tagNewBar').hidden, true, 'a document with no tree is offered a new tag');
  tree = baseTree();
  await reopenPanel();
});

test('the types offered are exactly the types the server accepts', () => {
  const src = fs.readFileSync(path.join(REPO, 'internal', 'pdfops', 'structedit.go'), 'utf8');
  const block = src.match(/var standardStructTypes = map\[string\]bool\{([\s\S]*?)\n\}/);
  assert.ok(block, 'standardStructTypes is not in structedit.go in the shape this scan reads — the guard is reading nothing');
  const server = [...block[1].matchAll(/"([A-Za-z0-9]+)": true/g)].map((m) => m[1]).sort();
  assert.ok(server.length > 40, `the scan found ${server.length} server types — the regex is probably wrong`);
  // Every element selected in the tests above is a standard type, so the picker holds only its own list.
  const offered = [...$('tagEditType').options].map((o) => o.value).sort();
  assert.deepEqual(offered, server, 'the type picker and pdfops.standardStructTypes disagree — one will offer a type the other refuses');
  assert.deepEqual([...$('tagNewType').options].map((o) => o.value).sort(), server,
    'the new-tag picker and pdfops.standardStructTypes disagree — it will offer a type a create refuses');
});
