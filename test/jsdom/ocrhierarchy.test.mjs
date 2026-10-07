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
// One boot per file — see boot.mjs. The scans need none; the driven flow at the foot of the file has the one.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { boot } from './boot.mjs';
import * as pdfjs from './stub-pdfjs.mjs';

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

// ── The flow, driven — ADR-101 (reading a page again) and ADR-094 (asking first) ─────────────────
// The recogniser is the one thing this tier cannot run, so it is the one thing stubbed: `window.Tesseract`
// hands back a fixed word for whatever page it is given, and jsdom's missing canvas is given the two methods
// `renderPageBlob` calls. Everything else is the shipped `runOCR`: which pages it reads, what it asks the user,
// and the request it sends. What stays out of reach is a real recognition and whether the server does what the
// request asks — internal/server/ocr_test.go and internal/pdfops/ocrreplace_test.go.
const DOC_ID = 'test-epoch:3';
let layers = { layered: [], own: [] };
let posted = null;
let facts = {};
const h = await boot({
  routes: {
    '/api/open': {
      id: DOC_ID, name: 'scan.pdf', path: '/tmp/nib-harness/scan.pdf', canSave: true,
      signature: { state: 'unsigned' }, canUndo: false, canRedo: false,
    },
    '/api/ocr/pages': () => layers,
    '/api/ocr': (opts) => {
      posted = JSON.parse(opts.body);
      return new Response(JSON.stringify({ id: DOC_ID, name: 'scan.pdf', path: '/tmp/nib-harness/scan.pdf', canSave: true,
        signature: { state: 'unsigned' }, canUndo: true, canRedo: false }),
      { status: 200, headers: { 'Content-Type': 'application/json', 'X-Nib-OCR': JSON.stringify(facts) } });
    },
  },
});
const recognised = [];
h.window.HTMLCanvasElement.prototype.getContext = () => ({ fillRect() {} });
h.window.HTMLCanvasElement.prototype.toBlob = function toBlob(cb) { cb({ page: this.width }); };
h.window.Tesseract = {
  createWorker: async () => ({
    setParameters: async () => {},
    recognize: async () => {
      recognised.push(recognised.length);
      const para = {};
      return { data: { words: [{ text: 'word', bbox: { x0: 10, y0: 10, x1: 60, y1: 30 }, block: para, paragraph: para, line: para }] } };
    },
    terminate: async () => {},
  }),
};

// run presses OCR on a three-page document whose layers the server describes as `told`, with the user
// answering the question (if one is asked) with `answer`. It returns the pages read, the question and the request.
let opened = false;
async function run(told, answer, serverFacts = {}) {
  if (!opened) {
    pdfjs.setNextDocument({ numPages: 3, renders: true });
    h.document.getElementById('pathInput').value = '/tmp/nib-harness/scan.pdf';
    h.document.getElementById('openGo').click();
    await h.settle();
    assert.equal(h.document.getElementById('viewerWrap').className, 'has-doc', 'the document never opened');
    opened = true;
  }
  layers = told; facts = serverFacts; posted = null;
  h.confirms.length = 0;
  h.setConfirmAnswer(answer);
  h.document.getElementById('ocrBtn').click();
  await h.settle(40);
  const toastEl = h.document.getElementById('toast');
  return { posted, asked: [...h.confirms], said: toastEl ? toastEl.textContent : '' };
}
const pagesOf = (body) => [...new Set(body.words.map((w) => w.page))].sort();

test('a page with a text layer is not read, and nothing is asked when none of the layers is Nib\'s own', async () => {
  const got = await run({ layered: [2], own: [] }, true);
  assert.deepEqual(got.asked, [], 'the user was asked about a layer Nib cannot replace');
  assert.ok(got.posted, 'the OCR request never went out — nothing below would mean anything');
  assert.deepEqual(pagesOf(got.posted), [1, 3], 'the layered page was read, or an unlayered one was not');
  assert.equal(got.posted.replace, undefined, 'a plain OCR asked the server to replace a layer');
  // The recognition's layout reaches the wire — what the scans above can only show is written.
  assert.deepEqual([got.posted.words[0].block, got.posted.words[0].para, got.posted.words[0].line], [1, 1, 1]);
});

test('the user is offered to read Nib\'s own layered pages again, and saying yes sends them with replace', async () => {
  const got = await run({ layered: [1, 2], own: [2] }, true, { replaced: [2] });
  assert.equal(got.asked.length, 1, 'the offer to read the page again was not made exactly once');
  assert.match(got.asked[0], /already has a text layer that Nib added\. Read it again\?/);
  assert.ok(got.posted, 'the OCR request never went out');
  assert.deepEqual(pagesOf(got.posted), [2, 3], 'page 2 (Nib\'s own layer, read again) and page 3 (no layer) are what is read; page 1 is another program\'s');
  assert.equal(got.posted.replace, true, 'the pages were read again and the server was not asked to replace their layer — it would skip them');
  assert.match(got.said, /Read 1 page again and replaced its text layer\. 1 page already had a text layer and was left as it is/);
});

test('saying no leaves every layered page unread and asks the server for nothing new', async () => {
  const got = await run({ layered: [1, 2], own: [2] }, false);
  assert.equal(got.asked.length, 1, 'the offer was not made');
  assert.ok(got.posted, 'the OCR request never went out');
  assert.deepEqual(pagesOf(got.posted), [3], 'a layered page was read after the user declined');
  assert.equal(got.posted.replace, undefined, 'the request asks to replace a layer the user chose to keep');
});

test('a page the server would not replace because its tags were changed is said so, in those words', async () => {
  const got = await run({ layered: [1, 2], own: [1, 2] }, true, { replaced: [1], skipped: [2], causes: { 2: 'structure' } });
  assert.ok(got.posted && got.posted.replace === true, 'the replace never went out');
  assert.match(got.said, /Read 1 page again and replaced its text layer\. 1 page already had a text layer and was left as it is\. 1 page was not read again: the tags on its text layer have been changed/);
  // And a page left for the other cause gets no such sentence.
  const other = await run({ layered: [1, 2], own: [1, 2] }, true, { replaced: [1], skipped: [2], causes: { 2: 'not-nibs-layer' } });
  assert.doesNotMatch(other.said, /tags/);
});

test('with every page layered and the offer declined, nothing is read and nothing is sent', async () => {
  const before = recognised.length;
  const got = await run({ layered: [1, 2, 3], own: [1, 2, 3] }, false);
  assert.equal(got.asked.length, 1);
  assert.match(got.asked[0], /3 pages already have a text layer that Nib added\. Read them again\?/);
  assert.equal(got.posted, null, 'a request went out with nothing to add');
  assert.equal(recognised.length, before, 'a page was recognised although every page keeps its layer');
  assert.match(got.said, /Every page already has a text layer/);
});
