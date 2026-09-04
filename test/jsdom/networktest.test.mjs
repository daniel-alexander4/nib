// The in-product network self-test (/pending 23).
//
// **The discovery counters had a reader and it was a terminal.** `discovery.Socket.Stats()` counts
// sent, heard-ourselves and heard-peers at the same moments, and `nib discover` has printed a
// verdict off them since v1.117.18 — but Nib's primary user is non-technical, on one machine, with
// no IT, and a LAN ceremony that fails is silent by nature: a firewall, a VPN swallowing the group,
// an interface with no carrier.
//
// **D19 cannot cover this and that is structural, not an omission.** `diagnose()` returns
// `causeUndiagnosed` for a LAN or TCP ceremony by construction (`c.rz == nil || c.gate == nil`),
// and the status path publishes nothing for an undiagnosed cause — so the armed screen has never
// had a sentence for exactly this failure. This is that sentence.
//
// **What is driven here**: that the answer is the SERVER's sentence rather than a word rebuilt in
// the client, that the evidence beneath it is rendered, and that "nobody else is here" is not
// styled as success — it is an ordinary answer and still the reason nothing is happening.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

let answer = {
  verdict: 'not-heard-back',
  summary: 'Nib sent announcements and did not hear its own come back. Something on this machine ' +
    'is blocking local network discovery — usually a firewall or a VPN. The other party will not ' +
    'hear you either.',
  sent: 12, own: 0, peers: 0, interfaces: 2, windowMs: 3000,
};
let asked = 0;

const { document: doc, settle } = await boot({
  routes: {
    '/api/lan/test': () => { asked += 1; return answer; },
  },
});

async function runTest() {
  doc.getElementById('srvNetTestGo').click();
  await settle();
  return doc.getElementById('srvNetTestOut');
}

test('the network test says what is wrong, in the words the server chose', async () => {
  const before = asked;
  const out = await runTest();

  // SETUP: the route was actually asked. Without this every assertion below is satisfied by a
  // control that renders a canned sentence and never calls anything.
  assert.equal(asked, before + 1, 'the control did not call /api/lan/test');

  const text = out.textContent;
  assert.ok(text.includes('blocking local network discovery'),
    "the server's own sentence is not rendered. The rule that decides which of the four states " +
    'obtains has ONE door and the wording travels with it — a client that rebuilt a sentence from ' +
    'the verdict tag would be a second copy of that rule, drifting from the CLI the day either ' +
    'is edited.');

  // The evidence beneath the sentence: the window separates "nobody is there" from "nobody
  // answered in three seconds", and the counters are what a user can quote.
  assert.match(text, /Listened for 3 seconds/,
    'the window is not rendered. A verdict with no window cannot be read: "nobody is there" and ' +
    '"nobody answered in three seconds" are different facts with the same sentence.');
  assert.match(text, /sent 12, heard itself 0, heard others 0/,
    'the counters the verdict was drawn from are not shown, so the answer can only be trusted ' +
    'and never checked — and a user reporting a problem has nothing to quote.');
  assert.match(text, /2 network connections/,
    'the interface count is dropped, and zero of them is its own failure — the one that arrives ' +
    'with no counters at all.');

  assert.ok(out.classList.contains('nettest-bad'),
    'a network that is dropping this machine\'s own multicast is not styled as a warning');
});

test('nobody else here is an ordinary answer, and still not success', async () => {
  answer = {
    verdict: 'nobody-else',
    summary: "This machine's network is working — Nib heard itself — and no other Nib is " +
      'announcing here. Either nobody else has armed a session yet, or they are on a different ' +
      'network.',
    sent: 6, own: 6, peers: 0, interfaces: 1, windowMs: 3000,
  };
  const out = await runTest();
  const text = out.textContent;

  assert.ok(text.includes('no other Nib is announcing here'),
    'the "nobody else" sentence is not rendered');
  assert.match(text, /1 network connection\b/,
    'the singular is wrong for one interface — this sentence is read by somebody already ' +
    'confused about their network');
  assert.ok(out.classList.contains('nettest-bad'),
    '"nobody else is here" is styled as success. The network works and the ceremony still is not ' +
    'starting, which is the whole reason the user pressed this — only a verdict of `working` is ' +
    'an answer that needs no action.');
});

test('a working network is the one answer that is not a warning', async () => {
  answer = {
    verdict: 'working', summary: 'Working: another Nib was heard on this network.',
    sent: 6, own: 6, peers: 3, interfaces: 1, windowMs: 3000,
  };
  const out = await runTest();
  assert.ok(out.textContent.includes('another Nib was heard'), 'the working sentence is missing');
  assert.equal(out.classList.contains('nettest-bad'), false,
    'a working network is styled as a warning, which would send a user hunting for a fault that ' +
    'is not there');
});

test('a test that could not run at all says why, and does not claim a clean network', async () => {
  answer = {
    verdict: 'nothing-sent',
    summary: 'Nib could not send anything on this network. No network connection on this machine ' +
      'accepted an announcement, so the other party cannot possibly hear you.',
    note: 'Nib could not listen on this network at all: no usable interface.',
    sent: 0, own: 0, peers: 0, interfaces: 0, windowMs: 3000,
  };
  const out = await runTest();
  const text = out.textContent;
  assert.ok(text.includes('could not send anything'), 'the verdict sentence is missing');
  assert.ok(text.includes('no usable interface'),
    'the note is dropped. It carries the reason the test could not run at all, and without it a ' +
    'machine with no network reads identically to one whose peers are simply absent.');
  assert.ok(out.classList.contains('nettest-bad'), 'a machine that can send nothing is not a warning');
});
