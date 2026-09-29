// /pending 750 (2) — the applying stage ends on the request being finished with, not on a disarm.
//
// ── The defect ───────────────────────────────────────────────────────────────
// After the user accepted, `pollRecv` waited for `!st.armed` to report where the document landed.
// `armed` is the MACHINE's answer — any arm makes it true — so with a delivery arm up beside the
// interactive one it never went false, and the page sat on "Saving…" for the life of that arm while
// the document had long since been saved.
//
// ── What this tier can see ───────────────────────────────────────────────────
// The poll against a stubbed status route that keeps `armed: true` throughout and reports the
// accepted request as `settled`. That the server sets `settled` after the save and the arrival is
// `internal/server`'s to show (TestAnAcceptedRequestIsSettledAfterItsSession).
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import * as pdfjs from './stub-pdfjs.mjs';

const PEER = { fingerprint: 'a'.repeat(64), label: 'Ada' };
const A = { id: 'req-A', signer: 'Ada', fingerprint: 'a'.repeat(64), reason: 'a lease', signers: [] };

let armed = false;
let pending = null;
let settled = '';
let received = null;

const { document: doc, settle } = await boot({
  routes: {
    '/api/peers': () => ({ self: 'f'.repeat(64), peers: [PEER] }),
    '/api/session/arm': () => { armed = true; return { armed: true, address: '127.0.0.1:8443' }; },
    '/api/session/status': () => (armed
      ? { armed: true, address: '127.0.0.1:8443', ...(pending ? { pending } : {}),
          ...(settled ? { settled } : {}), ...(received ? { received } : {}) }
      : { armed: false }),
    '/api/session/respond': () => ({ armed: true }),
    '/api/session/disarm': () => { armed = false; pending = null; return {}; },
  },
});

const tick = async () => { await new Promise((r) => setTimeout(r, 1800)); await settle(); };

test('an accepted transfer is reported when it is saved, while another arm keeps the machine armed', async () => {
  pending = A;
  pdfjs.setNextDocument({ numPages: 1, renders: true });
  doc.getElementById('sessionRecvDocBtn').click();
  await settle();
  doc.getElementById('srvPeer').selectedIndex = 0;
  doc.getElementById('srvArmGo').click();
  await settle();
  await tick();
  assert.equal(doc.getElementById('srvConsent').hidden, false, 'setup: the consent screen never opened');

  doc.getElementById('srvAccept').click();
  await settle();
  // The save lands; the interactive arm is gone and the DELIVERY arm stays up, so `armed` holds.
  pending = null;
  received = { path: '/home/u/nib/incoming/ada.pdf', peer: 'Ada' };
  settled = 'req-A';
  await tick();

  const modalUp = !doc.getElementById('sessionRecvModal').hidden;
  const said = doc.getElementById('toast')?.textContent || '';
  const pill = !doc.getElementById('armedPill')?.hidden;
  // Put everything away before asserting, or a failure leaves the poll's timer holding the runner.
  armed = false;
  await tick();
  assert.equal(modalUp, false,
    'the document was saved and the page is still on "Saving…", waiting for a disarm that a '
    + 'delivery arm beside it will not give');
  assert.match(said, /Saved \/home\/u\/nib\/incoming\/ada\.pdf/, `the save was not reported: ${JSON.stringify(said)}`);
  assert.equal(pill, true, 'the armed pill went down while the delivery arm is still up');
});
