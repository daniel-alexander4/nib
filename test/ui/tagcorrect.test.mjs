// A producer's broken tree, corrected end to end without leaving nib — `PLAN-accessibility.md` P09.S07.
//
// ── What only this tier can see ──────────────────────────────────────────────
// The phase's exit criterion, as one person would meet it: a document from another producer arrives with
// a figure that has no alternate text and header cells with no scope; nib's own report shows both
// problems; the person corrects all three in the Tags panel with the keyboard; the report shows both
// clauses passing; veraPDF agrees; and undo takes each correction back.
//
// ── Where the document comes from ────────────────────────────────────────────
// LibreOffice converts an ODT holding a heading, a paragraph, a table with a header row and an image with
// a title — the table-and-figure document the oracle corpus uses (P09.S05). Its `/Alt` and both `/Scope`
// entries are then renamed in the bytes, after checking the file has no object stream a byte edit could
// not see into. Without LibreOffice every test here SKIPS and says so.
//
// ── The rule this file lives by ──────────────────────────────────────────────
// The setup uses the harness. The CORRECTION uses the keyboard alone, and the last-but-one test scans that
// region of this file for a pointer or a harness helper.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import zlib from 'node:zlib';
import { execFileSync } from 'node:child_process';
import { launch, shutdown } from './harness.mjs';

const onPath = (name) => (process.env.PATH || '').split(path.delimiter).map((d) => path.join(d, name)).find((p) => {
  try { fs.accessSync(p, fs.constants.X_OK); return true; } catch { return false; }
});
const SOFFICE = onPath('soffice') || onPath('libreoffice') || '';
// The ACCOUNT's home, not $HOME: uirepro.sh points HOME at its work directory for the processes it
// starts, and the first run looked for ~/verapdf there and skipped the oracle on a machine that has it.
const HOME_VERAPDF = [os.homedir(), os.userInfo().homedir].map((d) => path.join(d, 'verapdf', 'verapdf')).find((p) => fs.existsSync(p)) || '';
const VERAPDF = process.env.NIB_VERAPDF || onPath('verapdf') || HOME_VERAPDF;
const SKIP = SOFFICE ? false : 'SKIP (not a pass): LibreOffice is absent, so there is no producer\'s document to correct and P09\'s exit criterion is UNCHECKED in this run.';

// zipStored writes a zip whose entries are all STORED, with a real date — ODF requires `mimetype` first and
// uncompressed, and LibreOffice refuses a zero date (measured in `internal/pdfops`' ODT builder).
function zipStored(entries) {
  const date = ((2026 - 1980) << 9) | (9 << 5) | 14;
  const parts = [];
  const central = [];
  let offset = 0;
  for (const { name, data } of entries) {
    const n = Buffer.from(name);
    const crc = zlib.crc32(data) >>> 0;
    const local = Buffer.alloc(30);
    local.writeUInt32LE(0x04034b50, 0); local.writeUInt16LE(20, 4); local.writeUInt16LE(date, 12);
    local.writeUInt32LE(crc, 14); local.writeUInt32LE(data.length, 18); local.writeUInt32LE(data.length, 22); local.writeUInt16LE(n.length, 26);
    parts.push(local, n, data);
    const c = Buffer.alloc(46);
    c.writeUInt32LE(0x02014b50, 0); c.writeUInt16LE(20, 4); c.writeUInt16LE(20, 6); c.writeUInt16LE(date, 14);
    c.writeUInt32LE(crc, 16); c.writeUInt32LE(data.length, 20); c.writeUInt32LE(data.length, 24); c.writeUInt16LE(n.length, 28); c.writeUInt32LE(offset, 42);
    central.push(c, n);
    offset += local.length + n.length + data.length;
  }
  const cd = Buffer.concat(central);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0); end.writeUInt16LE(entries.length, 8); end.writeUInt16LE(entries.length, 10);
  end.writeUInt32LE(cd.length, 12); end.writeUInt32LE(offset, 16);
  return Buffer.concat([...parts, cd, end]);
}

function grey(w, h) {
  const chunk = (type, data) => {
    const t = Buffer.from(type);
    const len = Buffer.alloc(4); len.writeUInt32BE(data.length);
    const crc = Buffer.alloc(4); crc.writeUInt32BE(zlib.crc32(Buffer.concat([t, data])) >>> 0);
    return Buffer.concat([len, t, data, crc]);
  };
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(w, 0); ihdr.writeUInt32BE(h, 4); ihdr[8] = 8; ihdr[9] = 2;
  const row = Buffer.concat([Buffer.from([0]), Buffer.alloc(w * 3, 0x80)]);
  return Buffer.concat([Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]), chunk('IHDR', ihdr),
    chunk('IDAT', zlib.deflateSync(Buffer.concat(Array.from({ length: h }, () => row)))), chunk('IEND', Buffer.alloc(0))]);
}

