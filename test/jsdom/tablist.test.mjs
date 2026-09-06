// /pending 329 — four tab-like surfaces, one rule, and it reached one of them.
//
// The mode tabs, the sidebar tabs and Collaborate's Originate/Receive toggle carried no
// role, no aria-selected and no aria-current: active state was two greys and a 2px
// underline, which is colour alone (WCAG 1.4.1). The document strip DID claim
// `role="tablist"` — and gave every tab `tabIndex = 0` while binding no arrow key, so it
// announced a widget it had not built, which the strip's own comment calls "worse than
// plain buttons, because the promise is louder".
//
// ── What this tier can reach ─────────────────────────────────────────────────
// Roles, aria-selected, the roving tabindex, and arrow-key focus movement — all of it is
// attributes and `activeElement`, which jsdom models. The MutationObserver that re-syncs
// from the `.active` class runs here too.
//
// ── What it cannot, and who covers it ────────────────────────────────────────
// That the underline is not the only signal to a sighted user, and that focus is visibly
// somewhere — jsdom has no layout and no rendering. Tier 3 drives the real UI.
//
// **Why these are asserted through the DOOR and not per surface.** Naming the four
// containers here would be a second copy of the wiring list, green the day a fifth
// surface is added without it — which is the exact defect being fixed. The surfaces are
// DISCOVERED by role="tablist" and every one found must satisfy the whole contract.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { boot, REPO } from './boot.mjs';

const h = await boot({});
const { document: doc } = h;

// Discovered, not listed. The static surfaces are all present at boot; #tabstrip is
// hidden until a second document opens and is covered by the same wiring.
const lists = () => [...doc.querySelectorAll('[role="tablist"]')].filter((l) => l.querySelector('[role="tab"]'));

test('every tab-like surface is wired as a tablist', () => {
  const found = lists();
  // **Two, not three, since v1.122.0.** The sidebar's tab strip became an accordion: its
  // buttons are disclosure headers carrying aria-expanded, sitting above the panel each one
  // opens, which is a different widget from a tab strip and must not announce itself as one.
  // The remaining static surfaces are the mode tabs and the Collaborate role toggle.
  assert.ok(found.length >= 2,
    `only ${found.length} populated tablists found — two static surfaces were expected, so this guard is reading nothing`);
});

test('every tablist reports which tab is selected', () => {
  for (const l of lists()) {
    const tabs = [...l.querySelectorAll('[role="tab"]')];
    for (const t of tabs) {
      assert.ok(t.hasAttribute('aria-selected'),
        `a tab in ${l.className || l.id} has no aria-selected — its active state is colour alone, which WCAG 1.4.1 refuses`);
    }
    const selected = tabs.filter((t) => t.getAttribute('aria-selected') === 'true');
    assert.equal(selected.length, 1,
      `${l.className || l.id} reports ${selected.length} selected tabs, want exactly 1`);
  }
});

test('every tablist is ONE tab stop, not N', () => {
  // The roving tabindex, and the half the document strip had backwards: it set
  // tabIndex = 0 on every tab, so a keyboard user Tabbed through all of them.
  for (const l of lists()) {
    const tabs = [...l.querySelectorAll('[role="tab"]')];
    const stops = tabs.filter((t) => t.tabIndex === 0);
    assert.equal(stops.length, 1,
      `${l.className || l.id} has ${stops.length} tab stops across ${tabs.length} tabs — a tablist is one stop, and the rest are reached with arrows`);
  }
});

test('arrows move focus within a tablist, and wrap', () => {
  const l = lists().find((x) => x.querySelectorAll('[role="tab"]').length >= 3);
  assert.ok(l, 'no tablist with 3+ tabs — the wrap below would be measuring nothing');
  const tabs = [...l.querySelectorAll('[role="tab"]')].filter((t) => !t.hidden && !t.disabled);

  tabs[0].focus();
  assert.equal(doc.activeElement, tabs[0], 'setup: the first tab did not take focus');

  l.dispatchEvent(new h.window.KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true }));
  assert.equal(doc.activeElement, tabs[1],
    'ArrowRight did not move focus to the next tab — the surface claims a tablist and does not implement one, which is the defect this item is about');

  // And the stop moves with it, or the user Tabs back into a tab that is no longer where
  // they left off.
  assert.equal(tabs[1].tabIndex, 0, 'the focused tab is not the tab stop');
  assert.equal(tabs[0].tabIndex, -1, 'the previous tab kept the tab stop');

  tabs[0].focus();
  l.dispatchEvent(new h.window.KeyboardEvent('keydown', { key: 'ArrowLeft', bubbles: true }));
  assert.equal(doc.activeElement, tabs[tabs.length - 1],
    'ArrowLeft from the first tab did not wrap to the last');
});

