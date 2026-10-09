// File, Mark Up, Page Functions and Settings are always in the menu (ADR-110, ADR-112). Dan, 2026-10-09: *"File and Mark Up
// should always be present and not subject to toggling."*
//
// **Its own file because it needs its own boot.** The case is a vault that ALREADY names one of
// them as hidden — written before ADR-110, or by another build — and the only way that list reaches
// the window is the status it boots on; no box can make the request. `modevisibility.test.mjs`
// boots on a vault that hides nothing, and `internal/server/modevisibility_test.go` proves the
// server no longer hands such a list out. This is the window's own half: handed one anyway, it
// hides none of the three, because there would be no box to bring the tab back.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot, BOOT_ROUTES } from './boot.mjs';

let sent = null;
const h = await boot({
  routes: {
    '/api/settings': (opts) => { sent = JSON.parse((opts && opts.body) || '{}'); return { status: 'ok' }; },
    '/api/status': { ...BOOT_ROUTES['/api/status'], hiddenModes: ['file', 'markup', 'edit', 'settings', 'secure'] } },
});
const { document: doc } = h;
await h.settle();

const shown = (el) => !!el && !el.hidden;
const tab = (m) => doc.querySelector(`.modetab[data-tab="${m}"]`);
const jump = (m) => doc.querySelector(`[data-modejump="${m}"]`);

test('a stored list naming File, Mark Up, Page Functions or Settings hides none of them, in either list', () => {
  // The stimulus arrived: the one id in the list that MAY be hidden is hidden.
  assert.equal(shown(tab('secure')), false, 'stimulus: the stored list was not applied at all, so nothing below is about it');
  for (const m of ['file', 'markup', 'edit', 'settings']) {
    assert.ok(shown(tab(m)), `${m} was hidden by a stored list — it is always present, and no box could bring it back`);
    assert.ok(shown(jump(m)), `${m} was hidden in the width-swap dropdown by a stored list`);
  }
});

test('the next change of a box sends a list without them, so the vault is put right', async () => {
  const box = doc.querySelector('.modeChk[data-mode="accessibility"]');
  box.checked = false;
  box.onchange();
  await h.settle();
  assert.ok(sent && Array.isArray(sent.hiddenModes), 'unticking a box sent no hiddenModes list');
  assert.deepEqual([...sent.hiddenModes].sort(), ['accessibility', 'secure'], 'the list sent after a change still names a tab that is always present, or dropped one that was hidden');
});
