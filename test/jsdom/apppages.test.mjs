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

// `/api/settings` answers ok: the Signing tests below switch an Advanced feature and hide a mode.
const h = await boot({ routes: { '/api/settings': () => ({ status: 'ok' }) } });
const { document: doc } = h;
const CODE = fs.readFileSync(path.join(REPO, 'web', 'app.js'), 'utf8');

const NINE = ['Identity & Keys', 'Vault', 'Updates', 'Advanced features', 'Main menu', 'Appearance', 'Read Aloud', 'Colours', 'About'];
const settingsPane = () => doc.querySelector('.tbtab[data-tab="settings"]');
// The Settings pane's own: Signing has entries too (ADR-105), in its own pane.
const entries = () => [...settingsPane().querySelectorAll('.sbhead[data-entry]')];
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

// ── The Signing menu (ADR-105) ────────────────────────────────────────────────
//
// Dan, 2026-10-08: "The Signing menu should be the same, they should open in their own tab", and
// then the test for which entries: "Only the pages that requires configuration or runs a wizard
// should be opened in a tab. Any settings that are actions on an open document, like placing flags
// should be expandable in menu." So Simple Sign and the ceremony are pages; Place Signing Flags and
// Send & Receive are in the menu exactly as they were.
//
// What this tier cannot say: a real document beside a Signing page — a flag armed from the menu
// with the page in front, a tick that follows a real flag. `test/ui/signsteps.test.mjs`.
const signingPane = () => doc.querySelector('.tbtab[data-tab="collaborate"]');
const signingEntries = () => [...signingPane().querySelectorAll('.sbhead[data-entry]')];
const headOf = (label) => [...signingPane().querySelectorAll('.sbhead.groupcard')].find((x) => x.textContent.trim() === label);
const enterSigning = () => { doc.querySelector('.modetab[data-tab="file"]').click(); doc.querySelector('.modetab[data-tab="collaborate"]').click(); };
const closeEveryPage = () => { for (const t of tabs()) if (t.page) t.el.querySelector('.tabclose').click(); };
const SEND = ['sessionSendBtn', 'sessionRecvDocBtn', 'sessionInitBtn', 'sessionRecvBtn', 'cosignBtn'];

test('Signing: the guided path and the ceremony are entries with a page each; the actions on the document are not', () => {
  assert.deepEqual([...signingPane().querySelectorAll('.tbgroup')].map((g) => g.dataset.label), ['Simple Sign', 'Send & Receive', 'About Ceremonies']);
  for (const [label, id, title] of [['Simple Sign', 'signingStepsPage', 'Simple Sign'], ['About Ceremonies', 'signingCeremonyPage', 'Signing Ceremonies']]) {
    const g = signingPane().querySelector(`.tbgroup[data-label="${label}"]`);
    assert.ok(g.hasAttribute('data-entry'), `${label} is not an entry, so it expands in the menu`);
    const controls = [...g.querySelectorAll('button, input, select, textarea, label, p, div')].filter((c) => !c.classList.contains('menucap'));
    assert.deepEqual(controls.map((c) => c.dataset.apppage), [id], `${label} holds something other than the one button that opens its page`);
    const page = doc.getElementById(id);
    assert.ok(page.classList.contains('apppage') && page.parentElement.id === 'viewerCol', `${id} is not an app page in the main area`);
    assert.deepEqual([page.dataset.menu, page.dataset.group, page.dataset.title, page.getAttribute('role'), page.hidden],
      ['collaborate', 'Signing', title, 'region', true]);
    const heading = doc.getElementById(page.getAttribute('aria-labelledby'));
    assert.ok(heading && heading.tagName === 'H2' && heading.getAttribute('tabindex') === '-1' && page.contains(heading));
    assert.equal(heading.textContent.trim(), title);
  }
  // The checklist is on its page, once, under the id it had.
  assert.equal(doc.querySelectorAll('[id="signSteps"]').length, 1);
  assert.equal(doc.getElementById('signSteps').closest('.apppage')?.id, 'signingStepsPage', 'the checklist is not on the Simple Sign page');
  // Send & Receive: a card, in the menu, with the eight buttons it had.
  const send = signingPane().querySelector('.tbgroup[data-label="Send & Receive"]');
  assert.equal(send.hasAttribute('data-entry'), false, 'Send & Receive became an entry — it is actions on the open document and stays a card');
  assert.deepEqual([...send.querySelectorAll('button')].map((b) => b.id || `>${b.dataset.forward}`),
    [...SEND, '>timestampVerifyBtn', '>managePeersBtn', '>returnedBtn'], 'the Send & Receive card does not hold the eight buttons it held');
  for (const id of SEND) assert.equal(doc.getElementById(id).closest('.apppage'), null, `#${id} moved onto a page`);
  // Place Signing Flags: a panel, in the menu, with the six flags, its two acts and its three hints.
  const flags = doc.getElementById('flags');
  assert.ok(flags.classList.contains('panel') && flags.closest('#sidebar'), 'the flag tools left the sidebar');
  assert.deepEqual([...flags.querySelectorAll('.markers button')].map((b) => b.dataset.marker), ['sign', 'date', 'initial', 'name', 'title', 'company']);
  assert.deepEqual([...flags.querySelectorAll(':scope > button')].map((b) => b.id), ['signCompleteBtn', 'saveForSigningBtn']);
  assert.equal(flags.querySelectorAll('p.libhint').length, 3, 'the flag panel lost the words it had');
  assert.equal(doc.querySelector('.apppage [data-marker]'), null, 'a flag button is on a page');
});

