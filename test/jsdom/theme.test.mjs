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
    for (const token of ['text', 'subtext0', 'subtext1', 'base', 'mantle']) {
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

test('--subtext1 meets AA for normal text in every theme', () => {
  // 4.5:1 is AA for normal-size text. --subtext0 is NOT asserted here: it sits at 4.06:1
  // in the light theme and is a separate, still-open item on the pending list. Naming
  // that rather than quietly asserting only the token that passes — a guard that tests
  // exactly what is already true teaches nothing about what is not.
  for (const t of THEMES) {
    const p = palette(t.selector);
    const c = contrast(p.subtext1, p.mantle);
    assert.ok(c >= 4.5,
      `--subtext1 in the ${t.name} theme is ${c.toFixed(2)}:1 on --mantle, below AA's 4.5:1 for normal text`);
  }
});
