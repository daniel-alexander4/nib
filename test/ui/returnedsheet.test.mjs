// PLAN-returned-document P03.S01 — the sheet for a document that came back, in a real browser, by keyboard alone.
//
// Tier 2 covers which element is ASKED to take focus; jsdom does not blur on hide, so where focus actually LANDS
// after the sheet goes is only observable here (the same reason `parkCeremonySheet` records). And the sheet hides
// `#viewerWrap`, which `/pending 372` found pdf.js re-lays out — so the reader's place is asserted too.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch, shutdown } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const h = await launch();
const { page } = h;
after(() => shutdown(h));

const DOC = writeFixture('returnedsheet.pdf', { pages: 6, label: 'returned page' });
// '#id' for the command, 'twin' for its data-forward alias — the two must not read alike.
const active = () => page.evaluate(() => {
  const a = document.activeElement;
  if (!a) return '';
  if (a.id) return '#' + a.id;
  return a.dataset.forward === 'returnedBtn' ? 'twin' : a.tagName;
});
const scrollTop = () => page.evaluate(() => {
  const c = document.querySelector('.viewerContainer:not([hidden])');
  return c ? Math.round(c.scrollTop) : -1;
});

test('by keyboard: Enter opens the sheet with focus inside, Escape leaves it with focus back on the command', async () => {
  await h.openDocument(DOC, 6);
  await page.evaluate(() => {
    const i = document.querySelector('.pageNum');
    i.value = '4';
    i.dispatchEvent(new Event('change', { bubbles: true }));
  });
  await page.waitForFunction(() => {
    const c = document.querySelector('.viewerContainer:not([hidden])');
    return c && c.scrollTop > 0;
  });
  const before = await scrollTop();
  assert.ok(before > 0, `setup: the viewer is at ${before}, so "the place survived" would hold of a reset`);

  // The command is a card's button, and a card is a collapsible group — reached as a user reaches it.
  await h.mode('secure');
  await h.group('Sign & Timestamp');
  await page.focus('#returnedBtn');
  assert.equal(await active(), '#returnedBtn', 'setup: the command cannot take focus, so nothing below is by keyboard');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#returnedSheet:not([hidden])');
  assert.equal(await page.$eval('#viewerWrap', (el) => el.hidden), true, 'the sheet did not stand in place of the viewer');
  assert.equal(await active(), '#returnedHeading', 'focus did not land inside the sheet');

  await page.keyboard.press('Escape');
  await page.waitForSelector('#returnedSheet', { state: 'hidden' });
  await page.waitForFunction(() => document.querySelector('.viewerContainer:not([hidden])'));
  assert.equal(await active(), '#returnedBtn', 'focus did not land back on the command after the sheet went');
  const after = await scrollTop();
  assert.ok(Math.abs(after - before) < 40, `the reader was ${before}px down and came back to ${after}px`);
});

test('by keyboard from the Send & Receive twin: "Back to the document" returns focus to the twin', async () => {
  await h.mode('collaborate');
  await h.group('Send & Receive');
  await page.focus('[data-forward="returnedBtn"]');
  assert.equal(await active(), 'twin', 'setup: the twin cannot take focus');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#returnedSheet:not([hidden])');
  assert.equal(await active(), '#returnedHeading');
  // Tab to the one way out, and press it.
  await page.keyboard.press('Tab');
  assert.equal(await active(), '#returnedClose', 'Tab from the heading does not reach "Back to the document"');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#returnedSheet', { state: 'hidden' });
  assert.equal(await active(), 'twin', 'focus did not land back on the twin that opened the sheet');
});
