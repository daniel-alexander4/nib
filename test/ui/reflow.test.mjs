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
function paragraphPDF(content = 'BT /F1 14 Tf 18 TL 72 700 Td (The quick brown fox jumps) Tj T* (over the lazy dog and runs) Tj T* (away from here.) Tj ET') {
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

// P07.S03 through the binary: an edit that needs more lines grows DOWN, and the paragraph below it is pushed lower — read
// from pdf.js's text layer of the re-rendered page, a reader the Go side has no hand in.
const GROW = writeRawFixture('reflow-grow.pdf', paragraphPDF(
  'BT /F1 14 Tf 18 TL 72 700 Td (The quick brown fox jumps) Tj T* (over the lazy dog and runs) Tj T* (away from here.) Tj ET ' +
  'BT /F1 14 Tf 72 640 Td (Second stays.) Tj ET'));

const spanTop = (word) => page.evaluate((w) => {
  const s = [...document.querySelectorAll('.viewerContainer:not([hidden]) .page .textLayer span')].find((e) => e.textContent.includes(w));
  return s ? s.getBoundingClientRect().top : null;
}, word);

test('an edit that needs more lines pushes the paragraph below it down', async () => {
  await h.openDocument(GROW, 1);
  await page.waitForFunction(() => /Second stays/.test([...document.querySelectorAll('.viewerContainer:not([hidden]) .page .textLayer span')].map((s) => s.textContent).join(' ')));
  const before = await spanTop('Second');
  assert.notEqual(before, null, 'setup: the second paragraph must be on the page before the edit');
  await openReflow();
  await page.fill('#reflowText', 'The quick brown fox jumps over the lazy dog and runs away from here, and then it runs some more.');
  await page.click('#reflowGo');
  await page.waitForFunction(() => document.getElementById('reflowModal').hidden || !document.getElementById('reflowWhy').hidden);
  assert.equal(await page.$eval('#reflowModal', (m) => m.hidden), true,
    `the growth was refused: ${await page.$eval('#reflowWhy', (p) => p.textContent)}`);
  await page.waitForFunction(() => /some more/.test([...document.querySelectorAll('.viewerContainer:not([hidden]) .page .textLayer span')].map((s) => s.textContent).join(' ')));
  const after = await spanTop('Second');
  assert.ok(after > before + 5, `the paragraph below did not move down: top ${before} → ${after}`);
  assert.match(await pageText(), /Second stays\./, 'the paragraph below lost its text');
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

// P07's exit criterion through the binary (S06): a growth on a full page flows its last paragraph onto page 2 — and the link
// laid over that paragraph goes with it, onto page 2, rather than staying behind on page 1 over other words. Read from
// pdf.js's text and annotation layers of the re-rendered pages.
function flowPDF() {
  const line = (p, k) => `Page ${p} paragraph ${String(k).padStart(2, '0')} words run on`;
  const pageContent = (p, n) => Array.from({ length: n }, (_, k) =>
    `BT /F1 12 Tf 14 TL 72 ${700 - 38 * k} Td (${line(p, k)}) Tj T* (${line(p, k)}) Tj ET`).join('\n');
  const c1 = pageContent(1, 16), c2 = pageContent(2, 3);
  const objs = [
    '<< /Type /Catalog /Pages 2 0 R >>',
    '<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>',
    '<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 7 0 R >> >> /Contents 5 0 R /Annots [8 0 R] >>',
    '<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 7 0 R >> >> /Contents 6 0 R >>',
    `<< /Length ${c1.length} >>\nstream\n${c1}\nendstream`,
    `<< /Length ${c2.length} >>\nstream\n${c2}\nendstream`,
    '<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>',
    '<< /Type /Annot /Subtype /Link /Rect [72 114 300 142] /Border [0 0 0] /A << /S /URI /URI (https://example.com/carried) >> >>',
  ];
  let out = '%PDF-1.7\n';
  const offs = [];
  objs.forEach((o, i) => { offs.push(out.length); out += `${i + 1} 0 obj\n${o}\nendobj\n`; });
  const xref = out.length;
  out += `xref\n0 ${objs.length + 1}\n0000000000 65535 f \n` + offs.map((o) => `${String(o).padStart(10, '0')} 00000 n \n`).join('');
  out += `trailer\n<< /Size ${objs.length + 1} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF\n`;
  return { pdf: Buffer.from(out, 'latin1'), first: `${line(1, 0)} ${line(1, 0)}`, leaving: line(1, 15) };
}
const FLOW = flowPDF();
const FLOWDOC = writeRawFixture('reflow-flow.pdf', FLOW.pdf);

const onPage = (n, what) => page.evaluate(([n, what]) => {
  const pg = document.querySelector(`.viewerContainer:not([hidden]) .page[data-page-number="${n}"]`);
  if (!pg) return null;
  pg.scrollIntoView();
  if (what === 'text') return [...pg.querySelectorAll('.textLayer span')].map((s) => s.textContent).join(' ');
  return [...pg.querySelectorAll('.annotationLayer a[href]')].map((a) => a.href);
}, [n, what]);

// rerendered waits until page 1's text layer shows the EDIT — the first line five times, where the page drew it twice — so
// a check that something is absent from page 1 reads the new layer, not an empty or stale one (P07 phase-close review).
const rerendered = (line) => page.waitForFunction((w) => {
  const pg = document.querySelector('.viewerContainer:not([hidden]) .page[data-page-number="1"]');
  pg?.scrollIntoView();
  const text = [...(pg?.querySelectorAll('.textLayer span') || [])].map((s) => s.textContent).join(' ');
  return text.split(w).length - 1 >= 5;
}, line, { timeout: 15000 });

test('a growth on a full page flows its last paragraph onto the next page, and the link on it goes too', async () => {
  await h.openDocument(FLOWDOC, 2);
  await page.waitForFunction((w) => [...document.querySelectorAll('.viewerContainer:not([hidden]) .page[data-page-number="1"] .textLayer span')]
    .some((s) => s.textContent.includes(w)), FLOW.leaving);
  await page.waitForFunction(() => document.querySelectorAll('.viewerContainer:not([hidden]) .page[data-page-number="1"] .annotationLayer a[href]').length === 1);
  assert.deepEqual(await onPage(1, 'links'), ['https://example.com/carried'], 'setup: the link must be on page 1 before the edit');
  await openReflow();
  assert.equal(await page.$eval('#reflowText', (t) => t.value), FLOW.first, 'the dialog did not offer page 1\'s first paragraph');
  const l = FLOW.first.split(' ').slice(0, 7).join(' ');
  await page.fill('#reflowText', `${FLOW.first} ${l} ${l} ${l}`);
  await page.click('#reflowGo');
  await page.waitForFunction(() => document.getElementById('reflowModal').hidden || !document.getElementById('reflowWhy').hidden);
  assert.equal(await page.$eval('#reflowModal', (m) => m.hidden), true,
    `the flow was refused: ${await page.$eval('#reflowWhy', (p) => p.textContent)}`);
  await page.waitForFunction((w) => [...document.querySelectorAll('.viewerContainer:not([hidden]) .page[data-page-number="2"] .textLayer span')]
    .some((s) => s.textContent.includes(w)) || (document.querySelector('.viewerContainer:not([hidden]) .page[data-page-number="2"]')?.scrollIntoView(), false), FLOW.leaving, { timeout: 15000 });
  assert.match(await onPage(2, 'text'), new RegExp(FLOW.leaving), 'the paragraph did not arrive on page 2');
  await rerendered(FLOW.first.split(' ').slice(0, 7).join(' '));
  assert.doesNotMatch(await onPage(1, 'text'), new RegExp(FLOW.leaving), 'the paragraph is still drawn on page 1');
  await page.waitForFunction(() => document.querySelectorAll('.viewerContainer:not([hidden]) .page[data-page-number="2"] .annotationLayer a[href]').length > 0, null, { timeout: 15000 });
  assert.deepEqual(await onPage(2, 'links'), ['https://example.com/carried'], 'the link did not go to page 2 with its paragraph');
  // Page 1's layers are the re-rendered ones: `rerendered` above waited for the edit to show there.
  assert.deepEqual(await onPage(1, 'links'), [], 'the link stayed behind on page 1, over other words');
});

// taggedFlowPDF is flowPDF tagged as real producers tag: each paragraph its own /P element owning one MCID, the link a
// /Link element holding its OBJR, a nested /ParentTree as Acrobat and Word write it — P07.S07.
function taggedFlowPDF() {
  const line = (p, k) => `Page ${p} paragraph ${String(k).padStart(2, '0')} words run on`;
  const pageContent = (p, n) => Array.from({ length: n }, (_, k) =>
    `/P <</MCID ${k}>> BDC BT /F1 12 Tf 14 TL 72 ${700 - 38 * k} Td (${line(p, k)}) Tj T* (${line(p, k)}) Tj ET EMC`).join('\n');
  const c1 = pageContent(1, 16), c2 = pageContent(2, 3);
  const objs = {
    1: '<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 9 0 R /MarkInfo << /Marked true >> >>',
    2: '<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>',
    3: '<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 7 0 R >> >> /Contents 5 0 R /Annots [8 0 R] /StructParents 0 >>',
    4: '<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 7 0 R >> >> /Contents 6 0 R /StructParents 1 >>',
    5: `<< /Length ${c1.length} >>\nstream\n${c1}\nendstream`,
    6: `<< /Length ${c2.length} >>\nstream\n${c2}\nendstream`,
    7: '<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>',
    8: '<< /Type /Annot /Subtype /Link /Rect [72 114 300 142] /Border [0 0 0] /StructParent 2 /A << /S /URI /URI (https://example.com/carried) >> >>',
    11: '<< /Kids [12 0 R 13 0 R] >>',
    14: '<< /Type /StructElem /S /Link /P 9 0 R /Pg 3 0 R /K [<< /Type /OBJR /Obj 8 0 R >>] >>',
  };
  const elems = [], row = [[], []];
  [16, 3].forEach((n, i) => { for (let k = 0; k < n; k++) { const o = 20 + 20 * i + k; objs[o] = `<< /Type /StructElem /S /P /P 9 0 R /Pg ${3 + i} 0 R /K ${k} >>`; elems.push(`${o} 0 R`); row[i].push(`${o} 0 R`); } });
  objs[9] = `<< /Type /StructTreeRoot /K [${elems.join(' ')} 14 0 R] /ParentTree 11 0 R /ParentTreeNextKey 3 >>`;
  objs[12] = `<< /Limits [0 1] /Nums [0 [${row[0].join(' ')}] 1 [${row[1].join(' ')}]] >>`;
  objs[13] = '<< /Limits [2 2] /Nums [2 14 0 R] >>';
  const max = Math.max(...Object.keys(objs).map(Number));
  let out = '%PDF-1.7\n';
  const offs = {};
  for (let n = 1; n <= max; n++) if (objs[n]) { offs[n] = out.length; out += `${n} 0 obj\n${objs[n]}\nendobj\n`; }
  const xref = out.length;
  out += `xref\n0 ${max + 1}\n0000000000 65535 f \n`;
  for (let n = 1; n <= max; n++) out += offs[n] !== undefined ? `${String(offs[n]).padStart(10, '0')} 00000 n \n` : '0000000000 65535 f \n';
  out += `trailer\n<< /Size ${max + 1} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF\n`;
  return { pdf: Buffer.from(out, 'latin1'), first: `${line(1, 0)} ${line(1, 0)}`, leaving: line(1, 15) };
}
const TFLOW = taggedFlowPDF();
const TFLOWDOC = writeRawFixture('reflow-flow-tagged.pdf', TFLOW.pdf);

test('a growth on a TAGGED page flows its paragraph onto the next page with its structure element', async () => {
  // The structure is asked of the server directly with no X-Nib-Doc, so it answers for the ACTIVE document — this one.
  await h.openDocument(TFLOWDOC, 2);
  await page.waitForFunction((w) => [...document.querySelectorAll('.viewerContainer:not([hidden]) .page[data-page-number="1"] .textLayer span')]
    .some((s) => s.textContent.includes(w)), TFLOW.leaving);
  const before = await page.evaluate(async (w) => {
    const t = await (await nibFetch('/api/tags/tree')).json();
    return { tagged: t.tagged, el: t.elements.find((e) => e.kind === 'P' && e.text.includes(w)),
      link: t.elements.find((e) => e.kind === 'Link') };
  }, TFLOW.leaving);
  assert.equal(before.tagged, true, 'setup: the document must read as tagged');
  assert.equal(before.el?.page, 1, 'setup: the leaving paragraph\'s element must be on page 1');
  assert.equal(before.link?.page, 1, 'setup: the link element must be on page 1');
  await openReflow();
  assert.equal(await page.$eval('#reflowText', (t) => t.value), TFLOW.first, 'the dialog did not offer page 1\'s first paragraph');
  const l = TFLOW.first.split(' ').slice(0, 7).join(' ');
  await page.fill('#reflowText', `${TFLOW.first} ${l} ${l} ${l}`);
  await page.click('#reflowGo');
  await page.waitForFunction(() => document.getElementById('reflowModal').hidden || !document.getElementById('reflowWhy').hidden);
  assert.equal(await page.$eval('#reflowModal', (m) => m.hidden), true,
    `the flow was refused: ${await page.$eval('#reflowWhy', (p) => p.textContent)}`);
  await page.waitForFunction((w) => [...document.querySelectorAll('.viewerContainer:not([hidden]) .page[data-page-number="2"] .textLayer span')]
    .some((s) => s.textContent.includes(w)) || (document.querySelector('.viewerContainer:not([hidden]) .page[data-page-number="2"]')?.scrollIntoView(), false), TFLOW.leaving, { timeout: 15000 });
  await rerendered(TFLOW.first.split(' ').slice(0, 7).join(' '));
  assert.doesNotMatch(await onPage(1, 'text'), new RegExp(TFLOW.leaving), 'the paragraph is still drawn on page 1');
  const after = await page.evaluate(async (w) => {
    const t = await (await nibFetch('/api/tags/tree')).json();
    return { el: t.elements.filter((e) => e.kind === 'P' && e.text.includes(w)),
      link: t.elements.find((e) => e.kind === 'Link') };
  }, TFLOW.leaving);
  assert.equal(after.el.length, 1, `one element must own the carried text, got ${JSON.stringify(after.el)}`);
  assert.equal(after.el[0].page, 2, 'the carried paragraph\'s element is not on page 2');
  assert.equal(after.el[0].text, `${TFLOW.leaving}${TFLOW.leaving}`, 'the element does not read its text on page 2');
  assert.equal(after.link?.page, 2, 'the link\'s structure element stayed on page 1');
});

// P08.S01 through the binary: a paragraph whose runs differ in letter and word spacing — a word drawn across two `Tc`s,
// a line under `Tw` and `Tz` — was refused as mixed-state; now each glyph keeps its spacing and the edit is re-set.
const SPACED = writeRawFixture('reflow-spaced.pdf', paragraphPDF(
  'BT /F1 14 Tf 18 TL 72 700 Td 0.5 Tc (The qu) Tj 1.5 Tc (ick brown fox jumps) Tj T* 0 Tc 2 Tw 110 Tz (over the lazy dog and runs) Tj T* (away from here.) Tj ET'));

test('a paragraph set with mixed letter and word spacing is re-set, not refused', async () => {
  await h.openDocument(SPACED, 1);
  await page.waitForFunction(() => /lazy/.test([...document.querySelectorAll('.viewerContainer:not([hidden]) .page .textLayer span')].map((s) => s.textContent).join(' ')));
  await openReflow();
  assert.equal(await page.$eval('#reflowText', (t) => t.value), 'The quick brown fox jumps over the lazy dog and runs away from here.',
    'the dialog did not offer the mixed-spacing paragraph as one');
  assert.equal(await page.$eval('#reflowGo', (b) => b.disabled), false,
    `the dialog refused the mixed-spacing paragraph before anything was typed: ${await page.$eval('#reflowWhy', (p) => p.textContent)}`);
  await page.fill('#reflowText', 'The quick brown fox jumps over the tame dog and runs away from here.');
  await page.click('#reflowGo');
  await page.waitForFunction(() => document.getElementById('reflowModal').hidden || !document.getElementById('reflowWhy').hidden);
  assert.equal(await page.$eval('#reflowModal', (m) => m.hidden), true,
    `the mixed-spacing paragraph was refused: ${await page.$eval('#reflowWhy', (p) => p.textContent)}`);
  await page.waitForFunction(() => /tame/.test([...document.querySelectorAll('.viewerContainer:not([hidden]) .page .textLayer span')].map((s) => s.textContent).join(' ')));
  const after = await pageText();
  assert.match(after, /tame dog/, `the re-rendered page reads: ${after}`);
  assert.doesNotMatch(after, /lazy/, `"lazy" is still on the page: ${after}`);
});

// P08.S02 through the binary: a producer that shows each space as its own `( ) Tj` was refused as mixed-content; its
// spaces are the paragraph's and go with it.
const SPACESHOWN = writeRawFixture('reflow-spaceshown.pdf', paragraphPDF(
  'BT /F1 14 Tf 18 TL 72 700 Td (The) Tj ( ) Tj (quick) Tj ( ) Tj (brown) Tj ( ) Tj (fox) Tj ( ) Tj (jumps) Tj T* (over) Tj ( ) Tj (the) Tj ( ) Tj (lazy) Tj ( ) Tj (dog) Tj ( ) Tj (and) Tj ( ) Tj (runs) Tj T* (away) Tj ( ) Tj (from) Tj ( ) Tj (here.) Tj ( ) Tj ET'));

test('a paragraph whose spaces are each their own show is re-set, not refused', async () => {
  await h.openDocument(SPACESHOWN, 1);
  await page.waitForFunction(() => /lazy/.test([...document.querySelectorAll('.viewerContainer:not([hidden]) .page .textLayer span')].map((s) => s.textContent).join(' ')));
  await openReflow();
  assert.equal(await page.$eval('#reflowGo', (b) => b.disabled), false,
    `the dialog refused the paragraph before anything was typed: ${await page.$eval('#reflowWhy', (p) => p.textContent)}`);
  await page.fill('#reflowText', 'The quick brown fox jumps over the tame dog and runs away from here.');
  await page.click('#reflowGo');
  await page.waitForFunction(() => document.getElementById('reflowModal').hidden || !document.getElementById('reflowWhy').hidden);
  assert.equal(await page.$eval('#reflowModal', (m) => m.hidden), true,
    `the paragraph was refused: ${await page.$eval('#reflowWhy', (p) => p.textContent)}`);
  await page.waitForFunction(() => /tame/.test([...document.querySelectorAll('.viewerContainer:not([hidden]) .page .textLayer span')].map((s) => s.textContent).join(' ')));
  const after = await pageText();
  assert.match(after, /tame dog/, `the re-rendered page reads: ${after}`);
  assert.doesNotMatch(after, /lazy/, `"lazy" is still on the page: ${after}`);
});

// P08.S03 through the binary: a justified paragraph (Tw per line, InDesign's way) re-wraps justified — read from pdf.js's
// text layer of the re-rendered page, every line but the last ends at one right edge.
const JUSTIFIED = writeRawFixture('reflow-justified.pdf', paragraphPDF(
  // Tw computed from Helvetica's widths so the first three lines end at 330.896 (PLAN P08.S03's tier-3 case).
  'BT /F1 14 Tf 18 TL 72 700 Td 1.3051428571428605 Tw (The quick brown fox jumps over the lazy) Tj T* 1.2347500000000053 Tw (dog and then runs far away from here to) Tj T* 0.6666666666666666 Tw (the river where it rests in the shade of an) Tj T* 0 Tw (old tree.) Tj 0 Tw ET'));

const lineRights = () => page.evaluate(() => {
  const rows = new Map();
  for (const s of document.querySelectorAll('.viewerContainer:not([hidden]) .page .textLayer span')) {
    if (!s.textContent.trim()) continue;
    const r = s.getBoundingClientRect();
    const key = Math.round(r.top);
    rows.set(key, Math.max(rows.get(key) ?? -Infinity, r.right));
  }
  return [...rows.entries()].sort((a, b) => a[0] - b[0]).map((e) => e[1]);
});

test('a justified paragraph re-wraps justified', async () => {
  await h.openDocument(JUSTIFIED, 1);
  await page.waitForFunction(() => /lazy/.test([...document.querySelectorAll('.viewerContainer:not([hidden]) .page .textLayer span')].map((s) => s.textContent).join(' ')));
  const before = await lineRights();
  assert.ok(before.length === 4 && Math.abs(before[0] - before[1]) < 1.5 && Math.abs(before[1] - before[2]) < 1.5,
    `setup: the paragraph is not set flush — line ends ${before}`);
  await openReflow();
  await page.fill('#reflowText', 'The quick fox jumps over the lazy dog and then runs far away from here to the river where it rests in the shade of an old tree.');
  await page.click('#reflowGo');
  await page.waitForFunction(() => document.getElementById('reflowModal').hidden || !document.getElementById('reflowWhy').hidden);
  assert.equal(await page.$eval('#reflowModal', (m) => m.hidden), true,
    `the justified paragraph was refused: ${await page.$eval('#reflowWhy', (p) => p.textContent)}`);
  // Wait for the NEW text to be drawn, not the old to be gone: between the two the layer is empty.
  await page.waitForFunction(() => /quick fox/.test([...document.querySelectorAll('.viewerContainer:not([hidden]) .page .textLayer span')].map((s) => s.textContent).join(' ')));
  const after = await lineRights();
  assert.ok(after.length >= 3, `the re-set paragraph reads ${after.length} lines`);
  for (let i = 0; i < after.length - 1; i++) {
    assert.ok(Math.abs(after[i] - before[0]) < 1.5, `line ${i} ends at ${after[i]}, the flush edge is ${before[0]} — ends ${after}`);
  }
});

// P08.S04 through the binary: a heading centred on the page stays centred when its length changes — read from pdf.js's
// text layer, its middle before and after. (Helvetica 18: "Annual Report" is 115.056 wide, centred on 306.)
const CENTRED = writeRawFixture('reflow-centred.pdf', paragraphPDF(
  'BT /F1 18 Tf 1 0 0 1 248.472 720 Tm (Annual Report) Tj ET ' +
  'BT /F1 14 Tf 18 TL 72 680 Td (The body of the page runs from the left margin across the column,) Tj T* (and wraps here as a body paragraph does.) Tj ET'));

// The middle of the row a word is on: pdf.js may draw a line as several spans, so the row is every span at its top.
const rowMiddle = (word) => page.evaluate((w) => {
  const spans = [...document.querySelectorAll('.viewerContainer:not([hidden]) .page .textLayer span')].filter((s) => s.textContent.trim());
  const hit = spans.find((e) => e.textContent.includes(w));
  if (!hit) return null;
  const top = hit.getBoundingClientRect().top;
  const row = spans.map((s) => s.getBoundingClientRect()).filter((r) => Math.abs(r.top - top) < 2);
  return (Math.min(...row.map((r) => r.left)) + Math.max(...row.map((r) => r.right))) / 2;
}, word);

test('a centred heading stays centred when its length changes', async () => {
  // This file opens a ninth document here, past the cap of eight (ADR-005): close the last one first.
  await h.closeDocument();
  await h.openDocument(CENTRED, 1);
  await page.waitForFunction(() => /Annual/.test([...document.querySelectorAll('.viewerContainer:not([hidden]) .page .textLayer span')].map((s) => s.textContent).join(' ')));
  const before = await rowMiddle('Annual');
  assert.notEqual(before, null, 'setup: the heading is not on the page');
  await openReflow();
  await page.fill('#reflowText', 'The Annual Report for the Year');
  await page.click('#reflowGo');
  await page.waitForFunction(() => document.getElementById('reflowModal').hidden || !document.getElementById('reflowWhy').hidden);
  assert.equal(await page.$eval('#reflowModal', (m) => m.hidden), true,
    `the heading was refused: ${await page.$eval('#reflowWhy', (p) => p.textContent)}`);
  await page.waitForFunction(() => /Year/.test([...document.querySelectorAll('.viewerContainer:not([hidden]) .page .textLayer span')].map((s) => s.textContent).join(' ')));
  const after = await rowMiddle('Year');
  assert.ok(Math.abs(after - before) < 1.5, `the heading's middle moved from ${before} to ${after}`);
});

// P08.S05 through the binary: a typed word takes the kern the page draws for its pairs. The page kerns by table — "AV"
// tightened by 140 three times, and ten more pairs below the paragraph, each three times (a face lends only past a table
// of five) — and draws "VA" unkerned and "AA" never; two typed words of the same four glyphs, "AVAV" (two AV pairs) and
// "VAAV" (one), differ in width by one kern, read from pdf.js's text layer. (In the bytes, unkerned, they are the same width;
// pdf.js's own arithmetic reads them 0.80 pt apart — see the threshold below.)
// (A positive `TJ` number tightens: it moves the next glyph left.)
const TABLE = ['To', 'Yo', 'Wa', 'Va', 'Ye', 'Te', 'Pa', 'Ly', 'Fa', 'Ta'];
const KERNED = writeRawFixture('reflow-kerned.pdf', paragraphPDF(
  'BT /F1 14 Tf 18 TL 72 700 Td [(The C) (A) 140 (VE and the S) (A) 140 (VE are near the old) ] TJ T* [(A) 140 (VAIL road.) ] TJ ET ' +
  'BT /F1 14 Tf 18 TL 72 400 Td ' + [0, 4, 8].map((i) => '[' + TABLE.slice(i, i + 4).map((p) => `(${p[0]}) 40 (${p[1]} ) `.repeat(3)).join('') + '] TJ').join(' T* ') + ' ET'));

const rowWidth = (word) => page.evaluate((w) => {
  const spans = [...document.querySelectorAll('.viewerContainer:not([hidden]) .page .textLayer span')].filter((s) => s.textContent.trim());
  const hit = spans.find((e) => e.textContent.includes(w));
  if (!hit) return null;
  const top = hit.getBoundingClientRect().top;
  const row = spans.map((s) => s.getBoundingClientRect()).filter((r) => Math.abs(r.top - top) < 2);
  const rowSpans = spans.filter((s) => Math.abs(s.getBoundingClientRect().top - top) < 2);
  return { left: Math.min(...row.map((r) => r.left)), right: Math.max(...row.map((r) => r.right)),
    spans: rowSpans.map((s) => { const r = s.getBoundingClientRect(); return `${JSON.stringify(s.textContent)} ${r.left.toFixed(2)}-${r.right.toFixed(2)}`; }) };
}, word);

async function typedRow(word) {
  await h.closeDocument();
  await h.openDocument(KERNED, 1);
  await page.waitForFunction(() => /AVAIL/.test([...document.querySelectorAll('.viewerContainer:not([hidden]) .page .textLayer span')].map((s) => s.textContent).join(' ')));
  await openReflow();
  assert.equal(await page.$eval('#reflowText', (t) => t.value), 'The CAVE and the SAVE are near the old AVAIL road.',
    'setup: the dialog did not offer the kerned paragraph as one');
  await page.fill('#reflowText', `The CAVE and the SAVE are near the old AVAIL road. ${word}`);
  await page.click('#reflowGo');
  await page.waitForFunction(() => document.getElementById('reflowModal').hidden || !document.getElementById('reflowWhy').hidden);
  assert.equal(await page.$eval('#reflowModal', (m) => m.hidden), true,
    `the kerned paragraph was refused: ${await page.$eval('#reflowWhy', (p) => p.textContent)}`);
  await page.waitForFunction((w) => new RegExp(w).test([...document.querySelectorAll('.viewerContainer:not([hidden]) .page .textLayer span')].map((s) => s.textContent).join(' ')), word);
  return { after: await rowWidth(word) };
}

test('a typed word takes the kerning the page draws for its pairs', async () => {
  const two = await typedRow('AVAV');
  const one = await typedRow('VAAV');
  // px per point, from the page itself: its MediaBox is 612 points wide.
  const scale = await page.evaluate(() => document.querySelector('.viewerContainer:not([hidden]) .page').getBoundingClientRect().width / 612);
  assert.ok(Math.abs(two.after.left - one.after.left) < 0.5, `setup: the two rows start at ${two.after.left} and ${one.after.left}`);
  const kern = (one.after.right - two.after.right) / scale;
  // The Go read-back of these bytes is exact (1.96 pt, `TestATypedWordTakesThePagesKerning`); pdf.js's text layer measures
  // its own way: it reads 2.73 here, and 0.80 with the lending switched off (both measured 2026-10-01 — the two words draw
  // the same glyphs, but pdf.js's own arithmetic differs by their order). So this asserts the kern by a threshold between
  // the two, not its last digit; a kern written the wrong way round would read below the unkerned 0.80.
  assert.ok(kern > 1.4 && kern < 3.5, `"AVAV" is ${kern} pt narrower than "VAAV", want one AV kern (1.96 pt drawn) — scale ${scale}, rows ${JSON.stringify(two.after.spans)} / ${JSON.stringify(one.after.spans)}`);
});

// P08.S07 through the binary: a word the producer hyphenated across a line end ("accom-" / "modate") rejoins when the
// edit moves it mid-line — the document writes "accommodate" whole below, so the hyphen goes. Read from pdf.js's text
// layer: the word is on the page twice (the evidence and the paragraph), and "accom-" nowhere.
const BROKEN = writeRawFixture('reflow-broken.pdf', paragraphPDF(
  'BT /F1 12 Tf 14 TL 72 700 Td 17.159 Tw (We will do all that we can to accom-) Tj T* 12.065 Tw (modate the needs of each and every one of) Tj T* 0 Tw (our clients this year.) Tj ET ' +
  'BT /F1 12 Tf 72 500 Td (It is hard to accommodate them all here.) Tj ET'));

test('a hyphenated word rejoins when the edit moves it mid-line', async () => {
  await h.closeDocument();
  await h.openDocument(BROKEN, 1);
  await page.waitForFunction(() => /accom-/.test([...document.querySelectorAll('.viewerContainer:not([hidden]) .page .textLayer span')].map((s) => s.textContent).join(' ')));
  await openReflow();
  const offered = await page.$eval('#reflowText', (t) => t.value);
  assert.equal(offered, 'We will do all that we can to accom- modate the needs of each and every one of our clients this year.',
    'setup: the dialog did not offer the broken paragraph as one');
  await page.fill('#reflowText', offered.replace('We will ', ''));
  await page.click('#reflowGo');
  await page.waitForFunction(() => document.getElementById('reflowModal').hidden || !document.getElementById('reflowWhy').hidden);
  assert.equal(await page.$eval('#reflowModal', (m) => m.hidden), true,
    `the paragraph was refused: ${await page.$eval('#reflowWhy', (p) => p.textContent)}`);
  // Wait for the NEW text, not the old to be gone (between the two the layer is empty), white space collapsed: pdf.js
  // draws each word of a stretched line as its own span.
  await page.waitForFunction(() => {
    const t = [...document.querySelectorAll('.viewerContainer:not([hidden]) .page .textLayer span')].map((s) => s.textContent).join(' ').replace(/\s+/g, ' ');
    return /^Do all|\bdo all/.test(t) && !/We will/.test(t);
  });
  const after = (await pageText()).replace(/\s+/g, ' ');
  assert.doesNotMatch(after, /accom-/, `the fragment is still on the page: ${after}`);
  assert.equal((after.match(/accommodate/g) || []).length, 2, `the word is not rejoined in the paragraph: ${after}`);
});
