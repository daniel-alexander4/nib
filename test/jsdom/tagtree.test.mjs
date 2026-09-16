// The structure tree panel — `PLAN-accessibility.md` P09.S06a.
//
// ── What only this tier can see ──────────────────────────────────────────────
// The server tests prove the tree route reads a document's tree and changes nothing. This proves the
// panel a PERSON uses shows that tree: that it is reachable in Document mode without becoming its
// landing surface, that it is an ARIA tree with one tab stop and levels, that the arrow keys walk it the
// way the tree pattern says, that focusing an element takes the viewer to its page, and that it follows
// the document on screen — reloaded after an undo, re-read on a switch, and never overwritten by a late
// answer for the document the user left (ADR-001).
//
// What it cannot see: a rendered page (the stub viewer has none), so the outline's POSITION is tier 3's;
// and a real browser's Tab order, which is P02's keyboard reader's.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const meta = (id, name) => ({
  id, name, path: `/tmp/nib-harness/${name}`,
  canSave: true, canUndo: true, canRedo: false, signature: { state: 'unsigned' },
});
const ONE = meta('tree:1', 'one.pdf');
const TWO = meta('tree:2', 'two.pdf');
const BOX = [0, 0, 612, 792];
const el = (id, parent, standard, page, text, kids, extra = {}) => ({
  id, parent, kind: standard, standard, page, text, alt: '', hasAlt: false, scope: '', kids, rect: [72, 600, 300, 720], pageBox: BOX, ...extra,
});
const treeOne = () => ({
  tagged: true,
  unaddressable: 1,
  elements: [
    el(4, -1, 'Document', 1, 'A titleNameQty', [1, 2, 3]),
    el(5, 0, 'H1', 1, 'A title', [], { kind: 'Heading 1' }),
    el(6, 0, 'Figure', 2, '', [], { rect: [0, 0, 0, 0] }),
    el(7, 0, 'Table', 1, 'NameQty', [4]),
    el(8, 3, 'TH', 1, 'Name', [], { scope: 'Column' }),
  ],
});
const TREE_TWO = {
  tagged: true,
  unaddressable: 0,
  elements: [el(20, -1, 'Document', 1, 'The second document', [1]), el(21, 0, 'P', 1, 'The second document', [])],
};

let tree = treeOne();
let opening = ONE;
// A held answer for the first document's tree: set to a promise to make that request slow.
let holdOne = null;
const docOf = (opts) => {
  const h = opts.headers || {};
  return typeof h.get === 'function' ? h.get('X-Nib-Doc') : h['X-Nib-Doc'];
};

const { document: doc, window: win, settle, calls } = await boot({
  routes: {
    '/api/docs': () => ({ docs: [], activeId: '' }),
    '/api/open': () => opening,
    '/api/scan': { hidden: [] },
    '/api/undo': () => ({ ...ONE, canUndo: false }),
    '/api/tags/tree': (opts) => {
      if (docOf(opts) === TWO.id) return TREE_TWO;
      return holdOne ? holdOne.then(() => tree) : tree;
    },
  },
});

const items = () => [...doc.querySelectorAll('#tagTreeList [role="treeitem"]')];
const head = () => doc.querySelector('.tab[data-panel="tagtree"]');
const treeCalls = () => calls.filter((c) => c.url.includes('/api/tags/tree')).length;
const key = async (k) => {
  doc.getElementById('tagTreeList').dispatchEvent(new win.KeyboardEvent('keydown', { key: k, bubbles: true }));
  await settle();
};
const focused = () => items().indexOf(doc.activeElement);

async function openDoc(m, pages) {
  opening = m;
  setNextDocument({ numPages: pages });
  doc.getElementById('pathInput').value = m.path;
  doc.getElementById('openGo').click();
  await settle();
  await settle();
}

async function openPanel() {
  if (head().classList.contains('active')) head().click(); // close, so the next click is an open
  head().click();
  await settle();
  await settle();
}

test('the panel is offered in Accessibility mode, second, and loads the active document\'s tree when opened', async () => {
  await openDoc(ONE, 3);
  // Accessibility mode since ADR-035; it was Document/`edit` until then, and the panel moved with
  // the two buttons that were in Page Functions and Secure.
  doc.querySelector('.modetab[data-tab="accessibility"]').click();
  await settle();
  assert.equal(head().hidden, false, 'Accessibility mode does not offer the structure tree panel');
  assert.equal(doc.getElementById('tagtree').classList.contains('active'), false,
    'Accessibility mode LANDED on the tree panel — its landing surface is its commands, and the tree is second');
  const before = treeCalls();
  await openPanel();
  assert.ok(treeCalls() > before, 'opening the panel did not read the tree');
  assert.equal(items().length, 5, 'one tree item per element');
  assert.deepEqual(items().map((i) => i.getAttribute('aria-level')), ['1', '2', '2', '2', '3'], 'levels follow the tree');
  assert.equal(items()[0].getAttribute('aria-expanded'), 'true', 'an element with kids does not say it is expanded');
  assert.equal(items()[1].hasAttribute('aria-expanded'), false, 'a leaf claims to expand');
  assert.match(items()[1].textContent, /H1 \(Heading 1\)/, 'a role-mapped element does not show both its type and the standard one');
  assert.match(items()[2].textContent, /no alt text/, 'a figure without alt text does not say so');
  assert.match(items()[4].textContent, /scope: Column/, 'a header cell does not show its scope');
  assert.match(doc.getElementById('tagTreeSummary').textContent, /5 element\(s\)\..*1 written inline/, 'the summary does not count the elements and the inline one');
});

