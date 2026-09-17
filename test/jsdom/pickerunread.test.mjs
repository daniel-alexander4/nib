// /pending 566 — a failed `/api/peers` said "you have not paired with anyone" and then destroyed
// the saved roster on disk.
//
// **Two defects that only matter together, which is why they are driven together here.** The first
// is a sentence: `loadPeerPicker`'s failure left `peers = []` and fell into the empty-list branch,
// so the sheet made a false statement about the user's own data. The second is what happens next:
// the form's `change` listener calls `saveCeremonyDraft`, which reads the roster out of
// `#cerPeerPick`, finds zero rows, and POSTs `roster: []` — and the server REPLACES the stored
// blob, so the roster a convener picked in an earlier session is gone from the vault for every
// later open. One failed fetch plus one keystroke.
//
// **What this tier can see, and why it is the right one.** The whole defect is client state: which
// branch rendered, and what bytes went out on the next `change`. There is no server behaviour in
// it — `handleCeremonyDraft` stores whatever it is handed and is right to. So the assertion that
// matters is the BODY of the POST, which this harness can read because a route function is handed
// the `opts` the app passed to `fetch`.
//
// **What it cannot see** is the sentence being legible next to a picker that is not there, and the
// retry being reachable by keyboard from the recital — tier 3's, and not approximated here.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

const FP = 'a'.repeat(64);

// 'throw' is the door the finding named — `apiFetch` rejecting. '500' is the OTHER door it did not:
// `apiFetch` returns a non-ok response rather than throwing it (see its own note on why a 409 is
// not thrown), so a `/api/peers` that answers with an error reached the same empty branch by a
// route no `catch` covers. Both are driven, because a fix that covered only the named one would
// leave the roster destructible through the leg nobody wrote down.
let peersMode = 'throw';

// A roster the user picked in an EARLIER session, which is the thing at risk. An empty stored draft
// would make every assertion below vacuous: there would be nothing for `roster: []` to destroy.
const storedDraft = JSON.stringify({
  intent: 'We agree to the lease of 14 Elm Row',
  expires: '',
  iSign: true,
  roster: [{ fingerprint: FP, capacity: 'Tenant', picked: true }],
});

// Every POST body the app sent to the draft route. `boot`'s own `calls` records the URL, the method
// and the headers and NOT the body — and the body is the entire finding here, so it is recorded at
// the route instead.
const draftPosts = [];

const { document: doc, window: win, settle, calls } = await boot({
  routes: {
    '/api/peers': () => {
      if (peersMode === 'throw') throw new Error('network');
      if (peersMode === '500') {
        return new Response(JSON.stringify({ error: 'peers unavailable' }), {
          status: 500, headers: { 'Content-Type': 'application/json' },
        });
      }
      return {
        self: 'f'.repeat(64),
        peers: [{ fingerprint: FP, label: 'lively otter marble finch amber cove' }],
      };
    },
    '/api/ceremony/draft': (opts) => {
      if (opts.method === 'POST') {
        draftPosts.push(JSON.parse(JSON.parse(opts.body).draft));
        return { draft: '' };
      }
      return { draft: storedDraft };
    },
    // Stubbed so that a retry button which accidentally SUBMITS its form reaches a route rather
    // than an exception — the assertion below is that it was never called, and a throwing stub
    // would make that test pass for the wrong reason.
    '/api/ceremony/convene': () => ({ ceremony: 'c'.repeat(64), invitations: [] }),
    '/api/ceremonies': () => ({ ceremonies: [] }),
    '/api/scan': { hidden: [] },
  },
});

const pick = () => doc.getElementById('cerPeerPick');
const rows = () => pick().querySelectorAll('.cerpeerrow');

async function openSheet() {
  doc.querySelector('.modetab[data-tab="collaborate"]').click();
  await settle();
  doc.getElementById('ceremonyConveneBtn').click();
  await settle();
}

// `change` and not `input`: the listener is on `change`, which is what a blur fires, and dispatching
// the wrong event would make every save assertion below vacuously green.
async function typeAndBlur(id, value) {
  const el = doc.getElementById(id);
  el.value = value;
  el.dispatchEvent(new win.Event('change', { bubbles: true }));
  await settle();
}

test('a failed peer fetch does not claim the user has paired with nobody', async () => {
  await openSheet();
  const text = pick().textContent;
  assert.ok(!text.includes('not paired with anyone'),
    'a `/api/peers` that FAILED rendered the empty-peer-list sentence. That is a false statement '
    + 'about the user\'s own data — the app does not know how many peers they have, it knows it '
    + 'could not ask — and it is the sentence that makes the next defect invisible, because a '
    + 'convener told they have no peers has no reason to suspect a roster is about to be lost');
  assert.ok(/could not be loaded/.test(text),
    'the failure branch does not say the peers could not be loaded, so the user is left to guess '
    + 'why the picker is empty');
  assert.ok(/is being saved|untouched/.test(text),
    'the sentence never says what the app is DOING about the failure. The draft save is refused '
    + 'while this is on screen, and a refusal nobody is told about trades a silent loss on disk '
    + 'for a silent loss of whatever the convener types next');
});

