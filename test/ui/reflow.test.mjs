import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { launch, shutdown, WORK } from './harness.mjs';
import { writeRawFixture } from './fixtures.mjs';

// Reflow a paragraph — PLAN-text-reflow.md P06, the whole flow through the real binary: the server reads the page's
// paragraphs, the dialog edits one, the server re-sets it in the document's own font and commits, and the page the user
// sees is re-rendered from the committed bytes. The assertions read pdf.js's text layer of that re-rendered page — a
// reader the server's Go code has no hand in — so "the old word is gone" is observed by a second reader, not claimed by
// the one that wrote it.

const h = await launch();
const { page } = h;
after(() => shutdown(h));

// A three-line Helvetica paragraph in one text object, built by hand so the operators on the page are known.
function paragraphPDF() {
  const content = 'BT /F1 14 Tf 18 TL 72 700 Td (The quick brown fox jumps) Tj T* (over the lazy dog and runs) Tj T* (away from here.) Tj ET';
  const objs = [
    '<< /Type /Catalog /Pages 2 0 R >>',
    '<< /Type /Pages /Kids [3 0 R] /Count 1 >>',
    '<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>',
    `<< /Length ${content.length} >>\nstream\n${content}\nendstream`,
    '<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>',
  ];
  let out = '%PDF-1.7\n';
  const offs = [];
  objs.forEach((o, i) => { offs.push(out.length); out += `${i + 1} 0 obj\n${o}\nendobj\n`; });
  const xref = out.length;
  out += `xref\n0 ${objs.length + 1}\n0000000000 65535 f \n` + offs.map((o) => `${String(o).padStart(10, '0')} 00000 n \n`).join('');
  out += `trailer\n<< /Size ${objs.length + 1} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF\n`;
  return Buffer.from(out, 'latin1');
}

const DOC = writeRawFixture('reflow.pdf', paragraphPDF());

const pageText = () => page.evaluate(() =>
  [...document.querySelectorAll('.viewerContainer:not([hidden]) .page .textLayer span')].map((s) => s.textContent).join(' '));

async function openReflow() {
  await h.mode('markup');
  await h.group('Edit Page Text');
  await page.click('#reflowBtn');
  await page.waitForFunction(() => !document.getElementById('reflowModal').hidden);
}

test('a word changed in the reflow dialog is re-set on the page, and the old word is gone', async () => {
  await h.openDocument(DOC, 1);
  await page.waitForFunction(() => /lazy/.test([...document.querySelectorAll('.viewerContainer:not([hidden]) .page .textLayer span')].map((s) => s.textContent).join(' ')));
  assert.match(await pageText(), /lazy/, 'setup: the page must show "lazy" before the edit, or its absence afterwards proves nothing');

  await openReflow();
  const listed = await page.$eval('#reflowText', (t) => t.value);
  assert.equal(listed, 'The quick brown fox jumps over the lazy dog and runs away from here.',
    'the dialog did not offer the paragraph as the server reads it');
  await page.fill('#reflowText', 'The quick brown fox jumps over the sleepy old dog and runs away from here.');
  await page.click('#reflowGo');
  await page.waitForFunction(() => document.getElementById('reflowModal').hidden);
  await page.waitForFunction(() => /sleepy/.test([...document.querySelectorAll('.viewerContainer:not([hidden]) .page .textLayer span')].map((s) => s.textContent).join(' ')));
  const after = await pageText();
  assert.match(after, /sleepy old dog/, `the re-rendered page reads: ${after}`);
  assert.doesNotMatch(after, /lazy/, `"lazy" is still on the page — covered, not removed: ${after}`);
});

test('a character the font cannot draw keeps the dialog open and says why', async () => {
  await openReflow();
  const before = await page.$eval('#reflowText', (t) => t.value);
  await page.fill('#reflowText', before.replace('fox', 'fox Ω'));
  await page.click('#reflowGo');
  await page.waitForFunction(() => !document.getElementById('reflowWhy').hidden);
  const why = await page.$eval('#reflowWhy', (p) => p.textContent);
  assert.match(why, /no glyph for a character/, `the refusal named nothing a user can act on: ${why}`);
  assert.match(why, /Edit text/, 'the refusal did not point at the fallback');
  assert.equal(await page.$eval('#reflowModal', (m) => m.hidden), false, 'the dialog closed on a refusal, losing the typed text');
  await page.click('#reflowCancel');
});

