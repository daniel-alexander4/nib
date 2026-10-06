// The icon toolbar in a real browser (ADR-087).
//
// What only this tier can see: that a button's word is really out of sight in the bar and really
// in sight once its group folds into More; that the bar fits on one row at the widths people use;
// and the zoom level, which the real viewer computes and jsdom's stand-in does not.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch, shutdown } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const A = writeFixture('bar-a.pdf', { pages: 3, label: 'bar A page' });
const B = writeFixture('bar-b.pdf', { pages: 2, label: 'bar B page' });

const h = await launch();
const page = h.page;
after(async () => { await shutdown(h); });

const pct = () => page.$eval('#zoomPct', (e) => e.textContent);
const pageWidth = () => page.evaluate(() =>
  document.querySelector('.viewerContainer:not([hidden]) .page')?.getBoundingClientRect().width ?? 0);
// ROWS counts the lines the bar's visible buttons sit on. Tops within a few pixels are one row: a
// text readout and an icon do not share a box height, and that is not a wrap.
const ROWS = `(() => {
  const bar = document.querySelector('#toolbar .tbfixed');
  const tops = [...bar.querySelectorAll(':scope > .tbgroup > button')].filter((b) => b.offsetParent)
    .map((b) => b.getBoundingClientRect().top).sort((a, b) => a - b);
  let rows = tops.length ? 1 : 0;
  for (let i = 1; i < tops.length; i++) if (tops[i] - tops[i - 1] > 12) rows++;
  return rows;
})()`;

test('in the bar every button shows a picture and no word, on one row', async () => {
  await h.openDocument(A, 3);
  await page.setViewportSize({ width: 1280, height: 900 });
  const r = await page.evaluate(() => {
    const bar = document.querySelector('#toolbar .tbfixed');
    const buttons = [...bar.querySelectorAll(':scope > .tbgroup > button')].filter((b) => b.offsetParent);
    return {
      n: buttons.length,
      worded: buttons.filter((b) => { const l = b.querySelector('.tblabel').getBoundingClientRect(); return l.width > 1 || l.height > 1; }).map((b) => b.id || b.dataset.forward),
      blank: buttons.filter((b) => { const f = b.querySelector('svg, #zoomPct').getBoundingClientRect(); return f.width < 8 || f.height < 8; }).map((b) => b.id || b.dataset.forward),
    };
  });
  assert.ok(r.n >= 18, `only ${r.n} visible bar buttons at 1280 — groups folded that should not have`);
  assert.equal(await page.evaluate(() => document.querySelector('#toolbar .tbfixed .tbmore').classList.contains('hasfolded')), false, 'something is folded into More at 1280');
  assert.deepEqual(r.worded, [], 'these buttons show their word in the bar');
  assert.deepEqual(r.blank, [], 'these buttons show no picture');
  assert.equal(await page.evaluate(ROWS), 1, 'the bar wraps at 1280');
});

test('the bar stays on one row at every tested width, folding rather than wrapping', async (t) => {
  for (const w of [1366, 1024, 900, 800, 700, 600, 500, 414, 360]) {
    await page.setViewportSize({ width: w, height: 768 });
    await page.waitForTimeout(120);
    const r = await page.evaluate(() => {
      const bar = document.querySelector('#toolbar .tbfixed');
      const shown = [...bar.querySelectorAll(':scope > .tbgroup')].filter((g) => !g.hidden);
      const box = (e) => { const r = e.getBoundingClientRect(); return Math.round(r.left) + '+' + Math.round(r.width) + '@' + Math.round(r.top); };
      return { groups: shown.map((g) => g.dataset.label + ':' + box(g)), h: Math.round(document.querySelector('#toolbar').getBoundingClientRect().height),
        other: [...document.querySelectorAll('#toolbar > *, #toolbar .tbfixed > :not(.tbgroup)')].filter((e) => e.offsetParent).map((e) => (e.id || e.className) + ':' + box(e)) };
    });
    r.rows = await page.evaluate(ROWS);
    r.drop = await page.evaluate(() => Math.round(document.querySelector('#toolbar .tbfixed').getBoundingClientRect().top - document.getElementById('toggleSidebarBtn').getBoundingClientRect().top));
    t.diagnostic(`width ${w}: rows=${r.rows} toolbar=${r.h}px in-bar=${JSON.stringify(r.groups)} other=${JSON.stringify(r.other)}`);
    assert.ok(r.rows <= 1, `at ${w}px the bar's buttons sit on ${r.rows} rows (${JSON.stringify(r.groups)} still in the bar)`);
    // One row among themselves is not enough: a bar too wide for the line drops WHOLE beneath the
    // sidebar toggle, still in a single row of its own. Below 414 the title, the three groups that
    // never fold and More are already wider than the window, and it does.
    if (w >= 414) assert.ok(r.drop <= 12, `at ${w}px the whole bar has dropped ${r.drop}px below the sidebar toggle's row`);
  }
  await page.setViewportSize({ width: 1280, height: 900 });
});

test('the zoom level is shown, follows the zoom, and its button returns to 100%', async () => {
  await page.click('#actualSizeBtn');
  await page.waitForFunction(() => document.getElementById('zoomPct').textContent === '100%');
  await page.waitForTimeout(300);
  const actual = await pageWidth();
  assert.ok(actual > 0, 'setup: no page is rendered');
  // The readout against the PAGE: its width now over its width at actual size.
  const measured = async () => Math.round((await pageWidth()) / actual * 100);
  await page.click('#zoomInBtn');
  await page.waitForFunction(() => document.getElementById('zoomPct').textContent !== '100%');
  await page.waitForTimeout(300);
  const shown = parseInt(await pct(), 10);
  assert.ok(shown > 100, `after Zoom in the readout says ${shown}%`);
  assert.ok(Math.abs(shown - await measured()) <= 1, `the readout says ${shown}% and the page is at ${await measured()}% of its actual size`);

  // The readout is shared chrome: it must say the FOCUSED document's zoom.
  const zoomedA = await pct();
  await h.openDocument(B, 2);
  await page.click('#actualSizeBtn');
  await page.waitForFunction(() => document.getElementById('zoomPct').textContent === '100%');
  await page.click('#tabstrip .tab:first-child');
  await page.waitForFunction((want) => document.getElementById('zoomPct').textContent === want, zoomedA)
    .catch(() => assert.fail('after a tab switch the readout still shows the other document\'s zoom'));
});

test('folded into More, a button shows its word beside its picture', async () => {
  await page.setViewportSize({ width: 360, height: 768 });
  await page.waitForTimeout(150);
  await page.click('#toolbar .tbfixed .tbmore .menutop');
  await page.waitForTimeout(150);
  const r = await page.evaluate(() => {
    const b = document.getElementById('zoomInBtn');
    const l = b.querySelector('.tblabel').getBoundingClientRect();
    const s = b.querySelector('svg').getBoundingClientRect();
    return { inMore: !!b.closest('.tbmore'), word: b.querySelector('.tblabel').textContent, w: Math.round(l.width), h: Math.round(l.height), icon: Math.round(s.width) };
  });
  assert.equal(r.inMore, true, 'setup: at 360px the View group is not in More');
  assert.ok(r.w > 20 && r.h > 8, `inside More the word "${r.word}" is ${r.w}x${r.h}px — a picture with no name`);
  assert.ok(r.icon >= 8, 'inside More the picture is gone');
  await page.keyboard.press('Escape');
  await page.setViewportSize({ width: 1280, height: 900 });
});
