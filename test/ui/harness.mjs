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
  await page.goto(BASE);
  // The app boots asynchronously (status -> applyStatus -> the UI), so wait for a
  // marker of readiness rather than a fixed sleep.
  await page.waitForSelector('#empty', { state: 'attached' });
  return { browser, page, consoleErrors };
}
