// A menu entry opens a PAGE in the main area; it does not expand in the menu and it is not a popup
// (ADR-104). Dan, 2026-10-08: "in the Settings menu, I would like each button to popup a nice page
// instead of expanding in the menu", then "I prefer new tabs/pages over popups".
//
// **The pages share ONE tab (ADR-107).** Dan, the same day: "instead of each button getting it's
// own tab, let's use a single tab and when I click on a new button it replaces the content".
//
// ── What this tier can say ────────────────────────────────────────────────────
// Which entry opens which page, that there is one page tab however many entries are clicked and
// that it names the page showing, that the page it replaced was left (its typed folder saved) and
// left once, that the row shows for a page with no document open, that nothing in the menu is
// marked open, and — read from the source — that the registry has the writers it says it has and
// that `#viewerWrap.hidden` has one.
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
// What was sent is kept: a page that is replaced must save the folder typed on it.
const posted = [];
// The two documents the About page shows (ADR-108) are answered as TEXT, and the licence's holds
// markup: it must arrive on the page as the characters it is. The notices can be made to fail.
const LICENCE = 'GNU AFFERO <b>GENERAL</b> PUBLIC LICENSE\n<img src=x onerror="window.pwned=1">\n';
let noticesStatus = 200;
const h = await boot({ routes: {
  '/api/settings': (o) => { posted.push(JSON.parse(o.body || '{}')); return { status: 'ok' }; },
  '/legal/LICENSE': () => new Response(LICENCE, { status: 200 }),
  '/legal/THIRD-PARTY-NOTICES.md': () => new Response('# Third-party notices\n', { status: noticesStatus }),
} });
const { document: doc } = h;
const CODE = fs.readFileSync(path.join(REPO, 'web', 'app.js'), 'utf8');

// In the menu's own order: how Nib looks and reads, what it does and offers, your identity and its backup, About.
// Seven since ADR-109: Updates, Advanced features and Main menu are the three sections of Toggle Features.
const ENTRIES = ['Appearance', 'Colours', 'Read Aloud', 'Toggle Features', 'Identity & Keys', 'Vault', 'About'];
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