// The P06 phase-close review: every 409 was shown as "That paragraph has changed", the dialog closed, and what the user
// had typed was lost — while the server also answers 409 for a document frozen for a signing ceremony and for one past
// the size cap, each with its own sentence. The one response is intercepted, so this reads what the dialog does with it.
test('a refused reflow shows the server\'s own reason and keeps what the user typed', async () => {
  const reason = 'This document is part of a signing ceremony and cannot change until it ends.';
  await page.route('**/api/reflow', (route) => route.fulfill({ status: 409, contentType: 'application/json', body: JSON.stringify({ error: reason }) }));
  try {
    await openReflow();
    const typed = (await page.$eval('#reflowText', (t) => t.value)).replace('fox', 'cat');
    await page.fill('#reflowText', typed);
    await page.click('#reflowGo');
    // Either outcome ends the wait, so the old behaviour is reported by the assertions rather than as a timeout.
    await page.waitForFunction(() => !document.getElementById('reflowWhy').hidden || document.getElementById('reflowModal').hidden);
    assert.equal(await page.$eval('#reflowModal', (m) => m.hidden), false, 'the dialog closed on a 409');
    assert.equal(await page.$eval('#reflowWhy', (p) => p.textContent), reason, 'the server\'s reason was replaced');
    assert.equal(await page.$eval('#reflowText', (t) => t.value), typed, 'what the user typed was lost');
    assert.equal(await page.$eval('#reflowGo', (b) => b.disabled), false, 'Reflow stayed disabled after the answer');
  } finally {
    await page.unroute('**/api/reflow');
    if (!(await page.$eval('#reflowModal', (m) => m.hidden))) await page.click('#reflowCancel');
  }
});

// P06 exit clause 4 through the binary: "a signed document is refused at the server door (D11)". Signed by the app's own
// Finalize, saved, and opened again as the file a user would open — so the badge the gate reads is the server's verdict on
// real signed bytes, not a flag set by the test. Two doors are asked, separately: the UI must say why and send nothing,
// and the server must refuse a request the UI never made.
test('a signed document is refused: the dialog says why before any request, and the server refuses one sent anyway', async () => {
  const outDir = path.join(WORK, 'reflow-signed');
  fs.mkdirSync(outDir, { recursive: true });
  const out = path.join(outDir, 'signed.pdf');
  assert.match(await page.$eval('#sigBadge', (el) => el.textContent), /Unsigned/,
    'setup: the reflowed document must open unsigned, or the signature below is not this test\'s doing');
  await h.mode('secure');
  await h.group('Sign & Timestamp');
  await page.click('#finalizeBtn');
  await page.waitForFunction(() => !document.getElementById('finalizeModal').hidden);
  await page.click('#fzGo');
  await page.waitForFunction(() => !document.getElementById('saveAsModal').hidden, null, { timeout: 30000 });
  await page.fill('#saveAsName', 'signed.pdf');
  await page.fill('#saveAsDir', outDir);
  await page.click('#saveAsGo');
  await page.waitForFunction(() => document.getElementById('saveAsModal').hidden);
  const deadline = Date.now() + 15000;
  while (!fs.existsSync(out) && Date.now() < deadline) await page.waitForTimeout(200);
  assert.ok(fs.readFileSync(out).includes('/ByteRange'), 'setup: the finalized file is not signed');

  // Both documents are one page, so openDocument's page-count wait cannot tell them apart; the badge's transition can.
  await h.openDocument(out, 1);
  await page.waitForFunction(() => /Untampered/.test(document.getElementById('sigBadge').textContent), null, { timeout: 15000 });

  const asked = [];
  const onRequest = (r) => { if (/\/api\/(reflow|paragraphs)\b/.test(r.url())) asked.push(r.url()); };
  page.on('request', onRequest);
  await h.mode('markup');
  await h.group('Edit Page Text');
  assert.equal(await page.$eval('#reflowBtn', (b) => b.disabled), false,
    'setup: the button is disabled, so the click below never reaches the gate this test is about');
  await page.click('#reflowBtn');
  // Either outcome ends the wait, so a gate that fails is reported by the assertions below rather than as a timeout.
  await page.waitForFunction(() => /document is signed/.test(document.getElementById('toast')?.textContent ?? '')
    || !document.getElementById('reflowModal').hidden);
  await page.waitForTimeout(500); // a request the gate failed to stop would be in flight by now
  page.off('request', onRequest);
  const modalHidden = await page.$eval('#reflowModal', (m) => m.hidden);
  if (!modalHidden) await page.click('#reflowCancel');
  assert.deepEqual(asked, [], 'a signed document reached the server from the Reflow button');
  assert.equal(modalHidden, true, 'the dialog opened on a signed document');
  assert.match(await page.$eval('#toast', (t) => t.textContent), /This document is signed/, 'the user was not told why');

  // The server door, asked directly: no X-Nib-Doc, so it resolves the active document — the signed one just opened.
  const answer = await page.evaluate(async () => {
    const fd = new FormData();
    fd.append('page', '1'); fd.append('paragraph', '0');
    fd.append('original', 'The quick brown fox jumps over the sleepy old dog and runs away from here.');
    fd.append('text', 'Rewritten after signing.');
    const r = await nibFetch('/api/reflow', { method: 'POST', body: fd });
    return { status: r.status, body: await r.json() };
  });
  assert.equal(answer.status, 200, `the server answered ${answer.status}`);
  assert.equal(answer.body.ok, false, 'the server reflowed a signed document');
  assert.equal(answer.body.cause, 'signed', `the server refused for another reason: ${answer.body.cause}`);
});
