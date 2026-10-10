// Tagging what no tag owns, from the structure tree panel — ADR-125.
//
// ── What only this tier can see ──────────────────────────────────────────────
// The server tests prove a region edit takes the right pieces and refuses the rest. This proves the edit a
// PERSON makes is the edit that is sent: that the list shows what the server listed, in the server's words
// for decoration and for a form's content; that a tick sends nothing; that **Tag selected** sends ONE region
// edit naming exactly the ticked pieces, the page they were read from, the chosen type and where the new tag
// goes; that the new tag takes the selection and the focus, and the page's list is read again; and that a
// refusal keeps the person's ticks and shows the server's sentence.
//
// What it cannot see: the outline a tick draws on the page (the stub viewer has no page to draw on) and a
// real browser's Tab order — tier 3's keyboard reader for this section.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { boot, REPO } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const OPEN = {
  id: 'untagged:1', name: 'doc.pdf', path: '/tmp/nib-harness/doc.pdf',
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
    el(4, -1, 'Document', 'A titleAn opening paragraph', [1, 2, 3]),
    el(5, 0, 'H1', 'A title', []),
    el(6, 0, 'P', 'An opening paragraph', []),
    el(0, 0, 'Sect', 'inline', [4]),
    el(7, 3, 'P', 'inside an inline section', []),
  ],
});
// The tree after a region made element 20 the Document's kid at `place`.
const withRegion = (place, standard = 'P') => {
  const t = baseTree();
  const at = place + 1;
  const shift = (j) => (j >= at ? j + 1 : j);
  for (const e of t.elements) {
    e.parent = e.parent >= 0 ? shift(e.parent) : e.parent;
    e.kids = e.kids.map(shift);
  }
  t.elements.splice(at, 0, el(20, 0, standard, 'A paragraph nobody tagged', []));
  t.elements[0].kids.splice(place, 0, at);
  return t;
};
const LONG = 'A paragraph nobody tagged, which runs on for a good deal longer than the label has room to show';
const pieces = () => ([
  { page: 2, kind: 'text', text: LONG, rect: [0.1, 0.1, 0.6, 0.15], decoration: false, inForm: false },
  { page: 2, kind: 'text', text: 'Page 2 of 9', rect: [0.4, 0.94, 0.6, 0.96], decoration: true, inForm: false },
  { page: 2, kind: 'image', text: '', rect: [0.1, 0.3, 0.3, 0.4], decoration: false, inForm: false },
  { page: 2, kind: 'drawing', text: '', rect: [0.5, 0.3, 0.8, 0.5], decoration: true, inForm: false },
  { page: 2, kind: 'text', text: 'A watermark', rect: [0.2, 0.4, 0.8, 0.6], decoration: true, inForm: true },
  { page: 2, kind: 'rule', text: '', rect: [0.1, 0.2, 0.9, 0.201], decoration: false, inForm: false },
  { page: 2, kind: 'box', text: '', rect: [0.1, 0.6, 0.3, 0.7], decoration: false, inForm: false },
]);

let tree = baseTree();
let untagged = () => ({ pages: 9, pieces: pieces() });
const edits = [];
let editReply = null;

const { document: doc, window: win, settle, calls } = await boot({
  routes: {
    '/api/docs': () => ({ docs: [OPEN], activeId: OPEN.id }),
    '/api/open': OPEN,
    '/api/scan': { hidden: [] },
    '/api/tags/tree': () => tree,
    '/api/tags/untagged': () => untagged(),
    '/api/tags/edit': (opts) => {
      edits.push(JSON.parse(opts.body));
      return editReply ? editReply() : { ...OPEN, canUndo: true };
    },
  },
});

const $ = (id) => doc.getElementById(id);
const items = () => [...doc.querySelectorAll('#tagTreeList [role="treeitem"]')];
const boxes = () => [...$('tagUntaggedList').querySelectorAll('input[type="checkbox"]')];
const untaggedCalls = () => calls.filter((c) => c.url.includes('/api/tags/untagged'));
const selectedItem = () => doc.querySelector('#tagTreeList [aria-selected="true"]');

