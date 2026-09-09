// P03.S01 — the setup sheet, in a real browser.
//
// **This tier exists for one clause tier 2 cannot reach: the reader's page surviving the round
// trip.** jsdom lays nothing out, so a hidden-then-shown viewer is indistinguishable there from one
// that never moved. In a browser it is exactly the defect `/pending 372` recorded — *"nothing
// survives pdf.js re-laying the document out"*, because `currentPageNumber` runs
// `resetCurrentPageView` and `scrollIntoView` scrolls willingly — and `#viewerWrap` had never been
// hidden by anything before this slice.
//
// The second clause is the one the sheet exists for: the roster picker and its two fields are
// usable at 1024×768, with the sidebar still rendering the rail beside them.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

// **One stubbed route, and it is what makes the convene test possible at all.** This tier's nib
// enrols a fresh vault with no pinned peers, and the client refuses to post a convene with an empty
// roster ("Choose at least one other person to sign") — so without a peer the request under test is
// never made. The server still refuses the fingerprint; the request is the subject, not its answer.
const h = await launch({
  routes: {
    '**/api/peers': (route) => route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ self: 'f'.repeat(64), peers: [{ fingerprint: 'a'.repeat(64), label: 'lively otter marble finch amber cove' }] }),
    }),
  },
});
const { page } = h;
after(() => h.browser.close());

const DOC = writeFixture('setupsheet.pdf', { pages: 6, label: 'setup page' });

// **The viewer's own SCROLL position, not the `.pageNum` input** — and the difference is a probe
// finding. `.pageNum` is written by the app on open and close (`app.js:2633`), not on scroll, so it
// reported the page it was last told about whatever the viewer did: removing the restore entirely
// left this test green. Scroll is a fact the layout owns and pdf.js resets.
const scrollTop = () => page.evaluate(() => {
  const c = document.querySelector('.viewerContainer:not([hidden])');
  return c ? Math.round(c.scrollTop) : -1;
});

test("the reader's page survives a trip through the setup sheet", async () => {
  await h.openDocument(DOC, 6);
  // Move off page 1, or "the page did not change" is true of a build that resets to 1.
  await page.evaluate(() => {
    const i = document.querySelector('.pageNum');
    i.value = '4';
    i.dispatchEvent(new Event('change', { bubbles: true }));
  });
  await page.waitForFunction(() => Number(document.querySelector('.pageNum')?.value) === 4);
  await page.waitForFunction(() => {
    const c = document.querySelector('.viewerContainer:not([hidden])');
    return c && c.scrollTop > 0;
  });
  const before = await scrollTop();
  assert.ok(before > 0,
    `setup: the viewer is scrolled to ${before}, so it never left page 1 and "the page survived" `
    + 'would be true of a build that resets to the top');

  await page.click('.modetab[data-tab="collaborate"]');
  // The mode lands on its Commands panel since v1.121.0, so the ceremony panel is asked for by
  // name rather than assumed open — the same helper `pageops.test.mjs` uses for thumbs.
  await h.panel('ceremony');
  await page.click('#ceremonyConveneBtn');
  await page.waitForSelector('#ceremonySheet:not([hidden])');
  // The viewer really did stand down — otherwise this asserts a round trip that never happened.
  assert.equal(await page.$eval('#viewerWrap', (el) => el.hidden), true,
    'setup: the viewer is still shown, so the sheet did not take its place and no re-layout occurs');

  await page.click('#cerSheetClose');
  await page.waitForSelector('#viewerWrap:not([hidden])');
  await page.waitForFunction(() => document.querySelector('.viewerContainer:not([hidden])'));

  const after = await scrollTop();
  // A tolerance, because a re-layout can land a page a pixel or two off and the clause is about the
  // reader's PLACE, not about an exact offset. A reset to the top is hundreds of pixels away.
  assert.ok(Math.abs(after - before) < 40,
    `the reader was ${before}px down the document and came back to ${after}px. Opening setup hides `
    + 'the viewer, and pdf.js re-lays the document out when it comes back — /pending 372 is that '
    + 'exact defect, and this is the first thing in the app that has ever hidden #viewerWrap');
});

