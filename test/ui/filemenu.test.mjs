// ADR-103 — tier 3: a document is closed on its tab, Close all sits beside the tabs, and an export
// with several formats writes the format chosen in the Save dialog.
//
// **Why tier 3.** Tier 2 (`test/jsdom/saveformats.test.mjs`, `toolbargroups`, `perview`) holds the
// dialog's logic and where each control lives, and says what it cannot see: a picture actually
// rendered (jsdom has no canvas, so "Pages as images" always FAILS there), and layout — that
// Close all is at the right-hand end of the strip's own row, and that Tab reaches it after the
// tabs. Those are here, against the real binary: the files written are read back from disk.
//
// ## What this cannot see
//
// - A screen reader announcing "Preparing…": the every-page render on a three-page fixture is too
//   quick to observe reliably, so the working state is tier 2's (it holds the render open).
// - Any browser but Chromium.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { launch, WORK, shutdown } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const A = writeFixture('menu-a.pdf', { pages: 3, label: 'ledger row' });
const B = writeFixture('menu-b.pdf', { pages: 2, label: 'second doc' });
const OUT_DIR = path.join(WORK, 'filemenu');
fs.mkdirSync(OUT_DIR, { recursive: true });

const h = await launch();
const { page } = h;

after(async () => {
  try {
    if (await page.evaluate(() => !document.getElementById('saveAsModal').hidden)) await page.click('#saveAsCancel');
    await h.closeAll();
  } catch { /* the assertion that already failed is the one worth reporting */ }
  await shutdown(h);
});

const toast = () => page.evaluate(() => document.getElementById('toast')?.textContent ?? '');

// exportAs presses an export button, optionally picks a format by its label, saves into OUT_DIR
// and returns the bytes written — read from DISK, which is the only place "what was written" is.
async function exportAs(button, format) {
  await h.mode('file');
  await h.card('Export & Print');
  await page.click(button);
  await page.waitForFunction(() => !document.getElementById('saveAsModal').hidden);
  assert.equal(await page.$eval('#saveAsFormatRow', (el) => el.hidden), false, `${button} opened a save dialog with no Format line`);
  if (format) await page.selectOption('#saveAsFormat', { label: format });
  const name = await page.inputValue('#saveAsName');
  await page.fill('#saveAsDir', OUT_DIR);
  await page.click('#saveAsGo');
  await page.waitForFunction(() => document.getElementById('saveAsModal').hidden, null, { timeout: 60000 });
  const out = path.join(OUT_DIR, name);
  const deadline = Date.now() + 15000;
  while (!fs.existsSync(out) && Date.now() < deadline) await page.waitForTimeout(200);
  assert.ok(fs.existsSync(out), `nothing was written to ${out}`);
  return { name, bytes: fs.readFileSync(out) };
}

test('File has no Close control, and the Export card is eight buttons', async () => {
  await h.openDocument(A, 3);
  await h.mode('file');
  const file = await page.evaluate(() => {
    const pane = document.getElementById('sbFunctions');
    const cards = [...pane.querySelectorAll('.sbhead.groupcard')].filter((x) => x.getBoundingClientRect().height > 0).map((x) => x.textContent.trim());
    // Every command a card holds, shown or folded away — a Close hidden inside a shut card is still one.
    const buttons = [...pane.querySelectorAll('.tbgroup button'), ...document.querySelectorAll('#toolbar button')].map((b) => b.textContent.trim());
    return { cards, close: buttons.filter((t) => /^Close( view| all)?$/.test(t)) };
  });
  assert.ok(file.cards.includes('Export & Print') && file.cards.includes('Save a Copy'),
    `setup: File mode's cards were not read (${file.cards.join(', ')}), so the absence below means nothing`);
  assert.equal(file.cards.includes('Close Document'), false, 'the Close Document card is back');
  assert.deepEqual(file.close, [], 'there is a Close control among the sidebar\'s or the bar\'s commands');

  await h.card('Export & Print');
  const shown = await page.evaluate(() => [...document.querySelectorAll('#sbFunctions .tbgroup.open > button')]
    .filter((b) => b.getBoundingClientRect().height > 0).map((b) => b.textContent.trim()));
  assert.deepEqual(shown, [
    'Print…', 'Pages as images…', 'Pictures in the document (ZIP)', 'Document text (.txt)',
    'This page\'s table…', 'Form data…', 'Split by bookmarks…', 'Split by page range…',
  ]);
});

