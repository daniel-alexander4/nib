// /pending 793 — the spoken-check answer names the words it was given for.
//
// The answer was `{confirmed}` and nothing else, so the server resolved whichever check was parked
// when it landed. And the receive poller drew a check only while the card was HIDDEN: a first check
// that timed out with its card up, followed by a second, left the first one's words on screen over
// the second one's check. The server half (the refusal) is `TestASpokenCheckAnswerIsForTheWordsItNames`.
//
// What this tier cannot see: the words as a person reads them. It asserts the card's text and the
// request body.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

const PEER = { fingerprint: 'a'.repeat(64), label: 'Ada' };
const FIRST = 'amber birch cobalt dune';
const SECOND = 'ember flint grove harbour';

let armed = false;
let words = null;
const answers = [];

const { document: doc, settle } = await boot({
  routes: {
    '/api/peers': () => ({ self: 'b'.repeat(64), peers: [PEER] }),
    '/api/session/arm': () => { armed = true; return { armed: true, address: '127.0.0.1:8443' }; },
    '/api/session/status': () => ({ armed, address: '127.0.0.1:8443', ...(words ? { verify: { words } } : {}) }),
    '/api/session/verify': (opts) => { answers.push(JSON.parse(opts.body)); words = null; return { ok: true }; },
    '/api/session/disarm': () => { armed = false; return {}; },
  },
});

// One poll interval plus a margin — recvpoll.test.mjs measured it.
const tick = async () => { await new Promise((r) => setTimeout(r, 1800)); await settle(); };

// The poll is a real timer and ends only when the arm does; left running after a failed assertion
// it holds the file open, and the failure reads as a hang.
test.after(async () => { armed = false; await tick(); });

test('the card shows the check that is waiting, and the answer names its words', async () => {
  doc.getElementById('sessionRecvBtn').click();
  await settle();
  doc.getElementById('srvPeer').selectedIndex = 0;
  doc.getElementById('srvArmGo').click();
  await settle();

  words = FIRST;
  await tick();
  assert.equal(doc.getElementById('verifyModal').hidden, false, 'setup: the spoken check never went up');
  assert.equal(doc.getElementById('verifyWords').textContent, FIRST, 'setup: the card shows other words');

  // The first check goes away and a second parks between two polls: the card never hides.
  words = SECOND;
  await tick();
  assert.equal(doc.getElementById('verifyWords').textContent, SECOND,
    'the card still shows the FIRST check\'s words while the server is waiting on the second — '
    + 'a press now answers words nobody compared');

  doc.getElementById('verifyConfirm').click();
  await settle();
  assert.equal(answers.length, 1, 'setup: the answer was never sent');
  assert.deepEqual(answers[0], { confirmed: true, words: SECOND },
    'the answer does not name the words on screen, so the server cannot tell which check it is for');
});