async function press(id) {
  setNextDocument({ numPages: 9 });
  $(id).focus();
  $(id).click();
  await settle();
  await settle();
  await settle();
}
async function reopenPanel() {
  const head = doc.querySelector('.tab[data-panel="tagtree"]');
  if (head.classList.contains('active')) head.click();
  head.click();
  await settle();
  await settle();
}
async function show(page) {
  $('tagUntaggedPage').value = String(page);
  await press('tagUntaggedShow');
}

test('a tagged document is offered the section, and nothing is listed until it is asked for', async () => {
  setNextDocument({ numPages: 9 });
  $('pathInput').value = OPEN.path;
  $('openGo').click();
  await settle();
  doc.querySelector('.modetab[data-tab="accessibility"]').click(); // ADR-035
  await settle();
  doc.querySelector('.tab[data-panel="tagtree"]').click();
  await settle();
  await settle();
  assert.equal(items().length, 5, 'setup: the tree did not render');
  assert.equal($('tagUntaggedBar').hidden, false, 'a tagged document is not offered its untagged content');
  assert.equal($('tagUntaggedPieces').hidden, true, 'a list is shown before a page was asked for');
  assert.equal(untaggedCalls().length, 0, 'the page was read before anyone asked');
  assert.equal($('tagUntaggedBar').getAttribute('role'), 'group', 'the section is not a group');
  assert.equal($($('tagUntaggedBar').getAttribute('aria-labelledby'))?.textContent, 'Tag untagged content', 'the group is not named by its heading');
  assert.equal($('tagUntaggedShow').getAttribute('aria-describedby'), 'tagUntaggedHelp', 'the button does not carry the section\'s help');
  assert.equal($('tagUntaggedType').value, 'P', 'the type chooser does not open on a paragraph');
  assert.match($('tagUntaggedAlt').parentElement.textContent, /required for a figure/, 'the description field does not say a figure needs it');
  // After the edit bar: no Tab stop is added between the tree and the controls for the selected tag.
  const order = [...$('tagtree').children].map((c) => c.id);
  assert.ok(order.indexOf('tagUntaggedBar') > order.indexOf('tagEditBar'), 'the section sits before the edit bar, adding Tab stops between the tree and it');

  tree = { tagged: false, unaddressable: 0, elements: [] };
  await reopenPanel();
  assert.equal($('tagUntaggedBar').hidden, true, 'a document with no tree is offered a region, which needs a tree');
  tree = baseTree();
  await reopenPanel();
});

test('Show untagged content lists what the server listed, as labelled tick boxes', async () => {
  await show(2);
  const [call] = untaggedCalls().slice(-1);
  assert.match(call.url, /\/api\/tags\/untagged\?page=2$/, 'the page asked for is not the page in the field');
  const h = call.headers || {};
  assert.equal(typeof h.get === 'function' ? h.get('X-Nib-Doc') : h['X-Nib-Doc'], OPEN.id, 'the read went out without naming its document (ADR-004)');
  assert.equal($('tagUntaggedPieces').hidden, false, 'the list is not shown');
  assert.equal($('tagUntaggedLegend').textContent, 'Untagged on page 2', 'the list does not say which page it is of');
  assert.equal($('tagUntaggedList').tagName, 'UL', 'the pieces are not a list');
  assert.deepEqual(boxes().map((b) => [b.parentElement.tagName, b.parentElement.textContent.trim(), b.disabled, b.checked]), [
    ['LABEL', `Text — “${LONG.slice(0, 50)}…”`, false, false],
    ['LABEL', 'Text — “Page 2 of 9” (marked as decoration)', false, false],
    ['LABEL', 'Picture', false, false],
    ['LABEL', 'Drawing (marked as decoration)', false, false],
    ['LABEL', 'Text — “A watermark” (marked as decoration; drawn inside a form, so it cannot be tagged here)', true, false],
    ['LABEL', 'Rule', false, false],
    ['LABEL', 'Box', false, false],
  ], 'the tick boxes are not the pieces the server listed, each labelled with what it is');
  assert.match($('tagUntaggedStatus').textContent, /7 piece\(s\) on page 2 have no tag/, 'the status does not count the pieces');
  assert.equal($('tagUntaggedPage').max, '9', 'the page field does not know how many pages there are');
});

