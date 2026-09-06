// Every toolbar control lives in a labelled group — the structural half of the small-screen
// fold (v1.119.0).
//
// ── Why this guard is the important one ──────────────────────────────────────
// Folding reads the DOM: `applyFold` moves `.tbgroup[data-fold]` elements between the bar and
// the pane's ⋯ More menu. A control added as a DIRECT child of a `.tbtab` therefore never
// folds — it stays in the bar at every width, silently, and the palette it belongs to quietly
// stops meeting its height ceiling on a small screen. Nothing else in the tree would notice:
// tier 3 measures the panes it knows about, and a new control looks exactly like an old one.
//
// ── What this tier cannot reach ──────────────────────────────────────────────
// All of the geometry. jsdom has no layout engine, so every rect is 0×0 at every viewport and
// the fold's actual EFFECT — heights, row counts, horizontal overflow — is unmeasurable here.
// That is `test/ui/responsive.test.mjs`, and it is the only tier that can see it.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

const h = await boot({});
const doc = h.document;

// The fold ranks app.js declares. 0 means "never folds"; the rest are the ladder.
const RANKS = ['0', '1', '2', '3', '4', '5', '6', '7'];

const panes = () => [...doc.querySelectorAll('.tbtab')];

test('every toolbar control sits inside a group', () => {
  // The fixed bar is a group host too since v1.121.0 — the mode-independent commands (open,
  // save, page, zoom, find) live there and must be grouped and labelled like any other.
  const ps = [...panes(), ...doc.querySelectorAll('#toolbar .tbfixed')];
  assert.ok(ps.length >= 5, `only ${ps.length} toolbar panes found — this guard is reading nothing`);
  for (const pane of ps) {
    // Direct-child controls. The ⋯ More menu is itself a direct child and is not a control,
    // so it is excluded by tag rather than by name.
    // **Named exemption (ADR-009): the Collaborate pane's role containers.** `.roletoggle`
    // and `.roletools` are that palette's own grouping device, gated on a second axis —
    // `.roletools.active` follows the chosen role, so at most seven controls are ever on
    // screen there and it stays inside the height ceiling at 360px without folding at all.
    // Wrapping them in fold groups would add a layer that changes nothing.
    const loose = [...pane.children].filter((el) =>
      // `.sbhead` is a card header, not a control — it opens the group it sits above.
      !el.classList.contains('sbhead')
      && !el.classList.contains('roletoggle') && !el.classList.contains('roletools')
      && (/^(BUTTON|SELECT|INPUT)$/.test(el.tagName)
          || (el.tagName === 'SPAN' && el.querySelector('button, input'))));
    assert.deepEqual(loose.map((e) => e.id || e.textContent.trim().slice(0, 20)), [],
      `these controls in the ${pane.dataset.tab} palette are not inside a .tbgroup, so they can never fold and will hold the bar open on a narrow window`);
  }
});

test('every group declares a label and a fold rank', () => {
  const groups = [...doc.querySelectorAll('.tbgroup')];
  assert.ok(groups.length >= 15, `only ${groups.length} groups found — this guard is reading nothing`);
  for (const g of groups) {
    const label = g.dataset.label;
    assert.ok(label && label.trim(),
      `a .tbgroup in the ${g.closest('.tbtab')?.dataset.tab} palette has no data-label — it would appear in ⋯ More as an unlabelled run, which is the flat list the grouping exists to replace`);
    assert.ok(RANKS.includes(g.dataset.fold),
      `group "${label}" has data-fold="${g.dataset.fold}", which is not one of ${RANKS.join('/')} — applyFold looks its threshold up by that value and an unknown rank folds at undefined, i.e. never`);
  }
});

test('a foldable group is never left empty, and never holds a dropdown', () => {
  for (const g of doc.querySelectorAll('.tbgroup[data-fold]')) {
    if (g.dataset.fold === '0') continue;
    const controls = g.querySelectorAll('button, select, input');
    assert.ok(controls.length > 0,
      `foldable group "${g.dataset.label}" holds no controls — it would put an empty heading in the ⋯ More menu`);
    // A .menu inside ⋯ More would be a menu within a menu, and the bar tracks one open menu
    // at a time (app.js's openMenu). Recent / Save as / Export are pinned to the bar for
    // exactly this reason, and their groups carry fold rank 0.
    assert.equal(g.querySelector('.menu'), null,
      `foldable group "${g.dataset.label}" contains a dropdown. Folding it would nest a menu inside the ⋯ More menu, and only one menu can be open at a time — give the group fold rank 0`);
  }
});

test('the ⋯ More menu is built inside its own pane, not beside it', () => {
  // This is what makes moving safe. Mode gating is `#toolbar .tbtab.active` — a descendant
  // selector — so a group moved into a .tbmore inside the same pane stays gated by its mode.
  // A ⋯ More built as a sibling of the panes would show every folded group in all five modes.
  const mores = [...doc.querySelectorAll('.tbmore')];
  assert.ok(mores.length >= 3, `only ${mores.length} ⋯ More menus were built — expected one per host with foldable groups`);
  for (const m of mores) {
    // `.tbfixed` is the deliberate exemption (ADR-017): it holds the mode-INDEPENDENT commands,
    // so there is no mode gating for its ⋯ More to preserve. Every other menu must sit inside
    // its own pane, or the groups it holds would show in all five modes.
    assert.ok(m.closest('.tbtab') || m.closest('.tbfixed'),
      'a ⋯ More menu was built outside both a .tbtab and the fixed bar, so the groups it holds would appear in every mode');
  }
});
