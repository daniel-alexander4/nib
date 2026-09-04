// The consent screen with a THREE-signature document (P06.S07, D27, C-D27).
//
// **The render shipped at v1.117.220 and nothing has ever driven it.** `renderConsentSigners` lists
// every party already on the document, marks an invalid signature rather than dropping it, and
// separates "nobody yet" from "we did not look". Named search before this file:
// `grep -rn 'srvSigners|renderConsentSigners' test/ build/` returned **one** hit, and it was
// `published.test.mjs` naming the function as the reader that lets `pending.signers` out of the
// exclusions. So the field had a reader and the reader had no driver.
//
// **Three signatures and not two, which is the criterion's own words**: *"driven with a
// three-signature document, because a two-party fixture cannot tell a roster from a single peer."*
// A two-signature fixture is satisfied by a screen that renders the connected peer and one other,
// which is the defect D27 names — under a carry route the connected peer is a non-signing convener
// and at hop 6 the user is joining five signatures they were never shown.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const PEER = { fingerprint: 'a'.repeat(64), label: 'Ada' };
const SIGNERS = [
  { signer: 'Ada Landlord', fingerprint: 'a'.repeat(64), valid: true },
  { signer: 'Bo Surveyor', fingerprint: 'b'.repeat(64), valid: true },
  // The third does not verify, and it is listed and marked rather than dropped: a document
  // arriving with a broken signature is exactly what a user needs before adding theirs.
  { signer: 'Cy Witness', fingerprint: 'c'.repeat(64), valid: false },
];

let armed = false;
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
          signers: SIGNERS,
        },
      }
      : { armed: false }),
    '/api/session/disarm': () => { armed = false; return {}; },
  },
});
const { document: doc, settle } = h;

test('the consent screen names every party already on the document, not one', async () => {
  // `showConsent` renders the received document in its own pdf.js instance, so the stub needs a
  // document to hand it — without one the preview throws before the signer list is read.
  setNextDocument({ numPages: 1 });
  doc.getElementById('sessionRecvBtn').click();
  await settle();
  const peerSel = doc.getElementById('srvPeer');
  peerSel.selectedIndex = 0;
  doc.getElementById('srvArmGo').click();
  await settle();
  // The consent screen arrives on a POLL, not on the arm response. 1800 ms against a 1500 ms
  // interval — armprogress.test.mjs measured that a shorter wait reads the pre-poll DOM and
  // passes on the wrong thing.
  await new Promise((r) => setTimeout(r, 1800));
  await settle();

  const box = doc.getElementById('srvSigners');
  const text = box ? box.textContent : '';
  const rows = box ? box.querySelectorAll('.cidrow').length : 0;

  // Observe, clean up, THEN assert — an armed session leaves a repeating poll timer, and a file
  // that leaves one does not fail, it HANGS. armed.test.mjs measured both other orderings.
  doc.getElementById('srvCancel').click();
  await settle();

  assert.ok(box, 'there is no #srvSigners in index.html');
  // SETUP: the consent screen was actually reached. Without this every assertion below is
  // satisfied by an empty box on a screen that never opened.
  assert.ok(text, 'setup: the consent screen never rendered its signer list, so nothing below is ' +
    'being tested — the poll did not deliver `pending`');

  assert.equal(rows, 3,
    `the consent screen drew ${rows} signer row(s) for a document carrying three signatures. ` +
    'D27 is that the party is shown everyone they are joining: under a carry route the connected ' +
    'peer is a non-signing convener, so a screen naming one person names the wrong one.');
  for (const who of ['Ada Landlord', 'Bo Surveyor', 'Cy Witness']) {
    assert.ok(text.includes(who), `the consent screen omits ${who}`);
  }
  assert.match(text, /does not verify/,
    'the third signature does not verify and the screen does not say so. Dropping it would make ' +
    'the list shorter and the document look cleaner than it is, on the one screen where the user ' +
    'decides whether to add their own name to it.');
});
