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

// The accordion's geometry, and it is here because the same defect class has now hit this
// sidebar four times: a rule written for the horizontal TOOLBAR travelling into the column with
// the panes ADR-017 moves. Three are recorded in ADR-018 (headers into the bar, the pass-through
// claiming the column, every header stretching to 203px); the fourth was `.tbtab`'s own
// `gap: 10px`, which separates groups sitting side by side in the bar and became a 10px band of
// `--mantle` between every stacked card — measured, a card head ending at y=192 with its body
// beginning at 202.
//
// **jsdom cannot hold any of this.** A gap between two cards and a border-radius that resolves
// are both computed style over real layout; tier 2 sees `0×0` rects and would pass at any value.
test('the sidebar cards stack flush, and every one is a rounded pill', async () => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.waitForTimeout(200);

  const found = await page.evaluate(() => {
    const vis = (e) => e.getBoundingClientRect().height > 0;
    const out = { modes: 0, cards: 0, gaps: [], square: [] };
    // Every mode, because the cards are per-mode and a rule can reach one pane and not another.
    for (const t of document.querySelectorAll('.modetab')) {
      t.click();
      out.modes++;
      // The open card's BODY is in the walk deliberately: it sits between two heads, so a gap
      // above or below it is exactly as visible as one between two heads, and skipping it would
      // have missed both halves of the defect this test was written for.
      // `.panel.active` is in the walk as well as `.tbgroup.open`: a content panel is an open body
      // too, and one sitting BETWEEN two headers is exactly as much a gap as a card's is. The
      // omission was safe while every content panel was the last thing in the column — nothing
      // followed it, so nothing could be pushed away from it. Flags leads the column since
      // v1.126.1 and Simple Sign follows it, which is what surfaced it: a 684px "gap" that was
      // the Flags panel doing its job.
      const items = [...document.querySelectorAll('#sidebar .sbhead, #commands .tbgroup.open, #sidebar .panel.active:not(#commands)')].filter(vis);
      const rows = items.map((e) => {
        const r = e.getBoundingClientRect();
        return {
          // A body is anything that is not a HEADER — a card's `.tbgroup` or a content `.panel`.
          // Only headers are pills, so only headers are asked about corners; a panel has none and
          // never should. Classifying by "is it a tbgroup" reported the Flags panel as a pill with
          // square corners the moment a panel stopped being last in the column.
          body: !e.classList.contains('sbhead'),
          label: e.textContent.trim().slice(0, 28),
          top: Math.round(r.top),
          bottom: Math.round(r.bottom),
          radius: parseFloat(getComputedStyle(e).borderTopLeftRadius),
        };
      });
      for (const r of rows) {
        if (r.body) continue;
        out.cards++;
        if (!(r.radius > 0)) out.square.push(`${t.dataset.tab}: ${r.label}`);
      }
      for (let i = 1; i < rows.length; i++) {
        const d = rows[i].top - rows[i - 1].bottom;
        if (d !== 0) out.gaps.push(`${t.dataset.tab}: ${rows[i - 1].label} -> ${rows[i].label} = ${d}px`);
      }
    }
    return out;
  });

  assert.ok(found.modes >= 5 && found.cards >= 15,
    `walked ${found.modes} modes and ${found.cards} cards — this guard is reading nothing`);
  assert.deepEqual(found.gaps, [],
    `these sidebar cards do not touch their neighbour:\n  ${found.gaps.join('\n  ')}\nA gap is a band of the sidebar's own ground showing between two pills, which is what a toolbar rule reaching the column looks like`);
  assert.deepEqual(found.square, [],
    `these sidebar cards have square corners: ${found.square.join(', ')}. Every pill is rounded — the panel cards keep their .tab class, whose border-radius: 0 is for a tab strip the sidebar no longer is`);

  await h.mode('file'); // the cleanup below closes from File, and this test ends in the last mode
});

// ── An icon button must actually show an icon ────────────────────────────────
//
// `.tbicon` was introduced with the reload button and never given a rule of its own, so its
// `<svg>` had no width or height: the button rendered as padding around nothing and Find and
// Reload read as blank gaps in the bar. It shipped that way and was reported by Dan, not caught
// here — the structural tier sees the `<svg>` in the DOM and calls it present, and I looked at a
// screenshot where the icons were missing and called them "subtle".
//
// So the assertion is on the RENDERED box, which is the only thing that can tell an icon from an
// element that exists. It is written over every icon-carrying button in the bar rather than over
// the two that were broken, because the next one will be a third.
test('every icon button in the toolbar renders its icon', async () => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.waitForTimeout(150);

  const icons = await page.evaluate(() => {
    const out = [];
    for (const b of document.querySelectorAll('#toolbar button')) {
      const svg = b.querySelector('svg');
      if (!svg) continue;                       // a labelled button; not this test's subject
      if (b.offsetParent === null) continue;    // folded away or in a hidden pane
      const r = svg.getBoundingClientRect();
      out.push({ id: b.id || b.className, w: Math.round(r.width), h: Math.round(r.height) });
    }
    return out;
  });

  assert.ok(icons.length >= 3,
    `only ${icons.length} icon buttons found in the toolbar — this guard is reading nothing`);
  const blank = icons.filter((i) => i.w < 8 || i.h < 8);
  assert.deepEqual(blank, [],
    `these toolbar buttons carry an <svg> that renders at no usable size: ${blank.map((b) => `${b.id} (${b.w}x${b.h})`).join(', ')}. The element is in the DOM, so every structural check passes and the control still reads as an empty gap.`);
});

