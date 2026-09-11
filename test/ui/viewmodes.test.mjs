// Continuous layout, MEASURED — the only tier that can see it.
//
// ── What only this tier can reach ────────────────────────────────────────────
// `viewmodes.test.mjs` at tier 2 proves the class lands on every page stack, including one that is
// hidden, and that the choice is saved and restored. It cannot prove that the class DOES anything:
// jsdom resolves no layout, so every `getBoundingClientRect` is 0×0 and a CSS rule that silently
// stopped matching would be invisible there.
//
// ── What is asserted, and what is only recorded ──────────────────────────────
// Asserted: the gap between two adjacent pages is real in standard layout and **zero** in
// continuous. Recorded, not pinned: the standard gap's size in pixels.
//
// **Not asserted here, and deliberately: which classes are present.** Leaving the joining class on
// in BOTH layouts is a real defect and this file cannot see it — test 1 re-measures the standard
// gap each run, so a mutated standard baseline and a mutated comparison agree with each other.
// `test/jsdom/viewmodes.test.mjs` catches it a tier down, where a class on an element is exactly
// what can be checked, and that is the right home for the claim.
// pdf.js draws it as
// `--page-border: 9px solid transparent` plus `--page-margin: 1px auto -8px`, so the number moves
// with a vendored upgrade and none of that would be a bug — the invariant is "separated" versus
// "touching", not "11".
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const h = await launch();
const { page } = h;
after(() => h.browser.close());

const DOC = writeFixture('viewmodes.pdf', { pages: 3, label: 'view mode page' });
const DOC2 = writeFixture('viewmodes-2.pdf', { pages: 2, label: 'second document' });

// The standard gap, measured once and compared against later. Recorded rather than pinned as a
// constant — see the file header — but a REMEMBERED measurement is what lets "restored" mean
// restored instead of merely non-zero.
let standardGap = null;

// gapBetweenPages measures the band a user actually sees between two pages — and it must measure
// the CONTENT boxes, not the border boxes.
//
// **The first cut measured `b.top - a.bottom` and read -7px in the standard layout**, which is
// correct and useless: pdf.js draws the separation as `--page-border: 9px solid transparent` with
// `background-clip: content-box`, so the visible band is INSIDE each element's rect and the
// negative `--page-margin` bottom of -8px then overlaps them. A border-box measurement therefore
// reports the pages as overlapping while the screen shows a gap. The white the user sees is the
// content box, so that is what is measured: 9 + (-8) + 1 + 9 ≈ 11px standard, 0 joined.
const gapBetweenPages = () => page.evaluate(() => {
  const pages = [...document.querySelectorAll('.viewerContainer:not([hidden]) .pdfViewer .page')];
  if (pages.length < 2) return null;
  const edge = (el, side) => parseFloat(getComputedStyle(el)[`border${side}Width`]) || 0;
  const a = pages[0].getBoundingClientRect();
  const b = pages[1].getBoundingClientRect();
  const contentBottomOfA = a.bottom - edge(pages[0], 'Bottom');
  const contentTopOfB = b.top + edge(pages[1], 'Top');
  return Math.round((contentTopOfB - contentBottomOfA) * 10) / 10;
});

test('standard layout separates the pages, and continuous joins them', async () => {
  await h.openDocument(DOC, 3);

  const standard = await gapBetweenPages();
  standardGap = standard;
  assert.notEqual(standard, null, 'setup: fewer than two pages rendered, so there is no gap to measure');
  console.log(`  viewmodes: standard layout leaves ${standard}px between pages`);
  // SETUP as an assertion: if the default already joined the pages, the comparison below would be
  // between two identical states and would pass on a build where the continuous class did nothing.
  assert.ok(standard > 0,
    `the default layout already leaves ${standard}px between pages — there is nothing for `
    + 'continuous mode to remove, and the assertion below could not fail');

  await page.click('#viewContinuousBtn');
  await page.waitForTimeout(300);
  const continuous = await gapBetweenPages();
  assert.equal(continuous, 0,
    `continuous layout still leaves ${continuous}px between pages. pdf.js draws the separation as a `
    + '9px transparent border plus a negative margin; `.removePageBorders` takes the border and '
    + 're-adds `margin: 0 auto 10px`, so the Nib rule that zeroes that last margin is what makes '
    + 'the pages actually touch');
});