test('the setup form has room at 1024x768, with the rail still beside it', async () => {
  await page.setViewportSize({ width: 1024, height: 768 });
  await page.click('.modetab[data-tab="collaborate"]');
  // The mode lands on its Commands panel since v1.121.0, so the ceremony panel is asked for by
  // name rather than assumed open — the same helper `pageops.test.mjs` uses for thumbs.
  await h.panel('ceremony');
  await page.click('#ceremonyConveneBtn');
  await page.waitForSelector('#ceremonySheet:not([hidden])');

  const box = await page.$eval('#ceremonySheet', (el) => {
    const r = el.getBoundingClientRect();
    return { w: Math.round(r.width), h: Math.round(r.height), left: Math.round(r.left) };
  });
  const side = await page.$eval('#sidebar', (el) => {
    const r = el.getBoundingClientRect();
    return { w: Math.round(r.width), visible: r.width > 0 && r.height > 0 };
  });

  // The whole point of the slice: more than a 200px column.
  assert.ok(box.w > 400,
    `the sheet is ${box.w}px wide at a 1024px viewport, which is not the room a roster picker and `
    + 'two fields were moved out of the sidebar to get');
  assert.ok(side.visible && side.w > 0,
    'the sidebar is gone while setup is open, so the running rail went with it — D3 keeps the rail '
    + 'because Running is document-referential and interrupt-driven');
  assert.ok(box.left >= side.w - 1,
    `the sheet starts at ${box.left}px and the sidebar is ${side.w}px wide, so they overlap — the `
    + 'sheet takes the document column, not the whole window');

  // Every control the convener has to reach is inside the viewport, not below its fold.
  const spill = await page.evaluate(() => {
    const ids = ['cerIntent', 'cerExpires', 'cerPeerPick', 'cerConveneGo'];
    return ids.filter((id) => {
      const el = document.getElementById(id);
      if (!el) return true;
      const r = el.getBoundingClientRect();
      return r.bottom > window.innerHeight || r.right > window.innerWidth;
    });
  });
  assert.deepEqual(spill, [],
    `these setup controls fall outside a 1024x768 viewport: ${spill.join(', ')}. The sheet exists `
    + 'so the roster, recital and deadline have room; controls below the fold is the 200px column '
    + 'problem in a wider box');
});

