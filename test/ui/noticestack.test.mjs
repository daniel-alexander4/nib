// The bottom-right corner's notices in a real window (/pending 876).
//
// Four notices share that corner — the fit report, the lost accessibility structure, the content a page
// operation could not carry, and the XFA form — and any of them can be up together. Each is a sentence
// that wraps, so each is as tall as its words at the width it has. They were placed by a fixed `bottom`
// apiece, 52 px apart, and measured in a real window stood 162, 162 and 107 px tall: on top of each
// other, with two of the three Dismiss buttons under a neighbour. jsdom cannot see any of this — it
// lays nothing out — which is why the three were built and tested and still unreadable together.
//
// What puts the notices up is the SERVER's answer to a page operation, and each of its three facts has
// its own Go test. Here the real answer to a real rotation is given all three at once on its way to the
// page, because no one document loses its tags, a comment and an XFA form in a single step.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch, shutdown } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const DOC = writeFixture('noticestack.pdf', { pages: 2, label: 'notice stack page' });

const h = await launch({
  routes: {
    '**/api/pages': async (route) => {
      const res = await route.fetch();
      const answer = await res.json();
      Object.assign(answer, { taggingDropped: true, lostAnnots: 3, lostFields: 2, xfa: 'removed' });
      await route.fulfill({ response: res, json: answer });
    },
  },
});
const page = h.page;

after(async () => { await shutdown(h); });

const NOTICES = ['tagNotice', 'lostNotice', 'xfaNotice'];

const boxes = () => page.evaluate((ids) => {
  const wrap = document.getElementById('viewerWrap').getBoundingClientRect();
  const out = { wrap: { left: wrap.left, top: wrap.top, right: wrap.right, bottom: wrap.bottom }, notices: {} };
  for (const id of ids) {
    const el = document.getElementById(id);
    if (!el || el.hidden) { out.notices[id] = null; continue; }
    const r = el.getBoundingClientRect();
    const b = el.querySelector('button').getBoundingClientRect();
    const hit = document.elementFromPoint(b.left + b.width / 2, b.top + b.height / 2);
    const text = el.querySelector('span');
    out.notices[id] = {
      left: r.left, top: r.top, right: r.right, bottom: r.bottom,
      reachable: !!hit && hit === el.querySelector('button'),
      clipped: text.scrollWidth > text.clientWidth + 1,
    };
  }
  return out;
}, NOTICES);

// settled waits out the window's own response to a resize — at phone width the sidebar leaves the view's
// side — by reading the view's box until two readings a frame apart agree.
async function settled(width) {
  await page.waitForFunction((w) => innerWidth === w, width);
  await page.waitForFunction(() => new Promise((done) => {
    const read = () => JSON.stringify(document.getElementById('viewerWrap').getBoundingClientRect());
    const a = read();
    requestAnimationFrame(() => requestAnimationFrame(() => done(read() === a)));
  }));
}

