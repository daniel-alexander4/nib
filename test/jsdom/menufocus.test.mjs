// /pending 327 — closing a dropdown must not strand the keyboard user on `<body>`.
//
// `closeMenu()` only stripped `.open`, so the element holding focus went `display: none`
// and the browser dropped focus to the document. WCAG SC 2.4.3, measured three ways in
// the browser walk that filed it: Export→body, ⚙→body, and Save-as via click-outside.
//
// The repo had already solved this once. The dialog focus-restore block says, in its own
// comment, that a menu collapsing under an opener leaves focus "in a `display: none`
// subtree, where .focus() is a silent no-op and the browser drops focus to <body>. That
// is the same SC 2.4.3 harm this code exists to remove, relocated." It had a door for the
// 38 dialogs and none for the 4 menus — ADR-009's shape.
//
// ── What this tier can reach ─────────────────────────────────────────────────
// The attributes and the focus target, because the app is booted and the DOM is real
// enough for `contains` and `activeElement`. The ARIA is stamped at BOOT from the live
// DOM, so this has to boot rather than scan index.html — a static scan would be red
// forever, which is the argument the dialog guard already makes for itself.
//
// ── What it cannot, and who covers it ────────────────────────────────────────
// jsdom does not blur a focused element when its ancestor is hidden; a real browser does.
// So the DEFECT itself is invisible here — this file can only assert that the restore
// happens. `test/ui/dialogfocus.test.mjs` is where the blur is real, and the dialog
// comment records that exactly this difference is what made tier 2 green against the
// dialog version of this bug.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

const h = await boot({});
const { document: doc } = h;

const menus = () => [...doc.querySelectorAll('.menu')];
const triggerOf = (m) => m.querySelector('.menutop');

test('every menu trigger says it opens a menu', () => {
  const ms = menus();
  assert.ok(ms.length >= 4, `only ${ms.length} menus found — this guard is reading nothing`);
  for (const m of ms) {
    const t = triggerOf(m);
    assert.ok(t, 'a .menu has no .menutop trigger');
    assert.equal(t.getAttribute('aria-haspopup'), 'true',
      `${t.textContent.trim()} does not announce that it opens a menu`);
    assert.equal(t.getAttribute('aria-expanded'), 'false',
      `${t.textContent.trim()} does not report its closed state`);
  }
});

test('opening and closing a menu keeps aria-expanded in step', () => {
  const m = menus().find((x) => triggerOf(x).textContent.includes('Export')) || menus()[0];
  const t = triggerOf(m);
  t.click();
  assert.equal(t.getAttribute('aria-expanded'), 'true',
    'the menu is open and its trigger still reports closed');
  t.click();
  assert.equal(t.getAttribute('aria-expanded'), 'false',
    'the menu is closed and its trigger still reports open');
});

test('closing a menu returns focus to its trigger, not to the document', () => {
  // A menu with an ENABLED item, discovered rather than named. With nothing open most
  // dropdown items are in DOC_REQUIRED and therefore disabled, and a disabled button
  // cannot take focus — so picking Export by name gave a test whose setup silently held
  // focus on <body> and whose assertion would then have passed for the wrong reason. The
  // setup assertion below is what caught that.
  const pick = menus()
    .map((m) => ({ m, item: m.querySelector('.dropdown button:not([disabled])') }))
    .find((x) => x.item);
  assert.ok(pick, 'no menu has an enabled item with nothing open — this guard is reading nothing');
  const { m, item } = pick;
  const t = triggerOf(m);
  t.click();
  item.focus();
  assert.ok(m.contains(doc.activeElement),
    'setup: focus is not inside the open menu, so the restore below would be measuring nothing');

  t.click(); // close
  assert.equal(doc.activeElement, t,
    `closing the menu left focus on <${doc.activeElement?.tagName?.toLowerCase()}> instead of the trigger — a keyboard user is stranded at the top of the document and must Tab back through everything`);
});

test('closing a menu does NOT steal focus that has already moved away', () => {
  // The other direction, and it is the one that would regress the app rather than the
  // reader. A dropdown item's own onclick runs BEFORE the menubar's delegated handler, so
  // a command that opens a dialog and focuses a field synchronously has already moved
  // focus out by the time closeMenu runs. Restoring unconditionally would yank the user
  // back out of the dialog they just opened.
  const m = menus()[0];
  const t = triggerOf(m);
  t.click();
  const outside = doc.getElementById('pathInput') || doc.querySelector('input');
  assert.ok(outside, 'no control outside the menu to focus — this case is untested');
  outside.focus();
  assert.ok(!m.contains(doc.activeElement), 'setup: focus is still inside the menu');

  t.click(); // close
  assert.equal(doc.activeElement, outside,
    'closing the menu pulled focus back to the trigger even though focus had already moved elsewhere — a command that opens a dialog would be yanked out of it');
});
