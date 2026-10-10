// Making inline tags editable — ADR-126.
//
// ── What only this tier can see ──────────────────────────────────────────────
// The Go tests prove a `promote` edit numbers every inline element and changes nothing a reader reads. This
// proves the way a PERSON reaches it: that an inline tag's edit bar is not a dead end — it says why nothing
// can be changed and offers the one button — that the button sends exactly one edit naming no element, that
// after the re-read the tag at the same PLACE is selected, focused and live (the tag has a new number, so
// place is the only identity), that the line and the button are gone once no tag is inline, and that the
// reasons the two moves and Add tag give for an inline neighbour name the button.
//
// What it cannot see: a real browser's Tab order, and the real server's tree after the edit — tier 3's
// tagpromote.test.mjs.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const OPEN = {
  id: 'promote:1', name: 'doc.pdf', path: '/tmp/nib-harness/doc.pdf',
  canSave: true, canUndo: false, canRedo: false, signature: { state: 'unsigned' },
};
const BOX = [0, 0, 612, 792];
const el = (id, parent, standard, text, kids) => ({
  id, parent, kind: standard, standard, page: 1, text, alt: '', hasAlt: false, scope: '', kids,
  colSpan: 1, rowSpan: 1, headers: [], rect: [72, 600, 300, 720], pageBox: BOX,
});
// A Document holding a heading, an INLINE section (which holds a paragraph), and a closing paragraph.
const inlineTree = () => ({
  tagged: true, unaddressable: 1, roleMap: [],
  elements: [
    el(4, -1, 'Document', 'TitleInsideAfter', [1, 2, 4]),
    el(5, 0, 'H1', 'Title', []),
    el(0, 0, 'Sect', 'Inside', [3]),
    el(6, 2, 'P', 'Inside', []),
    el(7, 0, 'P', 'After', []),
  ],
});
// The same tree as the server answers it after a promote: the section has a number, nothing else moved.
const promotedTree = () => {
  const t = inlineTree();
  t.unaddressable = 0;
  t.elements[2].id = 31;
  return t;
};
let tree = inlineTree();
const edits = [];
let editReply = null;

const { document: doc, settle } = await boot({
  routes: {
    '/api/docs': () => ({ docs: [OPEN], activeId: OPEN.id }),
    '/api/open': OPEN,
    '/api/scan': { hidden: [] },
    '/api/tags/tree': () => tree,
    '/api/tags/edit': (opts) => {
      edits.push(JSON.parse(opts.body));
      return editReply ? editReply() : { ...OPEN, canUndo: true };
    },
  },
});

const $ = (id) => doc.getElementById(id);
const items = () => [...doc.querySelectorAll('#tagTreeList [role="treeitem"]')];
const selected = () => items().findIndex((li) => li.getAttribute('aria-selected') === 'true');
const shown = (id) => !$(id).hidden;
const BAR = ['tagEditType', 'tagEditTypeApply', 'tagEditAlt', 'tagEditAltApply', 'tagEditUp', 'tagEditDown', 'tagEditArtifact', 'tagEditDelete'];

async function select(i) {
  items()[i].focus();
  await settle();
}
async function press(id) {
  setNextDocument({ numPages: 2 });
  $(id).focus();
  $(id).click();
  await settle();
  await settle();
  await settle();
}

test('an inline tag says why it cannot be changed and offers the one button; a numbered tag shows neither', async () => {
  setNextDocument({ numPages: 2 });
  $('pathInput').value = OPEN.path;
  $('openGo').click();
  await settle();
  doc.querySelector('.modetab[data-tab="accessibility"]').click(); // ADR-035
  await settle();
  doc.querySelector('.tab[data-panel="tagtree"]').click();
  await settle();
  await settle();
  assert.equal(items().length, 5, 'setup: the tree did not render');
  assert.match($('tagTreeSummary').textContent, /1 written inline cannot be changed yet.*Make inline tags editable/,
    'the summary does not say a tag is inline and name the button that ends it');

  await select(1);
  assert.deepEqual([shown('tagEditInlineWhy'), shown('tagEditPromote'), shown('tagEditPromoteHelp')], [false, false, false],
    'a tag with a number is shown the inline line or its button');
  assert.equal($('tagEditType').disabled, false, 'setup: a numbered tag cannot be retyped');

  await select(2);
  assert.deepEqual([shown('tagEditInlineWhy'), shown('tagEditPromote'), shown('tagEditPromoteHelp')], [true, true, true],
    'an inline tag is not shown the line that says why and the button');
  assert.equal($('tagEditInlineWhy').textContent,
    'This tag is written inline in the file, so it cannot be changed until it is given a number of its own.',
    'the line is not the sentence the design gives');
  assert.equal($('tagEditPromote').textContent, 'Make inline tags editable', 'the button is not named for what it does');
  assert.equal($('tagEditPromote').disabled, false, 'the one button an inline tag has is disabled');
  assert.deepEqual($('tagEditPromote').getAttribute('aria-describedby').split(' '), ['tagEditInlineWhy', 'tagEditPromoteHelp'],
    'the button is not described by the line that says why and the line that says what it does');
  assert.match($('tagEditPromoteHelp').textContent, /Ctrl\+Z takes it back as one step/, 'the button does not say it is one undo step');
  for (const id of BAR) assert.equal($(id).disabled, true, `${id} is enabled on a tag no edit can name`);
  assert.equal($('tagEditStatus').textContent, '', 'the status repeats the reason the line above it already gives');
});

