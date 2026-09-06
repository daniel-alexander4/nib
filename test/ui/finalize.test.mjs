// M5's finalize/export front end, live: the half the ledger called "unverified".
//
// The Go core is fully tested — sign, verify, tamper-detect, the watermark bake — and
// what was never driven is everything the browser does either side of it: baking the
// open document into multipart, posting it, taking the returned blob into the Save-As
// dialog, and writing it where the user chose. That last step is a folder browser no
// test had ever opened.
//
// The file it produces is read from disk with `fs`. A signed document that the app says
// it saved is not a signed document on disk, and only one of those is what the user has
// in the morning.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { launch, WORK } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const h = await launch();
const { page } = h;
after(() => h.browser.close());

const DOC = writeFixture('finalize.pdf', { pages: 2, label: 'finalize page' });
const OUT_DIR = path.join(WORK, 'finalized');
fs.mkdirSync(OUT_DIR, { recursive: true });

test('finalize signs the open document and writes it where you choose', async () => {
  await h.openDocument(DOC, 2);

  // The stimulus, and it is what makes "it is signed now" mean anything: the document
  // opens unsigned, and the badge says so.
  const before = await page.$eval('#sigBadge', (el) => el.textContent);
  assert.match(before, /Unsigned/,
    `setup: the badge already reads "${before}" before anything was signed, so a signature afterwards is not this test's doing`);

  await h.mode('secure'); // Finalize & sign moved to Secure's Certify group
  await h.group('Certify');
  await page.click('#finalizeBtn');
  await page.waitForFunction(() => !document.getElementById('finalizeModal').hidden);
  await page.click('#fzGo');

  // The Save-As dialog is the export path's last mile and no test had opened it. Its
  // folder field is filled directly rather than browsed to: browsing is the folder
  // browser's own behaviour, which pageops covers, and what this test is about is
  // whether the bytes reach the path.
  await page.waitForFunction(() => !document.getElementById('saveAsModal').hidden, null, { timeout: 30000 });
  await page.fill('#saveAsName', 'signed.pdf');
  await page.fill('#saveAsDir', OUT_DIR);
  await page.click('#saveAsGo');

  const out = path.join(OUT_DIR, 'signed.pdf');
  await page.waitForFunction(() => document.getElementById('saveAsModal').hidden);
  // Polled rather than read once: the click returns before the server has written, and
  // a single read would race it. If it never appears, that is the failure.
  const deadline = Date.now() + 15000;
  while (!fs.existsSync(out) && Date.now() < deadline) await page.waitForTimeout(200);
  assert.ok(fs.existsSync(out),
    `nothing was written to ${out}. The finalize round-trip returned a blob and the Save-As dialog closed, so the app believes it saved a signed document that is not there`);

  const bytes = fs.readFileSync(out);
  assert.ok(bytes.subarray(0, 5).toString() === '%PDF-',
    'the written file is not a PDF at all');
  // /ByteRange is the one part of a signature that CANNOT be hidden in an object stream:
  // it is the byte span the signature covers, and a verifier has to find and patch it in
  // the raw file. So its literal presence is a fact about a signed document rather than a
  // guess — and this repo has been caught before asserting a string that pdfcpu had
  // compressed out of reach.
  assert.ok(bytes.includes('/ByteRange'),
    `the saved file carries no /ByteRange, so it is not signed. ${bytes.length} bytes were written — the export path ran and produced an unsigned document`);
});

test('this file leaves the shared server as it found it', async () => {
  // Tier 3 runs every file against ONE nib process. Observe, clean up, THEN assert.
  const openPages = (await h.counts()).pages;
  await h.closeDocument();
  const left = await page.evaluate(() => document.querySelectorAll('.viewerContainer .page').length);

  assert.ok(openPages > 0, 'setup: no document was open, so this cleanup covered nothing');
  assert.equal(left, 0,
    `${left} page divs survive the close — the next file in this tier will count them as its own`);
});
