// P04.S01 — the ceremony rail at a full roster, MEASURED.
//
// D6 owes this plan a worklist threshold: the roster size above which the rail stops showing one
// action and shows a worklist instead. The plan's own standing caveat says it "is chosen from
// rendering at several roster sizes, not guessed", and CLAUDE.md says a claim about scale is
// settled by running it. This is the run.
//
// ── Why tier 3, and why no other tier will do ────────────────────────────────
// Every candidate metric is geometry. CONTRIBUTING.md states jsdom's ceiling in as many words —
// "jsdom models the DOM, not an engine — no layout (every `clientWidth` is 0)" — so a tier-2
// version of this file would report the same numbers at every roster size and the threshold would
// be a number produced by a harness that cannot see the thing it measures.
//
// ── What this file asserts, and what it only RECORDS ─────────────────────────
// It asserts the SHAPE and the REACHABILITY, never the number. A committed test that pinned
// "the threshold is N" would go red for `system-ui` resolving differently, for a new badge, for a
// locale — none of them bugs. What is stable is: every party is rendered, every party can be
// reached, one thing scrolls, and the per-party cost has not tripled. The NUMBER goes in
// PLAN-ceremony-wizard.md with its conditions beside it, and this file is what re-derives it.
//
// ── The reference window is declared here because the app declares none ──────
// Searched: no `@media (max-height` rule exists in web/style.css, and internal/browser launches
// `--app` with no `--window-size`. So a threshold is meaningless until a window is named.
// 1280x768 — the width because `#sidebar` is a fixed 200px that `display:none`s below 899, so
// every width where the rail exists renders an identical card and width cannot be the axis; the
// height because 768 is what test/ui/responsive.test.mjs already measures against, and a
// threshold taken at the harness's default 900 is wrong on every 768-tall laptop.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './harness.mjs';

const REF = { width: 1280, height: 768 };
const SIZES = [2, 4, 8, 16, 24, 32];   // 32 is MaxRoster (internal/ceremony/invitation.go)
const ME = 'aa'.repeat(32);

// **TWO fixtures, because a single one does not measure a threshold — it measures a fixture.**
// `.cerparty` is a flex row whose children wrap internally in a 200px column, so the row's height
// is a function of the label and the capacity. A short fixture errs optimistic (a threshold too
// high) and a worst-case one errs pessimistic, and the honest product is the BRACKET plus the
// conditions, not one number presented as universal.
//
//   plain — a person's name and nothing else. The floor.
//   rich  — a long name AND a real capacity, which D20's amendment makes part of the agreement
//           rather than a label, so it is what a real commercial signing looks like.
//
// One row in each is this machine's, because `.cerme` and the `you` tag carry their own weight and
// their own border.
const FIXTURES = {
  plain: (i) => ({ label: `Party ${i + 1}` }),
  rich: (i) => ({
    label: `Alexandra Fairweather-Blount ${i + 1}`,
    capacity: 'as attorney-in-fact for Northbridge Holdings Ltd',
  }),
};
let shape = 'plain';
const roster = (n) => Array.from({ length: n }, (_, i) => ({
  fingerprint: i === 1 ? ME : String(i).padStart(2, '0').repeat(32),
  signs: true,
  ...FIXTURES[shape](i),
}));

let size = 2;
const listing = () => ({
  primary: true,
  ceremonies: [{
    id: '1'.repeat(32),
    state: 'ok',
    intent: shape === 'plain' ? 'The Elm Row lease' : 'We agree to the lease of 14 Elm Row, Edinburgh, for a term of five years',
    expires: '2026-10-01T12:00:00Z',
    me: ME,
    roster: roster(size),
  }],
  ended: [],
});

