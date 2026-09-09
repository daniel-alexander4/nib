// P04.S04 — D12 stated before the first hop, at both doors.
//
// # What this slice is actually about
//
// D12's own words are *"the remedy is to abandon and re-convene, losing every signature
// collected"* — and there is no abandon. The plan review traced it: no route, `unconvene` is the
// rollback verb with one caller inside the convene failure path, `endCeremony` fires only on a
// counterparty's decline, `SignTermination` refuses any end state but `declined` and `completed`,
// and `StateAbandoned` is derived, local, and up to three days past a deadline that may be a month
// out. Leaving is local too and sends nothing.
//
// So the sentence this ships is NOT D12's. It is what is true of this code, and these tests are
// about that difference: a statement naming an action the product does not offer would be a false
// expectation on the one surface built to prevent one.
//
// # Why the statement is DRIVEN rather than read at boot
//
// The first version filled both paragraphs once, at load. The phase-close review measured what that
// produced: `#srvPermanence` rendered on the RECEIVE flow — where `#srvIntentRow` is already hidden
// because "a plain transfer needs no agreement statement", nothing being signed there at all — and
// `#sinPermanence` sits in the generic "Co-sign live with a peer" dialog, so every plain two-party
// co-sign was told the document "has to be run again as a new ceremony". S04's own acceptance clause
// is that the statement is TRUE of the shipped code; on those two paths it was not. It is written
// when each surface opens now, with the ceremony half only where there is a ceremony.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const PEER = { fingerprint: 'a'.repeat(64), label: 'Ada' };
const RECITAL = 'We agree to the terms of the Fitzroy Street lease.';

let armed = false;
let withRecital = true;
// The open document, and whether it is IN a ceremony — which is what the initiating door branches
// on, the same way the consent screen branches on the recital's presence.
let openResp = {
  id: 'test-epoch:1', name: 'deed.pdf', path: '/tmp/nib-harness/deed.pdf',
  canSave: false, canUndo: false, canRedo: false,
  signature: { state: 'unsigned', signers: [] },
};
const { document: doc, settle } = await boot({
  routes: {
    '/api/open': () => openResp,
    '/api/attestations': () => ({ attestations: [], obliged: 2, signed: 0 }),
    '/api/peers': () => ({ self: 'f'.repeat(64), peers: [PEER] }),
    '/api/session/arm': () => { armed = true; return { armed: true, address: '127.0.0.1:8443' }; },
    '/api/session/status': () => (armed
      ? {
        armed: true,
        address: '127.0.0.1:8443',
        pending: {
          signer: 'Ada Landlord', fingerprint: 'a'.repeat(64),
          reason: 'I agree to co-sign the lease', signers: [],
          ...(withRecital ? { recital: RECITAL } : {}),
        },
      }
      : { armed: false }),
    '/api/session/disarm': () => { armed = false; return {}; },
  },
});

// The consent screen arrives on a POLL rather than on the arm response — the same 1800ms against a
// 1500ms interval that `consentrecital.test.mjs` measured.
async function reachConsent() {
  setNextDocument({ numPages: 1 });
  doc.getElementById('sessionRecvBtn').click();
  await settle();
  doc.getElementById('srvPeer').selectedIndex = 0;
  doc.getElementById('srvArmGo').click();
  await settle();
  await new Promise((r) => setTimeout(r, 1800));
  await settle();
}
async function cleanUp() {
  doc.getElementById('srvCancel').click();
  await settle();
}

const sin = () => doc.getElementById('sinPermanence');
const srv = () => doc.getElementById('srvPermanence');

test('the consent screen states it, and inside a ceremony it names the ceremony', async () => {
  withRecital = true;
  await reachConsent();
  const onConsent = !doc.getElementById('srvConsent').hidden;
  const text = srv().textContent;
  const hidden = srv().hidden;
  await cleanUp();

  // SETUP: the screen was actually reached, or the paragraph below is one on a screen that never
  // opened, holding whatever it was last set to.
  assert.ok(onConsent, 'the consent screen was never reached, so the statement below proves nothing');
  assert.equal(hidden, false, 'the statement is hidden on the flow that signs');
  assert.match(text, /cannot remove a signature/i,
    'the statement does not say a signature cannot be removed, which is D12\'s first half');
  assert.match(text, /cancel a ceremony/i,
    'inside a ceremony the statement does not say Nib cannot cancel one. That is the half D12 got '
    + 'wrong — it names "abandon and re-convene" as the remedy, and there is no abandon route');
  assert.match(text, /does not tell the other parties/i,
    'the statement does not say the other parties are not told. Without it a convener reads "run it '
    + 'again" as something the software communicates — the first ceremony stays live in every other '
    + 'party\'s rail until its deadline');
  assert.doesNotMatch(text, /\babandon/i,
    'the statement offers to "abandon" a ceremony. There is no abandon route, so naming it would be '
    + 'a false expectation on the surface built to prevent one');
});

