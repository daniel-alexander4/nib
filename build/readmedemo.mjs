// Records the README's demo: a flat form opened, its blanks detected, and filled in.
//
// Driven by build/readmedemo.sh, which starts a throwaway nib and hands over NIB_DEMO_BASE,
// NIB_DEMO_CSRF, NIB_DEMO_BROWSER and NIB_DEMO_WORK. This is not a test: it asserts only what it
// needs to keep recording (that Detect placed the fields the form draws), and it writes a video
// Chrome's screencast frames into $NIB_DEMO_WORK/frames, with a concat list giving each frame its
// own duration, for the shell script to turn into docs/readme-demo.gif. (Playwright's recordVideo
// needs a browser-bundled ffmpeg download; the screencast needs nothing but the browser.)
//
// The form is drawn here, from nothing, so the GIF never shows anyone's real document.
import fs from 'node:fs';
import path from 'node:path';
import { chromium } from 'playwright-core';

const BASE = process.env.NIB_DEMO_BASE;
const CSRF = process.env.NIB_DEMO_CSRF;
const WORK = process.env.NIB_DEMO_WORK;
const BROWSER = process.env.NIB_DEMO_BROWSER;
if (!BASE || !CSRF || !WORK || !BROWSER) throw new Error('run build/readmedemo.sh, not this file');

// What the viewer types, one row per blank, top to bottom.
const ANSWERS = ['Jordan Rivera', '14 Elm Street, Springfield', 'jordan@example.com', '555-0142', '19 Nov 2026'];
const LABELS = ['Full name', 'Address', 'Email', 'Phone', 'Date'];

// A one-page US Letter form: a heading, a label per line and a 0.5pt rule to write on. The rules
// follow test/ui/fixtures.mjs makeRuledPDF's geometry so web/detect.js reads each one as a blank.
function formPDF() {
  const esc = (s) => s.replace(/[\\()]/g, '\\$&');
  const ops = ['BT /F2 22 Tf 72 712 Td (' + esc('Volunteer Sign-up') + ') Tj ET',
    'BT /F1 11 Tf 72 690 Td (' + esc('Please fill in every line and return this form to the front desk.') + ') Tj ET',
    '0.5 w 0 G'];
  LABELS.forEach((l, i) => {
    const y = 630 - i * 56;
    ops.push(`BT /F1 12 Tf 72 ${y + 3} Td (${esc(l)}) Tj ET`);
    ops.push(`160 ${y} m 520 ${y} l S`);
  });
  const content = ops.join('\n');
  const objs = [
    '<< /Type /Catalog /Pages 2 0 R >>',
    '<< /Type /Pages /Kids [3 0 R] /Count 1 >>',
    '<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R '
      + '/Resources << /Font << /F1 5 0 R /F2 6 0 R >> >> >>',
    `<< /Length ${content.length} >>\nstream\n${content}\nendstream`,
    '<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>',
    '<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>',
  ];
  let out = '%PDF-1.7\n';
  const offsets = [];
  objs.forEach((o, i) => { offsets.push(out.length); out += `${i + 1} 0 obj\n${o}\nendobj\n`; });
  const xref = out.length;
  out += `xref\n0 ${objs.length + 1}\n0000000000 65535 f \n`;
  for (const o of offsets) out += `${String(o).padStart(10, '0')} 00000 n \n`;
  out += `trailer\n<< /Size ${objs.length + 1} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF\n`;
  return Buffer.from(out, 'latin1');
}

const doc = path.join(WORK, 'volunteer-sign-up.pdf');
fs.writeFileSync(doc, formPDF());
const frameDir = path.join(WORK, 'frames');
fs.mkdirSync(frameDir, { recursive: true });

const key = await (await fetch(BASE + '/api/launch/key', { method: 'POST', headers: { 'X-CSRF-Token': CSRF } })).json()
  .then((j) => j.key);
const browser = await chromium.launch({ executablePath: BROWSER, headless: true });
const size = { width: 1100, height: 720 };
const context = await browser.newContext({ viewport: size });
const page = await context.newPage();
const frames = []; // [file, timestamp in seconds]
const cdp = await context.newCDPSession(page);
cdp.on('Page.screencastFrame', async ({ data, metadata, sessionId }) => {
  const file = path.join(frameDir, `f${String(frames.length).padStart(5, '0')}.jpg`);
  fs.writeFileSync(file, Buffer.from(data, 'base64'));
  frames.push([file, metadata.timestamp]);
  await cdp.send('Page.screencastFrameAck', { sessionId }).catch(() => {});
});
const pause = (ms) => page.waitForTimeout(ms);
const visibleCard = async (label) => {
  const open = await page.evaluate((l) => {
    const h = [...document.querySelectorAll('.sbhead.groupcard')].find((x) => x.textContent.trim() === l);
    return !!h && h.getAttribute('aria-expanded') === 'true';
  }, label);
  if (!open) await page.click(`.sbhead.groupcard:text-is("${label}")`);
};

await page.goto(BASE + '#k=' + key);
await page.waitForSelector('#empty', { state: 'attached' });
// Boot is not recorded: the screencast starts on the ready window.
await cdp.send('Page.startScreencast', { format: 'jpeg', quality: 92, maxWidth: size.width, maxHeight: size.height });
await pause(600);

// Open the form through the real Open… dialog.
await page.click('[data-tab="file"]');
await visibleCard('Open & Import');
await page.click('#openMenuItem');
await page.fill('#pathInput', doc);
await pause(300);
await page.click('#openGo');
await page.waitForFunction(() => document.getElementById('viewerWrap').classList.contains('has-doc'));
await page.waitForSelector('.viewerContainer:not([hidden]) .page canvas');
await pause(900);

// Detect the blanks.
await page.click('[data-tab="markup"]');
await visibleCard('Detect & Fill Fields');
await pause(500);
await page.click('#detectBtn');
await page.waitForFunction(
  (n) => document.querySelectorAll('.viewerContainer:not([hidden]) .ovl-text').length >= n,
  LABELS.length, { timeout: 15000 });
await pause(700);

// Fill them in, top to bottom.
const fields = page.locator('.viewerContainer:not([hidden]) .ovl-text');
const count = await fields.count();
if (count !== LABELS.length) throw new Error(`Detect placed ${count} fields on a form that draws ${LABELS.length}`);
for (let i = 0; i < count; i++) {
  await fields.nth(i).click();
  await fields.nth(i).pressSequentially(ANSWERS[i], { delay: 28 });
  await pause(150);
}
await page.mouse.click(1080, 700); // blur the last field so its caret is not the closing frame
await pause(1200);

await cdp.send('Page.stopScreencast');
await context.close();
await browser.close();
// The screencast sends a frame only when the screen changes, so each frame lasts until the next.
const last = frames[frames.length - 1];
const lines = ['ffconcat version 1.0'];
frames.forEach(([file, ts], i) => {
  const next = i + 1 < frames.length ? frames[i + 1][1] : ts + 1.2;
  lines.push(`file '${file}'`, `duration ${Math.max(0.01, next - ts).toFixed(3)}`);
});
lines.push(`file '${last[0]}'`); // concat's last entry carries no duration
fs.writeFileSync(path.join(WORK, 'frames.txt'), lines.join('\n') + '\n');
fs.writeFileSync(path.join(WORK, 'demo-seconds'), String(last[1] + 1.2 - frames[0][1]));
