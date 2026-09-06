// The chrome collapses on a narrow window instead of growing — measured in a real browser,
// which is the only tier that can see any of it.
//
// ── What this tier can reach ─────────────────────────────────────────────────
// Everything here is geometry: rendered heights, wrapped row counts, the document's share
// of the viewport, whether the page scrolls sideways, whether a control sits past the right
// edge. All of it needs a layout engine.
//
// ── What it cannot, and who covers it ────────────────────────────────────────
// **jsdom cannot resolve a single assertion in this file.** It has no layout: every
// getBoundingClientRect is 0×0 at every viewport size, so a tier-2 copy of these tests would
// report pass unconditionally at every width — the vacuous green this repo keeps finding,
// arriving through the wrong tier. What tier 2 CAN hold is the structural half, and it does:
// `test/jsdom/toolbargroups.test.mjs` asserts every control lives in a labelled group, which
// is what keeps a control added later from being invisible down here.
//
// The baseline every number below is measured against, taken on this harness before the
// change (v1.118.0): Edit chrome was 19.6% of the viewport at 1920 and **63% at 360**, the
// palette grew from 3 rows to 12, the page scrolled sideways from 611px down, and at 360 the
// Collaborate tab, the theme toggle, the settings gear and the version pill were all off the
// right edge.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const h = await launch();
const { page } = h;
after(() => h.browser.close());

const DOC = writeFixture('responsive.pdf', { pages: 3, label: 'responsive page' });

// The ceiling the redesign was built to. 33% of the viewport for menubar + toolbar together,
// which at 768 tall is 253px.
const CHROME_CEILING = 33;
const WIDTHS = [1366, 1024, 900, 800, 700, 600, 500, 414, 360];

// Switching modes cannot go through .modetab below 575px — the tabs are display:none there
// and Playwright's click on a hidden element does nothing silently. That is not a hypothetical:
// it made every reading under 500px in the first measurement run meaningless.
async function setMode(mode) {
  await page.evaluate((m) => {
    const tab = document.querySelector(`.modetab[data-tab="${m}"]`);
    const jump = document.querySelector(`[data-modejump="${m}"]`);
    const shown = (e) => e && e.getBoundingClientRect().width > 0;
    (shown(tab) ? tab : jump).click();
  }, mode);
  await page.waitForTimeout(200);
}

const geometry = () => page.evaluate(() => {
  const vw = innerWidth, vh = innerHeight;
  const box = (s) => document.querySelector(s)?.getBoundingClientRect() ?? null;
  const mb = box('#menubar'), tb = box('#toolbar');
  const chrome = (mb?.height ?? 0) + (tb?.height ?? 0);
  const offscreen = [...document.querySelectorAll('#menubar button, #menubar .pill, #menubar nav')]
    .filter((e) => { const r = e.getBoundingClientRect(); return r.width > 0 && r.right > vw + 1; })
    .map((e) => (e.textContent || e.id || e.tagName).trim().slice(0, 20));
  return {
    menubarH: Math.round(mb?.height ?? 0),
    chromePct: Math.round((chrome / vh) * 1000) / 10,
    pageScrollsSideways: document.documentElement.scrollWidth > vw + 1,
    offscreen,
  };
});

test('the empty state holds its shape too, at every width', async () => {
  // **The launch view, and it was unmeasured.** Every other test in this file opens a document
  // first, so the whole suite spoke only for the populated state — a population bias in the
  // guard rather than in the code. The status cluster is a different width before a document
  // arrives (the signature badge and the version pill settle asynchronously), and during that
  // window the menubar can wrap at widths where it later fits.
  for (const w of WIDTHS) {
    await page.setViewportSize({ width: w, height: 768 });
    await page.waitForTimeout(250);
    const g = await geometry();
    assert.ok(g.menubarH <= 50,
      `with nothing open, at ${w}px the menubar is ${g.menubarH}px — it has wrapped to a second row on the first screen a user ever sees`);
    assert.equal(g.pageScrollsSideways, false,
      `with nothing open, at ${w}px the page scrolls sideways`);
    assert.deepEqual(g.offscreen, [],
      `with nothing open, at ${w}px these menubar items are past the right edge: ${g.offscreen.join(', ')}`);
  }
});

test('the chrome never takes a third of the window, at any width', async () => {
  // Widen first. The sweep above ends at 360, and since v1.121.0 `Open` is a foldable group in
  // the fixed bar — at 360 it is inside ⋯ More, so opening a document from there is a click on
  // something that is not on screen.
  await page.setViewportSize({ width: 1366, height: 768 });
  await h.openDocument(DOC, 3);
  for (const w of WIDTHS) {
    await page.setViewportSize({ width: w, height: 768 });
    for (const mode of ['file', 'edit']) {
      await setMode(mode);
      const g = await geometry();
      assert.ok(g.chromePct <= CHROME_CEILING,
        `at ${w}px the ${mode} chrome is ${g.chromePct}% of the viewport (ceiling ${CHROME_CEILING}%) — the toolbar is wrapping instead of folding, which is the defect this replaced: it reached 63% at 360px`);
    }
  }
});

