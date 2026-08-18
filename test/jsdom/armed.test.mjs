// An armed co-signing session stays VISIBLE across a Close.
//
// The behaviour under it is deliberate and unchanged: arming a session is a separate
// lifecycle from the open document, so closing the document does not disarm — and
// internal/server/session.go calls setDoc() when an exchange completes, which means a
// completed co-sign can make a document appear with no user action at all. Disarming on
// close would silently destroy state the user set up independently.
//
// What was wrong is that nothing said so. After a Close the app looked exactly as idle as
// an app with nothing armed, and then a document appeared. This file drives the fix: arm,
// close, and assert the indicator is still there — which is the whole claim, because the
// indicator is only worth anything if it survives the event that makes it necessary.
//
// P06's tabs do NOT resolve this on their own, checked at the close of that phase: an
// arrival after a Close lands in the single empty view, because after a Close there is
// nothing to arrive beside.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const DOC = '/tmp/nib-harness/doc.pdf';
const PEER = { fingerprint: 'a'.repeat(64), label: 'Ada' };

let armed = false;
const h = await boot({
  routes: {
    '/api/open': () => ({
      id: 'test-epoch:1', name: 'doc.pdf', path: DOC, canSave: true,
      signature: { state: 'unsigned' }, canUndo: false, canRedo: false,
    }),
    '/api/close': () => ({ name: '', path: '', canSave: false, signature: { state: '' }, canUndo: false, canRedo: false }),
    '/api/peers': () => ({ self: 'b'.repeat(64), peers: [PEER] }),
    '/api/session/arm': () => { armed = true; return { armed: true, address: '127.0.0.1:8443' }; },
    // Still armed after the close — which is the server behaviour this indicator exists
    // to make visible, not a convenience for the test.
    '/api/session/status': () => ({ armed, address: '127.0.0.1:8443' }),
    '/api/session/disarm': () => { armed = false; return {}; },
  },
});
const { document: doc, settle } = h;

test('the armed indicator appears on arming and survives closing the document', async () => {
  const pill = doc.getElementById('armedPill');
  assert.ok(pill, 'there is no #armedPill in index.html');
  const pillAtBoot = pill.hidden;

  setNextDocument({ numPages: 2 });
  doc.getElementById('pathInput').value = DOC;
  doc.getElementById('openGo').click();
  await settle();
  const opened = doc.getElementById('viewerWrap').className;

  // Arm through the real dialog — the app exports nothing, and the path a user takes is
  // the one worth driving.
  doc.getElementById('sessionRecvBtn').click();
  await settle();
  const peerSel = doc.getElementById('srvPeer');
  const peerCount = peerSel.options.length;
  peerSel.selectedIndex = 0;
  doc.getElementById('srvArmGo').click();
  await settle();
  const pillWhenArmed = pill.hidden;

  // The event the indicator exists for.
  doc.getElementById('closeBtn').click();
  await settle();
  const wrapAfterClose = doc.getElementById('viewerWrap').className;
  const pillAfterClose = pill.hidden;

  // ── OBSERVE, THEN CLEAN UP, THEN ASSERT ─────────────────────────────────────
  // Every reading above is taken into a local, and the session is disarmed BEFORE a
  // single assertion runs. The ordering is forced, not stylistic: arming schedules a
  // repeating setTimeout poll, a pending timer keeps node's event loop alive, and a file
  // that leaves a session armed does not FAIL — it HANGS, which reads as an
  // infrastructure problem rather than as the assertion failure it is.
  //
  // Two shapes were measured and rejected first. A cleanup after the assertions is
  // skipped exactly when an assertion throws, i.e. exactly when it is needed. And a
  // node:test module-scope `after` hook was measured not to run in that case either —
  // instrumented with a console.log that never printed while the run hung. This ordering
  // depends on neither.
  doc.getElementById('srvCancel').click();
  await settle();
  const pillAfterCancel = pill.hidden;

  assert.equal(pillAtBoot, true, 'setup: the indicator is showing before anything is armed');
  assert.equal(opened, 'has-doc', 'setup: no document is open');
  assert.ok(peerCount > 0, 'setup: the peer list is empty, so arming cannot be driven');
  assert.equal(pillWhenArmed, false,
    'arming a co-signing session shows nothing in the chrome — the one place it would be visible once the dialog is dismissed');
  assert.equal(wrapAfterClose, '', 'setup: the close did not happen, so nothing below is about a closed document');
  assert.equal(pillAfterClose, false,
    'the armed-session indicator vanished when the document closed. The session is STILL ARMED — the server can install a document into this empty view with no user action — so the app is now showing an idle empty state that is lying.');
  assert.equal(pillAfterCancel, true,
    'cancelling the session left the indicator up, so it now describes nothing');
});
