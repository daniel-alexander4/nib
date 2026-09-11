// Every button has an accessible name — P02.S03, WCAG SC 4.1.2 Level A.
//
// ── The measurement, and why the slice was scoped against the wrong number ────
// The phase-open sketch said 21 icon-only buttons named by `title` alone; the firmed slice said
// four. Measured here: **279 buttons, 272 named by their own text, 3 by `aria-label`, 2
// title-only, and 2 with no name in the markup at all** — and those last two set real text at
// runtime, so they are named where it counts. The defect surface was **two**.
//
// ── Why `title` alone does not count as a name in this rule ───────────────────
// The ARIA accessible-name computation does fall back to `title`, so a title-only button is not
// strictly nameless and would pass a naive 4.1.2 check. It is still the wrong answer here: a
// `title` is announced inconsistently across screen readers and is invisible to anyone not using
// a mouse — which is exactly the population this phase exists for. So this guard requires a name
// that does not depend on hover.
//
// ── And the case that is worse than title-only ───────────────────────────────
// `updateGet` sets `textContent` to `v1.129.23` at runtime, and text content BEATS `title` in the
// name computation. Its accessible name was a version string that describes nothing, while a
// perfectly good sentence sat in the `title` where no keyboard user would find it. A button can
// have a name and still be unnamed in every sense that matters.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const HTML = readFileSync(new URL('../../web/index.html', import.meta.url), 'utf8');
const APP = readFileSync(new URL('../../web/app.js', import.meta.url), 'utf8');

// Buttons that carry no name in the markup because they are named at runtime. Each entry names
// the symbol app.js must be seen assigning a name to — so an entry cannot outlive the code that
// justifies it.
const NAMED_AT_RUNTIME = [
  { id: 'sessionNoticeAction', by: "els.sessionNoticeAction.textContent" },
  { id: 'signAction', by: 'els.signAction.textContent' },
  { id: 'themeToggle', by: 'describeButton(els.themeToggle' },
  { id: 'updateGet', by: 'describeButton(els.updateGet' },
];

function buttons() {
  return [...HTML.matchAll(/<button\b([^>]*)>([\s\S]*?)<\/button>/g)].map((m) => {
    const attrs = m[1];
    const id = (attrs.match(/id="([^"]+)"/) || [])[1] || null;
    return {
      id,
      attrs,
      text: m[2].replace(/<[^>]+>/g, '').trim(),
      hasLabel: /aria-label=|aria-labelledby=/.test(attrs),
    };
  });
}

test('every button has a name that does not depend on hover', () => {
  const all = buttons();
  // Stimulus: a scan that finds no buttons passes everything.
  assert.ok(all.length > 100,
    `only ${all.length} <button> found in index.html — the scan has stopped matching and every ` +
    'assertion below is vacuous');

  const runtime = new Set(NAMED_AT_RUNTIME.map((e) => e.id));
  const unnamed = all
    .filter((b) => !b.text && !b.hasLabel && !runtime.has(b.id))
    .map((b) => b.id || b.attrs.trim().slice(0, 60));
  assert.deepEqual(unnamed, [],
    'these buttons have no visible text and no aria-label: ' + unnamed.join(', ') + '.\n' +
    'A `title` does not count here — it is announced inconsistently and is invisible without a ' +
    'mouse. Give the button an aria-label (describeButton sets title and label together), or add ' +
    'it to NAMED_AT_RUNTIME with the app.js assignment that names it.');
});

test('every runtime-named button is actually named in app.js', () => {
  // The exemption list is the hole in the rule above, so it is checked against the code rather
  // than trusted. An entry whose evidence has gone is a button that is now silently unnamed.
  const missing = NAMED_AT_RUNTIME.filter((e) => !APP.includes(e.by));
  assert.deepEqual(missing.map((e) => `${e.id} (expected: ${e.by})`), [],
    'these buttons are exempted as runtime-named, and app.js no longer names them');
});

test('the runtime-named list names buttons that still exist', () => {
  // The other direction: an entry for a departed button silently exempts whatever takes that id.
  const ids = new Set(buttons().map((b) => b.id).filter(Boolean));
  const stale = NAMED_AT_RUNTIME.filter((e) => !ids.has(e.id)).map((e) => e.id);
  assert.deepEqual(stale, [], `these exemptions name buttons that no longer exist: ${stale.join(', ')}`);
});

test('describeButton sets the label and the tooltip together', () => {
  // **The stimulus check for the door.** The census above is satisfied by a describeButton that
  // sets nothing: every exempted button would still point at a call site that exists.
  const m = APP.match(/function describeButton\(el, text\) \{([\s\S]*?)\n\}/);
  assert.ok(m, 'describeButton is gone or reshaped — the two buttons it names are unnamed again');
  assert.match(m[1], /el\.title = text;/, 'describeButton no longer sets the tooltip');
  assert.match(m[1], /setAttribute\('aria-label', text\)/,
    'describeButton no longer sets aria-label — it is now just a tooltip setter, and the ' +
    'accessible name is gone while every call site still looks correct');
});

test("updateGet's accessible name describes the action, not the version", () => {
  // The case that is worse than title-only: textContent beats title in the name computation, so
  // a button whose text is `v1.129.23` is named after a version string. Every site that writes
  // that textContent must also set a label.
  const sites = [...APP.matchAll(/els\.updateGet\.textContent = [^\n]*\n/g)];
  assert.ok(sites.length >= 3,
    `found ${sites.length} site(s) setting updateGet's text — the scan has stopped matching`);
  for (const s of sites) {
    const after = APP.slice(s.index, s.index + 400);
    assert.match(after, /describeButton\(els\.updateGet/,
      'a site sets updateGet\'s text to a version string without giving it a label, so its ' +
      'accessible name becomes that version string:\n  ' + s[0].trim());
  }
});
