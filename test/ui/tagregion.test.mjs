// Tagging what no tag owns, by keyboard, in a real browser — ADR-125.
//
// ── What only this tier can see ──────────────────────────────────────────────
// Tier 2 proves the section sends one region edit naming the ticked pieces. It cannot prove a person can DO
// it: that Tab reaches the page field, the button, each tick box and **Tag selected** in that order; that
// Space ticks; that the piece the keyboard is on is outlined on ITS page, over its text (jsdom has no page);
// or that the real server, given what the real page sends, lists the paragraph a person just marked as
// decoration and puts it back so the tree — and the accessibility check — read as they did before. This file
// drives that against the real binary.
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

const DOC = writeFixture('tagregion.pdf', { pages: 2, label: 'section' });

const h = await launch();
const { page } = h;

const heldDocs = () => page.evaluate(async () => (await (await nibFetch('/api/docs')).json()).docs.length);
let foundHeld = null;

// serverTree is the tree as the server reads the document now: each element's type, text, parent and page.
const serverTree = () => page.evaluate(async () => {
  const t = await (await nibFetch('/api/tags/tree')).json();
  return t.elements.map((e) => ({ type: e.standard, text: e.text, parent: e.parent, page: e.page }));
});
// checked is the accessibility check as the server answers it now: every clause and its verdict.
const checked = () => page.evaluate(async () => {
  const rep = await (await nibFetch('/api/uacheck')).json();
  return rep.results.map((r) => `${r.clause} ${r.verdict}`);
});
// untaggedOn is what the server lists as untagged on a page.
const untaggedOn = (n) => page.evaluate(async (p) => (await (await nibFetch(`/api/tags/untagged?page=${p}`)).json()).pieces, n);
// focused says where the keyboard is: the tree item's position, or the control's id.
const focused = () => page.evaluate(() => {
  const a = document.activeElement;
  const items = [...document.querySelectorAll('#tagTreeList [role="treeitem"]')];
  return { item: items.indexOf(a), id: a?.id || '', selected: items.findIndex((li) => li.getAttribute('aria-selected') === 'true') };
});
const statusOf = (id, re) => page.waitForFunction(([i, src]) => new RegExp(src).test(document.getElementById(i).textContent), [id, re.source], { timeout: 20000 });

let startTree = null;
let startChecked = null;

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
  startTree = await serverTree();
  startChecked = await checked();
  assert.deepEqual(startTree.map((e) => [e.type, e.parent, e.page]), [['P', -1, 1], ['P', -1, 2]],
    `setup: the committed tree is not two top-level paragraphs, one a page: ${JSON.stringify(startTree)}`);
  assert.ok(startChecked.length > 50, `setup: the check answered ${startChecked.length} clause(s)`);
  assert.deepEqual(await untaggedOn(2), [], 'setup: page 2 already has untagged content');
});

// tabTo presses Tab until focus matches, returning the stops it passed — the only way in: reaching a control
// is the claim, so `focus()` would skip it.
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
test('the second paragraph is marked as decoration from the tree, and the page then lists it as untagged', async () => {
  const header = await page.evaluate(() => {
    document.querySelector('.tab[data-panel="tagtree"]').focus();
    return document.activeElement?.dataset?.panel;
  });
  assert.equal(header, 'tagtree', 'setup: the panel header cannot take focus');
  const toTree = await tabTo('#tagTreeList [role="treeitem"]');
  assert.ok(toTree.ok, `Tab from the panel header never reached the tree: ${toTree.seen.map((s) => s.id).join(' → ')}`);
  await page.keyboard.press('ArrowDown');
  assert.deepEqual(await focused(), { item: 1, id: '', selected: 1 }, 'ArrowDown did not reach the second paragraph');
  const toArtifact = await tabTo('#tagEditArtifact');
  assert.ok(toArtifact.ok, `Tab from the tree never reached Mark as decoration: ${toArtifact.seen.map((s) => s.id).join(' → ')}`);
  await page.keyboard.press('Enter');
  await statusOf('tagEditStatus', /Changed/);

  const tree = await serverTree();
  assert.deepEqual(tree.map((e) => [e.type, e.page]), [['P', 1]], 'the second paragraph is still in the tree');
  const listed = await untaggedOn(2);
  assert.deepEqual(listed.map((p) => [p.kind, p.text, p.decoration, p.inForm]), [['text', startTree[1].text, true, false]],
    `the server does not list the paragraph as decoration on page 2: ${JSON.stringify(listed)}`);
});

