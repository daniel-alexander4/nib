// /pending 908 — the Reduce dialog's third mode, "Shrink pictures".
//
// What this tier reaches: the mode's controls show only for it, the request names the setting or the target (and a
// target wins), the result says how many pictures changed and why the rest did not, and a pass that could not meet a
// target or changed nothing says so and points at Compress. What it cannot: that the file is smaller — internal/pdfops.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const DOC = '/tmp/nib-harness/pics.pdf';
let facts = { images: 4, shrunk: 3, kept: 0, skipped: 1, level: 2, before: 5000000, after: 2000000, met: false, left: { graphic: 1 } };
let sizes = ['5000000', '2000000'];
const h = await boot({
  routes: {
    '/api/open': () => ({ id: 'test-epoch:1', name: 'pics.pdf', path: DOC, canSave: true, signature: { state: 'unsigned' }, canUndo: false, canRedo: false }),
    '/api/close': () => ({}),
    '/api/lastopen': () => [],
    '/api/shrink-pictures': () => new Response(new Uint8Array([37, 80, 68, 70]), {
      status: 200,
      headers: { 'X-Original-Size': sizes[0], 'X-New-Size': sizes[1], 'X-Nib-Shrink': JSON.stringify(facts) },
    }),
  },
});
const { document: doc, settle, calls } = h;
const $ = (id) => doc.getElementById(id);
const radio = (v) => $('reduceModal').querySelector(`input[value="${v}"]`);
async function choose(v) {
  radio(v).checked = true;
  radio(v).dispatchEvent(new h.window.Event('change', { bubbles: true }));
}
async function run() {
  const before = calls.length;
  $('reduceGo').click();
  await settle(); await settle();
  return calls.slice(before).find((c) => c.url.includes('/api/shrink-pictures'));
}
async function reopen() {
  $('reduceCancel').click();
  $('reduceBtn').click();
  await settle();
  await choose('pictures');
}

test('setup: a document is open, and the mode shows its own controls only', async () => {
  setNextDocument({ numPages: 1, outline: null });
  $('pathInput').value = DOC;
  $('openGo').click();
  await settle(); await settle();
  $('reduceBtn').click();
  await settle();
  assert.ok(radio('pictures'), 'the dialog has no Shrink pictures mode');
  assert.equal($('reducePictures').hidden, true);
  await choose('pictures');
  assert.equal($('reducePictures').hidden, false, 'the picture controls did not show');
  assert.equal($('reduceQuality').hidden, true, 'Compress\'s quality showed beside it');
  await choose('compress');
  assert.equal($('reducePictures').hidden, true);
  assert.equal($('reduceQuality').hidden, false);
  await choose('pictures');
});

test('the setting is sent as a level, the result says what changed and what was left, and a target wins', async () => {
  $('reducePicLevel').value = '4';
  const call = await run();
  assert.ok(call && /\?level=4$/.test(call.url), `the request was ${call && call.url}`);
  const text = $('reduceResult').textContent;
  assert.match(text, /3 of 4 pictures shrunk/);
  assert.match(text, /left as they are: 1 flat-colour graphics/);

  await reopen();
  $('reducePicTarget').value = '1.5';
  facts = { ...facts, met: true, left: {} };
  const t = await run();
  assert.ok(t && t.url.endsWith(`?target=${Math.round(1.5 * 1024 * 1024)}`), `a target of 1.5 MB was sent as ${t && t.url}`);
  assert.doesNotMatch($('reduceResult').textContent, /smallest shrinking pictures alone/);
});

test('a target that could not be met, and a pass that changed nothing, each say so and point at Compress', async () => {
  await reopen();
  $('reducePicTarget').value = '0.1';
  facts = { ...facts, met: false, shrunk: 3 };
  await run();
  assert.match($('reduceResult').textContent, /smallest shrinking pictures alone reaches/);
  assert.match($('reduceResult').textContent, /Compress goes further/);

  await reopen();
  facts = { images: 2, shrunk: 0, kept: 0, skipped: 2, level: 2, before: 900, after: 900, met: false, left: { graphic: 2 } };
  sizes = ['900', '900'];
  await run();
  assert.match($('reduceResult').textContent, /Nothing here would get smaller by shrinking pictures/);
});
