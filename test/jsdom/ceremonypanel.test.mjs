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

// The server's answer, stubbed per test. The DISAGREEMENT with the roster above is deliberate:
// `/api/ceremonies` lists Bob first, and this says it is Alice's turn.
const defaultNext = {
  ceremony: '1'.repeat(32), state: 'waiting',
  label: 'Alice Tenant', capacity: 'as attorney-in-fact',
  position: 1, of: 1, isMe: true, meKnown: true,
};
let nextAnswer = defaultNext;

const { document: doc, settle } = await boot({
  routes: {
    '/api/ceremonies': () => listing,
    '/api/ceremony/next': () => nextAnswer,
    '/api/peers': () => peers,
    '/api/ceremony/convene': (opts) => { convenePosted = JSON.parse((opts && opts.body) || '{}'); return convened; },
    '/api/ceremony/accept': () => ({
      ceremony: '4'.repeat(32), pinned: 1, signing: 2,
      roster: [
        { fingerprint: PEER_FP, label: 'Bob Landlord', name: 'oak river amber quiet stone gate', signs: true, convener: true },
        { fingerprint: 'ee'.repeat(32), label: 'Alice Tenant', capacity: 'as attorney-in-fact', signs: true, self: true },
      ],
    }),
  },
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

// ── P06.S03: the next action comes from the server, and the panel never computes it ──────────
//
// The criterion is that the panel's enabled action is *"computed from the record by the same
// function the server's L3 check uses"*. Tier 1 proves the route answers from `p2p.NextContributor`
// and that its answer agrees with the gate that refuses; this file's job is that the panel RENDERS
// the server's answer and derives nothing of its own.
//
// **The fixture disagrees with the roster on purpose.** `/api/ceremonies` says party 1 is Bob and
// party 2 is Alice; `/api/ceremony/next` says it is Alice's turn. A panel computing "the first
// party who has not signed" would say Bob, which is what a JS reimplementation produces and what
// this asserts against.
test('the next action is the server\'s answer, not the roster order', async () => {
  const host = await showPanel();
  const btn = host.querySelector('.cernextbtn');
  assert.ok(btn, 'a healthy ceremony offers no way to ask what happens next');
  btn.click();
  await settle();
  const line = host.querySelector('.cernextline');
  assert.ok(line, 'nothing was rendered for the next action');
  assert.match(line.textContent, /your turn to sign/i,
    `the panel rendered ${JSON.stringify(line.textContent)}. The server said it is this machine's ` +
    'turn; a panel that answered from the roster would name the first party instead');
  assert.doesNotMatch(line.textContent, /Bob Landlord/,
    'the panel named the roster\'s first party — which is what computing the answer here, ' +
    'rather than rendering the server\'s, produces');
});

test('a machine that does not know its position is not told it is somebody else\'s turn', async () => {
  nextAnswer = { ceremony: '1'.repeat(32), state: 'waiting', label: 'Bob Landlord', position: 1, of: 2, isMe: false, meKnown: false };
  try {
    const host = await showPanel();
    host.querySelector('.cernextbtn').click();
    await settle();
    const line = host.querySelector('.cernextline');
    assert.match(line.textContent, /cannot tell whether that is you/,
      '`meKnown: false` was rendered as somebody else\'s turn. A machine that never recorded ' +
      'which party it is has not been told it is not their turn — that is an answer, and the ' +
      'honest one is that Nib does not know');
  } finally {
    nextAnswer = defaultNext;
  }
});

test('a finished ceremony says so rather than reporting a failure', async () => {
  nextAnswer = { ceremony: '1'.repeat(32), state: 'complete' };
  try {
    const host = await showPanel();
    host.querySelector('.cernextbtn').click();
    await settle();
    assert.match(host.querySelector('.cernextline').textContent, /Everyone has signed/,
      'a complete ceremony is folded into the failure sentence, which tells a user whose ' +
      'ceremony finished that their document could not be read');
  } finally {
    nextAnswer = defaultNext;
  }
});

// ── P06.S04: convene and accept, with no hex anywhere on the primary flow ────────────────────
//
// **The no-hex criterion is what shapes this whole surface.** A convener must name each party and a
// fingerprint is hex, so the roster is PICKED from the peers this machine has already pinned rather
// than typed. The fingerprint still has to reach the server — it just never reaches the screen.
const PEER_FP = 'cc'.repeat(32);
const peers = { fingerprint: 'dd'.repeat(32), name: 'my six word name here now', peers: [
  { fingerprint: PEER_FP, name: 'oak river amber quiet stone gate', label: 'Bob Landlord' },
] };
let convened = {
  ceremony: '4'.repeat(32), intent: 'We agree', expires: '2026-10-01T12:00:00Z',
  invites: [{ fingerprint: PEER_FP, label: 'Bob Landlord', name: 'oak river amber quiet stone gate',
    signs: true, invitation: 'nib-invite-v1:AAAA' }],
  warnings: [],
};
let convenePosted = null;

test('the convene form picks a roster from pinned peers and shows no hex', async () => {
  await showPanel();
  doc.getElementById('ceremonyConveneBtn').click();
  await settle();
  const pick = doc.getElementById('cerPeerPick');
  assert.ok(pick, 'there is no peer picker');
  assert.match(pick.textContent, /Bob Landlord/, 'the pinned peer is not offered');
  assert.doesNotMatch(pick.textContent, new RegExp(PEER_FP.slice(0, 16)),
    'the picker renders a hex fingerprint. The criterion is that the primary flow contains none — ' +
    'the fingerprint travels in a data attribute so it reaches the server without reaching the user');
  // And it is CARRIED, not dropped: a picker that showed no hex by simply losing it would convene
  // a roster of nobody.
  const box = pick.querySelector('.cerpeerbox');
  assert.equal(box.dataset.fingerprint, PEER_FP,
    'the picker does not carry the fingerprint at all, so nothing could be convened');
});

test('the invitations screen says what an invitation is, in D21\'s terms', async () => {
  await showPanel();
  doc.getElementById('ceremonyConveneBtn').click();
  await settle();
  doc.querySelector('.cerpeerbox').checked = true;
  doc.getElementById('cerIntent').value = 'We agree to the lease';
  doc.getElementById('cerExpires').value = '2026-10-01T12:00';
  doc.getElementById('ceremonyConveneForm').dispatchEvent(new doc.defaultView.Event('submit', { cancelable: true }));
  await settle();

  const out = doc.getElementById('ceremonyResult');
  assert.match(out.textContent, /channel secret/,
    'the screen does not say the invitation is a channel secret. D21 says P06 "says so on screen", ' +
    'and the criterion asks for those terms');
  assert.match(out.textContent, /not a signing credential/,
    'the screen does not say what an invitation is NOT, which is the half a user who forwards one needs');
  // **Read from the field, not from the container's text.** The invitation lives in a read-only
  // `<textarea>` so it can be selected and copied, and a textarea's `value` is not part of its
  // parent's `textContent` — asserting on the container would fail against a screen that is
  // working. It is also why the no-hex assertion below is still meaningful: nothing about the
  // invitation is in the page's text flow.
  const field = out.querySelector('.cerinvitetext');
  assert.ok(field, 'there is no field to copy the invitation from');
  assert.match(field.value, /nib-invite-v1:AAAA/, 'the invitation itself is not shown');
  assert.doesNotMatch(out.textContent, new RegExp(PEER_FP.slice(0, 16)),
    'the invitations screen renders a hex fingerprint — the payload carries one per invite and it ' +
    'does not belong on screen');
  // The request actually sent: the roster carries the fingerprint and the deadline is absolute.
  assert.ok(convenePosted, 'no convene request was sent');
  assert.equal(convenePosted.roster[0].fingerprint, PEER_FP);
  assert.match(convenePosted.expires, /Z$/,
    'the deadline was sent as local wall time. `datetime-local` has no zone, so sending it as ' +
    'typed is off by the user\'s offset — a ceremony that closes at the wrong hour');
});

test('a warning is bound to the control that caused it, by its code', async () => {
  const saved = convened;
  convened = { ...convened, warnings: [{ code: 'sitting-ceiling', text: 'This ceremony has 9 parties.' }] };
  try {
    await showPanel();
    doc.getElementById('ceremonyConveneBtn').click();
    await settle();
    doc.querySelector('.cerpeerbox').checked = true;
    doc.getElementById('cerExpires').value = '2026-10-01T12:00';
    doc.getElementById('ceremonyConveneForm').dispatchEvent(new doc.defaultView.Event('submit', { cancelable: true }));
    await settle();
    const warn = doc.querySelector('.cerwarn[data-warn="sitting-ceiling"]');
    assert.ok(warn, 'the warning was not bound to a control by its code. `Warnings` are ' +
      '"machine-tagged so a panel can bind one to the control that caused it rather than ' +
      're-parsing English" — printing the text alone throws the tag away');
    assert.match(warn.textContent, /9 parties/, 'the warning text is not rendered');
  } finally {
    convened = saved;
  }
});

test('accepting an invitation shows the roster with you and the convener marked', async () => {
  await showPanel();
  doc.getElementById('ceremonyAcceptBtn').click();
  await settle();
  doc.getElementById('cerInviteText').value = 'nib-invite-v1:AAAA';
  doc.getElementById('ceremonyAcceptForm').dispatchEvent(new doc.defaultView.Event('submit', { cancelable: true }));
  await settle();
  const out = doc.getElementById('ceremonyResult');
  assert.match(out.textContent, /Alice Tenant/, 'the roster is not shown');
  assert.match(out.textContent, /convened this/,
    'the convener is not marked. The server marks it — "the two entries a reader needs to find ' +
    'without re-deriving them" — and a client working it out from fingerprints would be a second ' +
    'derivation in the one place hex is forbidden');
  assert.equal(out.querySelectorAll('.cerme').length, 1, 'the invitee is not marked as themselves');
  assert.doesNotMatch(out.textContent, new RegExp(PEER_FP.slice(0, 16)),
    'the accepted roster renders a hex fingerprint');
});