test('three notices up together are each readable, apart, inside the view, and can each be dismissed', async () => {
  await h.openDocument(DOC, 2);
  await page.$eval('#rotateRightBtn', (el) => el.click());
  await page.waitForFunction((ids) => ids.every((id) => !document.getElementById(id).hidden), NOTICES);

  for (const [width, height] of [[1366, 768], [375, 667]]) {
    await page.setViewportSize({ width, height });
    await settled(width);
    const at = `${width}×${height}`;
    const { wrap, notices } = await boxes();
    const stack = await page.$eval('#noticeStack', (el) => {
      const r = el.getBoundingClientRect();
      return { left: r.left, top: r.top, right: r.right, bottom: r.bottom, scrolls: el.scrollHeight > el.clientHeight + 1 };
    });
    // The stack itself never leaves the view; where the three are taller than the view it scrolls.
    assert.ok(stack.left >= wrap.left - 0.5 && stack.right <= wrap.right + 0.5 && stack.top >= wrap.top - 0.5 && stack.bottom <= wrap.bottom + 0.5,
      `${at}: the stack runs outside the view: ${JSON.stringify(stack)} in ${JSON.stringify(wrap)}`);
    if (width === 1366) assert.ok(!stack.scrolls, `${at}: three notices do not fit a ${height}px window without scrolling`);
    for (const id of NOTICES) {
      // Stimulus: all three are up, or "no two overlap" is true of fewer than it claims.
      assert.ok(notices[id], `${at}: #${id} is not showing, so this size measured fewer than three notices`);
      // Brought into the stack's own view first: at phone width that is a scroll, at desktop width nothing.
      await page.$eval('#' + id, (el) => el.scrollIntoView({ block: 'nearest' }));
      const n = (await boxes()).notices[id];
      assert.ok(n.left >= wrap.left - 0.5 && n.right <= wrap.right + 0.5 && n.top >= wrap.top - 0.5 && n.bottom <= wrap.bottom + 0.5,
        `${at}: #${id} runs outside the view: ${JSON.stringify(n)} in ${JSON.stringify(wrap)}`);
      assert.ok(n.reachable, `${at}: #${id}'s Dismiss is under something else`);
      assert.ok(!n.clipped, `${at}: #${id}'s sentence is cut off`);
    }
    for (let i = 0; i < NOTICES.length; i++) {
      for (let j = i + 1; j < NOTICES.length; j++) {
        const a = notices[NOTICES[i]], b = notices[NOTICES[j]];
        const apart = a.bottom <= b.top + 0.5 || b.bottom <= a.top + 0.5 || a.right <= b.left + 0.5 || b.right <= a.left + 0.5;
        assert.ok(apart, `${at}: #${NOTICES[i]} and #${NOTICES[j]} overlap: ${JSON.stringify(a)} ${JSON.stringify(b)}`);
      }
    }
    // Nearest the corner first, in the order the markup gives them.
    assert.ok(notices.tagNotice.top > notices.lostNotice.top && notices.lostNotice.top > notices.xfaNotice.top,
      `${at}: the notices are not stacked in their order`);
  }
});

test('dismissing the middle one closes the gap, and the page beside the stack still takes a click', async () => {
  await page.setViewportSize({ width: 1366, height: 768 });
  await settled(1366);
  const before = (await boxes()).notices;
  await page.click('#lostNoticeDismiss');
  await page.waitForFunction(() => document.getElementById('lostNotice').hidden);
  const after = (await boxes()).notices;
  assert.equal(after.lostNotice, null);
  assert.ok(after.xfaNotice.bottom <= after.tagNotice.top + 0.5 && after.tagNotice.top - after.xfaNotice.bottom < 20,
    `the XFA notice did not come down to the tagging notice: ${JSON.stringify(after)}`);
  assert.ok(after.xfaNotice.top > before.xfaNotice.top, 'the XFA notice did not move when the one below it went');
  // The stack is a box over the page's corner; only the notices in it take the pointer.
  const beside = await page.evaluate(() => {
    const s = document.getElementById('noticeStack').getBoundingClientRect();
    const x = document.getElementById('xfaNotice').getBoundingClientRect();
    const hit = document.elementFromPoint(s.left + 4, x.top - 30 > s.top ? x.top - 30 : s.top + 2);
    return hit ? hit.id || hit.className || hit.tagName : null;
  });
  assert.notEqual(beside, 'noticeStack', 'the empty part of the stack takes the pointer from the page under it');
  // Resizing the window while pdf.js is still drawing a page makes pdf.js log that its link layer was asked
  // for before the page was rendered (`injectLinkAnnotations`). It is the resize this file does, not the
  // notices, and it is filed on its own (/pending 887); anything else here is this file's to explain.
  assert.deepEqual(h.consoleErrors.filter((e) => !/injectLinkAnnotations/.test(e)), []);
});
