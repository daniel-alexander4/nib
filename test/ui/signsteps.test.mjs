// The Simple Sign checklist tells the truth about what is done.
//
// Tier 2 holds the DECLARATION — every step names itself, says whether it is required, goes
// somewhere, and is either observable or honestly marked untracked. What it cannot hold is whether
// a tick is TRUE, because every probe reads live state: a document being open, a stamp on a page,
// a signature on the file. That is this file.
//
// The property is one-directional and worth stating plainly: a step must not read `done` until the
// thing is actually done. A list that ticks early is worse than no list, because its whole purpose
// is answering "what is left before I sign".
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { join } from 'node:path';
import { launch, shutdown } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const h = await launch();
const { page } = h;
after(() => shutdown(h));

const DOC = writeFixture('signsteps.pdf', { pages: 2, label: 'steps page' });

// The checklist is on the Simple Sign PAGE (ADR-105), opened by its entry in the Signing menu. The rows are the
// `.signstep` children: the list also holds a heading before each phase, which is not a step.
const steps = async () => {
  await h.appPage('collaborate', 'Simple Sign');
  return page.evaluate(() => [...document.querySelectorAll('#signSteps .signstep')].map((r) => ({
    label: r.querySelector('.signstep-label').textContent,
    state: r.dataset.state,
    need: r.dataset.need,
  })));
};
const stateOf = (rows, label) => rows.find((r) => r.label === label)?.state;

test('the checklist renders every declared step, with its need', async () => {
  const rows = await steps();
  assert.ok(rows.length >= 10, `the checklist rendered ${rows.length} rows — the list is the feature`);
  assert.ok(rows.some((r) => r.need === 'required'), 'no step is marked required');
  assert.ok(rows.some((r) => r.state === 'untracked'),
    'no step is shown as untracked. Nib cannot observe several of these, and the dash is how it says so rather than showing an empty circle that reads as "not done yet"');
});

test('a step flips to done only when the thing is actually done', async () => {
  // The stimulus, and the direction that matters: it must read NOT done first, or "it says done
  // after" is true of a row that always said done.
  const before = await steps();
  assert.equal(stateOf(before, 'Open the document'), 'todo',
    'the checklist says a document is open before one has been opened');
  assert.equal(stateOf(before, 'Finalize & sign'), 'todo',
    'the checklist says the document is signed before anything has been signed');

  await h.openDocument(DOC, 2);
  // **Read where it is, behind the document, before anything comes forward (ADR-105).** A page coming forward
  // repaints the list, so `steps()` here would read a list that had just been repainted for the occasion and
  // could not tell one that follows the document from one that does not.
  const behindDoc = await page.evaluate(() => ({
    hidden: document.getElementById('signingStepsPage').hidden,
    rows: [...document.querySelectorAll('#signSteps .signstep')].map((r) => ({
      label: r.querySelector('.signstep-label').textContent, state: r.dataset.state, need: r.dataset.need })),
  }));
  assert.equal(behindDoc.hidden, true, 'stimulus: the page is in front, so the list below may have been repainted by coming forward');
  assert.equal(stateOf(behindDoc.rows, 'Open the document'), 'done',
    'a document is open and the checklist still shows that step as outstanding — the list is not reading live state, so every tick in it is decoration');
  const after = await steps();
  assert.equal(stateOf(after, 'Open the document'), 'done',
    'a document is open and the checklist still shows that step as outstanding — the list is not reading live state, so every tick in it is decoration');
  // …and the steps that are NOT done must not have moved with it.
  assert.equal(stateOf(after, 'Finalize & sign'), 'todo',
    'opening a document marked "Finalize & sign" as done. A checklist that ticks early is worse than no checklist');

  // **The list tracks while it is the thing on screen (ADR-105).** A page coming forward repaints the list, so
  // "open a document, then look" is true of a list that never repaints by itself. The document is closed by its
  // tab's × BEHIND the page — the page stays in front — and the row has to follow with nothing coming forward.
  h.answerDialogs(true);
  await page.click('#tabstrip .tab:not(.pagetab) .tabclose');
  await h.documentClosed();
  const behind = await page.evaluate(() => ({
    front: !document.getElementById('signingStepsPage').hidden,
    state: [...document.querySelectorAll('#signSteps .signstep')]
      .find((r) => r.querySelector('.signstep-label').textContent === 'Open the document').dataset.state,
  }));
  assert.equal(behind.front, true, 'stimulus: closing the document took the page out of the front, so the list was repainted by coming back');
  assert.equal(behind.state, 'todo',
    'the document was closed behind the page and the checklist still shows that step as done — the list is not reading live state, so every tick in it is decoration');
  await h.closeAppPages();
});

