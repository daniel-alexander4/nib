// Correcting a structure tree by keyboard, in a real browser — `PLAN-accessibility.md` P09.S06b.
//
// ── What only this tier can see ──────────────────────────────────────────────
// Tier 2 proves each control sends its edit and the person is returned to where they were. It cannot
// prove the panel answers a real keyboard — Tab reaching the tree and the bar, a closed select changing
// by typing, Enter applying, Ctrl+Z reaching the undo — or that the outline lands on the element's text,
// or that the report nib shows changes because of an edit and changes back after an undo. This file
// drives all of that against the real binary.
//
// ── The rule this file lives by ──────────────────────────────────────────────
// The SETUP — opening a document and committing a proposal so there is a tree to correct — uses the
// harness, like every file in this tier: it is not the claim. The CORRECTION is the claim, and it uses
// the keyboard alone: no `page.click`, no `page.mouse`, no helper that reaches past the UI. The last
// test scans this file's correction region and fails if either appears there.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { launch } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const DOC = writeFixture('tagedit.pdf', { pages: 2, label: 'section' });

const h = await launch();
const { page } = h;

const heldDocs = () => page.evaluate(async () => (await (await fetch('/api/docs')).json()).docs.length);
let foundHeld = null;

// clause73 is 7.3 t1's verdict from the report door the modal reads.
const clause73 = () => page.evaluate(async () => {
  const rep = await (await fetch('/api/uacheck')).json();
  return (rep.results || []).find((r) => r.clause === '7.3 t1')?.verdict;
});

after(async () => {
  try {
    h.answerDialogs(true);
    if (await page.evaluate(() => !document.getElementById('tagsModal').hidden)) await page.click('#tagsClose');
    for (let i = 0; i < 8 && await page.$eval('#viewerWrap', (el) => el.className) === 'has-doc'; i++) {
      await h.closeDocument();
    }
  } catch { /* the assertion that already failed is the one worth reporting */ }
  await h.browser.close();
});

test('setup: a document with a committed tree, and the structure tree panel open', async () => {
  foundHeld = await heldDocs();
  await h.openDocument(DOC, 2);
  await h.mode('edit');
  await h.group('Tag Structure');
  await page.click('#tagsBtn');
  await page.waitForFunction(() => document.querySelectorAll('#tagsList .tags-row').length > 0, null, { timeout: 20000 });
  await page.click('#tagsCommit');
  await page.waitForFunction(() => document.getElementById('tagsModal').hidden, null, { timeout: 20000 });
  await page.click('.tab[data-panel="tagtree"]');
  await page.waitForFunction(() => document.querySelectorAll('#tagTreeList [role="treeitem"]').length >= 2, null, { timeout: 20000 });
  assert.equal(await clause73(), 'not applicable', 'setup: the committed tree already has a figure');
});

// tabTo presses Tab until focus matches, returning the stops it passed — the only way in, as in
// keyboardflow.test.mjs: reaching a control is the claim, so `focus()` would skip it.
async function tabTo(selector, max = 60) {
  const seen = [];
  for (let i = 0; i < max; i++) {
    const here = await page.evaluate((s) => {
      const a = document.activeElement;
      const r = a?.getBoundingClientRect?.();
      return { hit: !!a?.matches?.(s), id: a ? `${a.tagName}#${a.id || a.className}`.slice(0, 40) : 'none', visible: !!r && r.width > 0 && r.height > 0, inPanel: !!a?.closest?.('#tagtree') };
    }, selector);
    if (here.hit) return { ok: true, seen };
    seen.push(here);
    await page.keyboard.press('Tab');
  }
  return { ok: false, seen };
}

