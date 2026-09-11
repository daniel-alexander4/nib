// Advanced features, at the surface (`/pending 451`). Dan, 2026-09-10: *"default off. menus hidden."*
//
// ── The half this tier owns, and the half it must not claim ──────────────────
// **This file proves the HIDING and nothing else.** The Go tests prove the switch stops the
// function — the announcer's socket, the DHT bootstrap, the arm sweep, the two timestamp routes —
// and that half is the one that matters: a toggle which only hid UI would be a lie the product
// tells, and this repo has shipped that shape twice (`/pending 378`, `/pending 3`). So nothing here
// asserts that anything stopped; it asserts that what is offered matches what the machine will do.
//
// ── The collision this file is the guard for ─────────────────────────────────
// "Off ⇒ hidden" cuts at PANEL granularity, never at the mode. The ceremony lives inside the
// `collaborate` mode, which also carries co-signing and Simple Sign — hiding the mode tab would
// take ordinary two-party signing with it. That is the failure an item-by-item design round is
// structurally blind to, and it is asserted below.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

let saved = null;
let refuse = null;

const h = await boot({
  routes: {
    // **Its own status, with the features OFF.** boot.mjs's default has them on so that every
    // other file here is about its own subject rather than about this switch; this file is about
    // the switch, so it states the default explicitly. One place where "off is the default" is
    // written down and driven, rather than an emergent property of a shared fixture.
    '/api/status': {
      state: 'ready', csrf: 'test-csrf', version: 'test',
      autoUpdate: false, updateCheckLocked: false, ghostscript: false, libreoffice: false,
    },
    '/api/settings': (opts) => {
      saved = JSON.parse((opts && opts.body) || '{}');
      if (refuse) return new Response(JSON.stringify({ error: refuse }), { status: 409, headers: { 'Content-Type': 'application/json' } });
      return { status: 'ok' };
    },
  },
});
const { document: doc, settle } = h;

const shown = (sel) => { const el = doc.querySelector(sel); return !!el && !el.hidden; };
const box = (id) => doc.getElementById(id);

test('the default is off, and the card says so with four unticked boxes', () => {
  for (const id of ['advCeremonyChk', 'advDiscoveryChk', 'advRendezvousChk', 'advTimestampChk']) {
    assert.ok(box(id), `${id} is missing — the switch has no surface at all`);
    assert.equal(box(id).checked, false,
      `${id} is ticked on a machine whose vault holds no advanced settings. Off is the default, `
      + 'and a box that says otherwise is the one place a user checks before trusting it');
  }
});

test('a feature that is off is not offered', () => {
  assert.equal(shown('#ceremony'), false,
    'the Signing Ceremonies panel is offered while ceremonies are switched off — the product is '
    + 'inviting a user into a flow the server will refuse with 403');
  assert.equal(shown('#timestampBtn'), false, 'Timestamp is offered while timestamping is off');
  assert.equal(shown('#timestampVerifyBtn'), false, 'Verify a timestamp is offered while it is off');
});

test('hiding the ceremony does NOT take ordinary co-signing with it', () => {
  // The collision. `collaborate` is a whole mode and the ceremony is one panel inside it; cutting
  // at the mode would remove Simple Sign and Send & Receive, which are not the ceremony.
  // **`shown`, not `querySelector`.** The first cut asserted these elements EXIST, and
  // `querySelector` finds hidden ones — so a mutation that cut at mode granularity, hiding the
  // Signing tab and both co-sign buttons, left this test entirely green. Existence was never the
  // claim; being offered is.
  assert.equal(shown('.modetab[data-tab="collaborate"]'), true,
    'the Signing MODE TAB was hidden along with the ceremony panel. Co-signing, Simple Sign and '
    + 'Send & Receive live in that mode and are not the feature that was switched off');
  assert.equal(shown('#cosignBtn'), true, 'Co-sign with a peer went with the ceremony');
  assert.equal(shown('#sessionInitBtn'), true, 'Co-sign live with a peer went with the ceremony');
});

test('switching one on sends all four, and reveals its surface', async () => {
  saved = null;
  box('advCeremonyChk').checked = true;
  box('advCeremonyChk').onchange();
  await settle();
  assert.ok(saved && saved.advanced, 'ticking a box sent no advanced object');
  assert.deepEqual(saved.advanced, { ceremony: true, discovery: false, rendezvous: false, timestamp: false },
    'the request did not carry all four. A partial update cannot express "configured, all off" — '
    + 'four absent fields is what a client sends when it is changing something else entirely');
  assert.equal(shown('#ceremony'), true,
    'the ceremony panel is still hidden after switching ceremonies on, so the only way to reach '
    + 'the feature is a restart');
});

test('a refusal puts the box back, rather than showing a state the machine is not in', async () => {
  // The server refuses switching ceremonies off while a proceeding is live (409). Echoing the
  // REQUEST would leave the box saying off while the feature is on — the control and the thing it
  // controls disagreeing, which is worse than either state.
  refuse = 'a signing ceremony on this machine has not finished';
  box('advCeremonyChk').checked = false;
  box('advCeremonyChk').onchange();
  await settle();
  assert.equal(box('advCeremonyChk').checked, true,
    'the box stayed unticked after the server refused the change — it now reports a setting this '
    + 'machine does not hold, and the feature is still running');
  assert.equal(shown('#ceremony'), true, 'the panel was hidden on a change the server refused');
  const err = doc.getElementById('advError');
  assert.ok(err && !err.hidden, 'the refusal was silent; the user sees a box spring back and no reason');
  assert.match(err.textContent, /not finished/,
    `the error reads ${JSON.stringify(err.textContent)} and does not say what is running`);
  refuse = null;
});