test('a step link goes where it says', async () => {
  // The rows are links; a link that lands nowhere is the failure this catches. Finalize lives in
  // Secure → Sign & Timestamp, so clicking its row must leave the app there with the card open.
  const rows = await steps();
  const i = rows.findIndex((r) => r.label === 'Finalize & sign');
  assert.ok(i >= 0, 'the Finalize step is missing from the checklist');
  // The LABEL is the link; the marker beside it is the tick. They became separate buttons at
  // v1.128.0 so that ticking a step off and going to do it are different targets — clicking the
  // row itself now does nothing, which is what this line used to do.
  await page.evaluate((n) => document.querySelectorAll('#signSteps .signstep')[n].querySelector('.signstep-label').click(), i);
  await page.waitForFunction(() => document.body.dataset.tab === 'secure');
  // The step leads to a tool for the document, so the page it was on yields — and stays open in its tab.
  assert.equal(await page.evaluate(() => document.getElementById('signingStepsPage').hidden), true,
    'the Finalize row left the checklist in front of the document Finalize acts on');
  assert.equal(await page.$$eval('#tabstrip .pagetab[data-apppage="signingStepsPage"]', (t) => t.length), 1, 'the row closed the page instead of leaving it');
  const cardOpen = await page.evaluate(() => {
    const head = [...document.querySelectorAll('#commands .sbhead.groupcard')]
      .find((x) => x.textContent.trim() === 'Sign & Timestamp');
    return head?.getAttribute('aria-expanded') === 'true';
  });
  assert.equal(cardOpen, true,
    'the Finalize row switched mode but did not open the card holding the command — the link lands in the right room and leaves you looking for the door');
});

// A quick stamp is not a signature.
//
// The probe behind "Place your signature or initials" counted `.ovl-stamp`, which covers the quick
// stamps too — a date, a checkmark, an "approved" — so stamping today's date ticked the step as
// though you had signed. Reported by Dan in as many words: the tick should appear once the item
// has been completed.
//
// A signature or initials comes from the Library and carries `/api/images/<id>` in its src; a quick
// stamp is a `data:` URL built on the spot. That is the whole difference between the step being
// done and not, and it is only visible in a rendered overlay — no source scan reaches it.
test('a date stamp does not tick the signature step', async () => {
  await h.openDocument(DOC, 2);
  const sig = async () => (await steps()).find((r) => r.label === 'Place your signature or initials')?.state;
  assert.equal(await sig(), 'todo', 'setup: the signature step is already ticked before anything was placed');

  await h.mode('markup');
  await h.panel('library');
  await page.click('.quickstamps button[data-stamp="date"]');
  await page.waitForSelector('.viewerContainer:not([hidden]) .ovl-stamp');

  const stamps = await page.evaluate(() => document.querySelectorAll('.viewerContainer:not([hidden]) .ovl-stamp').length);
  assert.ok(stamps > 0, 'setup: no stamp was placed, so the assertion below is about nothing');
  assert.equal(await sig(), 'todo',
    'stamping a DATE ticked "Place your signature or initials". Any stamp is being counted as a signature, so the checklist reports a step done that the user has not done');

  await h.closeAppPages(); // the checklist's page is in front, and the document's × is on the document's tab
  h.answerDialogs(true);
  await h.closeDocument();
});

// A step can be ticked by hand, and the tick says whose claim it is.
//
// The probes answer "has this happened"; a person also needs to say "I have dealt with this" — for
// the eight steps Nib cannot observe at all, and for the ones where her judgement differs from the
// probe's. What must NOT happen is the two becoming indistinguishable: a manual tick is her claim,
// an observed one is evidence, and a checklist that blurs them is back to decoration.
test('any step can be ticked by hand, and it is marked as the user\'s claim', async () => {
  const row = async (label) => (await page.evaluate((l) => {
    const r = [...document.querySelectorAll('#signSteps .signstep')]
      .find((x) => x.querySelector('.signstep-label').textContent === l);
    return r ? { state: r.dataset.state, by: r.dataset.by } : null;
  }, label));
  const clickMark = (label) => page.evaluate((l) => {
    [...document.querySelectorAll('#signSteps .signstep')]
      .find((x) => x.querySelector('.signstep-label').textContent === l)
      .querySelector('.signstep-mark').click();
  }, label);

  await steps();
  const LABEL = 'Scan for hidden content';   // one Nib genuinely cannot observe
  const before = await row(LABEL);
  assert.equal(before.state, 'untracked',
    'setup: the step this test ticks is not the untracked one it was written against');

  await clickMark(LABEL);
  const after = await row(LABEL);
  assert.equal(after.state, 'done', 'clicking the marker did not tick the step — an optional step cannot be checked off at all');
  assert.equal(after.by, 'hand',
    'a hand-made tick is recorded as an observation. The list distinguishes what Nib saw from what you told it, and losing that makes every tick mean the weaker of the two');

  // And it clears again: a tick you cannot undo is a trap on a list you are using to think.
  await clickMark(LABEL);
  assert.equal((await row(LABEL)).state, 'untracked', 'a hand-made tick cannot be cleared');
});