// `locale` and `timezoneId` are pinned: the deadline line is toLocaleDateString() +
// toLocaleTimeString(), so without them the fixture's height depends on the host's settings and
// the "measurement" is a fact about this machine.
const h = await launch({
  locale: 'en-GB',
  timezoneId: 'UTC',
  routes: {
    '**/api/ceremonies': (route) => route.fulfill({
      status: 200, contentType: 'application/json', body: JSON.stringify(listing()),
    }),
    // The worklist's own answer (P04.S02), stubbed so the rendering the threshold TRIGGERS can be
    // measured. Without it this file measured only the rendering the threshold replaces — which is
    // the half S02 changed, and the instrument was not re-pointed until the phase close.
    '**/api/ceremony/next*': (route) => route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({
        ceremony: '1'.repeat(32), state: 'waiting', label: 'Signer 1', position: 1, of: size,
        isMe: false, meKnown: true, worklist: true,
        parties: roster(size).map((p, i) => ({
          label: p.label, capacity: p.capacity,
          state: i < Math.floor(size / 2) ? 'signed' : i === Math.floor(size / 2) ? 'signing' : 'waiting',
          isMe: i === 1,
        })),
      }),
    }),
  },
});
const { page } = h;
after(() => h.browser.close());

// read() measures the PANEL, not one card. A coordinator with three 8-party ceremonies has exactly
// the problem D6 describes while every card is individually "below threshold", so a per-card metric
// cannot see the case the threshold is for.
const read = () => page.evaluate(() => {
  const pane = document.getElementById('sbFunctions');
  const card = document.querySelector('#ceremonyList .cercard');
  const r = (e) => e.getBoundingClientRect();
  const rows = [...document.querySelectorAll('#ceremonyList .cerparty')].filter((e) => r(e).height > 0);
  const action = document.querySelector('#ceremonyList .cernextbtn');
  const pr = r(pane);

  // **Hit-testing, because the two cheaper observables have both been measured LYING in this exact
  // flex nesting** — see the note in responsive.test.mjs. `scrollHeight` reported 223 = 223 while
  // holding 505px of items, because a flex column with a capped height clips its children without
  // establishing a scroll extent; and an element's own rect still HAS a position while the element
  // is invisible behind that clip. Only the document can say what is painted at a point.
  //
  // **It scrolls the NEAREST SCROLLABLE ANCESTOR, not the pane, and that correction is a finding
  // this file produced about itself.** The first cut scrolled `#sbFunctions`, copying
  // responsive.test.mjs, and every reading past n=2 came back unreachable with the pane reporting
  // `scrollHeight - clientHeight === 0`. The pane genuinely does not scroll here: `.sbpane >
  // .panel.active:not(#commands)` is `flex: 1 1 auto; min-height: 0`, so the panel claims the
  // leftover height, and `.panel` carries `overflow: auto` — **the ceremony panel is its own
  // scroller.** The accordion's cards and a content panel scroll in two different boxes, and a
  // probe that assumes one of them measures nothing about the other.
  const scrollerFor = (el) => {
    for (let p = el.parentElement; p; p = p.parentElement) {
      const oy = getComputedStyle(p).overflowY;
      if ((oy === 'auto' || oy === 'scroll') && p.scrollHeight > p.clientHeight + 1) return p;
      if (p === document.body) break;
    }
    return null;
  };
  const reachable = (el) => {
    if (!el) return null;
    const sc = scrollerFor(el);
    const before = sc ? sc.scrollTop : 0;
    if (sc) sc.scrollTop = sc.scrollHeight;
    const b = r(el);
    const hit = document.elementFromPoint(Math.round(b.left + b.width / 2), Math.round(b.top + b.height / 2));
    const ok = !!hit && (hit === el || el.contains(hit) || el.contains(hit.parentElement));
    if (sc) sc.scrollTop = before;
    return ok;
  };

  // How many things scroll inside the panel. The property responsive.test.mjs defends for the
  // accordion is that there is exactly ONE scroller; a nested one turns a long card into an island
  // the surrounding list appears to flow around.
  const scrollerEls = [pane, ...pane.querySelectorAll('*')]
    .filter((e) => e.scrollHeight > e.clientHeight + 1 && ['auto', 'scroll'].includes(getComputedStyle(e).overflowY));
  const scrollers = scrollerEls.length;
  const scrollerNames = scrollerEls.map((e) => e.id || e.className || e.tagName).slice(0, 4);

  return {
    rows: rows.length,
    cardH: card ? Math.round(r(card).height) : 0,
    paneH: Math.round(pr.height),
    // Does the whole card fit in the visible pane with no scrolling at all?
    fits: card ? Math.round(r(card).height) <= Math.round(pr.height) : false,
    // Is the card's one action visible WITHOUT scrolling? This is D6's actual subject.
    actionAbove: action ? r(action).bottom <= pr.bottom + 1 : null,
    lastRowReachable: reachable(rows[rows.length - 1]),
    actionReachable: reachable(action),
    scrollers,
    scrollerNames,
    // Diagnostics, recorded because the first run produced a combination that reads as impossible
    // — a card that FITS whose action is unreachable — and a number nobody can explain is not a
    // measurement.
    scrollExtent: pane.scrollHeight - pane.clientHeight,
    panelExtent: (() => { const q = document.getElementById('ceremony'); return q ? q.scrollHeight - q.clientHeight : -1; })(),
    actionH: action ? Math.round(r(action).height) : 0,
    hitAtAction: (() => {
      if (!action) return null;
      const sc = scrollerFor(action);
      const before = sc ? sc.scrollTop : 0;
      if (sc) sc.scrollTop = sc.scrollHeight;
      const b = r(action);
      const el = document.elementFromPoint(Math.round(b.left + b.width / 2), Math.round(b.top + b.height / 2));
      const out = { top: Math.round(b.top), paneTop: Math.round(pr.top), paneBottom: Math.round(pr.bottom),
        what: el ? (el.id || el.className || el.tagName) : null };
      if (sc) sc.scrollTop = before;
      return out;
    })(),
  };
});

