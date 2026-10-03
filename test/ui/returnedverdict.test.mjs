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

// P03.S03 — "see what changed", live: the version the route recovered is handed to the shipped Compare as bytes and the
// real pdf.js parses it; the chain is offered in its order. The appended bytes are a comment, so the text is identical —
// which is the point here: the differ ran over the server's bytes. Which way the diff reads is tier 2's
// (`returnedcompare.test.mjs`), where the two texts can differ.
test('the recovered version opens in Compare from the server\'s bytes, and the chain is offered in order', async () => {
  assert.ok(fs.existsSync(GREW), 'setup: the grown file from the test above is missing');
  await h.openDocument(GREW, 1);
  await h.mode('secure');
  await h.group('Sign & Timestamp');
  await page.click('#returnedBtn');
  await page.waitForSelector('#returnedCmpSigned:not([hidden])', { timeout: 30000 });
  const chain = await page.$$eval('#returnedCompare .rvchain > li', (lis) => lis.map((li) => li.textContent.trim()));
  assert.equal(chain.length, 3);
  assert.match(chain[0], /copy kept when you signed/);
  assert.match(chain[1], /ceremony/);
  assert.match(chain[2], /file you choose/);
  await page.click('#returnedCmpSigned');
  await page.waitForSelector('#compareModal:not([hidden])');
  await page.waitForFunction(() => /No text differences|→ this file/.test(document.getElementById('compareBody').textContent),
    null, { timeout: 30000 });
  const body = await page.textContent('#compareBody');
  assert.match(body, /No text differences/, `Compare over the recovered version read: ${body}`);
  assert.equal(await page.isVisible('#compareTools'), true, 'the compared document did not load');
  await page.click('#compareClose');
  // The ceremony link over the real route: this document was signed solo, so it names no ceremony and says so.
  await page.click('#returnedCmpCeremony');
  await page.waitForFunction(() => /ceremony/.test(document.getElementById('returnedCmpCeremonyNote').textContent),
    null, { timeout: 30000 });
  assert.match(await page.textContent('#returnedCmpCeremonyNote'), /names no signing ceremony/);
  assert.equal(await page.isVisible('#compareModal'), false, 'a refused ceremony copy opened Compare');
  await page.click('#returnedClose');
  await h.closeDocument();
});

// P04.S01 — "Keep a copy for my records", live: Finalize with the tick on the real binary. The confirmation is sent only
// after the server's durable write landed (`X-Nib-Kept` follows `saveKept`), and the tick is off again at the next opening.
test('Finalize with "Keep a copy" keeps one and says so; the tick is off at the next opening', async () => {
  await h.openDocument(DOC, 1);
  await h.mode('secure');
  await h.group('Sign & Timestamp');
  await page.click('#finalizeBtn');
  await page.waitForFunction(() => !document.getElementById('finalizeModal').hidden);
  assert.equal(await page.isChecked('#fzKeep'), false, 'the tick started ON');
  await page.check('#fzKeep');
  await page.click('#fzGo');
  await page.waitForFunction(() => /A copy was kept when you signed/.test(document.getElementById('toast')?.textContent || ''),
    null, { timeout: 30000 });
  assert.match(await page.textContent('#toast'), /~\/nib\/signed\/kept_returnedverdict_\d{8}-\d{6}-[0-9a-f]{8}\.pdf/);
  await page.waitForFunction(() => !document.getElementById('saveAsModal').hidden, null, { timeout: 30000 });
  await page.click('#saveAsCancel');
  await page.click('#finalizeBtn');
  await page.waitForFunction(() => !document.getElementById('finalizeModal').hidden);
  assert.equal(await page.isChecked('#fzKeep'), false, 'the tick was remembered from the last opening');
  await page.click('#fzCancel');
  await h.closeDocument();
});