// ── The checklist is a PAGE (ADR-105) ─────────────────────────────────────────
//
// Dan, 2026-10-08: "The Signing menu should be the same, they should open in their own tab", and
// the test for which entries: "Only the pages that requires configuration or runs a wizard should
// be opened in a tab. Any settings that are actions on an open document, like placing flags should
// be expandable in menu." Simple Sign and the ceremony are pages; Place Signing Flags and Send &
// Receive are in the menu as they were. What only a real browser and a real document can say is
// here: the document coming back as it was, a flag armed from the menu with the page in front,
// ticks that follow a real flag, focus, and whether the pages fit a narrow window.
const SIGNING = '#commands .tbtab[data-tab="collaborate"] .sbhead.groupcard';
const look = () => page.evaluate(() => {
  const vis = (e) => !!e && e.getClientRects().length > 0;
  const a = document.activeElement;
  return {
    pages: [...document.querySelectorAll('#viewerCol > .apppage')].filter(vis).map((p) => p.id),
    viewer: vis(document.getElementById('viewerWrap')),
    sheet: vis(document.getElementById('ceremonySheet')),
    row: vis(document.getElementById('tabrow')),
    pageTabs: [...document.querySelectorAll('#tabstrip .pagetab')].map((t) => t.dataset.apppage),
    selected: [...document.querySelectorAll('#tabstrip .tab[aria-selected="true"]')].map((t) => t.dataset.apppage || 'document'),
    flags: vis(document.querySelector('#flags .markers')),
    armed: [...document.querySelectorAll('.markers button.active')].map((b) => b.dataset.marker),
    expanded: [...document.querySelectorAll('#commands .tbtab[data-tab="collaborate"] > .tbgroup')].filter(vis).map((g) => g.dataset.label),
    heads: [...document.querySelectorAll('#commands .tbtab[data-tab="collaborate"] .sbhead.groupcard')].filter(vis)
      .map((e) => [e.textContent.trim(), e.getAttribute('aria-expanded')]),
    locked: document.getElementById('viewerWrap').classList.contains('signing-locked'),
    focus: a ? (a.id || a.tagName) : null,
  };
});
const shot = async (name) => { if (process.env.NIB_UI_SHOTS) await page.screenshot({ path: join(process.env.NIB_UI_SHOTS, name) }); };

test('entering Signing lands on the flag tools and opens no tab; Flags and Send & Receive expand in the menu as before', async () => {
  await h.closeAppPages();
  await h.mode('file');
  await h.mode('collaborate');
  let s = await look();
  assert.deepEqual([s.pages, s.row], [[], false], 'entering Signing put a page or a tab up');
  assert.equal(s.flags, true, 'Signing did not land on the six flag buttons');
  assert.deepEqual(s.heads.slice(0, 2), [['Simple Sign', null], ['Send & Receive', 'false']],
    'the Signing menu is not the Simple Sign entry (which expands nothing) and the Send & Receive card');
  await h.card('Send & Receive');
  s = await look();
  assert.deepEqual([s.expanded, s.pages, s.row], [['Send & Receive'], [], false], 'Send & Receive did not expand in the menu, or opened a page');
  const sendButtons = await page.$$eval('#commands .tbgroup[data-label="Send & Receive"] button', (bs) => bs.filter((b) => b.getClientRects().length).map((b) => b.textContent.trim()));
  assert.ok(sendButtons.includes('Send a document to a peer…') && sendButtons.includes('Check a signed document that came back…'),
    `the open Send & Receive card does not show its buttons: ${sendButtons.join(' | ')}`);
  await h.panel('flags');
  assert.equal((await look()).flags, true, 'the flag panel did not open on its header');
});

