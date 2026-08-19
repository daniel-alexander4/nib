// P01.S02, live: the identity panel leads with the six-word name, and the fingerprint is
// reachable by actually clicking the disclosure.
//
// ── Why this needs tier 3 when tier 2 already covers the same panel ─────────
// jsdom parses `<details>` but implements none of its behaviour: the summary is not
// focusable, clicking it does not toggle `open`, and nothing is hidden. So tier 2 can
// assert the STRUCTURE — the fingerprint is inside a `<details>` — and cannot assert the
// only thing a user cares about, which is that clicking the word "Fingerprint" reveals it
// and that not clicking leaves it genuinely off the screen.
//
// That distinction is the whole of the phase's W7 criterion: hex "reachable only behind
// the advanced disclosure, never merely de-emphasised in place". "Reachable" is a claim
// about a real browser, and this is where it is asked.
//
// Keyboard reachability is asserted for the same reason it was asserted for the thumbnail
// buttons in P06: a control that only a mouse can open is a control some users do not
// have. `<details>`/`<summary>` gets that natively, which is why it was chosen over a JS
// toggle — and an assertion is what turns "natively" from a claim into a fact.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch } from './harness.mjs';

const h = await launch();
const { page } = h;
after(() => h.browser.close());

const GROUPED = /(?:\b[0-9a-f]{4}\b[ \t]+){8,}/i;

test('the identity panel shows a name, and hex only after you open the disclosure', async () => {
  // The button lives in the settings dropdown, which only exists while its menu is open —
  // Playwright refuses a display:none target, which is the UI's own rule showing through
  // rather than a harness quirk. Same shape as the Save-as menu in stamplace.
  await page.click('.menu.settings > .menutop');
  await page.click('#managePeersBtn');
  await page.waitForFunction(() => !document.getElementById('peersModal').hidden);

  // The stimulus: the panel really rendered and really derived a name. Without this, every
  // absence assertion below is satisfied by a panel that failed to load.
  await page.waitForFunction(() => {
    const el = document.getElementById('peerSelfName');
    return el && el.textContent.trim().length > 0;
  }, null, { timeout: 15000 });

  const name = await page.$eval('#peerSelfName', (el) => el.textContent.trim());
  assert.equal(name.split(/\s+/).length, 6,
    `the panel shows ${JSON.stringify(name)}, which is not a six-word name — the server did ` +
    `not derive one, or the client is not rendering it`);
  assert.ok(!/[0-9a-f]{8}/i.test(name), `the name slot contains hex: ${name}`);

  // innerText, not textContent: this is a real browser, so it reports what is actually
  // rendered — a closed <details> contributes nothing. That is the difference between
  // "not visible" and "not in the DOM", and only one of them is what the criterion asks.
  const closed = await page.$eval('#peersModal', (el) => el.innerText);
  assert.ok(closed.includes(name), 'setup: the name is not in the panel’s rendered text');
  assert.ok(!GROUPED.test(closed),
    `a fingerprint is on screen before anything was opened. The criterion is that hex is ` +
    `reachable only behind the advanced disclosure, never merely de-emphasised in place. ` +
    `Rendered text: ${closed.slice(0, 240)}`);

  // Reachable — by clicking, the way a user does.
  await page.click('#peersModal details.advanced > summary');
  await page.waitForFunction(() => document.querySelector('#peersModal details.advanced').open);
  const opened = await page.$eval('#peersModal', (el) => el.innerText);
  assert.ok(GROUPED.test(opened),
    'opening the disclosure did not reveal the fingerprint — so hex is not merely hidden, ' +
    'it is unreachable, and pinning by hand is impossible');

  // And reachable without a mouse. `<details>` was chosen over a JS toggle for exactly
  // this; asserting it is what stops a later "nicer" replacement quietly removing it.
  await page.click('#peersModal details.advanced > summary'); // close again
  await page.waitForFunction(() => !document.querySelector('#peersModal details.advanced').open);
  const focused = await page.evaluate(() => {
    const s = document.querySelector('#peersModal details.advanced > summary');
    s.focus();
    return document.activeElement === s;
  });
  assert.ok(focused, 'the disclosure’s summary cannot take keyboard focus');
  await page.keyboard.press('Enter');
  await page.waitForFunction(() => document.querySelector('#peersModal details.advanced').open);
});

test('this file leaves the shared server as it found it', async () => {
  await page.click('#peersClose');
  await page.waitForFunction(() => document.getElementById('peersModal').hidden);
  const left = await page.evaluate(() => document.querySelectorAll('.viewerContainer .page').length);
  assert.equal(left, 0, `${left} page divs survive — this file opened a document it did not close`);
});
