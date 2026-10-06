// Drag to pan, and Resume last session (ADR-085) — the halves only a real browser has.
//
// **Why tier 3.** A pan is a scroll offset following a pointer, and every scroll dimension at
// tier 2 is 0: test/jsdom/pan.test.mjs writes the overflow onto the box by hand and so proves
// WHOSE drag it is, never that a page moves. Here the page is zoomed until it really overflows,
// a real mouse drags it, and the distance scrolled is compared with the distance dragged. The
// hand cursor is the same story — `.can-pan` is kept by a ResizeObserver jsdom does not have.
//
// Resume is here for its first half: the record is written when the last window's stream DROPS,
// which needs a real window dropping a real socket. A reload is that, and it is also the case the
// session boundary must not move — the documents are still there when the page comes back.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { launch, shutdown, WORK } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const A = writeFixture('pan-a.pdf', { pages: 3, label: 'pan A page' });
const B = writeFixture('pan-b.pdf', { pages: 2, label: 'pan B page' });

const h = await launch();
const page = h.page;

after(async () => { await shutdown(h); });

const BOX = '.viewerContainer:not([hidden])';
const scroll = () => page.$eval(BOX, (el) => ({ left: el.scrollLeft, top: el.scrollTop, w: el.clientWidth, sw: el.scrollWidth }));
const selected = () => page.evaluate(() => String(window.getSelection()));

// A point on the first page's blank paper and one on its run of text, both inside the viewport,
// each checked against what is actually under it — a "blank" point that lands on a text span
// would make the pan assertion pass or fail for the wrong reason.
async function points() {
  return page.evaluate((sel) => {
    const box = document.querySelector(sel).getBoundingClientRect();
    const span = [...document.querySelectorAll(sel + ' .textLayer span')].find((s) => s.textContent.trim());
    const sr = span.getBoundingClientRect();
    const text = { x: sr.left + 6, y: sr.top + sr.height / 2 };
    const blank = { x: box.left + box.width / 2, y: box.top + box.height * 0.75 };
    const under = (p) => document.elementFromPoint(p.x, p.y);
    return {
      text, blank,
      textIsText: !!under(text)?.closest('.textLayer span'),
      blankIsBlank: !!under(blank)?.closest('.page') && !under(blank).closest('.textLayer span'),
      blankCursor: getComputedStyle(under(blank)).cursor,
      textCursor: getComputedStyle(under(text)).cursor,
    };
  }, BOX);
}

test('zoomed in, the page is dragged by the hand and text is still selected by the I-beam', async () => {
  await h.openDocument(A, 3);
  for (let i = 0; i < 6; i++) await page.$eval('#zoomInBtn', (el) => el.click());
  await page.waitForFunction((sel) => {
    const el = document.querySelector(sel);
    return el.scrollWidth > el.clientWidth + 200 && el.classList.contains('can-pan');
  }, BOX);
  // Scroll to where the text is on screen and there is room to move both ways.
  await page.$eval(BOX, (el) => { el.scrollLeft = 150; el.scrollTop = 120; });

  const p = await points();
  assert.equal(p.blankIsBlank, true, 'setup: the "blank" point is not on blank paper');
  assert.equal(p.textIsText, true, 'setup: the "text" point is not on a run of text');
  assert.equal(p.blankCursor, 'grab', 'the page overflows its view and the pointer over it is not the hand');
  assert.equal(p.textCursor, 'text', 'the hand replaced the I-beam over text');

  // The pan: 120 right and 90 down, so the view moves 120 left and 90 up.
  const before = await scroll();
  await page.mouse.move(p.blank.x, p.blank.y);
  await page.mouse.down();
  await page.mouse.move(p.blank.x + 120, p.blank.y + 90, { steps: 6 });
  const held = await page.$eval(BOX, (el) => el.classList.contains('panning'));
  await page.mouse.up();
  const after = await scroll();
  assert.equal(held, true, 'the closed hand was not shown while the page was held');
  assert.deepEqual({ dx: after.left - before.left, dy: after.top - before.top }, { dx: -120, dy: -90 },
    'the page did not move by the distance the pointer did');
  assert.equal(await selected(), '', 'dragging the page selected text on it');
  assert.equal(await page.$eval(BOX, (el) => el.classList.contains('panning')), false);

  // The same drag starting on the text selects and moves nothing.
  const q = await points();
  const b2 = await scroll();
  await page.mouse.move(q.text.x, q.text.y);
  await page.mouse.down();
  await page.mouse.move(q.text.x + 160, q.text.y + 2, { steps: 6 });
  await page.mouse.up();
  const a2 = await scroll();
  assert.deepEqual({ dx: a2.left - b2.left, dy: a2.top - b2.top }, { dx: 0, dy: 0 }, 'a drag across text scrolled the page');
  assert.notEqual(await selected(), '', 'a drag across text selected nothing — the pan took text selection away');
  await page.evaluate(() => window.getSelection().removeAllRanges());

  // With a tool armed the press is the tool's: Redact draws a box and the page stays put.
  await h.mode('secure');
  await page.$eval('#redactBtn', (el) => el.click());
  await page.waitForFunction(() => document.getElementById('viewerWrap').hasAttribute('data-no-pan'));
  const r = await points();
  assert.notEqual(r.blankCursor, 'grab', 'Redact is armed and the page still offers the hand');
  const b3 = await scroll();
  await page.mouse.move(r.blank.x, r.blank.y);
  await page.mouse.down();
  await page.mouse.move(r.blank.x + 80, r.blank.y + 60, { steps: 4 });
  await page.mouse.up();
  const a3 = await scroll();
  assert.deepEqual({ dx: a3.left - b3.left, dy: a3.top - b3.top }, { dx: 0, dy: 0 }, 'a drag with Redact armed scrolled the page');
  assert.equal(await page.$$eval(BOX + ' .redactmark', (e) => e.length), 1, 'Redact was armed and the drag drew no box');
  await page.$eval('#redactBtn', (el) => el.click());
});