test('switching back restores the separation exactly', async () => {
  await page.click('#viewStandardBtn');
  await page.waitForTimeout(300);
  const back = await gapBetweenPages();
  // **Compared against the measurement, not against zero.** `back > 0` passed on a mutation that
  // left the joining class on in BOTH layouts — the gap came back as 18px instead of 11 and the
  // assertion could not tell "restored" from "different". The number is still not pinned as a
  // constant; it is pinned to what this same document measured a moment ago.
  assert.equal(back, standardGap,
    `after returning to standard the pages are ${back}px apart and they were ${standardGap}px `
    + 'before continuous was ever entered — the layout does not round-trip, so a user who tries '
    + 'continuous does not get their document back as it was');
});

test('Fit page fits, and the automatic re-fit does not overwrite it', async () => {
  // **The second half is what found a real defect, and reaching it took three attempts.**
  // `userScale === false` meant "re-fit to WIDTH" at both automatic sites, so any later re-fit
  // silently replaced a Fit page with a Fit width.
  //
  // Clicking and asserting immediately cannot see it — nothing re-fits until something provokes
  // it. The provocation used here is **Reload**, because it is the one that drives the exact site:
  // it re-opens the same document, which fires `pagesloaded`, and `installOpened`'s same-document
  // branch deliberately keeps the scale the user chose — so if the re-fit picks the wrong fit, the
  // wrong scale is what survives.
  //
  // (Two earlier attempts are recorded because each passed for a wrong reason. Asserting straight
  // after the click never provoked anything. Opening a SECOND document and switching back read the
  // other document's pages — the tab strip was empty, so the "second" open had replaced the first,
  // and 1345px was simply fit-width on the wrong file. The provenance assertion below is what
  // catches that class: only the view Fit page was pressed on carries `fitPageButton` in its log.)
  await page.click('#fitPageBtn');
  await page.waitForTimeout(400);
  const fitted = await tallestAgainstWindow();
  assert.ok(fitted.why.includes('fitPageButton'),
    `Fit page did not reach the view being measured (provenance "${fitted.why}")`);
  assert.ok(fitted.tallest <= fitted.avail,
    `the tallest page renders ${fitted.tallest}px in a ${fitted.avail}px window — Fit page is the `
    + 'one control whose whole promise is that a page fits without scrolling');

  await page.click('#reloadBtn');
  await page.waitForTimeout(1200);

  const after = await tallestAgainstWindow();
  console.log(`  viewmodes: fit page ${fitted.tallest}px in ${fitted.avail}px; after a reload ${after.tallest}px (${after.why})`);
  assert.ok(after.why.includes('fitPageButton'),
    `after the reload the view's scale provenance is "${after.why}" — Fit page is not in it, so `
    + 'this is not the view the fit was applied to and the measurement means nothing');
  assert.ok(after.tallest <= after.avail,
    `the reload put the tallest page at ${after.tallest}px in a ${after.avail}px window `
    + `(provenance: ${after.why}). The automatic re-fit overwrote Fit page with the WIDTH fit — `
    + 'the view remembers that the user did not pick a number, but not WHICH fit produced the one '
    + 'it has');
});

// tallestAgainstWindow reads the active view only: the other document's pages are in a hidden
// container, and a hidden container's boxes are not what the user is looking at.
const tallestAgainstWindow = () => page.evaluate(() => {
  const all = [...document.querySelectorAll('.viewerContainer')];
  const c = all.find((e) => e.getBoundingClientRect().height > 0) || all[0];
  const pages = [...c.querySelectorAll('.pdfViewer .page')];
  const tallest = Math.max(...pages.map((p) => p.getBoundingClientRect().height));
  return {
    tallest: Math.round(tallest), avail: Math.round(c.clientHeight),
    // `userScaleLog` is the provenance trail app.js already keeps; printed into the failure so a
    // wrong scale names the call that set it instead of being a bare number.
    why: c.dataset.userScaleLog || '(none)', containers: all.length, id: c.id,
  };
});
