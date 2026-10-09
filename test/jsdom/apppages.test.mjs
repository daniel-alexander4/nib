// A menu entry opens a PAGE with its own tab; it does not expand in the menu and it is not a popup
// (ADR-104). Dan, 2026-10-08: "in the Settings menu, I would like each button to popup a nice page
// instead of expanding in the menu", then "I prefer new tabs/pages over popups".
//
// ── What this tier can say ────────────────────────────────────────────────────
// Which entry opens which page, that a page gets one tab and a second click does not make a
// second, that the row shows for a page with no document open, that nothing in the menu is marked
// open, and — read from the source — that the registry has the writers it says it has and that
// `#viewerWrap.hidden` has one.
//
// ── What it cannot, and who covers it ─────────────────────────────────────────
// A real document beside a page: the document count, Close all at two documents, the document
// coming back as it was, its controls going inert. The stubbed pdf.js here opens nothing. And
// where focus lands, and whether a page fits a 375px window. `test/ui/apppages.test.mjs`.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { boot, REPO } from './boot.mjs';

const h = await boot({});
const { document: doc } = h;
const CODE = fs.readFileSync(path.join(REPO, 'web', 'app.js'), 'utf8');

const NINE = ['Identity & Keys', 'Vault', 'Updates', 'Advanced features', 'Main menu', 'Appearance', 'Read Aloud', 'Colours', 'About'];
const settingsPane = () => doc.querySelector('.tbtab[data-tab="settings"]');
const entries = () => [...doc.querySelectorAll('#sidebar .sbhead[data-entry]')];
const pagesShown = () => [...doc.querySelectorAll('#viewerCol > .apppage')].filter((p) => !p.hidden).map((p) => p.id);
const tabs = () => [...doc.getElementById('tabstrip').children].map((t) => ({
  page: t.dataset.apppage || null, name: t.querySelector('.tabname').textContent, selected: t.getAttribute('aria-selected'),
  kind: t.querySelector('.tabkind')?.textContent || null, label: t.getAttribute('aria-label'), el: t,
}));
const marked = () => [...doc.querySelectorAll('#commands .tbgroup.open')].map((g) => g.dataset.label);
const openDialogs = () => [...doc.querySelectorAll('body > div[id$="Modal"]')].filter((m) => !m.hidden).map((m) => m.id);

test('Settings is nine entries and no settings: each group holds one button, and eight name a page that exists', () => {
  const groups = [...settingsPane().querySelectorAll('.tbgroup')];
  assert.deepEqual(groups.map((g) => g.dataset.label), NINE, 'the Settings pane is not the nine entries in their order');
  for (const g of groups) {
    assert.ok(g.hasAttribute('data-entry'), `${g.dataset.label} is not marked as an entry, so the accordion makes it a card that expands`);
    const controls = [...g.querySelectorAll('button, input, select, textarea, label, p')];
    assert.deepEqual(controls.map((c) => c.tagName), ['BUTTON'],
      `${g.dataset.label} holds ${controls.map((c) => c.tagName).join(', ')} in the menu — an entry is one button, and its settings are on its page`);
    if (g.dataset.label === 'About') { assert.equal(controls[0].id, 'aboutBtn'); continue; }
    const page = doc.getElementById(controls[0].dataset.apppage || '');
    assert.ok(page && page.classList.contains('apppage') && page.parentElement.id === 'viewerCol',
      `${g.dataset.label} names "${controls[0].dataset.apppage}", which is not an app page in the main area`);
    assert.equal(page.getAttribute('role'), 'region', `${g.dataset.label}'s page is not a region`);
    assert.equal(page.hasAttribute('aria-modal'), false, `${g.dataset.label}'s page says it is modal — it is a page, and the rest of the window is still there`);
    const heading = doc.getElementById(page.getAttribute('aria-labelledby'));
    assert.ok(heading && heading.tagName === 'H2' && heading.getAttribute('tabindex') === '-1' && page.contains(heading),
      `${g.dataset.label}'s page is not named by a heading of its own that focus can be put on`);
    assert.equal(heading.textContent.trim(), g.dataset.label, `${g.dataset.label}'s page is headed "${heading.textContent.trim()}"`);
    assert.equal(page.dataset.title, g.dataset.label, 'the tab would say something other than the entry');
    assert.equal(page.dataset.menu, 'settings');
    assert.equal(page.hidden, true, `${g.dataset.label}'s page is showing at boot`);
  }
});

