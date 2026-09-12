// Armed state is programmatic, not colour alone — P02.S02, WCAG SC 1.4.1 and SC 4.1.2.
//
// ── The defect, measured ─────────────────────────────────────────────────────
// 27 sites in `web/app.js` call `classList.toggle('active', …)` and **22 of them set no ARIA
// state at all**. Every armed tool, every selected preset and every mode tab announced itself to
// a screen reader as an ordinary button, while the sighted user saw a highlight. The state was
// conveyed by colour alone, which is the failure SC 1.4.1 names.
//
// ── Why this is a census and not a list ──────────────────────────────────────
// Fixing 22 sites is an afternoon; the defect that outlives the fix is the TWENTY-EIGHTH — a new
// toggle added with a class and no state, shipping a fresh Level A failure with every existing
// test green. A hand-written list of sites rots the first time somebody adds one. So this
// enumerates the call sites from the source and requires each to route through a door.
//
// ── Two doors, because there are two vocabularies ────────────────────────────
// `aria-pressed` belongs to a toggle button; `aria-selected` belongs to something with a
// `tab`/`option`/`row` role. `aria-selected` on a plain `<button>` is not a weaker statement, it
// is an invalid one — so the assertion is not "some ARIA attribute is set nearby" but "it went
// through the door that matches what this element is".
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const APP = readFileSync(new URL('../../web/app.js', import.meta.url), 'utf8');
const HTML = readFileSync(new URL('../../web/index.html', import.meta.url), 'utf8');

// Sites that toggle `active` on something that is NOT a control, each with what it is instead.
// A bare exclusion list is how a real control gets quietly dropped, so every entry says why.
const NOT_A_CONTROL = [
  {
    match: "all('.tbtab')",
    why: 'a container <div class="tbtab"> holding one mode\'s toolbar group — it is shown or hidden, ' +
         'and a region has no pressed or selected state to expose',
  },
  {
    match: "if (pages) pages.classList.toggle('active'",
    why: '#sbPages is the PANEL the sidebar tablist controls, not a tab. Its tab already carries ' +
         'aria-selected through setSelected',
  },
  {
    match: "if (functions) functions.classList.toggle('active'",
    why: '#sbFunctions, as #sbPages',
  },
  {
    match: "$(tab.dataset.panel)?.classList.remove('active')",
    why: 'the PANEL a header discloses, not the header. The header carries aria-expanded through ' +
         'setExpanded',
  },
  {
    match: "$(tab.dataset.panel).classList.add('active')",
    why: 'as above, the disclosed panel',
  },
  {
    match: "document.querySelectorAll('.panel').forEach((p) => p.classList.remove('active'))",
    why: 'every disclosed panel, closed together; the headers carry the state',
  },
  {
    match: "all('.panel').forEach((p) => { if (p.id !== 'commands')",
    why: 'as above',
  },
];

// The doors' own bodies necessarily contain the call they wrap.
const DOOR_BODIES = ['function setArmed', 'function setSelected', 'function setExpanded'];

// **All three spellings of the same state change.** The first version of this scan read only
// `classList.toggle('active', …)` and was blind to `classList.add('active')` /
// `.remove('active')` — which is how the sidebar's panel headers, nine sites, sat outside a
// census written to have no outside. A scan that knows one spelling of a thing is a scan with a
// hole the width of the other spellings.
const SPELLINGS = ["classList.toggle('active'", "classList.add('active')", "classList.remove('active')"];

function toggleSites() {
  const lines = APP.split('\n');
  const out = [];
  for (let i = 0; i < lines.length; i++) {
    const l = lines[i];
    if (!SPELLINGS.some((sp) => l.includes(sp))) continue;
    if (l.trimStart().startsWith('//')) continue;          // prose about the rule, not a site
    const near = lines.slice(Math.max(0, i - 8), i + 1).join('\n');
    if (DOOR_BODIES.some((d) => near.includes(d))) continue; // the doors themselves
    out.push({ line: i + 1, text: l.trim() });
  }
  return out;
}

test('every armed-state toggle routes through a door or is declared not a control', () => {
  const sites = toggleSites();
  // Stimulus: a scan that finds nothing passes everything.
  assert.ok(sites.length >= 1,
    'no bare classList.toggle(\'active\') sites found at all — either every site now routes ' +
    'through a door (good, but then this scan can no longer fail) or the scan has stopped matching');

  const unexplained = sites.filter((s) => !NOT_A_CONTROL.some((e) => s.text.includes(e.match)));
  assert.deepEqual(unexplained.map((s) => `app.js:${s.line} ${s.text.slice(0, 70)}`), [],
    'these sites toggle the `active` class without exposing the state programmatically.\n' +
    'A control whose armed state is conveyed by colour alone fails WCAG SC 1.4.1 and SC 4.1.2. ' +
    'Route it through `setArmed` (aria-pressed) or `setSelected` (aria-selected, tab role only), ' +
    'or add it to NOT_A_CONTROL with what it is instead.');
});

