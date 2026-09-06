// Shared setup for the tier-3 browser tests.
//
// build/uirepro.sh owns the expensive half — building nib, launching it under a
// throwaway HOME, enrolling a vault key — and hands the result over in the
// environment. This module only opens a browser page against it, so a test file
// stays about behaviour rather than plumbing.
import { chromium } from 'playwright-core';

export const BASE = process.env.NIB_UI_BASE;
// The second, never-enrolled nib (P06.S07). The vault has no lock route, so the locked state is
// unreachable on the shared server this tier enrols at startup; uirepro.sh starts a second process
// for it and asserts it is not `ready` before any file runs.
export const LOCKED_BASE = process.env.NIB_UI_LOCKED_BASE;
export const WORK = process.env.NIB_UI_WORK;
const EXECUTABLE = process.env.NIB_UI_BROWSER;

if (!BASE || !EXECUTABLE) {
  // Loud rather than skipped. These tests are only ever run by uirepro.sh, which
  // sets both; reaching here means someone ran `node --test test/ui/` directly and
  // would otherwise get a confusing failure deep inside a page call.
  throw new Error('test/ui is driven by build/uirepro.sh — run that instead (NIB_UI_BASE / NIB_UI_BROWSER unset)');
}

// launch opens a page on the running nib. Headless by default; NIB_UI_HEADED=1
// shows the window, which is how you watch a failing test happen.
//
// # Options
//
// `routes` installs Playwright route handlers BEFORE the first navigation, and `waitFor`
// replaces the readiness marker. Both exist for one flow: the auth overlay.
//
// uirepro.sh enrols a vault key with curl before any browser opens — it has to, since
// every document route is behind `requireUnlocked` — so the app this tier drives is
// always in `state: 'ready'` and the overlay's other states are unreachable by
// navigating. They are also unreachable by re-opening: the vault unlocks once per process
// and there is no Lock control in the UI. Interception is how they are reached, and it is
// faithful because the overlay is a pure function of `/api/status` — app.js's
// `applyStatus` branches on `st.state` and nothing else.
//
// A test that wants the overlay must also pass `waitFor`, because `#empty` is the
// READY-state marker and waiting for it against a locked app would time out at 30 s
// inside the harness rather than failing in the test.
// `base` names which nib to drive. It defaults to the shared unlocked one; the locked-view file
// passes LOCKED_BASE. `waitFor` moves with it — a locked app never reaches `#empty`, because
// applyStatus returns at the overlay.
export async function launch({ routes = null, waitFor = '#empty', base = BASE } = {}) {
  const browser = await chromium.launch({
    executablePath: EXECUTABLE,
    headless: process.env.NIB_UI_HEADED !== '1',
  });
  const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
  const consoleErrors = [];
  page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text()); });
  page.on('pageerror', (e) => consoleErrors.push('pageerror: ' + e.message));

  // Dialogs are handled explicitly and COUNTED, never left to Playwright's
  // default — which auto-dismisses, i.e. silently answers "cancel". Left alone,
  // a test asserting "closing prompts" and one asserting "cancel leaves the
  // document alone" would both pass for the wrong reason, and "a clean document
  // does not prompt" would be indistinguishable from either. The count is the
  // observable; the answer is set per test.
  const dialogs = [];
  const state = { accept: true };
  page.on('dialog', async (d) => {
    dialogs.push(d.message());
    if (state.accept) await d.accept(); else await d.dismiss();
  });

  if (routes) {
    for (const [pattern, handler] of Object.entries(routes)) {
      await page.route(pattern, handler);
    }
  }

  await page.goto(base);
  // The app boots asynchronously (status -> applyStatus -> the UI), so wait for a
  // marker of readiness rather than a fixed sleep.
  await page.waitForSelector(waitFor, { state: 'attached' });
  return {
    browser,
    page,
    consoleErrors,
    dialogs,
    answerDialogs(accept) { state.accept = accept; },

    // mode switches the menubar tab. It matters more than it looks: the toolbar
    // swaps per mode, so File-tab controls (Open…, Save, Close) are genuinely not
    // visible while the app is in collaborate or sign mode. A test that placed a
    // signing flag and then reached for Close would be clicking a hidden button —
    // Playwright refuses, correctly, and that refusal is the UI's own rule
    // showing through rather than a harness quirk.
    // panel(name) shows one of the sidebar's panels. Needed since v1.121.0, because a mode's
    // own commands are a sidebar panel and switching mode lands on it — so a test that wants
    // the thumbnails, or a control the mode does not land on, has to say so.
    // Since v1.123.1 a panel header TOGGLES, so clicking one that is already open closes it —
    // and a mode lands on its own first panel, which is exactly the case a test hits. This
    // helper means "show me that panel", so it clicks only when the panel is not already open.
    async panel(name) {
      const open = await page.$eval(`.sbhead[data-panel="${name}"]`, (el) => el.classList.contains('active'));
      if (!open) await page.click(`.sbhead[data-panel="${name}"]`);
      await page.waitForFunction((n) => {
        const el = document.getElementById(n);
        return el && el.classList.contains('active');
      }, name);
    },

    // group(label) opens one command card in the sidebar. Since v1.122.0 the sidebar is an
    // accordion — one card expanded at a time — so a control in a group other than the mode's
    // first is behind a header until someone clicks it, exactly as it is for a user.
    async group(label) {
      await page.click(`.sbhead.groupcard:text-is("${label}")`);
      await page.waitForFunction((l) => {
        const h = [...document.querySelectorAll('.sbhead.groupcard')].find((x) => x.textContent.trim() === l);
        return h && h.getAttribute('aria-expanded') === 'true';
      }, label);
    },

    async mode(tab) {
      await page.click(`[data-tab="${tab}"]`);
      await page.waitForFunction((t) => document.body.dataset.tab === t, tab);
    },

    // openDocument drives the REAL dialog — File mode first, then the Open…
    // button, because #pathInput does not exist to fill until the modal is up.
    // Playwright's actionability checks make both steps impossible to skip.
    // `pages` is required, and it is not decoration: waiting on `has-doc` alone is
    // ALREADY TRUE when a document is open, so opening a second document would
    // return instantly and every assertion after it would read the OLD document
    // while looking like it read the new one. Waiting for the new page count is a
    // transition check rather than a state check — the same distinction this whole
    // phase keeps turning on, and this helper got it wrong first time.
    async openDocument(file, pages) {
      await this.mode('file');
      await page.click('#openMenuItem');
      await page.fill('#pathInput', file);
      await page.click('#openGo');
      await page.waitForFunction(() => document.getElementById('viewerWrap').className === 'has-doc');
      await page.waitForFunction(
        (n) => document.querySelector('.pageCount').textContent === `/ ${n}`,
        pages,
      );
      // Scoped to the VISIBLE view (P05.S03). These three selectors read `#viewer .page`
      // until that slice gave each open document its own container, and an id cannot be
      // shared — but a bare `.viewerPages .page` would have been worse than the rename
      // looks: with several containers in the wrap it silently measures whichever one
      // the document ordered first, which need not be the one on screen. That failure
      // surfaces as Playwright's 30s TimeoutError, not an assertion failure, because this
      // line is inside openDocument() — which every tier-3 test calls.
      await page.waitForFunction(() => document.querySelectorAll('.viewerContainer:not([hidden]) .page').length > 0);
    },

    // deleteMarker(type) removes the flag placed by placeMarker(type).
    //
    // **It deletes with the tool STILL ARMED, deliberately.** This used to disarm first,
    // because with the flag tool armed a click on a flag's × planted ANOTHER flag
    // instead of removing it (measured here: 1 marker in, 2 out) — placement listens on
    // `pointerdown` on #viewerWrap while the × only stops propagation on `click`, so the
    // placement fired first and was never stopped. `onExistingOverlay` fixed that in the
    // app, and REMOVING the disarm is the regression check: put the defect back and this
    // helper's own `=== 0` wait fails, because the deleted flag is replaced by a new one.
    // A workaround kept after its defect is fixed is a test that can no longer see it.
    //
    // The × itself is display:none until :hover OR :focus on the marker
    // (style.css), and buildMarker gives the marker tabIndex 0 so it can be
    // focused. Focus rather than hover: a click performs its own pointer
    // movement, which can re-hide the button between the actionability check and
    // the click. Focus does not move.
    async deleteMarker() {
      await page.locator('.ovl-marker').first().focus();
      await page.locator('.ovl-marker .marker-del').first().click();
      await page.waitForFunction(() => document.querySelectorAll('.ovl-marker').length === 0);
    },

    // placeMarker arms a signing flag and drops it on page 1. The flags live in
    // the sidebar's Flags panel, which belongs to COLLABORATE mode, not Sign —
    // SIDEBAR_FOR.markup is ['library'] (app.js). Discovered by driving it; nothing
    // in the UI's naming suggests it.
    // **Scrolls the target page into view first**, and that is a fix rather than a
    // flourish. This clicked the box of `.page` FIRST unconditionally, so on any
    // document scrolled past page 1 the click landed outside the viewport and the marker
    // never appeared — surfacing as a 30-second waitForSelector timeout INSIDE this
    // helper rather than as an assertion failure in the test that called it. Every
    // caller until P06.S02 happened to open a fresh document and never met the
    // precondition, which is why nothing stated it. Found by a caller that violated it.
    async placeMarker(type = 'date') {
      await this.topOfDocument();
      await page.click('[data-tab="collaborate"]');
      await page.click(`[data-marker="${type}"]`);
      const box = await page.locator('.viewerContainer:not([hidden]) .page').first().boundingBox();
      await page.mouse.click(box.x + box.width / 2, box.y + box.height / 3);
      await page.waitForSelector('.ovl-marker');
    },

    // topOfDocument scrolls the visible view back to its first page, which every
    // placement helper needs: they address `.page` FIRST, and a document left scrolled
    // puts that element above the viewport.
    async topOfDocument() {
      await page.evaluate(() => {
        const c = document.querySelector('.viewerContainer:not([hidden])');
        if (c) c.scrollTop = 0;
      });
      await page.waitForFunction(() => {
        const p = document.querySelector('.viewerContainer:not([hidden]) .page');
        if (!p) return false;
        const r = p.getBoundingClientRect();
        return r.top > -1 && r.height > 0;
      });
    },

    // placeEditField drags a cover-and-replace box over the top-left text of page one
    // and returns the `<input>` it produces — a real overlay carrying a real typed
    // value, which is the affordance no tier had.
    //
    // It is what P06's exit criterion asks for in as many words: "typed overlay values —
    // asserted by reading a typed value back out of its DOM element". Until this existed
    // that clause could not be driven at any tier: jsdom cannot place an overlay at all
    // (every getBoundingClientRect is 0, so pageAt never resolves a page), and the only
    // tier-3 placement helper put down a signing flag carrying no text.
    //
    // The drag region is where the generated fixtures put their text — `BT /F1 36 Tf
    // 72 700 Td` on a 612x792 page, so about a tenth in from the left and a tenth down.
    // A cover box needs to overlap real text: makeEditField samples the text under it
    // and an empty region yields an empty field, which would still be an input and would
    // still hold a typed value, but would not be the flow a user takes.
    async placeEditField() {
      await this.topOfDocument();
      await this.mode('edit');
      await page.click('#editTextBtn');
      const box = await page.locator('.viewerContainer:not([hidden]) .page').first().boundingBox();
      await page.mouse.move(box.x + box.width * 0.10, box.y + box.height * 0.08);
      await page.mouse.down();
      await page.mouse.move(box.x + box.width * 0.60, box.y + box.height * 0.16, { steps: 8 });
      await page.mouse.up();
      await page.waitForSelector('.viewerContainer:not([hidden]) .ovl-edit');
      // Disarm, or the next click anywhere on the page starts another edit box.
      await page.click('#editTextBtn');
      await page.waitForFunction(() => !document.getElementById('editTextBtn').classList.contains('active'));
    },

    // formValues reads every AcroForm input pdf.js rendered in the VISIBLE view. A form
    // fill lives in pdf.js's annotationStorage, not in a Nib overlay, so it is a
    // separate observable from typedValues and the acceptance clause names it
    // separately.
    formValues() {
      return page.$$eval('.viewerContainer:not([hidden]) .annotationLayer input[type="text"]',
        (els) => els.map((e) => e.value));
    },

    // typedValues reads every edit-overlay value in the VISIBLE view. The point of the
    // exercise is reading a value back out of its DOM element, so this reads `.value`
    // and not, say, a count of overlays.
    typedValues() {
      return page.$$eval('.viewerContainer:not([hidden]) .ovl-edit', (els) => els.map((e) => e.value));
    },

    // closeDocument clicks Close — from File mode, for the reason above.
    async closeDocument() {
      await this.mode('file');
      await page.click('#closeBtn');
      await page.waitForTimeout(400); // the confirm + the round-trip + the teardown
    },

    counts() {
      return page.evaluate(() => ({
        overlays: document.querySelectorAll('.ovl').length,
        markers: document.querySelectorAll('.ovl-marker').length,
        // The VISIBLE view's grid, matching the `pages` idiom below: each open document
        // has its own `.thumbgrid` since P05.S05, and a bare class selector would sum
        // across them.
        thumbs: document.querySelector('.thumbgrid:not([hidden])')?.children.length ?? 0,
        // The visible view's pages, not every visible container's summed — with two
        // views open a sum would silently report 3 + 5 = 8 and read as a page count.
        pages: document.querySelector('.viewerContainer:not([hidden])')?.querySelectorAll('.page').length ?? 0,
      }));
    },
  };
}