test('Settings is seven entries and no settings: each group holds one button, and each names a page that exists', () => {
  const groups = [...settingsPane().querySelectorAll('.tbgroup')];
  assert.deepEqual(groups.map((g) => g.dataset.label), ENTRIES, 'the Settings pane is not the seven entries in their order');
  for (const g of groups) {
    assert.ok(g.hasAttribute('data-entry'), `${g.dataset.label} is not marked as an entry, so the accordion makes it a card that expands`);
    const controls = [...g.querySelectorAll('button, input, select, textarea, label, p')];
    assert.deepEqual(controls.map((c) => c.tagName), ['BUTTON'],
      `${g.dataset.label} holds ${controls.map((c) => c.tagName).join(', ')} in the menu — an entry is one button, and its settings are on its page`);
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
  assert.equal(doc.querySelectorAll('.apppage .modeChk').length, 4, 'the four Main menu boxes are not on a page');
  // No document is open in this harness, and the boxes share `data-mode` with the annotation tools' buttons.
  assert.equal(doc.querySelectorAll('.modeChk:disabled').length, 0,
    'a Main menu box is disabled with no document open — it was taken for an annotation tool, and the menu cannot be cut until a file is opened');
  assert.equal([...doc.querySelectorAll('.modeChk')].filter((c) => c.onclick).length, 0, 'a Main menu box is wired as an annotation tool');
  assert.equal(doc.querySelectorAll('.apppage input[name="cardhue"]').length, 7, 'the seven Colours choices are not on a page');
  assert.ok(doc.querySelector('.apppage [data-forward="themeToggle"]'), 'the theme switch is not on a page');
  assert.equal(doc.querySelector('.apppage input[name="cardhue"]').closest('[role="radiogroup"]')?.getAttribute('aria-labelledby'), 'settingsColoursPageTitle');
  assert.equal(doc.querySelector('.apppage .modeChk').closest('[role="group"]')?.getAttribute('aria-labelledby'), 'featuresMenuTitle');
  assert.equal(doc.getElementById('downloadDirError').getAttribute('role'), 'alert', 'the folder refusal lost its role');
});

test('entering Settings opens nothing: no page, no tab, no dialog, no card', () => {
  doc.querySelector('.modetab[data-tab="file"]').click();
  doc.querySelector('.modetab[data-tab="settings"]').click();
  assert.deepEqual(pagesShown(), [], 'entering Settings put a page up');
  assert.equal(doc.getElementById('tabrow').hidden, true, 'entering Settings showed the tab row');
  assert.deepEqual(openDialogs(), []);
  assert.deepEqual(marked(), [], 'a card is marked open while Settings is showing');
  assert.deepEqual(entries().map((e) => e.textContent.trim()), ENTRIES, 'the sidebar does not show the seven entries');
  for (const e of entries()) assert.equal(e.hasAttribute('aria-expanded'), false, `${e.textContent.trim()} says it expands`);
});

test('an entry opens its page in the main area; the pages share ONE tab, which names the page showing; a second click makes no second tab', () => {
  assert.equal(entries().length, 7, 'setup: the sidebar does not show seven entries');
  for (const e of entries()) {
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
    assert.deepEqual(t.map((x) => x.page), [id],
      `after opening ${name} the strip holds ${t.map((x) => x.page).join(', ')} — the pages share one tab, and it is ${name}'s now`);
    assert.deepEqual([t[0].name, t[0].kind, t[0].selected, t[0].label], [name, 'Settings', 'true', `${name}, Settings page`]);
    assert.equal(t[0].el.getAttribute('aria-controls'), id, 'the tab names another page as the one it shows');
    assert.equal(t[0].el.querySelector('.tabclose').getAttribute('aria-label'), `Close the ${name} page`);
    // The entry again: the page is the one open, so it stays and nothing is added.
    e.click();
    assert.deepEqual([pagesShown(), tabs().map((x) => x.page)], [[id], [id]], `a second click on ${name} opened a second copy`);
  }
  assert.equal(doc.getElementById('closeAllBtn').hidden, true, 'Close all is offered for pages — it closes documents, and none is open');
  assert.equal(doc.getElementById('viewerWrap').classList.contains('has-doc'), false, 'a page made the app think a document is open');
});

test('the page tab brings its page forward, and its × closes the page — in front or not', () => {
  const entry = (n) => entries().find((e) => e.textContent.trim() === n);
  // The × of the page in front (Colours, the last one opened above): the document's place comes
  // back (here, the empty state) and the row goes with its only tab.
  tabs()[0].el.querySelector('.tabclose').click();
  assert.deepEqual(pagesShown(), []);
  assert.equal(doc.getElementById('viewerWrap').hidden, false, 'closing the page in front did not bring the document area back');
  assert.equal(doc.getElementById('tabrow').hidden, true, 'the tab row is still up with nothing in it');
  // Left, the page keeps the tab, which no longer says it is in front; the tab brings it back.
  entry('Vault').click();
  doc.querySelector('.modetab[data-tab="markup"]').click();
  assert.deepEqual([pagesShown(), tabs().map((t) => [t.page, t.selected])], [[], [['settingsVaultPage', 'false']]], 'a page that is not showing has the selected tab, or lost its tab');
  tabs()[0].el.click();
  assert.deepEqual(pagesShown(), ['settingsVaultPage'], 'clicking the page tab did not show its page');
  // The × of the page while it is NOT in front: it closes, and what is showing stays.
  doc.querySelector('.modetab[data-tab="file"]').click();
  tabs()[0].el.querySelector('.tabclose').click();
  assert.deepEqual([pagesShown(), tabs().length], [[], 0], 'the × did not close a page that was behind');
  assert.equal(doc.getElementById('viewerWrap').hidden, false);
  assert.equal(doc.getElementById('tabrow').hidden, true);
  doc.querySelector('.modetab[data-tab="settings"]').click();
});

// ── One tab: a page that is opened takes the place of the page that was open (ADR-107) ──────
//
// The page being replaced is LEFT, by the same door as any page that stops being on screen, and
// left once. Toggle Features is the one with something to lose (its Updates section): a folder typed in its box saves
// on `change`, which a click on another entry does not fire in this harness (and a keyboard
// shortcut does not fire in a browser), so `appPageLeave` saves it as the page goes.
test('a page replaced by another is left, once: the folder typed on Toggle Features is saved, and not saved again', async () => {
  const entry = (n) => entries().find((e) => e.textContent.trim() === n);
  const folders = () => posted.filter((b) => 'downloadDir' in b).map((b) => b.downloadDir);
  const box = doc.getElementById('downloadDirInput');
  await h.settle();
  posted.length = 0;

  // Replaced while it is in front.
  entry('Toggle Features').click();
  box.value = '/h/typed-in-front';
  entry('Vault').click();
  await h.settle();
  assert.deepEqual([pagesShown(), tabs().map((t) => t.page)], [['settingsVaultPage'], ['settingsVaultPage']], 'Vault did not take the tab Toggle Features had');
  assert.deepEqual(folders(), ['/h/typed-in-front'],
    'the Toggle Features page was replaced by another and the folder typed on it was dropped, or saved more than once');

  // Replaced while it is open BEHIND what the main area shows: it was left when it went behind,
  // and is not left a second time. Text put in the box after that is how a second leave would show.
  entry('Toggle Features').click();
  box.value = '/h/typed-then-left';
  doc.querySelector('.modetab[data-tab="markup"]').click();
  await h.settle();
  assert.deepEqual([pagesShown(), tabs().map((t) => [t.page, t.selected]), folders().slice(1)],
    [[], [['settingsFeaturesPage', 'false']], ['/h/typed-then-left']], 'setup: leaving the menu did not leave the page with its folder saved');
  box.value = '/h/a-second-leave-would-send-this';
  doc.querySelector('.modetab[data-tab="settings"]').click();
  assert.deepEqual(pagesShown(), [], 'setup: entering Settings brought the page forward by itself');
  entry('Read Aloud').click();
  await h.settle();
  // And the new page comes FORWARD: it does not take the tab and stay behind, as the old one was.
  assert.deepEqual([pagesShown(), tabs().map((t) => [t.page, t.selected, t.label])],
    [['settingsReadAloudPage'], [['settingsReadAloudPage', 'true', 'Read Aloud, Settings page']]],
    'a page opened while another was open behind did not replace it and come forward');
  assert.equal(doc.activeElement, doc.querySelector('#settingsReadAloudPage h2'), 'the page that took the tab did not get focus on its heading');
  assert.deepEqual(folders().slice(2), [], 'a page that had already been left was left again when it was replaced');
  box.value = '';
  tabs()[0].el.querySelector('.tabclose').click();
});

// ── Toggle Features: three collapsible cards, in an order (ADR-111) ─────────────────────────
// The MARKUP's own state, read before anything has opened a card: jsdom does not lay a shut
// <details> out, so what is on screen is tier 3's; what is open at first and what order things
// are in is here.
test('Toggle Features is three cards in order — Main menu open, the others shut — and each card holds its own switches in order', () => {
  const page = doc.getElementById('settingsFeaturesPage');
  const cards = [...page.querySelectorAll('details.setcard')];
  assert.deepEqual(cards.map((c) => [c.id, c.querySelector('summary h3').textContent, c.hasAttribute('open')]),
    [['featuresMenuCard', 'Main menu', true], ['featuresAdvancedCard', 'Advanced features', false], ['featuresUpdatesCard', 'Updates', false]],
    'the cards are not Main menu, Advanced features, Updates in that order, with only the first open');
  for (const c of cards) {
    assert.equal(c.firstElementChild.tagName, 'SUMMARY', `${c.id} does not start with its summary, so it has no name to press`);
    assert.ok(c.querySelector('summary .setcardsub')?.textContent.trim(), `${c.id}'s header does not say what is in it while it is shut`);
    assert.ok(c.querySelector('.setcardbody > .pagelead'), `${c.id} lost its own sentence about what off means (ADR-109 §2)`);
  }
  const inCard = (id) => [...doc.getElementById(id).querySelectorAll('input')].map((i) => i.id || i.dataset.mode);
  assert.deepEqual(inCard('featuresMenuCard'), ['edit', 'accessibility', 'secure', 'collaborate'], 'the Main menu boxes are not in the menu\'s own order');
  assert.deepEqual([...doc.querySelectorAll('.modetab')].map((t) => t.dataset.tab).filter((t) => !['file', 'markup', 'settings'].includes(t)),
    inCard('featuresMenuCard'), 'the boxes are in a different order from the tabs they hide');
  assert.deepEqual(inCard('featuresAdvancedCard'), ['advDiscoveryChk', 'advRendezvousChk', 'advCeremonyChk', 'advTimestampChk'],
    'the advanced switches are not nearest reach first, with ceremonies after the two that find a peer');
  assert.deepEqual(inCard('featuresUpdatesCard'), ['autoUpdateChk', 'downloadDirInput']);
  // Nothing on the page is outside a card but its name and lead.
  assert.deepEqual([...page.querySelectorAll('input')].filter((i) => !i.closest('details.setcard')).length, 0, 'a switch is outside every card');
});

test('leaving the Settings menu leaves the page, which stays open in the page tab', () => {
  doc.querySelector('.modetab[data-tab="settings"]').click();
  entries().find((e) => e.textContent.trim() === 'Toggle Features').click();
  assert.deepEqual(pagesShown(), ['settingsFeaturesPage']);
  // A box on the page re-settles the sidebar (applyAdvanced) — that is not a mode change.
  doc.querySelector('.modetab[data-tab="markup"]').click();
  assert.deepEqual(pagesShown(), [], 'the page is still in front in Mark Up, whose tools act on the document');
  assert.deepEqual(tabs().map((t) => [t.name, t.selected]), [['Toggle Features', 'false']], 'the page was closed, not left');
  tabs()[0].el.querySelector('.tabclose').click();
});

// ── About is a page like the other eight (ADR-108) ───────────────────────────────────────────
//
// Dan, 2026-10-08: "About should use the same tab as the rest of the settings pages". It was a
// dialog — the one entry ADR-104 left alone.
test('About opens the About page in the one shared tab, in place of the page that was there; there is no About dialog', async () => {
  const entry = (n) => entries().find((e) => e.textContent.trim() === n);
  doc.querySelector('.modetab[data-tab="settings"]').click();
  entry('Vault').click();
  entry('About').click();
  assert.deepEqual(pagesShown(), ['settingsAboutPage'], 'About did not put its page, and only its page, in the main area');
  assert.deepEqual(openDialogs(), [], 'About opened a dialog');
  assert.deepEqual(tabs().map((t) => [t.page, t.kind, t.name, t.selected, t.label]),
    [['settingsAboutPage', 'Settings', 'About', 'true', 'About, Settings page']], 'About did not take the tab Vault had, or the tab does not read "Settings About"');
  assert.equal(doc.activeElement, doc.getElementById('settingsAboutPageTitle'), 'opening About did not put focus on its heading');
  assert.equal(doc.getElementById('aboutModal'), null, 'the About dialog is still in the document');
  for (const id of ['aboutClose', 'aboutBackBtn', 'aboutTitle', 'aboutDocText', 'aboutBtn']) assert.equal(doc.getElementById(id), null, `#${id} belonged to the dialog and is still here`);
  // What the dialog said is on the page, and the version the server reported is in it.
  await h.settle();
  const main = doc.getElementById('aboutMain');
  assert.ok(main && main.closest('#settingsAboutPage'), 'the account of what a signature proves is not on the About page');
  assert.equal(doc.getElementById('aboutVersion').textContent, 'test', 'the About page does not show the running version');
  assert.deepEqual([...main.querySelectorAll('h3')].map((x) => x.textContent),
    ['What a Nib signature proves', "What it doesn't prove", 'Co-signing with other people', 'Working with several documents']);
  // Left for another menu, the entry brings it forward again and adds nothing.
  doc.querySelector('.modetab[data-tab="markup"]').click();
  doc.querySelector('.modetab[data-tab="settings"]').click();
  assert.deepEqual(pagesShown(), [], 'setup: the page came forward by itself');
  entry('About').click();
  assert.deepEqual([pagesShown(), tabs().map((t) => t.page)], [['settingsAboutPage'], ['settingsAboutPage']], 'a second click on About did not bring its one page forward');
});

test('the licence and the notices open in place when asked for, as text, and are put away again; nothing is fetched before', async () => {
  const legal = () => h.calls.filter((c) => c.url.includes('/legal/')).map((c) => c.url.replace(/^.*\/legal\//, ''));
  const lic = doc.getElementById('aboutLicenseBtn'); const licText = doc.getElementById('aboutLicenseText');
  const not = doc.getElementById('aboutNoticesBtn'); const notText = doc.getElementById('aboutNoticesText');
  assert.ok(lic.closest('#settingsAboutPage') && not.closest('#settingsAboutPage'), 'the two document buttons are not on the About page');
  assert.deepEqual(legal(), [], 'a licence document was fetched before anybody asked for it');
  assert.deepEqual([licText.hidden, notText.hidden, lic.getAttribute('aria-expanded'), not.getAttribute('aria-expanded'), licText.textContent, notText.textContent],
    [true, true, 'false', 'false', '', ''], 'a document is showing, or said to be, before its button is pressed');
  assert.deepEqual([lic.getAttribute('aria-controls'), not.getAttribute('aria-controls')], ['aboutLicenseText', 'aboutNoticesText']);

  lic.focus();
  lic.click();
  assert.deepEqual([licText.hidden, lic.getAttribute('aria-expanded'), notText.hidden], [false, 'true', true], 'the Licence button did not show the licence, and only the licence');
  await h.settle();
  assert.deepEqual(legal(), ['LICENSE'], 'the Licence button did not fetch the licence, once');
  assert.equal(licText.textContent, LICENCE, 'the licence on the page is not the text that was served');
  assert.equal(licText.children.length, 0, 'the licence was parsed as markup — a served file put elements on the page');
  assert.equal(h.document.defaultView.pwned, undefined);
  assert.equal(doc.activeElement, lic, 'showing the licence moved focus off its button');
  assert.deepEqual(pagesShown(), ['settingsAboutPage'], 'showing the licence left the page');
  assert.deepEqual(openDialogs(), []);

  // The notices beside it; a refusal from the server is said in words, in place.
  noticesStatus = 404;
  not.click();
  await h.settle();
  assert.deepEqual([notText.hidden, not.getAttribute('aria-expanded'), notText.textContent, licText.hidden], [false, 'true', 'Could not load document.', false]);
  // Put away: hidden, said so, and emptied; asked for again, fetched again.
  not.click();
  assert.deepEqual([notText.hidden, not.getAttribute('aria-expanded'), notText.textContent], [true, 'false', ''], 'the notices were not put away by their button');
  noticesStatus = 200;
  not.click();
  await h.settle();
  assert.equal(notText.textContent, '# Third-party notices\n');
  assert.deepEqual(legal(), ['LICENSE', 'THIRD-PARTY-NOTICES.md', 'THIRD-PARTY-NOTICES.md']);
  not.click();
  lic.click();
  assert.deepEqual([licText.hidden, lic.getAttribute('aria-expanded'), licText.textContent], [true, 'false', '']);
  tabs()[0].el.querySelector('.tabclose').click();
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
  assert.deepEqual([...signingPane().querySelectorAll('.tbgroup')].map((g) => g.dataset.label), ['Simple Sign', 'Send & Receive', 'Start or join a ceremony']);
  for (const [label, id, title] of [['Simple Sign', 'signingStepsPage', 'Simple Sign'], ['Start or join a ceremony', 'signingCeremonyPage', 'Start or join a ceremony']]) {
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
  assert.deepEqual(signingEntries().map((e) => e.textContent.trim()), ['Simple Sign', 'Start or join a ceremony']);
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

test('a Signing entry opens its page in the one page tab, which says Signing; a second click makes no second; nothing expands for it', () => {
  enterSigning();
  for (const e of signingEntries()) {
    const id = e.nextElementSibling.querySelector('button').dataset.apppage;
    const title = doc.getElementById(id).dataset.title;
    e.click();
    assert.deepEqual(pagesShown(), [id]);
    assert.equal(doc.activeElement, doc.getElementById(id).querySelector('h2'), `opening ${title} did not put focus on its heading`);
    assert.deepEqual(marked(), [], `${title} expanded in the menu as well`);
    assert.ok(doc.getElementById('flags').classList.contains('active'), `opening ${title} closed the flag tools beside it`);
    const t = tabs();
    assert.deepEqual(t.map((x) => x.page), [id], `${title} did not take the one page tab`);
    assert.deepEqual([t[0].name, t[0].kind, t[0].selected, t[0].label], [title, 'Signing', 'true', `${title}, Signing page`]);
    e.click();
    assert.deepEqual(tabs().map((x) => x.page), [id], `a second click on ${title} opened a second copy`);
  }
  // Re-entering the mode with its page open opens nothing more and does not bring it forward.
  doc.querySelector('.modetab[data-tab="file"]').click();
  assert.deepEqual(pagesShown(), [], 'leaving Signing did not leave its page');
  doc.querySelector('.modetab[data-tab="collaborate"]').click();
  assert.deepEqual([pagesShown(), tabs().map((x) => x.page)], [[], ['signingCeremonyPage']], 'entering Signing opened or raised a page');
});

test('the one tab is shared across menus: a Signing page takes a Settings page\'s place, and a Settings page takes it back', () => {
  closeEveryPage();
  doc.querySelector('.modetab[data-tab="settings"]').click();
  entries().find((e) => e.textContent.trim() === 'Appearance').click();
  assert.deepEqual(tabs().map((t) => [t.page, t.kind]), [['settingsAppearancePage', 'Settings']]);
  doc.querySelector('.modetab[data-tab="collaborate"]').click();
  assert.deepEqual([pagesShown(), tabs().map((t) => [t.page, t.selected])], [[], [['settingsAppearancePage', 'false']]], 'setup: leaving Settings did not leave its page in the tab');
  signingEntries()[0].click();
  assert.deepEqual([pagesShown(), tabs().map((t) => [t.page, t.kind, t.selected, t.label])],
    [['signingStepsPage'], [['signingStepsPage', 'Signing', 'true', 'Simple Sign, Signing page']]],
    'a Signing page did not take the tab a Settings page held — there are two page tabs, or the old one');
  doc.querySelector('.modetab[data-tab="settings"]').click();
  entries().find((e) => e.textContent.trim() === 'Appearance').click();
  assert.deepEqual([pagesShown(), tabs().map((t) => [t.page, t.kind, t.selected, t.label])],
    [['settingsAppearancePage'], [['settingsAppearancePage', 'Settings', 'true', 'Appearance, Settings page']]],
    'a Settings page did not take the tab back from a Signing page');
  closeEveryPage();
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
  go('Enroll your key'); // a Settings entry: its page takes the checklist's place, in the tab as on screen
  assert.deepEqual([pagesShown(), doc.body.dataset.tab, tabs().map((t) => t.page)], [['settingsIdentityPage'], 'settings', ['settingsIdentityPage']]);
  closeEveryPage();
});

test('the ceremony page starts the two flows through the panel\'s own buttons', async () => {
  enterSigning();
  const entry = signingEntries().find((e) => e.textContent.trim() === 'Start or join a ceremony');
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
  signingEntries().find((e) => e.textContent.trim() === 'Start or join a ceremony').click();
  const page = doc.getElementById('signingCeremonyPage');
  const rows = () => ({
    live: [...page.querySelectorAll('[data-adv="ceremony"]')].map((r) => r.hidden),
    off: page.querySelector('[data-advoff="ceremony"]').hidden,
    entry: signingPane().querySelector('.tbgroup[data-label="Start or join a ceremony"]').hidden,
    head: headOf('Start or join a ceremony').hidden,
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
  assert.deepEqual([pagesShown(), doc.body.dataset.tab], [['settingsFeaturesPage'], 'settings']);
  // …and to the CARD the switch is in, opened, with focus on it (ADR-111): that card is shut at first.
  assert.equal(doc.getElementById('featuresAdvancedCard').open, true, 'the off row went to Toggle Features and left the card holding the switch shut');
  assert.equal(doc.activeElement, doc.querySelector('#featuresAdvancedCard > summary'), 'focus is not on the card the off row went to');
  doc.getElementById('featuresAdvancedCard').open = false;
  for (const id of ['advCeremonyChk', 'advTimestampChk']) { doc.getElementById(id).checked = true; }
  doc.getElementById('advCeremonyChk').onchange();
  await h.settle();
  assert.deepEqual(rows(), { live: [false, false], off: true, entry: false, head: false });
  assert.equal(tsRow().dataset.off, undefined);
  closeEveryPage();
});

// **The box is on a Settings page, and opening that page now takes the Signing page's tab** — so
// through the window, a Signing page is never open when Signing is hidden from this box (the
// second half below). The rule is still needed where the menu is hidden with the page open: the
// server's answer to a setting changed elsewhere reaches `applyModeVisibility` the same way. Here
// that is the box's handler run with the Signing page open, which is the same call.
test('hiding the Signing menu closes its page: no tab is left naming a mode the window no longer has', async () => {
  const box = doc.querySelector('.modeChk[data-mode="collaborate"]');
  const hideSigning = async (hide) => { box.checked = !hide; box.onchange(); await h.settle(); };
  enterSigning();
  signingEntries()[0].click();
  assert.deepEqual([pagesShown(), tabs().map((t) => t.page)], [['signingStepsPage'], ['signingStepsPage']]);
  await hideSigning(true);
  assert.equal(doc.querySelector('.modetab[data-tab="collaborate"]').hidden, true, 'stimulus: Signing was not hidden');
  assert.deepEqual([pagesShown(), tabs().map((t) => t.page)], [[], []], 'a Signing page kept its tab after Signing was hidden');
  assert.equal(doc.getElementById('tabrow').hidden, true);
  await hideSigning(false);

  // From the page the box is on: that page has the tab, and hiding Signing leaves it alone.
  enterSigning();
  signingEntries()[0].click();
  doc.querySelector('.modetab[data-tab="settings"]').click();
  entries().find((e) => e.textContent.trim() === 'Toggle Features').click();
  assert.deepEqual(tabs().map((t) => t.page), ['settingsFeaturesPage'], 'the Toggle Features page did not take the Signing page\'s tab');
  await hideSigning(true);
  assert.deepEqual([pagesShown(), tabs().map((t) => t.page)], [['settingsFeaturesPage'], ['settingsFeaturesPage']], 'the page the box is on did not stay in front');
  await hideSigning(false);
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

test('the page registry is written by its own functions and by nothing else, and nothing adds a second page to it', () => {
  assert.deepEqual(writersOf(/(?<![.\w$]|let\s)activeAppPage\s*=(?!=)/), ['leaveAppPage', 'showAppPage'].sort(),
    'something else decides which page is in front');
  assert.deepEqual(writersOf(/(?<![.\w$])openAppPages\s*\.\s*(push|splice|pop|shift|unshift|sort|reverse|length\s*=(?!=))/),
    ['closeAppPage', 'openAppPage'].sort(), 'something else opens or closes a page');
  // One tab (ADR-107): nothing ADDS to the registry. The one site that puts a page in it replaces
  // whatever it held, so a second page tab cannot be made by any path through the code.
  assert.deepEqual(writersOf(/(?<![.\w$])openAppPages\s*\.\s*(push|unshift)\b/), [], 'a page is added to the registry beside the one it holds — that is a second page tab');
  assert.match(CODE, /if \(!openAppPages\.includes\(p\)\) openAppPages\.splice\(0, openAppPages\.length, p\);/,
    'openAppPage no longer replaces the open page with the one asked for');
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
