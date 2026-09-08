// windowfreeze — does a frozen or backgrounded window keep its /api/window stream open?
//
// **NOT part of the routine loop, and deliberately so** — same footing as `build/dhtlive.sh` and
// `build/winrepro.sh`. It spends seven minutes waiting out a browser's own backgrounding timer,
// which is a cost no `go test` should pay; and `PLAN-window-lifetime.md`'s own standing caveat
// says this answer has no standing guard, because no tier can hold a window minimised for five
// minutes. Run it when the browser changes, or when D1 is doubted.
//
//     NIB_UI_BROWSER=$(command -v google-chrome) node build/windowfreeze.mjs
//
// It must be run from the repo root: it resolves `playwright-core` from `node_modules`, and it
// builds `./cmd/nib` into a throwaway HOME so it never touches the developer's vault.
//
// /pending 375, P01.S02 — the plan's load-bearing assumption, MEASURED.
//
// D1: "A window's life is a connection, not a timer." The design rests on a minimised or frozen
// page keeping its stream socket open while a closed one drops it. If a frozen page drops it, D1
// is wrong and the rest of P01 is re-planned — which is why the slice is a gate and is second.
//
// Two observations, because they answer different questions:
//   1. EXPLICIT freeze via CDP Page.setWebLifecycleState — the most aggressive state short of
//      discarding the page, applied deterministically rather than waited for.
//   2. A BACKGROUNDED page held past the ~5-minute automatic threshold, which is the state a
//      minimised window actually reaches.
import { chromium } from 'playwright-core';
import { spawn, execSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

const BROWSER = process.env.NIB_UI_BROWSER;
const PORT = process.env.PROBE_PORT || '18771';
const BASE = `http://127.0.0.1:${PORT}`;
const work = fs.mkdtempSync(path.join(os.tmpdir(), 'freezeprobe-'));
const log = path.join(work, 'nib.log');

execSync(`go build -o ${work}/nib ./cmd/nib`, { stdio: 'inherit' });
const srv = spawn(`${work}/nib`, [], {
  env: { ...process.env, HOME: `${work}/home`, XDG_CONFIG_HOME: `${work}/config`,
         NIB_NO_BROWSER: '1', NIB_NO_UPDATE_CHECK: '1', NIB_ADDR: `127.0.0.1:${PORT}` },
  stdio: ['ignore', fs.openSync(log, 'a'), fs.openSync(log, 'a')],
});
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const logText = () => { try { return fs.readFileSync(log, 'utf8'); } catch { return ''; } };
const opens = () => { const m = [...logText().matchAll(/window (connected|gone) \((\d+) open\)/g)]; return m.length ? Number(m[m.length - 1][2]) : null; };

for (let i = 0; i < 80; i++) { try { const r = await fetch(`${BASE}/api/status`); if (r.ok) break; } catch {} await sleep(250); }

const browser = await chromium.launch({ executablePath: BROWSER, headless: true });
const ctx = await browser.newContext();
const page = await ctx.newPage();
await page.goto(BASE, { waitUntil: 'domcontentloaded' });
for (let i = 0; i < 60 && opens() !== 1; i++) await sleep(250);
console.log(`[setup] after opening a window the server reports ${opens()} open`);
if (opens() !== 1) { console.log('SETUP FAILED — no stream, so nothing below is measured'); process.exit(2); }

const cdp = await page.context().newCDPSession(page);

// ── Observation 1: explicit freeze ──────────────────────────────────────────
await cdp.send('Page.setWebLifecycleState', { state: 'frozen' });
await sleep(20_000);
console.log(`[frozen +20s]  open=${opens()}`);
await sleep(70_000);
console.log(`[frozen +90s]  open=${opens()}`);
await cdp.send('Page.setWebLifecycleState', { state: 'active' });
await sleep(3_000);
console.log(`[thawed]       open=${opens()}`);

// ── Observation 2: backgrounded past the automatic threshold ────────────────
// A second tab takes the foreground, so the first is a genuinely hidden page rather than one
// told to freeze. Chromium's own timer is what is being measured here.
const other = await ctx.newPage();
await other.goto('about:blank');
await other.bringToFront();
const start = Date.now();
for (const at of [60, 180, 300, 360, 420]) {
  while ((Date.now() - start) / 1000 < at) await sleep(2_000);
  console.log(`[hidden +${at}s] open=${opens()}`);
}

// ── The control: closing the page MUST drop it, or "still 1" means nothing ──
await page.close();
for (let i = 0; i < 40 && opens() !== 0; i++) await sleep(250);
console.log(`[closed]       open=${opens()}   <- control: this must be 0`);

console.log('\n--- server log ---');
console.log(logText().split('\n').filter((l) => l.includes('window ')).join('\n'));
await browser.close();
srv.kill();
