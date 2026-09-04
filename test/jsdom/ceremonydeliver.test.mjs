// The convener's delivery round, in the product (/pending 353).
//
// **The round was reachable from nothing.** `POST /api/ceremony/deliver` walks the roster and
// returns a per-party outcome; `grep -c "ceremony/deliver" web/app.js` returned **0**, and
// `observables_test.go` exempted `deliveryOutcome.Delivered` and `.Skipped` by name against this
// item. So a convener could finish a ceremony and have no way in the product to hand anyone their
// copy, and the fields that say which party missed out reached nobody.
//
// **What this drives, and why each arm is separate.**
//
//   1. The control appears for the convener on an ENDED ceremony, and the round posts.
//   2. `skipped` renders as TWO different sentences, told apart by `delivered` — the server's own
//      doc says so. A surface that rendered `skipped` as one thing tells a convener their decliner
//      "already has it", which is the opposite of true.
//   3. A round that reached three of four says so, rather than reading as a failure. That is the
//      item's whole point and C10's case.
//   4. The control is ABSENT for a party who did not convene, absent while the proceeding is still
//      running, and absent behind the lock. Each is a different way it would be wrong: the server
//      answers a non-convener with "Nib no longer holds the invitation secret", a live proceeding
//      has nothing to deliver (D29 orders end state → delivery → close-out), and the lock screen
//      renders this same card from a route that is unlocked-safe while delivery is not.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

const ME = 'aa'.repeat(32);
const THEM = 'bb'.repeat(32);
const THIRD = 'cc'.repeat(32);
const FOURTH = 'dd'.repeat(32);

const roster = [
  { fingerprint: ME, label: 'Alice Convener', signs: true },
  { fingerprint: THEM, label: 'Bob Landlord', signs: true },
  { fingerprint: THIRD, label: 'Cy Witness', signs: true },
  { fingerprint: FOURTH, label: 'Dee Surveyor', signs: true },
];

// Four ceremonies, one per condition the control is gated on. Only the first may show it.
const listing = {
  primary: true,
  ceremonies: [
    // Mine, convened by me, and ended — the one case that earns the control.
    {
      id: '1'.repeat(32), state: 'ok', intent: 'The lease of 14 Elm Row',
      me: ME, convener: ME, ended: 'completed', roster,
    },
    // Ended, but somebody else convened it. The round mints per-party invitations from the
    // convener's own stored secret, so this machine cannot run one.
    {
      id: '2'.repeat(32), state: 'ok', intent: 'A ceremony I merely joined',
      me: ME, convener: THEM, ended: 'completed', roster,
    },
    // Mine, but the proceeding has not ended — `ended` absent. Empty means UNKNOWN, so this is
    // the arm that catches a surface testing `ended !== 'running'` instead of testing for a value.
    {
      id: '3'.repeat(32), state: 'ok', intent: 'A ceremony still running',
      me: ME, convener: ME, roster,
    },
    // Ended, and Nib does not know which party this machine is. A convener is known, so this
    // catches a surface that treats an absent position as "probably me".
    {
      id: '4'.repeat(32), state: 'ok', intent: 'A ceremony with no position recorded',
      convener: ME, ended: 'completed', roster,
    },
    // **Ended, and NEITHER marker is known — the `'' === ''` case, and it needs its own row.**
    // A first version of this file asserted that bug against ceremony 4 above, and a mutation
    // proved the claim empty: with `convener` known, `'' === convener` is false and the button
    // stays hidden for the wrong reason. Two unknowns comparing equal is the only shape that
    // reaches it, and a record whose `ConvenerCert` will not parse produces exactly that beside a
    // mirror with no position marker.
    {
      id: '5'.repeat(32), state: 'ok', intent: 'A ceremony that knows neither party',
      ended: 'completed', roster,
    },
  ],
  ended: [],
};

// Three of four: Bob is reached, Cy already had it, Dee ended the proceeding, and the fourth leg
// fails. Every branch of the outcome shape in one answer.
let delivered = null;
const round = {
  ceremony: '1'.repeat(32),
  parties: [
    { fingerprint: THEM, label: 'Bob Landlord', delivered: true },
    { fingerprint: THIRD, label: 'Cy Witness', delivered: true, skipped: true },
    {
      fingerprint: FOURTH, label: 'Dee Surveyor', skipped: true,
      reason: 'this party ended the proceeding, so Nib did not try to reach them',
    },
    { fingerprint: 'ee'.repeat(32), label: 'Eve Notary', reason: 'tried 2 addresses, none answered' },
  ],
};

const { document: doc, settle } = await boot({
  routes: {
    '/api/ceremonies': () => listing,
    '/api/ceremony/deliver': (opts) => {
      delivered = JSON.parse((opts && opts.body) || '{}');
      return round;
    },
  },
});