test('entering Signing lands on Flags and opens no page and no tab; the two that expand still expand', () => {
  closeEveryPage();
  enterSigning();
  assert.deepEqual(pagesShown(), [], 'entering Signing put a page up');
  assert.equal(doc.getElementById('tabrow').hidden, true, 'entering Signing opened a tab');
  assert.ok(doc.getElementById('flags').classList.contains('active'), 'Signing did not land on the flag tools');
  assert.deepEqual(signingEntries().map((e) => e.textContent.trim()), ['Simple Sign', 'About Ceremonies']);
  for (const e of signingEntries()) assert.equal(e.hasAttribute('aria-expanded'), false, `${e.textContent.trim()} says it expands`);
  // Send & Receive expands and closes, in the menu, as a card does.
  const send = headOf('Send & Receive');
  assert.equal(send.getAttribute('aria-expanded'), 'false');
  send.click();
  assert.deepEqual([send.getAttribute('aria-expanded'), marked()], ['true', ['Send & Receive']], 'Send & Receive did not expand in the menu');
  assert.deepEqual(pagesShown(), [], 'Send & Receive opened a page');
  assert.equal(doc.getElementById('tabrow').hidden, true, 'Send & Receive opened a tab');
  send.click();
  assert.equal(send.getAttribute('aria-expanded'), 'false', 'the open card did not close');
  // And the flag panel's header toggles it. One card open at a time: Send & Receive opening closed it.
  const fh = doc.querySelector('.sbhead[data-panel="flags"]');
  assert.equal(fh.hidden, false);
  assert.equal(doc.getElementById('flags').classList.contains('active'), false, 'the flag panel stayed open beside an open card');
  fh.click();
  assert.ok(doc.getElementById('flags').classList.contains('active'), 'the flag panel did not open on its header');
  fh.click();
  assert.equal(doc.getElementById('flags').classList.contains('active'), false, 'the flag panel did not close on its header');
});

test('a Signing entry opens its page with one tab that says Signing; a second click makes no second; nothing expands for it', () => {
  enterSigning();
  for (const [i, e] of signingEntries().entries()) {
    const id = e.nextElementSibling.querySelector('button').dataset.apppage;
    const title = doc.getElementById(id).dataset.title;
    e.click();
    assert.deepEqual(pagesShown(), [id]);
    assert.equal(doc.activeElement, doc.getElementById(id).querySelector('h2'), `opening ${title} did not put focus on its heading`);
    assert.deepEqual(marked(), [], `${title} expanded in the menu as well`);
    assert.ok(doc.getElementById('flags').classList.contains('active'), `opening ${title} closed the flag tools beside it`);
    const mine = tabs().filter((x) => x.page === id);
    assert.equal(tabs().length, i + 1);
    assert.deepEqual([mine.length, mine[0].name, mine[0].kind, mine[0].selected, mine[0].label], [1, title, 'Signing', 'true', `${title}, Signing page`]);
    e.click();
    assert.equal(tabs().length, i + 1, `a second click on ${title} opened a second copy`);
  }
  // Re-entering the mode with its pages open opens nothing more and brings none forward.
  doc.querySelector('.modetab[data-tab="file"]').click();
  assert.deepEqual(pagesShown(), [], 'leaving Signing did not leave its page');
  doc.querySelector('.modetab[data-tab="collaborate"]').click();
  assert.deepEqual([pagesShown(), tabs().length], [[], 2], 'entering Signing opened or raised a page');
});

