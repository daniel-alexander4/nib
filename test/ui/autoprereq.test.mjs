import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { launch, shutdown, WORK } from './harness.mjs';
import { makeScanPDF, writeRawFixture, writeRuledFixture } from './fixtures.mjs';

// A command runs its own prerequisite — ADR-106, through the real binary with a REAL read.
//
// test/jsdom/autoprereq.test.mjs holds every rule of the door against a stubbed recogniser. What it cannot hold is
// the one thing the feature is for: a page that is a picture of words, the shipped tesseract reading it in a real
// browser, the server stamping what was read, pdf.js loading the result, and the command the user pressed then
// working — in one press. That is here, on a page whose words are drawn into the picture by this browser's own
// canvas, so the fixture is a scan in every sense the app can tell: no glyph is set on it.
//
// **Few on purpose.** A read is seconds of wasm, so this is three flows and not a suite: one per-page command
// (the table export, twice — the two timings are the cost measurement the ADR quotes), the redaction search
// (the path where an unread scan is a leak), and Save as fillable form… running Detect fields.

const h = await launch();
const { page } = h;
after(() => shutdown(h));

const OUT_DIR = path.join(WORK, 'autoprereq-out');
fs.mkdirSync(OUT_DIR, { recursive: true });

// Each page is `lines` drawn in black on white at 150 dpi, handed back as one grey byte a pixel.
const W = 1275, H = 1650;
async function scanOf(name, pages) {
  const grays = await page.evaluate(({ w, h: hh, pgs }) => pgs.map((lines) => {
    const c = document.createElement('canvas');
    c.width = w; c.height = hh;
    const g = c.getContext('2d');
    g.fillStyle = '#fff'; g.fillRect(0, 0, w, hh);
    g.fillStyle = '#000'; g.font = '56px Arial, Helvetica, sans-serif';
    lines.forEach((t, i) => g.fillText(t, 120, 240 + i * 120));
    const d = g.getImageData(0, 0, w, hh).data;
    const grey = new Uint8Array(w * hh);
    for (let i = 0; i < grey.length; i++) grey[i] = d[i * 4];
    let s = '';
    for (let i = 0; i < grey.length; i += 0x8000) s += String.fromCharCode(...grey.subarray(i, i + 0x8000));
    return btoa(s);
  }), { w: W, h: H, pgs: pages });
  const fields = grays.map((b) => Buffer.from(b, 'base64'));
  return writeRawFixture(name, makeScanPDF(pages.map((_, i) => i), {}, { w: W, h: H, field: (idx) => fields[idx] }));
}

const toast = () => page.evaluate(() => document.getElementById('toast')?.textContent ?? '');
const told = () => page.evaluate(async () => (await window.nibFetch('/api/ocr/pages')).json());
const READ_TIMEOUT = 180000;

// tableOf presses This page's table… and returns how long the press took to reach its Save dialog, and how long
// the reading note took to appear (-1 when it never did: nothing was read).
async function tableOf() {
  await h.mode('file');
  await h.card('Export & Print');
  const t0 = Date.now();
  await page.click('#exportTableBtn');
  let noteAt = -1;
  const done = page.waitForFunction(() => !document.getElementById('saveAsModal').hidden, null, { timeout: READ_TIMEOUT });
  const noted = page.waitForFunction(() => { const n = document.getElementById('readingNote'); return n && !n.hidden; }, null, { timeout: READ_TIMEOUT })
    .then(() => { noteAt = Date.now() - t0; }).catch(() => {});
  await done;
  const took = Date.now() - t0;
  await Promise.race([noted, page.waitForTimeout(50)]);
  return { took, noteAt };
}

