// The ceremony panel with the vault LOCKED, in a real browser (P06.S07, D29).
//
// **This is the tier the old claim died at.** P06.S02 marked *"the panel renders roster, position
// and next action with the vault locked"* met, and nothing anywhere had rendered the PANEL in that
// state: `boot.mjs` defaults to `state: 'ready'`, and the one file that overrides it
// (`firstrun.test.mjs`, `setup` and `key-missing`) asserts the overlay's own content and nothing
// behind it. In a browser it was not merely
// undriven — `applyStatus` shows `#authOverlay`, `role="dialog" aria-modal="true"`, for every state
// but ready, so the sidebar the panel lives in sits behind a modal. A tier-2 assertion on
// `element.hidden` cannot see that: the node is there, attached, and covered.
//
// So this file asserts **geometry and stacking**, which is the only reading that distinguishes
// "rendered" from "in the DOM behind a dialog": a non-zero box, and the topmost element at the
// panel's own centre point being inside the panel.
//
// It drives a SECOND nib that was never enrolled — uirepro.sh starts it and refuses to run if its
// status says `ready`, because a locked-view file against an unlocked app passes for the wrong
// reason. The ceremony list itself is a routed fixture: whether the server has ceremonies on disk
// is not what this tier is for, and stubbing it keeps the file independent of what its siblings
// left behind on a shared ~/nib.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch, LOCKED_BASE } from './harness.mjs';

const ID = '1'.repeat(32);
const CEREMONIES = {
  ceremonies: [{
    id: ID,
    state: 'ok',
    intent: 'We agree to co-sign the lease',
    expires: '2030-01-01T00:00:00Z',
    roster: [
      { fingerprint: 'a'.repeat(64), label: 'Ada Landlord', capacity: 'as Director', signs: true },
      { fingerprint: 'b'.repeat(64), label: 'Bo Tenant', signs: true },
    ],
    me: 'b'.repeat(64),
  }],
  primary: true,
};

const { browser, page } = await launch({
  base: LOCKED_BASE,
  waitFor: '#authOverlay',
  routes: {
    '**/api/ceremonies': (route) => route.fulfill({
      status: 200, contentType: 'application/json', body: JSON.stringify(CEREMONIES),
    }),
  },
});
after(() => browser.close());

test('it is driving a LOCKED nib, not the shared unlocked one', async () => {
  const st = await page.evaluate(async () => (await fetch('/api/status')).json());
  assert.notEqual(st.state, 'ready',
    `expected a locked vault, got ${st.state} — every assertion below would pass against an ` +
    'unlocked app, which is the fixture-shaped vacuous green this tier keeps finding');
  const overlayShown = await page.locator('#authOverlay').isVisible();
  assert.equal(overlayShown, true, 'setup: the unlock overlay is not showing');
});

test('the ceremony panel is visible and on top while the vault is locked', async () => {
  await page.waitForSelector('#authCeremonies', { state: 'visible' });
  const box = await page.locator('#authCeremonies').boundingBox();
  assert.ok(box && box.width > 0 && box.height > 0,
    `the locked ceremony panel has no box (${JSON.stringify(box)}) — it is in the DOM and not on ` +
    'the screen, which is exactly the state P06.S02 marked met');

  // **The reading that tier 2 cannot make.** `hidden === false` is true of an element under a
  // modal; what a user needs is that the panel is what they hit when they click it.
  const onTop = await page.evaluate(() => {
    const el = document.getElementById('authCeremonies');
    const r = el.getBoundingClientRect();
    const hit = document.elementFromPoint(r.left + r.width / 2, r.top + Math.min(20, r.height / 2));
    return !!hit && el.contains(hit);
  });
  assert.equal(onTop, true,
    'something else is on top of the locked ceremony panel at its own centre point. An aria-modal ' +
    'overlay over the panel is how this criterion was false for a slice while reading as met');

  const text = await page.locator('#authCerList').innerText();
  assert.match(text, /We agree to co-sign the lease/, 'the locked panel does not name the ceremony');
  assert.match(text, /Ada Landlord/, 'the locked panel does not render the roster');
  assert.match(text, /as Director/, 'the locked panel drops the capacity');
  const next = await page.locator('#authCerList .cernextbtn').count();
  assert.ok(next > 0, 'the locked panel offers no next-action control');

  // And the password is for signing, not for looking — said on the screen that makes the point.
  const note = await page.locator('#authCerNote').innerText();
  assert.match(note, /signing/, 'the locked screen does not say why it is showing this');
});
