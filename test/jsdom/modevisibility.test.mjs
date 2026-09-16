// The main-menu visibility switch (ADR-036). Dan, 2026-09-16: *"each main menu item is
// enable/disable-able in advanced settings. If an item is disabled in the advanced settings, it
// should not show in the main menu."*
//
// ── The half this tier owns ──────────────────────────────────────────────────
// **This file proves the HIDING, and here that is the whole of the feature.** The switch is
// presentational by design (ADR-036): there is no door to stop, because hiding a tab stops
// nothing. `internal/server/modevisibility_test.go` proves the setting is stored, refused and
// handed back; everything a user can SEE is below.
//
// ── The collision this file is the guard for ─────────────────────────────────
// Cutting at mode granularity is exactly what `advanced.test.mjs` forbids for the FEATURE switch —
// hiding the Signing tab when ceremonies are off would take co-signing and Simple Sign with it.
// The two mechanisms must stay separate, so this file asserts that hiding a mode here does NOT
// move any advanced surface, and `advanced.test.mjs` asserts the reverse.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

let saved = null;
let refuse = null;

const h = await boot({
  routes: {
    '/api/settings': (opts) => {
      saved = JSON.parse((opts && opts.body) || '{}');
      if (refuse) return new Response(JSON.stringify({ error: refuse }), { status: 400, headers: { 'Content-Type': 'application/json' } });
      return { status: 'ok' };
    },
  },
});
const { document: doc, settle } = h;

const tab = (m) => doc.querySelector(`.modetab[data-tab="${m}"]`);
const jump = (m) => doc.querySelector(`[data-modejump="${m}"]`);
const box = (m) => doc.querySelector(`.modeChk[data-mode="${m}"]`);
const shown = (el) => !!el && !el.hidden;

test('every mode shows by default, and Settings has no box at all', () => {
  for (const m of ['file', 'markup', 'edit', 'accessibility', 'secure', 'collaborate', 'settings']) {
    assert.ok(shown(tab(m)), `${m} is hidden on a vault that has never been asked — every mode shows by default`);
    assert.ok(shown(jump(m)), `${m} is missing from the width-swap dropdown, so below 719px it is unreachable`);
  }
  // The exemption, and it is the one that cannot be recovered from: the card that un-hides a mode
  // lives inside Settings.
  assert.equal(box('settings'), null,
    'Settings has a visibility box — unticking it would hide the mode that holds the switch, and the '
    + 'only way back would be editing the vault');
  for (const m of ['file', 'markup', 'edit', 'accessibility', 'secure', 'collaborate']) {
    assert.ok(box(m), `${m} has no box in the Main menu card, so it cannot be switched off at all`);
    assert.equal(box(m).checked, true, `${m}'s box is unticked while its tab is showing`);
  }
});

test('unticking a mode hides it in BOTH lists, and sends the whole set', async () => {
  saved = null;
  box('secure').checked = false;
  box('secure').onchange();
  await settle();

  assert.ok(saved && Array.isArray(saved.hiddenModes), 'unticking a box sent no hiddenModes list');
  assert.deepEqual(saved.hiddenModes, ['secure'],
    'the request did not carry the whole set. A partial update cannot express "configured, and '
    + 'nothing is hidden" — an absent field is what a client sends when it is changing something else');

  assert.equal(shown(tab('secure')), false, 'the mode tab is still in the menu after being switched off');
  // **Both lists, and this is the half nothing else can see.** `modes.test.mjs` compares list
  // MEMBERSHIP, and both lists still hold every id — so a mode hidden in the strip and left in the
  // dropdown passes every other guard while being reachable at one width and not the other.
  assert.equal(shown(jump('secure')), false,
    'the mode is hidden in the tab strip and still in the width-swap dropdown, so below 719px it is '
    + 'still reachable — the two lists are cut by one door precisely so this cannot happen');
  assert.ok(shown(tab('file')), 'hiding Secure took another mode with it');
});

test('the mode showing is never one that was just hidden', async () => {
  doc.querySelector('.modetab[data-tab="markup"]').click();
  await settle();
  assert.equal(doc.body.dataset.tab, 'markup', 'setup: the click did not change mode');

  box('markup').checked = false;
  box('markup').onchange();
  await settle();

  assert.notEqual(doc.body.dataset.tab, 'markup',
    'the app is still showing the mode that was just hidden — the pane is on screen with no tab '
    + 'above it, which is the same silent shape a missing SIDEBAR_FOR entry produces');
  assert.ok(shown(tab(doc.body.dataset.tab)), 'the app landed on a mode that is itself hidden');

  box('markup').checked = true;
  box('markup').onchange();
  await settle();
});

test('a jump into a hidden mode lands somewhere visible instead', async () => {
  box('accessibility').checked = false;
  box('accessibility').onchange();
  await settle();

  // setMode is the one door every cross-mode jump goes through — goCard/goPanel, the marker-fill
  // path, the ceremony resume — and it is module-scope in app.js, so it cannot be called from here.
  // The dropdown entry is wired straight to it (`[data-modejump]` → setMode), and jsdom dispatches
  // a click to a hidden element, so this drives the real door rather than a stand-in for it.
  doc.querySelector('[data-modejump="accessibility"]').click();
  await settle();

  assert.notEqual(doc.body.dataset.tab, 'accessibility',
    'a jump reached a hidden mode. Guarding at setMode rather than at each caller is ADR-009: a '
    + 'caller added later inherits the rule');

  box('accessibility').checked = true;
  box('accessibility').onchange();
  await settle();
});

test('hiding a mode moves no advanced surface, and a refusal puts the box back', async () => {
  // The two mechanisms are separate (ADR-036). Hiding Signing must not touch what the ceremony
  // switch governs, or the collision advanced.test.mjs guards has been reintroduced from the
  // other side.
  const ceremonyPanelBefore = shown(doc.getElementById('ceremony'));
  box('collaborate').checked = false;
  box('collaborate').onchange();
  await settle();
  assert.equal(shown(doc.getElementById('ceremony')), ceremonyPanelBefore,
    'hiding the Signing TAB moved the ceremony panel — that surface belongs to the advanced-features '
    + 'switch, and one control now means two things');

  refuse = 'invalid hiddenModes: nosuchmode';
  box('edit').checked = false;
  box('edit').onchange();
  await settle();
  assert.equal(box('edit').checked, true,
    'the box stayed unticked after the server refused — it now reports a setting this machine does '
    + 'not hold');
  assert.ok(shown(tab('edit')), 'the tab was hidden on a change the server refused');
  const err = doc.getElementById('modeError');
  assert.ok(err && !err.hidden, 'the refusal was silent; the user sees a box spring back and no reason');
  refuse = null;
});
