// Naming a proceeding on this machine, at the surface.
//
// **What only this tier can see.** The Go tests prove the route stores, caps and clears a name,
// and that the listing carries it. They cannot see which of the two strings ends up as the card's
// heading, whether the recital survives being demoted, or whether the field a user types into is
// refilled from the card rather than from whatever they abandoned last time.
//
// **The recital assertion is the one worth writing carefully.** D20 makes the Intent the recital's
// only home, so a name that REPLACED it would put this panel in the business of paraphrasing what
// the parties agreed — and the failure is silent, because a card with a good name on it looks
// entirely correct. So both strings are asserted present, on the same card, in that order.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

const ID = '7'.repeat(32);
const INTENT = 'We agree to the lease of 14 Elm Row, Edinburgh, for a term of five years';

let listing = {};
let named = null;

const h = await boot({
  routes: {
    '/api/ceremonies': () => listing,
    // `opts`, not a parsed body: the harness hands the route the fetch options, so what is
    // recorded here is what the client actually SENT.
    '/api/ceremony/name': (opts) => {
      named = JSON.parse((opts && opts.body) || '{}');
      return { ceremony: ID, name: named.name || '' };
    },
    '/api/ceremony/next': () => ({ ceremony: ID, state: 'unavailable', reason: 'no record' }),
  },
});
const { document: doc, settle } = h;

const card = (extra) => Object.assign({
  id: ID, state: 'absent', me: 'b'.repeat(64), intent: INTENT,
  reason: 'nothing has arrived yet',
}, extra || {});

async function render(data) {
  listing = data;
  named = null;
  doc.querySelector('.modetab[data-tab="collaborate"]').click();
  await settle();
}

test('an unnamed ceremony is still headed by its recital, and says it once', async () => {
  await render({ ceremonies: [card()], primary: true });
  assert.equal(doc.querySelector('.cerintent').textContent, INTENT,
    'an unnamed ceremony lost its heading — the recital is what it had, and the name is an '
    + 'addition to the card rather than a replacement for it');
  assert.equal(doc.querySelector('.cerrecital'), null,
    'the recital is printed twice on an unnamed ceremony: once as the heading and once as the '
    + 'line under it');
});

test('a named ceremony is headed by the name AND still carries the recital', async () => {
  await render({ ceremonies: [card({ name: 'The Elm Row lease' })], primary: true });
  assert.equal(doc.querySelector('.cerintent').textContent, 'The Elm Row lease',
    'the card is headed by the recital even though this machine gave it a name — which is the '
    + 'whole reason the name exists: a clause meant to be signed is not a handle to scan a list by');
  const recital = doc.querySelector('.cerrecital');
  assert.ok(recital, 'naming the ceremony HID the recital. D20 makes the Intent the recital\'s '
    + 'only home, so a name that replaces it leaves the panel paraphrasing what the parties agreed');
  assert.equal(recital.textContent, INTENT, 'the recital line does not hold the Intent');
});

test('the field opens holding the name the card carries, and Save posts it', async () => {
  await render({ ceremonies: [card({ name: 'The Elm Row lease' })], primary: true });
  const open = doc.querySelector('.cernamebtn');
  assert.match(open.textContent, /rename/i,
    'a ceremony that already has a name offers "Name this ceremony…", which reads as though the '
    + 'name were gone');
  const body = doc.querySelector('.cernamebody');
  assert.equal(body.hidden, true, 'every card in the list opens with a text field showing');
  open.click();
  await settle();
  const field = doc.querySelector('.cernameinput');
  assert.equal(body.hidden, false, 'pressing the button did not open the field');
  assert.equal(field.value, 'The Elm Row lease',
    `the field opened holding ${JSON.stringify(field.value)} — a rename that starts empty reads `
    + 'as though the name had already been cleared, and a Save on it would clear it');

  field.value = 'Elm Row';
  doc.querySelector('.cernamesave').click();
  await settle();
  assert.ok(named, 'Save posted nothing');
  assert.equal(named.ceremony, ID, `the request named ceremony ${JSON.stringify(named.ceremony)}`);
  assert.equal(named.name, 'Elm Row', `the request carried ${JSON.stringify(named.name)}`);
});

test('an emptied field is SENT, because clearing has no second door', async () => {
  await render({ ceremonies: [card({ name: 'The Elm Row lease' })], primary: true });
  doc.querySelector('.cernamebtn').click();
  await settle();
  const field = doc.querySelector('.cernameinput');
  field.value = '';
  doc.querySelector('.cernamesave').click();
  await settle();
  // The assertion is that a request was made AT ALL with an empty name: a client that skipped the
  // post on an empty field would leave the user with no way back to the agreement, and the card
  // would still be showing the name they just deleted.
  assert.ok(named, 'clearing the field and pressing Save sent nothing, so a name cannot be removed '
    + 'from the product at all — the server treats empty as "forget it" and nothing reaches it');
  assert.equal(named.name, '', `the cleared request carried ${JSON.stringify(named.name)}`);
});

test('the name says plainly that it never leaves this machine', async () => {
  await render({ ceremonies: [card()], primary: true });
  doc.querySelector('.cernamebtn').click();
  await settle();
  const note = doc.querySelector('.cernamenote').textContent;
  assert.match(note, /this machine/i,
    `the box says ${JSON.stringify(note)}. The name is local, unsigned and never on the wire — a `
    + 'user who believes the other parties see their label would write a different label');
  assert.match(note, /signs?\b/i, 'the box does not say the name is outside what anyone signs');
});

test('a non-primary Nib is not offered the control', async () => {
  await render({ ceremonies: [card()], primary: false });
  assert.equal(doc.querySelector('.cernamebtn'), null,
    'a non-primary Nib offers a control that writes to ~/nib — `mayAct` folds that together with '
    + 'the lock screen, and both are states where the user is looking rather than acting');
});

test('a finished ceremony is still identifiable by its name', async () => {
  await render({
    ceremonies: [],
    ended: [{ ceremony: ID, state: 'completed', observed_at: '2026-09-01T10:00:00Z', name: 'The Elm Row lease' }],
    primary: true,
  });
  const row = doc.querySelector('.cerended-row');
  assert.ok(row, 'the finished list rendered nothing');
  assert.equal(row.querySelector('.cerendedname')?.textContent, 'The Elm Row lease',
    `the finished row reads ${JSON.stringify(row.textContent)} — a state and a date, which is `
    + 'five identical lines to anyone who has completed five proceedings. The name travelled with '
    + 'the folder ADR-012 moves and is the only text on the row a person recognises');
});

test('a finished ceremony that was never named renders no empty slot', async () => {
  await render({
    ceremonies: [],
    ended: [{ ceremony: ID, state: 'completed', observed_at: '2026-09-01T10:00:00Z' }],
    primary: true,
  });
  assert.equal(doc.querySelector('.cerendedname'), null,
    'an unnamed finished ceremony renders an empty name element, which pushes the date out of '
    + 'line with every other row in the column');
});