async function at(n) {
  size = n;
  await page.setViewportSize(REF);
  await page.click('.modetab[data-tab="collaborate"]');
  await h.panel('ceremony');
  // The panel reloads on every mode entry, so a re-entry is how the new fixture reaches the DOM.
  await page.waitForFunction((want) =>
    document.querySelectorAll('#ceremonyList .cerparty').length === want, n, { timeout: 15000 });
  return read();
}

const measured = [];

test('the rail is measured at every roster size up to MaxRoster, on both fixtures', async () => {
  for (const sh of ['plain', 'rich']) {
    shape = sh;
    for (const n of SIZES) {
      const g = await at(n);
      measured.push({ shape: sh, n, ...g });
      // The floor for every assertion below: the fixture really did reach the DOM at this size.
      assert.equal(g.rows, n,
        `at ${n} parties (${sh}) the rail rendered ${g.rows} rows. Every other reading in this file `
        + 'is about a roster of this size, so a miscount makes all of them measurements of something '
        + 'else — and it is the guard that catches a future slice() silently truncating the rail');
    }
  }
  // The record. This is the file's product; the number it feeds goes in PLAN-ceremony-wizard.md.
  console.log('# rail geometry at %dx%d, en-GB/UTC:', REF.width, REF.height);
  for (const m of measured) {
    console.log('#   %s n=%d card=%d pane=%d actionAbove=%s reach=%s/%s scrollers=%d panelExt=%d',
      m.shape, m.n, m.cardH, m.paneH, m.actionAbove, m.lastRowReachable, m.actionReachable,
      m.scrollers, m.panelExtent);
  }
});

test('every party is reachable at every size, including a full roster', async () => {
  assert.equal(measured.length, SIZES.length * Object.keys(FIXTURES).length,
    `the sweep produced ${measured.length} readings, not ${SIZES.length * Object.keys(FIXTURES).length}. `
    + 'Every assertion below iterates that list, so a short sweep makes them all vacuous — this floor '
    + 'caught its own staleness when the second fixture was added, which is what it is for');
  for (const m of measured) {
    assert.equal(m.lastRowReachable, true,
      `at ${m.n} parties the last roster row cannot be reached: nothing is painted at its own centre `
      + 'after scrolling the pane to the bottom. That is the clip this repo has measured before — a '
      + 'flex column that clips rather than scrolls, where the row has a position but no visibility');
    assert.equal(m.actionReachable, true,
      `at ${m.n} parties the card's action cannot be reached after scrolling`);
  }
});