test('a change after a failed peer fetch does not overwrite the saved roster', async () => {
  draftPosts.length = 0;
  await typeAndBlur('cerIntent', 'We agree to the lease of 14 Elm Row, as amended');
  const emptied = draftPosts.filter((d) => Array.isArray(d.roster) && d.roster.length === 0);
  assert.deepEqual(emptied, [],
    'a `change` event posted a draft carrying `roster: []` while the picker held no answer from '
    + '`/api/peers`. The POST REPLACES the stored blob, so this is the saved roster being '
    + 'destroyed in the vault — permanently, for every later open — by one keystroke after one '
    + 'failed fetch. What was posted: ' + JSON.stringify(emptied));
  assert.equal(draftPosts.length, 0,
    'the draft was written at all while the roster could not be read. Omitting the roster field is '
    + 'not a partial write: `handleCeremonyDraft` REPLACES the blob, and restoreCeremonyDraft '
    + 'reads a missing `roster` exactly as it reads an empty one — so the only write that does not '
    + 'destroy the roster is no write. Posted: ' + JSON.stringify(draftPosts));
});

test('a 500 from the peer route is the same refusal as a thrown one', async () => {
  peersMode = '500';
  doc.getElementById('cerSheetClose').click();
  await settle();
  await openSheet();
  draftPosts.length = 0;
  await typeAndBlur('cerIntent', 'We agree to the lease of 14 Elm Row, as further amended');
  assert.ok(!pick().textContent.includes('not paired with anyone'),
    'a `/api/peers` answering 500 rendered the empty-peer-list sentence. `apiFetch` RETURNS a 500 '
    + 'rather than throwing it, so a guard that only covers the catch leaves this leg destroying '
    + 'the roster exactly as before');
  assert.equal(draftPosts.length, 0,
    'a `change` wrote the draft after `/api/peers` answered 500 — the not-ok leg is a second door '
    + 'to the same destruction, and it is the one no `catch` covers. Posted: '
    + JSON.stringify(draftPosts));
});

test('Check again re-reads the peers and does not convene the ceremony', async () => {
  peersMode = 'ok';
  const again = pick().querySelector('button');
  assert.ok(again, 'the failure branch offers no way out. The picker is the only control that can '
    + 'end this state, and without a retry the user\'s only route is to guess that dismissing and '
    + 'reopening the sheet re-fetches');
  // **The drive FIRST and the attribute second, because only one of them is evidence.**
  // `#cerPeerPick` is inside `<form id="ceremonyConveneForm">`, whose submit handler convenes
  // irreversibly — posts the roster, renders the invitations and consumes the draft — so a
  // default-type button built into this branch turns a retry into a ceremony. index.html keeps the
  // sheet's own two buttons OUTSIDE the form for exactly this reason and says so; a control built
  // into the picker cannot be kept outside it, so the attribute is the whole guard and it has to
  // be checked by pressing the thing.
  //
  // **The spy counts the SUBMIT EVENT and not a `/api/ceremony/convene` call**, and the difference
  // is the difference between a probe that can fail and one that cannot: with the picker in its
  // failure state there are no rows, so `conveneFromPanel` refuses on "choose at least one other
  // person" BEFORE it posts anything. A convene-call count would therefore stay at zero with the
  // button spelled `type="submit"` — green against the defect it was written for. jsdom does run a
  // submit button's activation behaviour (measured), so the event itself is reachable here.
  const form = doc.getElementById('ceremonyConveneForm');
  let submits = 0;
  const spy = (ev) => { submits += 1; ev.preventDefault(); };
  form.addEventListener('submit', spy);
  const before = calls.filter((c) => c.url.includes('/api/ceremony/convene')).length;
  again.click();
  await settle();
  form.removeEventListener('submit', spy);
  assert.equal(submits, 0,
    'pressing Check again submitted the convene form. The submit handler convenes — it fixes the '
    + 'document, posts the roster, renders the invitations and consumes the draft — and none of '
    + 'that is undoable. A retry that can convene is worse than no retry at all');
  assert.equal(calls.filter((c) => c.url.includes('/api/ceremony/convene')).length, before,
    'pressing Check again reached the convene route');
  assert.equal(again.type, 'button',
    'the retry is a submit button inside the convene form. It is only harmless today because the '
    + 'picker it sits in has no rows for `conveneFromPanel` to accept');
  assert.ok(rows().length > 0,
    'Check again did not refill the picker, so the control does nothing and the sentence offering '
    + 'it is a dead end');
});

test('the refusal ends with the fetch that succeeds, and the roster comes back intact', async () => {
  draftPosts.length = 0;
  await typeAndBlur('cerIntent', 'We agree to the lease of 14 Elm Row');
  assert.equal(draftPosts.length, 1,
    'the draft still is not being saved after a SUCCESSFUL peer fetch. The refusal is per-fetch, '
    + 'so a load that answered must clear it — a latch that outlived the failure would turn a '
    + 'transient blip into a sheet that never saves again for the rest of the session');
  assert.deepEqual(draftPosts[0].roster, [{ fingerprint: FP, capacity: 'Tenant', picked: true }],
    'the draft saved after the retry, but the roster it wrote is not the one that was stored. '
    + 'Check again routes through `openCeremonySetup`, which is the picker AND the restore in that '
    + 'order — a retry that rebuilt the picker without the restore behind it would leave every box '
    + 'unticked with the refusal now lifted, and the next change would post the empty roster this '
    + 'whole file exists to stop. Posted: ' + JSON.stringify(draftPosts[0].roster));
});