test('This page\'s table… on a scanned page reads that one page — a real read — and then offers its table', { timeout: 420000 }, async () => {
  const doc = await scanOf('prereq-scan.pdf', [['Invoice number 4471', 'Total due 1250'], ['Second page ledger', 'Balance 9982']]);
  await h.openDocument(doc, 2);
  assert.deepEqual(await told(), { layered: [], own: [], unread: [1, 2] }, 'setup: the fixture is not two scanned pages nobody has read');

  const cold = await tableOf();
  assert.ok(cold.noteAt >= 0, 'nothing on the page said a read was under way');
  assert.equal(await page.$eval('#readingNote', (n) => n.hidden), true, 'the reading note is still up after the read ended');
  const after1 = await told();
  assert.deepEqual(after1.layered, [1], 'the page the command was on was not read, or another page was');
  assert.deepEqual(after1.unread, [2], 'page 2 was read too — the command is about one page');
  // What was read is what the table is made of: saved as CSV and read back from disk.
  await page.selectOption('#saveAsFormat', { label: 'Comma-separated values (.csv)' });
  const name = await page.inputValue('#saveAsName');
  await page.fill('#saveAsDir', OUT_DIR);
  await page.click('#saveAsGo');
  await page.waitForFunction(() => document.getElementById('saveAsModal').hidden, null, { timeout: 60000 });
  const out = path.join(OUT_DIR, name);
  const deadline = Date.now() + 15000;
  while (!fs.existsSync(out) && Date.now() < deadline) await page.waitForTimeout(200);
  assert.match(fs.readFileSync(out, 'utf8'), /Invoice/i, 'the table saved after the read does not hold the words on the scanned page');

  // The second page: the same press, a second read in the same session.
  await page.evaluate(() => {
    const f = document.querySelector('.viewerContainer:not([hidden])') && document.querySelector('#sbPages .pageNum, .pageNum');
    f.value = '2';
    f.dispatchEvent(new Event('change'));
  });
  await page.waitForFunction(() => document.querySelector('#sbPages .pageNum, .pageNum').value === '2');
  const warm = await tableOf();
  await page.click('#saveAsCancel');
  assert.deepEqual((await told()).layered, [1, 2], 'the second page was not read by the second press');

  // A third press on a page that now has its layer reads nothing.
  const again = await tableOf();
  await page.click('#saveAsCancel');
  assert.equal(again.noteAt, -1, 'a page that already has a text layer was read again');

  // The cost, printed because the ADR quotes it: one page, press to result.
  console.log(`# one-page read, press to the command's result: first in the session ${cold.took} ms (note up after ${cold.noteAt} ms), second ${warm.took} ms (note up after ${warm.noteAt} ms); no read ${again.took} ms`);
  assert.deepEqual(h.consoleErrors, [], 'the read left errors in the console');
});

test('Redact text… over a scan reads it and marks the word it holds, in one press', { timeout: 300000 }, async () => {
  const doc = await scanOf('prereq-redact.pdf', [['Account holder Margaret', 'Reference 55-1023']]);
  await h.openDocument(doc, 1);
  await h.mode('secure');
  await page.click('#redactTextBtn');
  await page.fill('#rtTerm', 'Margaret');
  await page.click('#rtFind');
  // The dialog says it is reading, and closes itself when the search has marked something.
  await page.waitForFunction(() => /Reading 1 scanned page first/.test(document.getElementById('rtStatus').textContent), null, { timeout: 30000 });
  await page.waitForFunction(() => document.getElementById('redactTextModal').hidden, null, { timeout: READ_TIMEOUT });
  assert.match(await toast(), /^1 match\(es\) marked on 1 page\(s\)/, 'the word on the scanned page was not found by the search that read it');
  assert.doesNotMatch(await toast(), /NOT searched/);
  assert.deepEqual((await told()).unread, [], 'the scanned page is still unread after the search');
});

test('Save as fillable form… with no fields runs Detect fields and goes on to name what it found', async () => {
  const doc = writeRuledFixture('prereq-form.pdf', { rules: 2 });
  await h.openDocument(doc, 1);
  await h.topOfDocument();
  await h.mode('file');
  await h.card('Save a Copy');
  await page.click('#saveFillableBtn');
  await page.waitForFunction(() => !document.getElementById('fieldNameModal').hidden, null, { timeout: 30000 });
  assert.equal(await page.$$eval('#fieldNameList input', (els) => els.length), 2,
    'the naming dialog does not list the two blanks Detect fields finds on this page');
});