test('the checklist on its page: a phase heading before each group, the hint in words, and a tick made by hand', () => {
  enterSigning();
  signingEntries()[0].click();
  const host = doc.getElementById('signSteps');
  const rows = [...host.querySelectorAll('.signstep')];
  assert.equal(rows.length, 16, 'the checklist is not the sixteen steps');
  assert.deepEqual([...host.querySelectorAll('h3.signphase')].map((x) => x.textContent),
    ['Once, before your first signing', 'Prepare the document', 'Marks on the page', 'Seal it and keep it']);
  assert.equal(host.firstElementChild.tagName, 'H3', 'the list does not open with its first phase');
  const row = (label) => rows.find((r) => r.querySelector('.signstep-label').textContent === label);
  assert.equal(row('Redact, then apply').querySelector('.signstep-hint').textContent, 'Applying is irreversible — do it before you sign',
    'the warning about the one-way door is a tooltip only');
  assert.equal(row('Open the document').querySelector('.signstep-hint'), null, 'a step with no hint grew an empty line');
  assert.equal(row('Open the document').dataset.state, 'todo', 'no document is open and the step is not "not done yet"');
  // A hand tick re-renders the row, as hers.
  row('Mark it up').querySelector('.signstep-mark').click();
  const again = [...host.querySelectorAll('.signstep')].find((r) => r.querySelector('.signstep-label').textContent === 'Mark it up');
  assert.deepEqual([again.dataset.state, again.dataset.by], ['done', 'hand']);
  again.querySelector('.signstep-mark').click();
});

test('a step that leads to a tool for the document leaves the page; one that leads to a page brings that page forward', () => {
  enterSigning();
  signingEntries()[0].click();
  const go = (label) => [...doc.querySelectorAll('#signSteps .signstep-label')].find((b) => b.textContent === label).click();
  go('Plant flags for someone else'); // Signing's own panel: the mode does not change, so only the step can leave the page
  assert.deepEqual(pagesShown(), [], 'the Flags step left the checklist in front of the document the flags go on');
  assert.ok(doc.getElementById('flags').classList.contains('active'));
  assert.ok(tabs().some((t) => t.page === 'signingStepsPage'), 'the step closed the page instead of leaving it');
  tabs().find((t) => t.page === 'signingStepsPage').el.click();
  go('Verify a signature or timestamp'); // Signing's own card
  assert.deepEqual([pagesShown(), marked()], [[], ['Send & Receive']], 'the step did not open Send & Receive beside the document');
  tabs().find((t) => t.page === 'signingStepsPage').el.click();
  go('Enroll your key'); // a Settings entry: its page takes the checklist's place
  assert.deepEqual([pagesShown(), doc.body.dataset.tab], [['settingsIdentityPage'], 'settings']);
  closeEveryPage();
});