// brokenDocument converts the table-and-figure ODT and strips its alternate text and header scopes.
function brokenDocument(dir) {
  const cell = (s) => `<table:table-cell office:value-type="string"><text:p>${s}</text:p></table:table-cell>`;
  const body = '<text:h text:outline-level="1">Report</text:h><text:p>Intro paragraph.</text:p>' +
    '<table:table table:name="T1"><table:table-column table:number-columns-repeated="2"/>' +
    `<table:table-header-rows><table:table-row>${cell('Name')}${cell('Qty')}</table:table-row></table:table-header-rows>` +
    `<table:table-row>${cell('Apple')}${cell('3')}</table:table-row><table:table-row>${cell('Pear')}${cell('5')}</table:table-row></table:table>` +
    '<text:p><draw:frame draw:name="img1" text:anchor-type="as-char" svg:width="2cm" svg:height="1.5cm">' +
    '<draw:image xlink:href="Pictures/a.png" xlink:type="simple" xlink:show="embed" xlink:actuate="onLoad"/>' +
    '<svg:title>A grey square</svg:title></draw:frame></text:p>';
  const content = '<?xml version="1.0" encoding="UTF-8"?><office:document-content' +
    ' xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0"' +
    ' xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0" xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0"' +
    ' xmlns:svg="urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0" xmlns:xlink="http://www.w3.org/1999/xlink" office:version="1.2">' +
    `<office:body><office:text>${body}</office:text></office:body></office:document-content>`;
  const manifest = '<?xml version="1.0" encoding="UTF-8"?><manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0" manifest:version="1.2">' +
    '<manifest:file-entry manifest:full-path="/" manifest:media-type="application/vnd.oasis.opendocument.text"/>' +
    '<manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/>' +
    '<manifest:file-entry manifest:full-path="Pictures/a.png" manifest:media-type="image/png"/></manifest:manifest>';
  const odt = path.join(dir, 'report.odt');
  fs.writeFileSync(odt, zipStored([
    { name: 'mimetype', data: Buffer.from('application/vnd.oasis.opendocument.text') },
    { name: 'content.xml', data: Buffer.from(content) },
    { name: 'META-INF/manifest.xml', data: Buffer.from(manifest) },
    { name: 'Pictures/a.png', data: grey(40, 30) },
  ]));
  execFileSync(SOFFICE, ['--headless', '--nologo', '--nofirststartwizard', `-env:UserInstallation=file://${dir}/profile`,
    '--convert-to', 'pdf', '--outdir', dir, odt], { timeout: 150000, stdio: 'ignore' });
  const pdf = fs.readFileSync(path.join(dir, 'report.pdf'));
  const text = pdf.toString('latin1');
  assert.equal((text.match(/\/ObjStm/g) || []).length, 0, 'setup: the converted document has object streams, so a byte strip cannot be trusted');
  assert.equal((text.match(/\/Alt/g) || []).length, 1, 'setup: the converted document does not carry exactly one /Alt');
  assert.equal((text.match(/\/Scope/g) || []).length, 2, 'setup: the converted document does not carry exactly two /Scope');
  const broken = path.join(dir, 'report-broken.pdf');
  fs.writeFileSync(broken, Buffer.from(text.replaceAll('/Alt', '/Xlt').replaceAll('/Scope', '/Xcope'), 'latin1'));
  return broken;
}

const DIR = fs.mkdtempSync(path.join(os.tmpdir(), 'nib-tagcorrect-'));
const h = await launch();
const { page } = h;
let foundHeld = null;
let brokenPath = '';

const heldDocs = () => page.evaluate(async () => (await (await fetch('/api/docs')).json()).docs.length);
const result = (clause) => page.evaluate(async (c) => (await (await fetch('/api/uacheck')).json()).results.find((r) => r.clause === c), clause);
const waitVerdict = (clause, verdict) => page.waitForFunction(async ([c, v]) =>
  (await (await fetch('/api/uacheck')).json()).results.find((r) => r.clause === c)?.verdict === v, [clause, verdict], { timeout: 30000, polling: 500 });