test('the tree and the bar are reached by Tab, and the outline lands on the element\'s text', async () => {
  const header = await page.evaluate(() => {
    document.querySelector('.tab[data-panel="tagtree"]').focus();
    return document.activeElement?.dataset?.panel;
  });
  assert.equal(header, 'tagtree', 'setup: the panel header cannot take focus');
  const toTree = await tabTo('#tagTreeList [role="treeitem"]');
  assert.ok(toTree.ok, `Tab from the panel header never reached the tree: ${toTree.seen.map((s) => s.id).join(' → ')}`);
  await page.waitForSelector('.tag-outline', { timeout: 10000 });
  // The commit reloaded the document, and its text layer is rebuilt after the page is: the first run
  // compared the outline with a span still at [0,0,0,0], and the second waited 20 s on the FIRST match,
  // which never laid out. So any matching span with a box — and if none ever has one, the failure says
  // what was there rather than timing out mute.
  const found = await page.waitForFunction(() => [...document.querySelectorAll('.viewerContainer:not([hidden]) .page .textLayer span')]
    .some((s) => s.textContent.includes('section 1') && s.getBoundingClientRect().width > 0 && s.getBoundingClientRect().height > 0),
  null, { timeout: 20000 }).then(() => true, () => false);
  if (!found) {
    const seen = await page.evaluate(() => ({
      spans: [...document.querySelectorAll('.textLayer span')].filter((s) => s.textContent.includes('section 1')).map((s) => {
        const r = s.getBoundingClientRect();
        return { box: [r.left, r.top, r.width, r.height].map(Math.round), inVisibleView: !!s.closest('.viewerContainer:not([hidden])') };
      }),
      layers: [...document.querySelectorAll('.viewerContainer:not([hidden]) .page .textLayer')].map((l) => getComputedStyle(l).display),
      pages: document.querySelectorAll('.viewerContainer:not([hidden]) .page').length,
    }));
    assert.fail(`no laid-out text-layer span reads "section 1" after 20 s — what was there: ${JSON.stringify(seen)}`);
  }
  const geometry = await page.evaluate(() => {
    const o = document.querySelector('.tag-outline').getBoundingClientRect();
    const span = [...document.querySelectorAll('.viewerContainer:not([hidden]) .page .textLayer span')]
      .find((s) => s.textContent.includes('section 1') && s.getBoundingClientRect().width > 0 && s.getBoundingClientRect().height > 0);
    if (!span) return null;
    const t = span.getBoundingClientRect();
    return { overlaps: o.left < t.right && o.right > t.left && o.top < t.bottom && o.bottom > t.top, outline: [o.left, o.top, o.right, o.bottom], text: [t.left, t.top, t.right, t.bottom] };
  });
  assert.ok(geometry, 'setup: page one has no text-layer span reading "section 1"');
  assert.ok(geometry.overlaps, `the outline does not cover the selected element's text: ${JSON.stringify(geometry)}`);
});

test('retype, alt text and undo, by keyboard alone, move the report there and back', async () => {
  const toType = await tabTo('#tagEditType');
  assert.ok(toType.ok, `Tab from the tree never reached the type picker: ${toType.seen.map((s) => s.id).join(' → ')}`);
  await page.keyboard.type('Fig');
  await page.waitForFunction(() => document.getElementById('tagEditType').value === 'Figure', null, { timeout: 5000 });
  const toApply = await tabTo('#tagEditTypeApply');
  assert.ok(toApply.ok, 'Tab never reached Change type');
  await page.keyboard.press('Enter');
  await page.waitForFunction(() => /Ctrl\+Z/.test(document.getElementById('tagEditStatus').textContent), null, { timeout: 20000 });
  assert.equal(await clause73(), 'fail', 'a paragraph retyped as a figure without alt text does not fail 7.3 t1 in nib\'s report');

  const toAlt = await tabTo('#tagEditAlt');
  assert.ok(toAlt.ok, 'Tab never reached the alt text field');
  await page.keyboard.type('A section heading');
  await page.keyboard.press('Enter');
  await page.waitForFunction(async () => (await (await fetch('/api/uacheck')).json()).results.find((r) => r.clause === '7.3 t1')?.verdict === 'pass', null, { timeout: 20000 });

  // Off the text field first: a field with text in it owns Ctrl+Z, and the claim is the document's undo.
  const toSet = await tabTo('#tagEditAltApply');
  assert.ok(toSet.ok, 'Tab never reached Set alt text');
  await page.keyboard.press('Control+z');
  await page.waitForFunction(async () => (await (await fetch('/api/uacheck')).json()).results.find((r) => r.clause === '7.3 t1')?.verdict === 'fail', null, { timeout: 20000 });
});