test('pages as images writes a PNG of this page by default, and a ZIP of every page when chosen', async () => {
  const png = await exportAs('#exportPagesBtn');
  assert.equal(png.name, 'menu-a-page1.png');
  assert.equal(png.bytes.subarray(0, 8).toString('latin1'), '\x89PNG\r\n\x1a\n', 'the default format did not write a PNG');

  const zip = await exportAs('#exportPagesBtn', 'Every page (ZIP)');
  assert.equal(zip.name, 'menu-a-pages.zip', 'choosing the ZIP did not rename the file for it');
  assert.equal(zip.bytes.subarray(0, 2).toString('latin1'), 'PK', 'the file named .zip is not a ZIP — one format\'s bytes went out under the other\'s name');
  // Three pages in the fixture: three entries, each named in the archive's directory.
  assert.equal((zip.bytes.toString('latin1').match(/PK\x01\x02/g) || []).length, 3, 'the ZIP does not hold one picture per page');
});

test('this page\'s table writes the spreadsheet format chosen, under its extension', async () => {
  const xlsx = await exportAs('#exportTableBtn');
  assert.equal(xlsx.name, 'menu-a-p1-table.xlsx');
  assert.equal(xlsx.bytes.subarray(0, 2).toString('latin1'), 'PK', 'the default did not write an xlsx (a ZIP container)');

  const csv = await exportAs('#exportTableBtn', 'Comma-separated values (.csv)');
  assert.equal(csv.name, 'menu-a-p1-table.csv');
  assert.match(csv.bytes.toString('utf8'), /ledger row 1 of 3/, 'the file named .csv does not hold the page\'s text as CSV');
  assert.notEqual(csv.bytes.subarray(0, 2).toString('latin1'), 'PK', 'the xlsx bytes were written under the .csv name');
});

test('form data on a document with no form is refused before any dialog', async () => {
  await h.mode('file');
  await h.card('Export & Print');
  await page.click('#exportFormBtn');
  // The refusal's own words — not merely "a toast", since the last export's "Saved to …" may still be up.
  await page.waitForFunction(() => /could not export/i.test(document.getElementById('toast')?.textContent ?? ''), null, { timeout: 15000 });
  assert.equal(await page.$eval('#saveAsModal', (el) => el.hidden), true,
    `a document with no form opened the save dialog (the toast said: ${await toast()})`);
});