after(async () => {
  try {
    h.answerDialogs(true);
    for (let i = 0; i < 8 && await page.$eval('#viewerWrap', (el) => el.className) === 'has-doc'; i++) await h.closeDocument();
  } catch { /* the assertion that already failed is the one worth reporting */ }
  await shutdown(h);
  fs.rmSync(DIR, { recursive: true, force: true });
});

test('setup: the producer\'s broken document is open, and nib\'s report names both problems', { skip: SKIP }, async () => {
  brokenPath = brokenDocument(DIR);
  foundHeld = await heldDocs();
  await h.openDocument(brokenPath, 1);
  await h.mode('accessibility'); // ADR-035: the tree panel moved out of Page Functions with the two buttons
  await page.click('.tab[data-panel="tagtree"]');
  await page.waitForFunction(() => document.querySelectorAll('#tagTreeList [role="treeitem"]').length >= 20, null, { timeout: 20000 });
  const figure = await result('7.3 t1');
  const table = await result('7.5 t1');
  assert.equal(figure?.verdict, 'fail', `nib's report does not fail 7.3 t1 for a figure with no alt text: ${JSON.stringify(figure)}`);
  // `fail` since P03.S04 ported veraPDF's table layout — it was `cannot check` while nib did not build the grid.
  assert.equal(table?.verdict, 'fail', `nib's report does not fail the unscoped header cells: ${JSON.stringify(table)}`);
  assert.match(table.why || '', /header at row 1, cell 1/, `the report does not name the header cell to fix: ${JSON.stringify(table)}`);
});

// tabTo presses Tab (or Shift+Tab) until focus matches — reaching a control is part of the claim.
async function tabTo(selector, { back = false, max = 60 } = {}) {
  for (let i = 0; i < max; i++) {
    if (await page.evaluate((s) => !!document.activeElement?.matches?.(s), selector)) return true;
    await page.keyboard.press(back ? 'Shift+Tab' : 'Tab');
  }
  return false;
}

// arrowTo walks the tree from its first element (Home) down, until the focused element's label starts with
// prefix. From the top, so the direction never depends on where the last correction left focus — the first
// run pressed ArrowUp from "TH — Name" toward "TH — Qty", which comes after it.
async function arrowTo(prefix, max = 40) {
  await page.keyboard.press('Home');
  for (let i = 0; i < max; i++) {
    const here = await page.evaluate(() => {
      const a = document.activeElement;
      return a?.getAttribute?.('role') === 'treeitem' ? a.textContent.trim() : '';
    });
    if (here.startsWith(prefix)) return true;
    await page.keyboard.press('ArrowDown');
  }
  return false;
}

const changed = () => page.waitForFunction(() => /Ctrl\+Z/.test(document.getElementById('tagEditStatus').textContent), null, { timeout: 30000 });

test('the corrections are made by keyboard alone, and both clauses pass', { skip: SKIP }, async () => {
  const header = await page.evaluate(() => {
    document.querySelector('.tab[data-panel="tagtree"]').focus();
    return document.activeElement?.dataset?.panel;
  });
  assert.equal(header, 'tagtree', 'setup: the panel header cannot take focus');
  assert.ok(await tabTo('#tagTreeList [role="treeitem"]'), 'Tab from the panel header never reached the tree');

  // The figure's alternate text.
  assert.ok(await arrowTo('Figure'), 'the arrow keys never reached the figure');
  assert.ok(await tabTo('#tagEditAlt'), 'Tab never reached the alt text field');
  await page.keyboard.type('A grey square');
  await page.keyboard.press('Enter');
  await changed();
  await waitVerdict('7.3 t1', 'pass');

  // Each header cell's scope. Back into the tree first, from the field the reload returned focus to.
  for (const [cell, afterIt] of [['TH — Name', 'cannot check'], ['TH — Qty', 'pass']]) {
    assert.ok(await tabTo('#tagTreeList [role="treeitem"]', { back: true }), `Shift+Tab never returned to the tree before ${cell}`);
    assert.ok(await arrowTo(cell), `the arrow keys never reached ${cell}`);
    assert.ok(await tabTo('#tagEditScope'), `Tab never reached the scope picker for ${cell}`);
    await page.keyboard.type('C');
    await page.waitForFunction(() => document.getElementById('tagEditScope').value === 'Column', null, { timeout: 5000 });
    assert.ok(await tabTo('#tagEditScopeApply'), `Tab never reached Set scope for ${cell}`);
    await page.keyboard.press('Enter');
    await changed();
    await waitVerdict('7.5 t1', afterIt);
  }
});