test('the reasons an inline neighbour gives name the button', async () => {
  await select(4); // the closing paragraph: the tag above it is the inline section
  assert.equal($('tagEditIn').disabled, true, 'a tag can be moved into one written inline');
  assert.match($('tagEditInWhy').textContent, /written inline.*Make inline tags editable/, 'Move into the tag above does not point at the button');
  await select(3); // the paragraph inside the inline section
  assert.equal($('tagEditOut').disabled, true, 'a tag can be moved out of one written inline');
  assert.match($('tagEditOutWhy').textContent, /inside one written inline.*Make inline tags editable/, 'Move out one level does not point at the button');
  edits.length = 0;
  await press('tagNewAdd');
  assert.deepEqual(edits, [], 'a create beside a tag whose parent is inline was sent');
  assert.match($('tagEditStatus').textContent, /inside one written inline.*Make inline tags editable/, 'Add tag does not point at the button');
});

test('a refusal leaves the tag where it was, with the server\'s sentence', async () => {
  await select(2);
  edits.length = 0;
  editReply = () => new Response(JSON.stringify({ error: 'this document is signed, and correcting its structure would change the bytes its signatures cover' }),
    { status: 409, headers: { 'Content-Type': 'application/json' } });
  await press('tagEditPromote');
  editReply = null;
  assert.equal(edits.length, 1, 'the button did not send');
  assert.match($('tagEditStatus').textContent, /this document is signed/, 'the server\'s sentence is not shown');
  assert.equal(selected(), 2, 'a refusal moved the selection');
  assert.equal(shown('tagEditPromote'), true, 'a refusal took the button away');
});

test('the button sends one promote edit naming nothing, and the tag at the same place comes back selected, focused and live', async () => {
  await select(2);
  edits.length = 0;
  tree = promotedTree();
  await press('tagEditPromote');
  assert.deepEqual(edits, [{ edits: [{ kind: 'promote' }] }], 'the button did not send exactly one promote edit that names no element');
  assert.equal(items().length, 5, 'the tree was not read again');
  assert.equal(selected(), 2, 'the tag at the same place is not selected');
  assert.equal(doc.activeElement, items()[2], 'the tag at the same place is not focused');
  assert.equal(items()[2].dataset.id, '31', 'the selected tag is not the one the server numbered');
  assert.deepEqual([shown('tagEditInlineWhy'), shown('tagEditPromote'), shown('tagEditPromoteHelp')], [false, false, false],
    'the inline line or its button is still shown for a tag that has a number');
  for (const id of ['tagEditType', 'tagEditTypeApply', 'tagEditAlt', 'tagEditAltApply', 'tagEditArtifact', 'tagEditDelete']) {
    assert.equal($(id).disabled, false, `${id} is still disabled on the tag that was just given a number`);
  }
  assert.match($('tagEditStatus').textContent, /one step.*Ctrl\+Z takes it back/, 'the status does not say this is one undo step');
  assert.doesNotMatch($('tagTreeSummary').textContent, /inline/, 'the summary still counts an inline tag');

  // And it is now a tag like any other: its type is changed by the edit that names its new number.
  edits.length = 0;
  $('tagEditType').value = 'Art';
  await press('tagEditTypeApply');
  assert.deepEqual(edits, [{ edits: [{ element: 31, kind: 'retype', value: 'Art' }] }], 'the promoted tag is not named by its new number');
});