test('every control that was a Settings card is on a page, once, under the id it had', () => {
  const ids = ['managePeersBtn', 'manageKeysBtn', 'backupBtn', 'restoreInput', 'autoUpdateChk', 'downloadDirInput', 'downloadDirWhere',
    'downloadDirError', 'advCeremonyChk', 'advDiscoveryChk', 'advRendezvousChk', 'advTimestampChk', 'advError', 'modeError',
    'readAloudVoiceSel', 'readAloudRateSel'];
  for (const id of ids) {
    assert.equal(doc.querySelectorAll(`[id="${id}"]`).length, 1, `#${id} is not in the document exactly once`);
    assert.ok(doc.getElementById(id).closest('.apppage'), `#${id} is not on a page`);
  }
  assert.equal(doc.querySelectorAll('.apppage .modeChk').length, 6, 'the six Main menu boxes are not on a page');
  // No document is open in this harness, and the boxes share `data-mode` with the annotation tools' buttons.
  assert.equal(doc.querySelectorAll('.modeChk:disabled').length, 0,
    'a Main menu box is disabled with no document open — it was taken for an annotation tool, and the menu cannot be cut until a file is opened');
  assert.equal([...doc.querySelectorAll('.modeChk')].filter((c) => c.onclick).length, 0, 'a Main menu box is wired as an annotation tool');
  assert.equal(doc.querySelectorAll('.apppage input[name="cardhue"]').length, 7, 'the seven Colours choices are not on a page');
  assert.ok(doc.querySelector('.apppage [data-forward="themeToggle"]'), 'the theme switch is not on a page');
  assert.equal(doc.querySelector('.apppage input[name="cardhue"]').closest('[role="radiogroup"]')?.getAttribute('aria-labelledby'), 'settingsColoursPageTitle');
  assert.equal(doc.querySelector('.apppage .modeChk').closest('[role="group"]')?.getAttribute('aria-labelledby'), 'settingsMenuPageTitle');
  assert.equal(doc.getElementById('downloadDirError').getAttribute('role'), 'alert', 'the folder refusal lost its role');
});

test('entering Settings opens nothing: no page, no tab, no dialog, no card', () => {
  doc.querySelector('.modetab[data-tab="file"]').click();
  doc.querySelector('.modetab[data-tab="settings"]').click();
  assert.deepEqual(pagesShown(), [], 'entering Settings put a page up');
  assert.equal(doc.getElementById('tabrow').hidden, true, 'entering Settings showed the tab row');
  assert.deepEqual(openDialogs(), []);
  assert.deepEqual(marked(), [], 'a card is marked open while Settings is showing');
  assert.deepEqual(entries().map((e) => e.textContent.trim()), NINE, 'the sidebar does not show the nine entries');
  for (const e of entries()) assert.equal(e.hasAttribute('aria-expanded'), false, `${e.textContent.trim()} says it expands`);
});

test('an entry opens its page in the main area with one tab; a second click brings it forward and makes no second tab', () => {
  const eight = entries().filter((e) => e.textContent.trim() !== 'About');
  for (const [i, e] of eight.entries()) {
    const name = e.textContent.trim();
    const id = e.nextElementSibling.querySelector('button').dataset.apppage;
    e.click();
    assert.deepEqual(pagesShown(), [id], `${name} did not put its page, and only its page, in the main area`);
    assert.equal(doc.getElementById('viewerWrap').hidden, true, `the viewer is still showing under ${name}'s page`);
    assert.equal(doc.getElementById('tabrow').hidden, false, `${name}'s page is open with no document and the tab row is hidden — it has no tab`);
    assert.equal(doc.activeElement, doc.getElementById(id).querySelector('h2'), `opening ${name} did not put focus on the page's heading`);
    assert.deepEqual(marked(), [], `${name} expanded in the menu as well`);
    assert.deepEqual(openDialogs(), [], `${name} opened a dialog`);
    const t = tabs();
    assert.equal(t.length, i + 1, `after opening ${name} the strip holds ${t.length} tabs — with no document open every tab is a page's`);
    const mine = t.filter((x) => x.page === id);
    assert.equal(mine.length, 1);
    assert.deepEqual([mine[0].name, mine[0].kind, mine[0].selected, mine[0].label], [name, 'Settings', 'true', `${name}, Settings page`]);
    assert.equal(t.filter((x) => x.selected === 'true').length, 1, 'more than one tab says it is the one in front');
  }
  // Every page is open now. Each entry again: it comes forward, and the strip does not grow.
  for (const e of eight) {
    const id = e.nextElementSibling.querySelector('button').dataset.apppage;
    e.click();
    assert.deepEqual(pagesShown(), [id]);
    assert.equal(tabs().length, 8, `a second click on ${e.textContent.trim()} opened a second copy`);
  }
  assert.equal(doc.getElementById('closeAllBtn').hidden, true, 'Close all is offered for pages — it closes documents, and none is open');
  assert.equal(doc.getElementById('viewerWrap').classList.contains('has-doc'), false, 'a page made the app think a document is open');
});

