// Adding a tag, filling it and deleting it again by keyboard, in a real browser — ADR-124.
//
// ── What only this tier can see ──────────────────────────────────────────────
// Tier 2 proves each of the four controls sends its edit and that focus is asked to land on the new tag.
// It cannot prove a person can DO it: that Tab reaches the new-tag picker and its button, that a closed
// select answers typing, that Enter applies, that the tree's arrow keys reach the tag just made and the
// paragraph to put in it, that a disabled move says why in a line that has a box on screen — or that the
// real server, given what the real page sends, hands back a tree that reads as it did at the start. This
// file drives that against the real binary.
//
// ── The rule this file lives by ──────────────────────────────────────────────
// The SETUP — opening a document and committing a proposal so there is a tree — uses the harness, like
// every file in this tier: it is not the claim. Everything between the two markers below uses the
// keyboard alone: no `page.click`, no `page.mouse`, no helper that reaches past the UI. The
// last-but-one test scans that region of this file and fails if either appears there.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { launch, shutdown } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const DOC = writeFixture('tagcreate.pdf', { pages: 2, label: 'section' });

const h = await launch();
const { page } = h;

const heldDocs = () => page.evaluate(async () => (await (await nibFetch('/api/docs')).json()).docs.length);
let foundHeld = null;

// serverTree is the tree as the server reads the document now: each element's type, text and parent.
const serverTree = () => page.evaluate(async () => {
  const t = await (await nibFetch('/api/tags/tree')).json();
  return t.elements.map((e) => ({ type: e.standard, text: e.text, parent: e.parent, kids: e.kids.length }));
});
// shownTree is the tree as the panel draws it: each item's level and label.
const shownTree = () => page.evaluate(() => [...document.querySelectorAll('#tagTreeList [role="treeitem"]')]
  .map((li) => ({ level: li.getAttribute('aria-level'), label: li.textContent })));
// focused says where the keyboard is: the tree item's position, or the control's id.
const focused = () => page.evaluate(() => {
  const a = document.activeElement;
  const items = [...document.querySelectorAll('#tagTreeList [role="treeitem"]')];
  return { item: items.indexOf(a), id: a?.id || '', selected: items.findIndex((li) => li.getAttribute('aria-selected') === 'true') };
});
const status = (re) => page.waitForFunction((src) => new RegExp(src).test(document.getElementById('tagEditStatus').textContent), re.source, { timeout: 20000 });

let startServer = null;
let startShown = null;

after(async () => {
  try {
    h.answerDialogs(true);
    if (await page.evaluate(() => !document.getElementById('tagsModal').hidden)) await page.click('#tagsClose');
    for (let i = 0; i < 8 && await h.hasDocument(); i++) {
      await h.closeDocument();
    }
  } catch { /* the assertion that already failed is the one worth reporting */ }
  await shutdown(h);
});

test('setup: a document with a committed tree of two paragraphs, and the structure tree panel open', async () => {
  foundHeld = await heldDocs();
  await h.openDocument(DOC, 2);
  await h.mode('accessibility'); // ADR-035
  await h.group('Tag Structure');
  await page.click('#tagsBtn');
  await page.waitForFunction(() => document.querySelectorAll('#tagsList .tags-row').length > 0, null, { timeout: 20000 });
  await page.click('#tagsCommit');
  await page.waitForFunction(() => document.getElementById('tagsModal').hidden, null, { timeout: 20000 });
  await page.click('.tab[data-panel="tagtree"]');
  await page.waitForFunction(() => document.querySelectorAll('#tagTreeList [role="treeitem"]').length >= 2, null, { timeout: 20000 });
  startServer = await serverTree();
  startShown = await shownTree();
  assert.deepEqual(startServer.map((e) => [e.type, e.parent, e.kids]), [['P', -1, 0], ['P', -1, 0]],
    `setup: the committed tree is not two top-level paragraphs: ${JSON.stringify(startServer)}`);
  assert.ok(startServer[0].text.includes('section 1') && startServer[1].text.includes('section 2'), 'setup: the paragraphs do not read their pages');
});

