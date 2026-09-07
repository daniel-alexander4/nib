// P05.S01 — leaving a ceremony (D17), at the surface.
//
// **What only this tier can see.** The Go tests prove the route stops the arm and keeps it
// stopped. They cannot see whether the control is OFFERED to the right ceremonies, whether the
// confirmation is asked at all, or what a `left` receipt reads as afterwards — and the last of
// those fell to the "Ended in a way this version does not recognise" fallback until it was
// noticed, which is a sentence about a damaged file shown to a user for the thing they just did.
//
// **The confirmation is the case worth writing carefully.** A harness that answers dialogs
// automatically turns a declined confirmation into an accepted one silently — measured on this
// repo before (`/pending 333`'s overwrite test was accepting the dialog it meant to decline). So
// `window.confirm` is stubbed per case and the DECLINE case asserts no request was made at all.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

const ID = 'a'.repeat(32);
let listing = {};
let leaveCalls = 0;

const h = await boot({
  routes: {
    '/api/ceremonies': () => listing,
    '/api/ceremony/leave': () => { leaveCalls += 1; return { ceremony: ID, state: 'left' }; },
    '/api/ceremony/next': () => ({ ceremony: ID, state: 'unavailable', reason: 'no record' }),
  },
});
const { document: doc, settle, window: win } = h;

// `confirm` is set on BOTH the jsdom window and the module global: boot.mjs copies only a named
// list of globals across, and app.js calls it bare (as `confirmSignatureLoss` already does), so
// which one it resolves to is a fact about the harness rather than about this test.
function answerConfirm(fn) { win.confirm = fn; globalThis.confirm = fn; }

const preHop = { id: ID, state: 'absent', me: 'b'.repeat(64), reason: 'nothing has arrived yet' };
const signed = { id: ID, state: 'ok', me: 'b'.repeat(64), convener: 'c'.repeat(64), intent: 'x' };

// `setMode` has no early return — it calls `syncSidebarForMode` every time — so clicking the tab
// is a fresh fetch and render, which is what lets one boot drive five listings.
async function render(data) {
  listing = data;
  leaveCalls = 0;
  doc.querySelector('.modetab[data-tab="collaborate"]').click();
  await settle();
}

test('a party who has not signed is offered a way out', async () => {
  await render({ ceremonies: [preHop], primary: true });
  const btn = doc.querySelector('.cerleavebtn');
  assert.ok(btn, 'a ceremony this machine accepted and has not signed offers no way to leave — '
    + 'since D14 the arm is raised by a sweep and renewed at every unlock, so without this the '
    + 'only lever is quitting Nib, and quitting is a pause rather than a decision');
});

test('a party who has already signed is not offered it', async () => {
  await render({ ceremonies: [signed], primary: true });
  assert.equal(doc.querySelector('.cerleavebtn'), null,
    'the control is offered on a ceremony this machine has signed, where the server refuses it — '
    + 'an offer the app knows it cannot keep. Leaving there withdraws nothing and only stops this '
    + "party's own finished copy arriving");
});

test('declining the confirmation sends nothing at all', async () => {
  await render({ ceremonies: [preHop], primary: true });
  answerConfirm(() => false);
  doc.querySelector('.cerleavebtn').click();
  await settle();
  // The assertion is on the REQUEST, not on the DOM: a harness that answers dialogs on the user's
  // behalf makes "the card is still there" true whether or not anything was sent.
  assert.equal(leaveCalls, 0,
    'saying no to the confirmation still left the ceremony — leaving forgets the invitation and '
    + 'cannot be undone without a fresh one');
});

test('accepting the confirmation leaves, and asks first', async () => {
  await render({ ceremonies: [preHop], primary: true });
  let asked = '';
  answerConfirm((m) => { asked = String(m || ''); return true; });
  doc.querySelector('.cerleavebtn').click();
  await settle();
  assert.equal(leaveCalls, 1, 'accepting the confirmation did not leave the ceremony');
  assert.match(asked, /declin/i,
    'the confirmation does not mention declining. Leaving and declining are one keystroke apart '
    + 'in a user\'s head and completely different in the record — a decline is attested and the '
    + 'convener acts on it, and this reaches nobody');
});

test('a ceremony this machine left reads as its own act, not as damage', async () => {
  await render({ ceremonies: [], ended: [{ ceremony: ID, state: 'left', observed_at: new Date().toISOString() }], primary: true });
  const text = doc.getElementById('ceremonyList').textContent;
  assert.match(text, /You left/,
    'a ceremony this machine left does not say so');
  assert.doesNotMatch(text, /does not recognise/,
    'a ceremony this machine left reads as "Ended in a way this version does not recognise" — a '
    + 'sentence about a damaged file, shown for the thing the user did on purpose a moment ago');
});