test('the exemptions name sites that still exist', () => {
  // An exemption for a departed site is a claim about code that is not there, and it silently
  // widens the hole when a new site matches that text.
  const stale = NOT_A_CONTROL.filter((e) => !APP.includes(e.match)).map((e) => e.match);
  assert.deepEqual(stale, [], `these exemptions match nothing in web/app.js: ${stale.join(', ')}`);
});

test('all three doors exist and set the attribute they are named for', () => {
  // The census above is satisfied by doors that set nothing at all — this is its stimulus check.
  assert.match(APP, /function setArmed\(el, on\) \{[\s\S]{0,200}?aria-pressed/,
    'setArmed no longer sets aria-pressed — every site routed through it now exposes nothing');
  assert.match(APP, /function setSelected\(el, on\) \{[\s\S]{0,200}?aria-selected/,
    'setSelected no longer sets aria-selected');
  assert.match(APP, /function setExpanded\(el, on\) \{[\s\S]{0,200}?aria-expanded/,
    'setExpanded no longer sets aria-expanded');
  assert.match(APP, /function setArmed\(el, on\) \{[\s\S]{0,200}?classList\.toggle\('active'/,
    'setArmed no longer sets the class — the visual state is gone for sighted users');
});

test('aria-selected is used only where a tab role backs it', () => {
  // **The reason there are two doors and not one.** `aria-selected` on an element with no
  // tab/option/row role is invalid ARIA — ignored by some readers, misreported by others, which
  // is worse than the silence this slice exists to fix.
  // **Counting `setSelected(` outright counts the DEFINITION too**, so a threshold of >= 2 is
  // satisfied by `function setSelected(el, on)` plus a single call — and >= 1 by the definition
  // alone, with no caller anywhere. Excluded explicitly; found by pasting the count at the commit
  // gate and seeing 2 where there is 1 call.
  const selectedCalls = (APP.match(/(?<!function )setSelected\(/g) || []).length;
  assert.ok(selectedCalls >= 1,
    `setSelected has ${selectedCalls} caller(s) — it is defined and unused, so either the ` +
    'tablist stopped using it or the door is dead code');
  assert.match(HTML, /role="tablist"/,
    'no tablist in index.html, so nothing legitimately takes aria-selected and setSelected has ' +
    'no valid subject');
});

test('nothing wireTablist runs as a tablist is given aria-pressed', () => {
  // **The test this replaces asserted the WRONG THING and shipped a defect behind it.** It read
  // `index.html` for `role="tab"` on `.modetab`, found none — they are plain `<button>` in the
  // markup — and concluded they should take `aria-pressed`. But `wireTablist` adds
  // `role="tablist"`, `role="tab"`, `aria-selected` and a roving tabindex at RUNTIME
  // (`app.js:9008`), so v1.129.23 put `aria-pressed` on elements carrying `role="tab"`: exactly
  // the invalid pairing the three doors exist to prevent. A markup scan cannot see a runtime
  // decision, and this one is made ten lines away in the same file.
  //
  // So the question is asked of the code that makes the tablist, not of the HTML.
  const wired = [...APP.matchAll(/wireTablist\([^,]+,\s*'([^']+)'\)/g)].map((m) => m[1]);
  assert.ok(wired.length >= 3,
    `found ${wired.length} wireTablist call(s) — the scan has stopped matching and this guard is ` +
    'vacuous');

  for (const sel of wired) {
    // Every site that reflects state onto this selector must use setSelected.
    const cls = sel.replace(/^\./, '');
    const armed = new RegExp(`setArmed\\([^)]*${cls}|all\\('\\.${cls}'\\)[^\n]*setArmed`, 'g');
    const hits = APP.match(armed) || [];
    assert.deepEqual(hits, [],
      `${sel} is run as a tablist by wireTablist, and something routes it through setArmed: ` +
      `${hits.join(', ')}. aria-pressed on an element with role="tab" is invalid ARIA — it must ` +
      'go through setSelected.');
  }
});

test('the sidebar panel headers do not reach the document strip', () => {
  // `.tab` matches two different things: the sidebar's panel headers, which carry `data-panel`,
  // and the document strip's tabs built at `app.js:2509`, which do not — and which wireTablist
  // runs as a real tablist. A bare `.tab` selector in the panel handler set `aria-expanded` on
  // both, which is the same vocabulary collision one selector over.
  assert.match(APP, /querySelectorAll\('\.tab\[data-panel\]'\)[^\n]*setExpanded/,
    'the panel-header handler no longer scopes to [data-panel], so it reaches the document ' +
    'strip’s tabs as well');
});
