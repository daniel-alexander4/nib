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
import fs from 'node:fs';
import path from 'node:path';
import { boot, REPO } from './boot.mjs';

const h = await boot({});
const doc = h.document;
// SIGN_STEPS is module-scope in app.js and not exposed, so it is read from the source the same
// way doccontrols reads DOC_REQUIRED and modes reads SIDEBAR_FOR.
const APP_SRC = fs.readFileSync(path.join(REPO, 'web', 'app.js'), 'utf8');

// The fold ranks app.js declares. **Rank 0 means "never folds" ONLY in the fixed bar** — see the
// exemption below, which used to state it unconditionally and was wrong for 25 of the 28 groups.
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
  // ── The rank-0 exemption is a FIXED-BAR exemption (/pending 384) ────────────
  //
  // This used to skip every `data-fold="0"` group on the stated grounds that a rank-0 group
  // never folds. That is `applyFold`'s rule and `applyFold` governs the fixed bar alone
  // (`app.js`: `all('#toolbar .tbfixed')`). `foldAll` — the sidebar-closed path — puts EVERY
  // registered group into its pane's ⋯ More "ignoring the width ladder", rank 0 included, and
  // its own comment says a rank-0 group left unregistered "would have no home to return to".
  //
  // So a rank-0 `.tbtab` group holding a dropdown WOULD nest a menu inside ⋯ More whenever the
  // sidebar is shut, and the guard was written not to look at exactly those four groups
  // (Detect & Fill Fields, Annotate & Draw, Redact Content, Sign & Timestamp). No such group
  // exists today; an unsound exemption is how a guard decays into one, which is why this is a
  // fix rather than a note.
  const exemptCount = [];
  for (const g of doc.querySelectorAll('.tbgroup[data-fold]')) {
    if (g.dataset.fold === '0' && g.closest('.tbfixed')) { exemptCount.push(g.dataset.label); continue; }
    const controls = g.querySelectorAll('button, select, input');
    assert.ok(controls.length > 0,
      `foldable group "${g.dataset.label}" holds no controls — it would put an empty heading in the ⋯ More menu`);
    // A .menu inside ⋯ More would be a menu within a menu, and the bar tracks one open menu
    // at a time (app.js's openMenu).
    assert.equal(g.querySelector('.menu'), null,
      `foldable group "${g.dataset.label}" contains a dropdown. Folding it would nest a menu inside the ⋯ More menu, and only one menu can be open at a time — move the dropdown out, or pin the group to the fixed bar at rank 0`);
  }
  // The exemption is still real and still small. An exemption that stops matching reads exactly
  // like a clean run — the lesson `docattach_test.go` states for its own exempt map. Measured
  // 2026-09-08: Reload and Save. The guard's own prose used to cite "Recent / Save as / Export",
  // which have not been the bar's rank-0 groups for some time.
  assert.deepEqual(exemptCount.sort(), ['Reload', 'Save'],
    `the fixed bar's rank-0 groups are now ${JSON.stringify(exemptCount)} — the exemption skips the dropdown check for these, so a change to the set is a change to what this guard does not look at`);
});

