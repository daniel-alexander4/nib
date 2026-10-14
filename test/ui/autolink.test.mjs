// A page keeps its links through a rotation and a resize that land while it is drawing (/pending 887, ADR-167).
//
// pdf.js makes a page's links in two steps. The `/Link` annotations the file declares come with the
// annotation layer. A web address that is only text is found afterwards, in the text layer, and added
// by a later step that waits for that layer. A page redrawn while its text layer is still rendering —
// the window resized just after a rotation reloaded the document — keeps both layers, and the second
// drawing aborts the first one's text. pdf.js treats the abort as the wait being over, so the FIRST
// drawing's step runs, reads the second drawing's still-empty text, finds no addresses and marks the
// layer as done; the second drawing's own step is then skipped, and the page has no text links until it
// is thrown away and drawn afresh. Nothing is logged.
//
// Measured with this file's stimulus, rounds that left page 1 without its text link: 22 of 40 on pdf.js
// 6.0.227, 15 of 40 on 6.1.200, 11 of 30 on 6.4.299 — against 0 of 40 with the rotation alone and 0 of
// 60 with the resize alone. So it is not fixed upstream, and Nib's vendored viewer carries two lines
// that are (ADR-167). This file is what holds them: it is red on the viewer as published.
//
// jsdom cannot see any of this: tier 2 stubs the viewer.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch, shutdown } from './harness.mjs';
import { makeLinkPDF, writeRawFixture } from './fixtures.mjs';

const PAGES = 3;
const DOC = writeRawFixture('autolink.pdf', makeLinkPDF({ pages: PAGES }));

const h = await launch();
const page = h.page;

after(async () => { await shutdown(h); });

// One reading per page on screen: is it drawn, and how many of each kind of link does it hold.
const links = () => page.evaluate(() => [...document.querySelectorAll('.viewerContainer:not([hidden]) .page')].map((p) => ({
  n: Number(p.dataset.pageNumber),
  drawn: p.dataset.loaded === 'true',
  declared: p.querySelectorAll('.annotationLayer a[href*="example.com/real"]').length,
  fromText: p.querySelectorAll('.annotationLayer a[href*="example.com/auto"]').length,
})));

// settled reads until two readings 400 ms apart agree and a page is drawn. The late step runs after
// the page reports itself drawn, so "drawn" alone would read a page before its text links could exist.
async function settled() {
  let prev = '';
  for (let i = 0; i < 75; i++) {
    const now = await links();
    const s = JSON.stringify(now);
    if (s === prev && now.some((p) => p.drawn)) return now;
    prev = s;
    await page.waitForTimeout(400);
  }
  throw new Error('the pages never stopped changing: ' + prev);
}

test('a page rotated and then resized while it draws keeps the link its file declares and the one found in its text', async () => {
  await h.openDocument(DOC, PAGES);
  // Stimulus: both kinds are there to lose. Without this, "none was lost" is true of a build that
  // never made a text link at all.
  const first = (await settled()).filter((p) => p.drawn);
  assert.ok(first.length > 0, 'setup: no page was drawn');
  for (const p of first) {
    assert.equal(p.declared, 1, `setup: page ${p.n} does not show its declared link`);
    assert.equal(p.fromText, 1, `setup: page ${p.n} has no link made from its text, so there is nothing for the race to lose`);
  }

  // Twelve rounds with the two waits walking through different values: the loss was 37% a round on
  // the published 6.1.200 viewer, so twelve clean rounds there are about one run in two hundred and eighty.
  for (let i = 0; i < 12; i++) {
    await page.$eval('#rotateRightBtn', (el) => el.click());
    await page.waitForTimeout((i * 17) % 250);
    await page.setViewportSize({ width: 375, height: 667 });
    await page.waitForTimeout((i * 13) % 120);
    await page.setViewportSize({ width: 1366, height: 768 });
    await settled();
    // A page can report itself drawn a moment before its layers are filled, so a reading with a link
    // missing is given five seconds to become whole. A link lost to the race never comes back.
    await page.waitForFunction(() => {
      const drawn = [...document.querySelectorAll('.viewerContainer:not([hidden]) .page')].filter((p) => p.dataset.loaded === 'true');
      return drawn.length > 0 && drawn.every((p) => p.querySelector('.annotationLayer a[href*="example.com/real"]') && p.querySelector('.annotationLayer a[href*="example.com/auto"]'));
    }, null, { timeout: 5000 }).catch(() => {});
    const after = (await links()).filter((p) => p.drawn);
    assert.ok(after.length > 0, `round ${i + 1}: no page was drawn after the resize`);
    for (const p of after) {
      assert.equal(p.declared, 1, `round ${i + 1}: page ${p.n} lost the link its file declares`);
      assert.equal(p.fromText, 1, `round ${i + 1}: page ${p.n} lost the link made from its text`);
    }
  }
  assert.deepEqual(h.consoleErrors, []);
});