// ── P03.S04 — leaving the sheet for the document, and coming back ─────────────────────────────
//
// **What only this tier can see.** Two things, and both are geometry. First, that the way back is
// actually ON SCREEN during the excursion: jsdom reports `hidden === false` for an element whose
// container is `display: none`, so tier 2's "the bar is not hidden" is true of a build where nobody
// can see or click it. Second, that it survives the sidebar collapsing — which is the reason the
// control is in `#viewerWrap` at all. `#sidebar.collapsed { display: none }` and a crossing
// listener collapses the sidebar automatically below 899px, so a return control in the Flags or
// Ceremony panel disappears when a user narrows the window mid-excursion, leaving a half-filled
// ceremony in memory with nothing on screen saying so. That is a computed-style fact, and this is
// the only tier that computes styles.
test('the way back is on the document, and survives the sidebar going away', async () => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await h.openDocument(DOC, 6);
  await page.click('.modetab[data-tab="collaborate"]');
  await h.panel('ceremony');
  await page.click('#ceremonyConveneBtn');
  await page.waitForSelector('#ceremonySheet:not([hidden])');

  const RECITAL = 'We agree to the lease of 14 Elm Row';
  await page.fill('#cerIntent', RECITAL);

  // Count the two requests that ARE the rebuild, from here to the end of the return leg.
  //
  // **GET only, and that is a correction this tier made.** The first cut counted every request to
  // either path and went red at two — both POSTs to `/api/ceremony/draft`, which are the draft
  // being SAVED as `change` fires on blur. That is the draft working, not a rebuild. jsdom never
  // showed it because setting `.value` from script fires no `change`, so tier 2's version of this
  // assertion was green against a build that saves and one that does not. The rebuild door READS:
  // `loadPeerPicker` GETs `/api/peers` and `restoreCeremonyDraft` GETs `/api/ceremony/draft`.
  const rebuildCalls = [];
  const writes = [];
  const onRequest = (r) => {
    const u = r.url();
    if (!u.includes('/api/peers') && !u.includes('/api/ceremony/draft')) return;
    if (r.method() === 'GET') rebuildCalls.push(u); else writes.push(u);
  };
  page.on('request', onRequest);

  await page.click('#cerSeeDoc');
  await page.waitForSelector('#viewerWrap:not([hidden])');

  // The bar is really painted, over the real document, at a real size.
  assert.ok(await page.locator('#cerSetupBar').isVisible(),
    'the parked-setup bar is not visible, so the user stepped out of a half-filled ceremony with '
    + 'nothing on screen saying so and no route back');
  const inside = await page.evaluate(() => {
    const b = document.getElementById('cerSetupBar').getBoundingClientRect();
    const v = document.getElementById('viewerWrap').getBoundingClientRect();
    return b.width > 0 && b.height > 0 && b.left >= v.left - 1 && b.bottom <= v.bottom + 1;
  });
  assert.ok(inside,
    'the bar is not laid out inside the viewer. It has to be on the DOCUMENT, because the document '
    + 'is the one surface guaranteed present while parked — parked is #viewerWrap showing');
  // And the page really is back, which is what the excursion is FOR.
  assert.ok(await page.locator('.viewerContainer:not([hidden]) .page').first().isVisible(),
    'the document did not come back, so there is nothing to step out to');
  // **Visible is not reachable, and only a hit test separates them.** `.viewerContainer` is
  // positioned with `z-index: auto` and creates no stacking context, so the page's own overlays
  // (`.ovl` 8, its variants 9, `.shapemark` 10) paint in #viewerWrap's context and are
  // pointer-interactive. A bar that lost its z-index is still `isVisible()` and still has the right
  // rect — Playwright's click just times out, which is a failure with no assertion behind it. This
  // asks the question directly, so the defect has a sentence.
  const onTop = await page.evaluate(() => {
    const r = document.getElementById('cerBackToSetup').getBoundingClientRect();
    const el = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
    return !!(el && (el.id === 'cerBackToSetup' || el.closest('#cerSetupBar')));
  });
  assert.ok(onTop,
    'the way back is painted under the document — something else answers a hit test at the centre '
    + 'of "Back to setup". The bar carries the ONLY route back to a half-filled ceremony, so it has '
    + 'to outrank the page overlays rather than share #signBanner\'s rank');
  // **Focus landing is a browser fact.** jsdom will focus an element inside a `display: none`
  // subtree and report it as `activeElement`, so tier 2's version of this assertion is green
  // against a focus call placed BEFORE the visibility flip — where a real browser silently refuses
  // and drops focus to <body>. That ordering is one line apart from the shipped one.
  assert.equal(await page.evaluate(() => document.activeElement?.id), 'cerBackToSetup',
    'focus did not land on the way back. The control the user pressed has just been hidden, so '
    + 'focus is on <body> and her next Tab restarts from the top of the page (SC 2.4.3)');

  // Now take the sidebar away — the exact thing that happens on its own below 899px.
  await page.click('#toggleSidebarBtn');
  await page.waitForFunction(() => document.getElementById('sidebar').classList.contains('collapsed'));
  assert.equal(await page.locator('#sidebar').isVisible(), false,
    'setup: the sidebar is still visible, so "the way back survives it going away" is vacuous');
  assert.ok(await page.locator('#cerSetupBar').isVisible(),
    'the way back went with the sidebar. Below 899px the sidebar collapses on its own, so a user '
    + 'who narrows the window mid-excursion would be left with no route back to her ceremony');

  await page.click('#toggleSidebarBtn');
  await page.waitForFunction(() => !document.getElementById('sidebar').classList.contains('collapsed'));

  await page.click('#cerBackToSetup');
  await page.waitForSelector('#ceremonySheet:not([hidden])');
  page.off('request', onRequest);

  assert.equal(await page.$eval('#cerIntent', (el) => el.value), RECITAL,
    'the recital did not survive the trip to the document and back');
  assert.equal(await page.$eval('#viewerWrap', (el) => el.hidden), true,
    'the sheet came back without standing in the document\'s place');
  assert.equal(await page.locator('#cerSetupBar').isVisible(), false,
    'the parked bar is still showing behind the sheet, telling the user setup is paused while she '
    + 'is looking at it');
  // The stimulus floor for the assertion below: this listener really is seeing traffic on these
  // paths, so an empty GET list means "no read happened" rather than "nothing was observed".
  assert.ok(writes.length > 0,
    'no write to /api/ceremony/draft was observed across the trip, so the request listener saw no '
    + 'traffic on these paths at all and "no rebuild request" is a statement about the listener');
  assert.equal(await page.evaluate(() => document.activeElement?.id), 'cerIntent',
    'focus did not land back in the sheet. At the moment of the call #cerIntent is inside both a '
    + 'hidden sheet and a hidden form, so a focus placed before the flips is a silent no-op here '
    + 'and green in jsdom');
  assert.deepEqual(rebuildCalls, [],
    `the round trip made ${rebuildCalls.length} rebuild request(s) (${rebuildCalls.join(', ')}). `
    + 'The clause is that the sheet is RE-ENTERED rather than rebuilt, and the rebuild is exactly '
    + 'loadPeerPicker + restoreCeremonyDraft — which empties the picker before its fetch and '
    + 'replaces anything typed and not yet blurred with the last committed draft');
});

