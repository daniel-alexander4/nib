// A zoom holds what the user is looking at (/pending 850) — the half only a real browser has.
//
// **Why tier 3.** "The point under the pointer is still under the pointer" is two bounding
// rectangles and a scroll offset, and at tier 2 every one of them is 0. Here a real page overflows
// a real container, a real wheel turns with Ctrl down, and the point is read back off the page.
//
// **What it measures, and why not the scroll offset.** The anchor is a FRACTION of a page's own
// rectangle, read before the zoom and located again after it. A test that compared scrollLeft with
// arithmetic of its own would restate the code's formula and agree with it either way; this one
// asks only where the paper went.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch, shutdown } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const DOC = writeFixture('zoomanchor.pdf', { pages: 3, label: 'zoom anchor page' });

const h = await launch();
const page = h.page;

after(async () => { await shutdown(h); });

const BOX = '.viewerContainer:not([hidden])';
// Within this many CSS pixels the point has not moved. pdf.js rounds a scale to two decimals and
// a scroll offset is a whole pixel at this device ratio, so the slack is rounding, nothing else:
// the defect this file was written against moved the point by 40 to 110 pixels a notch.
const HELD = 3;

const box = () => page.$eval(BOX, (el) => {
  const r = el.getBoundingClientRect();
  return { left: r.left, top: r.top, width: el.clientWidth, height: el.clientHeight, sl: el.scrollLeft, st: el.scrollTop, sw: el.scrollWidth, sh: el.scrollHeight };
});
// The page under a window point, and that point as a fraction of the page's rectangle.
const anchorAt = (x, y) => page.evaluate(({ sel, x, y }) => {
  const div = document.elementFromPoint(x, y)?.closest(sel + ' .page');
  if (!div) return null;
  const r = div.getBoundingClientRect();
  return { n: div.dataset.pageNumber, fx: (x - r.left) / r.width, fy: (y - r.top) / r.height };
}, { sel: BOX, x, y });
// Where that fraction of that page is now.
const whereIs = (a) => page.evaluate(({ sel, a }) => {
  const r = document.querySelector(`${sel} .page[data-page-number="${a.n}"]`).getBoundingClientRect();
  return { x: r.left + a.fx * r.width, y: r.top + a.fy * r.height };
}, { sel: BOX, a });
const scale = () => page.evaluate(() => Number(document.querySelector('#zoomPct')?.textContent.replace('%', '')) || 0);

// zoomedIn leaves the page overflowing the container both ways, scrolled to the middle, so there
// is room for the view to follow the point in every direction. A page that fits cannot be scrolled
// and so cannot hold anything — that is geometry, not a defect, and it would make every assertion
// below pass or fail for the wrong reason.
async function zoomedIn() {
  for (let i = 0; i < 6; i++) await page.$eval('#zoomInBtn', (el) => el.click());
  await page.waitForFunction((sel) => {
    const el = document.querySelector(sel);
    return el.scrollWidth > el.clientWidth + 400;
  }, BOX);
  await page.$eval(BOX, (el) => { el.scrollLeft = (el.scrollWidth - el.clientWidth) / 2; el.scrollTop = 700; });
}

// One notch of a real wheel with Ctrl really down — the path a mouse takes.
async function ctrlWheel(x, y, deltaY) {
  await page.mouse.move(x, y);
  await page.keyboard.down('Control');
  await page.mouse.wheel(0, deltaY);
  await page.keyboard.up('Control');
}
// A trackpad pinch: a ctrlKey wheel with no Control key pressed, which is how the app tells it
// from the notch above. No device here produces one, so the event is built.
const pinch = (x, y, deltaY) => page.evaluate(({ x, y, deltaY }) => {
  window.dispatchEvent(new WheelEvent('wheel', { ctrlKey: true, deltaY, deltaMode: WheelEvent.DOM_DELTA_PIXEL, clientX: x, clientY: y, bubbles: true, cancelable: true }));
}, { x, y, deltaY });

// held zooms by `act` with the pointer at a fraction of the container and reports how far the
// point of the page that was there has moved.
async function held(fx, fy, act) {
  const b = await box();
  const x = Math.round(b.left + b.width * fx), y = Math.round(b.top + b.height * fy);
  const a = await anchorAt(x, y);
  assert.ok(a, `there is a page under the pointer at ${fx},${fy} of the view`);
  const before = await scale();
  await act(x, y);
  await page.waitForFunction((s) => (Number(document.querySelector('#zoomPct')?.textContent.replace('%', '')) || 0) !== s, before);
  const now = await whereIs(a);
  return { dx: now.x - x, dy: now.y - y };
}

test('Ctrl+scroll keeps the point under the pointer under the pointer, in and out, wherever the pointer is', async () => {
  await h.openDocument(DOC, 3);
  await zoomedIn();
  for (const [fx, fy] of [[0.2, 0.3], [0.5, 0.5], [0.85, 0.7]]) {
    for (const deltaY of [-100, 100]) {
      const d = await held(fx, fy, (x, y) => ctrlWheel(x, y, deltaY));
      assert.ok(Math.abs(d.dx) <= HELD && Math.abs(d.dy) <= HELD,
        `pointer at ${fx},${fy} of the view, wheel ${deltaY}: the point moved ${d.dx.toFixed(1)}px across and ${d.dy.toFixed(1)}px down`);
    }
  }
});

test('the sidebar being open or shut does not move the point: the view starts where the sidebar ends', async () => {
  // The defect was the sidebar's own width: the pointer was measured from the window's edge and
  // the page from the container's. So the two states must both hold, and must be seen to differ
  // in where the container starts — or this test has driven one state twice.
  const lefts = [];
  for (let i = 0; i < 2; i++) {
    lefts.push((await box()).left);
    const d = await held(0.7, 0.4, (x, y) => ctrlWheel(x, y, -100));
    assert.ok(Math.abs(d.dx) <= HELD && Math.abs(d.dy) <= HELD,
      `container starting ${lefts[i]}px in: the point moved ${d.dx.toFixed(1)}px across and ${d.dy.toFixed(1)}px down`);
    await ctrlWheel(600, 500, 100); // back out, so the second pass starts from the same zoom
    await page.click('#toggleSidebarBtn');
    const was = lefts[i];
    await page.waitForFunction(({ sel, was }) => document.querySelector(sel).getBoundingClientRect().left !== was, { sel: BOX, was });
  }
  assert.notEqual(lefts[0], lefts[1], 'the container starts somewhere else with the sidebar shut');
});

test('a trackpad pinch holds the point too', async () => {
  const d = await held(0.3, 0.6, (x, y) => pinch(x, y, -30));
  assert.ok(Math.abs(d.dx) <= HELD && Math.abs(d.dy) <= HELD, `the point moved ${d.dx.toFixed(1)}px across and ${d.dy.toFixed(1)}px down`);
});

test('the zoom buttons and the keyboard, which have no pointer, hold the middle of the view', async () => {
  for (const [name, act] of [
    ['Zoom in', () => page.$eval('#zoomInBtn', (el) => el.click())],
    ['Zoom out', () => page.$eval('#zoomOutBtn', (el) => el.click())],
    ['Ctrl+=', () => page.keyboard.press('Control+=')],
    ['Ctrl+-', () => page.keyboard.press('Control+-')],
  ]) {
    const d = await held(0.5, 0.5, act);
    assert.ok(Math.abs(d.dx) <= HELD && Math.abs(d.dy) <= HELD, `${name}: the middle of the view moved ${d.dx.toFixed(1)}px across and ${d.dy.toFixed(1)}px down`);
  }
});
