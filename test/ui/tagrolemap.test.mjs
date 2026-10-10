// Changing what a custom tag type means, and undoing it, by keyboard in a real browser — ADR-127.
//
// ── What only this tier can see ──────────────────────────────────────────────
// Tier 2 proves each button of the Role map section sends its one edit and where focus is asked to land. It
// cannot prove a person can DO it: that Tab reaches the section's summary, that Enter opens a native
// details, that the row's picker answers typing and does NOT apply on its own, that the button next to it
// does, that the real server then resolves the tag typed with that name to the new type — the line in the
// tree changing under the person's eyes — and that Ctrl+Z restores it. This file drives that against the
// real binary, on a file that really has a role map.
//
// ── The rule this file lives by ──────────────────────────────────────────────
// The SETUP — opening the document and the panel — uses the harness: it is not the claim. Everything
// between the two markers below uses the keyboard alone. The last-but-one test scans that region of this
// file and fails if a pointer or a harness shortcut appears there.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { launch, shutdown } from './harness.mjs';
import { makeInlineTaggedPDF, writeRawFixture } from './fixtures.mjs';

const DOC = writeRawFixture('tagrolemap.pdf', makeInlineTaggedPDF());

const h = await launch();
const { page } = h;

const heldDocs = () => page.evaluate(async () => (await (await nibFetch('/api/docs')).json()).docs.length);
let foundHeld = null;

// serverTree is what the server reads now: each tag's type as written and as resolved, and the role map.
const serverTree = () => page.evaluate(async () => {
  const t = await (await nibFetch('/api/tags/tree')).json();
  return { tags: t.elements.map((e) => [e.kind, e.standard, e.text]), roles: t.roleMap.map((m) => [m.name, m.to, m.standard, m.elements]) };
});
const shownTree = () => page.evaluate(() => [...document.querySelectorAll('#tagTreeList [role="treeitem"]')].map((li) => li.textContent));
const rowSelect = '#tagRoleList li[data-role="Heading 1"] select';
const rowChange = '#tagRoleList li[data-role="Heading 1"] .tagRoleChange';
const roleStatus = (re) => page.waitForFunction((src) => new RegExp(src).test(document.getElementById('tagRoleStatus').textContent), re.source, { timeout: 20000 });

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

test('setup: a document whose heading is typed with a custom name the role map explains, and the tree panel open', async () => {
  foundHeld = await heldDocs();
  await h.openDocument(DOC, 1);
  await h.mode('accessibility'); // ADR-035
  await page.click('.tab[data-panel="tagtree"]');
  await page.waitForFunction(() => document.querySelectorAll('#tagTreeList [role="treeitem"]').length === 4, null, { timeout: 20000 });
  start = await serverTree();
  startShown = await shownTree();
  assert.deepEqual(start.tags[1], ['Heading 1', 'H1', 'Title'], `setup: the heading does not read Heading 1 as H1: ${JSON.stringify(start.tags)}`);
  assert.deepEqual(start.roles, [['Heading 1', 'H1', 'H1', 1], ['Unused', 'Note', 'Note', 0]], 'setup: the role map is not the two mappings the file has');
  assert.match(startShown[1], /^H1 \(Heading 1\)/, 'setup: the panel does not show the heading under both names');
});

