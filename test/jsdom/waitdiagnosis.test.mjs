// /pending 349 — a waiting arm says WHY nothing has connected.
//
// The server has computed this since P05.S11 and published it as `status.diagnosis`. Its own doc
// states the purpose: "so the polling UI shows why nothing has connected yet, RATHER THAN A BLANK
// WAIT". Nothing read it. A named search found `diagnosis` in web/app.js exactly once, inside a
// comment — so for the life of the feature the user watching an arm got the blank wait it exists to
// prevent. Found by the reader scan the first pass it could see internal/server (/pending 347).
//
// ── What this tier can reach ─────────────────────────────────────────────────
// All of the presentation, which is where the defect was: whether the sentence appears at all,
// which half is visible and which is behind the disclosure (D19's presentation pin), that it
// CLEARS when the reason stops applying, and that it is announced politely rather than assertively.
//
// ── What it cannot, and who covers it ────────────────────────────────────────
// That the server ever SETS `diagnosis` — that is tier 1 (`internal/server`'s diagnose tests) and
// the reader scan in observables_test.go, which is what will now fail if the field goes unread
// again. This tier stubs the reply, so a server that never sent one would leave every assertion
// here green. That is the ceiling and it is why the tier-1 rows exist.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

const PEER = { fingerprint: 'a'.repeat(64), label: 'Ada' };

let armed = false;
let diagnosis = null;
const h = await boot({
  routes: {
    '/api/peers': () => ({ self: 'b'.repeat(64), peers: [PEER] }),
    '/api/session/arm': () => { armed = true; return { armed: true, address: '127.0.0.1:8443' }; },
    '/api/session/status': () => ({ armed, address: '127.0.0.1:8443', diagnosis }),
    '/api/session/disarm': () => { armed = false; return {}; },
  },
});
const { document: doc, settle } = h;

const why = () => doc.getElementById('srvWaitWhy');
const more = () => doc.getElementById('srvWaitWhyMore');
const detail = () => doc.getElementById('srvWaitWhyDetail');

// The poll is scheduled 1200 ms after arming and this tier has no fake clock, so the wait is real.
// It is one wait for the whole file: the routes are re-read on every poll, so a later phase only
// needs the NEXT tick rather than another arm.
const tick = async () => { await new Promise((r) => setTimeout(r, 1400)); await settle(); };

test('a waiting arm shows why nothing has connected, and stops when the reason does', async () => {
  doc.getElementById('sessionRecvBtn').click();
  await settle();
  doc.getElementById('srvPeer').selectedIndex = 0;

  // BEFORE: armed, no diagnosis yet — the line must not be up, or it is furniture rather than
  // information and the user learns to ignore it.
  diagnosis = null;
  doc.getElementById('srvArmGo').click();
  await settle();
  const hiddenWithNoDiagnosis = why().hidden;

  // THE CASE THE FIELD EXISTS FOR: the counterparty has not started.
  diagnosis = {
    cause: 'peer-not-started',
    summary: "The other person hasn't opened their side yet.",
    detail: 'No announcement for that peer has been seen on the rendezvous network in this window.',
  };
  await tick();
  const shown = why().hidden;
  const shownText = why().textContent;
  const cause = why().dataset.cause;
  const moreShown = more().hidden;
  const detailText = detail().textContent;
  const role = why().getAttribute('role');
  const live = why().getAttribute('aria-live');

  // AND IT CLEARS. A diagnosis is live state, not a sticky record: when the server stops sending
  // one the reason has stopped applying, and leaving the sentence up explains a condition that
  // has passed.
  diagnosis = null;
  await tick();
  const clearedText = why().hidden;
  const clearedMore = more().hidden;

  // ── OBSERVE, THEN CLEAN UP, THEN ASSERT ─────────────────────────────────────
  // Arming schedules a repeating poll, and a pending timer keeps node's event loop alive: a file
  // that leaves a session armed does not FAIL, it HANGS — which reads as infrastructure trouble
  // rather than as the assertion failure it is. armed.test.mjs paid for this ordering; it is
  // copied deliberately rather than re-derived.
  doc.getElementById('srvCancel').click();
  await settle();

  assert.equal(hiddenWithNoDiagnosis, true,
    'the "why" line is up before the server has diagnosed anything — a line that always shows is one the user stops reading');
  assert.equal(shown, false,
    'the server said why nothing has connected and the wait view shows nothing. This is /pending 349: the field states its purpose as "rather than a blank wait", and the blank wait is what the user got');
  assert.match(shownText, /hasn't opened their side/,
    'the visible line is not the plain-language summary');
  assert.equal(cause, 'peer-not-started',
    'the cause is not carried onto the element, so nothing can distinguish the ordinary early state from a real fault');
  assert.ok(!shownText.includes('rendezvous network'),
    'the technical detail is rendered inline. D19’s presentation pin puts the plain sentence first and the detail behind a disclosure');
  assert.equal(moreShown, false, 'the disclosure carrying the technical detail is hidden even though there is detail to show');
  assert.match(detailText, /rendezvous network/, 'the disclosure does not carry the technical detail');

  assert.equal(role, 'status',
    'the line has no role, so it appears with no user action and is never announced');
  assert.notEqual(role, 'alert',
    'the line is assertive: #sessionNotice reserves that for "you are about to lose a signature", and a line that updates every 1.5s while somebody waits would devalue it');
  assert.equal(live, 'polite', 'the line is not a live region, so its text is never read out');

  assert.equal(clearedText, true,
    'the reason stopped applying and the sentence stayed on screen — a stale explanation of a condition that has passed');
  assert.equal(clearedMore, true, 'the disclosure survived the diagnosis it belonged to');
});