test('Simple Sign opens its page with one tab — by mouse, and from the keyboard — and × closes it', async () => {
  await h.mode('collaborate');
  assert.equal(await h.appPage('collaborate', 'Simple Sign'), 'signingStepsPage');
  let s = await look();
  assert.deepEqual([s.pages, s.viewer, s.pageTabs, s.selected, s.expanded], [['signingStepsPage'], false, ['signingStepsPage'], ['signingStepsPage'], []],
    'the entry did not put the page in the main area with one selected tab and nothing expanded');
  assert.equal(s.focus, 'signingStepsPageTitle', 'opening the page did not put focus on its heading');
  assert.equal(await page.$eval('#tabstrip .pagetab', (t) => t.getAttribute('aria-label')), 'Simple Sign, Signing page');
  await page.click(`${SIGNING}:text-is("Simple Sign")`);
  assert.deepEqual((await look()).pageTabs, ['signingStepsPage'], 'a second click opened a second tab');
  await h.closeAppPage('signingStepsPage');
  s = await look();
  assert.deepEqual([s.pages, s.row, s.viewer], [[], false, true], 'closing the page did not give the main area back');
  // The keyboard: the entry is a button in the menu; Enter on it opens the page and focus goes to the heading.
  await page.focus(`${SIGNING}:text-is("Simple Sign")`);
  await page.keyboard.press('Enter');
  await page.waitForSelector('#signingStepsPage:not([hidden])');
  s = await look();
  assert.deepEqual([s.pages, s.pageTabs, s.focus], [['signingStepsPage'], ['signingStepsPage'], 'signingStepsPageTitle'], 'Enter on the entry did not open the page with focus on its heading');
  // The hint is words on the page, not a tooltip: the one-way door is readable without a pointer.
  const hint = await page.$eval('#signSteps', (host) => {
    const r = [...host.querySelectorAll('.signstep')].find((x) => x.querySelector('.signstep-label').textContent === 'Redact, then apply');
    const e = r.querySelector('.signstep-hint');
    return { text: e.textContent, shown: e.getClientRects().length > 0 };
  });
  assert.deepEqual(hint, { text: 'Applying is irreversible — do it before you sign', shown: true });
  await h.closeAppPages();
});

test('beside a real document: the ticks are live, a flag armed from the menu shows the document, and the document comes back as it was', async () => {
  await h.openDocument(DOC, 2);
  // Somewhere other than the top, so "as it was" is not true of a document that was reset.
  await page.evaluate(() => { document.querySelector('.viewerContainer:not([hidden])').scrollTop = 400; });
  await page.waitForFunction(() => document.querySelector('.viewerContainer:not([hidden])').scrollTop > 300);
  const before = await page.evaluate(() => document.querySelector('.viewerContainer:not([hidden])').scrollTop);
  let rows = await steps();
  assert.equal(stateOf(rows, 'Open the document'), 'done', 'tick 1: a document is open and the page says it is not');
  assert.equal(stateOf(rows, 'Plant flags for someone else'), 'todo', 'setup: the flags step is ticked before any flag is placed');
  let s = await look();
  assert.deepEqual([s.pages, s.viewer, s.selected], [['signingStepsPage'], false, ['signingStepsPage']]);
  // The flag tools are in the menu, beside the page, and are not switched off by it.
  assert.equal(s.flags, true, 'the flag tools are not in the menu beside the page');
  await page.click('.markers button[data-marker="sign"]');
  s = await look(); // no wait: the page yields in the press itself, or it does not yield
  assert.deepEqual([s.pages, s.viewer, s.armed, s.selected], [[], true, ['sign'], ['document']],
    'arming a flag with the page in front did not show the document with that flag armed');
  assert.deepEqual(s.pageTabs, ['signingStepsPage'], 'arming a flag closed the page instead of leaving it in its tab');
  assert.equal(await page.evaluate(() => document.querySelector('.viewerContainer:not([hidden])').scrollTop), before,
    'the document came back somewhere other than where it was left');
  await shot('signing-flag-armed-from-page.png');
  const box = await page.locator('.viewerContainer:not([hidden]) .page').first().boundingBox();
  await page.mouse.click(box.x + box.width / 2, Math.max(box.y + 40, 260));
  await page.waitForSelector('.viewerContainer:not([hidden]) .ovl-marker');
  // Back to the page by its tab: the tick followed the flag.
  await page.click('#tabstrip .pagetab[data-apppage="signingStepsPage"]');
  await page.waitForSelector('#signingStepsPage:not([hidden])');
  rows = await page.evaluate(() => [...document.querySelectorAll('#signSteps .signstep')].map((r) => ({
    label: r.querySelector('.signstep-label').textContent, state: r.dataset.state })));
  assert.equal(stateOf(rows, 'Plant flags for someone else'), 'done', 'tick 2: a flag is on the page and the checklist still shows that step as outstanding');
  assert.equal(stateOf(rows, 'Finalize & sign'), 'todo', 'placing a flag ticked the signing step');
  // The lock is the document's: pressed from the menu with the page in front, the document is what shows it.
  await page.click('#signCompleteBtn');
  await page.waitForFunction(() => document.getElementById('viewerWrap').classList.contains('signing-locked'));
  s = await look();
  assert.deepEqual([s.pages, s.viewer, s.locked], [[], true, true], 'locking the marks left the page in front of the document that was locked');
  // A step that leads to Signing's own panel leaves the page, though the mode does not change.
  await page.click('#tabstrip .pagetab[data-apppage="signingStepsPage"]');
  await page.waitForSelector('#signingStepsPage:not([hidden])');
  await page.click('#signSteps .signstep-label:text-is("Plant flags for someone else")');
  s = await look();
  assert.deepEqual([s.pages, s.viewer, s.flags], [[], true, true], 'the Flags step left the checklist in front of the document the flags go on');
  // And a document's tab is the way back from the page, with the page still in its tab.
  await page.click('#tabstrip .pagetab[data-apppage="signingStepsPage"]');
  await page.click('#tabstrip .tab:not(.pagetab)');
  s = await look();
  assert.deepEqual([s.pages, s.viewer, s.pageTabs, s.selected], [[], true, ['signingStepsPage'], ['document']]);
  await page.click('#signCompleteBtn'); // Edit marks again
  await page.waitForFunction(() => !document.getElementById('viewerWrap').classList.contains('signing-locked'));
  await h.closeAppPages();
  h.answerDialogs(true);
  await h.closeDocument();
});

