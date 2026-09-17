// The autotagger's review, in a real browser — `PLAN-accessibility.md` P08.S06c.
//
// ── What only this tier can see ──────────────────────────────────────────────
// Tier 2 proves the review a person builds is the review that is sent. It cannot prove the outline
// lands on the element — its viewer has no rendered page — or that the native controls really answer
// the keyboard in a browser, or that a commit reaches the accessibility report as an inferred tree.
// This file drives all three against the real binary.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const DOC = writeFixture('tagsreview.pdf', { pages: 2, label: 'section' });

const h = await launch();
const { page } = h;

const pageDivs = () => page.evaluate(() => document.querySelectorAll('.viewerContainer .page').length);
// The documents the SERVER holds — what this file must leave as it found it. Counted there and not in
// page divs, measured: this file found zero page divs, closed its document, and then saw three, because
// the server still held a document an earlier file in the run leaked (`/pending 474`) and the app
// activated it once this file's closed. Page divs cannot tell that from a leak of this file's own.
const heldDocs = () => page.evaluate(async () => (await (await fetch('/api/docs')).json()).docs.length);
let foundHeld = null;

// A review left open by a failed assertion covers the page and intercepts every later click, so the
// cleanups close it first — otherwise one failure here reports as a second, unrelated one.
async function closeReviewIfOpen() {
  if (await page.evaluate(() => !document.getElementById('tagsModal').hidden)) await page.click('#tagsClose');
  if (await page.evaluate(() => !document.getElementById('uaModal').hidden)) await page.click('#uaClose');
}

after(async () => {
  try {
    await closeReviewIfOpen();
    h.answerDialogs(true);
    for (let i = 0; i < 8 && await page.$eval('#viewerWrap', (el) => el.className) === 'has-doc'; i++) {
      await h.closeDocument();
    }
  } catch { /* the assertion that already failed is the one worth reporting */ }
  await h.browser.close();
});

test('the review proposes, outlines on focus, and commits from the keyboard', async () => {
  foundHeld = await heldDocs();
  await h.openDocument(DOC, 2);
  await h.mode('accessibility'); // ADR-035
  await h.group('Tag Structure');
  await page.click('#tagsBtn');
  await page.waitForFunction(() => document.querySelectorAll('#tagsList .tags-row').length > 0, null, { timeout: 20000 });
  const texts = await page.$$eval('#tagsList .tags-row .tags-text', (rs) => rs.map((r) => r.textContent));
  assert.equal(texts.length, 2, `one proposed paragraph per page, got ${JSON.stringify(texts)}`);

  // Keyboard focus — not a click — on the first row, and its element is outlined over its own text.
  await page.focus('#tagsList .tags-row:first-child select');
  await page.waitForSelector('.tag-outline');
  const geometry = await page.evaluate(() => {
    const o = document.querySelector('.tag-outline').getBoundingClientRect();
    const span = [...document.querySelectorAll('.viewerContainer:not([hidden]) .page .textLayer span')]
      .find((s) => s.textContent.includes('section 1'));
    if (!span) return null;
    const t = span.getBoundingClientRect();
    return {
      outline: [o.left, o.top, o.right, o.bottom].map(Math.round),
      text: [t.left, t.top, t.right, t.bottom].map(Math.round),
      overlaps: o.left < t.right && o.right > t.left && o.top < t.bottom && o.bottom > t.top,
    };
  });
  assert.ok(geometry, 'setup: page one has no text-layer span reading "section 1" to compare the outline with');
  assert.ok(geometry.overlaps, `the outline does not cover its element's text: ${JSON.stringify(geometry)}`);

  // Retype with the arrow key on the focused select: Paragraph → Heading 6, the option above it.
  const before = await page.$eval('#tagsList .tags-row:first-child select', (s) => s.value);
  assert.equal(before, 'P', `setup: the first row was proposed as ${before}`);
  await page.keyboard.press('ArrowUp');
  await page.waitForFunction(() => document.querySelector('#tagsList .tags-row:first-child select')?.value === 'H6');

  // Ignore the second row with Space on its checkbox.
  await page.focus('#tagsList .tags-row:nth-child(2) input[type="checkbox"]');
  await page.keyboard.press('Space');
  await page.waitForFunction(() => document.querySelector('#tagsList .tags-row:nth-child(2)')?.classList.contains('tags-ignored'));

  // Commit with Enter.
  await page.focus('#tagsCommit');
  await page.keyboard.press('Enter');
  await page.waitForFunction(() => document.getElementById('tagsModal').hidden, null, { timeout: 20000 });
  assert.equal(await page.$$eval('.tag-outline', (els) => els.length), 0, 'the outline outlived the review');

  // The report now says the structure was inferred.
  await h.mode('accessibility'); // ADR-035: the report left Secure's Protect & Inspect for its own card
  await h.group('Check Accessibility');
  await page.click('#uaBtn');
  await page.waitForSelector('#uaBody .ua-provenance', { timeout: 20000 });
  const provenance = await page.$eval('#uaBody .ua-provenance', (el) => el.textContent);
  assert.match(provenance, /inferred/, `after a commit the report says: "${provenance}"`);
  await page.click('#uaClose');
});

test('this file leaves the shared server as it found it', async () => {
  await closeReviewIfOpen();
  h.answerDialogs(true);
  assert.notEqual(foundHeld, null, 'setup: the first test never ran, so there is no baseline to return to');
  const heldOpen = await heldDocs();
  assert.equal(heldOpen, foundHeld + 1, `setup: the server holds ${heldOpen} document(s) against the ${foundHeld} this file found — this file's document is not the one open`);
  await h.closeDocument();
  const held = await heldDocs();
  const left = await pageDivs();
  // Said in full when it fails: which views and tabs survived, and how many pages each holds — a
  // count alone cannot tell a close that did not happen from a view the commit duplicated.
  const state = await page.evaluate(() => ({
    containers: [...document.querySelectorAll('.viewerContainer')].map((c) => ({
      hidden: c.hidden, pages: c.querySelectorAll('.page').length,
    })),
    tabs: [...document.querySelectorAll('#tabstrip .tab')].map((t) => t.textContent.trim()),
    wrap: document.getElementById('viewerWrap').className,
  }));
  assert.equal(held, foundHeld,
    `the server holds ${held} document(s) after this file closed its own, and it found ${foundHeld} — the next file will inherit the difference. State: ${JSON.stringify({ ...state, pageDivs: left })}`);
});