// ── The fold RANK's value is a fixed-bar concern, and 25 of 28 groups carry an inert one ──
//
// /pending 384. `data-fold` does two jobs and only one of them is per-group:
//
//   - Its PRESENCE registers a group as foldable. `buildOverflowMenus` and `foldAll` both select
//     `.tbgroup[data-fold]`, so a group without the attribute gets no `_home`, no menu caption,
//     and never folds — it would sit in the bar when the sidebar is shut. That is load-bearing
//     for all 28.
//   - Its VALUE is a threshold index into `foldThresholds`, read at exactly ONE place, inside
//     `applyFold`, which iterates `#toolbar .tbfixed` and nothing else. So it decides something
//     for the three groups in the bar and nothing at all for the 25 in the panes.
//
// The 25 inert values are deliberately left as they are, and this test is why. Deleting the
// attribute breaks folding; rewriting them all to 0 would assert "never folds", which is true in
// the bar and false in every pane. The honest move was to make the claim checkable instead — so
// if a second reader of `foldThresholds` ever appears, the ladder is live for the panes again and
// 25 numbers that mean nothing today suddenly mean something, in an order nobody chose.
test('the fold ladder has exactly one reader, and it reads the fixed bar', () => {
  const reads = [...APP_SRC.matchAll(/foldThresholds\s*\[/g)];
  assert.equal(reads.length, 1,
    `foldThresholds is looked up ${reads.length} times; it was 1 when /pending 384 measured the 25 inert ranks. A second reader means the width ladder now governs something beyond the fixed bar — every .tbtab group's data-fold value becomes live, and those 25 values were never chosen for an order`);

  // The one reader's enclosing sweep. Asserted as the query rather than as a line number,
  // because the claim is "the ladder only ever sees the fixed bar" and that is what the query says.
  const applyFold = APP_SRC.slice(APP_SRC.indexOf('function applyFold()'));
  const body = applyFold.slice(0, applyFold.indexOf('\n}\n') + 3);
  assert.ok(body.length > 200 && body.includes('foldThresholds['),
    `applyFold's body did not parse out of the source (${body.length} chars) — this assertion is reading nothing`);
  assert.ok(body.includes("all('#toolbar .tbfixed')"),
    "applyFold no longer sweeps `#toolbar .tbfixed`. The width ladder's scope is what makes 25 groups' data-fold values inert; if it has widened, style.css's `data-fold=\"0\" means never folds` and this file's rank-0 exemption both need re-deriving");
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

// ── The Simple Sign checklist (v1.127.0) ─────────────────────────────────────
//
// The list is DECLARED in app.js and rendered; the markup holds only an empty container, because
// a row written into index.html would be a claim about progress that nothing checks.
//
// What this tier can hold is the declaration's shape: every step names itself, says whether it is
// required, and goes somewhere. What it cannot hold is whether a tick is TRUE — that needs a real
// document in a real browser, and `test/ui/signsteps.test.mjs` drives it.
const STEP_SRC = (() => {
  const at = APP_SRC.indexOf('const SIGN_STEPS = [');
  assert.notEqual(at, -1, 'SIGN_STEPS is not in web/app.js — this scan is reading nothing');
  return APP_SRC.slice(at, APP_SRC.indexOf('\n];', at));
})();

test('every sign step declares a label, a need, and somewhere to go', () => {
  const steps = [...STEP_SRC.matchAll(/\{\s*label: '([^']+)'[\s\S]*?need: '([a-z]+)'/g)]
    .map((m) => ({ label: m[1], need: m[2] }));
  assert.ok(steps.length >= 10,
    `found ${steps.length} steps in SIGN_STEPS — the list is the whole feature, so this guard must be reading it`);
  const badNeed = steps.filter((s) => s.need !== 'required' && s.need !== 'optional');
  assert.deepEqual(badNeed, [],
    `these steps declare a need that is neither required nor optional: ${badNeed.map((s) => s.label).join(', ')}`);
  // A step with nowhere to go is a line of text pretending to be a link.
  const gos = [...STEP_SRC.matchAll(/go: \(\) =>/g)].length;
  assert.equal(gos, steps.length,
    `${steps.length} steps declare a label but only ${gos} declare a go — a row with no destination is text wearing a link's clothes`);
  assert.ok(steps.some((s) => s.need === 'required') && steps.some((s) => s.need === 'optional'),
    'every step carries the same need, so the distinction the list exists to draw is not being drawn');
});

test('a step is ticked only where something can observe it', () => {
  // `done` is a probe or it is literally `null`. The null is the honest half of this feature —
  // Nib cannot know whether you ran a hidden-content scan or where you saved an .ots — and a
  // `done` that always returns true would be a permanent tick nothing backs.
  const dones = [...STEP_SRC.matchAll(/done: (null|\(\) => [^,]+),/g)].map((m) => m[1]);
  assert.ok(dones.length >= 10, `only ${dones.length} steps declare a done — the scan is not reading them`);
  const alwaysTrue = dones.filter((d) => /^\(\) => (true|1)$/.test(d.trim()));
  assert.deepEqual(alwaysTrue, [],
    'a step declares `done: () => true`, which is a tick that nothing observes. Use null — the list says "Nib cannot tell" and means it');
  assert.ok(dones.includes('null'),
    'no step is declared untracked. Some of these Nib genuinely cannot see, and claiming otherwise is what this guard exists to stop');
});
