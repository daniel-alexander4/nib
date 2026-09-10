// /pending 463 — the signing commands sit under the signing button, not at the bottom edge.
//
// Dan, 2026-09-10, with a screenshot of Place Signing Flags: *"The following menu items should be
// up under the signing button."* The panel filled the sidebar, then a tall stretch of empty space,
// and only at the very bottom edge the **Simple Sign**, **Send & Receive** and **Signing
// Ceremonies** headers — reading as leftovers below the fold rather than as the three main things
// somebody came to this surface to do.
//
// ── Why tier 3, and why no other tier will do ────────────────────────────────
// The whole complaint is a DEAD GAP, and a gap is geometry. CONTRIBUTING.md states jsdom's ceiling
// in as many words — "jsdom models the DOM, not an engine — no layout (every `clientWidth` is 0)"
// — so a tier-2 version of this file would measure zero at every position and pass against the
// screenshot Dan sent.
//
// ── What it asserts, and what it only records ────────────────────────────────
// Not a pixel figure: the GAP, against the ordinary spacing between two stacked headers in the
// same column. A committed test pinning "the gap is 12px" would go red for a font, a locale or a
// new button, none of them this defect. What is stable is that the space below the signing button
// is not larger than the space between the cards themselves.
//
// ── The cause, for whoever reads this after changing the CSS ─────────────────
// `.sbpane > .panel.active:not(#commands)` claimed `flex: 1 1 auto`, so the ACTIVE content panel
// ate the column's leftover height and pushed everything after it to the bottom edge. The rule
// three lines above it in style.css had already fixed exactly this defect one selector over:
// *"Without this the commands container held `flex: 1` and pushed the content-panel headers to the
// bottom of the column with a dead gap above them."*
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './harness.mjs';

const REF = { width: 1280, height: 900 };

// `browser.close()`, and the first cut of this file wrote `ctx.close()` — which `launch` does not
// return. The teardown threw, the browser stayed open, and `node --test` never drained its event
// loop: the whole tier-3 run hung for half an hour rather than failing. Every other file here
// spells it `h.browser.close()`.
let ctx;
after(async () => { if (ctx) await ctx.browser.close(); });

test('the signing cards follow the signing button instead of sinking to the bottom', async () => {
  ctx = await launch({});
  const { page } = ctx;
  await page.setViewportSize(REF);
  // Collaborate mode is where all three live; `flags` is the mode's first panel and so the
  // active one.
  await page.click('.modetab[data-tab="collaborate"]');
  await page.waitForSelector('#flags.active', { timeout: 5000 });

  const geom = await page.evaluate(() => {
    const save = document.getElementById('saveForSigningBtn');
    const heads = [...document.querySelectorAll('.sbpane.active .sbhead')]
      .filter((h) => h.offsetParent !== null)
      .map((h) => ({ text: h.textContent.trim(), top: h.getBoundingClientRect().top }))
      .sort((a, b) => a.top - b.top);
    const saveRect = save.getBoundingClientRect();
    const below = heads.filter((h) => h.top >= saveRect.bottom);
    return {
      saveBottom: saveRect.bottom,
      saveVisible: saveRect.height > 0,
      below: below.map((h) => h.text),
      firstBelowTop: below.length ? below[0].top : null,
      // The yardstick: the ordinary distance between two consecutive headers in this column.
      headGaps: below.slice(1).map((h, i) => h.top - below[i].top),
      paneBottom: document.querySelector('.sbpane.active').getBoundingClientRect().bottom,
    };
  });

  // Stimulus floors, both halves. A hidden button or an empty header list would make every
  // assertion below vacuous, and the second one is the load-bearing one: this test is about
  // WHERE the cards are, so it must first prove there are cards.
  assert.equal(geom.saveVisible, true,
    'the Save-for-signing button is not rendered, so "under the signing button" has no anchor');
  assert.ok(geom.below.length >= 2,
    `only ${geom.below.length} card header(s) render below the signing button, so this test is `
    + 'not measuring the stack Dan photographed: ' + JSON.stringify(geom.below));

  // The assertion. A dead gap is one materially larger than the column's own rhythm.
  const gap = geom.firstBelowTop - geom.saveBottom;
  const rhythm = Math.max(...geom.headGaps, 1);
  assert.ok(gap <= rhythm * 2,
    `the first signing card sits ${Math.round(gap)}px below the Save-for-signing button, while `
    + `consecutive cards in the same column are ${geom.headGaps.map(Math.round).join('/')}px `
    + 'apart. That space is the active panel claiming the column\'s leftover height, which pushes '
    + 'Simple Sign, Send & Receive and Signing Ceremonies to the bottom edge — where they read as '
    + 'leftovers below the fold rather than as the next step after saving for signing');
});
