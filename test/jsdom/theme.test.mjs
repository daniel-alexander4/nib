// The two muted text tokens keep their ordering in BOTH themes.
//
// `--subtext1` is the higher-contrast of the pair — that is what the name means across
// every Catppuccin surface, and it is what the dark theme has always done. The light
// theme had them INVERTED: `--subtext1` was #7c7f93, which is Latte's *overlay2*, giving
// 3.25:1 on --mantle against --subtext0's 4.06:1. Below AA, and backwards.
//
// The trap that hid it is why this is a guard and not just a fix. "Use the more muted
// token" made LIGHT mode worse while making dark mode better, and the dark theme is where
// the palette gets read during development — so the inversion was invisible to the person
// most likely to touch these values. It was found by COMPUTING the contrast while picking
// a colour for the tab strip, not by looking.
//
// Pure source scan of web/style.css: no boot, no DOM. The contrast maths is the WCAG
// relative-luminance formula, written out here rather than imported, because the whole
// point is that this file does not depend on the thing it is checking.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { REPO } from './boot.mjs';

const CSS = fs.readFileSync(path.join(REPO, 'web', 'style.css'), 'utf8');

function luminance(hex) {
  const h = hex.replace('#', '');
  const ch = [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16) / 255);
  const lin = ch.map((c) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4));
  return 0.2126 * lin[0] + 0.7152 * lin[1] + 0.0722 * lin[2];
}
const contrast = (a, b) => {
  const [la, lb] = [luminance(a), luminance(b)];
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05);
};

