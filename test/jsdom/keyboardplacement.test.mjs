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

// **Comments are stripped before anything here reads the source, and that is /pending 553.** This
// file's scan is `view.<name>Mode` out of app.js, and prose about the tools is written in exactly
// that vocabulary — so a mode NAMED in a comment read as a twelfth pointer-only tool and turned the
// population guard red over a sentence. It cost /pending 513 one false red (`xMode`), and the
// workaround was a comment in app.js telling the next author not to spell a mode out, which is a
// rule enforced by a sentence. `pinning.test.mjs` has had this strip since its own probe found a
// pin satisfied by a comment; this is the same helper, one file over.
//
// The URL guard (`(?<!:)`) keeps `https://` out of it. Measured when this landed: the twelve flags
// found are byte-identical before and after the strip, and so is every other assertion below —
// including `layoutField`'s fixed 900-character window, which a strip can only widen.
const stripComments = (s) => s
  .replace(/\/\*[\s\S]*?\*\//g, ' ')   // block comments
  .replace(/(?<!:)\/\/[^\n]*/g, '');      // line comments, not URLs

// CODE is what every assertion in this file reads. `APP` stays available for the one test that is
// ABOUT the difference between them.
const CODE = stripComments(APP);

// The flags that are NOT placement tools, each with the reason it is exempt. A bare exclusion
// list is how a real tool gets quietly dropped, so every entry states what it is instead.
const NOT_A_PLACEMENT_TOOL = {
  fitMode: 'a zoom/fit setting on the view (actualSize / fitPage / fitWidth), not a tool that draws',
};

function declaredTools() {
  const m = CODE.match(/const PLACEMENT_TOOLS = \[([\s\S]*?)\];/);
  assert.ok(m, 'PLACEMENT_TOOLS is gone or reshaped — the keyboard door has no population and ' +
    'every assertion below would pass over an empty set');
  return (m[1].match(/'([a-zA-Z]+)'/g) || []).map((s) => s.replace(/'/g, ''));
}

// Parameterised on the source it reads, so the strip itself can be put in front of a fixture
// rather than only in front of app.js — a scan that can only be run on the real file cannot be
// shown to reject anything.
function modeFlagsIn(src) {
  return [...new Set((src.match(/\bview\.([a-zA-Z]+Mode)\b/g) || [])
    .map((s) => s.slice('view.'.length)))].sort();
}

function modeFlagsInSource() { return modeFlagsIn(CODE); }

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
  assert.match(CODE, /function placeFromKeyboard\(\)/, 'placeFromKeyboard is gone');
  assert.match(CODE, /function nudgeFocusedOverlay\(e\)/, 'nudgeFocusedOverlay is gone');
  assert.match(CODE, /nudgeFocusedOverlay\(e\)\)\s*\{\s*e\.preventDefault\(\)/,
    'nothing calls nudgeFocusedOverlay from a key handler — the arrow keys do nothing');
  assert.match(CODE, /'Enter'.*armedTool\(\) && placeFromKeyboard\(\)/s,
    'nothing calls placeFromKeyboard from a key handler — no tool can be used without a pointer');
});

test('every overlay is focusable through one door, not eleven constructors', () => {
  // Arrow-nudge keys on document.activeElement, so an overlay that cannot take focus cannot be
  // moved. `layoutField` is the single door every overlay kind passes through.
  const m = CODE.match(/function layoutField\(f, pv\) \{([\s\S]{0,900})/);
  assert.ok(m, 'layoutField is gone or reshaped');
  assert.match(m[1], /f\.el\.tabIndex !== 0\) f\.el\.tabIndex = 0/,
    'layoutField no longer makes overlays focusable — arrow-nudge has nothing to act on, and ' +
    'only stamps and markers set tabIndex at construction');
});

// ── /pending 553 — prose is not a tool ────────────────────────────────────────
//
// The guard above is a scan over source text, and the thing a text scan gets wrong is the text that
// is not code. Both halves are asserted here because they fail differently: the STRIP can stop
// working, and the FIXTURE that proves it can be deleted by someone tidying a comment. Either one
// alone goes green while the other is broken.
test('a mode named in a comment is prose, not a twelfth pointer-only tool', () => {
  // Half one: the strip, against a fixture. Three shapes, because the line-comment and block-comment
  // cases are different regexes and the URL case is the exception written into one of them.
  const fixture = [
    "// every handler takes its own `if (view.xMode) { view.xMode = false; }` toggle first",
    '/* view.ghostMode is named here and is not a tool */',
    "const wat = view.noteMode;              // a real read, with view.proseMode in its comment",
    "const href = 'https://nib.example/a//b'; // a URL the strip must not eat",
  ].join('\n');
  assert.deepEqual(modeFlagsIn(stripComments(fixture)), ['noteMode'],
    'a view.<x>Mode written in a comment reached the population. Every one is then an unserved '
    + 'flag with no keyboard path, so this guard goes red over a sentence somebody wrote — which '
    + 'is what it cost /pending 513, and what the comment in app.js used to work around by '
    + 'forbidding the prose instead of stripping it');
  assert.match(stripComments(fixture), /https:\/\/nib\.example/,
    'the strip ate a URL, so it is removing code as well as comments and every scan in this file '
    + 'is reading a source with holes in it');

  // Half two: the fixture in app.js is still there. The difference between the raw scan and the
  // stripped one IS the fixture, so it needs no second copy of the name to go stale against.
  const onlyInProse = modeFlagsIn(APP).filter((f) => !modeFlagsInSource().includes(f));
  assert.ok(onlyInProse.length > 0,
    'nothing in web/app.js names a view.<x>Mode inside a comment any more, so the strip above is '
    + 'guarding a case this repo no longer has an example of — restore the placeholder in '
    + '`disarmEditingTools`\'s comment rather than deleting this assertion');
  const served = new Set(declaredTools().map((t) => `${t}Mode`));
  const biting = onlyInProse.filter((f) => !served.has(f) && !NOT_A_PLACEMENT_TOOL[f]);
  assert.ok(biting.length > 0,
    `the only modes named in comments are ${onlyInProse.join(', ')}, and every one is a real tool `
    + 'or an exempted flag — so the fixture cannot bite: removing the strip would leave this file '
    + 'green and the defect would come back with the next sentence somebody writes');
});