test('the panel is the only scroller, at every size', async () => {
  for (const m of measured) {
    // **The scroller is NAMED, not counted, and a probe is why.** Counting was the first form and a
    // mutation walked straight through it: capping `.cerroster` and giving it `overflow-y: auto`
    // makes the card short enough that the PANEL stops scrolling, so the count stays at one while
    // the one thing scrolling is the wrong element. A scroller inside a scroller turns a long card
    // into an island the surrounding list appears to flow around — the defect
    // `responsive.test.mjs` records being tried and reverted — and the count cannot see it.
    assert.ok(m.scrollers <= 1,
      `at ${m.n} parties (${m.shape}) ${m.scrollers} elements scroll: ${m.scrollerNames.join(', ')}`);
    if (m.scrollers === 1) {
      assert.equal(m.scrollerNames[0], 'ceremony',
        `at ${m.n} parties (${m.shape}) the thing that scrolls is "${m.scrollerNames[0]}", not the `
        + 'ceremony panel. The panel is the scroller by design — `.panel { overflow: auto }` plus '
        + '`flex: 1 1 auto; min-height: 0` — so anything else scrolling means a box inside the card '
        + 'has taken it over, and the roster scrolls independently of the card it belongs to');
    }
  }
});

test('the per-party cost has not tripled', async () => {
  const big = measured.find((m) => m.shape === 'plain' && m.n === 32);
  const small = measured.find((m) => m.shape === 'plain' && m.n === 4);
  assert.ok(big && small, 'setup: the sweep is missing an endpoint');
  const perParty = (big.cardH - small.cardH) / (big.n - small.n);
  // **A band, not a number.** The value is affine in N and its slope is one rendered row; the
  // assertion exists to catch a future element that triples a row, not to pin a pixel. `.cerparty`
  // is 12px/1.4 with 2px of padding, so a single-line row is ~19px and a two-line row ~36px — the
  // ceiling sits above the wrapped case so that wrapping is not a failure, and the floor above zero
  // so that a roster which stopped rendering rows is not read as "cheap".
  assert.ok(perParty >= 8 && perParty <= 40,
    `each additional party costs ${perParty.toFixed(1)}px of card on the PLAIN fixture. Outside `
    + '8..40 means a single-name row is no longer one or two lines — either an element was added to '
    + 'every party, or the roster stopped rendering and the cost collapsed');
});

// This file stubs `/api/ceremonies` and opens no document, so it leaves the shared server exactly
// as it found it. The VIEWPORT is restored, because it narrows the window and a later file reading
// a wider layout would be measuring this one's leftovers.
test('this file leaves the shared server as it found it', async () => {
  await page.setViewportSize({ width: 1280, height: 900 });
  const open = await page.evaluate(() => document.querySelectorAll('.viewerContainer .page').length);
  assert.equal(open, 0, `${open} page divs are open — this file never opens a document, so they are not its own`);
});

// **The rendering the threshold TRIGGERS, measured — which nothing did until P04's phase close.**
//
// S01 measured the card at six roster sizes and the plan's bracket was read off it. S02 then changed
// what the card renders above the threshold, and this file was not re-pointed: it never clicked
// "What happens next?" and never stubbed `/api/ceremony/next`, so tier 3 — the only tier with
// layout — measured only the rendering the threshold replaces.
//
// The property is the one the whole threshold exists for and it is a GEOMETRY property, so it can
// only be asserted here: the card the worklist produces must be SHORTER than the roster it replaced.
// At tier 2 the same claim is a row count with no heights behind it.
test('above the threshold the worklist makes the card shorter, not taller', async () => {
  size = 32;
  await page.setViewportSize(REF);
  await page.click('.modetab[data-tab="collaborate"]');
  await h.panel('ceremony');
  await page.waitForFunction((want) =>
    document.querySelectorAll('#ceremonyList .cerparty').length === want, size, { timeout: 15000 });

  const cardH = () => page.$eval('#ceremonyList .cercard', (el) => Math.round(el.getBoundingClientRect().height));
  const before = await cardH();
  assert.ok(before > 0, 'setup: the card has no height, so "shorter" is about nothing');

  await page.click('#ceremonyList .cernextbtn');
  await page.waitForSelector('#ceremonyList .cerworklist');
  const after = await cardH();

  console.log('# worklist at n=%d: card %dpx -> %dpx', size, before, after);
  assert.ok(after < before,
    `at ${size} parties the card is ${after}px with the worklist and was ${before}px without it. `
    + 'The worklist appears when the roster stops fitting, so a worklist that makes the card TALLER '
    + 'points the remedy in the opposite direction from the problem — which is what shipped: the '
    + 'roster was left rendered above it and 32 rows became 59');
  assert.equal(await page.$eval('#ceremonyList .cerroster', (el) => el.hidden), true,
    'the roster is still rendered beside the worklist');
});
