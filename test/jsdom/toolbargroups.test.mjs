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
    // The Collaborate pane's `.roletoggle` / `.roletools` exemption was here until v1.126.1 and
    // is gone with them: those two containers held the Originate/Receive split, and that pane is
    // one `Simple Sign` card now. An exemption naming classes the markup no longer contains is a
    // claim about a shape that does not exist.
    const loose = [...pane.children].filter((el) =>
      // `.sbhead` is a card header, not a control — it opens the group it sits above.
      !el.classList.contains('sbhead')
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

// ── ADR-022: the bar holds what you reach for continuously ───────────────────
//
// The division is by RHYTHM, not by mode: Save, page, zoom and find are used repeatedly while
// reading one document; opening, exporting, printing and closing happen once per document and
// are File-mode cards. Before v1.124.0 all of it sat in the fixed bar, three rows deep.
//
// **Structural, and checked from the DOM rather than from a list of labels**, because the way
// this decays is a control being added back to the bar "just for now" — which reads as a
// one-line diff and costs a row at every width. jsdom has no layout, so the ROW COUNT is
// `responsive.test.mjs`'s to assert; what is checkable here is where each control lives.
const LIFECYCLE = [
  'openMenuItem', 'officeOpenBtn',                              // open
  'saveFlatBtn', 'saveEditableBtn', 'saveFillableBtn', 'reduceBtn',   // save a copy
  'exportZipBtn', 'exportPngBtn', 'exportCertBtn', 'printBtn',  // export & print
  'closeBtn', 'closeAllBtn',                                    // close
];

test('the file lifecycle lives in File mode, not in the fixed bar', () => {
  const filePane = doc.querySelector('.tbtab[data-tab="file"]');
  assert.ok(filePane, 'there is no File pane, so this guard is reading nothing');

  const inBar = LIFECYCLE.filter((id) => doc.getElementById(id)?.closest('.tbfixed'));
  assert.deepEqual(inBar, [],
    `these once-per-document controls are back in the fixed bar: ${inBar.join(', ')}. The bar is for what you reach for continuously (ADR-022) — everything else is a card, and each control put back here costs a toolbar row at every width`);

  const missing = LIFECYCLE.filter((id) => !filePane.contains(doc.getElementById(id)));
  assert.deepEqual(missing, [],
    `these controls are neither in the fixed bar nor in File mode: ${missing.join(', ')} — moved out of the bar and not rehomed, which is worse than leaving them there`);
});

test('what stayed in the bar is what you reach for continuously', () => {
  // The other direction, and it is not the same assertion: a bar emptied of everything would
  // pass the test above. Save is the one file command with a per-minute rhythm and a state to
  // show, and page/zoom/find are used while reading rather than once per document.
  // `prevBtn`/`nextBtn` were here until v1.125.0 and are not merely moved — the buttons are gone,
  // and paging is PageUp/PageDown/Home/End plus the thumbnail grid. The page NUMBER readout went
  // to the Pages tab, which is why `.pageCount` is still asserted elsewhere in this tier.
  for (const id of ['saveBtn', 'zoomInBtn', 'zoomOutBtn', 'fitBtn', 'findToggle', 'reloadBtn']) {
    assert.ok(doc.getElementById(id)?.closest('.tbfixed'),
      `${id} has left the fixed bar. It is used repeatedly while reading one document, so putting it behind a card charges a click for every use`);
  }
});

// ── The sidebar's two sections, and the title in the bar (v1.125.0) ──────────
//
// Pages is the thumbnail grid alone; Functions holds everything else. The split is asserted
// STRUCTURALLY because the way it decays is a panel being appended to the sidebar and landing
// nowhere — `buildSidebarTabs` moves whatever it finds, so a new surface joins Functions by
// default and only a mistake puts it outside both.
test('the sidebar is two sections, with the thumbnails in Pages and the rest in Functions', () => {
  const tabs = [...doc.querySelectorAll('.sbtab')].map((t) => t.dataset.sbtab);
  assert.deepEqual(tabs, ['pages', 'functions'],
    `the sidebar offers ${JSON.stringify(tabs)} — this guard is written against exactly two sections`);

  const pages = doc.getElementById('sbPages');
  const functions = doc.getElementById('sbFunctions');
  assert.ok(pages && functions, 'one of the two section containers is missing from index.html');

  assert.ok(pages.contains(doc.getElementById('thumbs')),
    'the thumbnail panel is not inside the Pages section, so the Pages tab opens onto nothing');
  // The page-number readout came with the thumbnails when Previous/[n]/Next left the toolbar.
  assert.ok(pages.querySelector('.pageNum') && pages.querySelector('.pageCount'),
    'the page readout is not in the Pages section — it left the toolbar in v1.125.0 and this is where it went');

  const stray = [...doc.querySelectorAll('#sidebar .panel')]
    .filter((p) => !pages.contains(p) && !functions.contains(p))
    .map((p) => p.id);
  assert.deepEqual(stray, [],
    `these sidebar panels are in neither section: ${stray.join(', ')} — they are in the DOM and unreachable, which looks identical to not existing`);
});

test('the document title is in the bar and cannot fold away', () => {
  const title = doc.getElementById('docTitle');
  assert.ok(title, 'the document title is not in the toolbar');
  assert.ok(title.closest('.tbfixed'), 'the document title has left the fixed bar');
  // NOT inside a .tbgroup, deliberately: groups fold into ⋯ More at narrow widths, and a bar
  // that hides WHICH FILE you are editing is worse on a small window than on a large one.
  assert.equal(title.closest('.tbgroup'), null,
    'the document title is inside a fold group, so at a narrow width it disappears into ⋯ More — the one place the name matters most');
  assert.ok(doc.getElementById('docDirty'),
    'the save-state indicator is gone, so the title says which file is open and not whether it is saved');
});
