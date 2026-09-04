// P06.S02 — the ceremony panel renders in a REAL browser, with real layout and real CSS.
//
// Tier 2 (test/jsdom/ceremonypanel.test.mjs) proves the RENDERING LOGIC: which parties are drawn,
// which row is marked as this machine, that a damaged ceremony keeps its card and its sentence.
// What jsdom cannot prove is what this file exists for, because jsdom has no layout (boot.mjs's
// own ceiling):
//
//   the panel is REACHABLE — a tab that exists, in a mode that shows it, whose content is
//   actually displayed — and the marked row is visibly distinguished rather than merely
//   carrying a class nothing renders.
//
// **The route is stubbed and that is the right split, not a shortcut.** Whether the server answers
// correctly is tier 1's (`internal/server/ceremonylocked_test.go`) and tier 6's (ceremonyrepro's
// CLAUSE 12, which drives a real locked process). This tier's job is the surface. Convening a real
// ceremony here would re-drive what two tiers already cover and would test the panel through a
// path this slice does not own.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './harness.mjs';

const ME = 'aa'.repeat(32);
const THEM = 'bb'.repeat(32);

const listing = {
  primary: true,
  ceremonies: [
    {
      id: '1'.repeat(32),
      state: 'ok',
      intent: 'We agree to the lease of 14 Elm Row',
      expires: '2026-10-01T12:00:00Z',
      me: ME,
      roster: [
        { fingerprint: THEM, label: 'Bob Landlord', signs: true },
        { fingerprint: ME, label: 'Alice Tenant', capacity: 'as attorney-in-fact', signs: true },
      ],
    },
    { id: '2'.repeat(32), state: 'unparseable', reason: 'this ceremony is damaged and Nib cannot read it' },
  ],
  ended: [],
};

const { browser, page, consoleErrors } = await launch({
  routes: {
    '**/api/ceremonies': (route) => route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(listing),
    }),
  },
});
after(async () => { await browser.close(); });

test('the ceremony panel is reachable from Collaborate and renders the roster', async () => {
  await page.click('.modetab[data-tab="collaborate"]');

  // **The tab is DISPLAYED, not merely present.** `syncSidebarForMode` hides the tabs that do not
  // belong to a mode, so a panel added to the wrong mode would still be in the DOM and would fail
  // exactly here rather than silently never being seen.
  const tab = page.locator('.tabs .tab[data-panel="ceremony"]');
  assert.equal(await tab.isVisible(), true,
    'the Ceremony tab is not shown in Collaborate — the panel is in the document and out of reach');

  // **Flags is still the mode's default surface**, which is the other half of the ordering
  // decision. Listing the ceremony panel first made it the Collaborate landing screen and took
  // Flags off it; eight tests in this tier went red waiting for a Flags control. Asserted here so
  // the ordering cannot drift back without something saying so.
  assert.equal(await page.locator('#flags').isVisible(), true,
    'the Flags panel is no longer the Collaborate default. Making the ceremony panel the landing ' +
    'screen is a real product decision about where this mode lands, and it is not P06.S02\'s');

  await tab.click();
  const host = page.locator('#ceremonyList');
  assert.equal(await host.isVisible(), true, 'the ceremony panel does not display its content');

  const text = await host.innerText();
  assert.match(text, /We agree to the lease of 14 Elm Row/, 'the recital is not rendered');
  assert.match(text, /You are party 2 of 2\./, 'the panel does not say which party this machine is');
  assert.match(text, /this ceremony is damaged/,
    "the damaged ceremony's sentence is missing — and a ceremony Nib will not admit exists is one " +
    'whose only remedy is finding and deleting the folder by hand');
  assert.doesNotMatch(text, new RegExp(ME.slice(0, 16)), 'a hex fingerprint is displayed');
});

