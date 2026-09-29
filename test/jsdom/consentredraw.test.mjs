// /pending 744 (3) — the consent screen follows the request the server holds.
//
// ── The defect ───────────────────────────────────────────────────────────────
// `pollRecv` promoted wait → consent ONCE and never compared again. So when the request on screen
// went away and a different one parked in its place (a second arm's hop, /pending 660's scenario),
// the page kept showing the first — its signer, its reason, its preview — and the answer the user
// typed for it was refused 409 by the server's id check. And when the request went away with
// nothing replacing it, the consent screen stayed up over nothing at all.
//
// ── What this tier can see ───────────────────────────────────────────────────
// The poll is real `setTimeout` against a stubbed status route, so the redraw is observable as DOM
// state, the preview's URL as the id pdf.js was asked for, and the answer as the id it posts. What
// it cannot see is the server's side of the race, which `internal/server/consentslot_test.go` owns.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import * as pdfjs from './stub-pdfjs.mjs';

const PEER = { fingerprint: 'a'.repeat(64), label: 'Ada' };
const A = { id: 'req-A', signer: 'Ada Landlord', fingerprint: 'a'.repeat(64), reason: 'lease A', signers: [] };
const B = { id: 'req-B', signer: 'Bea Tenant', fingerprint: 'b'.repeat(64), reason: 'lease B', signers: [] };

let armed = false;
let pending = null;
const answers = [];

const { document: doc, settle } = await boot({
  routes: {
    '/api/peers': () => ({ self: 'f'.repeat(64), peers: [PEER] }),
    '/api/session/arm': () => { armed = true; return { armed: true, address: '127.0.0.1:8443' }; },
    '/api/session/status': () => (armed
      ? { armed: true, address: '127.0.0.1:8443', ...(pending ? { pending } : {}) }
      : { armed: false }),
    '/api/session/respond': (req) => { answers.push(req.body); return { armed: true }; },
    '/api/session/disarm': () => { armed = false; pending = null; return {}; },
  },
});

// One poll interval plus a margin — recvpoll.test.mjs's measured figure and reasoning.
const tick = async () => { await new Promise((r) => setTimeout(r, 1800)); await settle(); };

test('a replaced request is redrawn, and a vanished one clears', async () => {
  pending = A;
  pdfjs.setNextDocument({ numPages: 1, renders: true });
  doc.getElementById('sessionRecvBtn').click();
  await settle();
  doc.getElementById('srvPeer').selectedIndex = 0;
  doc.getElementById('srvArmGo').click();
  await settle();
  await tick();

  // SETUP: the screen really is showing A, or the redraw below is indistinguishable from a first draw.
  assert.equal(doc.getElementById('srvConsent').hidden, false, 'setup: the consent screen never opened');
  assert.equal(doc.getElementById('srvPeerLabel').textContent, 'Ada Landlord', 'setup: the screen is not showing A');
  assert.match(String(pdfjs.lastGetDocumentUrl), /[?&]id=req-A(&|$)/,
    `the preview did not name the request it is of: ${pdfjs.lastGetDocumentUrl}`);

  pending = B;
  await tick();
  assert.equal(doc.getElementById('srvPeerLabel').textContent, 'Bea Tenant',
    'a DIFFERENT request is parked and the page still shows the first — the Accept the user types '
    + 'here is for a document the server is no longer asking about');
  assert.match(String(pdfjs.lastGetDocumentUrl), /[?&]id=req-B(&|$)/,
    `the preview was not reloaded for the request now on screen: ${pdfjs.lastGetDocumentUrl}`);

  // The answer names what is on screen now.
  doc.getElementById('srvDecline').click();
  await settle();
  const last = answers.at(-1);
  const body = typeof last === 'string' ? JSON.parse(last) : last;
  assert.equal(body && body.id, 'req-B', `the answer named ${JSON.stringify(body)}, want req-B`);
});

test('a request that goes away with nothing in its place takes its screen with it', async () => {
  pending = A;
  pdfjs.setNextDocument({ numPages: 1, renders: true });
  armed = false;
  doc.getElementById('sessionRecvBtn').click();
  await settle();
  doc.getElementById('srvPeer').selectedIndex = 0;
  doc.getElementById('srvArmGo').click();
  await settle();
  await tick();
  assert.equal(doc.getElementById('srvConsent').hidden, false, 'setup: the consent screen never opened');

  pending = null; // A timed out, or its arm was cancelled — the arm beside it is still up
  await tick();
  // Read, THEN put the arm away, THEN assert: a failed assertion before the disarm leaves pollRecv's
  // timer holding the runner open, and the whole suite hangs behind this file (recvpoll.test.mjs).
  const consentUp = !doc.getElementById('srvConsent').hidden;
  const waiting = !doc.getElementById('srvWait').hidden;
  armed = false;
  doc.getElementById('srvCancel').click();
  await settle();
  await tick();
  assert.equal(consentUp, false,
    'the request went away and its consent screen is still up, answerable, over nothing');
  assert.equal(waiting, true, 'the page did not go back to waiting');
});
