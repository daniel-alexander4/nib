// ADR-169 — the power button ends the process and the window, in a real browser against a real nib.
//
// **Its own nib and its own Chrome, and both are the point.** Quit ends the process, so it cannot be pressed on
// the one nib every other file here shares. And the harness's page is one Chrome will NOT let close itself: it is
// opened blank and then navigated, so it has history before Nib, and Chrome closes a page by script only when the
// entry it is on is the only one it has. A window Nib launches is opened ON its address — one entry — so that is
// how the first window here is opened: the address on Chrome's command line, as `internal/browser` does it.
//
// The second window is the harness's kind on purpose. It is the one a browser refuses to close, and it never
// pressed anything: it must be told, and left saying Nib has quit rather than that Nib could not be reached.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { spawn, execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium } from 'playwright-core';
import { WORK, BASE } from './harness.mjs';

const CHROME = process.env.NIB_UI_BROWSER;
const REPO = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', '..');
const skip = process.env.NIB_UI_ENGINE === 'firefox' ? 'this file drives Chrome over its debugging port' : false;
const port = Number(new URL(BASE).port) + 9;
const base = `http://127.0.0.1:${port}`;
const home = fs.mkdtempSync(path.join(WORK, 'quit-'));
const log = path.join(home, 'nib.log');
// Chrome refuses a profile whose socket path is too long, so the profile is under /tmp whatever TMPDIR says.
const profile = fs.mkdtempSync('/tmp/nibquit-');

let nib = null;
let chrome = null;
let cdp = null;
after(async () => {
  if (cdp) await cdp.close().catch(() => {});
  if (chrome && chrome.exitCode === null) {
    // Chrome goes on writing its profile until it has gone, so the profile is removed after it has.
    const gone = new Promise((r) => chrome.once('exit', r));
    chrome.kill();
    await Promise.race([gone, new Promise((r) => setTimeout(r, 5000))]);
  }
  if (nib && nib.exitCode === null) nib.kill();
  try { fs.rmSync(profile, { recursive: true, force: true, maxRetries: 10, retryDelay: 200 }); } catch { /* a throwaway profile under /tmp */ }
});

const until = async (what, fn, ms = 10000) => {
  const deadline = Date.now() + ms;
  for (;;) {
    const v = await fn();
    if (v) return v;
    if (Date.now() > deadline) assert.fail(`timed out waiting for ${what}`);
    await new Promise((r) => setTimeout(r, 100));
  }
};

test('Quit ends the process, closes the window it was pressed in, and tells the other one', { skip }, async () => {
  fs.mkdirSync(path.join(home, 'config'), { recursive: true });
  nib = spawn(path.join(WORK, 'nib'), [], {
    env: { ...process.env, HOME: home, XDG_CONFIG_HOME: path.join(home, 'config'), NIB_NO_BROWSER: '1', NIB_NO_UPDATE_CHECK: '1', NIB_ADDR: `127.0.0.1:${port}` },
    stdio: ['ignore', fs.openSync(log, 'a'), fs.openSync(log, 'a')],
  });
  let exited = null;
  nib.on('exit', (code) => { exited = code; });
  // Headless, nib logs the address it would have opened a window at, launch key and all — read through
  // build/launchkey.sh, the one reader of that line (ADR-009), which waits for it to be written.
  const url = execFileSync('bash', ['-c', '. build/launchkey.sh && printf "%s/#k=%s" "$(launch_base "$1")" "$(launch_key "$1")"', '_', log], { cwd: REPO, encoding: 'utf8' }).trim();
  assert.match(url, /^http:\/\/127\.0\.0\.1:\d+\/#k=.+/, 'nib logged no window address');

  const debug = port + 1;
  chrome = spawn(CHROME, ['--headless=new', `--remote-debugging-port=${debug}`, `--user-data-dir=${profile}`, '--no-first-run', `--app=${url}`], { stdio: 'ignore' });
  cdp = await until('Chrome to answer on its debugging port', () => chromium.connectOverCDP(`http://127.0.0.1:${debug}`).catch(() => null), 20000);
  const ctx = cdp.contexts()[0];
  const first = await until('the launched window', () => ctx.pages().find((p) => p.url().startsWith(base)));
  // A fresh nib has no vault, so this window is on its setup screen — a real window all the same (D3), and its
  // stream is what the server counts.
  await until('the first window to be counted', () => /window connected/.test(fs.readFileSync(log, 'utf8')));

  // A second window, opened the harness's way: blank, then navigated. Its launch key is minted with the first
  // window's own session.
  const key = await first.evaluate(async () => {
    const res = await fetch('/api/launch/key', { method: 'POST', headers: { 'X-CSRF-Token': sessionStorage.getItem('nib-token') || '' } });
    return (await res.json()).key;
  });
  const second = await ctx.newPage();
  await second.goto(`${base}#k=${key}`);
  await until('the second window to be counted', () => (fs.readFileSync(log, 'utf8').match(/window connected/g) || []).length >= 2);

  // Stimulus: the two windows are the two kinds. Without this the file could pass on two windows of one kind.
  assert.equal(await first.evaluate(() => history.length), 1, 'setup: the launched window has history before Nib, so it is not the kind Nib opens');
  assert.ok(await second.evaluate(() => history.length) > 1, 'setup: the second window has no earlier history, so the browser would let it close and the fallback is not what this shows');
  assert.equal(await second.$eval('#launchOverlay', (el) => el.hidden), true, 'setup: the second window already shows the screen');

  let firstClosed = false;
  first.on('close', () => { firstClosed = true; });
  // The button's own handler: a window on the setup screen has it under the overlay, and a click through the page
  // is what any window's press comes to.
  await first.evaluate(() => document.getElementById('quitBtn').click()).catch(() => { /* the page may go mid-call */ });

  await until('the process to exit', () => exited !== null);
  assert.equal(exited, 0, 'nib did not exit cleanly');
  assert.match(fs.readFileSync(log, 'utf8'), /exiting: you chose Quit/, 'the process went, and not because Quit was pressed');
  await until('the window Quit was pressed in to close', () => firstClosed, 5000);

  // The other window was told. It cannot close, and it says what happened.
  assert.equal(second.isClosed(), false, 'setup: the second window closed after all — Chrome let it, and the fallback was not seen');
  await second.waitForFunction(() => !document.getElementById('launchOverlay').hidden, null, { timeout: 5000 });
  const said = () => second.$eval('#launchText', (el) => el.textContent);
  assert.match(await said(), /You quit Nib/, 'the window that did not press Quit was not told Nib had quit');
  // And it goes on saying so with the process gone. (That a dropped stream cannot replace the words is
  // `test/jsdom/quitbutton.test.mjs`'s: here the page has closed its stream itself, so nothing drops.)
  await second.waitForTimeout(3000);
  assert.match(await said(), /You quit Nib/, 'a moment later the window says something else');
  assert.equal(await second.$eval('#launchTitle', (el) => el.textContent), 'Nib has quit');
});