test('the page is asked for by number and its untagged content is listed as tick boxes', async () => {
  // From wherever the reload left the keyboard, the section is reached by Tab: it comes after the edit bar.
  const toPage = await tabTo('#tagUntaggedPage');
  assert.ok(toPage.ok, `Tab never reached the page field: ${toPage.seen.map((s) => s.id).join(' → ')}`);
  await page.keyboard.type('2');
  const toShow = await tabTo('#tagUntaggedShow');
  assert.ok(toShow.ok && toShow.seen.length === 1, `Show untagged content is not the next stop after the page field: ${toShow.seen.map((s) => s.id).join(' → ')}`);
  await page.keyboard.press('Enter');
  await statusOf('tagUntaggedStatus', /1 piece\(s\) on page 2 have no tag/);

  const list = await page.evaluate(() => {
    const boxes = [...document.querySelectorAll('#tagUntaggedList input[type="checkbox"]')];
    const field = document.getElementById('tagUntaggedPieces');
    const r = field.getBoundingClientRect();
    return {
      labels: boxes.map((b) => b.closest('label')?.textContent.trim()),
      legend: document.getElementById('tagUntaggedLegend').textContent,
      shown: r.width > 0 && r.height > 0,
      focus: document.activeElement?.id,
    };
  });
  assert.deepEqual(list.labels, [`Text — “${startTree[1].text}” (marked as decoration)`], `the list does not hold the one paragraph, labelled: ${JSON.stringify(list)}`);
  assert.equal(list.legend, 'Untagged on page 2', 'the list does not say which page it is of');
  assert.ok(list.shown, 'the list has no box on screen');
  assert.equal(list.focus, 'tagUntaggedShow', 'listing the page took the focus from the button');
});

