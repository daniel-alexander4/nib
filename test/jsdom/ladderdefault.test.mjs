// P05.S12 — the traversal ladder is the DEFAULT path for a live co-sign.
//
// Before this slice, sessionInit() refused to POST without a typed address
// (`if (!address) { toast("Enter the peer's address"); return; }`), so the shipped
// LAN tier (and, for an invited ceremony, the DHT) was unreachable from the product:
// the manual address was not merely undemoted, it was the ONLY path. S12 removes that
// refusal and moves the address field behind the existing `details.advanced`
// disclosure. This file is the instrument for both acceptance clauses:
//
//   (a) a co-sign initiates with NO typed address — the empty string reaches the
//       server, which browses the LAN / uses the DHT;
//   (b) the typed-address path still works from the disclosure (D8 tier 5 undemoted,
//       not deleted).
//
// Both are read off the `address` field of the /api/session/initiate POST, which is
// the exact byte the refusal used to gate. Restoring the refusal makes clause (a) go
// red: the initiate is never POSTed and `initiateAddress` stays undefined.
//
// sessionInit() rasterizes the attestation to a PNG via <canvas> on the way to that
// POST, and jsdom implements no canvas (boot.mjs's ceiling — the raster is tier 3's).
// We are NOT testing the raster; we stub the 2d context and toBlob to a no-op
// passthrough so the flow REACHES the POST whose address field is S12's subject. What
// the attestation actually looks like is tier 3's to verify.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const DOC = '/tmp/nib-harness/deed.pdf';
const PEER = { fingerprint: 'a'.repeat(64), label: 'Ada' };

let initiateAddress;   // the `address` field of the most recent /api/session/initiate POST
let quoteCalls = 0;    // /api/cosign/quote hits — reached only if the refusal is gone

const h = await boot({
  routes: {
    '/api/open': () => ({
      id: 'test-epoch:1', name: 'deed.pdf', path: DOC, canSave: true,
      signature: { state: 'unsigned' }, canUndo: false, canRedo: false,
    }),
    '/api/peers': () => ({ self: 'b'.repeat(64), peers: [PEER] }),
    '/api/cosign/quote': () => {
      quoteCalls++;
      return { lines: ['I agree to sign this document.'], rect: [0, 0, 120, 40], when: '2026-08-22T00:00:00Z' };
    },
    '/api/session/initiate': (opts) => {
      // opts.body is the FormData sessionInit() builds; `address` is the field S12
      // makes optional. Read it here rather than trusting the URL — the address never
      // appears in the path.
      initiateAddress = opts.body.get('address');
      return {
        id: 'test-epoch:1', name: 'deed.pdf', path: DOC, canSave: true,
        signature: { state: 'valid' }, canUndo: false, canRedo: false,
      };
    },
  },
});
const { document: doc, window: win, settle } = h;

// Canvas passthrough — see the header. A no-op 2d context and a toBlob that yields a
// one-byte PNG, just enough for renderAttestation() to resolve and the POST to fire.
const ctxStub = {
  fillRect() {}, strokeRect() {}, fillText() {},
  fillStyle: '', strokeStyle: '', lineWidth: 0, textBaseline: '', font: '',
};
win.HTMLCanvasElement.prototype.getContext = () => ctxStub;
win.HTMLCanvasElement.prototype.toBlob = function (cb) {
  cb(new win.Blob([new Uint8Array([1])], { type: 'image/png' }));
};

async function openDoc() {
  setNextDocument({ numPages: 1, outline: null });
  doc.getElementById('pathInput').value = DOC;
  doc.getElementById('openGo').click();
  await settle();
}

async function coSign(address) {
  doc.getElementById('sessionInitBtn').click();
  await settle();
  const peerSel = doc.getElementById('sinPeer');
  assert.ok(peerSel.options.length >= 1, 'the peer select was never populated — /api/peers not reached');
  peerSel.value = PEER.fingerprint;
  doc.getElementById('sinAddr').value = address;
  quoteCalls = 0;
  initiateAddress = undefined;
  doc.getElementById('sinGo').click();
  await settle();
}

test('an empty address co-signs over the ladder — the initiate POSTs with a blank address', async () => {
  await openDoc();
  await coSign('');
  assert.equal(quoteCalls, 1, 'the co-sign never reached the quote — the empty-address refusal is back');
  assert.equal(initiateAddress, '', 'initiate was not POSTed with an empty address (the ladder path is unreachable)');
});

test('the typed-address fallback still reaches the initiate from behind the disclosure', async () => {
  // The field now lives inside <details class="advanced">; moving it must not break the
  // wiring. A value typed there is the manual tier (D8 tier 5), which stays reachable.
  await coSign('203.0.113.4:8443');
  assert.equal(initiateAddress, '203.0.113.4:8443', 'the typed address did not reach the initiate POST');
});

test('the address input is inside the advanced disclosure, not on the default surface', async () => {
  const addr = doc.getElementById('sinAddr');
  assert.ok(addr, 'no #sinAddr in index.html');
  const details = addr.closest('details.advanced');
  assert.ok(details, 'the address field is not behind <details class="advanced"> — it is on the default path');
});
