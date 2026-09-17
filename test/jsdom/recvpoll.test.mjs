// /pending 513 — the receive poll survives a blip and gives up OUT LOUD.
//
// ── The defect ───────────────────────────────────────────────────────────────
// `pollRecv` drives the whole receive state machine off `/api/session/status`, and it is the ONLY
// thing that moves the wait screen to consent, reports the arm's progress, or notices the server
// disarming. Its error arm was `catch { return; }` — one failed request, from any cause, and the
// chain stopped for good. The screen it leaves behind is the one that says a peer is expected: the
// armed pill stays lit, the wait view stays up, and the document that actually arrives a moment
// later is never mentioned. Nothing else reschedules it, so the failure is permanent and silent.
//
// ── The policy, and why it is this one ───────────────────────────────────────
// Three consecutive failures, retried at the poll's own 1.5 s, and the counter resets on any poll
// that answers. Flat, not exponential, deliberately: backoff exists to spare a service that many
// clients share, and this one is nib's own server on loopback in the same session — there is
// nothing to spare, and the only question a bound answers is "how long before we stop lying to the
// user". The bound matters in the other direction too: a poll that retried forever against a
// server that is gone would hold a 1.5 s timer for the life of the tab and keep an armed pill lit
// over nothing.
//
// A 401 is NOT one of the three. `apiFetch` throws `locked` after putting the app on the unlock
// screen, so the arm is over by decision rather than by accident, and retrying is two more 401s.
//
// ── What this tier can see ───────────────────────────────────────────────────
// The poll is real `setTimeout` against a stubbed fetch, so the recovery and the give-up are both
// observable here as DOM state plus the call record. What it cannot see is a real transport
// failure — the stub throws where the network would — which is the ordinary shape of this tier's
// ceiling and not a gap in the assertion.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

const PEER = { fingerprint: 'a'.repeat(64), label: 'Ada' };

let armed = false;
// How many of the NEXT status polls throw. Counted down by the route itself, so a test says "the
// next one fails" rather than having to switch a flag back at the right moment.
let failNext = 0;
// The same counter for the OTHER failure, the one that must not be retried at all: a 401, which
// `apiFetch` turns into a thrown `locked` after sending the app to the unlock screen.
let lockNext = 0;
let statusCalls = 0;

const { document: doc, settle } = await boot({
  routes: {
    '/api/peers': () => ({ self: 'b'.repeat(64), peers: [PEER] }),
    '/api/session/arm': () => { armed = true; return { armed: true, address: '127.0.0.1:8443' }; },
    '/api/session/status': () => {
      statusCalls++;
      if (failNext > 0) { failNext--; throw new Error('harness: the status route is down'); }
      if (lockNext > 0) {
        lockNext--;
        return new Response(JSON.stringify({ error: 'locked' }), {
          status: 401, headers: { 'Content-Type': 'application/json' },
        });
      }
      return { armed, address: '127.0.0.1:8443' };
    },
    '/api/session/disarm': () => { armed = false; return {}; },
  },
});

// The first poll is scheduled 1200 ms after arming and every later one 1500 ms, and this tier has
// no fake clock — armprogress.test.mjs makes the same argument and measured the same number. 1800
// is one interval plus a margin, because a wait shorter than the thing it is waiting for is a
// flake that passes most of the time.
const tick = async () => { await new Promise((r) => setTimeout(r, 1800)); await settle(); };

async function arm() {
  doc.getElementById('sessionRecvBtn').click();
  await settle();
  doc.getElementById('srvPeer').selectedIndex = 0;
  doc.getElementById('srvArmGo').click();
  await settle();
}

test('one failed status poll does not end the poll', async () => {
  await arm();
  await tick();
  const before = statusCalls;
  assert.ok(before > 0, 'setup: the poller never ran at all, so nothing below is about its error arm');

  failNext = 1;
  await tick();          // this one throws
  assert.equal(statusCalls, before + 1, 'setup: the failing poll was never made');
  await tick();          // the retry — the whole assertion
  assert.ok(statusCalls > before + 1,
    'the poll stopped after ONE failed request. Nothing else reschedules it, so the wait screen '
    + 'and the armed pill stay up over a session this client has stopped watching — including '
    + 'through the arrival it exists to report');
});

test('a status route that stays down ends the wait out loud', async () => {
  const before = statusCalls;
  // Three consecutive failures is the bound. Four are queued so the test cannot pass by the route
  // recovering underneath it.
  failNext = 4;
  for (let i = 0; i < 5; i++) await tick();
  assert.equal(statusCalls, before + 3,
    `the poll made ${statusCalls - before} requests after the route went down, not 3 — either it `
    + 'gave up early (a blip is not a dead server) or it never gave up at all, which is a 1.5 s '
    + 'timer for the life of the tab and an armed pill over nothing');
  assert.equal(doc.getElementById('sessionRecvModal').hidden, true,
    'the receive dialog is still up after the poll gave up, so the user is looking at a wait '
    + 'screen that nothing is driving any more');
  assert.equal(doc.getElementById('armedPill').hidden, true,
    'the armed indicator is still lit after the client stopped watching the session');
  // `#toast` is created lazily by app.js and appended to <body>, so it may not exist at all — and
  // "no node" is one of the two ways this can fail silently.
  const said = doc.getElementById('toast')?.textContent || '';
  assert.match(said, /Nib/,
    `nothing told the user the wait ended, so it ended silently: ${JSON.stringify(said)}`);
  failNext = 0;
});

// A 401 is the vault locking, and it is a DECISION rather than an accident: `apiFetch` puts the app
// on the unlock screen and throws `locked`, so the arm is over and the three retries would be three
// more 401s and three more `refreshStatus` calls against a server that is answering correctly.
// Asserted separately because "stops immediately" and "stops after three" are the same observable
// once the polling has stopped — only the call count tells them apart.
test('a 401 stops the poll at once rather than spending the retries on it', async () => {
  await arm();
  await tick();
  const before = statusCalls;
  lockNext = 3;
  for (let i = 0; i < 3; i++) await tick();
  assert.equal(statusCalls, before + 1,
    `the poll made ${statusCalls - before} requests after the vault locked, not 1 — a locked vault `
    + 'is not a blip, and apiFetch has already moved the app to the unlock screen');
  lockNext = 0;
});

// **Disarmed at the end, and it is not tidiness** — armprogress.test.mjs's words, and its measured
// consequence: `pollRecv` reschedules itself for as long as the arm is up, node's test runner waits
// for a quiet event loop, and a file that leaves one armed hangs the whole suite behind it.
test('the arm is put away, so this file leaves no timer behind', async () => {
  doc.getElementById('srvDisarm')?.click();
  await settle();
  armed = false;
  await tick();
  assert.equal(armed, false, 'the arm is still up');
});