// ── P03.S04 — the ceremony is convened over the document setup was STARTED on ────────────────
//
// **A corruption channel the repo already had a guard for, that the guard structurally could not
// see.** `/api/ceremony/convene` is in `test/jsdom/pinning.test.mjs`'s MUTATING inventory, and the
// comment beside it names this exact defect: *"an unpinned convene would commit a ceremony record
// into whichever tab the user switched to while it ran."* But `scanUnpinned` finds mutating calls
// "preceded by an `await` in its own function", and in `conveneFromPanel` the convene POST **is**
// the first await — so the scan skipped it. The pause that lets the document change is not an
// `await` at all: it is the user, filling in a form.
//
// **And this slice is what drives through it.** `#tabstrip` is a sibling of the sheet and
// `showCeremonySheet` never touches it, so the switcher has always been live behind the sheet;
// before P03.S04 the only ways out read as abandonment. `parkCeremonySheet` now invites the user
// onto the page with the strip right there, and names "checking the file is the right one" as a
// reason to go.
//
// **404 versus 409 is the whole assertion, and it is ADR-004's own contract.** `docFor` answers 409
// for a header naming a document the server no longer holds, and `resolveDoc` answers 404 when
// there is no header and nothing is open. So closing the document mid-excursion separates the two
// builds cleanly: pinned refuses the ceremony by name, unpinned reports "no document open" — and
// with a second document open the unpinned build would not refuse at all. It would convene.
test('the convene is bound to the document the setup was started on', async () => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await h.openDocument(DOC, 6);
  await page.click('.modetab[data-tab="collaborate"]');
  await h.panel('ceremony');
  await page.click('#ceremonyConveneBtn');
  await page.waitForSelector('#ceremonySheet:not([hidden])');
  await page.waitForSelector('#cerPeerPick .cerpeerrow');

  await page.fill('#cerIntent', 'We agree to the lease of 14 Elm Row');
  await page.fill('#cerExpires', '2027-10-01T12:00');
  await page.check('#cerPeerPick .cerpeerbox');

  // Out to the document, and then the document goes away underneath the parked setup — which is
  // the cheapest reachable stand-in for "the user switched tabs", and the only one that separates
  // the two builds by STATUS rather than by which document got rewritten.
  await page.click('#cerSeeDoc');
  await page.waitForSelector('#viewerWrap:not([hidden])');
  // The same loop this file's cleanup test uses. A single closeDocument() is not enough to assert
  // against: it returns after a fixed 400 ms and the class is the state the app itself reads.
  for (let i = 0; i < 8 && await page.$eval('#viewerWrap', (el) => el.className) === 'has-doc'; i++) {
    await h.closeDocument();
  }
  assert.equal(await page.evaluate(() => document.querySelectorAll('.viewerContainer .page').length), 0,
    'the document did not close, so the convene below still has a live document to fall back on and '
    + 'a 404 would be indistinguishable from a 409');

  const statuses = [];
  const onResponse = (r) => { if (r.url().includes('/api/ceremony/convene')) statuses.push(r.status()); };
  page.on('response', onResponse);

  await page.click('#cerBackToSetup');
  await page.waitForSelector('#ceremonySheet:not([hidden])');
  await page.click('#cerConveneGo');
  await page.waitForFunction(() => {
    const e = document.getElementById('cerConveneError');
    return e && !e.hidden && e.textContent.length > 0;
  });
  page.off('response', onResponse);

  assert.equal(statuses.length, 1,
    `the convene was posted ${statuses.length} time(s); the client refused before reaching the `
    + 'server, so this test measured its own form validation rather than the pin');
  assert.equal(statuses[0], 409,
    `the convene answered ${statuses[0]}, not 409. 409 is "that document is no longer open" — the `
    + 'request named the document setup was started on. 404 is "no document open", which is what an '
    + 'UNPINNED convene gets here: it asks after whatever is current instead, and with a second '
    + 'document open that is not a refusal at all, it is a ceremony convened over the wrong file');

  // This test is the only one here that closes the document as part of its subject, and the cleanup
  // test below asserts it had something to clean — a floor that exists so the cleanup cannot pass
  // vacuously. Re-opening restores that precondition rather than weakening the floor.
  await page.click('#cerSheetClose');
  await h.openDocument(DOC, 6);
});