// The palette blocks: `:root { … }` is the dark theme and
// `:root[data-appearance="light"] { … }` is the light one.
function palette(selector) {
  const at = CSS.indexOf(selector);
  assert.notEqual(at, -1, `${selector} is not in web/style.css — this scan is not reading the palette`);
  const body = CSS.slice(at, CSS.indexOf('}', at));
  const out = {};
  for (const m of body.matchAll(/--([a-z0-9]+):\s*(#[0-9a-fA-F]{6})/g)) out[m[1]] = m[2];
  return out;
}

const THEMES = [
  { name: 'dark', selector: ':root {' },
  { name: 'light', selector: ':root[data-appearance="light"] {' },
];

test('both palettes define the tokens this file reasons about', () => {
  // The stimulus. A regex that matched nothing would make every assertion below pass over
  // an empty object — the exact shape of green this repo keeps finding.
  for (const t of THEMES) {
    const p = palette(t.selector);
    for (const token of ['text', 'subtext0', 'subtext1', 'base', 'mantle', 'crust', 'surface0', 'overlay0', 'yellow']) {
      assert.ok(p[token], `the ${t.name} palette has no --${token}, so the scan is not reading it`);
    }
  }
});

test('--subtext1 is the HIGHER-contrast muted token in every theme', () => {
  for (const t of THEMES) {
    const p = palette(t.selector);
    const c1 = contrast(p.subtext1, p.mantle);
    const c0 = contrast(p.subtext0, p.mantle);
    assert.ok(c1 > c0,
      `in the ${t.name} theme --subtext1 (${p.subtext1}, ${c1.toFixed(2)}:1 on --mantle) is LESS readable than --subtext0 (${p.subtext0}, ${c0.toFixed(2)}:1). The pair is inverted: every other Catppuccin surface and the other theme here treat subtext1 as the stronger of the two, so "use the more muted token" now means the opposite thing depending on which theme you are looking at.`);
  }
});

test('BOTH muted tokens meet AA on every surface muted text sits on', () => {
  // 4.5:1 is AA for normal-size text. --subtext0 used to be excluded here, with a comment
  // saying it sat at 4.06:1 in the light theme and was a separate open item — a guard
  // asserting only the token that already passed. It is asserted now because the light
  // theme deliberately left Catppuccin Latte to make it true (see web/style.css).
  //
  // --surface0 is excluded from the sweep and named rather than silently dropped:
  // --subtext0 is 3.93:1 there and ONE rule puts muted text on that background
  // (.badge-none), which uses --subtext1 (4.57:1) for exactly this reason.
  for (const t of THEMES) {
    const p = palette(t.selector);
    for (const token of ['subtext0', 'subtext1']) {
      for (const surface of ['base', 'mantle', 'crust']) {
        const c = contrast(p[token], p[surface]);
        assert.ok(c >= 4.5,
          `--${token} in the ${t.name} theme is ${c.toFixed(2)}:1 on --${surface}, below AA's 4.5:1 for normal text`);
      }
    }
  }
});

test('the muted scale stays a scale — three distinguishable steps under --text', () => {
  // The cost of clearing AA by darkening is that the steps converge, and converged far
  // enough they stop being a hierarchy: three tokens all reading as body text. Each step
  // must be a real one, and every muted token must stay LIGHTER than --text — a "muted"
  // token darker than the body colour is not muted, it is emphasis.
  for (const t of THEMES) {
    const p = palette(t.selector);
    const [text, s1, s0] = [p.text, p.subtext1, p.subtext0].map((c) => contrast(c, p.mantle));
    assert.ok(text > s1 && s1 > s0,
      `the ${t.name} theme's muted scale is not ordered on --mantle: --text ${text.toFixed(2)}, --subtext1 ${s1.toFixed(2)}, --subtext0 ${s0.toFixed(2)}`);
    assert.ok(text / s1 >= 1.05 && s1 / s0 >= 1.05,
      `the ${t.name} theme's muted steps have collapsed into each other on --mantle (--text ${text.toFixed(2)} / --subtext1 ${s1.toFixed(2)} / --subtext0 ${s0.toFixed(2)}): the tokens are still three names but no longer three visible weights`);
  }
});

// --- empty-state messages are content, not decoration -------------------------
//
// Four lists said why they were blank — "No keys enrolled.", "No peers pinned yet.",
// "Loading the document…", "no sub-folders" — from `:empty::after { content: … }` in this
// stylesheet. Generated content cannot be selected or copied and is exposed to assistive
// tech inconsistently, so a load-bearing sentence reached some users and not others. They
// are real elements now (`emptyNote`, and `li.blank` in the folder browser).
//
// A source scan, like the rest of this file: the property is that the STYLESHEET does not
// carry these strings, and that is a fact about the file rather than about a rendered DOM.

// Comments stripped FIRST. This repo has twice shipped a guard satisfied by prose — a
// `strings.Contains` matching a doc comment that named the thing instead of the code
// doing it, and a `.deb` check satisfied by "xdg-utils" inside a comment. The comment
// above this block names `:empty::after` in exactly the words the scan hunts for.
const CSS_CODE = CSS.replace(/\/\*[\s\S]*?\*\//g, '');
const afterRules = [...CSS_CODE.matchAll(/([^{}]+)::(?:after|before)\s*\{([^}]*content:[^}]*)\}/g)]
  .map((m) => ({ selector: m[1].trim(), body: m[2].trim() }));

test('the ::after scan actually finds generated content', () => {
  // The stimulus, and it is not ceremony: a regex that matched nothing would make the
  // assertion below pass over an empty array forever, including on the day someone puts
  // a message back. There ARE decorative ::before/::after rules here (the menu caret, the
  // folder and file icons) and they are legitimate — the next test is about which ones.
  assert.ok(afterRules.length >= 3,
    `the scan found ${afterRules.length} generated-content rules in web/style.css, so it is not reading the file`);
});

test('no empty-state message is generated content', () => {
  const offenders = afterRules.filter((r) => /:empty/.test(r.selector));
  assert.deepEqual(offenders, [],
    `these rules put a message in generated content: ${offenders.map((o) => o.selector).join(', ')}. A ':empty::after' string cannot be selected or copied and is not reliably announced — build a real element instead (see emptyNote in web/app.js).`);
});

test('the muted token those messages use meets AA on the surfaces they sit on', () => {
  // --crust as well as --mantle: the folder browser and the document preview both set
  // `background: var(--crust)`, which is the darkest/lightest surface in each theme and
  // therefore the hardest case, and it is exactly where two of the four messages live.
  for (const t of THEMES) {
    const p = palette(t.selector);
    for (const surface of ['mantle', 'crust', 'base']) {
      const c = contrast(p.subtext1, p[surface]);
      assert.ok(c >= 4.5,
        `--subtext1 in the ${t.name} theme is ${c.toFixed(2)}:1 on --${surface}, below AA's 4.5:1`);
    }
  }
});

test('the warning note carries its message in text, not in its colour', () => {
  // --yellow is a fine warning colour in the dark theme and 2.15:1 on --mantle in the
  // light one, so `.stampwarn` puts the words in --text on --surface0 and lets the yellow
  // be a left border. This asserts the pair that carries the MESSAGE, which is the pair a
  // future "make the warning look more like a warning" edit would move.
  for (const t of THEMES) {
    const p = palette(t.selector);
    const c = contrast(p.text, p.surface0);
    assert.ok(c >= 4.5,
      `--text on --surface0 is ${c.toFixed(2)}:1 in the ${t.name} theme, below AA — .stampwarn's words are unreadable there`);
  }
  // And the fact that decides it is not symmetric — which the first draft of this test
  // got wrong by asserting it of BOTH themes. --yellow is 9.89:1 on --surface0 in the
  // dark theme and 2.45:1 in the light one, so the token is only unusable for text in
  // ONE of them. One is enough: a warning cannot be readable in one theme and not the
  // other. (The deeper reason stands whatever these numbers do — colour alone is not an
  // accessible signal, WCAG 1.4.1 — so this asserts the arithmetic, not the principle.)
  const failing = THEMES.filter((t) => contrast(palette(t.selector).yellow, palette(t.selector).surface0) < 4.5);
  assert.ok(failing.length > 0,
    '--yellow now clears AA on --surface0 in BOTH themes, so the arithmetic no longer forces .stampwarn to keep its words in --text. The design should still not put the message in the colour (WCAG 1.4.1) — change this test only with that reasoning in hand, not because the number moved.');
});

test('--overlay0 does NOT meet AA there — which is why the messages moved off it', () => {
  // The counter-assertion, and the reason the one above is not decoration. These four
  // messages were `--overlay0`. If overlay0 ever became readable this test fails and
  // someone gets to delete it; while it does not, this records what the move bought and
  // stops the previous test reading as "a token we happened to pick is fine".
  for (const t of THEMES) {
    const p = palette(t.selector);
    const c = contrast(p.overlay0, p.crust);
    assert.ok(c < 4.5,
      `--overlay0 in the ${t.name} theme is now ${c.toFixed(2)}:1 on --crust, which is AA-compliant — the premise of moving the empty-state messages to --subtext1 no longer holds, and this file should say so`);
  }
});
