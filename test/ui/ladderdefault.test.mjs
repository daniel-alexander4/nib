// P05.S12 — the traversal ladder is the default; the manual address is a disclosure.
//
// Tier 2 (test/jsdom/ladderdefault.test.mjs) proves the ADDRESS PLUMBING: an empty
// address reaches /api/session/initiate, and a typed one still does. What it cannot
// prove is the VISUAL claim of the slice — that the address field is off the default
// surface — because jsdom has no layout, so `<details>` collapse is unobservable there
// (boot.mjs's ceiling). That is this file's single job, in the real browser:
//
//   the co-sign dialog shows no address field until the user opens the Advanced
//   disclosure — so the LAN/DHT ladder is what a user reaches by default, and the
//   manual address (D8 tier 5) is present but undemoted, not deleted.
//
// Reverting T02 (moving #sinAddr back onto the default surface) makes the first
// assertion go red: the field is visible the moment the dialog opens. No peer is
// pinned in this harness, so this drives only the dialog's surface, not a co-sign —
// the co-sign itself needs two nibs and is tier 4's (pairrepro).
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const h = await launch();
const { browser, page } = h;
// This tier shares ONE nib process across files, and Open ADDS (the multiple-documents
// laws), so a document left open here shows up in the next file's launch and breaks its
// empty-state expectation. Leave the server as we found it: dismiss the modal, close the
// document, then the browser — tolerant of a failed assertion above so a real failure is
// not buried under cascade failures in later files.
after(async () => {
  try { await page.click('#sinCancel'); } catch {}
  try { await h.closeDocument(); } catch {}
  await browser.close();
});

test('the co-sign dialog keeps the address field behind the Advanced disclosure', async () => {
  await h.openDocument(writeFixture('deed.pdf', { pages: 1 }), 1);
  await h.mode('collaborate');
  await h.panel('commands'); // Collaborate lands on Flags; its commands are a sibling panel
  await page.click('#sessionInitBtn');
  await page.waitForSelector('#sessionInitModal:not([hidden])');

  // The default surface: no address field visible. This is the layout fact tier 2
  // cannot see — inside a closed <details>, the input is not rendered at all.
  assert.equal(await page.isVisible('#sinAddr'), false,
    'the address field is visible on the default co-sign surface — the manual path is not demoted');

  // And it is genuinely reachable — the disclosure opens it (D8 tier 5 undemoted).
  await page.click('#sessionInitModal details.advanced > summary');
  await page.waitForFunction(() => {
    const el = document.getElementById('sinAddr');
    return !!(el && el.offsetParent !== null);
  });
  assert.equal(await page.isVisible('#sinAddr'), true,
    'opening the Advanced disclosure did not reveal the address field — the fallback is unreachable');
});