test('a tick sends nothing, and Tag selected with nothing ticked says so', async () => {
  edits.length = 0;
  boxes()[0].checked = true;
  boxes()[0].dispatchEvent(new win.Event('change', { bubbles: true }));
  $('tagUntaggedType').value = 'H2';
  $('tagUntaggedType').dispatchEvent(new win.Event('change', { bubbles: true }));
  await settle();
  assert.deepEqual(edits, [], 'a tick or a change of type sent an edit — a button applies');
  boxes()[0].checked = false;
  await press('tagUntaggedApply');
  assert.deepEqual(edits, [], 'an edit was sent with nothing ticked');
  assert.match($('tagUntaggedStatus').textContent, /Tick at least one piece/, 'the status does not say what to do first');
  $('tagUntaggedType').value = 'P';
});

test('Tag selected sends one region naming exactly the ticked pieces, after the selected tag, and the new tag takes the focus', async () => {
  items()[1].focus(); // the H1, first of the Document's kids
  await settle();
  boxes()[0].checked = true;
  boxes()[2].checked = true;
  edits.length = 0;
  tree = withRegion(1);
  untagged = () => ({ pages: 9, pieces: pieces().filter((_, i) => i !== 0 && i !== 2) });
  const before = untaggedCalls().length;
  await press('tagUntaggedApply');
  assert.deepEqual(edits, [{ edits: [{ kind: 'region', value: 'P', page: 2, pieces: [[0.1, 0.1, 0.6, 0.15], [0.1, 0.3, 0.3, 0.4]], parent: 4, index: 1 }] }],
    'the edit sent is not one region of the two ticked pieces, under the Document, directly after the heading');
  assert.equal(selectedItem()?.dataset.id, '20', 'the new tag is not selected');
  assert.equal(doc.activeElement, items()[2], 'focus did not land on the new tag in the tree');
  assert.equal(untaggedCalls().length, before + 1, 'the page\'s list was not read again after the region');
  assert.match(untaggedCalls().slice(-1)[0].url, /page=2$/, 'the list read again is not the same page\'s');
  assert.equal(boxes().length, 5, 'the list still shows what was just tagged');
  assert.match($('tagUntaggedStatus').textContent, /Tagged — Ctrl\+Z takes it back\. 5 piece/, 'the status does not say the region was tagged and what is left');
  assert.ok(calls.filter((c) => c.url.includes('/api/tags/edit')).every((c) => {
    const h = c.headers || {};
    return (typeof h.get === 'function' ? h.get('X-Nib-Doc') : h['X-Nib-Doc']) === OPEN.id;
  }), 'the region went out without naming its document (ADR-004)');
});

test('a description is sent when one is given, and with nothing selected the new tag goes last at the top', async () => {
  tree = baseTree();
  untagged = () => ({ pages: 9, pieces: pieces() });
  await reopenPanel();
  assert.equal(selectedItem(), null, 'setup: something is selected after a fresh read');
  assert.equal($('tagUntaggedPieces').hidden, true, 'a list read from the document as it was is still shown after the tree was read again');
  await show(2);
  boxes()[3].checked = true;
  $('tagUntaggedType').value = 'Figure';
  $('tagUntaggedAlt').value = '  A chart rising to the right ';
  edits.length = 0;
  await press('tagUntaggedApply');
  assert.deepEqual(edits, [{ edits: [{ kind: 'region', value: 'Figure', page: 2, pieces: [[0.5, 0.3, 0.8, 0.5]], parent: -1, index: -1, alt: 'A chart rising to the right' }] }],
    'the figure sent does not carry its description, or is not placed last at the top of the tree');
  $('tagUntaggedAlt').value = '';
  $('tagUntaggedType').value = 'P';
});

