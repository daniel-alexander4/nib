// Shared setup for the tier-3 browser tests.
//
// build/uirepro.sh owns the expensive half — building nib, launching it under a
// throwaway HOME, enrolling a vault key — and hands the result over in the
// environment. This module only opens a browser page against it, so a test file
// stays about behaviour rather than plumbing.
import { chromium } from 'playwright-core';

export const BASE = process.env.NIB_UI_BASE;
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
export async function launch() {
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

  await page.goto(BASE);
  // The app boots asynchronously (status -> applyStatus -> the UI), so wait for a
  // marker of readiness rather than a fixed sleep.
  await page.waitForSelector('#empty', { state: 'attached' });
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
      await page.waitForFunction(() => document.querySelectorAll('#viewer .page').length > 0);
    },

    // deleteMarker(type) removes the flag placed by placeMarker(type).
    //
    // It DISARMS the tool first, and that is not tidiness — with the flag tool
    // still armed, clicking a flag's × plants ANOTHER flag instead of removing
    // it (measured: 1 marker in, 2 out). Placement listens on `pointerdown` on
    // the viewer container while the × only calls stopPropagation() on `click`,
    // so the placement fires first and is never stopped. That is a real defect in
    // the app, filed separately; the harness works around it because a test
    // should not be the thing that fixes it.
    //
    // The × itself is display:none until :hover OR :focus on the marker
    // (style.css), and buildMarker gives the marker tabIndex 0 so it can be
    // focused. Focus rather than hover: a click performs its own pointer
    // movement, which can re-hide the button between the actionability check and
    // the click. Focus does not move.
    async deleteMarker(type = 'date') {
      await page.click(`[data-marker="${type}"]`); // toggle the tool off
      await page.locator('.ovl-marker').first().focus();
      await page.locator('.ovl-marker .marker-del').first().click();
      await page.waitForFunction(() => document.querySelectorAll('.ovl-marker').length === 0);
    },

    // placeMarker arms a signing flag and drops it on page 1. The flags live in
    // the sidebar's Flags panel, which belongs to COLLABORATE mode, not Sign —
    // SIDEBAR_FOR.sign is ['library'] (app.js). Discovered by driving it; nothing
    // in the UI's naming suggests it.
    async placeMarker(type = 'date') {
      await page.click('[data-tab="collaborate"]');
      await page.click(`[data-marker="${type}"]`);
      const box = await page.locator('#viewer .page').first().boundingBox();
      await page.mouse.click(box.x + box.width / 2, box.y + box.height / 3);
      await page.waitForSelector('.ovl-marker');
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
        thumbs: document.getElementById('thumbGrid').children.length,
        pages: document.querySelectorAll('#viewer .page').length,
      }));
    },
  };
}
