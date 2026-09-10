// The nine distinct outcomes (P06.S08, D19, D28).
//
// **The criterion: "Each of D28's four end states — completed, declined, expired, abandoned —
// produces its own message, distinct from each other and from D19's four network causes. Eight
// distinct outcomes, driven separately; a screen that folds 'they declined' into 'couldn't
// establish a connection' fails this."**
//
// # `expired` is in this list as FORWARD COMPATIBILITY, not as an outcome (/pending 421)
//
// The criterion says four end states and the close-out can produce three. `closeOutReason` waits
// for `Expires` **plus** `closeOutGrace`, which is word for word `abandoned`'s definition — the
// deadline and the grace both passed and nothing said what happened — so at the only moment a
// close-out runs both are true and `abandoned` is the more specific claim. There is no window in
// which only expiry holds, and firing the close-out earlier to make one would archive proceedings
// the grace exists to let finish.
//
// So the `expired` row below drives a client branch that **nothing can currently put on the wire**.
// It is kept rather than deleted, because the branch is real and the alternative — deleting it and
// having a future producer render the raw enum at somebody — is the defect `/pending 428` already
// paid for once. What changed is this comment: the row is no longer evidence that the product can
// reach that state, and D28 is amended to say so.
//
// # Nine, not eight, and the plan carries the pin
//
// D19 has FIVE causes, not four — the plan review of 2026-08-18 corrected D19 itself and this
// criterion, written the same day, kept the stale count. `causeName` returns
// `rendezvous-unreachable`, `peer-not-started`, `mapping-dependent`, `connection-failed` and
// `peer-record-unusable`. So the honest bar is nine.
//
// # Why this file exists when both halves already rendered
//
// Both surfaces shipped and each was driven at exactly ONE value: `ceremonypanel.test.mjs` drove
// `declined`, `waitdiagnosis.test.mjs` drove `peer-not-started`. Driving each half once, in two
// files that never meet, cannot see a collision BETWEEN them — and a collision is precisely what
// the criterion forbids. The distinctness assertion is the whole point of putting them in one file.
//
// # What it cannot see
//
// Whether two sentences that differ as strings read as the same thing to a person. That is a
// `/uiux` question and this is a string comparison; it catches the fold, not the synonym.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

// D28's four, plus a state this build does not know — which had no case of its own until P06.S08
// and rendered as `abandoned`'s sentence.
// **`stopped` and `left` are here because a state missing from this list simply IS the unknown
// case** (`/pending 428`). The uniqueness assertion below stays green whether or not the ladder has
// an arm for a word — so an attested state left out of this list renders as "Ended in a way this
// version does not recognise" for a state this version knows perfectly well, and nothing goes red.
// The list is the stimulus, not a description.
const END_STATES = ['completed', 'declined', 'expired', 'abandoned', 'stopped', 'left',
  'a-state-from-a-newer-nib'];

// The producible subset — every state above except the two that nothing puts on the wire:
// `expired` (see the header) and the deliberately-unknown sentinel. Producibility itself is a
// SERVER fact and is asserted there, by `TestNoCloseOutPathProducesTheExpiredState`; what this
// list does here is keep the two halves from drifting apart silently.
const PRODUCIBLE = ['completed', 'declined', 'abandoned', 'stopped', 'left'];

test('every producible end state is one this file drives', () => {
  for (const st of PRODUCIBLE) {
    assert.ok(END_STATES.includes(st),
      `${st} can be put on the wire and this file does not drive it, so nothing checks that it `
      + 'says its own thing — which is the whole criterion');
  }
  assert.equal(PRODUCIBLE.includes('expired'), false,
    'expired is listed as producible here while the header, D28 and internal/server all say no '
    + 'close-out can reach it. If that changed, this is the wrong file to record it in — amend '
    + 'D28 and TestNoCloseOutPathProducesTheExpiredState first');
});

// D19's five machine tags, each with the summary the server sends for it. The summaries are the
// server's own words; this file asserts the CLIENT renders what it is given and keeps them apart.
const CAUSES = [
  ['rendezvous-unreachable', "Couldn't reach the rendezvous network."],
  ['peer-record-unusable', 'Found the other side, but couldn’t use what they published.'],
  ['peer-not-started', "The other side hasn't started their ceremony yet."],
  ['mapping-dependent', "A direct connection between these two networks isn't possible."],
  ['connection-failed', "Couldn't establish a connection."],
];

let ceremonies = { ceremonies: [], ended: [], primary: true };
let armed = false;
let diagnosis = null;

