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
let armBind;           // the `bind` field of the most recent /api/session/arm POST (S12 twin)

const h = await boot({
  routes: {
    '/api/open': () => ({
      id: 'test-epoch:1', name: 'deed.pdf', path: DOC, canSave: true,
      signature: { state: 'unsigned' }, canUndo: false, canRedo: false,
    }),
    '/api/peers': () => ({ self: 'b'.repeat(64), fingerprint: 'b'.repeat(64), name: 'Me', peers: [PEER] }),
    '/api/session/status': () => ({ armed: false }),
    '/api/session/arm': (opts) => {
      // The arm side's twin of the empty-address default: an empty bind is the LAN receive
      // path (server binds 0.0.0.0:0 and announces it), not an error.
      armBind = JSON.parse(opts.body).bind;
      return { armed: true, address: '0.0.0.0:54321' };
    },
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

test('an empty bind arms the LAN receive path — /api/session/arm POSTs with a blank bind', async () => {
  // The receive side's twin of S12: armRecv used to refuse an empty bind and #srvBind hardcoded
  // 0.0.0.0:8443, so P03's LAN receive path was unreachable from the UI and two Nibs on one
  // machine collided on the port. Restoring the refusal makes this red (arm never POSTs).
  doc.getElementById('sessionRecvBtn').click();
  await settle();
  const peerSel = doc.getElementById('srvPeer');
  assert.ok(peerSel.options.length >= 1, 'the receive peer select was never populated');
  peerSel.value = PEER.fingerprint;
  doc.getElementById('srvBind').value = '';
  armBind = undefined;
  doc.getElementById('srvArmGo').click();
  await settle();
  assert.equal(armBind, '', 'arm was not POSTed with an empty bind (the LAN receive path is unreachable)');
});

test('the listen address input is inside the advanced disclosure, not on the default surface', async () => {
  const bind = doc.getElementById('srvBind');
  assert.ok(bind, 'no #srvBind in index.html');
  assert.ok(bind.closest('details.advanced'), 'the listen address is not behind <details class="advanced">');
  assert.equal(bind.value, '', 'the listen address must default to blank (the old 0.0.0.0:8443 broke two Nibs on one machine)');
});