// P04.S02 — the whole loop, live: Finalize with the tick and save the result; append bytes (the document "came back");
// open it, and the sheet's FIRST link finds the copy kept when you signed and compares against it. Then the list shows
// that copy and removes it after the confirm.
// P04.S03's row reads that same match: the Simple Sign checklist, asked through the one door.
const keptRowState = async () => {
  await h.mode('collaborate');
  await h.card('Simple Sign');
  const want = 'A copy kept when you signed';
  await page.waitForFunction((l) => [...document.getElementById('signSteps').children]
    .some((r) => r.querySelector('.signstep-label').textContent === l && r.querySelector('.signstep-mark').title
      && !/not checked this document yet/.test(r.querySelector('.signstep-mark').title)), want, { timeout: 15000 });
  return page.evaluate((l) => {
    const r = [...document.getElementById('signSteps').children].find((x) => x.querySelector('.signstep-label').textContent === l);
    return { state: r.dataset.state, by: r.dataset.by, title: r.querySelector('.signstep-mark').title };
  }, want);
};

test('a document kept when signed and returned changed is matched to its kept copy, which the list then removes', async () => {
  await h.openDocument(DOC, 1);
  const unsigned = await keptRowState();
  assert.equal(unsigned.state, 'untracked', 'the checklist row ticked for a document that carries no signature');
  assert.match(unsigned.title, /carries no signature/);
  await h.mode('secure');
  await h.group('Sign & Timestamp');
  await page.click('#finalizeBtn');
  await page.waitForFunction(() => !document.getElementById('finalizeModal').hidden);
  await page.check('#fzKeep');
  await page.click('#fzGo');
  await page.waitForFunction(() => !document.getElementById('saveAsModal').hidden, null, { timeout: 30000 });
  const KEPT_OUT = path.join(OUT_DIR, 'kept-then-returned.pdf');
  await page.fill('#saveAsName', 'kept-signed.pdf');
  await page.fill('#saveAsDir', OUT_DIR);
  await page.click('#saveAsGo');
  const saved = path.join(OUT_DIR, 'kept-signed.pdf');
  const deadline = Date.now() + 15000;
  while (!fs.existsSync(saved) && Date.now() < deadline) await page.waitForTimeout(200);
  assert.ok(fs.existsSync(saved), 'setup: the finalized document was not saved');
  await h.closeDocument();
  fs.writeFileSync(KEPT_OUT, Buffer.concat([fs.readFileSync(saved), Buffer.from('\n% came back with this added\n')]));

  await h.openDocument(KEPT_OUT, 1);
  const matched = await keptRowState();
  assert.equal(matched.state, 'done', `the checklist row did not find the copy kept when this was signed (${matched.title})`);
  assert.equal(matched.by, 'nib');
  await h.mode('secure');
  await h.group('Sign & Timestamp');
  await page.click('#returnedBtn');
  await page.waitForSelector('#returnedSheet:not([hidden])');
  await page.click('#returnedCmpKept');
  await page.waitForSelector('#compareModal:not([hidden])', { timeout: 30000 });
  assert.match(await page.textContent('#returnedCmpKeptNote'), /begins with that copy/);
  await page.click('#compareClose');
  await page.click('#returnedClose');
  // The list lives in the Sign & Timestamp card, which the menu strip shows once a document is open (ADR-037).
  await page.click('#keptBtn');
  await page.waitForSelector('#keptModal:not([hidden]) .keptrow', { timeout: 15000 });
  const before = await page.$$eval('#keptList .keptrow', (rows) => rows.length);
  assert.ok(before >= 1, 'the list shows no kept copy');
  const asked = h.dialogs.length;
  await page.click('#keptList .keptrow button'); // the harness accepts the confirm and records what it said
  await page.waitForFunction((n) => document.querySelectorAll('#keptList .keptrow').length === n - 1, before, { timeout: 15000 });
  assert.equal(h.dialogs.length, asked + 1, 'the removal asked for no confirmation');
  assert.match(h.dialogs.at(-1), /permanent/);
  await page.click('#keptClose');
  // The copy is gone; coming back to the checklist re-asks, and the row reads ○ — never the ✓ it showed before.
  const after = await keptRowState();
  assert.equal(after.state, 'todo', `the row still reads ${after.state} after its kept copy was removed (${after.title})`);
  await h.closeDocument();
});