async function tabTo(selector, max = 80) {
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
test('Tab reaches the Role map summary, which says how many types are mapped, and Enter opens it', async () => {
  const header = await page.evaluate(() => {
    document.querySelector('.tab[data-panel="tagtree"]').focus();
    return document.activeElement?.dataset?.panel;
  });
  assert.equal(header, 'tagtree', 'setup: the panel header cannot take focus');
  const closed = await page.evaluate(() => {
    const d = document.getElementById('tagRoleMap');
    // checkVisibility, not a box: a closed details skips its content's rendering, and asking for a box lays it out.
    return { open: d.open, summary: document.getElementById('tagRoleSummary').textContent, rowsOnScreen: document.querySelector('#tagRoleList').checkVisibility() };
  });
  assert.deepEqual(closed, { open: false, summary: 'Role map — 2 custom types mapped', rowsOnScreen: false },
    'the section is not closed at first, or its summary does not count the mappings');
  const toSummary = await tabTo('#tagRoleSummary');
  assert.ok(toSummary.ok, `Tab from the panel header never reached the Role map summary: ${toSummary.seen.map((s) => s.id).join(' → ')}`);
  await page.keyboard.press('Enter');
  await page.waitForFunction(() => document.getElementById('tagRoleMap').open, null, { timeout: 5000 });
  const rows = await page.evaluate(() => [...document.querySelectorAll('#tagRoleList li')].map((li) => {
    const r = li.getBoundingClientRect();
    return { role: li.dataset.role, to: li.querySelector('select').value, removable: !li.querySelector('.tagRoleRemove').disabled, note: li.querySelector('p').textContent, shown: li.checkVisibility() && r.width > 0 && r.height > 0 };
  }));
  assert.deepEqual(rows.map((r) => [r.role, r.to, r.removable, r.shown]), [['Heading 1', 'H1', false, true], ['Unused', 'Note', true, true]],
    `the rows are not the two mappings, on screen, with only the unused one removable: ${JSON.stringify(rows)}`);
  assert.match(rows[0].note, /1 tag is of this type.*cannot be removed/, 'the row a tag uses does not say why its mapping cannot be removed');
});

test('typing in the row\'s picker changes nothing; Enter on Change does, and the heading\'s line in the tree reads the new type', async () => {
  const toSelect = await tabTo(rowSelect);
  assert.ok(toSelect.ok, `Tab from the summary never reached the row's picker: ${toSelect.seen.map((s) => s.id).join(' → ')}`);
  assert.equal(toSelect.seen.length, 1, `the first row's picker is not the next stop after the summary: ${toSelect.seen.map((s) => s.id).join(' → ')}`);
  await page.keyboard.type('Heading 2');
  await page.waitForFunction((s) => document.querySelector(s).value === 'H2', rowSelect, { timeout: 5000 });
  assert.deepEqual(await serverTree(), start, 'choosing in the picker changed the document before any button was pressed');

  const toChange = await tabTo(rowChange);
  assert.ok(toChange.ok && toChange.seen.length === 1, `Change is not the next stop after the picker: ${toChange.seen.map((s) => s.id).join(' → ')}`);
  await page.keyboard.press('Enter');
  await roleStatus(/Heading 1 now means H2/);

  const now = await serverTree();
  assert.deepEqual(now.tags[1], ['Heading 1', 'H2', 'Title'], 'the heading is not the same tag resolving to H2');
  assert.deepEqual(now.roles[0], ['Heading 1', 'H2', 'H2', 1], 'the role map does not send Heading 1 to H2');
  assert.deepEqual(now.tags.filter((_, i) => i !== 1), start.tags.filter((_, i) => i !== 1), 'another tag reads differently');
  const shown = await shownTree();
  assert.match(shown[1], /^H2 \(Heading 1\)/, `the heading's line in the tree reads ${JSON.stringify(shown[1])}`);
  assert.deepEqual(shown.filter((_, i) => i !== 1), startShown.filter((_, i) => i !== 1), 'another line of the tree changed');
  const at = await page.evaluate((s) => ({ onChange: document.activeElement === document.querySelector(s), open: document.getElementById('tagRoleMap').open }), rowChange);
  assert.deepEqual(at, { onChange: true, open: true }, 'after the re-read focus is not on the row\'s Change button, or the section closed');
  assert.match(await page.evaluate(() => document.getElementById('tagRoleStatus').textContent), /Ctrl\+Z takes it back/, 'the status does not say it can be undone');
});

test('a standard type typed as the name is refused by the server in its own words, and what was typed stays', async () => {
  // The Add row, with a standard type as the name: the server's refusal, shown, and nothing changed.
  const toName = await tabTo('#tagRoleNewName');
  assert.ok(toName.ok, `Tab never reached the name field: ${toName.seen.map((s) => s.id).join(' → ')}`);
  const before = await serverTree();
  await page.keyboard.type('H1');
  await page.keyboard.press('Enter');
  await roleStatus(/H1 is a standard structure type/);
  assert.deepEqual(await serverTree(), before, 'a refused mapping changed the document');
  assert.equal(await page.evaluate(() => document.getElementById('tagRoleNewName').value), 'H1', 'a refusal emptied the field');
  await page.keyboard.press('Control+a');
  await page.keyboard.press('Backspace');
});

test('Ctrl+Z restores the mapping, the heading\'s type and its line in the tree', async () => {
  // Out of the text field first: Ctrl+Z in a field is the field's own undo.
  await page.keyboard.press('Shift+Tab');
  await page.keyboard.press('Control+z');
  await page.waitForFunction(async () => (await (await nibFetch('/api/tags/tree')).json()).elements[1].standard === 'H1', null, { timeout: 20000 });
  assert.deepEqual(await serverTree(), start, 'after the undo the server does not read the tree and the role map it read at the start');
  await page.waitForFunction((want) => JSON.stringify([...document.querySelectorAll('#tagTreeList [role="treeitem"]')].map((li) => li.textContent)) === want,
    JSON.stringify(startShown), { timeout: 20000 });
  assert.equal(await page.evaluate((s) => document.querySelector(s).value, rowSelect), 'H1', 'the row\'s picker does not show the restored mapping');
});
// ── keyboard only, to here ───────────────────────────────────────────────────

test('the region between the markers used no pointer at all', () => {
  const src = readFileSync(new URL('./tagrolemap.test.mjs', import.meta.url), 'utf8');
  const START = '// ── keyboard only, from here';
  const STOP = '// ── keyboard only, to here';
  const body = src.slice(src.indexOf(START), src.indexOf(STOP));
  assert.ok(body.length > 4000, `the scanned region is ${body.length} chars — a marker has drifted`);
  for (const name of ['Tab reaches the Role map summary', 'Enter on Change does', 'refused by the server', 'Ctrl+Z restores the mapping']) {
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
