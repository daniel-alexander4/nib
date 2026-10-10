// Making an inline tag editable, changing it and undoing both by keyboard, in a real browser — ADR-126.
//
// ── What only this tier can see ──────────────────────────────────────────────
// Tier 2 proves the button sends one `promote` edit and that focus is asked to land on the tag at the same
// place. It cannot prove a person can DO it: that the arrow keys reach a tag written inline, that Tab
// reaches the one button it has — with the line that says why on screen — that Enter presses it, that the
// real server, given what the real page sends, numbers the tag and hands back a tree that reads the same,
// that the type picker then answers typing, and that Ctrl+Z twice leaves the tree as the file had it. This
// file drives that against the real binary, on a file that really has a tag written inline.
//
// ── The rule this file lives by ──────────────────────────────────────────────
// The SETUP — opening the document and the panel — uses the harness, like every file in this tier: it is
// not the claim. Everything between the two markers below uses the keyboard alone: no `page.click`, no
// `page.mouse`, no helper that reaches past the UI. The last-but-one test scans that region of this file and
// fails if either appears there.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { launch, shutdown } from './harness.mjs';
import { makeInlineTaggedPDF, writeRawFixture } from './fixtures.mjs';

const DOC = writeRawFixture('tagpromote.pdf', makeInlineTaggedPDF());

const h = await launch();
const { page } = h;

const heldDocs = () => page.evaluate(async () => (await (await nibFetch('/api/docs')).json()).docs.length);
let foundHeld = null;

// serverTree is the tree as the server reads the document now.
const serverTree = () => page.evaluate(async () => {
  const t = await (await nibFetch('/api/tags/tree')).json();
  return { inline: t.unaddressable, elements: t.elements.map((e) => ({ id: e.id, type: e.standard, text: e.text, parent: e.parent })) };
});
const reading = (t) => t.elements.map((e) => [e.type, e.text, e.parent]);
const shownTree = () => page.evaluate(() => [...document.querySelectorAll('#tagTreeList [role="treeitem"]')]
  .map((li) => ({ level: li.getAttribute('aria-level'), label: li.textContent })));
const focused = () => page.evaluate(() => {
  const a = document.activeElement;
  const items = [...document.querySelectorAll('#tagTreeList [role="treeitem"]')];
  return { item: items.indexOf(a), id: a?.id || '', selected: items.findIndex((li) => li.getAttribute('aria-selected') === 'true') };
});
const status = (re) => page.waitForFunction((src) => new RegExp(src).test(document.getElementById('tagEditStatus').textContent), re.source, { timeout: 20000 });
// onScreen says whether an element has a box a person can see.
const onScreen = (id) => page.evaluate((i) => {
  const r = document.getElementById(i).getBoundingClientRect();
  return r.width > 0 && r.height > 0;
}, id);

let start = null;
let startShown = null;

after(async () => {
  try {
    h.answerDialogs(true);
    for (let i = 0; i < 8 && await h.hasDocument(); i++) {
      await h.closeDocument();
    }
  } catch { /* the assertion that already failed is the one worth reporting */ }
  await shutdown(h);
});

test('setup: a document whose tree has a tag written inline, and the structure tree panel open', async () => {
  foundHeld = await heldDocs();
  await h.openDocument(DOC, 1);
  await h.mode('accessibility'); // ADR-035
  await page.click('.tab[data-panel="tagtree"]');
  await page.waitForFunction(() => document.querySelectorAll('#tagTreeList [role="treeitem"]').length === 4, null, { timeout: 20000 });
  start = await serverTree();
  startShown = await shownTree();
  assert.equal(start.inline, 1, `setup: the document does not have exactly one inline tag: ${JSON.stringify(start)}`);
  assert.deepEqual(reading(start), [['Document', 'TitleInsideBody', -1], ['H1', 'Title', 0], ['Sect', 'InsideBody', 0], ['P', 'Body', 2]],
    'setup: the tree does not read Document › Heading, Sect › P');
  assert.equal(start.elements[2].id, 0, 'setup: the Sect has a number, so nothing here is inline');
});

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
test('the arrow keys reach the inline tag, which says why it cannot be changed and offers one button', async () => {
  const header = await page.evaluate(() => {
    document.querySelector('.tab[data-panel="tagtree"]').focus();
    return document.activeElement?.dataset?.panel;
  });
  assert.equal(header, 'tagtree', 'setup: the panel header cannot take focus');
  const toTree = await tabTo('#tagTreeList [role="treeitem"]');
  assert.ok(toTree.ok, `Tab from the panel header never reached the tree: ${toTree.seen.map((s) => s.id).join(' → ')}`);
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('ArrowDown');
  assert.deepEqual(await focused(), { item: 2, id: '', selected: 2 }, 'two ArrowDowns did not reach the Sect');

  assert.equal(await onScreen('tagEditInlineWhy'), true, 'the line that says why an inline tag cannot be changed is not on screen');
  assert.match(await page.evaluate(() => document.getElementById('tagEditInlineWhy').textContent),
    /written inline in the file.*until it is given a number of its own/, 'the line does not say what the matter is');
  assert.equal(await onScreen('tagEditPromote'), true, 'the button is not on screen');
  const dead = await page.evaluate(() => ['tagEditType', 'tagEditTypeApply', 'tagEditAltApply', 'tagEditDelete', 'tagEditArtifact']
    .filter((id) => !document.getElementById(id).disabled));
  assert.deepEqual(dead, [], `controls that cannot name an inline tag are enabled: ${dead}`);

  // The button is reached by Tab, and nothing between the tree and it but the new-tag row.
  const toButton = await tabTo('#tagEditPromote');
  assert.ok(toButton.ok, `Tab from the tree never reached Make inline tags editable: ${toButton.seen.map((s) => s.id).join(' → ')}`);
  assert.ok(toButton.seen.length <= 3, `the button is ${toButton.seen.length} stops from the tree: ${toButton.seen.map((s) => s.id).join(' → ')}`);
});

