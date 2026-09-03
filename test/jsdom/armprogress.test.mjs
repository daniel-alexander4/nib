// The armed screen shows what each tier is doing, and never a blank wait (P06.S05, D16, D15, D34).
//
// ── What this is written against ─────────────────────────────────────────────
// `sessionStatus.diagnosis` is filled only once `bootstrapDone` is true, which is right for a
// VERDICT — a cause computed before the DHT has had its chance would accuse the wrong tier. But
// under ADR-011 nothing bootstraps until the local link has had its window, and where a browse has
// answered that hold is `lanFirstBudget`: thirty seconds. So the product was deliberately silent
// for the longest stretch of an arm, which is precisely the window D16 says must never be a blank
// spinner.
//
// ── Why the router's states are asserted separately ──────────────────────────
// D15's criterion has two halves and the second is the one that ships unexercised: the screen
// "discloses that a temporary router opening was requested and names the port; **when no mapping
// was obtained it says so rather than staying silent**". Silence, a refusal and an unroutable
// answer are three different next actions — D9's advice diverges to a VPN on the third — so a
// screen collapsing them gives one answer to three users who need different ones.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

const PEER = { fingerprint: 'a'.repeat(64), label: 'Ada' };

let armed = false;
let progress = { link: 'watching', dht: 'holding' };
let until;

const { document: doc, settle } = await boot({
  routes: {
    '/api/peers': () => ({ self: 'b'.repeat(64), peers: [PEER] }),
    '/api/session/arm': () => { armed = true; return { armed: true, address: '127.0.0.1:8443' }; },
    '/api/session/status': () => ({ armed, address: '127.0.0.1:8443', progress, until }),
    '/api/session/disarm': () => { armed = false; return {}; },
  },
});

// The poll is scheduled 1200 ms after arming and this tier has no fake clock, so the wait is real
// — `waitdiagnosis.test.mjs`' own note, and the routes are re-read every poll, so a later case
// only needs the NEXT tick rather than another arm.
//
// **1800 ms and not 1400.** The poll interval is 1500 ms, so a tick shorter than one interval can
// return before the changed state has been fetched — measured: the `refused` case rendered the
// `silent` sentence, because the render it read was the previous poll's. A wait shorter than the
// thing it is waiting for is a flake that happens to pass most of the time.
const tick = async () => { await new Promise((r) => setTimeout(r, 1800)); await settle(); };

async function arm() {
  doc.getElementById('sessionRecvBtn').click();
  await settle();
  doc.getElementById('srvPeer').selectedIndex = 0;
  doc.getElementById('srvArmGo').click();
  await settle();
}

async function poll() {
  await tick();
  return doc.getElementById('srvWaitTiers');
}

test('the pre-bootstrap wait is described rather than left blank', async () => {
  await arm();
  const host = await poll();
  assert.ok(host, 'there is no tier list at all');
  assert.equal(host.hidden, false, 'the tier list is hidden while an arm is waiting');
  const text = host.textContent;
  assert.match(text, /local network/,
    'the link tier is not described. This is the window the diagnosis structurally cannot speak ' +
    'in, and it is the longest part of a LAN arm');
  assert.match(text, /Not using the internet yet/,
    'the DHT hold is not described. It is not a failure and not a delay to apologise for — it is ' +
    'the product deliberately not touching the public network until the link has had its chance');
});

test('the router opening names its port', async () => {
  progress = { link: 'found', dht: 'reaching', router: 'open', port: 41234 };
  const host = await poll();
  assert.match(host.textContent, /port 41234/,
    'D15 says the screen discloses that a temporary router opening was requested and NAMES THE ' +
    'PORT; a line saying only that one was opened is half the criterion');
});

test('no mapping says so, and says which kind of no', async () => {
  for (const [state, want] of [
    ['silent', /did not answer/],
    ['refused', /answered and declined/],
    ['unroutable', /cannot be reached from outside/],
  ]) {
    progress = { link: 'watching', router: state };
    const host = await poll();
    assert.match(host.textContent, want,
      `router=${state} is not described distinctly. Silence, a refusal and an unroutable answer ` +
      'are three different next actions — the third points at a VPN rather than a port-forward — ' +
      'and one sentence for all three gives one answer to three users');
  }
  // And they are DIFFERENT sentences, not one string that happens to match three patterns.
  const seen = new Set();
  for (const state of ['silent', 'refused', 'unroutable']) {
    progress = { link: 'watching', router: state };
    seen.add((await poll()).textContent);
  }
  assert.equal(seen.size, 3, 'the three router failures render the same text');
});

test('no tier line is a countdown', async () => {
  until = '2026-10-01T12:00:00Z';
  progress = { link: 'found', dht: 'reaching', router: 'open', port: 41234 };
  const host = await poll();
  // D16's amendment: only the ceremony deadline appears in human units, and neither the connect
  // deadline nor the exchange deadline appears as a countdown. A tier line showing seconds left
  // would be exactly that — and it would invite a user to watch a number instead of reading the
  // router line, which is the only one they can act on.
  assert.doesNotMatch(host.textContent, /\b\d+\s*(s|sec|second|m|min|minute)s?\b/i,
    `a tier line reads like a countdown: ${JSON.stringify(host.textContent)}`);
});

test('the tier list clears when the arm stops reporting', async () => {
  progress = { link: 'watching' };
  assert.equal((await poll()).hidden, false, 'setup: nothing was shown to clear');
  progress = undefined;
  const host = await poll();
  assert.equal(host.hidden, true,
    'the ladder is still on screen after the arm stopped. Progress is LIVE state, not a sticky ' +
    'record — leaving it up describes a wait nobody is doing');
});

// **Disarmed at the end, and it is not tidiness.** `pollRecv` reschedules itself every 1.5 s for as
// long as the arm is up, and node's test runner waits for a quiet event loop — a file that leaves
// one armed keeps a timer alive and the whole suite hangs behind it. Measured: the run went from
// ten seconds to the harness's 900-second ceiling with this missing.
test('the arm is put away, so this file leaves no timer behind', async () => {
  doc.getElementById('srvDisarm')?.click();
  await settle();
  armed = false;
  await tick();
  assert.equal(armed, false, 'the arm is still up');
});
