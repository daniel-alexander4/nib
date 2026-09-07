// P02.S01 — the signer's statement defaults to the ceremony's recital, not a hardcoded sentence.
//
// **The defect this drives.** `convene.go:40` says the record's `Intent` is *"the recital every
// party agrees to"* and that D20 makes it *"the only home for it"* — while the consent screen's
// box defaulted to the literal `"I agree to sign this document."` and that string is what the
// signature carried, because `respond` signs whatever is in the box. Two statements of one
// agreement, and the signed one was the generic one. Nobody would notice from either side: the
// convener sees their recital in the record, the signer sees a sentence that reads fine.
//
// **Two cases, and the second is the one that keeps this honest.** Inside a ceremony the box takes
// the recital; OUTSIDE one — a plain two-party co-sign, which has no record and no recital — the
// original default must stand. A change that set the box from a field that is simply absent would
// leave it empty, and an empty agreement statement is worse than a generic one.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const PEER = { fingerprint: 'a'.repeat(64), label: 'Ada' };
const RECITAL = 'We agree to the terms of the Fitzroy Street lease.';

// The recital is served only while `withRecital` is true, so one boot can drive both cases.
let armed = false;
let withRecital = true;
const h = await boot({
  routes: {
    '/api/peers': () => ({ self: 'f'.repeat(64), peers: [PEER] }),
    '/api/session/arm': () => { armed = true; return { armed: true, address: '127.0.0.1:8443' }; },
    '/api/session/status': () => (armed
      ? {
        armed: true,
        address: '127.0.0.1:8443',
        pending: {
          signer: 'Ada Landlord',
          fingerprint: 'a'.repeat(64),
          reason: 'I agree to co-sign the lease',
          signers: [],
          // Absent, not empty, outside a ceremony — which is what the server sends, since the
          // field is `omitempty` and there is no ceremony to have a recital.
          ...(withRecital ? { recital: RECITAL } : {}),
        },
      }
      : { armed: false }),
    '/api/session/disarm': () => { armed = false; return {}; },
  },
});
const { document: doc, settle } = h;

// reachConsent arms and waits for the consent screen, which arrives on a POLL rather than on the
// arm response — 1800 ms against a 1500 ms interval, the figure armprogress.test.mjs measured.
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

// cleanUp cancels before asserting: an armed session leaves a repeating poll timer, and a file
// that leaves one does not fail, it HANGS (armed.test.mjs measured both orderings).
async function cleanUp() {
  doc.getElementById('srvCancel').click();
  await settle();
}

test('inside a ceremony the box carries the ceremony\'s own recital', async () => {
  withRecital = true;
  await reachConsent();
  const box = doc.getElementById('srvIntent');
  const got = box ? box.value : '';
  const onConsent = !doc.getElementById('srvConsent').hidden;
  await cleanUp();

  assert.ok(box, 'there is no #srvIntent in index.html');
  // SETUP: the consent screen was actually reached. Without this the assertion below is satisfied
  // by a box on a screen that never opened, holding whatever it was last set to.
  assert.ok(onConsent, 'the consent screen was never reached, so the box below proves nothing');
  assert.equal(got, RECITAL,
    'the signer is agreeing to a sentence this app made up rather than the one in the record — '
    + 'and `respond` signs what is in this box');
});

test('outside a ceremony the original default stands', async () => {
  withRecital = false;
  await reachConsent();
  const box = doc.getElementById('srvIntent');
  const got = box ? box.value : '';
  const onConsent = !doc.getElementById('srvConsent').hidden;
  await cleanUp();

  assert.ok(onConsent, 'the consent screen was never reached, so the box below proves nothing');
  assert.notEqual(got, '',
    'a plain co-sign has no recital, and the box was left empty — an empty agreement statement '
    + 'is worse than a generic one');
  assert.notEqual(got, RECITAL, 'the recital leaked into a session that has no ceremony');
});