test('the panel is no keyboard trap, and every stop in it is visible', async () => {
  const start = await tabTo('#tagTreeList [role="treeitem"]', 1);
  if (!start.ok) {
    const back = await page.evaluate(() => { document.querySelector('#tagTreeList [aria-selected="true"], #tagTreeList [role="treeitem"]').focus(); return true; });
    assert.ok(back, 'setup');
  }
  const stops = [];
  for (let i = 0; i < 40; i++) {
    await page.keyboard.press('Tab');
    const here = await page.evaluate(() => {
      const a = document.activeElement;
      const r = a?.getBoundingClientRect?.();
      return { id: a ? `${a.tagName}#${a.id || a.className}`.slice(0, 40) : 'none', inPanel: !!a?.closest?.('#tagtree'), visible: !!r && r.width > 0 && r.height > 0 };
    });
    stops.push(here);
    if (!here.inPanel) break;
  }
  const inside = stops.filter((s) => s.inPanel);
  assert.ok(stops.length && !stops[stops.length - 1].inPanel, `Tab stayed inside the panel for 40 presses: ${stops.map((s) => s.id).join(' → ')}`);
  assert.deepEqual(inside.filter((s) => !s.visible).map((s) => s.id), [], 'focus stopped on a control in the panel that has no box on screen');
  assert.ok(inside.length >= 4, `only ${inside.length} stop(s) inside the panel — the bar's controls are not in the tab order`);
});

test('the correction region used no pointer at all', () => {
  const src = readFileSync(new URL('./tagedit.test.mjs', import.meta.url), 'utf8');
  const START = "test('the tree and the bar are reached by Tab";
  const STOP = "test('the correction region used no pointer at all'";
  const body = src.slice(src.indexOf(START), src.indexOf(STOP));
  assert.ok(body.length > 2000, `the scanned region is ${body.length} chars — an anchor has drifted`);
  for (const banned of ['page' + '.click(', 'page' + '.mouse', 'h' + '.openDocument(', 'h' + '.card(', 'h' + '.mode(', 'h' + '.panel(', 'h' + '.group(']) {
    assert.ok(!body.includes(banned), `the correction region calls ${banned}, so it proves a mouse or the harness can correct a tree, not a keyboard user`);
  }
});

// The Reading Order view — P09.S06c. After the undo above the tree is the committed proposal again: one
// paragraph per page. Outside the correction region, so a pointer is allowed here.
test('the reading order view numbers each element on its own page, over its text, and goes when switched off', async () => {
  await page.click('#tagOrderToggle');
  await page.waitForFunction(() => document.querySelectorAll('.tag-order').length === 2, null, { timeout: 20000 });
  const badges = await page.evaluate(() => [...document.querySelectorAll('.tag-order')].map((b) => ({
    n: b.textContent, page: b.closest('.page')?.dataset.pageNumber,
  })));
  assert.deepEqual(badges, [{ n: '1', page: '1' }, { n: '2', page: '2' }], `the badges read ${JSON.stringify(badges)} — want 1 on page 1 and 2 on page 2`);
  const near = await page.evaluate(() => {
    const b = [...document.querySelectorAll('.tag-order')].find((x) => x.textContent === '1').getBoundingClientRect();
    const span = [...document.querySelectorAll('.viewerContainer:not([hidden]) .page .textLayer span')]
      .find((s) => s.textContent.includes('section 1') && s.getBoundingClientRect().width > 0);
    if (!span) return null;
    const t = span.getBoundingClientRect();
    const cx = (b.left + b.right) / 2, cy = (b.top + b.bottom) / 2;
    return { badge: [cx, cy], text: [t.left, t.top, t.right, t.bottom], near: cx >= t.left - 24 && cx <= t.right && cy >= t.top - 24 && cy <= t.bottom + 8 };
  });
  assert.ok(near, 'setup: page one has no laid-out span reading "section 1"');
  assert.ok(near.near, `badge 1 is not at its element's text: ${JSON.stringify(near)}`);
  await page.click('#tagOrderToggle');
  await page.waitForFunction(() => document.querySelectorAll('.tag-order').length === 0, null, { timeout: 5000 });
});

test('this file leaves the shared server as it found it', async () => {
  h.answerDialogs(true);
  assert.notEqual(foundHeld, null, 'setup: the first test never ran, so there is no baseline to return to');
  await h.closeDocument();
  assert.equal(await heldDocs(), foundHeld, 'the server holds a different number of documents than this file found');
});
