// A menu entry opens a PAGE in the main area (ADR-104), and the pages share ONE tab (ADR-107).
//
// Dan, 2026-10-08: "in the Settings menu, I would like each button to popup a nice page instead
// of expanding in the menu", then "I prefer new tabs/pages over popups"; and then "instead of each
// button getting it's own tab, let's use a single tab and when I click on a new button it replaces
// the content".
//
// ── Why this is tier 3 ────────────────────────────────────────────────────────
// Every claim here is one jsdom cannot make. A page beside a REAL document — the document still
// counted as one, coming back at the scroll and zoom it was left at, its controls inert while the
// page is in front — needs a document, and tier 2's pdf.js is a stub. Where focus lands is the
// browser's. Whether a page fits a 375px window is layout. And a setting "still set" is only
// worth saying after the real server has stored it and a reloaded window has read it back.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { tmpdir } from 'node:os';
import { mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { launch, shutdown } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const h = await launch();
const { page } = h;
after(() => shutdown(h));

// The seven entries, in the order the menu shows them (seven since ADR-109: Updates, Advanced features
// and Main menu are the sections of Toggle Features). Each opens a page — About too, since
// ADR-108; it opened a dialog until then.
const PAGES = [
  ['Appearance', 'settingsAppearancePage'], ['Colours', 'settingsColoursPage'],
  ['Read Aloud', 'settingsReadAloudPage'], ['Toggle Features', 'settingsFeaturesPage'],
  ['Identity & Keys', 'settingsIdentityPage'], ['Vault', 'settingsVaultPage'], ['About', 'settingsAboutPage'],
];
const ENTRIES = PAGES.map((p) => p[0]);
const ENTRY = '#commands .tbtab[data-tab="settings"] .sbhead.groupcard';
const MORE = '#toolbar .tbtab[data-tab="settings"] .tbmore';

// What the main area and the strip show, read off the screen.
const state = () => page.evaluate(() => {
  const vis = (e) => !!e && e.getClientRects().length > 0;
  const a = document.activeElement;
  return {
    pages: [...document.querySelectorAll('#viewerCol > .apppage')].filter(vis).map((p) => p.id),
    viewer: vis(document.getElementById('viewerWrap')),
    row: vis(document.getElementById('tabrow')),
    docTabs: [...document.querySelectorAll('#tabstrip .tab:not(.pagetab)')].map((t) => t.querySelector('.tabname').textContent),
    pageTabs: [...document.querySelectorAll('#tabstrip .pagetab')].map((t) => t.dataset.apppage),
    selected: [...document.querySelectorAll('#tabstrip .tab[aria-selected="true"]')].map((t) => t.dataset.apppage || t.querySelector('.tabname').textContent),
    closeAll: vis(document.getElementById('closeAllBtn')),
    hasDoc: document.getElementById('viewerWrap').classList.contains('has-doc'),
    dialogs: [...document.querySelectorAll('body > div[id$="Modal"]:not([hidden])')].map((m) => m.id),
    expanded: [...document.querySelectorAll('#commands .tbtab[data-tab="settings"] > .tbgroup')].filter(vis).map((g) => g.dataset.label),
    focus: a ? { tag: a.tagName, id: a.id, text: (a.textContent || '').trim().slice(0, 40), shown: vis(a), inStrip: !!a.closest('#tabstrip') } : null,
  };
});

test('entering Settings shows seven entries, in order, and opens nothing', async () => {
  await h.mode('settings');
  const entries = await page.$$eval(ENTRY, (es) => es.filter((e) => e.getClientRects().length).map((e) => ({
    text: e.textContent.trim(), tag: e.tagName, expanded: e.getAttribute('aria-expanded') })));
  assert.deepEqual(entries.map((e) => e.text), ENTRIES, 'the Settings menu is not the seven entries in their order');
  for (const e of entries) {
    assert.equal(e.tag, 'BUTTON', `${e.text} is not a button`);
    assert.equal(e.expanded, null, `${e.text} carries aria-expanded="${e.expanded}" — it announces a card that expands, and it opens a page`);
  }
  const s = await state();
  assert.deepEqual(s.pages, [], 'clicking the Settings tab put a page up');
  assert.equal(s.row, false, 'clicking the Settings tab showed the tab row with nothing open');
  assert.deepEqual(s.dialogs, [], 'clicking the Settings tab put a dialog up');
  assert.deepEqual(s.expanded, [], `a Settings entry is expanded in the menu on arrival: ${s.expanded.join(', ')}`);
});

test('each entry opens its page, named in the page tab, with no document open; a second click opens no second tab; × closes it', async () => {
  for (const [label, id] of PAGES) {
    assert.equal(await h.settingsPage(label), id, `${label} names a different page`);
    let s = await state();
    assert.deepEqual(s.pages, [id], `${label} did not put its page, and only its page, in the main area`);
    assert.equal(s.viewer, false, `the document area is still showing beside ${label}'s page`);
    assert.deepEqual(s.pageTabs, [id], `${label}'s page is not the one in the page tab`);
    assert.deepEqual(s.docTabs, [], `a document tab appeared for ${label}'s page — nothing is open`);
    assert.deepEqual(s.selected, [id], `${label}'s tab is not the selected one`);
    assert.equal(s.hasDoc, false, `${label}'s page made the app think a document is open`);
    assert.equal(s.closeAll, false, 'Close all is offered with no document open');
    assert.deepEqual(s.expanded, [], `${label} expanded in the menu as well`);
    assert.deepEqual(s.dialogs, [], `${label} opened a dialog`);
    assert.ok(s.focus.tag === 'H2' && s.focus.text === label, `opening ${label} put focus on ${s.focus.tag}#${s.focus.id} "${s.focus.text}", not on the page's heading`);
    const named = await page.$eval(`#${id}`, (p) => ({ role: p.getAttribute('role'), modal: p.getAttribute('aria-modal'),
      name: document.getElementById(p.getAttribute('aria-labelledby'))?.textContent.trim() }));
    assert.deepEqual(named, { role: 'region', modal: null, name: label }, `${label}'s page is not a region named by its heading`);
    const tab = await page.$eval(`#tabstrip .pagetab[data-apppage="${id}"]`, (t) => ({ role: t.getAttribute('role'), label: t.getAttribute('aria-label'),
      kind: t.querySelector('.tabkind').textContent, name: t.querySelector('.tabname').textContent, controls: t.getAttribute('aria-controls') }));
    assert.deepEqual(tab, { role: 'tab', label: `${label}, Settings page`, kind: 'Settings', name: label, controls: id });

    await page.click(`${ENTRY}:text-is("${label}")`);
    s = await state();
    assert.deepEqual(s.pageTabs, [id], `a second click on ${label} opened a second tab`);
    assert.deepEqual(s.pages, [id]);

    await h.closeAppPage(id);
    s = await state();
    assert.deepEqual(s.pages, [], `${label}'s × left its page up`);
    assert.equal(s.viewer, true, `after ${label}'s page closed the document area did not come back`);
    assert.equal(s.row, false, 'the tab row is still up with no tab in it');
    assert.ok(s.focus.shown && s.focus.tag !== 'BODY', `after ${label}'s page closed focus is on ${s.focus.tag}#${s.focus.id}, which is ${s.focus.shown ? 'the body' : 'not on screen'}`);
  }
});

// ── About is a page like the other eight (ADR-108) ───────────────────────────────────────────
// Dan, 2026-10-08: "About should use the same tab as the rest of the settings pages".
test('About is a page in the shared tab: it takes another page\'s place, shows the version, and there is no About dialog', async () => {
  await h.settingsPage('Vault');
  assert.equal(await h.settingsPage('About'), 'settingsAboutPage');
  let s = await state();
  assert.deepEqual([s.pages, s.pageTabs, s.selected, s.dialogs], [['settingsAboutPage'], ['settingsAboutPage'], ['settingsAboutPage'], []],
    'About did not take the one page tab from Vault, or it opened a dialog');
  assert.ok(s.focus.tag === 'H2' && s.focus.text === 'About', `opening About put focus on ${s.focus.tag}#${s.focus.id} "${s.focus.text}"`);
  const tab = await page.$eval('#tabstrip .pagetab', (t) => t.innerText.replace(/\s+/g, ' ').trim());
  assert.match(tab, /^SETTINGS About\b/, `the tab reads "${tab}"`);
  assert.equal(await page.$('#aboutModal'), null, 'the About dialog is still in the document');
  // The version the real server reports, on the page and on screen.
  const want = (await (await page.evaluate(() => window.nibFetch('/api/status').then((r) => r.json())))).version;
  const ver = await page.$eval('#aboutVersion', (e) => ({ text: e.textContent, shown: e.getClientRects().length > 0 }));
  assert.ok(want && ver.text === want && ver.shown, `the About page shows version "${ver.text}", and the server is "${want}"`);
  const seen = await page.$eval('#aboutMain', (m) => m.innerText);
  for (const claim of ['signed by whoever holds your signing key', 'two or more people', 'not a qualified electronic signature']) {
    assert.ok(seen.replace(/\s+/g, ' ').includes(claim), `the About page does not show "${claim}"`);
  }
  // Escape does nothing to a page (ADR-104 §6).
  await page.keyboard.press('Escape');
  s = await state();
  assert.deepEqual(s.pages, ['settingsAboutPage'], 'Escape closed the About page — it is not a dialog');
  await h.closeAppPage('settingsAboutPage');
});

// The About page keeps a document open across leaving and coming back, and the window is shared
// by every test in this file: a test that opened one puts it away, by the button, as a user would.
const aboutDocsAway = () => page.$$eval('#aboutDocs button[aria-expanded="true"]', (bs) => bs.forEach((b) => b.click()));

test('the licence and the notices open in place and are put away again, by mouse and by keyboard, with focus left on the button', async () => {
  await h.settingsPage('About');
  const doc = (id) => page.$eval(`#${id}`, (e) => ({ shown: e.getClientRects().length > 0, chars: e.textContent.length, head: e.textContent.slice(0, 400),
    kids: e.children.length, scrolls: e.scrollHeight > e.clientHeight, box: e.getBoundingClientRect().height, win: window.innerHeight }));
  const fetched = [];
  const seen = (r) => { if (r.url().includes('/legal/')) fetched.push(r.url().replace(/^.*\/legal\//, '')); };
  page.on('request', seen);
  try {
    await aboutDocsAway();
    assert.deepEqual([(await doc('aboutLicenseText')).shown, (await doc('aboutNoticesText')).chars], [false, 0], 'a document is on the page before it is asked for');
    // Mouse: the licence.
    await page.click('#aboutLicenseBtn');
    await page.waitForFunction(() => document.getElementById('aboutLicenseText').textContent.length > 1000);
    let d = await doc('aboutLicenseText');
    assert.ok(d.shown && /GNU AFFERO GENERAL PUBLIC LICENSE/.test(d.head) && d.kids === 0, `the licence is not showing as text: ${d.head.slice(0, 60)}`);
    assert.ok(d.scrolls && d.box <= 0.6 * d.win + 2, `the licence box is ${d.box}px tall in a ${d.win}px window and ${d.scrolls ? 'scrolls' : 'does not scroll'} — it should be a box that scrolls, not the whole text laid out`);
    assert.equal(await page.$eval('#aboutLicenseBtn', (b) => b.getAttribute('aria-expanded')), 'true');
    assert.equal((await state()).focus.id, 'aboutLicenseBtn', 'showing the licence moved focus off its button');
    await page.click('#aboutLicenseBtn');
    assert.deepEqual([(await doc('aboutLicenseText')).shown, await page.$eval('#aboutLicenseBtn', (b) => b.getAttribute('aria-expanded'))], [false, 'false'], 'the Licence button did not put the licence away');
    // Keyboard: Tab from the licence button reaches the notices button; Enter shows them; Tab
    // goes INTO the box, which the arrow keys then scroll; Shift+Tab and Enter put them away.
    assert.deepEqual(fetched, ['LICENSE'], 'the notices were fetched before anybody asked for them');
    await page.focus('#aboutLicenseBtn');
    await page.keyboard.press('Tab');
    assert.equal((await state()).focus.id, 'aboutNoticesBtn', 'Tab from the Licence button did not reach the Third-party notices button');
    await page.keyboard.press('Enter');
    await page.waitForFunction(() => document.getElementById('aboutNoticesText').textContent.length > 10000);
    d = await doc('aboutNoticesText');
    assert.ok(d.shown && d.kids === 0 && d.scrolls, 'the notices are not showing as text in a box that scrolls');
    assert.equal((await state()).focus.id, 'aboutNoticesBtn', 'Enter moved focus off the notices button');
    await page.keyboard.press('Tab');
    assert.equal((await state()).focus.id, 'aboutNoticesText', 'Tab from the button did not reach the notices — a keyboard user cannot scroll them');
    await page.keyboard.press('PageDown');
    await page.waitForFunction(() => document.getElementById('aboutNoticesText').scrollTop > 0);
    await page.keyboard.press('Shift+Tab');
    await page.keyboard.press('Enter');
    assert.deepEqual([(await doc('aboutNoticesText')).shown, (await state()).focus.id], [false, 'aboutNoticesBtn'], 'Enter on the button did not put the notices away with focus left on it');
    assert.deepEqual((await state()).pages, ['settingsAboutPage']);
  } finally {
    page.off('request', seen);
    await aboutDocsAway();
    await h.closeAppPages();
  }
});

const DOC_A = writeFixture('apppage-a.pdf', { pages: 6, label: 'page doc A' });
const DOC_B = writeFixture('apppage-b.pdf', { pages: 2, label: 'page doc B' });
// The document on screen: where it is scrolled to, how big its first page is drawn, which page.
const docView = () => page.evaluate(() => {
  const c = document.querySelector('.viewerContainer:not([hidden])');
  return { scrollTop: Math.round(c.scrollTop), pageWidth: Math.round(c.querySelector('.page').getBoundingClientRect().width),
    pageNumber: Number(document.querySelector('#sbPages .pageNum').value), zoom: document.getElementById('zoomPct').textContent };
});
// Controls that act on the document: Save, three of the DOC_REQUIRED set from three menus, and a drawing tool.
const docControls = () => page.evaluate(() => Object.fromEntries(
  ['saveBtn', 'printBtn', 'redactBtn', 'rotateLeftBtn', 'highlightToolBtn'].map((id) => [id, document.getElementById(id).disabled])));

test('a page beside a document: the document is still the one document, inert while the page is in front, and comes back as it was', async () => {
  await h.openDocument(DOC_A, 6);
  await page.click('#zoomInBtn');
  await page.click('#zoomInBtn');
  await h.gotoPage(4);
  await page.waitForFunction(() => document.querySelector('.viewerContainer:not([hidden])').scrollTop > 0);
  const before = await docView();
  assert.ok(before.scrollTop > 0 && before.pageNumber === 4, `setup: the document is not scrolled to page 4 (${JSON.stringify(before)})`);
  assert.deepEqual(await docControls(), { saveBtn: false, printBtn: false, redactBtn: false, rotateLeftBtn: false, highlightToolBtn: false },
    'setup: the document\'s controls are not enabled with it showing, so their going inert cannot be seen');

  const id = await h.settingsPage('Toggle Features');
  let s = await state();
  assert.deepEqual(s.docTabs.length, 1, `with a page open the strip counts ${s.docTabs.length} documents — one is open`);
  assert.deepEqual(s.pageTabs, [id]);
  assert.deepEqual(s.selected, [id], 'the document\'s tab still says it is the one in front');
  assert.equal(s.hasDoc, true, 'opening a page made the app think no document is open');
  assert.equal(s.closeAll, false, 'Close all is offered for one document and a page — a page is not a document');
  assert.deepEqual(await docControls(), { saveBtn: true, printBtn: true, redactBtn: true, rotateLeftBtn: true, highlightToolBtn: true },
    'a control that acts on the document is live while a page stands in front of it');
  // The bar's own document groups are out of reach, not merely grey: a press where Zoom in is
  // drawn reaches nothing. Quit is not the document's and stays.
  const bar = await page.evaluate(() => Object.fromEntries(['zoomInBtn', 'viewContinuousBtn', 'readAloudBtn', 'saveBtn', 'quitBtn']
    .map((i) => [i, !!document.getElementById(i).closest('.tbgroup').inert])));
  assert.deepEqual(bar, { zoomInBtn: true, viewContinuousBtn: true, readAloudBtn: true, saveBtn: true, quitBtn: false },
    'the fixed bar\'s document groups are not inert while a page is in front');
  const z = await page.$eval('#zoomInBtn', (b) => { const r = b.getBoundingClientRect(); return [r.left + r.width / 2, r.top + r.height / 2]; });
  await page.mouse.click(z[0], z[1]);
  // Keys that act on the document do nothing either. Focus on the page itself, not in a box.
  await page.focus(`#${id} h2`);
  await page.keyboard.press('PageDown');
  await page.keyboard.press('Control+=');
  // Escape neither closes a page nor leaves it: a page is not a dialog.
  await page.keyboard.press('Escape');
  s = await state();
  assert.deepEqual([s.pages, s.pageTabs], [[id], [id]], 'Escape put the page away');

  // Back by the document's own tab. Its NAME is what is clicked: the tab is wider than its words,
  // and for a name this long the middle of the tab is its ×.
  await page.click('#tabstrip .tab:not(.pagetab) .tabname');
  await page.waitForSelector(`#${id}`, { state: 'hidden' });
  s = await state();
  assert.equal(s.viewer, true, 'the document did not come back');
  assert.deepEqual(s.pageTabs, [id], 'going back to the document closed the page — it should stay open in its tab');
  assert.equal(s.selected.length === 1 && s.selected[0] !== id, true, 'the document\'s tab is not the selected one again');
  assert.deepEqual(await docView(), before, 'the document did not come back as it was left — scroll, zoom and page');
  assert.equal(await page.$eval('#zoomInBtn', (b) => !!b.closest('.tbgroup').inert), false, 'the document is back and Zoom is still out of reach');
  assert.deepEqual(await docControls(), { saveBtn: false, printBtn: false, redactBtn: false, rotateLeftBtn: false, highlightToolBtn: false },
    'the document is back and its controls are still inert');

  // Two documents and the page: Close all is about the documents, and leaves the page.
  await page.click(`#tabstrip .pagetab[data-apppage="${id}"]`);
  await page.waitForSelector(`#${id}:not([hidden])`);
  await h.openDocument(DOC_B, 2);
  s = await state();
  assert.deepEqual(s.pages, [], 'a document was opened and the page stayed in front of it');
  assert.equal(s.docTabs.length, 2);
  // Another entry, with the first page open BEHIND a document: its page takes the tab the first
  // one had — one page tab, where it was, after the documents' — and comes forward.
  const strip = () => page.$$eval('#tabstrip > *', (ts) => ts.map((t) => (t.classList.contains('pagetab') ? t.getAttribute('aria-label') : 'document')));
  assert.deepEqual(await strip(), ['document', 'document', 'Toggle Features, Settings page'], 'setup: the strip is not two documents and the Toggle Features page');
  await h.settingsPage('Vault');
  s = await state();
  assert.deepEqual(await strip(), ['document', 'document', 'Vault, Settings page'],
    'a second entry did not replace what the page tab shows — there are two page tabs, or the tab moved, or it still names the first page');
  assert.deepEqual([s.pages, s.selected], [['settingsVaultPage'], ['settingsVaultPage']], 'the page that took the tab was opened behind the document');
  assert.deepEqual([s.docTabs.length, s.pageTabs.length, s.closeAll], [2, 1, true], 'two documents and a page: Close all should be offered, for the documents');
  if (process.env.NIB_UI_SHOTS) await page.screenshot({ path: join(process.env.NIB_UI_SHOTS, 'tabs-two-documents-one-page.png') });

  // The document behind the page is closed by its own ×, and the one that takes its place is
  // activated BEHIND the page: it is not showing either, so nothing of its comes alive.
  await page.click('#tabstrip .tab:not(.pagetab):nth-child(2) .tabclose');
  await page.waitForFunction(() => document.querySelectorAll('#tabstrip .tab:not(.pagetab)').length === 1);
  s = await state();
  assert.deepEqual(s.pages, ['settingsVaultPage'], 'closing a document from under a page took the page out of the front');
  assert.equal(s.closeAll, false, 'one document is left and Close all is still offered');
  assert.deepEqual(await docControls(), { saveBtn: true, printBtn: true, redactBtn: true, rotateLeftBtn: true, highlightToolBtn: true },
    'a document activated behind a page brought its controls to life');

  // Ctrl+O is Open from wherever you are — here, from a page, with no mode change to leave it.
  // The document that opens is what was asked for, so the page yields to it.
  await page.focus('#settingsVaultPage h2');
  await page.keyboard.press('Control+o');
  await page.fill('#pathInput', DOC_B);
  await page.click('#openGo');
  await page.waitForFunction(() => document.querySelector('.pageCount').textContent === '/ 2'
    && document.querySelectorAll('#tabstrip .tab:not(.pagetab)').length === 2);
  // The load has finished (the page count is the new document's), so this is not a wait on the load.
  await page.waitForSelector('#settingsVaultPage', { state: 'hidden', timeout: 5000 })
    .catch(() => assert.fail('a document was opened from a page and the page stayed in front of it'));
  assert.equal((await state()).viewer, true);
  await page.click('#tabstrip .pagetab[data-apppage="settingsVaultPage"]');
  await page.waitForSelector('#settingsVaultPage:not([hidden])');
  await page.click('#closeAllBtn');
  await h.documentClosed();
  s = await state();
  assert.deepEqual([s.docTabs.length, s.pageTabs, s.closeAll, s.row], [0, ['settingsVaultPage'], false, true], 'Close all should close both documents and leave the page in its tab');
  assert.deepEqual(s.pages, ['settingsVaultPage'], 'Close all took the page out of the front');
  await h.closeAppPages();
});

// ── One tab for the pages (ADR-107) ───────────────────────────────────────────
//
// What the tier below cannot say: that the tab is drawn in the SAME PLACE when its page changes
// (a left edge), that a folder typed on the page being replaced reaches the real server, and that
// focus is on the new page's heading in a browser.
test('the pages share one tab: another entry replaces what it shows, where it was; the replaced page saved what was typed on it; Signing and Settings take turns in it', async () => {
  await h.openDocument(DOC_A, 6);
  const tab = () => page.$eval('#tabstrip', (strip) => {
    const ts = [...strip.querySelectorAll('.pagetab')];
    const t = ts[0];
    return { count: ts.length, index: [...strip.children].indexOf(t), of: strip.children.length, left: Math.round(t.getBoundingClientRect().left),
      page: t.dataset.apppage, kind: t.querySelector('.tabkind').textContent, name: t.querySelector('.tabname').textContent,
      label: t.getAttribute('aria-label'), controls: t.getAttribute('aria-controls'), selected: t.getAttribute('aria-selected'),
      close: t.querySelector('.tabclose').getAttribute('aria-label') };
  });
  const stored = () => page.evaluate(async () => (await (await window.nibFetch('/api/status')).json()).downloadDirSet || '');
  const good = tmpdir();
  try {
    await h.settingsPage('Toggle Features');
    await page.waitForFunction(() => /^Updates download to .+ — .+\.$/.test(document.getElementById('downloadDirWhere').textContent), null, { timeout: 15000 });
    const first = await tab();
    assert.deepEqual([first.count, first.index, first.of, first.label], [1, 1, 2, 'Toggle Features, Settings page'], 'setup: the strip is not one document and the Toggle Features page');

    // A folder typed, and another entry clicked straight from the box.
    await page.fill('#downloadDirInput', good);
    await h.settingsPage('Colours');
    let t = await tab();
    assert.deepEqual(t, { ...first, page: 'settingsColoursPage', name: 'Colours', label: 'Colours, Settings page', controls: 'settingsColoursPage', close: 'Close the Colours page' },
      'Colours did not take the tab Toggle Features had: a second page tab, or the tab moved, or it does not name the page showing');
    let s = await state();
    assert.deepEqual([s.pages, s.selected, s.docTabs.length], [['settingsColoursPage'], ['settingsColoursPage'], 1]);
    assert.ok(s.focus.tag === 'H2' && s.focus.text === 'Colours', `the page that took the tab has focus on ${s.focus.tag}#${s.focus.id} "${s.focus.text}", not on its heading`);
    await page.waitForFunction(async (g) => (await (await window.nibFetch('/api/status')).json()).downloadDirSet === g, good, { timeout: 15000 }).catch(() => {});
    assert.equal(await stored(), good, 'the Toggle Features page was replaced by another and the folder typed on it was never saved');

    // A Signing page takes the same tab, and says Signing.
    assert.equal(await h.appPage('collaborate', 'Simple Sign'), 'signingStepsPage');
    t = await tab();
    assert.deepEqual(t, { ...first, page: 'signingStepsPage', kind: 'Signing', name: 'Simple Sign', label: 'Simple Sign, Signing page', controls: 'signingStepsPage', close: 'Close the Simple Sign page' },
      'a Signing page did not take the tab a Settings page held');
    assert.deepEqual((await state()).pages, ['signingStepsPage']);

    // And a Settings page takes it back, from behind the document this time.
    await page.click('#tabstrip .tab:not(.pagetab) .tabname');
    await page.waitForSelector('#signingStepsPage', { state: 'hidden' });
    await h.settingsPage('Read Aloud');
    t = await tab();
    assert.deepEqual(t, { ...first, page: 'settingsReadAloudPage', name: 'Read Aloud', label: 'Read Aloud, Settings page', controls: 'settingsReadAloudPage', close: 'Close the Read Aloud page' },
      'a Settings page did not take the tab back from a Signing page that was behind the document');
    s = await state();
    assert.deepEqual([s.pages, s.viewer], [['settingsReadAloudPage'], false], 'the page that took the tab stayed behind the document');

    // The × closes the page, and with it the only page tab.
    await h.closeAppPage('settingsReadAloudPage');
    s = await state();
    assert.deepEqual([s.pages, s.pageTabs, s.viewer, s.docTabs.length], [[], [], true, 1], 'the × did not close the page and bring the document back');
  } finally {
    // Leave the shared server as it was found.
    await h.closeAppPages();
    const id = await h.settingsPage('Toggle Features');
    await page.fill('#downloadDirInput', '');
    await h.closeAppPage(id);
    await page.waitForFunction(async () => !((await (await window.nibFetch('/api/status')).json()).downloadDirSet), null, { timeout: 15000 });
    await h.closeDocument();
    await h.documentClosed();
  }
});

test('by keyboard: Enter on an entry opens its page; its tab is in the strip\'s arrow order with the document\'s; × by keyboard closes it', async () => {
  await h.openDocument(DOC_B, 2);
  await h.mode('settings');
  await page.focus(`${ENTRY}:text-is("Colours")`);
  await page.keyboard.press('Enter');
  await page.waitForSelector('#settingsColoursPage:not([hidden])');
  let s = await state();
  assert.ok(s.focus.tag === 'H2' && s.focus.text === 'Colours', `Enter on the entry put focus on ${s.focus.tag} "${s.focus.text}"`);
  // Tab from the heading reaches the page's first control.
  await page.keyboard.press('Tab');
  assert.equal(await page.evaluate(() => document.activeElement.matches('#settingsColoursPage input[name="cardhue"]')), true,
    'Tab from the heading did not reach the page\'s first control');
  // The strip is one tab stop, on the tab in front; arrows move along it across both kinds.
  const stops = await page.$$eval('#tabstrip .tab', (ts) => ts.map((t) => [t.classList.contains('pagetab'), t.tabIndex]));
  assert.deepEqual(stops, [[false, -1], [true, 0]], 'the strip\'s one tab stop is not the page\'s tab while the page is in front');
  await page.focus('#tabstrip .pagetab');
  await page.keyboard.press('ArrowLeft');
  assert.equal(await page.evaluate(() => document.activeElement.matches('#tabstrip .tab:not(.pagetab)')), true, 'ArrowLeft from the page\'s tab did not reach the document\'s');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#settingsColoursPage', { state: 'hidden' });
  s = await state();
  assert.equal(s.viewer, true, 'Enter on the document\'s tab did not bring the document back');
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#settingsColoursPage:not([hidden])');
  // Tab from the page's tab is its ×; Enter closes the page and focus stays in the strip.
  await page.keyboard.press('Tab');
  assert.equal(await page.evaluate(() => document.activeElement.matches('#tabstrip .pagetab .tabclose')), true, 'Tab from the page\'s tab did not reach its ×');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#tabstrip .pagetab', { state: 'detached' });
  s = await state();
  assert.ok(s.focus.inStrip && s.focus.shown, `after closing the page by keyboard focus is on ${s.focus.tag}#${s.focus.id}, not on a tab`);
  assert.equal(s.viewer, true);
  await h.closeDocument();
  await h.documentClosed();
});

test('a setting changed on a page is saved, and still set when the page is opened again and after a reload', async () => {
  const id = await h.settingsPage('Read Aloud');
  assert.equal(await page.$eval('#readAloudRateSel', (s) => s.value), '1', 'this server did not start at the default speed, so a change cannot be told from what was there');
  const saved = page.waitForResponse((r) => r.url().endsWith('/api/settings') && r.request().method() === 'POST');
  await page.selectOption('#readAloudRateSel', '1.25');
  assert.equal((await saved).status(), 200, 'the server did not take the setting');
  await h.closeAppPage(id);
  await h.settingsPage('Read Aloud');
  assert.equal(await page.$eval('#readAloudRateSel', (s) => s.value), '1.25', 'the speed was not still set when the page was opened again');

  await page.reload();
  await page.waitForFunction(() => document.getElementById('readAloudRateSel').value === '1.25', null, { timeout: 15000 });
  // Which pages are open is the window's, not the vault's: a reload starts with none.
  assert.deepEqual((await state()).pageTabs, []);

  // Leave the shared server as it was found.
  await h.settingsPage('Read Aloud');
  const back = page.waitForResponse((r) => r.url().endsWith('/api/settings') && r.request().method() === 'POST');
  await page.selectOption('#readAloudRateSel', '1');
  await back;
  await h.closeAppPage(id);
});

test('a download folder typed and then left is saved, or said to be refused — never dropped', async () => {
  const id = await h.settingsPage('Toggle Features');
  await page.waitForFunction(() => /^Updates download to .+ — .+\.$/.test(document.getElementById('downloadDirWhere').textContent), null, { timeout: 15000 });
  const good = tmpdir();
  const stored = () => page.evaluate(async () => (await (await window.nibFetch('/api/status')).json()).downloadDirSet || '');

  // A folder that is there, and the page closed straight from the box.
  await page.fill('#downloadDirInput', good);
  await h.closeAppPage(id);
  await page.waitForFunction(async (g) => (await (await window.nibFetch('/api/status')).json()).downloadDirSet === g, good, { timeout: 15000 })
    .catch(() => {});
  assert.equal(await stored(), good, 'the page was closed and the folder that had been typed was never saved');

  // A folder that is not there, and the page closed straight from the box: the refusal is said
  // where it can still be seen, and the box goes back to the folder that is stored.
  await h.settingsPage('Toggle Features');
  assert.equal(await page.inputValue('#downloadDirInput'), good, 'the folder is not in the box when the page is opened again');
  await page.fill('#downloadDirInput', join(good, 'no', 'such', 'folder'));
  await h.closeAppPage(id);
  await page.waitForFunction(() => /Download folder not changed\..*could not find that folder/.test(document.getElementById('toast')?.textContent || ''), null, { timeout: 15000 });
  assert.equal(await stored(), good, 'a refused folder replaced the stored one');
  await h.settingsPage('Toggle Features');
  assert.equal(await page.inputValue('#downloadDirInput'), good, 'after a refused folder the box does not show the folder that is stored');
  assert.equal(await page.isHidden('#downloadDirError'), true, 'the refusal is showing over a box that no longer holds the refused text');

  // Leave the shared server as it was found.
  await page.fill('#downloadDirInput', '');
  await h.closeAppPage(id);
  await page.waitForFunction(async () => !((await (await window.nibFetch('/api/status')).json()).downloadDirSet), null, { timeout: 15000 });
});

test('a button on a page opens its dialog over the page, and the dialog gives focus back to the button', async () => {
  const id = await h.settingsPage('Identity & Keys');
  for (const [btn, next, closer] of [['#managePeersBtn', 'peersModal', '#peersClose'], ['#manageKeysBtn', 'keysModal', '#keysClose']]) {
    await page.click(btn);
    await page.waitForSelector(`#${next}:not([hidden])`);
    await page.click(closer);
    await page.waitForSelector(`#${next}`, { state: 'hidden' });
    const s = await state();
    assert.deepEqual(s.pages, [id], `closing ${next} took the page with it`);
    assert.equal('#' + s.focus.id, btn, `after ${next} closed focus is on ${s.focus.tag}#${s.focus.id}, not on the button that opened it`);
  }
  await h.closeAppPage(id);
});

// Toggle Features' cards (ADR-111). The harness opens every card for the other tests; this one
// puts them as a new window has them and reads what is on screen.
test('a shut card shows its name and nothing in it; its header opens it, by mouse and by keyboard', async () => {
  const id = await h.settingsPage('Toggle Features');
  await page.evaluate(() => { for (const d of document.querySelectorAll('#settingsFeaturesPage details.setcard')) d.open = d.id === 'featuresMenuCard'; });
  const seen = () => page.evaluate(() => {
    // `checkVisibility`, not a rectangle: a shut <details> keeps its content laid out under
    // `content-visibility: hidden`, so every control in it still answers a rectangle while nothing is drawn.
    const vis = (e) => !!e && e.checkVisibility();
    return Object.fromEntries(['featuresMenuCard', 'featuresAdvancedCard', 'featuresUpdatesCard'].map((c) => {
      const d = document.getElementById(c);
      return [c, { name: vis(d.querySelector('summary h3')), said: vis(d.querySelector('.setcardsub')), inputs: [...d.querySelectorAll('input')].filter(vis).length }];
    }));
  });
  assert.deepEqual(await seen(), {
    featuresMenuCard: { name: true, said: true, inputs: 3 },
    featuresAdvancedCard: { name: true, said: true, inputs: 0 },
    featuresUpdatesCard: { name: true, said: true, inputs: 0 },
  }, 'a new window does not show Main menu open and the other two as their headers alone');
  if (process.env.NIB_UI_SHOTS) await page.screenshot({ path: join(process.env.NIB_UI_SHOTS, 'page-features-as-opened.png') });
  // Nothing in a shut card takes focus: Tab from its header goes to the next card's header.
  await page.focus('#featuresAdvancedCard > summary');
  await page.keyboard.press('Tab');
  assert.equal(await page.evaluate(() => document.activeElement.closest('details')?.id), 'featuresUpdatesCard', 'Tab from a shut card\'s header went into the card');
  await page.click('#featuresAdvancedCard > summary');
  assert.equal((await seen()).featuresAdvancedCard.inputs, 4, 'clicking the header did not open the card');
  assert.equal((await seen()).featuresMenuCard.inputs, 3, 'opening one card shut another');
  await page.focus('#featuresUpdatesCard > summary');
  await page.keyboard.press('Enter');
  assert.equal((await seen()).featuresUpdatesCard.inputs, 2, 'Enter on the header did not open the card');
  await page.keyboard.press('Enter');
  assert.equal((await seen()).featuresUpdatesCard.inputs, 0, 'Enter on the header did not shut the card again');
  // A box in a card works as it did on its own page.
  const was = await page.isChecked('.modeChk[data-mode="secure"]');
  await page.click('.modeChk[data-mode="secure"]');
  await page.waitForFunction((w) => document.querySelector('.modetab[data-tab="secure"]').hidden === w, was);
  await page.click('.modeChk[data-mode="secure"]');
  await page.waitForFunction((w) => document.querySelector('.modetab[data-tab="secure"]').hidden === !w, was);
  await h.closeAppPage(id);
});

test('with the sidebar shut the seven entries are in ⋯ More, and each opens its page', async () => {
  await h.mode('settings');
  await page.click('#toggleSidebarBtn');
  await page.waitForFunction(() => document.getElementById('sidebar').classList.contains('collapsed'));
  try {
    await page.click(`${MORE} .menutop`);
    const listed = await page.evaluate((sel) => {
      const vis = (e) => e.getClientRects().length > 0;
      const d = document.querySelector(sel + ' .dropdown');
      return { buttons: [...d.querySelectorAll('button')].filter(vis).map((b) => b.textContent.trim()),
        captions: [...d.querySelectorAll('.menucap')].filter(vis).length,
        other: [...d.querySelectorAll('input, select, p, label')].filter(vis).length };
    }, MORE);
    assert.deepEqual(listed.buttons, ENTRIES.map((n) => n + '…'), '⋯ More does not hold the seven entries in their order');
    assert.equal(listed.captions, 0, 'each entry is listed twice — a caption and a button of the same name');
    assert.equal(listed.other, 0, 'a setting is still drawn inside the menu');
    await page.keyboard.press('Escape');
    for (const [label, id] of PAGES) {
      await page.click(`${MORE} .menutop`);
      await page.click(`${MORE} .dropdown button:text-is("${label}…")`);
      await page.waitForSelector(`#${id}:not([hidden])`);
      const s = await state();
      assert.ok(s.pageTabs.length === 1 && s.pageTabs[0] === id && s.focus.tag === 'H2', `${label} from ⋯ More: tabs ${s.pageTabs.join(',')}, focus on ${s.focus.tag}`);
    }
  } finally {
    await h.closeAppPages();
    await h.showSidebar();
  }
});

test('at 414 and 375 pixels wide no page runs past the window, with the strip above it', async () => {
  // The longest thing any page shows is the line naming the download folder, and a path has no
  // spaces to break at. So the folder is a long one for the length of this test: 130 characters
  // in one name with no space and no hyphen to break at, which is wider than either window.
  const long = join(tmpdir(), 'nib_' + 'a_long_folder_name_with_no_space_in_it_'.repeat(3) + Date.now());
  mkdirSync(long, { recursive: true });
  const updates = await h.settingsPage('Toggle Features');
  await page.fill('#downloadDirInput', long);
  await page.focus('#settingsFeaturesPage h2');
  await page.waitForFunction((l) => document.getElementById('downloadDirWhere').textContent.includes(l), long, { timeout: 15000 });
  await h.closeAppPage(updates);
  // The mode is chosen before the window narrows: below 850px the mode strip is a dropdown.
  await h.mode('settings');
  try {
    for (const width of [414, 375]) {
      await page.setViewportSize({ width, height: 800 });
      await page.waitForFunction(() => document.getElementById('sidebar').classList.contains('collapsed'));
      for (const [label, id] of PAGES) {
        await page.click(`${MORE} .menutop`);
        await page.click(`${MORE} .dropdown button:text-is("${label}…")`);
        await page.waitForSelector(`#${id}:not([hidden])`);
        if (id === 'settingsFeaturesPage') {
          // The longest thing any page shows is the folder line, and a path has no spaces to break at.
          await page.waitForFunction(() => document.getElementById('downloadDirWhere').textContent.length > 0, null, { timeout: 15000 });
        }
        if (id === 'settingsAboutPage') {
          // About with BOTH documents showing: lines of 72 columns and more, set not to wrap.
          for (const [btn, text] of [['#aboutLicenseBtn', 'aboutLicenseText'], ['#aboutNoticesBtn', 'aboutNoticesText']]) {
            if (await page.$eval(btn, (b) => b.getAttribute('aria-expanded')) !== 'true') await page.click(btn);
            await page.waitForFunction((t) => document.getElementById(t).textContent.length > 1000, text);
          }
          const boxes = await page.$$eval('#settingsAboutPage .aboutdoc', (ps) => ps.map((e) => e.scrollWidth > e.clientWidth));
          assert.deepEqual(boxes, [true, true], `setup: at ${width}px a licence document fits its box, so this is not the wide case`);
        }
        const m = await page.evaluate((i) => {
          const p = document.getElementById(i); const c = p.getBoundingClientRect();
          const past = [...p.querySelectorAll('*')].filter((e) => {
            const b = e.getBoundingClientRect();
            return b.width > 0 && (b.right > c.right + 0.5 || b.left < c.left - 0.5);
          }).map((e) => `${e.tagName}#${e.id || e.className}`);
          return { left: c.left, right: c.right, win: window.innerWidth, sideways: p.scrollWidth - p.clientWidth,
            windowSideways: document.documentElement.scrollWidth - window.innerWidth, past };
        }, id);
        assert.ok(m.left >= 0 && m.right <= m.win, `at ${width}px ${label}'s page runs from ${m.left} to ${m.right} in a ${m.win}px window`);
        assert.ok(m.sideways <= 0, `at ${width}px ${label}'s page scrolls sideways by ${m.sideways}px`);
        assert.ok(m.windowSideways <= 0, `at ${width}px the window scrolls sideways by ${m.windowSideways}px with ${label}'s page up`);
        assert.deepEqual(m.past, [], `at ${width}px these run past the edge of ${label}'s page: ${m.past.join(', ')}`);
        if (id === 'settingsAboutPage') await aboutDocsAway();
      }
      // Seven entries, one tab — the last page's; the strip does not push the window sideways.
      assert.equal((await state()).pageTabs.length, 1, `at ${width}px seven entries left more than the one page tab`);
      assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), `at ${width}px the page tab pushes the window sideways`);
      await h.closeAppPages();
    }
  } finally {
    // Widening past 900 opens the sidebar by itself. Waited for, never toggled: a toggle that
    // raced the crossing would shut the sidebar it was meant to open.
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.waitForFunction(() => !document.getElementById('sidebar').classList.contains('collapsed'));
    await aboutDocsAway();
    await h.closeAppPages();
    // Leave the shared server as it was found.
    await h.settingsPage('Toggle Features');
    await page.fill('#downloadDirInput', '');
    await page.focus('#settingsFeaturesPage h2');
    await page.waitForFunction(() => !document.getElementById('downloadDirWhere').textContent.includes('a_long_folder_name'), null, { timeout: 15000 });
    await h.closeAppPages();
  }
});

// Pictures for a person to look at, never an assertion: NIB_UI_SHOTS names a folder and four
// pages are written into it in each theme. Unset — as it is in the tier — this does nothing.
test('pictures of the pages in both themes, when asked for', { skip: !process.env.NIB_UI_SHOTS }, async () => {
  const dir = process.env.NIB_UI_SHOTS;
  const start = await page.evaluate(() => document.documentElement.dataset.appearance);
  try {
    for (const theme of [start, start === 'light' ? 'dark' : 'light']) {
      await page.evaluate((t) => { document.documentElement.dataset.appearance = t; }, theme);
      for (const [label, file] of [['Toggle Features', 'features'], ['Identity & Keys', 'identity'], ['Colours', 'colours'], ['About', 'about']]) {
        await h.settingsPage(label);
        await page.screenshot({ path: join(dir, `page-${file}-${theme}.png`) });
      }
      // About with the notices showing, scrolled to them.
      await aboutDocsAway();
      await page.click('#aboutNoticesBtn');
      await page.waitForFunction(() => document.getElementById('aboutNoticesText').textContent.length > 1000);
      await page.$eval('#aboutNoticesBtn', (b) => b.scrollIntoView());
      await page.screenshot({ path: join(dir, `page-about-notices-${theme}.png`) });
      await page.click('#aboutNoticesBtn');
      await h.closeAppPages();
    }
  } finally {
    await page.evaluate((t) => { document.documentElement.dataset.appearance = t; }, start);
    await h.closeAppPages();
  }
});
