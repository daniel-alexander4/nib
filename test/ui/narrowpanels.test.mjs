// /pending 429 — the sidebar's CONTENT panels below 900px, measured in a real browser.
//
// ── The claim under test ─────────────────────────────────────────────────────
// `matchMedia('(max-width: 899px)')` collapses the sidebar, `#sidebar.collapsed { display: none }`
// takes it off screen, and `moveCommandsHome` rescues `all('.tbtab')` only — so `#ceremony`,
// `#flags`, `#library` and `#outline` have no toolbar home. The item concluded from that that "on
// a window narrower than 900px a signing ceremony is unreachable".
//
// **Reachability is a claim about a running layout and cannot be settled by reading.** The
// collapse is a *default*, not a lock: `#toggleSidebarBtn` sits outside the `.tbtab` groups
// (index.html:96) and no `@media` rule touches `#sidebar` — so the question is whether a user at
// 375px can get the rail back, and what it costs the document when they do.
//
// ── What only this tier can see ──────────────────────────────────────────────
// Every assertion here is geometry or hit-testing. jsdom resolves no layout at all — every rect is
// 0×0 at every viewport — so a tier-2 copy would report the same answer at 375 and at 1920, which
// is the vacuous green this repo keeps finding arriving through the wrong tier.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './harness.mjs';

const ME = 'aa'.repeat(32);
const NARROW = { width: 375, height: 768 };   // an iPhone-class window, well under the 899 threshold

const listing = {
  primary: true,
  ceremonies: [{
    id: '1'.repeat(32),
    state: 'ok',
    intent: 'We agree to the lease of 14 Elm Row',
    expires: '2026-10-01T12:00:00Z',
    me: ME,
    roster: [
      { fingerprint: ME, label: 'You', signs: true },
      { fingerprint: 'bb'.repeat(32), label: 'The other party', signs: true },
    ],
  }],
  ended: [],
};

const h = await launch({
  routes: {
    '**/api/ceremonies': (route) => route.fulfill({
      status: 200, contentType: 'application/json', body: JSON.stringify(listing),
    }),
  },
});
const { page } = h;
after(() => h.browser.close());

// Below 575px the `.modetab` strip is display:none and Playwright clicks a hidden element
// silently — measured in responsive.test.mjs, where it made every reading under 500px meaningless.
async function setMode(mode) {
  await page.evaluate((m) => {
    const tab = document.querySelector(`.modetab[data-tab="${m}"]`);
    const jump = document.querySelector(`[data-modejump="${m}"]`);
    const shown = (e) => e && e.getBoundingClientRect().width > 0;
    (shown(tab) ? tab : jump).click();
  }, mode);
  await page.waitForTimeout(250);
}

// **Hit-testing, not `getBoundingClientRect().width > 0`.** An element inside a collapsed flex
// column still HAS a rect; what says a user can reach it is whether the point at its centre
// belongs to it. railscale.test.mjs measured both lying in this exact nesting.
const reach = (sel) => page.evaluate((s) => {
  const el = document.querySelector(s);
  if (!el) return { present: false };
  const r = el.getBoundingClientRect();
  const hit = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
  return {
    present: true,
    width: Math.round(r.width),
    height: Math.round(r.height),
    reachable: r.width > 0 && r.height > 0 && !!hit && (hit === el || el.contains(hit)),
  };
}, sel);

const sidebarCollapsed = () => page.evaluate(() =>
  document.getElementById('sidebar').classList.contains('collapsed'));

// goNarrow crosses the threshold from a wide window every time, which is what makes each test
// independent: `sidebarNarrow` is a CROSSING listener, so resizing to a width the page is already
// at fires nothing and the sidebar arrives in whatever state the previous test left it.
async function goNarrow() {
  await page.setViewportSize({ width: 1280, height: 768 });
  await page.waitForTimeout(200);
  await page.setViewportSize(NARROW);
  await page.waitForTimeout(350);
}

// **Read the state, then click — never click and assume.** The page is shared by every test in
// this file and `setViewportSize` to a width we are ALREADY at fires no matchMedia crossing, so
// the sidebar arrives at each test in whatever state the last one left it. A blind click is
// therefore a toggle, and the first cut of the width measurement below closed the sidebar it was
// supposed to be measuring open — reporting 100% of the window for the document and passing.
async function ensureSidebarOpen() {
  if (await sidebarCollapsed()) {
    await page.click('#toggleSidebarBtn');
    await page.waitForTimeout(300);
  }
  assert.equal(await sidebarCollapsed(), false, 'the toggle did not reopen the sidebar');
}