// tabTo presses Tab until focus matches, returning the stops it passed — the only way in, as in
// tagedit.test.mjs: reaching a control is the claim, so `focus()` would skip it.
async function tabTo(selector, max = 60) {
  const seen = [];
  for (let i = 0; i < max; i++) {
    const here = await page.evaluate((s) => {
      const a = document.activeElement;
      return { hit: !!a?.matches?.(s), id: a ? `${a.tagName}#${a.id || a.className}`.slice(0, 40) : 'none' };
    }, selector);
    if (here.hit) return { ok: true, seen };
    seen.push(here);
    await page.keyboard.press('Tab');
  }
  return { ok: false, seen };
}

// ── keyboard only, from here ─────────────────────────────────────────────────
test('a new tag is added after the selected paragraph by Tab, typing and Enter, and takes the focus', async () => {
  const header = await page.evaluate(() => {
    document.querySelector('.tab[data-panel="tagtree"]').focus();
    return document.activeElement?.dataset?.panel;
  });
  assert.equal(header, 'tagtree', 'setup: the panel header cannot take focus');
  const toTree = await tabTo('#tagTreeList [role="treeitem"]');
  assert.ok(toTree.ok, `Tab from the panel header never reached the tree: ${toTree.seen.map((s) => s.id).join(' → ')}`);
  assert.deepEqual(await focused(), { item: 0, id: '', selected: 0 }, 'the first paragraph is not focused and selected');

  // The first paragraph is first of its siblings and at the top: neither move can be made, and each says
  // why in a line a person can see.
  const whys = await page.evaluate(() => ['tagEditIn', 'tagEditOut'].map((id) => {
    const why = document.getElementById(`${id}Why`);
    const r = why.getBoundingClientRect();
    return { id, disabled: document.getElementById(id).disabled, text: why.textContent, shown: r.width > 0 && r.height > 0 };
  }));
  assert.deepEqual(whys.map((w) => [w.disabled, w.shown]), [[true, true], [true, true]], `a move that cannot be made is enabled, or its reason is not on screen: ${JSON.stringify(whys)}`);
  assert.match(whys[0].text, /no tag above/, 'Move into the tag above does not say why it is disabled');
  assert.match(whys[1].text, /top level/, 'Move out one level does not say why it is disabled');

  const toType = await tabTo('#tagNewType');
  assert.ok(toType.ok, `Tab from the tree never reached the new-tag picker: ${toType.seen.map((s) => s.id).join(' → ')}`);
  assert.equal(toType.seen.length, 1, `the new-tag picker is not the next stop after the tree: ${toType.seen.map((s) => s.id).join(' → ')}`);
  await page.keyboard.type('Div');
  await page.waitForFunction(() => document.getElementById('tagNewType').value === 'Div', null, { timeout: 5000 });
  const toAdd = await tabTo('#tagNewAdd');
  assert.ok(toAdd.ok, 'Tab never reached Add tag');
  await page.keyboard.press('Enter');
  await status(/Tag added/);

  assert.deepEqual((await serverTree()).map((e) => [e.type, e.parent, e.kids]), [['P', -1, 0], ['Div', -1, 0], ['P', -1, 0]],
    'the document does not hold an empty Div between its two paragraphs');
  assert.deepEqual(await focused(), { item: 1, id: '', selected: 1 }, 'the new tag is not the focused, selected item of the tree');
  const shown = await shownTree();
  assert.equal(shown.length, 3, 'the panel does not show three elements');
  assert.match(shown[1].label, /^Div/, `the new tag reads ${JSON.stringify(shown[1])}`);
});

test('the next paragraph is moved into the new tag with the arrow keys, Tab and Enter', async () => {
  await page.keyboard.press('ArrowDown');
  assert.deepEqual(await focused(), { item: 2, id: '', selected: 2 }, 'ArrowDown from the new tag did not reach the paragraph after it');
  const toIn = await tabTo('#tagEditIn');
  assert.ok(toIn.ok, `Tab from the tree never reached Move into the tag above: ${toIn.seen.map((s) => s.id).join(' → ')}`);
  await page.keyboard.press('Enter');
  await status(/Changed/);

  const tree = await serverTree();
  assert.deepEqual(tree.map((e) => [e.type, e.parent, e.kids]), [['P', -1, 0], ['Div', -1, 1], ['P', 1, 0]], 'the second paragraph is not inside the Div');
  assert.equal(tree[1].text, startServer[1].text, 'the Div does not read the paragraph it was given');
  const shown = await shownTree();
  assert.deepEqual(shown.map((s) => s.level), ['1', '1', '2'], 'the panel does not show the paragraph one level in');
  assert.ok(shown[1].label.includes('section 2'), `the Div's own line does not read the text it now holds: ${shown[1].label}`);
  // The paragraph is now the Div's first kid, so the button that moved it has nothing more to do: focus is
  // on the paragraph in the tree, not lost.
  assert.deepEqual(await focused(), { item: 2, id: '', selected: 2 }, 'after the move focus is not on the moved paragraph');
});