test('Close all appears beside the tabs at two documents, is reached after them, and closes both', async () => {
  assert.equal(await page.$eval('#closeAllBtn', (el) => el.getBoundingClientRect().width), 0,
    'Close all is showing with one document open — the tab\'s × is the same act');
  await h.openDocument(B, 2);
  await page.waitForFunction(() => document.querySelectorAll('#tabstrip .tab').length === 2);

  const g = await page.evaluate(() => {
    const r = (e) => e.getBoundingClientRect();
    const strip = document.getElementById('tabstrip'), all = document.getElementById('closeAllBtn');
    const last = strip.lastElementChild, col = document.getElementById('viewerCol');
    return {
      stripKids: [...strip.children].map((c) => c.getAttribute('role')),
      text: all.textContent.trim(), title: all.title,
      width: Math.round(r(all).width),
      sameRow: Math.abs(r(all).top - r(strip).top) <= 1 && Math.abs(r(all).bottom - r(strip).bottom) <= 1,
      rightOfTabs: r(all).left >= r(last).right - 1,
      atTheEnd: Math.abs(r(all).right - r(col).right) <= 1,
    };
  });
  assert.deepEqual(g.stripKids, ['tab', 'tab'], 'the strip holds something that is not a tab');
  assert.equal(g.text, 'Close all');
  assert.equal(g.title, 'Close every open document and return to the empty state');
  assert.ok(g.width > 0, 'Close all is not showing with two documents open');
  assert.equal(g.sameRow, true, 'Close all is not on the tab strip\'s row');
  assert.equal(g.rightOfTabs, true, 'Close all overlaps the tabs');
  assert.equal(g.atTheEnd, true, 'Close all is not at the right-hand end of the row');

  // Keyboard: Tab from the last tab's × lands on Close all, and it shows a focus ring.
  await page.focus('#tabstrip .tab:last-child .tabclose');
  await page.keyboard.press('Tab');
  const focus = await page.evaluate(() => {
    const a = document.activeElement, cs = getComputedStyle(a);
    return { id: a.id, outline: cs.outlineStyle, width: parseFloat(cs.outlineWidth) };
  });
  assert.equal(focus.id, 'closeAllBtn', 'Tab from the last tab does not reach Close all');
  assert.ok(focus.outline !== 'none' && focus.width >= 2, `Close all shows no focus ring when reached by keyboard (${focus.outline} ${focus.width}px)`);

  // A narrow window: the button keeps its width and the tabs keep most of the row.
  await page.setViewportSize({ width: 360, height: 700 });
  await page.waitForTimeout(200);
  const narrow = await page.evaluate(() => {
    const r = (e) => e.getBoundingClientRect();
    return { strip: Math.round(r(document.getElementById('tabstrip')).width), all: Math.round(r(document.getElementById('closeAllBtn')).width), row: Math.round(r(document.getElementById('tabrow')).width) };
  });
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.waitForTimeout(200);
  assert.ok(narrow.all > 0 && narrow.strip >= narrow.row / 2,
    `at 360px wide the tabs have ${narrow.strip}px of a ${narrow.row}px row beside a ${narrow.all}px Close all`);

  h.answerDialogs(true);
  await page.focus('#closeAllBtn');
  await page.keyboard.press('Enter');
  await h.documentClosed();
  assert.equal(await page.$$eval('#tabstrip .tab', (e) => e.length), 0, 'Close all left a tab');
  assert.equal(await page.$eval('#tabrow', (el) => el.getBoundingClientRect().height), 0, 'the strip\'s row is still showing with nothing open');
});

test('one open document is closed with its tab\'s ×, back to the empty state', async () => {
  await h.openDocument(A, 3);
  assert.equal(await page.$$eval('#tabstrip .tab', (e) => e.length), 1, 'setup: exactly one document is not open');
  assert.equal(await page.$eval('#tabstrip .tabclose', (el) => el.getAttribute('aria-label')), 'Close menu-a.pdf');
  h.answerDialogs(true);
  await page.click('#tabstrip .tab.active .tabclose');
  await h.documentClosed();
  const s = await page.evaluate(() => ({
    wrap: document.getElementById('viewerWrap').className,
    empty: document.getElementById('empty').textContent,
    stripHidden: document.getElementById('tabstrip').hidden,
    xs: document.querySelectorAll('.tabclose').length,
  }));
  assert.deepEqual(s, { wrap: '', empty: 'Open a PDF to begin.', stripHidden: true, xs: 0 });
});

test('no console errors but the refusal this file asked for', () => {
  // The no-form refusal IS a 500 from /api/form-data, and the browser logs every failed fetch.
  // Exactly that one: a second entry is something this file did not ask for.
  assert.deepEqual(h.consoleErrors, ['Failed to load resource: the server responded with a status of 500 (Internal Server Error)']);
});