test('the paragraph is ticked in the list and tagged as a paragraph by Tab, Space and Enter', async () => {
  const toBox = await tabTo('#tagUntaggedList input[type="checkbox"]');
  assert.ok(toBox.ok && toBox.seen.length === 1, `the tick box is not the next stop after the button: ${toBox.seen.map((s) => s.id).join(' → ')}`);
  // The piece the keyboard is on is outlined on its page, over its text — before anything is ticked.
  const outline = await page.evaluate(() => {
    const boxes = [...document.querySelectorAll('.tag-outline')];
    const pageDiv = document.querySelector('.page[data-page-number="2"]');
    const o = boxes[0]?.getBoundingClientRect();
    const p = pageDiv?.getBoundingClientRect();
    return { n: boxes.length, onPage2: !!boxes[0] && boxes[0].parentElement === pageDiv, o: o && [o.left, o.top, o.right, o.bottom], p: p && [p.left, p.top, p.right, p.bottom, p.width, p.height] };
  });
  assert.equal(outline.n, 1, 'focusing a piece does not outline exactly it');
  assert.ok(outline.onPage2, 'the outline is not on page 2');
  // The fixture sets its line at 72pt from the left and 700pt up a 612 by 792 page, 36pt high.
  const [pl, pt, , , pw, ph] = outline.p;
  const [ol, ot, or, ob] = outline.o;
  const at = [(ol - pl) / pw, (ot - pt) / ph, (or - pl) / pw, (ob - pt) / ph];
  assert.ok(Math.abs(at[0] - 72 / 612) < 0.01 && at[2] > 0.3 && at[2] < 0.75, `the outline does not start where the line starts, or does not span it: ${JSON.stringify(at)}`);
  assert.ok(at[1] > 0.03 && at[1] < (792 - 700) / 792 && at[3] > (792 - 700) / 792 && at[3] < 0.16, `the outline is not over the line of text: ${JSON.stringify(at)}`);

  await page.keyboard.press('Space');
  assert.equal(await page.evaluate(() => document.activeElement.checked), true, 'Space did not tick the piece');
  assert.deepEqual((await serverTree()).map((e) => e.type), ['P'], 'a tick changed the document — a button applies');

  const toType = await tabTo('#tagUntaggedType');
  assert.ok(toType.ok, 'Tab never reached the type chooser');
  assert.equal(await page.evaluate(() => document.getElementById('tagUntaggedType').value), 'P', 'the type chooser does not open on a paragraph');
  const toApply = await tabTo('#tagUntaggedApply');
  assert.ok(toApply.ok, `Tab never reached Tag selected: ${toApply.seen.map((s) => s.id).join(' → ')}`);
  await page.keyboard.press('Enter');
  await statusOf('tagUntaggedStatus', /Tagged — Ctrl\+Z takes it back\. Nothing on page 2 is untagged\./);

  // The tree reads as it did before the paragraph was marked as decoration, and so does the check.
  assert.deepEqual(await serverTree(), startTree, 'the tree does not read as it did before the paragraph was marked as decoration');
  assert.deepEqual(await checked(), startChecked, 'the accessibility check does not read as it did before the paragraph was marked as decoration');
  assert.deepEqual(await untaggedOn(2), [], 'the server still lists untagged content on page 2');
  // The new tag is the second item of the tree — after the paragraph that was selected — and it has the focus.
  assert.deepEqual(await focused(), { item: 1, id: '', selected: 1 }, 'the new tag is not the focused, selected item of the tree');
  const shown = await page.evaluate(() => [...document.querySelectorAll('#tagTreeList [role="treeitem"]')].map((li) => li.textContent));
  assert.equal(shown.length, 2, 'the panel does not show two elements');
  assert.ok(shown[1].startsWith('P') && shown[1].includes('section 2'), `the new tag reads ${JSON.stringify(shown[1])}`);
  assert.equal(await page.evaluate(() => document.querySelectorAll('#tagUntaggedList input').length), 0, 'the list still offers the piece that was tagged');
});

test('one Ctrl+Z takes the region back', async () => {
  // Focus is on a tree item, not a text field, so Ctrl+Z is the document's.
  await page.keyboard.press('Control+z');
  await page.waitForFunction(() => document.querySelectorAll('#tagTreeList [role="treeitem"]').length === 1, null, { timeout: 20000 });
  assert.deepEqual((await serverTree()).map((e) => [e.type, e.page]), [['P', 1]], 'one undo did not take the new tag away');
  assert.equal((await untaggedOn(2)).length, 1, 'with the region undone the paragraph is not untagged again');
  await page.keyboard.press('Control+Shift+z');
  await page.waitForFunction(() => document.querySelectorAll('#tagTreeList [role="treeitem"]').length === 2, null, { timeout: 20000 });
  assert.deepEqual(await serverTree(), startTree, 'redo did not bring the region back');
});
// ── keyboard only, to here ───────────────────────────────────────────────────

test('the region between the markers used no pointer at all', () => {
  const src = readFileSync(new URL('./tagregion.test.mjs', import.meta.url), 'utf8');
  const START = '// ── keyboard only, from here';
  const STOP = '// ── keyboard only, to here';
  const body = src.slice(src.indexOf(START), src.indexOf(STOP));
  assert.ok(body.length > 4000, `the scanned region is ${body.length} chars — a marker has drifted`);
  for (const name of ['marked as decoration from the tree', 'asked for by number', 'ticked in the list', 'takes the region back']) {
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