// The ceremony feature is off on a new vault and this tier's server is shared by every file, so the switch is
// turned on through its own box, on its own page, and put back.
test('the ceremony page: its entry comes and goes with the feature, it starts the two flows, and it says when the feature is off', async () => {
  const setCeremonies = async (on) => {
    await h.settingsPage('Advanced features');
    if (await page.isChecked('#advCeremonyChk') !== on) await page.click('#advCeremonyChk');
    await page.waitForFunction((want) => document.getElementById('ceremony').hidden === !want, on);
  };
  const entryShown = () => page.$$eval(SIGNING, (es) => es.filter((e) => e.getClientRects().length).map((e) => e.textContent.trim()).includes('About Ceremonies'));
  const was = await (async () => { await h.settingsPage('Advanced features'); return page.isChecked('#advCeremonyChk'); })();
  try {
    await setCeremonies(false);
    await h.mode('collaborate');
    assert.equal(await entryShown(), false, 'the menu offers the ceremony page with ceremonies switched off');
    await setCeremonies(true);
    await h.mode('collaborate');
    assert.equal(await entryShown(), true, 'the ceremony entry did not appear when ceremonies were switched on');
    assert.equal(await h.appPage('collaborate', 'About Ceremonies'), 'signingCeremonyPage');
    let s = await look();
    assert.deepEqual([s.pages, s.focus, s.expanded], [['signingCeremonyPage'], 'signingCeremonyPageTitle', []]);
    assert.equal(await page.$eval('#tabstrip .pagetab[data-apppage="signingCeremonyPage"]', (t) => t.getAttribute('aria-label')), 'Signing Ceremonies, Signing page');
    // Accept: the invitation box is in the menu's ceremony panel; the page stays beside it.
    await page.click('#cerPageAcceptBtn');
    await page.waitForSelector('#ceremonyAcceptForm:not([hidden])');
    s = await look();
    assert.deepEqual([s.pages, s.focus], [['signingCeremonyPage'], 'cerInviteText'], 'Accept did not open the invitation box with focus in it, beside the page');
    await page.click('#cerAcceptCancel');
    // Convene: the setup sheet takes the main area and the page keeps its tab.
    await page.click('#cerPageConveneBtn');
    await page.waitForFunction(() => document.getElementById('ceremonySheet').getClientRects().length > 0);
    s = await look();
    assert.deepEqual([s.pages, s.sheet, s.pageTabs.includes('signingCeremonyPage')], [[], true, true], 'Convene did not raise the setup sheet in the page\'s place, or closed the page');
    await page.click('#cerSheetClose');
    // Switched off with the page open in its tab: the page says so and offers the switch, not the flows.
    await setCeremonies(false);
    await page.click('#tabstrip .pagetab[data-apppage="signingCeremonyPage"]');
    const off = await page.evaluate(() => {
      const vis = (e) => !!e && e.getClientRects().length > 0;
      return { convene: vis(document.getElementById('cerPageConveneBtn')), accept: vis(document.getElementById('cerPageAcceptBtn')),
        said: vis(document.querySelector('#signingCeremonyPage [data-advoff="ceremony"]')) };
    });
    assert.deepEqual(off, { convene: false, accept: false, said: true }, 'with ceremonies off the page offers the flows, or does not say they are off');
    await page.click('#signingCeremonyPage [data-advoff="ceremony"] button');
    await page.waitForSelector('#settingsAdvancedPage:not([hidden])');
  } finally {
    await setCeremonies(was);
    await h.closeAppPages();
  }
});

