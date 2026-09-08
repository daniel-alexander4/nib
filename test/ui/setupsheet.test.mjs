// P03.S01 — the setup sheet, in a real browser.
//
// **This tier exists for one clause tier 2 cannot reach: the reader's page surviving the round
// trip.** jsdom lays nothing out, so a hidden-then-shown viewer is indistinguishable there from one
// that never moved. In a browser it is exactly the defect `/pending 372` recorded — *"nothing
// survives pdf.js re-laying the document out"*, because `currentPageNumber` runs
// `resetCurrentPageView` and `scrollIntoView` scrolls willingly — and `#viewerWrap` had never been
// hidden by anything before this slice.
//
// The second clause is the one the sheet exists for: the roster picker and its two fields are
// usable at 1024×768, with the sidebar still rendering the rail beside them.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const h = await launch();
const { page } = h;
after(() => h.browser.close());

const DOC = writeFixture('setupsheet.pdf', { pages: 6, label: 'setup page' });

// **The viewer's own SCROLL position, not the `.pageNum` input** — and the difference is a probe
// finding. `.pageNum` is written by the app on open and close (`app.js:2633`), not on scroll, so it
// reported the page it was last told about whatever the viewer did: removing the restore entirely
// left this test green. Scroll is a fact the layout owns and pdf.js resets.
const scrollTop = () => page.evaluate(() => {
  const c = document.querySelector('.viewerContainer:not([hidden])');
  return c ? Math.round(c.scrollTop) : -1;
});

test("the reader's page survives a trip through the setup sheet", async () => {
  await h.openDocument(DOC, 6);
  // Move off page 1, or "the page did not change" is true of a build that resets to 1.
  await page.evaluate(() => {
    const i = document.querySelector('.pageNum');
    i.value = '4';
    i.dispatchEvent(new Event('change', { bubbles: true }));
  });
  await page.waitForFunction(() => Number(document.querySelector('.pageNum')?.value) === 4);
  await page.waitForFunction(() => {
    const c = document.querySelector('.viewerContainer:not([hidden])');
    return c && c.scrollTop > 0;
  });
  const before = await scrollTop();
  assert.ok(before > 0,
    `setup: the viewer is scrolled to ${before}, so it never left page 1 and "the page survived" `
    + 'would be true of a build that resets to the top');

  await page.click('.modetab[data-tab="collaborate"]');
  // The mode lands on its Commands panel since v1.121.0, so the ceremony panel is asked for by
  // name rather than assumed open — the same helper `pageops.test.mjs` uses for thumbs.
  await h.panel('ceremony');
  await page.click('#ceremonyConveneBtn');
  await page.waitForSelector('#ceremonySheet:not([hidden])');
  // The viewer really did stand down — otherwise this asserts a round trip that never happened.
  assert.equal(await page.$eval('#viewerWrap', (el) => el.hidden), true,
    'setup: the viewer is still shown, so the sheet did not take its place and no re-layout occurs');

  await page.click('#cerSheetClose');
  await page.waitForSelector('#viewerWrap:not([hidden])');
  await page.waitForFunction(() => document.querySelector('.viewerContainer:not([hidden])'));

  const after = await scrollTop();
  // A tolerance, because a re-layout can land a page a pixel or two off and the clause is about the
  // reader's PLACE, not about an exact offset. A reset to the top is hundreds of pixels away.
  assert.ok(Math.abs(after - before) < 40,
    `the reader was ${before}px down the document and came back to ${after}px. Opening setup hides `
    + 'the viewer, and pdf.js re-lays the document out when it comes back — /pending 372 is that '
    + 'exact defect, and this is the first thing in the app that has ever hidden #viewerWrap');
});

test('the setup form has room at 1024x768, with the rail still beside it', async () => {
  await page.setViewportSize({ width: 1024, height: 768 });
  await page.click('.modetab[data-tab="collaborate"]');
  // The mode lands on its Commands panel since v1.121.0, so the ceremony panel is asked for by
  // name rather than assumed open — the same helper `pageops.test.mjs` uses for thumbs.
  await h.panel('ceremony');
  await page.click('#ceremonyConveneBtn');
  await page.waitForSelector('#ceremonySheet:not([hidden])');

  const box = await page.$eval('#ceremonySheet', (el) => {
    const r = el.getBoundingClientRect();
    return { w: Math.round(r.width), h: Math.round(r.height), left: Math.round(r.left) };
  });
  const side = await page.$eval('#sidebar', (el) => {
    const r = el.getBoundingClientRect();
    return { w: Math.round(r.width), visible: r.width > 0 && r.height > 0 };
  });

  // The whole point of the slice: more than a 200px column.
  assert.ok(box.w > 400,
    `the sheet is ${box.w}px wide at a 1024px viewport, which is not the room a roster picker and `
    + 'two fields were moved out of the sidebar to get');
  assert.ok(side.visible && side.w > 0,
    'the sidebar is gone while setup is open, so the running rail went with it — D3 keeps the rail '
    + 'because Running is document-referential and interrupt-driven');
  assert.ok(box.left >= side.w - 1,
    `the sheet starts at ${box.left}px and the sidebar is ${side.w}px wide, so they overlap — the `
    + 'sheet takes the document column, not the whole window');

  // Every control the convener has to reach is inside the viewport, not below its fold.
  const spill = await page.evaluate(() => {
    const ids = ['cerIntent', 'cerExpires', 'cerPeerPick', 'cerConveneGo'];
    return ids.filter((id) => {
      const el = document.getElementById(id);
      if (!el) return true;
      const r = el.getBoundingClientRect();
      return r.bottom > window.innerHeight || r.right > window.innerWidth;
    });
  });
  assert.deepEqual(spill, [],
    `these setup controls fall outside a 1024x768 viewport: ${spill.join(', ')}. The sheet exists `
    + 'so the roster, recital and deadline have room; controls below the fold is the 200px column '
    + 'problem in a wider box');
});

// **This file leaves the shared server as it found it**, which is this tier's own convention and
// not optional: the files share one server, so a document left open is counted by the next file as
// its own. Measured when this file first shipped without it — `stamplace.test.mjs` timed out at 30s
// waiting for a state my leftover document had already changed, and it had been green.
//
// The viewport is restored too. It is per-page rather than shared, but the second test above
// narrows it to 1024×768 and a later test in THIS file reading a wider layout would be measuring
// the previous test's leftovers.
test('this file leaves the shared server as it found it', async () => {
  await page.setViewportSize({ width: 1280, height: 900 });
  const openPages = (await h.counts()).pages;
  for (let i = 0; i < 8 && await page.$eval('#viewerWrap', (el) => el.className) === 'has-doc'; i++) {
    await h.closeDocument();
  }
  const left = await page.evaluate(() => document.querySelectorAll('.viewerContainer .page').length);

  assert.ok(openPages > 0, 'setup: no document was open, so this cleanup covered nothing');
  assert.equal(left, 0,
    `${left} page divs survive the close — the next file in this tier will count them as its own`);
});
