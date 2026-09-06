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
import { boot } from './boot.mjs';

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
