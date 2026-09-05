// The five modes agree with themselves — every mode has a tab, a dropdown entry, a toolbar
// pane and a sidebar panel list.
//
// ── Why this guard exists ────────────────────────────────────────────────────
// Both halves fail SILENTLY, which is the only reason they are worth a test:
//
//   - **A mode missing from `SIDEBAR_FOR`.** `syncSidebarForMode` reads
//     `SIDEBAR_FOR[tab] || []` (web/app.js), hides every sidebar nav button because none is
//     in the empty list, and then never runs its re-activate branch because `panels.length`
//     is 0 — so the PREVIOUS mode's panel stays on screen with no tab above it. No throw, no
//     console warning, and no other test notices.
//   - **A mode missing from the `[data-modejump]` dropdown.** Below 694px `.modetabs` is
//     `display: none` (style.css), so the mode becomes simply unreachable at small widths.
//     Nothing else asserts that the two lists hold the same ids.
//
// Both were found while re-cutting the modes in v1.120.0, when `sign` became `markup` and
// three lists had to change together. Two of them would have been silent if missed.
//
// ── What this tier cannot reach ──────────────────────────────────────────────
// Whether the panels LOOK right, or whether the tab strip still fits — that is geometry, and
// jsdom has no layout engine. `test/ui/responsive.test.mjs` owns it.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { boot } from './boot.mjs';
import { REPO } from './boot.mjs';

const h = await boot({});
const doc = h.document;
const APP = fs.readFileSync(path.join(REPO, 'web', 'app.js'), 'utf8');

const idsOf = (sel, attr) => [...doc.querySelectorAll(sel)].map((e) => e.getAttribute(attr));

// SIDEBAR_FOR is module-scope in app.js and not exposed, so it is read from the source the
// same way doccontrols.test.mjs reads DOC_REQUIRED.
function sidebarForKeys() {
  const m = APP.match(/const SIDEBAR_FOR = \{([\s\S]*?)\n\};/);
  assert.ok(m, 'SIDEBAR_FOR is not in web/app.js in the shape this scan reads — the guard is reading nothing');
  return [...m[1].matchAll(/^\s*([a-z]+):/gm)].map((x) => x[1]);
}

test('every mode has a tab, a dropdown entry and a toolbar pane', () => {
  const tabs = idsOf('.modetab', 'data-tab');
  const jumps = idsOf('[data-modejump]', 'data-modejump');
  const panes = idsOf('#toolbar .tbtab', 'data-tab');
  assert.ok(tabs.length >= 5, `only ${tabs.length} mode tabs found — this guard is reading nothing`);

  assert.deepEqual(jumps, tabs,
    `the mode dropdown and the mode tabs list different modes.\n  tabs:  ${tabs.join(', ')}\n  jumps: ${jumps.join(', ')}\nBelow 694px the tabs are display:none, so a mode missing from the dropdown is simply unreachable — silently.`);
  assert.deepEqual([...panes].sort(), [...tabs].sort(),
    `these modes have a tab but no toolbar pane, or the reverse.\n  tabs:  ${tabs.join(', ')}\n  panes: ${panes.join(', ')}`);
});

test('every mode has a sidebar panel list, and every panel it names exists', () => {
  const tabs = idsOf('.modetab', 'data-tab');
  const keys = sidebarForKeys();
  const missing = tabs.filter((t) => !keys.includes(t));
  assert.deepEqual(missing, [],
    `${missing.join(', ')} have no SIDEBAR_FOR entry. syncSidebarForMode falls back to [], which hides every sidebar tab AND skips its own re-activate branch — so the previous mode's panel stays on screen with no tab above it, silently.`);

  const stray = keys.filter((k) => !tabs.includes(k));
  assert.deepEqual(stray, [],
    `SIDEBAR_FOR names ${stray.join(', ')}, which is not a mode. A stale key is dead configuration that reads as coverage.`);

  const panels = new Set([...doc.querySelectorAll('.tabs .tab')].map((t) => t.dataset.panel));
  const m = APP.match(/const SIDEBAR_FOR = \{([\s\S]*?)\n\};/);
  const named = [...m[1].matchAll(/'([a-z]+)'/g)].map((x) => x[1]);
  const unknown = [...new Set(named)].filter((p) => !panels.has(p));
  assert.deepEqual(unknown, [],
    `SIDEBAR_FOR points at ${unknown.join(', ')}, which no .tabs .tab declares — that mode would land on nothing`);
});
