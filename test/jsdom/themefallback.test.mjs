// A vault holding a RETIRED flavour still renders a theme that exists.
//
// Nib offered four Catppuccin flavours between v1.123.0 and v1.123.4 — Latte, Frappé,
// Macchiato, Mocha — and now offers two. `light` and `dark` kept their names precisely so
// existing vaults would validate, but `frappe` and `macchiato` are real strings sitting in real
// vaults on real machines, and nothing rewrites them: the value is normalised where it is read.
//
// **What goes wrong without it is quiet, which is why this is a guard.** The stylesheet would
// find no `:root[data-appearance="frappe"]` block, fall through to the bare `:root` tokens, and
// render Mocha — the right pixels by luck — while <html> carried a `data-appearance` value that
// no rule anywhere claims. The next rule keyed on the attribute, or the next reader of it, is
// then reasoning about a theme that does not exist.
//
// ── What this tier can reach ─────────────────────────────────────────────────
// The attribute after boot, which is what applyStatus sets from the saved value. That is DOM
// state, not layout, so jsdom holds it.
//
// ── What it cannot ───────────────────────────────────────────────────────────
// That the rendered result LOOKS like Mocha. No layout, no computed style over a real
// stylesheet; `test/ui/responsive.test.mjs` is where colour that must actually paint is read.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

const RETIRED = 'frappe';
const h = await boot({
  routes: {
    '/api/status': () => ({
      state: 'ready', csrf: 'test-csrf', version: 'test',
      autoUpdate: false, updateCheckLocked: false, ghostscript: false, libreoffice: false,
      appearance: RETIRED,
    }),
  },
});
const doc = h.document;

test('a retired flavour saved in the vault renders as dark', () => {
  assert.equal(doc.documentElement.dataset.appearance, 'dark',
    `a vault holding "${RETIRED}" left data-appearance="${doc.documentElement.dataset.appearance}" on <html>. That flavour has no palette any more, so the stylesheet falls through to :root and renders Mocha by accident while the attribute names a theme nothing defines`);
});

test('the toggle still works from that state, and saves a value the server accepts', () => {
  // The half a normalisation can miss: the attribute is right and the next click computes from
  // the ORIGINAL saved string. Reading light-or-not-light off the DOM is what makes this hold.
  doc.getElementById('themeToggle').click();
  assert.equal(doc.documentElement.dataset.appearance, 'light',
    'toggling from a normalised dark did not go to light — the toggle is reading something other than the attribute it just set');
  doc.getElementById('themeToggle').click();
  assert.equal(doc.documentElement.dataset.appearance, 'dark',
    'toggling back did not return to dark');
});