test('Enter on the button numbers the tag: the tree reads the same, and the same place is focused, selected and live', async () => {
  await page.keyboard.press('Enter');
  await status(/Every tag can now be changed/);
  const now = await serverTree();
  assert.equal(now.inline, 0, 'the document still has a tag written inline');
  assert.deepEqual(reading(now), reading(start), 'numbered, the tree does not read as it did');
  assert.ok(now.elements[2].id > 0, 'the Sect has no number');
  assert.deepEqual(now.elements.map((e) => e.id).filter((_, i) => i !== 2), start.elements.map((e) => e.id).filter((_, i) => i !== 2),
    'a tag that had a number has another');
  assert.deepEqual(await shownTree(), startShown, 'the panel does not show the tree it showed');
  assert.deepEqual(await focused(), { item: 2, id: '', selected: 2 }, 'the tag at the same place is not the focused, selected item');
  assert.equal(await onScreen('tagEditPromote'), false, 'the button is still on screen with nothing left to number');
  assert.equal(await onScreen('tagEditInlineWhy'), false, 'the line is still on screen for a tag that has a number');
  assert.match(await page.evaluate(() => document.getElementById('tagEditStatus').textContent), /one step.*Ctrl\+Z/, 'the status does not say it is one undo step');
});

test('its type is then changed by typing in the picker and Enter on its button', async () => {
  const toType = await tabTo('#tagEditType');
  assert.ok(toType.ok, `Tab from the tree never reached the type picker: ${toType.seen.map((s) => s.id).join(' → ')}`);
  assert.equal(await page.evaluate(() => document.getElementById('tagEditType').disabled), false, 'the type picker is still disabled');
  await page.keyboard.type('Art');
  await page.waitForFunction(() => document.getElementById('tagEditType').value === 'Art', null, { timeout: 5000 });
  const toApply = await tabTo('#tagEditTypeApply');
  assert.ok(toApply.ok, 'Tab never reached Change type');
  await page.keyboard.press('Enter');
  await status(/Changed/);
  const now = await serverTree();
  assert.deepEqual(reading(now)[2], ['Art', 'InsideBody', 0], 'the promoted tag is not an Art holding what it held');
  assert.match((await shownTree())[2].label, /^Art/, 'the panel does not show the new type');
});

test('Ctrl+Z twice takes back the type and then the numbering, and the tree is as the file had it', async () => {
  await page.keyboard.press('Control+z');
  await page.waitForFunction(async () => {
    const t = await (await nibFetch('/api/tags/tree')).json();
    return t.elements[2].standard === 'Sect' && t.unaddressable === 0;
  }, null, { timeout: 20000 });
  const once = await serverTree();
  assert.ok(once.elements[2].id > 0, 'one undo took the numbering back with the type — they are two steps');
  await page.keyboard.press('Control+z');
  await page.waitForFunction(async () => (await (await nibFetch('/api/tags/tree')).json()).unaddressable === 1, null, { timeout: 20000 });
  assert.deepEqual(await serverTree(), start, 'after two undos the document\'s tree is not the tree the file had');
  await page.waitForFunction((want) => JSON.stringify([...document.querySelectorAll('#tagTreeList [role="treeitem"]')]
    .map((li) => ({ level: li.getAttribute('aria-level'), label: li.textContent }))) === want, JSON.stringify(startShown), { timeout: 20000 });
  assert.match(await page.evaluate(() => document.getElementById('tagTreeSummary').textContent), /1 written inline/, 'the panel does not count the inline tag again');
});
// ── keyboard only, to here ───────────────────────────────────────────────────

test('the region between the markers used no pointer at all', () => {
  const src = readFileSync(new URL('./tagpromote.test.mjs', import.meta.url), 'utf8');
  const START = '// ── keyboard only, from here';
  const STOP = '// ── keyboard only, to here';
  const body = src.slice(src.indexOf(START), src.indexOf(STOP));
  assert.ok(body.length > 4000, `the scanned region is ${body.length} chars — a marker has drifted`);
  for (const name of ['arrow keys reach the inline tag', 'Enter on the button numbers the tag', 'its type is then changed', 'Ctrl+Z twice']) {
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
