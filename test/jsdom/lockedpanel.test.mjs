// The ceremony panel with the vault LOCKED (P06.S07, D29, C-D29's looking half).
//
// **Nothing had ever rendered the ceremony PANEL locked.** `boot.mjs` answers `/api/status` with
// `state: 'ready'` and one file overrides it — `firstrun.test.mjs`, with `setup` and `key-missing` —
// and what that file asserts is the overlay's own content: the warning sentence, the key choice.
// Never anything behind it. So the criterion *"the panel renders roster, position and next action
// with the vault locked"* was marked met at P06.S02 by tests that never rendered it in that state. In a browser it was worse than undriven: `applyStatus`
// shows `#authOverlay` — `role="dialog" aria-modal="true"` — for every state but `ready`, so the
// sidebar the panel lives in sits behind a modal.
//
// The routes were never the obstacle. S01 and S03 put `/api/ceremonies` and `/api/ceremony/next`
// on `requirePublicLoopback`, and `ceremonyCard`'s only action reads the second — so this surface
// needs the vault for nothing, which is what lets it be drawn inside the lock screen.
//
// **What this file cannot see**: whether the box is legible or reachable by keyboard in a real
// browser. It asserts the DOM, and the modal that defeated the old claim is a rendering fact —
// tier 3 covers that, and the two together are the criterion.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

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
    // **This ceremony qualifies for the delivery control in every respect but one: the lock.**
    // It is ended, and this machine convened it — so the arm below is testing the lock and not
    // some other gate silently doing the work. A fixture that failed any other condition would
    // pass that assertion while the locked screen happily rendered the button.
    convener: 'b'.repeat(64),
    ended: 'completed',
  }],
  primary: true,
};

const h = await boot({
  routes: {
    // THE STIMULUS. Everything else in this file is downstream of this one field.
    '/api/status': {
      state: 'key-locked', version: 'test', autoUpdate: false, updateCheckLocked: false,
      ghostscript: false, libreoffice: false,
    },
    '/api/ceremonies': () => CEREMONIES,
  },
});
const { document: doc, settle } = h;

test('the ceremony panel renders while the vault is locked', async () => {
  await settle();

  const overlay = doc.getElementById('authOverlay');
  const box = doc.getElementById('authCeremonies');
  const list = doc.getElementById('authCerList');
  assert.ok(overlay, 'there is no #authOverlay in index.html');
  assert.ok(box && list, 'there is no locked-screen ceremony container in index.html');

  // SETUP: the app really is locked. Without this the assertions below are about the ordinary
  // ready state and prove nothing — which is exactly how the claim went unexercised for a slice.
  assert.equal(overlay.hidden, false,
    'setup: the unlock overlay is not showing, so this app is not in the state this file is about');

  assert.equal(box.hidden, false,
    'the ceremonies box is hidden on a locked machine that HAS a ceremony. The criterion is that ' +
    'the panel renders with the vault locked; a user who opens Nib to a password prompt and no ' +
    'sign of the proceeding they were told about cannot tell Nib from a Nib that lost it.');

  const text = list.textContent;
  assert.match(text, /We agree to co-sign the lease/,
    'the locked panel does not name the ceremony');
  assert.match(text, /Ada Landlord/, 'the locked panel does not render the roster');
  assert.match(text, /as Director/,
    'the locked panel drops the capacity. A roster showing the name and not the capacity shows a ' +
    'different agreement from the one the signature covers — P06.S02’s finding, and it must ' +
    'survive into this surface.');

  // The next action is a control, not a sentence, and it must be THERE while locked — its route
  // is public-loopback for exactly this reason.
  const next = list.querySelector('.cernextbtn');
  assert.ok(next, 'the locked panel offers no "what happens next" control, so it renders roster ' +
    'and position but not the next action — two of the criterion’s three nouns');

  // And the password is asked for SIGNING, not for looking: the note says so, on the screen where
  // the distinction is made.
  assert.match(doc.getElementById('authCerNote').textContent, /signing/,
    'the locked screen does not say why it is showing this, so a user reads the password prompt ' +
    'as the price of looking');
});

// The lock screen shows ceremonies and offers no ACTION on them (/pending 353).
//
// **Its two routes are unlocked-safe and a delivery round is not.** `/api/ceremonies` and
// `/api/ceremony/next` sit on `requirePublicLoopback` — which is what makes this second home
// possible at all — while `POST /api/ceremony/deliver` is `requireUnlocked` and mutates. A control
// rendered here could only ever earn a 401, and it would offer an action to somebody who has not
// yet proved they may act.
//
// **The fixture above is deliberately deliverable**, so this asserts the lock and nothing else.
test('the locked panel offers no delivery control, however deliverable the ceremony', async () => {
  await settle();
  const list = doc.getElementById('authCerList');
  assert.ok(list, 'there is no #authCerList in index.html');

  // SETUP: the card is actually there. Without this the absence below is satisfied by an empty
  // box on a screen that never rendered.
  assert.ok(list.querySelector('.cercard'),
    'setup: the locked screen drew no ceremony card, so nothing below is being tested');

  assert.equal(list.querySelector('.cerdeliverbtn'), null,
    'the delivery control renders behind the lock. This card is drawn from two unlocked-safe ' +
    'routes; the round it would start is requireUnlocked and mutating, so the button can only ' +
    'produce a 401 — and P06.S07 is that the panel renders here WITHOUT offering actions.');
});