test('a reload keeps its documents, and what was open is offered back from the launch state', async () => {
  await h.openDocument(B, 2);
  assert.equal(await page.$$eval('#tabstrip .tab', (e) => e.length), 2, 'setup: two documents are not open');

  // The reload: this window's stream drops (the last window going, which records) and comes back.
  await page.reload();
  await page.waitForFunction(() => document.querySelectorAll('#tabstrip .tab').length === 2);

  // Close all through the app's own button: `h.closeAll` empties the SERVER behind the page's back,
  // which is right for a cleanup and would leave this page showing two stale tabs.
  await page.$eval('#closeAllBtn', (el) => el.click());
  await page.waitForFunction(() => document.getElementById('viewerWrap').className === '');
  await page.waitForSelector('#resumeBtn:not([hidden])');
  assert.equal(await page.$eval('#resumeBtn', (el) => el.textContent), 'Resume last session — 2 documents');
  assert.equal(await page.$eval('#resumeBtn', (el) => el.title), 'pan-a.pdf, pan-b.pdf');

  await page.click('#resumeBtn');
  // Waited on by NAME: a tab exists a moment before its document's name lands in it.
  const names = () => page.$$eval('#tabstrip .tab', (e) => e.map((t) => (/pan-[ab]\.pdf/.exec(t.textContent) || ['?'])[0]));
  await page.waitForFunction(() => [...document.querySelectorAll('#tabstrip .tab')].filter((t) => /pan-[ab]\.pdf/.test(t.textContent)).length === 2);
  assert.deepEqual(await names(), ['pan-a.pdf', 'pan-b.pdf'], 'the resumed documents are not the recorded ones, in tab order');
});

// ADR-086. The launch is the REAL binary, started the way a double-click starts it, against the Nib
// this window belongs to: it must hand the file over, exit, and ask for no window — and the window
// that is already open must show the document without being reloaded.
test('a second launch arrives as a tab in the window that is already open', async () => {
  const C = writeFixture('pan-c.pdf', { pages: 1, label: 'pan C page' });
  const count = () => page.$$eval('#tabstrip .tab', (e) => e.length);
  const before = await count();
  assert.ok(before >= 1, 'setup: no document is open, so "a tab beside the others" is not what this would show');
  const loads = await page.evaluate(() => performance.getEntriesByType('navigation').length);

  const out = spawnSync(path.join(WORK, 'nib'), [C], {
    env: { ...process.env, HOME: path.join(WORK, 'home'), XDG_CONFIG_HOME: path.join(WORK, 'config'), NIB_NO_BROWSER: '1', NIB_NO_UPDATE_CHECK: '1' },
    encoding: 'utf8', timeout: 60000,
  });
  assert.equal(out.status, 0, 'the second launch did not hand off and exit:\n' + out.stderr);
  assert.match(out.stderr, /its open window has the document/, 'the launch was not told a window has it:\n' + out.stderr);
  // Headless, "opening a window" is logging its URL; a launch that still surfaces one logs this.
  assert.doesNotMatch(out.stderr, /open Nib at/, 'the launch asked for a second window:\n' + out.stderr);

  await page.waitForFunction((n) => document.querySelectorAll('#tabstrip .tab').length === n + 1, before);
  await page.waitForFunction(() => /pan-c\.pdf/.test(document.querySelector('#tabstrip .tab[aria-selected="true"]')?.textContent || ''));
  assert.equal(await page.evaluate(() => performance.getEntriesByType('navigation').length), loads, 'the window was reloaded to show it');
});
