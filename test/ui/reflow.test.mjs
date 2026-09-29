import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch, shutdown } from './harness.mjs';
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