test('a paragraph at the top that holds content cannot be deleted, and the bar says why', async () => {
  await page.keyboard.press('Home');
  assert.deepEqual(await focused(), { item: 0, id: '', selected: 0 }, 'Home did not reach the first paragraph');
  const toDelete = await tabTo('#tagEditDelete');
  assert.ok(toDelete.ok, `Tab from the tree never reached Delete this tag: ${toDelete.seen.map((s) => s.id).join(' → ')}`);
  const before = await serverTree();
  await page.keyboard.press('Enter');
  await status(/top of the structure tree/);
  assert.deepEqual(await serverTree(), before, 'a refused delete changed the document');
  assert.equal((await focused()).id, 'tagEditDelete', 'a refused delete took the focus from the button');
});

test('the tag is deleted again and the tree reads as it did before it was made', async () => {
  // Back to the tree — Shift+Tab, the way a person goes back — then to the Div.
  for (let i = 0; i < 30 && (await focused()).item < 0; i++) await page.keyboard.press('Shift+Tab');
  assert.equal((await focused()).item, 0, 'Shift+Tab from the bar did not return to the selected item of the tree');
  await page.keyboard.press('ArrowDown');
  assert.deepEqual(await focused(), { item: 1, id: '', selected: 1 }, 'ArrowDown did not reach the Div');
  const toDelete = await tabTo('#tagEditDelete');
  assert.ok(toDelete.ok, 'Tab never reached Delete this tag');
  const help = await page.evaluate(() => {
    const b = document.getElementById('tagEditDelete');
    const p = document.getElementById(b.getAttribute('aria-describedby'));
    const r = p.getBoundingClientRect();
    return { label: b.textContent, help: p.textContent, shown: r.width > 0 && r.height > 0 };
  });
  assert.match(help.label, /keep its content/, 'the button a person is about to press does not say the content is kept');
  assert.ok(help.shown && /move up to the tag above/.test(help.help), `the line that says where the content goes is not on screen: ${JSON.stringify(help)}`);
  await page.keyboard.press('Enter');
  await status(/Changed/);

  assert.deepEqual(await serverTree(), startServer, 'with the tag deleted the document\'s tree does not read as it did before the tag was made');
  assert.deepEqual(await shownTree(), startShown, 'with the tag deleted the panel does not show the tree it showed at the start');
  // The paragraph that was inside now stands where the tag stood, and it is what is selected.
  assert.equal((await focused()).selected, 1, 'after the delete the selection is not on the paragraph that took the tag\'s place');
});
// ── keyboard only, to here ───────────────────────────────────────────────────

test('the region between the markers used no pointer at all', () => {
  const src = readFileSync(new URL('./tagcreate.test.mjs', import.meta.url), 'utf8');
  const START = '// ── keyboard only, from here';
  const STOP = '// ── keyboard only, to here';
  const body = src.slice(src.indexOf(START), src.indexOf(STOP));
  assert.ok(body.length > 4000, `the scanned region is ${body.length} chars — a marker has drifted`);
  for (const name of ['a new tag is added', 'moved into the new tag', 'cannot be deleted', 'deleted again']) {
    assert.ok(body.includes(name), `the scanned region does not hold the test "${name}" — a marker has drifted`);
  }
  for (const banned of ['page' + '.click(', 'page' + '.mouse', 'h' + '.openDocument(', 'h' + '.card(', 'h' + '.mode(', 'h' + '.panel(', 'h' + '.group(', '.cli' + 'ck()']) {
    assert.ok(!body.includes(banned), `the keyboard region calls ${banned}, so it proves a mouse or the harness can do this, not a keyboard user`);
  }
});

test('this file leaves the shared server as it found it', async () => {
  h.answerDialogs(true);
  assert.notEqual(foundHeld, null, 'setup: the first test never ran, so there is no baseline to return to');
  await h.closeDocument();
  assert.equal(await heldDocs(), foundHeld, 'the server holds a different number of documents than this file found');
});
