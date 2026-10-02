// PLAN-returned-document P03.S02 — the verdict, live: a document signed in THIS app, opened back, and the sheet asked.
//
// Tier 2 stubs every answer; this is the one place the whole chain runs for real — the server saying which signature
// is yours (`signerWhose`), the route recovering your version from the file's own bytes, and the sheet wording it.
// Two documents: the signed file exactly as written, and the same file with bytes appended after it, which is the
// returned-document case in its smallest form.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { launch, WORK, shutdown } from './harness.mjs';
import { writeFixture } from './fixtures.mjs';

const h = await launch();
const { page } = h;
after(() => shutdown(h));

const DOC = writeFixture('returnedverdict.pdf', { pages: 1, label: 'returned verdict page' });
const OUT_DIR = path.join(WORK, 'returned');
fs.mkdirSync(OUT_DIR, { recursive: true });
const SIGNED = path.join(OUT_DIR, 'signed.pdf');
const GREW = path.join(OUT_DIR, 'signed-then-added.pdf');

async function verdictFor(file) {
  await h.openDocument(file, 1);
  await h.mode('secure');
  await h.group('Sign & Timestamp');
  await page.click('#returnedBtn');
  await page.waitForSelector('#returnedSheet:not([hidden])');
  await page.waitForFunction(() => {
    const t = document.getElementById('returnedVerdict').textContent;
    return t && !/Looking for the version you signed/.test(t);
  }, null, { timeout: 30000 });
  const text = await page.evaluate(() => document.getElementById('returnedVerdict').textContent + '\n'
    + document.getElementById('returnedSigners').textContent);
  await page.click('#returnedClose');
  await h.closeDocument();
  return text;
}

test('a document signed here and opened back: the sheet finds your version, and then the bytes added after it', async () => {
  await h.openDocument(DOC, 1);
  await h.mode('secure');
  await h.group('Sign & Timestamp');
  await page.click('#finalizeBtn');
  await page.waitForFunction(() => !document.getElementById('finalizeModal').hidden);
  await page.click('#fzGo');
  await page.waitForFunction(() => !document.getElementById('saveAsModal').hidden, null, { timeout: 30000 });
  await page.fill('#saveAsName', 'signed.pdf');
  await page.fill('#saveAsDir', OUT_DIR);
  await page.click('#saveAsGo');
  const deadline = Date.now() + 15000;
  while (!fs.existsSync(SIGNED) && Date.now() < deadline) await page.waitForTimeout(200);
  assert.ok(fs.existsSync(SIGNED) && fs.readFileSync(SIGNED).includes('/ByteRange'), 'setup: no signed document was written');
  await h.closeDocument();

  const exact = await verdictFor(SIGNED);
  assert.match(exact, /exactly the version you signed/, `the untouched signed file read: ${exact}`);
  assert.match(exact, /yours/, 'the signer list does not mark your signature as yours');
  assert.doesNotMatch(exact, /Untampered/);

  // The returned-document case at its smallest: bytes after your signature's %%EOF.
  fs.writeFileSync(GREW, Buffer.concat([fs.readFileSync(SIGNED), Buffer.from('\n% added after signing\n')]));
  const grew = await verdictFor(GREW);
  assert.match(grew, /The version you signed is inside this file, and \d+ bytes were added after it/,
    `the grown file read: ${grew}`);
});