test('a page tab brings its page forward, and its × closes that page and no other', () => {
  const byName = (n) => tabs().find((t) => t.name === n);
  byName('Vault').el.click();
  assert.deepEqual(pagesShown(), ['settingsVaultPage'], 'clicking a page tab did not show its page');
  // The × of a page that is NOT in front: what is showing stays.
  byName('Colours').el.querySelector('.tabclose').click();
  assert.deepEqual(pagesShown(), ['settingsVaultPage'], 'closing another page changed what is showing');
  assert.equal(tabs().length, 7);
  assert.equal(byName('Colours'), undefined, 'the closed page still has a tab');
  // The × of the page in front: the document's place comes back (here, the empty state).
  byName('Vault').el.querySelector('.tabclose').click();
  assert.deepEqual(pagesShown(), []);
  assert.equal(doc.getElementById('viewerWrap').hidden, false, 'closing the page in front did not bring the document area back');
  assert.equal(tabs().filter((t) => t.selected === 'true').length, 0, 'a page that is not showing has the selected tab');
  // Close the rest; the row goes with the last one.
  for (const t of tabs()) t.el.querySelector('.tabclose').click();
  assert.equal(doc.getElementById('tabrow').hidden, true, 'the tab row is still up with nothing in it');
});

test('leaving the Settings menu leaves the page, which stays open in its tab', () => {
  doc.querySelector('.modetab[data-tab="settings"]').click();
  entries().find((e) => e.textContent.trim() === 'Advanced features').click();
  assert.deepEqual(pagesShown(), ['settingsAdvancedPage']);
  // A box on the page re-settles the sidebar (applyAdvanced) — that is not a mode change.
  doc.querySelector('.modetab[data-tab="markup"]').click();
  assert.deepEqual(pagesShown(), [], 'the page is still in front in Mark Up, whose tools act on the document');
  assert.deepEqual(tabs().map((t) => [t.name, t.selected]), [['Advanced features', 'false']], 'the page was closed, not left');
  tabs()[0].el.querySelector('.tabclose').click();
});

test('About opens the About dialog, and no page', () => {
  doc.querySelector('.modetab[data-tab="settings"]').click();
  entries().find((e) => e.textContent.trim() === 'About').click();
  assert.deepEqual(openDialogs(), ['aboutModal']);
  assert.deepEqual(pagesShown(), []);
  doc.getElementById('aboutClose').click();
});

// ── The registry and the main area have the writers they say they have ─────────
//
// Read from the source, because the property is about every line of it: a fifth site that pushed
// a page, or a fourth that wrote the viewer's `hidden`, would work on the day it was written.
function writersOf(re) {
  const lines = CODE.split('\n');
  const found = [];
  let fn = '(module)';
  lines.forEach((line) => {
    const m = /^(?:async )?function (\w+)\(/.exec(line);
    if (m) fn = m[1];
    if (line.trim().startsWith('//')) return;
    if (re.test(line)) found.push(fn);
  });
  return [...new Set(found)].sort();
}

test('the page registry is written by its own functions and by nothing else', () => {
  assert.deepEqual(writersOf(/(?<![.\w$]|let\s)activeAppPage\s*=(?!=)/), ['leaveAppPage', 'showAppPage'].sort(),
    'something else decides which page is in front');
  assert.deepEqual(writersOf(/(?<![.\w$])openAppPages\s*\.\s*(push|splice|pop|shift|unshift|sort|reverse|length\s*=(?!=))/),
    ['closeAppPage', 'openAppPage'].sort(), 'something else opens or closes a page');
});

test('what stands in the main area is written at one site', () => {
  assert.deepEqual(writersOf(/viewerWrap\.hidden\s*=(?!=)/), ['syncMainArea'],
    'the viewer is hidden or shown from somewhere other than syncMainArea — a page or a sheet in front can now be uncovered by it');
  assert.deepEqual(writersOf(/\bp\.hidden\s*=\s*p\s*!==\s*activeAppPage/), ['syncMainArea']);
});

test('a page is never a view', () => {
  // The three mutators of `views` are guarded in view.test.mjs. This is the other half: none of
  // the page functions reaches for `views` at all.
  for (const fn of ['openAppPage', 'showAppPage', 'leaveAppPage', 'closeAppPage', 'syncMainArea']) {
    const start = CODE.indexOf(`function ${fn}(`);
    assert.ok(start > 0, `${fn} is gone`);
    const body = CODE.slice(start, CODE.indexOf('\n}\n', start));
    assert.doesNotMatch(body, /(?<![.\w$])views\b/, `${fn} reads or writes views — a page is not a document`);
    assert.doesNotMatch(body, /\b(addView|removeView|resetViews|newView)\(/, `${fn} makes or drops a view`);
  }
});