test('a refusal shows the server\'s sentence, keeps the ticks and reads nothing again', async () => {
  tree = baseTree();
  await reopenPanel();
  await show(2);
  boxes()[1].checked = true;
  editReply = () => new Response(JSON.stringify({ error: 'the region cuts through content page 2 marks as one piece of decoration (1 of its 2 pieces are in the region) — widen the region to take all of it' }),
    { status: 400, headers: { 'Content-Type': 'application/json' } });
  const reads = untaggedCalls().length;
  const trees = calls.filter((c) => c.url.includes('/api/tags/tree')).length;
  await press('tagUntaggedApply');
  editReply = null;
  assert.match($('tagUntaggedStatus').textContent, /widen the region to take all of it/, 'a refused region does not show the server\'s sentence');
  assert.equal(boxes()[1].checked, true, 'a refused region lost the person\'s ticks');
  assert.equal(untaggedCalls().length, reads, 'a refused region read the list again');
  assert.equal(calls.filter((c) => c.url.includes('/api/tags/tree')).length, trees, 'a refused region reloaded the tree');
  assert.equal(doc.activeElement, $('tagUntaggedApply'), 'a refused region took the focus from the button');

  // A tag inside one written inline is selected: a new tag cannot be placed beside it, and nothing is sent.
  items()[4].focus();
  await settle();
  edits.length = 0;
  await press('tagUntaggedApply');
  assert.deepEqual(edits, [], 'a region beside an element whose parent is inline was sent, naming a parent with no id');
  assert.match($('tagUntaggedStatus').textContent, /inside one written inline/, 'the status does not say why the tag cannot be placed there');
});

test('a page that cannot be read says so, an empty page says nothing is untagged, and an empty field means the page on screen', async () => {
  untagged = () => new Response(JSON.stringify({ error: 'page 12 is not a page of this document, which has 9' }), { status: 400, headers: { 'Content-Type': 'application/json' } });
  await show(12);
  assert.match($('tagUntaggedStatus').textContent, /page 12 is not a page of this document/, 'the server\'s reason is not shown');
  assert.equal($('tagUntaggedPieces').hidden, true, 'a list is shown for a page that could not be read');

  untagged = () => ({ pages: 9, pieces: [] });
  await show(3);
  assert.equal($('tagUntaggedStatus').textContent, 'Nothing on page 3 is untagged.', 'an empty page is not said to be fully tagged');
  assert.equal($('tagUntaggedPieces').hidden, true, 'an empty list is shown');

  const reads = untaggedCalls().length;
  await show(0);
  assert.equal(untaggedCalls().length, reads, 'page 0 was asked of the server');
  assert.match($('tagUntaggedStatus').textContent, /number of the page/, 'a page that is not a number is not explained');

  $('tagUntaggedPage').value = '';
  $('tagUntaggedPage').focus();
  $('tagUntaggedPage').dispatchEvent(new win.KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
  await settle();
  await settle();
  const [last] = untaggedCalls().slice(-1);
  assert.match(last.url, /page=\d+$/, 'Enter in the empty page field did not ask for the page on screen');
  assert.equal($('tagUntaggedPage').value, last.url.match(/page=(\d+)$/)[1], 'the field does not show the page that was read');
  untagged = () => ({ pages: 9, pieces: pieces() });
});

test('a list that arrives after the tree was read again is dropped', async () => {
  let release;
  untagged = () => new Promise((resolve) => { release = () => resolve({ pages: 9, pieces: pieces() }); });
  $('tagUntaggedPage').value = '2';
  $('tagUntaggedShow').click();
  await settle();
  await reopenPanel(); // the tree is read again while the list is still on its way
  release();
  await settle();
  await settle();
  assert.equal(boxes().length, 0, 'a list read before the tree was read again was drawn over the fresh panel');
  assert.equal($('tagUntaggedPieces').hidden, true, 'the stale list is shown');
  untagged = () => ({ pages: 9, pieces: pieces() });
});

test('the types offered are exactly the types the server accepts', () => {
  const src = fs.readFileSync(path.join(REPO, 'internal', 'pdfops', 'structedit.go'), 'utf8');
  const block = src.match(/var standardStructTypes = map\[string\]bool\{([\s\S]*?)\n\}/);
  assert.ok(block, 'standardStructTypes is not in structedit.go in the shape this scan reads — the guard is reading nothing');
  const server = [...block[1].matchAll(/"([A-Za-z0-9]+)": true/g)].map((m) => m[1]).sort();
  assert.ok(server.length > 40, `the scan found ${server.length} server types — the regex is probably wrong`);
  assert.deepEqual([...$('tagUntaggedType').options].map((o) => o.value).sort(), server,
    'the region\'s type chooser and pdfops.standardStructTypes disagree — it will offer a type a region refuses');
});