test('at 414 and 375 pixels wide neither Signing page runs past the window', async () => {
  await h.settingsPage('Advanced features');
  const was = await page.isChecked('#advCeremonyChk');
  if (!was) await page.click('#advCeremonyChk');
  await page.waitForFunction(() => !document.getElementById('ceremony').hidden);
  await h.closeAppPages();
  await h.mode('collaborate');
  const MORE = '#toolbar .tbtab[data-tab="collaborate"] .tbmore';
  try {
    for (const width of [414, 375]) {
      await page.setViewportSize({ width, height: 800 });
      await page.waitForFunction(() => document.getElementById('sidebar').classList.contains('collapsed'));
      for (const [label, id] of [['Simple Sign', 'signingStepsPage'], ['About Ceremonies', 'signingCeremonyPage']]) {
        // With the sidebar shut the entry is in ⋯ More, and that is the way in.
        await page.click(`${MORE} .menutop`);
        await page.click(`${MORE} .dropdown button:text-is("${label}…")`);
        await page.waitForSelector(`#${id}:not([hidden])`);
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
        assert.ok(m.sideways <= 0 && m.windowSideways <= 0, `at ${width}px ${label}'s page scrolls sideways (${m.sideways}px, window ${m.windowSideways}px)`);
        assert.deepEqual(m.past, [], `at ${width}px these run past the edge of ${label}'s page: ${m.past.join(', ')}`);
      }
      await h.closeAppPages();
    }
  } finally {
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.waitForFunction(() => !document.getElementById('sidebar').classList.contains('collapsed'));
    await h.closeAppPages();
    await h.settingsPage('Advanced features');
    if (await page.isChecked('#advCeremonyChk') !== was) await page.click('#advCeremonyChk');
    await page.waitForFunction((want) => document.getElementById('ceremony').hidden === !want, was);
    await h.closeAppPages();
  }
});

// Pictures for a person to look at, never an assertion: NIB_UI_SHOTS names a folder. Unset — as it is in the
// tier — this does nothing.
test('pictures of the Signing pages in both themes, when asked for', { skip: !process.env.NIB_UI_SHOTS }, async () => {
  const start = await page.evaluate(() => document.documentElement.dataset.appearance);
  await h.settingsPage('Advanced features');
  const was = await page.isChecked('#advCeremonyChk');
  if (!was) await page.click('#advCeremonyChk');
  await page.waitForFunction(() => !document.getElementById('ceremony').hidden);
  await h.closeAppPages();
  try {
    for (const theme of ['dark', 'light']) {
      await page.evaluate((t) => { document.documentElement.dataset.appearance = t; }, theme);
      for (const [label, file] of [['Simple Sign', 'simple-sign'], ['About Ceremonies', 'ceremonies']]) {
        await h.appPage('collaborate', label);
        await page.screenshot({ path: join(process.env.NIB_UI_SHOTS, `signing-${file}-${theme}.png`), fullPage: false });
      }
      await h.closeAppPages();
    }
  } finally {
    await page.evaluate((t) => { document.documentElement.dataset.appearance = t; }, start);
    await h.settingsPage('Advanced features');
    if (await page.isChecked('#advCeremonyChk') !== was) await page.click('#advCeremonyChk');
    await page.waitForFunction((want) => document.getElementById('ceremony').hidden === !want, was);
    await h.closeAppPages();
  }
});