// ── An open card reads as part of the list it is in ─────────────────────────
//
// Reported: "the content for an expanded pill shows below all of the pills and not directly under
// the header." The card's body was capped at `60vh` with `overflow-y: auto`, which made it a
// SCROLLER INSIDE A SCROLLER: Export & Print's fifteen items ran 505px, so the body became a
// fixed 540px island and the pills after it were pushed to the bottom of the column — at 900px
// tall they landed at y=724 of an 818px pane, and on a shorter window off the fold entirely.
//
// The property is that the accordion behaves like a list: the content sits directly under its own
// header, the next header comes after the content, and there is ONE scroller — the pane. A nested
// scroller is what turns a long card into a box the surrounding list appears to flow around.
//
// **A version that made the body claim the leftover height instead was tried and reverted**, and
// the measurement is why: nested in two flex containers it CLIPPED rather than scrolled —
// `clientHeight` 146 against 505px of children, `scrollTop` refusing to move, every item past the
// fold unreachable. Worse than what it fixed.
test('an expanded card sits under its own header, with one scroller', async () => {
  await page.setViewportSize({ width: 1280, height: 900 });
  // Reuses the document the tests above opened rather than opening a second one — this file's
  // cleanup closes ONE, so an extra open leaves page divs behind and fails it instead of this.
  await h.mode('file');
  await h.card('Export & Print');   // the tallest card in the app

  const read = () => page.evaluate(() => {
    const pane = document.getElementById('sbFunctions');
    const body = document.querySelector('#sbFunctions .tbgroup.open');
    // The header immediately above THIS body. Searching for `aria-expanded="true"` finds the
    // first one in document order, and cards in the other modes' panes keep their expanded state
    // while hidden — so that search returned a card from a pane nobody is looking at, and the
    // measured gap was one header's height.
    const head = body.previousElementSibling;
    const next = body.nextElementSibling;
    const r = (e) => e.getBoundingClientRect();
    return {
      items: [...body.children].filter((e) => r(e).height > 0).length,
      gapFromHeader: Math.round(r(body).top) - Math.round(r(head).bottom),
      nextHeaderOffset: next ? Math.round(r(next).top) - Math.round(r(body).bottom) : null,
      paneScrolls: pane.scrollHeight > pane.clientHeight + 1,
      // Can the LAST item actually be SEEN after scrolling? Two cheaper observables were tried
      // and both lie about this:
      //   * `scrollHeight` — with the old cap the body reported 223 = 223 while holding 505px of
      //     items, because a flex column with a capped height clips its children without
      //     establishing any scroll extent;
      //   * the item's own rect — a clipped element still HAS a position, and scrolling the pane
      //     moves that position into the pane's box while the item stays invisible behind the
      //     clip.
      // Hit-testing is the one that accounts for the clip: ask the document what is painted at
      // the item's own centre.
      lastItemReachable: (() => {
        const kids = [...body.children].filter((e) => r(e).height > 0);
        const last = kids[kids.length - 1];
        pane.scrollTop = pane.scrollHeight;
        const lr = r(last);
        const hit = document.elementFromPoint(Math.round(lr.left + lr.width / 2), Math.round(lr.top + lr.height / 2));
        const ok = !!hit && (hit === last || last.contains(hit));
        pane.scrollTop = 0;
        return ok;
      })(),
    };
  });

  const wide = await read();
  assert.ok(wide.items >= 10,
    `the open card shows ${wide.items} items — this guard wants the tall card, and a short one cannot demonstrate the defect`);
  assert.equal(wide.gapFromHeader, 0,
    `the open card's content starts ${wide.gapFromHeader}px below its own header rather than directly under it`);
  assert.equal(wide.nextHeaderOffset, 0,
    `the next pill sits ${wide.nextHeaderOffset}px after the content instead of directly after it — the card is not reading as part of the list`);
  assert.equal(wide.lastItemReachable, true,
    'the last item of the open card cannot be brought into view. A capped, flex-column body clips its children WITHOUT reporting any overflow — so the items past the cap are simply unreachable and nothing says so');

  // And on a window short enough that the column cannot hold it, the same three hold — the pane
  // takes over the scrolling rather than the card growing its own.
  await page.setViewportSize({ width: 1280, height: 420 });
  await page.waitForTimeout(300);
  const short = await read();
  assert.equal(short.gapFromHeader, 0, 'on a short window the content parted company with its header');
  assert.equal(short.paneScrolls, true,
    'nothing scrolls on a window too short to hold the open card, so its items past the fold cannot be reached at all');
  assert.equal(short.lastItemReachable, true,
    'on a short window the open card\'s last item cannot be reached by scrolling the sidebar');

  // Left OPEN: this file's last test is a cleanup that closes what is open, and #closeBtn is
  // disabled with nothing there — a disabled button is a 30-second timeout rather than a failed
  // assertion. Restoring the viewport matters for the same reason.
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.waitForTimeout(200);
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