test('the page never scrolls sideways, and nothing leaves the menubar', async () => {
  for (const w of WIDTHS) {
    await page.setViewportSize({ width: w, height: 768 });
    await setMode('edit');
    const g = await geometry();
    assert.equal(g.pageScrollsSideways, false,
      `at ${w}px the page scrolls sideways. #menubar had a hard minimum content width of 611px — brand + five mode tabs + the status cluster — and it alone caused this`);
    assert.deepEqual(g.offscreen, [],
      `at ${w}px these menubar items sit past the right edge: ${g.offscreen.join(', ')}. At 360 the Signing Ceremony tab, the theme toggle and the settings gear were all unreachable without scrolling the window sideways`);
    // The menubar is one row at every width, and the mode-tab fold threshold is what keeps
    // it so. Renaming a tab is what breaks this: "Collaborate" → "Signing Ceremony" widened
    // the five tabs to 409px and made the nav wrap below 690, silently adding 21px of chrome
    // at every width in between. The threshold was moved to 694 on that measurement.
    assert.ok(g.menubarH <= 50,
      `at ${w}px the menubar is ${g.menubarH}px — it has wrapped to a second row. A mode-tab label grew past what fits, and the fold threshold in style.css needs to move above the width where the nav starts wrapping`);
  }
});

test('a folded command still runs, from inside the ⋯ More menu', async () => {
  // The proof that folding MOVED a control rather than orphaning it. Zoom is fold rank 2 in
  // the FIXED bar since v1.121.0 — the bar that does not change with the mode still folds with
  // the width — so at 360 it is inside that bar's ⋯ More; clicking it must still change the render.
  await page.setViewportSize({ width: 360, height: 768 });
  await setMode('file');

  const more = await page.evaluate(() => {
    const m = document.querySelector('#toolbar .tbfixed .tbmore');
    return m ? { shown: m.classList.contains('hasfolded'), label: m.querySelector('.menutop')?.textContent.trim() } : null;
  });
  assert.ok(more && more.shown, 'no ⋯ More menu is offered at 360px, so every folded command is simply gone');

  const widthOfPage = () => page.evaluate(() =>
    Math.round(document.querySelector('.viewerContainer:not([hidden]) .page')?.getBoundingClientRect().width ?? 0));
  const before = await widthOfPage();
  assert.ok(before > 0, 'setup: no page is rendered, so a zoom change would be unobservable');

  await page.click('#toolbar .tbfixed .tbmore .menutop');
  await page.waitForTimeout(150);
  const labels = await page.evaluate(() =>
    [...document.querySelectorAll('#toolbar .tbfixed .tbmore .menucap')].map((e) => e.textContent.trim()));
  assert.ok(labels.includes('Zoom'),
    `the ⋯ More menu offers no labelled Zoom group — it holds ${JSON.stringify(labels)}. A fold with no group heading is a second flat list one click further away`);

  await page.evaluate(() => {
    const b = [...document.querySelectorAll('#toolbar .tbfixed .tbmore button')]
      .find((x) => x.textContent.trim() === 'Zoom in');
    b.click();
  });
  await page.waitForTimeout(400);
  assert.ok(await widthOfPage() > before,
    'Zoom in did nothing from inside the ⋯ More menu — the control was moved out of the bar and lost its wiring on the way');
});

test('a folded group stays inside its own mode', async () => {
  // The one thing that makes moving groups safe rather than reckless. Mode gating is
  // `#toolbar .tbtab.active`, a DESCENDANT selector — so a group folded into a ⋯ More menu
  // inside its own pane is still gated, and one folded anywhere else would appear in all
  // five modes at once. Nothing about the fold looks wrong until you change mode.
  await page.setViewportSize({ width: 360, height: 768 });
  await setMode('edit');
  const foldedInEdit = await page.evaluate(() =>
    document.querySelectorAll('#toolbar .tbtab[data-tab="edit"] .tbmore .tbgroup').length);
  assert.ok(foldedInEdit > 0, 'setup: nothing is folded in Edit at 360px, so this proves nothing');

  await setMode('file');
  const leaked = await page.evaluate(() => {
    const shown = (id) => {
      const e = document.getElementById(id);
      if (!e) return false;
      const r = e.getBoundingClientRect();
      return r.width > 0 && r.height > 0;
    };
    // Three Edit-only controls, each in a group that folds at 360.
    return ['ocrBtn', 'splitBtn', 'cropBtn'].filter(shown);
  });
  assert.deepEqual(leaked, [],
    `these Edit controls are visible while File is the active mode: ${leaked.join(', ')}. A group folded OUT of its .tbtab loses the mode gating that only applies to its descendants`);
});

test('the sidebar yields to the document on a narrow window, and comes back', async () => {
  // It was a hard 200px that never yielded: 55.6% of a 360px window, with the document down
  // to 14.4%. Collapsing it below 900 is worth 15-19 points of document area, measured.
  const sidebarShown = () => page.evaluate(() =>
    getComputedStyle(document.getElementById('sidebar')).display !== 'none');

  await page.setViewportSize({ width: 1200, height: 768 });
  await page.waitForTimeout(200);
  assert.equal(await sidebarShown(), true, 'setup: the sidebar is already hidden at 1200px');

  await page.setViewportSize({ width: 700, height: 768 });
  await page.waitForTimeout(250);
  assert.equal(await sidebarShown(), false,
    'the sidebar still holds 200px on a 700px window — more than a quarter of it, over a document that has the rest');

  await page.setViewportSize({ width: 1200, height: 768 });
  await page.waitForTimeout(250);
  assert.equal(await sidebarShown(), true,
    'the sidebar did not come back when the window widened — auto-collapse is a function of width, not a one-way trip');
});

test('this file leaves the shared server as it found it', async () => {
  await page.setViewportSize({ width: 1280, height: 900 });
  const openPages = (await h.counts()).pages;
  h.answerDialogs(true);
  await h.closeDocument();
  const left = await page.evaluate(() => document.querySelectorAll('.viewerContainer .page').length);
  assert.ok(openPages > 0, 'setup: no document was open, so this cleanup covered nothing');
  assert.equal(left, 0, `${left} page divs survive the close — the next file in this tier will count them as its own`);
});