// **The no-capture case, which the first version of the pin left open.** A convener can reach the
// setup sheet with nothing loaded — Nib launched empty, Collaborate, "Convene a ceremony…" — and a
// null pin is NOT a refusal: `docFor` answers an absent `X-Nib-Doc` with `activeDoc()`, so the
// ceremony would land on whatever the excursion happened to open, unpinned, which is the channel
// ADR-001 exists to close. `bindCeremonySetupDoc` fills an empty binding on the way back and can
// never re-point a full one.
//
// Driven the same way as the test above, and for the same reason: 409 ("that document is no longer
// open") is reachable only by a request that NAMED a document, and 404 ("no document open") is what
// the unbound build gets.
test('a setup opened with no document binds to the one the excursion opens', async () => {
  await page.setViewportSize({ width: 1280, height: 900 });
  for (let i = 0; i < 8 && await page.$eval('#viewerWrap', (el) => el.className) === 'has-doc'; i++) {
    await h.closeDocument();
  }
  assert.equal(await page.$eval('#viewerWrap', (el) => el.className), '',
    'setup: a document is still open, so the sheet would capture it and the binding below is not the '
    + 'no-capture case this test is about');

  await page.click('.modetab[data-tab="collaborate"]');
  await h.panel('ceremony');
  await page.click('#ceremonyConveneBtn');
  await page.waitForSelector('#ceremonySheet:not([hidden])');
  await page.waitForSelector('#cerPeerPick .cerpeerrow');
  await page.fill('#cerIntent', 'We agree to the lease of 14 Elm Row');
  await page.fill('#cerExpires', '2027-10-01T12:00');
  await page.check('#cerPeerPick .cerpeerbox');

  // Out to the page with nothing on it, open the lease there, and come back — the supported path,
  // and the one that used to leave the setup bound to nothing.
  await page.click('#cerSeeDoc');
  await page.waitForSelector('#viewerWrap:not([hidden])');
  await h.openDocument(DOC, 6);
  await page.click('.modetab[data-tab="collaborate"]');
  await h.panel('ceremony');
  await page.click('#cerBackToSetup');
  await page.waitForSelector('#ceremonySheet:not([hidden])');
  assert.equal(await page.$eval('#cerIntent', (el) => el.value), 'We agree to the lease of 14 Elm Row',
    'the recital was lost on the way back, so the trip went through the rebuild and the binding '
    + 'below would be measuring a different setup');

  const statuses = [];
  const onResponse = (r) => { if (r.url().includes('/api/ceremony/convene')) statuses.push(r.status()); };
  page.on('response', onResponse);

  // Now take that document away. A BOUND setup names it and is refused 409; an unbound one asks
  // after whatever is current and gets 404.
  await page.click('#cerSeeDoc');
  await page.waitForSelector('#viewerWrap:not([hidden])');
  for (let i = 0; i < 8 && await page.$eval('#viewerWrap', (el) => el.className) === 'has-doc'; i++) {
    await h.closeDocument();
  }
  await page.click('.modetab[data-tab="collaborate"]');
  await h.panel('ceremony');
  await page.click('#cerBackToSetup');
  await page.waitForSelector('#ceremonySheet:not([hidden])');
  await page.click('#cerConveneGo');
  await page.waitForFunction(() => {
    const e = document.getElementById('cerConveneError');
    return e && !e.hidden && e.textContent.length > 0;
  });
  page.off('response', onResponse);

  assert.equal(statuses.length, 1,
    `the convene was posted ${statuses.length} time(s); the client refused before reaching the server`);
  assert.equal(statuses[0], 409,
    `the convene answered ${statuses[0]}, not 409 — so the setup never bound to the document the `
    + 'excursion opened. A null binding is not a refusal: the server answers an absent X-Nib-Doc '
    + 'with whatever document is active, so this ceremony would have landed on a file the setup was '
    + 'never started on');

  await page.click('#cerSheetClose');
  await h.openDocument(DOC, 6);
});