async function showPanel() {
  doc.querySelector('.modetab[data-tab="collaborate"]')?.click();
  await settle();
  return doc.getElementById('ceremonyList');
}

function cardFor(host, id) {
  return host.querySelector(`.cercard[data-ceremony="${id}"]`);
}

test('the convener gets a delivery control, and only on the ceremony that earns one', async () => {
  const host = await showPanel();
  assert.ok(host, 'the ceremony panel has no host element');

  // SETUP: all four cards rendered. Without this every absence below is satisfied by a panel
  // that drew nothing at all, which is the vacuous green this whole assertion set turns on.
  const cards = host.querySelectorAll('.cercard');
  assert.equal(cards.length, 5,
    `the panel drew ${cards.length} cards for 5 ceremonies — nothing below is being tested`);

  assert.ok(cardFor(host, '1'.repeat(32)).querySelector('.cerdeliverbtn'),
    'the convener of an ENDED ceremony has no way to send anyone their copy. That is the whole ' +
    'of /pending 353: POST /api/ceremony/deliver walks the roster and reports every party, and ' +
    'nothing in the product ever called it.');

  assert.equal(cardFor(host, '2'.repeat(32)).querySelector('.cerdeliverbtn'), null,
    'a party who did not convene is offered the delivery round. The round mints each invitation ' +
    "from the convener's own stored secret, so the server can only answer \"Nib no longer holds " +
    'the invitation secret" — true, useless, and identical to a ceremony whose secrets were ' +
    'cleaned up.');

  assert.equal(cardFor(host, '3'.repeat(32)).querySelector('.cerdeliverbtn'), null,
    'a ceremony that has NOT ended offers a delivery round. D29 orders the lifecycle end state → ' +
    'delivery round → close-out, so there is nothing to deliver yet; an absent `ended` is ' +
    'UNKNOWN and must not read as "still fine to send".');

  assert.equal(cardFor(host, '4'.repeat(32)).querySelector('.cerdeliverbtn'), null,
    'a ceremony whose `me` is unknown offers the delivery round. An absent position is UNKNOWN, ' +
    'never "probably you", and this machine may not start a round it cannot show it owns.');

  assert.equal(cardFor(host, '5'.repeat(32)).querySelector('.cerdeliverbtn'), null,
    'a ceremony that knows NEITHER its position nor its convener offers the delivery round. Both ' +
    "fields are unknown-when-empty by their own doctrine, so `'' === ''` must not read as a " +
    'match — this is the arm a mutation proved the previous fixture could not reach.');
});

test('a round that reached three of four says so, and skipped means two different things', async () => {
  const host = await showPanel();
  const card = cardFor(host, '1'.repeat(32));
  card.querySelector('.cerdeliverbtn').click();
  await settle();

  // SETUP: the round was actually posted, and for THIS ceremony. Without it the rendering
  // assertions below could pass against a surface that never called the route.
  assert.ok(delivered, 'the delivery control did not post to /api/ceremony/deliver');
  assert.equal(delivered.ceremony, '1'.repeat(32),
    `the round was posted for ${delivered.ceremony}, not the ceremony whose card was clicked`);

  const text = card.textContent;

  assert.match(text, /Bob Landlord/, 'the delivered party is not listed');
  assert.match(text, /already had it/,
    'a party the round SKIPPED because they had already acknowledged is not reported as such. ' +
    '`skipped` with `delivered` true is a re-run correctly not repeating itself, and a surface ' +
    'that hides it makes a successful re-run look like it did nothing.');
  assert.match(text, /ended this proceeding/,
    'the party that ENDED the proceeding is reported as though they simply have the document. ' +
    "`skipped` carries two meanings told apart by `delivered` — the server's own doc says so — " +
    'and folding them tells a convener their decliner already has it.');
  assert.match(text, /could not be reached/,
    'the leg that failed is not reported. That party is the one the convener has to act on.');
  assert.match(text, /tried 2 addresses/,
    "the failed leg's own reason is dropped, so the convener is told something went wrong and " +
    'not what.');

  // The item's headline: a partial round is a partial round, not a failure.
  assert.match(text, /3 of 4 have their copy/,
    'a round that reached three of four parties does not say so. The route has always reported ' +
    'every party precisely so the user is not told "delivery failed" about a round that mostly ' +
    'worked — that sentence is what this item was filed about.');
  assert.doesNotMatch(text, /Everyone has their copy/,
    'a round with a failed leg claims everyone has their copy');
});

// The lock-screen arm lives in `lockedpanel.test.mjs`, which boots with `/api/status` answering
// `key-locked` — one boot per file, and that is the file whose boot renders this card behind the
// lock. Named here so the condition is not assumed to be untested.
