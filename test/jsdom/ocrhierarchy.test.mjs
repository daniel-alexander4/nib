// The OCR client sends tesseract's layout, not just its words — `PLAN-accessibility.md` P06.S06.
//
// ── What is at stake ─────────────────────────────────────────────────────────
// `pdfops.TagOCRLayer` builds a structure tree from `block`, `para` and `line`. With all three at
// zero — which is exactly what a client that stops sending them produces — every word becomes its
// own paragraph, and a scanned page is described to a screen reader as a column of disconnected
// words. That is a legitimate fallback for an OLD client and a silent regression from this one, and
// the two are indistinguishable at the server.
//
// ── Why a source scan, again ─────────────────────────────────────────────────
// `/pending 447`'s guard cannot see this: `fieldsRead` matches `FormValue`, `PostFormValue` and
// `Query().Get`, so its population is form fields and query parameters and `/api/ocr` is JSON.
// Probed — deleting the send below leaves that guard green. `/pending 477` carries the gap.
//
// Nothing drives the OCR client flow either (it needs a real wasm worker), so this is a scan in
// `keyboardplacement.test.mjs`'s shape and says so: it proves the send is still WRITTEN, not that a
// recognition produces it.
//
// One boot per file — see boot.mjs. This file needs none.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const APP = readFileSync(new URL('../../web/app.js', import.meta.url), 'utf8');

// The loop that turns a recognition into the POST payload, located by the request it feeds.
function ocrWordBlock() {
  const post = APP.indexOf("'/api/ocr'");
  assert.ok(post > 0, "the /api/ocr call is gone — this file's subject has moved and every " +
    'assertion below would be scanning unrelated source');
  // From the id helper, not from the `for` — the helper sits above the loop and is half of what
  // this file asserts. Anchoring on the loop alone silently excluded it, and two of the three
  // assertions below then scanned source that could not contain them.
  const start = APP.lastIndexOf('const idx = new Map()', post);
  assert.ok(start > 0 && start < post, 'the id helper that numbers tesseract\'s containers is no ' +
    'longer above the request it feeds');
  assert.ok(APP.indexOf('for (const word of data.words', start) < post,
    'the word loop is no longer between the id helper and the request');
  return APP.slice(start, post);
}

test('every word carries the block, paragraph and line tesseract put it in', () => {
  const block = ocrWordBlock();

  // Stimulus floor: a block that does not build the payload makes everything below vacuous.
  assert.match(block, /words\.push\(\{/,
    'the loop no longer pushes a word payload, so this scan is not looking at the OCR sender');

  for (const key of ['block', 'para', 'line']) {
    assert.match(block, new RegExp(`\\b${key}:`),
      `a word is sent without its \`${key}\`. With all three absent the server reads "no ` +
      'hierarchy" and describes every word as its own paragraph — a scanned page announced as a ' +
      'column of disconnected words, and indistinguishable at the server from an old client.');
  }
});

test('the ids come from the containers tesseract reports, not from the loop counter', () => {
  const block = ocrWordBlock();
  assert.match(block, /block: ord\(word\.block\)/,
    'the block id is not derived from `word.block`. tesseract hands each flattened word a ' +
    'back-pointer to the block it came from; anything else — an index, a counter — is a grouping ' +
    'nib invented rather than one the engine found.');
  assert.match(block, /para: ord\(word\.paragraph\)/,
    'the paragraph id is not derived from `word.paragraph`');
  assert.match(block, /line: ord\(word\.line\)/,
    'the line id is not derived from `word.line`');
});

test('an unrecognised container is sent as zero, which the server reads as no hierarchy', () => {
  const block = ocrWordBlock();
  assert.match(block, /if \(!o\) return 0;/,
    'a word whose container tesseract did not report no longer maps to 0. The server treats 0 as ' +
    '"nothing is known about what this word belongs with" and gives it its own element; any other ' +
    'value would group unrelated words under an id that means nothing.');
  assert.match(block, /idx\.set\(o, idx\.size \+ 1\)/,
    'the ids no longer start at 1. Zero is the wire\'s "no hierarchy" value, so a real container ' +
    'numbered 0 would be silently discarded.');
});
