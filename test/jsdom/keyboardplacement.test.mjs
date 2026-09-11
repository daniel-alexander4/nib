// Keyboard placement — `PLAN-accessibility.md` P02.S01, WCAG SC 2.1.1 Level A.
//
// ── What only this tier can see ──────────────────────────────────────────────
// The behaviour — press a key, get a mark, arrow it around — is tier 3's: it needs a real
// browser, real page geometry and a real `PointerEvent` reaching real handlers. What this tier
// can see, and tier 3 structurally cannot, is the POPULATION: whether every placement tool the
// code defines is one the keyboard door serves.
//
// ── Why that is the assertion worth having ───────────────────────────────────
// Eleven tools were pointer-only and `grep ArrowUp web/app.js` returned 0. Fixing eleven is a
// day's work; the defect that outlives the fix is the TWELFTH — a new tool added with a
// `pointerdown` handler and no keyboard path, shipping a fresh Level A failure with every
// existing test still green. A list would rot the first time somebody added one. So this
// enumerates `view.*Mode` from the source and requires each to be accounted for, which is
// P01.S03's census one file over.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const APP = readFileSync(new URL('../../web/app.js', import.meta.url), 'utf8');

// The flags that are NOT placement tools, each with the reason it is exempt. A bare exclusion
// list is how a real tool gets quietly dropped, so every entry states what it is instead.
const NOT_A_PLACEMENT_TOOL = {
  fitMode: 'a zoom/fit setting on the view (actualSize / fitPage / fitWidth), not a tool that draws',
};

function declaredTools() {
  const m = APP.match(/const PLACEMENT_TOOLS = \[([\s\S]*?)\];/);
  assert.ok(m, 'PLACEMENT_TOOLS is gone or reshaped — the keyboard door has no population and ' +
    'every assertion below would pass over an empty set');
  return (m[1].match(/'([a-zA-Z]+)'/g) || []).map((s) => s.replace(/'/g, ''));
}

function modeFlagsInSource() {
  return [...new Set((APP.match(/\bview\.([a-zA-Z]+Mode)\b/g) || [])
    .map((s) => s.slice('view.'.length)))].sort();
}

test('every placement tool the code defines has a keyboard path', () => {
  const flags = modeFlagsInSource();
  // Stimulus first: a scan that finds nothing passes everything.
  assert.ok(flags.length >= 10,
    `only ${flags.length} view.*Mode flag(s) found in web/app.js — the scan has stopped matching ` +
    'and every assertion below is vacuous');

  const tools = declaredTools();
  assert.ok(tools.length >= 10,
    `PLACEMENT_TOOLS holds ${tools.length} name(s) — too few to be the tool set, so this guard ` +
    'would pass while most tools stayed pointer-only');

  const served = new Set(tools.map((t) => `${t}Mode`));
  const unserved = flags.filter((f) => !served.has(f) && !NOT_A_PLACEMENT_TOOL[f]);
  assert.deepEqual(unserved, [],
    `these placement modes have no keyboard path: ${unserved.join(', ')}.\n` +
    'Every one is a WCAG 2.1.1 Level A failure — a tool that can only be used with a pointer. ' +
    'Add the tool to PLACEMENT_TOOLS, or name the flag in NOT_A_PLACEMENT_TOOL with what it is ' +
    'instead.');
});

test('the exemption list names only flags that exist', () => {
  // The other direction: an exemption for a flag that is gone is a claim about code that is not
  // there, and it silently widens the hole the next time a flag takes that name.
  const flags = new Set(modeFlagsInSource());
  const stale = Object.keys(NOT_A_PLACEMENT_TOOL).filter((f) => !flags.has(f));
  assert.deepEqual(stale, [],
    `these flags are exempted but no longer exist: ${stale.join(', ')}`);
});

test('every declared tool is a flag the view record actually has', () => {
  // And the third direction: a tool name with no matching flag means `armedTool()` tests a
  // binding that is always undefined, so that tool silently never arms from the keyboard.
  const flags = new Set(modeFlagsInSource());
  const phantom = declaredTools().filter((t) => !flags.has(`${t}Mode`));
  assert.deepEqual(phantom, [],
    `these tools are declared but have no view.<tool>Mode flag: ${phantom.join(', ')} — ` +
    '`armedTool()` would read undefined for each and they would never arm');
});

test('the keyboard door exists and is bound to a key', () => {
  // The population guard above is satisfied by a door that is never called. This is the
  // stimulus check for it: the handler exists, and something dispatches to it.
  assert.match(APP, /function placeFromKeyboard\(\)/, 'placeFromKeyboard is gone');
  assert.match(APP, /function nudgeFocusedOverlay\(e\)/, 'nudgeFocusedOverlay is gone');
  assert.match(APP, /nudgeFocusedOverlay\(e\)\)\s*\{\s*e\.preventDefault\(\)/,
    'nothing calls nudgeFocusedOverlay from a key handler — the arrow keys do nothing');
  assert.match(APP, /'Enter'.*armedTool\(\) && placeFromKeyboard\(\)/s,
    'nothing calls placeFromKeyboard from a key handler — no tool can be used without a pointer');
});

test('every overlay is focusable through one door, not eleven constructors', () => {
  // Arrow-nudge keys on document.activeElement, so an overlay that cannot take focus cannot be
  // moved. `layoutField` is the single door every overlay kind passes through.
  const m = APP.match(/function layoutField\(f, pv\) \{([\s\S]{0,900})/);
  assert.ok(m, 'layoutField is gone or reshaped');
  assert.match(m[1], /f\.el\.tabIndex !== 0\) f\.el\.tabIndex = 0/,
    'layoutField no longer makes overlays focusable — arrow-nudge has nothing to act on, and ' +
    'only stamps and markers set tabIndex at construction');
});