test('the row for this machine is visibly distinguished, not just classed', async () => {
  await page.click('.modetab[data-tab="collaborate"]');
  await page.click('.tabs .tab[data-panel="ceremony"]');

  // **The COMPUTED colour, because a class with no rule behind it is the failure this tier is for.**
  // Tier 2 asserts `.cerme` is applied and cannot see whether anything renders it; a stylesheet that
  // never shipped the rule would leave the marked row identical to its neighbours and every jsdom
  // assertion green.
  const mineColor = await page.locator('.cerparty.cerme .cerwho').evaluate((el) => getComputedStyle(el).color);
  const theirColor = await page.locator('.cerparty:not(.cerme) .cerwho').first()
    .evaluate((el) => getComputedStyle(el).color);
  assert.notEqual(mineColor, theirColor,
    `this machine's roster row renders identically to the others (${mineColor}). The class is ` +
    'applied and nothing draws it, which is exactly what tier 2 cannot see');

  assert.equal(await page.locator('.cerparty.cerme .certag').isVisible(), true,
    'the "you" tag is not displayed');
});

test('the panel logged no console errors', async () => {
  assert.deepEqual(consoleErrors, [], `console errors: ${consoleErrors.join('; ')}`);
});

// ── P06.S04: convene, in a real browser ──────────────────────────────────────────────────────
//
// Tier 2 proves the request shape and the sentences. What jsdom cannot prove, because it has no
// layout, is that the form is REACHABLE and its controls are DISPLAYED — a form added to the panel
// but never revealed, or a picker rendered into a container with no height, is invisible there and
// green.
test('the convene form opens from the panel and offers a pinned peer to choose', async () => {
  await page.click('.modetab[data-tab="collaborate"]');
  await page.click('.tabs .tab[data-panel="ceremony"]');

  // SETUP: the form starts hidden, or "it is visible after the click" is true of a form that is
  // always visible and the click proves nothing.
  assert.equal(await page.locator('#ceremonyConveneForm').isVisible(), false,
    'the convene form is showing before anything was clicked');

  await page.click('#ceremonyConveneBtn');
  assert.equal(await page.locator('#ceremonyConveneForm').isVisible(), true,
    'the convene form does not open');
  assert.equal(await page.locator('#ceremonyAcceptForm').isVisible(), false,
    'both forms are showing at once — they are two answers to one question, and a screen ' +
    'offering both invites a user to fill in the wrong one');

  // The picker is displayed, not merely present.
  //
  // **Waited for, and this was a latent race that an unrelated change exposed (P06.S09).**
  // `showCeremonyForm` calls `loadPeerPicker()` WITHOUT awaiting it, so at the instant the click
  // returns the div has been cleared and not yet filled — zero height, and `isVisible()` false.
  // The assertion happened to pass because the stubbed `/api/peers` usually resolved inside the
  // click's own microtask turn; adding six lines elsewhere in `web/app.js` was enough to lose that
  // race, 2 runs red against 3 green at the parent commit. Measured, not guessed: an instrumented
  // run printed `{"html":"","w":167,"h":0}` — cleared, empty, and still waiting.
  //
  // The product is not at fault and is deliberately unchanged: an un-awaited load is the right
  // shape for a panel that must not block its own dialog opening. What was wrong is a test that
  // read a value before the thing producing it had run.
  const pick = page.locator('#cerPeerPick');
  await pick.locator('.cerpeerrow, .libhint').first().waitFor({ state: 'visible', timeout: 10000 });
  assert.equal(await pick.isVisible(), true, 'the peer picker is not displayed');

  // **No hex anywhere the user can see**, asserted over the form's rendered text rather than its
  // markup — which is the only place this can be asked, since the fingerprint IS in the markup by
  // design, carried in a data attribute so it reaches the server without reaching the screen.
  const shown = await page.locator('#ceremonyConveneForm').innerText();
  assert.doesNotMatch(shown, /[0-9a-f]{32}/i,
    `the convene form displays a hex string: ${JSON.stringify(shown.slice(0, 200))}. The phase's ` +
    'criterion is that the primary flow contains no hex fingerprint');

  // And the way out is present and is not a hex box: pairing happens under Identity & peers,
  // which is where the read-it-aloud comparison and the Copy buttons already are.
  assert.match(shown, /Identity & peers/,
    'a convener whose counterparty is not yet pinned is offered no way forward. The advanced ' +
    'path is reachable and off the default flow, not absent');
});