test('OUTSIDE a ceremony it does not claim there is one', async () => {
  withRecital = false;
  await reachConsent();
  const text = srv().textContent;
  await cleanUp();

  // The floor: it still says the thing that IS true on a plain co-sign.
  assert.match(text, /cannot remove a signature/i,
    'outside a ceremony the statement says nothing at all. A signature is still permanent on a plain '
    + 'two-party co-sign, which is exactly what this surface is doing');
  assert.doesNotMatch(text, /ceremony/i,
    `outside a ceremony the statement says ${JSON.stringify(text)}. There is no ceremony on this `
    + 'flow — the branch is on the recital\'s presence, the same fact the box beside it branches on '
    + '— so telling the user to "run the document again as a new ceremony" names something that '
    + 'does not exist here');
});

test('the receive flow, which signs nothing, is told nothing', async () => {
  setNextDocument({ numPages: 1 });
  doc.getElementById('sessionRecvDocBtn').click();
  await settle();
  assert.equal(doc.getElementById('srvIntentRow').hidden, true,
    'setup: the receive flow is showing the agreement box, so this is not the flow that signs nothing');
  assert.equal(srv().hidden, true,
    'the receive flow is told that signing is permanent. Nothing is signed there — the agreement box '
    + 'beside it is hidden for exactly that reason — so it is a sentence about something the user is '
    + 'not doing');
  doc.getElementById('srvCancel').click();
  await settle();
});

async function openDoc(inCeremony) {
  openResp = { ...openResp, ...(inCeremony ? { inCeremony: true } : { inCeremony: false }) };
  setNextDocument({ numPages: 1 });
  doc.getElementById('pathInput').value = '/tmp/nib-harness/deed.pdf';
  doc.getElementById('openGo').click();
  await settle();
}

test('the initiating door states it too, and it is a paragraph in the card', async () => {
  await openDoc(true);
  doc.getElementById('sessionInitBtn').click();
  await settle();
  assert.equal(doc.getElementById('sessionInitModal').hidden, false,
    'setup: the initiate dialog did not open, so its statement is one nobody sees');
  assert.match(sin().textContent, /cannot remove a signature/i,
    'the initiating door does not state permanence. Both roles reach a signature and D12 is about '
    + 'the document, not about who pressed first');
  assert.equal(sin().tagName, 'P', `the statement is a <${sin().tagName.toLowerCase()}>, not a paragraph`);
  const card = sin().closest('.sigcard');
  assert.ok(card, 'the statement is not inside the card that carries the commit button');
  const go = card.querySelector('#sinGo');
  assert.ok(go, 'setup: the card has no commit button, so "before" is about nothing');
  assert.equal(sin().compareDocumentPosition(go) & 4, 4,
    'the statement comes AFTER the button that commits, so a user who has already pressed it is the '
    + 'first to read it');
  assert.match(sin().textContent, /cancel a ceremony/i,
    'the document is in a ceremony and the initiating door does not say Nib cannot cancel one');
  doc.getElementById('sinCancel').click();
  await settle();
});

test('and OUTSIDE a ceremony the initiating door does not claim there is one', async () => {
  await openDoc(false);
  doc.getElementById('sessionInitBtn').click();
  await settle();
  assert.equal(doc.getElementById('sessionInitModal').hidden, false, 'setup: the dialog did not open');
  const text = sin().textContent;
  doc.getElementById('sinCancel').click();
  await settle();

  assert.match(text, /cannot remove a signature/i,
    'a plain co-sign is told nothing, and it still puts a permanent signature on a document');
  assert.doesNotMatch(text, /ceremony/i,
    `a plain two-party co-sign is told ${JSON.stringify(text)}. There is no ceremony here — this `
    + 'dialog is the generic "Co-sign live with a peer" — so "run it again as a new ceremony" names '
    + 'something that does not exist on this flow');
});