test('arrows do not ACTIVATE, only move focus', () => {
  // Manual activation is the deliberate choice: on the document strip, activating
  // rebuilds the strip and destroys the element holding focus. One behaviour across all
  // four surfaces beats a rule with an exception nobody remembers.
  const l = lists().find((x) => x.querySelectorAll('[role="tab"]').length >= 3);
  const tabs = [...l.querySelectorAll('[role="tab"]')].filter((t) => !t.hidden && !t.disabled);
  const before = tabs.findIndex((t) => t.getAttribute('aria-selected') === 'true');
  tabs[0].focus();
  l.dispatchEvent(new h.window.KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true }));
  const after = tabs.findIndex((t) => t.getAttribute('aria-selected') === 'true');
  assert.equal(after, before,
    'moving focus with an arrow key also changed the selection — on the document strip that rebuilds the strip under the user mid-keystroke');
});

// ── The other widget: the sidebar's disclosure headers ───────────────────────
// The sidebar stopped being a tablist in v1.122.0 and became an accordion, so its buttons are
// the widget this file's four tests deliberately do NOT cover — and the half of the contract a
// disclosure carries that a tab does not is that it CLOSES again.
//
// It reached one of the two kinds. Group cards got the close branch in `openCard`; panel cards
// kept the tab wiring, which only ever ADDED `.active`, so *Pages* and *Outline* re-opened on
// every click and could not be put away (measured in Chromium at v1.123.0, 2 of File mode's 4
// pills). ADR-020.
//
// Discovered through the door, like the tablists above: every `.sbhead` found must toggle, so a
// third kind of card added without one fails here rather than being absent from a list.
//
// **What this tier cannot reach:** whether the body actually stops RENDERING. jsdom has no
// layout, so this reads `.active` / `aria-expanded`, and a card whose state flips while its CSS
// keeps it on screen would be green here. `test/ui/responsive.test.mjs` drives the real browser.
//
// **`#commands` is exempt, and the exemption is checked rather than asserted.** It is a
// pass-through, not a card: it has no body of its own — its GROUPS are the cards — and its
// header is `display: none` in the stylesheet, so nothing can click it and its `.active` is
// meaningless. `openCard` leaves it active on purpose while clearing every other panel head,
// which is the state split this guard would otherwise report as a stuck card. The stylesheet
// rule is read below, so making it a real card again fails here instead of silently dropping
// it from the set.
const CSS = fs.readFileSync(path.join(REPO, 'web', 'style.css'), 'utf8');
const cards = () => [...doc.querySelectorAll('#sidebar .sbhead')].filter((c) => c.dataset.panel !== 'commands');
const isOpen = (c) => (c.dataset.panel
  ? !!doc.getElementById(c.dataset.panel)?.classList.contains('active')
  : c.getAttribute('aria-expanded') === 'true');

test('every sidebar card header toggles — a click on the open card closes it', () => {
  assert.match(CSS, /\.sbhead\[data-panel="commands"\]\s*\{[^}]*display:\s*none/,
    'the #commands header is no longer hidden by the stylesheet, so it is a real card now and this guard is skipping it — drop the exemption above');

  const all = cards();
  const panels = all.filter((c) => c.dataset.panel);
  const groups = all.filter((c) => c.classList.contains('groupcard'));
  // BOTH kinds, counted separately: the defect was one kind toggling and the other not, so a
  // guard that happened to read only the working kind would have been green through all of it.
  assert.ok(panels.length >= 2 && groups.length >= 2,
    `found ${panels.length} panel cards and ${groups.length} group cards — the sidebar has both kinds and this guard must read both`);

  const wontOpen = [];
  const wontClose = [];
  for (const c of all) {
    const name = c.textContent.trim() || c.dataset.panel;
    if (!isOpen(c)) c.click();
    if (!isOpen(c)) { wontOpen.push(name); continue; }
    c.click();
    if (isOpen(c)) wontClose.push(name);
  }
  assert.deepEqual(wontOpen, [],
    `these card headers did not open on a click: ${wontOpen.join(', ')} — reported separately from the close, because a card that never opens would make the close assertion vacuously true`);
  assert.deepEqual(wontClose, [],
    `these card headers stayed open when their own header was clicked a second time: ${wontClose.join(', ')}. A disclosure that only opens leaves the user no way to put it away, which is what half the sidebar did before ADR-020`);
});

// The door's own property, and it was a proven hole: with the toggle in place, deleting
// showPanel's is-it-already-open guard left all 188 tests green. Re-entering a mode then CLOSES
// the surface it is supposed to land on, because "show me this panel" had become "toggle it".
test('re-entering a mode lands on its panel rather than closing it', () => {
  const enter = () => doc.querySelector('.modetab[data-tab="collaborate"]').click();
  enter();
  assert.equal(doc.getElementById('flags').classList.contains('active'), true,
    'setup: Collaborate did not land on Flags, so the second entry below proves nothing');
  enter();
  assert.equal(doc.getElementById('flags').classList.contains('active'), true,
    'entering a mode that is already showing its landing panel CLOSED it. syncSidebarForMode means "show", not "toggle" — that is what showPanel() is for, and a bare .click() on the header now does the opposite');
});