// **This file leaves the shared server as it found it**, which is this tier's own convention and
// not optional: the files share one server, so a document left open is counted by the next file as
// its own. Measured when this file first shipped without it — `stamplace.test.mjs` timed out at 30s
// waiting for a state my leftover document had already changed, and it had been green.
//
// The viewport is restored too. It is per-page rather than shared, but the second test above
// narrows it to 1024×768 and a later test in THIS file reading a wider layout would be measuring
// the previous test's leftovers.
test('this file leaves the shared server as it found it', async () => {
  await page.setViewportSize({ width: 1280, height: 900 });
  // **And no saved ceremony draft**, which this file is the first here to create: filling #cerIntent
  // blurs it on the next click, which POSTs the draft to the vault. It survives a page reload, so
  // `launch()` does not clear it for the next file — a server-side leftover of exactly the kind the
  // note below forbids.
  const cleared = await page.evaluate(async () => {
    // The token is module state in app.js, not in the DOM, so it is re-read from the same place
    // app.js reads it — /api/status — rather than reached for through a global that does not exist.
    const csrf = (await (await fetch('/api/status')).json()).csrf;
    const r = await fetch('/api/ceremony/draft', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf },
      body: JSON.stringify({ draft: '' }),
    });
    return r.status;
  });
  assert.equal(cleared, 200,
    `clearing this file's ceremony draft answered ${cleared}; a draft left in the vault survives a `
    + 'page reload, so the next file in this tier would find a setup sheet that is not empty');
  const openPages = (await h.counts()).pages;
  for (let i = 0; i < 8 && await page.$eval('#viewerWrap', (el) => el.className) === 'has-doc'; i++) {
    await h.closeDocument();
  }
  const left = await page.evaluate(() => document.querySelectorAll('.viewerContainer .page').length);

  assert.ok(openPages > 0, 'setup: no document was open, so this cleanup covered nothing');
  assert.equal(left, 0,
    `${left} page divs survive the close — the next file in this tier will count them as its own`);
});