test('the tree is one tab stop, and the arrow keys walk it as the tree pattern says', async () => {
  assert.deepEqual(items().map((i) => i.tabIndex), [0, -1, -1, -1, -1], 'the tree is not exactly one tab stop');
  items()[0].focus();
  await settle();
  await key('ArrowDown');
  assert.equal(focused(), 1, 'Down does not move to the next element');
  await key('End');
  assert.equal(focused(), 4, 'End does not move to the last element');
  await key('ArrowLeft');
  assert.equal(focused(), 3, 'Left does not move to the parent');
  await key('ArrowRight');
  assert.equal(focused(), 4, 'Right does not move to the first kid');
  await key('Home');
  assert.equal(focused(), 0, 'Home does not move to the first element');
  await key('ArrowUp');
  assert.equal(focused(), 0, 'Up past the first element moved focus out of the tree');
  await key('ArrowRight');
  assert.equal(focused(), 1, 'Right from the root does not move to its first kid');
  assert.deepEqual(items().map((i) => i.getAttribute('aria-selected')), ['false', 'true', 'false', 'false', 'false'], 'selection does not follow focus');
  assert.deepEqual(items().map((i) => i.tabIndex), [-1, 0, -1, -1, -1], 'the tab stop does not follow focus');
});

// The stub viewer renders no page, so the outline itself is tier 3's — see the file header.
test('focusing an element takes the viewer to its page, a figure with no box included', async () => {
  assert.notEqual(doc.querySelector('.pageNum').value, '2', 'setup: the viewer is already on page 2');
  items()[2].focus();
  await settle();
  assert.equal(doc.querySelector('.pageNum').value, '2', 'focusing the page-2 figure did not take the viewer to page 2');
});

test('a document with no tree says so, and offers the way to make one', async () => {
  tree = { tagged: false, unaddressable: 0, elements: [] };
  await openPanel();
  assert.equal(items().length, 0, 'an untagged document shows tree items');
  assert.match(doc.getElementById('tagTreeSummary').textContent, /no structure tree.*Tag structure/, 'the summary does not say there is no tree, or where to make one');
  tree = treeOne();
  await openPanel();
  assert.equal(items().length, 5, 'setup: the tree is back for the tests below');
});

test('an undo reloads the document, and the panel reads that document\'s tree again', async () => {
  doc.body.focus();
  const before = treeCalls();
  setNextDocument({ numPages: 3 });
  win.dispatchEvent(new win.KeyboardEvent('keydown', { key: 'z', ctrlKey: true, bubbles: true }));
  await settle();
  await settle();
  await settle();
  assert.ok(calls.some((c) => c.url.includes('/api/undo')), 'setup: Ctrl+Z did not reach the server\'s undo');
  assert.ok(treeCalls() > before, 'the document reloaded after an undo and the panel kept showing the tree it had read before');
});

test('the panel follows a document switch, and a late answer for the document left behind is dropped', async () => {
  await openDoc(TWO, 2);
  assert.deepEqual(items().map((i) => i.dataset.id), ['20', '21'], 'opening a second document did not show its tree');
  const tabs = () => [...doc.querySelectorAll('#tabstrip .tab')];
  assert.equal(tabs().length, 2, 'setup: two documents are not open');

  // Back to the first: the switch itself must re-read, because a switch reloads nothing.
  tabs()[0].click();
  await settle();
  await settle();
  assert.equal(items().length, 5, 'switching to the first document still shows the second document\'s tree');

  // Now hold the first document's answer, switch to it, and leave before it arrives.
  tabs()[1].click();
  await settle();
  await settle();
  let release;
  holdOne = new Promise((r) => { release = r; });
  tabs()[0].click();
  await settle();
  tabs()[1].click();
  await settle();
  await settle();
  assert.deepEqual(items().map((i) => i.dataset.id), ['20', '21'], 'setup: the second document\'s tree is not showing');
  release();
  holdOne = null;
  await settle();
  await settle();
  await settle();
  assert.deepEqual(items().map((i) => i.dataset.id), ['20', '21'],
    'a late answer for the first document replaced the tree of the document on screen');
});

// The Reading Order view (P09.S06c). The stub viewer has no page views, so where the numbers land is
// tier 3's (test/ui/tagedit.test.mjs); what this tier can hold is the toggle's own state.
test('the reading order view is a toggle that says whether it is on', async () => {
  const toggle = doc.getElementById('tagOrderToggle');
  assert.equal(toggle.getAttribute('aria-pressed'), 'false', 'the reading order view starts on');
  toggle.click();
  await settle();
  assert.equal(toggle.getAttribute('aria-pressed'), 'true', 'switching the view on does not say it is pressed');
  assert.match(toggle.textContent, /Hide reading order/, 'the toggle does not offer to hide what it shows');
  toggle.click();
  await settle();
  assert.equal(toggle.getAttribute('aria-pressed'), 'false', 'switching the view off does not say so');
  assert.match(toggle.textContent, /Show reading order/, 'the toggle does not offer to show it again');
});