test('below 900px the sidebar collapses and takes every content panel with it', async () => {
  await goNarrow();
  // SETUP, and it is the item's own premise. Without this every assertion below is about a
  // window where nothing collapsed, and the file would pass on a build that had no threshold.
  assert.equal(await sidebarCollapsed(), true,
    `at ${NARROW.width}px the sidebar did not auto-collapse, so this file is measuring the wide `
    + 'layout and says nothing about the narrow one');
  await setMode('collaborate');
  const gone = await reach('#ceremony');
  assert.equal(gone.reachable, false,
    'the ceremony panel is reachable with the sidebar collapsed — which would mean the collapse '
    + 'is not what hides it, and the rest of this file is testing the wrong mechanism');
});

test('the sidebar toggle survives the collapse, and is the way back', async () => {
  await goNarrow();
  // This is what makes the collapse a DEFAULT rather than a lock. The control sits outside the
  // `.tbtab` groups so it never swaps out with the mode, and no @media rule hides it — if that
  // ever changes, the content panels become genuinely unreachable and this is the line that says so.
  const toggle = await reach('#toggleSidebarBtn');
  assert.equal(toggle.reachable, true,
    `at ${NARROW.width}px the sidebar toggle is not reachable (${JSON.stringify(toggle)}). The `
    + 'collapse hides every content panel — the ceremony rail, the signing flags, the library and '
    + 'the outline — so with no way to undo it a signing ceremony really would be unreachable here');
});

test('a signing ceremony is reachable at 375px, roster and all', async () => {
  await goNarrow();
  await ensureSidebarOpen();
  await setMode('collaborate');
  await page.evaluate(() => {
    const head = document.querySelector('.sbhead[data-panel="ceremony"]');
    if (head) head.click();
  });
  await page.waitForTimeout(400);

  const panel = await reach('#ceremony');
  assert.equal(panel.reachable, true,
    `the Signing Ceremonies panel is not reachable at ${NARROW.width}px even with the sidebar `
    + `open (${JSON.stringify(panel)}) — there would then be no rail, no convene button and no `
    + 'way to see whose turn it is on a window this size');
  const card = await reach('#ceremonyList .cercard');
  assert.equal(card.reachable, true,
    `the ceremony CARD is not reachable (${JSON.stringify(card)}). A panel that is open and empty `
    + 'is the same outcome for the user as a panel that is gone');
  const party = await reach('#ceremonyList .cerparty');
  assert.equal(party.reachable, true,
    `no roster row is reachable (${JSON.stringify(party)}) — the roster and this machine's `
    + 'position in it are what the panel exists to show');
});

test('the way back to the rail leaves the document a readable share of a 375px window', async () => {
  // **A floor, not a pin.** railscale.test.mjs gives the reason a rendered width is never asserted
  // exactly: a font, a scrollbar or a locale moves it and none of those is a bug. What a floor can
  // say is that reopening the sidebar is a REAL way back rather than a nominal one — a column that
  // took the window would make the ceremony rail reachable and the document not.
  //
  // 25% at 375px is 94px. The sidebar is a fixed 200px, so the true figure is 46.7% (measured
  // below, and printed rather than asserted); the floor sits far enough under it to survive
  // ordinary rendering differences and still catch a sidebar that grew.
  const FLOOR = 25;
  await goNarrow();
  await ensureSidebarOpen();
  const share = await page.evaluate(() => {
    const col = document.getElementById('viewerCol');
    const r = col ? col.getBoundingClientRect() : { width: 0 };
    return Math.round((r.width / innerWidth) * 1000) / 10;
  });
  console.log(`  narrowpanels: with the sidebar reopened at 375px the document keeps ${share}% of the width`);
  assert.ok(share >= FLOOR,
    `with the sidebar reopened at 375px the document column has ${share}% of the window, under the `
    + `${FLOOR}% floor — the only way back to the ceremony rail on a window this size costs the `
    + 'document, and past this point it costs all of it');
});