test('the ceremony page starts the two flows through the panel\'s own buttons', async () => {
  enterSigning();
  const entry = signingEntries().find((e) => e.textContent.trim() === 'About Ceremonies');
  entry.click();
  assert.deepEqual(pagesShown(), ['signingCeremonyPage']);
  doc.getElementById('cerPageAcceptBtn').click();
  assert.ok(doc.getElementById('ceremony').classList.contains('active'), 'Accept did not show the ceremony panel, where the invitation box is');
  assert.equal(doc.getElementById('ceremonyAcceptForm').hidden, false, 'Accept did not open the invitation box');
  assert.equal(doc.activeElement, doc.getElementById('cerInviteText'), 'focus is not in the invitation box');
  assert.deepEqual(pagesShown(), ['signingCeremonyPage'], 'accepting took the page away — the box is in the menu, beside it');
  doc.getElementById('cerAcceptCancel').click();
  doc.getElementById('cerPageConveneBtn').click();
  await h.settle();
  assert.equal(doc.getElementById('ceremonySheet').hidden, false, 'Convene did not raise the setup sheet');
  assert.deepEqual(pagesShown(), [], 'the page is still in front of the setup sheet');
  assert.ok(tabs().some((t) => t.page === 'signingCeremonyPage'), 'convening closed the page instead of leaving it');
  doc.getElementById('cerSheetClose').click();
  closeEveryPage();
});

test('a feature that is switched off: its entry goes, its page says so and offers the switch, and its step says so', async () => {
  enterSigning();
  signingEntries().find((e) => e.textContent.trim() === 'About Ceremonies').click();
  const page = doc.getElementById('signingCeremonyPage');
  const rows = () => ({
    live: [...page.querySelectorAll('[data-adv="ceremony"]')].map((r) => r.hidden),
    off: page.querySelector('[data-advoff="ceremony"]').hidden,
    entry: signingPane().querySelector('.tbgroup[data-label="About Ceremonies"]').hidden,
    head: headOf('About Ceremonies').hidden,
  });
  assert.deepEqual(rows(), { live: [false, false], off: true, entry: false, head: false }, 'setup: ceremonies are on in this harness');
  const tsRow = () => [...doc.querySelectorAll('#signSteps .signstep')].find((r) => r.querySelector('.signstep-label').textContent === 'Timestamp (OpenTimestamps)');
  assert.equal(tsRow().dataset.off, undefined);
  for (const id of ['advCeremonyChk', 'advTimestampChk']) { doc.getElementById(id).checked = false; }
  doc.getElementById('advCeremonyChk').onchange();
  await h.settle();
  assert.deepEqual(rows(), { live: [true, true], off: false, entry: true, head: true },
    'with ceremonies off the page still offers Convene and Accept, or does not say they are off, or the menu still offers the entry');
  assert.equal(doc.getElementById('ceremony').hidden, true, 'the live panel is offered with ceremonies off');
  assert.equal(tsRow().dataset.off, 'timestamp', 'the Timestamp step does not say timestamping is off');
  assert.match(tsRow().querySelector('.signstep-hint').textContent, /Switched off/);
  // The off row's button goes to the switch.
  page.querySelector('[data-advoff="ceremony"] button').click();
  assert.deepEqual([pagesShown(), doc.body.dataset.tab], [['settingsAdvancedPage'], 'settings']);
  for (const id of ['advCeremonyChk', 'advTimestampChk']) { doc.getElementById(id).checked = true; }
  doc.getElementById('advCeremonyChk').onchange();
  await h.settle();
  assert.deepEqual(rows(), { live: [false, false], off: true, entry: false, head: false });
  assert.equal(tsRow().dataset.off, undefined);
  closeEveryPage();
});

test('hiding the Signing menu closes its pages: no tab is left naming a mode the window no longer has', async () => {
  enterSigning();
  for (const e of signingEntries()) e.click();
  doc.querySelector('.modetab[data-tab="settings"]').click();
  entries().find((e) => e.textContent.trim() === 'Main menu').click();
  assert.deepEqual(tabs().map((t) => t.page).sort(), ['settingsMenuPage', 'signingCeremonyPage', 'signingStepsPage']);
  const box = doc.querySelector('.modeChk[data-mode="collaborate"]');
  box.checked = false;
  box.onchange();
  await h.settle();
  assert.equal(doc.querySelector('.modetab[data-tab="collaborate"]').hidden, true, 'stimulus: Signing was not hidden');
  assert.deepEqual(tabs().map((t) => t.page), ['settingsMenuPage'], 'a Signing page kept its tab after Signing was hidden');
  assert.deepEqual(pagesShown(), ['settingsMenuPage'], 'the page the box is on did not stay in front');
  box.checked = true;
  box.onchange();
  await h.settle();
  closeEveryPage();
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
