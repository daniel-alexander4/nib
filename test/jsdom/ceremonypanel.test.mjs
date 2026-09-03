// The Signing Ceremony panel renders the roster and this machine's position (P06.S02).
//
// ── What this is written against ─────────────────────────────────────────────
// The client had ZERO ceremony readers until this slice: `grep -n "api/ceremon" web/app.js`
// returned nothing, and `ceremoniesResponse` sat in this suite's EXCLUDED map with the words
// "P08.S03 ships the listing before P06 builds its panel; no client reader yet". A route that
// answers correctly and a surface that renders none of it is the exact shape tier 6 found at
// P07.S05a — the server was right and no user could reach it.
//
// ── Why the degraded case is half the test ───────────────────────────────────
// C12 says a corrupt record degrades THAT ceremony's entry and leaves every other ceremony
// working. A panel that dropped the bad row would satisfy "the healthy ones render" and fail the
// criterion — and a ceremony Nib will not admit exists is one whose only remedy is finding and
// deleting the folder by hand, which is where the user already is. So both are asserted from one
// response, and the count is asserted as well as the contents.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

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
    {
      id: '2'.repeat(32),
      state: 'unparseable',
      reason: 'this ceremony is damaged and Nib cannot read it',
    },
  ],
  ended: [{ ceremony: '3'.repeat(32), state: 'declined', observed_at: '2026-09-01T09:00:00Z' }],
};

const { document: doc, settle } = await boot({
  routes: { '/api/ceremonies': () => listing },
});

async function showPanel() {
  doc.querySelector('.modetab[data-tab="collaborate"]')?.click();
  await settle();
  return doc.getElementById('ceremonyList');
}

test('the panel renders both ceremonies, degrading only the damaged one', async () => {
  const host = await showPanel();
  assert.ok(host, 'the ceremony panel has no host element');
  const cards = host.querySelectorAll('.cercard');
  // The COUNT first: a panel that renders the healthy one and drops the damaged one passes every
  // content assertion below.
  assert.equal(cards.length, 2,
    `the panel drew ${cards.length} cards for 2 ceremonies — a ceremony Nib will not admit ` +
    'exists is one whose only remedy is finding and deleting the folder by hand');

  const text = host.textContent;
  assert.match(text, /We agree to the lease of 14 Elm Row/,
    'the recital is not rendered, and it is the only thing here worth reading first');
  assert.match(text, /this ceremony is damaged/,
    "the damaged ceremony's own sentence is missing — the class is a word, the sentence is what " +
    'tells the user whether this is damage, a forgery, or a Nib that is out of date');
});

test('it names this machine as the party the record says it is, and shows no hex', async () => {
  const host = await showPanel();
  assert.match(host.textContent, /You are party 2 of 2\./,
    'the panel does not say which party this machine is. That is the ONE fact a vault-less ' +
    'reader cannot work out for itself, and the server records it at convene and accept time ' +
    'precisely so this line can exist');
  const mine = host.querySelectorAll('.cerme');
  assert.equal(mine.length, 1, `${mine.length} roster rows are marked as this machine, want 1`);
  assert.match(mine[0].textContent, /Alice Tenant/, 'the wrong roster row is marked');
  assert.match(host.textContent, /as attorney-in-fact/,
    'the capacity is dropped. A roster that renders the name and not the capacity shows a ' +
    'different agreement from the one the signature covers');

  // **No hex fingerprint anywhere**, which is one of this phase's exit criteria — and asserted
  // against the actual values in the fixture rather than a shape, because a partial hex is still
  // a hex a user is being asked to read.
  assert.doesNotMatch(host.textContent, new RegExp(ME.slice(0, 16)),
    'this machine\'s fingerprint appears in the panel');
  assert.doesNotMatch(host.textContent, new RegExp(THEM.slice(0, 16)),
    "the other party's fingerprint appears in the panel");
});

// **The key is DELETED, not blanked, and that is the wire shape.** `Stored.Me` is tagged
// `json:"me,omitempty"`, so a ceremony with no recorded position arrives with the field ABSENT —
// the client sees `undefined`, never `''`. The first cut of this test set it to the empty string,
// which is a value the server cannot send, and a mutation removing the `typeof me === 'string'`
// guard came back GREEN against it: `'bbbb…' === ''` is false either way, so the fixture could not
// tell a guarded comparison from an unguarded one. Against the real shape, an unguarded
// `me.toLowerCase()` throws.
test('an unknown position says so rather than marking nobody silently', async () => {
  const saved = listing.ceremonies[0].me;
  delete listing.ceremonies[0].me;
  try {
    const host = await showPanel();
    assert.equal(host.querySelectorAll('.cerme').length, 0,
      'a row is marked as this machine when the server recorded no position');
    assert.match(host.textContent, /cannot tell which of these parties you are/,
      'with no position recorded the panel says nothing at all about it. Empty means UNKNOWN, ' +
      'never "you are not a party" — a ceremony mirrored before the marker shipped has none, ' +
      'and its user is still very much a party');
    // The rest of the ceremony is untouched: one missing label must not cost the entry.
    assert.match(host.textContent, /We agree to the lease of 14 Elm Row/,
      'an unknown position degraded the whole entry');
  } finally {
    listing.ceremonies[0].me = saved;
  }
});

test('the finished ceremonies are listed with what happened and when', async () => {
  const host = await showPanel();
  assert.match(host.textContent, /Declined/,
    'the ended list is not rendered. The close-out MOVES a ceremony rather than deleting it ' +
    '(ADR-012) precisely so the signed contribution survives — and a user who is never shown ' +
    'where it went has it preserved in secret');
  assert.match(host.textContent, /ended/,
    'nothing tells the user where their preserved copy is');
});
