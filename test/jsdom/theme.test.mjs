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

// Four flavours since v1.123.0. **Every assertion below iterates this list**, so a palette in
// the stylesheet but missing here is checked by none of them — which is why the agreement test
// at the bottom exists rather than trusting whoever adds the next one to remember.
const THEMES = [
  { name: 'dark', selector: ':root {' },                                 // Mocha
  { name: 'light', selector: ':root[data-appearance="light"] {' },       // Latte
  { name: 'frappe', selector: ':root[data-appearance="frappe"] {' },
  { name: 'macchiato', selector: ':root[data-appearance="macchiato"] {' },
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

test('--text meets AA on every surface it is set on', () => {
  // --surface1 is the one that was under: 4.39:1 in the light theme, on a surface carrying
  // the active toolbar tab, button hover, the thumbnail actions and the stale-render retry.
  // Body text failing AA is a bigger fact than a muted token failing it, and it was found
  // while fixing the muted pair rather than by looking for it.
  for (const t of THEMES) {
    const p = palette(t.selector);
    for (const surface of ['base', 'mantle', 'crust', 'surface0', 'surface1']) {
      const c = contrast(p.text, p[surface]);
      assert.ok(c >= 4.5,
        `--text in the ${t.name} theme is ${c.toFixed(2)}:1 on --${surface}, below AA's 4.5:1 — the BODY colour is unreadable there`);
    }
  }
});

test('the active toolbar tab does not carry its label in --blue', () => {
  // --blue on --surface1 is 4.33:1 dark and 2.79:1 light — under AA in both — and the tab
  // already has a blue underline doing the signalling. This asserts the specific pairing
  // rather than the token, because --blue is fine elsewhere and it is this ONE rule that
  // put an unreadable label on a surface.
  const rule = CSS_CODE.split('\n').find((l) => l.includes('.tbtab > button.active'));
  assert.ok(rule, 'the active-tab rule is not in web/style.css — this scan is reading nothing');
  assert.ok(!/color:\s*var\(--blue\)/.test(rule),
    `the active tab sets its label colour to --blue again: ${rule.trim()}. It is under AA on --surface1 in both themes; the underline is what signals active.`);
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
  // **Three grounds, not one, and the widening is /pending 330's other half.** This
  // checked --crust alone, so it was silent about the two places --overlay0 was still
  // carrying text: `.menucap` on --surface0 (2.57 dark / 1.69 light) and `#empty` on
  // --base (3.36 / 2.30). A counter-assertion that names one ground reads as though the
  // token were only ever used there, and the next use goes to whichever ground it does
  // not read — which is exactly what happened. The grounds are discovered from the ones
  // the token is actually used against, so a fourth needs adding here deliberately.
  //
  // **The added grounds are load-bearing, and which one carries the check FLIPS with the
  // theme** — measured, because the reasoning goes the wrong way if you do it in your
  // head. A mid-grey contrasts most with whichever ground is furthest from it in
  // luminance, and the darkest ground is `--crust` in the dark theme but the LIGHTEST is
  // `--base` in the light one: overlay0 measures crust 3.84 / surface0 2.57 / base 3.36
  // dark, and crust 1.97 / surface0 1.69 / base 2.30 light. So `crust` dominates in dark
  // and `base` in light, and a crust-only check is blind in the light theme — proven by
  // setting the light token to #646464, which clears AA on --base at 5.23 while --crust
  // stays at 4.47 and says nothing.
  for (const t of THEMES) {
    const p = palette(t.selector);
    for (const ground of ['crust', 'surface0', 'base']) {
      const c = contrast(p.overlay0, p[ground]);
      assert.ok(c < 4.5,
        `--overlay0 in the ${t.name} theme is now ${c.toFixed(2)}:1 on --${ground}, which is AA-compliant — the premise of moving the empty-state messages to --subtext1 no longer holds, and this file should say so`);
    }
  }
});

test('nothing puts text in --overlay0, on any ground', () => {
  // The counter-assertion above says the token is unreadable; it does NOT say nothing
  // uses it, and those are different facts. /pending 330 found two live `color:` usages
  // sitting under a guard that had asserted the token's unreadability for four months —
  // the arithmetic was right and nothing checked the CONSUMERS. This is the half that
  // fails when someone reaches for it again.
  //
  // `color:` only. --overlay0 remains legitimate for non-text (the .ovl-dropdown border
  // at style.css:987), which WCAG judges at 3:1 under 1.4.11 rather than 4.5 under 1.4.3.
  const offenders = CSS.split('\n')
    .map((line, i) => [i + 1, line])
    .filter(([, line]) => /color:\s*var\(--overlay0\)/.test(line));
  assert.deepEqual(offenders, [],
    `--overlay0 carries text at style.css:${offenders.map(([n]) => n).join(', ')} — it measures below AA on every ground this file checks, so text in it is unreadable in at least one theme`);
});


// ── The four lists that describe one fact ────────────────────────────────────
// A theme exists in four places: the stylesheet's token block, the Go whitelist that decides
// whether the choice can be SAVED, the picker that offers it, and the THEMES list above that
// decides whether it is contrast-checked at all. Nothing compared them, and each disagreement
// fails silently in its own way — an unguarded palette, a choice that applies and is gone after
// a restart, a flavour nobody can pick, a palette no test reads.
test('the stylesheet, the server, the picker and this file name the same themes', () => {
  const inCss = new Set(['dark']); // the bare :root block IS dark
  for (const m of CSS.matchAll(/:root\[data-appearance="([a-z]+)"\] \{/g)) inCss.add(m[1]);

  const go = fs.readFileSync(path.join(REPO, 'internal', 'server', 'settings.go'), 'utf8');
  const caseLine = go.match(/case ((?:"[a-z]+"(?:, )?)+):\n\s*cur\.Appearance/);
  assert.ok(caseLine, 'the appearance whitelist is not in settings.go in the shape this scan reads');
  const inGo = new Set([...caseLine[1].matchAll(/"([a-z]+)"/g)].map((m) => m[1]));

  const html = fs.readFileSync(path.join(REPO, 'web', 'index.html'), 'utf8');
  const inPicker = new Set([...html.matchAll(/name="themechoice" value="([a-z]+)"/g)].map((m) => m[1]));

  const inThemes = new Set(THEMES.map((t) => t.name));
  const show = (s) => [...s].sort().join(', ');
  assert.equal(show(inGo), show(inCss),
    `the server accepts {${show(inGo)}} and the stylesheet defines {${show(inCss)}} — a theme the server rejects applies for the session and is gone after a restart, with nothing said`);
  assert.equal(show(inPicker), show(inCss),
    `the picker offers {${show(inPicker)}} and the stylesheet defines {${show(inCss)}}`);
  assert.equal(show(inThemes), show(inCss),
    `THEMES covers {${show(inThemes)}} and the stylesheet defines {${show(inCss)}} — every contrast assertion in this file iterates THEMES, so a palette missing from it is checked by none of them`);
});

// ── The sidebar cards ────────────────────────────────────────────────────────
// A card header is one of the six accents at 20% over --base, with --text on top. The level is
// measured rather than chosen: accent-coloured TEXT fails all six accents in Latte (1.70-3.52),
// a rail clears 3:1 for only three of them, and a tint over --surface0 leaves light red at 3.92.
// This recomputes the blend the CSS does with color-mix(), which no arithmetic guard can read.
test('every card header keeps its text readable, in every theme', () => {
  const ACCENTS = ['blue', 'green', 'red', 'yellow', 'peach', 'mauve'];
  // The tint level is a per-theme token: the dark flavours carry more of the accent than Latte
  // can, because Latte's --text is its darkest token and the margin runs out sooner.
  const tintOf = (selector) => {
    const at = CSS.indexOf(selector);
    const body = CSS.slice(at, CSS.indexOf('}', at));
    const m = body.match(/--card-tint:\s*(\d+)%/);
    assert.ok(m, `${selector} defines no --card-tint, so its card colour is unmeasurable`);
    return Number(m[1]) / 100;
  };
  const mix = (fg, bg, a) => {
    const h = (c) => [1, 3, 5].map((i) => parseInt(c.slice(i, i + 2), 16));
    const [f, b] = [h(fg), h(bg)];
    return '#' + f.map((v, i) => Math.round(a * v + (1 - a) * b[i]).toString(16).padStart(2, '0')).join('');
  };
  for (const t of THEMES) {
    const p = palette(t.selector);
    for (const a of ACCENTS) {
      assert.ok(p[a] && p.base && p.text, `${t.name} is missing --${a}, --base or --text`);
      const ratio = contrast(p.text, mix(p[a], p.base, tintOf(t.selector)));
      assert.ok(ratio >= 4.5,
        `${t.name}: a ${a} card header is ${ratio.toFixed(2)}:1 against its own text (needs 4.5). The tint level is 20% over --base and was measured against every theme; a new palette whose accent is too close to its base breaks it`);
    }
  }
});