test('veraPDF agrees: the stripped document fails both clauses, and the corrected one passes them', { skip: SKIP }, async (t) => {
  if (!VERAPDF) {
    t.skip('SKIP (not a pass): veraPDF is absent, so the corrected document is UNCHECKED against the oracle in this run.');
    return;
  }
  const bytes = await page.evaluate(async () => {
    const b = new Uint8Array(await (await fetch('/api/pdf')).arrayBuffer());
    let s = '';
    for (const x of b) s += String.fromCharCode(x);
    return btoa(s);
  });
  const corrected = path.join(DIR, 'report-corrected.pdf');
  fs.writeFileSync(corrected, Buffer.from(bytes, 'base64'));
  let out = '';
  try {
    out = execFileSync(VERAPDF, ['--flavour', 'ua1', '--passed', brokenPath, corrected], { timeout: 170000, maxBuffer: 64 << 20 }).toString();
  } catch (e) {
    out = (e.stdout || '').toString(); // veraPDF exits non-zero for a document that fails any rule
  }
  const jobs = out.split('<job>').slice(1);
  assert.equal(jobs.length, 2, `veraPDF reported ${jobs.length} job(s), want the stripped and the corrected document`);
  const state = (job, clause, number) => job.match(new RegExp(`<rule [^>]*clause="${clause.replace('.', '\\.')}"[^>]*testNumber="${number}"[^>]*status="(\\w+)"`))?.[1];
  const [broken, fixed] = jobs[0].includes('report-broken') ? jobs : [jobs[1], jobs[0]];
  assert.equal(state(broken, '7.3', 1), 'failed', 'setup: veraPDF does not fail 7.3 t1 on the stripped document');
  assert.equal(state(broken, '7.5', 1), 'failed', 'setup: veraPDF does not fail 7.5 t1 on the stripped document');
  assert.equal(state(fixed, '7.3', 1), 'passed', 'veraPDF still fails 7.3 t1 on the document corrected in nib');
  assert.equal(state(fixed, '7.5', 1), 'passed', 'veraPDF still fails 7.5 t1 on the document corrected in nib');
});

test('undo takes each correction back, clause by clause', { skip: SKIP }, async () => {
  // Focus is on Set scope, a button — not a text field, which would own Ctrl+Z.
  await page.keyboard.press('Control+z');
  await waitVerdict('7.5 t1', 'cannot check');
  await page.waitForFunction(() => [...document.querySelectorAll('#tagTreeList [role="treeitem"]')].some((i) => i.textContent.startsWith('TH — Qty — no scope')), null, { timeout: 30000 });
  await page.keyboard.press('Control+z');
  await page.waitForFunction(() => [...document.querySelectorAll('#tagTreeList [role="treeitem"]')].some((i) => i.textContent.startsWith('TH — Name — no scope')), null, { timeout: 30000 });
  assert.equal((await result('7.3 t1'))?.verdict, 'pass', 'undoing the scopes also took back the alt text');
  await page.keyboard.press('Control+z');
  await waitVerdict('7.3 t1', 'fail');
});

test('the correction region used no pointer at all', { skip: SKIP }, () => {
  const src = fs.readFileSync(new URL('./tagcorrect.test.mjs', import.meta.url), 'utf8');
  const START = "test('the corrections are made by keyboard alone";
  const STOP = "test('the correction region used no pointer at all'";
  const body = src.slice(src.indexOf(START), src.indexOf(STOP));
  assert.ok(body.length > 2000, `the scanned region is ${body.length} chars — an anchor has drifted`);
  for (const banned of ['page' + '.click(', 'page' + '.mouse', 'h' + '.openDocument(', 'h' + '.card(', 'h' + '.mode(', 'h' + '.panel(', 'h' + '.group(']) {
    assert.ok(!body.includes(banned), `the correction region calls ${banned}, so it proves a mouse or the harness can correct a tree, not a keyboard user`);
  }
});

test('this file leaves the shared server as it found it', { skip: SKIP }, async () => {
  h.answerDialogs(true);
  assert.notEqual(foundHeld, null, 'setup: the first test never ran, so there is no baseline to return to');
  await h.closeDocument();
  assert.equal(await heldDocs(), foundHeld, 'the server holds a different number of documents than this file found');
});
