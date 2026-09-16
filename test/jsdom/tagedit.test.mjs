// Correcting an existing structure tree — `PLAN-accessibility.md` P09.S06b.
//
// ── What only this tier can see ──────────────────────────────────────────────
// The server tests prove an edit is applied, refused and undone correctly. This proves the edit a PERSON
// makes is the edit that is sent: that each control in the bar sends exactly its one edit for the element
// selected, that the bar says what can and cannot be changed about that element, that after the reload an
// edit causes the person is still where they were — same element, same control — and that a refusal
// keeps them there with the server's reason.
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
  unaddressable: 1,
  elements: [
    el(4, -1, 'Document', 'A titleAn opening paragraphName', [1, 2, 3, 4]),
    el(5, 0, 'H1', 'A title', []),
    el(6, 0, 'P', 'An opening paragraph', [], { alt: 'kept' }),
    el(7, 0, 'TH', 'Name', [], { scope: 'Column' }),
    el(0, 0, 'P', 'An inline paragraph', []),
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

const { document: doc, window: win, settle, calls } = await boot({
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
  assert.equal(items().length, 5, 'setup: the tree did not render');
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

  await select(4);
  for (const id of ['tagEditType', 'tagEditTypeApply', 'tagEditAlt', 'tagEditAltApply', 'tagEditUp', 'tagEditDown', 'tagEditArtifact']) {
    assert.equal($(id).disabled, true, `${id} is enabled on an element written inline, which no edit can name`);
  }
  assert.match($('tagEditStatus').textContent, /inline/, 'the bar does not say why an inline element cannot be edited');

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
  await select(2);
  await press('tagEditArtifact');
  assert.deepEqual(edits, [
    { edits: [{ element: 6, kind: 'retype', value: 'Figure' }] },
    { edits: [{ element: 6, kind: 'alt', value: 'A chart' }] },
    { edits: [{ element: 6, kind: 'move', parent: 0, index: 0 }] },
    { edits: [{ element: 6, kind: 'move', parent: 0, index: 2 }] },
    { edits: [{ element: 7, kind: 'scope', value: 'Row' }] },
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

test('the types offered are exactly the types the server accepts', () => {
  const src = fs.readFileSync(path.join(REPO, 'internal', 'pdfops', 'structedit.go'), 'utf8');
  const block = src.match(/var standardStructTypes = map\[string\]bool\{([\s\S]*?)\n\}/);
  assert.ok(block, 'standardStructTypes is not in structedit.go in the shape this scan reads — the guard is reading nothing');
  const server = [...block[1].matchAll(/"([A-Za-z0-9]+)": true/g)].map((m) => m[1]).sort();
  assert.ok(server.length > 40, `the scan found ${server.length} server types — the regex is probably wrong`);
  // Every element selected in the tests above is a standard type, so the picker holds only its own list.
  const offered = [...$('tagEditType').options].map((o) => o.value).sort();
  assert.deepEqual(offered, server, 'the type picker and pdfops.standardStructTypes disagree — one will offer a type the other refuses');
});