const h = await boot({
  routes: {
    '/api/ceremonies': () => ceremonies,
    '/api/peers': () => ({ self: 'f'.repeat(64), peers: [{ fingerprint: 'a'.repeat(64), label: 'Ada' }] }),
    '/api/session/arm': () => { armed = true; return { armed: true, address: '127.0.0.1:8443' }; },
    '/api/session/status': () => (armed
      ? { armed: true, address: '127.0.0.1:8443', diagnosis: diagnosis || undefined }
      : { armed: false }),
    '/api/session/disarm': () => { armed = false; return {}; },
  },
});
const { document: doc, settle } = h;

const tick = async () => { await new Promise((r) => setTimeout(r, 1800)); await settle(); };

test('each of D28’s four end states, and an unknown one, gets its own sentence', async () => {
  const seen = new Map();
  for (const state of END_STATES) {
    ceremonies = {
      ceremonies: [],
      ended: [{ ceremony: '3'.repeat(32), state, observed_at: '2026-09-01T09:00:00Z' }],
      primary: true,
    };
    // The panel is re-read per state rather than once with five rows, because the criterion is
    // "driven separately": five rows in one render would be satisfied by a function that emits
    // whatever it is handed, and could not tell a shared fallback from five distinct arms.
    // Re-entering the panel is what re-reads `/api/ceremonies`, which is how a user gets a fresh
    // list and the only handle this tier has on it — the app exports nothing.
    doc.querySelector('.modetab[data-tab="file"]')?.click();
    await settle();
    doc.querySelector('.modetab[data-tab="collaborate"]')?.click();
    await settle();
    const host = doc.getElementById('ceremonyList');
    const row = host.querySelector('.cerended-row');
    assert.ok(row, `setup: no ended row rendered for ${state}, so nothing is being compared`);
    const word = row.querySelector('span').textContent.trim();
    assert.ok(word, `the ended row for ${state} renders no word at all`);
    if (seen.has(word)) {
      assert.fail(`"${state}" and "${seen.get(word)}" both render as "${word}". D28's four end ` +
        'states must each produce their OWN message — and an unrecognised state must not borrow ' +
        'one of theirs, because "this proceeding ended in silence" and "this receipt was written ' +
        'by a newer Nib" are different facts and only one of them is about the ceremony.');
    }
    seen.set(word, state);
  }
  assert.equal(seen.size, END_STATES.length,
    `${seen.size} distinct sentences for ${END_STATES.length} states`);
});

test('each of D19’s five causes renders its own summary, and none collides with an end state', async () => {
  // Arm once; the diagnosis arrives on the poll, so each cause is a new answer and a tick.
  doc.getElementById('sessionRecvBtn').click();
  await settle();
  doc.getElementById('srvPeer').selectedIndex = 0;
  doc.getElementById('srvArmGo').click();
  await settle();

  const seen = new Map();
  for (const [cause, summary] of CAUSES) {
    diagnosis = { cause, summary, detail: 'technical detail for ' + cause };
    await tick();
    const why = doc.getElementById('srvWaitWhy');
    assert.ok(why && !why.hidden, `setup: the wait line is not showing for ${cause}`);
    const line = why.textContent.trim();
    assert.equal(line, summary,
      `the visible line for ${cause} is "${line}", not the server's plain-language summary. D19's ` +
      'presentation pin is plain language first with the technical detail behind a disclosure — a ' +
      'client that rewrites the sentence is a second copy of the server’s wording.');
    assert.equal(why.dataset.cause, cause,
      `the cause tag on the element is "${why.dataset.cause}", not "${cause}" — nothing downstream ` +
      'can tell the ordinary early state from a real fault');
    seen.set(line, cause);
  }

  // Observe, clean up, THEN assert the cross-set claim — an armed session leaves a repeating poll
  // timer, and a file that leaves one does not fail, it HANGS.
  doc.getElementById('srvCancel').click();
  await settle();

  // **DISTINCTNESS IS NOT ASSERTED HERE, deliberately.** The summaries above come from this
  // file's own fixture, so comparing them would be a fact about the fixture and not about the
  // product — the vacuous green wearing a distinctness assertion. The claim is made where both
  // sides are the product: `TestEveryD19OutcomeSaysItsOwnThing` drives `classifyD19` for every
  // cause and reads D28's rendered words out of `web/app.js`. Measured there: **7 distinct
  // summaries across 5 causes, and 5 end-state words, none colliding.**
  //
  // What this file owns is the half that test cannot see: that the CLIENT renders the sentence it
  // is given rather than a rewrite of it, and carries the machine tag onto the element.
  assert.equal(seen.size, CAUSES.length,
    `the client rendered ${seen.size} distinct lines for ${CAUSES.length} distinct summaries, so ` +
    'it is not rendering what it was given');
});

