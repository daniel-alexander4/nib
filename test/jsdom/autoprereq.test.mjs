// A command runs its own prerequisite — ADR-106.
//
// ── What is at stake ─────────────────────────────────────────────────────────
// Five commands used to stop on a scanned page and tell the user to run OCR first; one told them to run
// Detect; one told them to edit a profile; three said "Open a PDF first" and four dialogs said "pin someone
// first". The app can do or open each of those, and now does. What must NOT happen is the other half, and
// it is most of this file: a read of a page that has text, of a blank page, of a page that already has a
// layer; a layer replaced; a signed document read without asking, a locked one read at all; a command
// carrying on against a document that is no longer in front; and — the redaction search — a scan passed
// over in silence.
//
// ── What is stubbed, and what that leaves out ────────────────────────────────
// The recogniser, as in ocrhierarchy.test.mjs: `window.Tesseract` hands back a fixed word for whatever it
// is given. The server: `/api/ocr/pages` answers what the test says it does, and `/api/ocr` records the
// request and makes the reloaded document carry the words. Everything between is the shipped `ensureText`,
// `runOCR` and the commands. A REAL read, and the server's own answer about which pages are scans, are
// tier 3's (test/ui/autoprereq.test.mjs) and tier 1's (internal/server/ocr_test.go,
// internal/pdfops/ocrreplace_test.go).
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import * as pdfjs from './stub-pdfjs.mjs';

const meta = (id, name, extra = {}) => ({
  id, name, path: '/tmp/nib-harness/' + name, canSave: true,
  signature: { state: 'unsigned' }, canUndo: false, canRedo: false, ...extra,
});

let nextOpen = meta('test-epoch:1', 'scan.pdf');
let told = { layered: [], own: [], unread: [] }; // what the server says about the open document's pages
let toldStatus = 200;
let asked = 0; // how many times the door asked the server
let posted = null; // the last OCR request
let ocrStatus = 200;
let postGate = null;
let afterRead = null; // the document the reload after a read loads
let paragraphs = [];
let paragraphsNow = null; // a page that has paragraphs without being read first
const reflows = []; // the paragraph each reflow request named
let reflowGate = null;
let reflowStatus = 200, reflowError = '';
let reflowDone = false; // a reflow that is applied, so the document is loaded back
const sent = []; // each sanitize, page operation and save the server was sent
let docsNow = null;
let redactAnswer = { status: 200, body: {} };
let profile = {};
let peers = [];
const h = await boot({
  routes: {
    '/api/open': () => nextOpen,
    '/api/ocr/pages': () => {
      asked++;
      return toldStatus === 200 ? told : new Response('{"cause":"document-unreadable"}', { status: toldStatus });
    },
    '/api/ocr': async (opts) => {
      posted = JSON.parse(opts.body);
      if (postGate) await postGate; // the words are with the server and its answer has not come back
      if (ocrStatus !== 200) return new Response('{"error":"no"}', { status: ocrStatus });
      if (afterRead) pdfjs.setNextDocument(afterRead);
      return new Response(JSON.stringify({ ...nextOpen, canUndo: true }), { status: 200, headers: { 'Content-Type': 'application/json', 'X-Nib-OCR': '{}' } });
    },
    '/api/pagemap': () => new Response('{}', { status: 404 }),
    // The page has paragraphs once it has been read.
    '/api/paragraphs': () => ({ paragraphs: paragraphsNow || (posted ? paragraphs : []) }),
    // /pending 792: a reflow held open, so the dialog can be worked while its answer is on the way.
    '/api/reflow': async (opts) => {
      reflows.push(opts.body.get('paragraph'));
      if (reflowGate) await reflowGate;
      if (reflowStatus !== 200) return new Response(JSON.stringify({ error: reflowError }), { status: reflowStatus, headers: { 'Content-Type': 'application/json' } });
      return reflowDone ? { ...nextOpen, ok: true, canUndo: true } : { ...nextOpen, ok: false, cause: 'page-full' };
    },
    // /pending 791: the operations that load the document back, counted.
    '/api/sanitize': () => { sent.push('sanitize'); return { ...nextOpen, ok: true, canUndo: true, residual: { findings: [] } }; },
    '/api/pages': () => { sent.push('pages'); return { ...nextOpen, canUndo: true }; },
    '/api/save': () => { sent.push('save'); return nextOpen; },
    // Every 409 makes the window reconcile its tabs with the server; a test that answers one says what is open.
    '/api/docs': () => docsNow || { docs: [], activeId: '' },
    // /pending 834: the flatten route refusing in its own words.
    '/api/redact': () => new Response(JSON.stringify(redactAnswer.body), { status: redactAnswer.status, headers: { 'Content-Type': 'application/json' } }),
    '/api/profile': (opts) => {
      if (opts.method === 'POST') { profile = JSON.parse(opts.body); return {}; }
      return profile;
    },
    '/api/peers': () => ({ fingerprint: 'aa', name: 'me', peers }),
    '/api/identity/external': () => ({}),
    '/api/switch': () => ({}),
    '/api/close-view': () => nextOpen,
    '/api/listdir': { path: '/home/someone/nib', parent: '/home/someone', dirs: [], files: [] },
  },
});
const { document: doc, window: win, settle } = h;
const $ = (id) => doc.getElementById(id);
const toastText = () => ($('toast') || {}).textContent || '';
const note = () => { const n = $('readingNote'); return n && !n.hidden ? n.firstChild.textContent : ''; };

// The recogniser. `gate`, when set, holds a recognition open until the test releases it — the window in
// which the user stops the read or moves to another document. `seenWhileReading` is what the page said
// while the read was under way.
const recognised = [];
let words = ['word'];
let gate = null;
let seenWhileReading = '';
// jsdom has no canvas: one that accepts every call and draws nothing is enough for a quick stamp's bitmap and
// for the page picture handed to the recogniser, neither of which is what is under test here.
win.HTMLCanvasElement.prototype.getContext = () => new Proxy({}, { get: () => () => ({ width: 8 }), set: () => true });
win.HTMLCanvasElement.prototype.toDataURL = () => 'data:image/png;base64,';
win.HTMLCanvasElement.prototype.toBlob = function toBlob(cb) { cb({ page: this.width }); };
// The stub's viewport has no transform of its own; a run's own matrix is where the search places it.
pdfjs.Util.transform = (a, b) => a || b;
// jsdom has no CSS.escape; Autofill uses it to find a field's element.
globalThis.CSS = { escape: (s) => String(s) };
let made = 0, letGo = 0; // recognisers made, and let go
win.Tesseract = {
  createWorker: async () => (made++, {
    setParameters: async () => {},
    recognize: async () => {
      recognised.push(recognised.length);
      seenWhileReading = note() || $('rtStatus').textContent;
      if (gate) await gate;
      const para = {};
      return { data: { words: words.map((text) => ({ text, bbox: { x0: 10, y0: 10, x1: 60, y1: 30 }, block: para, paragraph: para, line: para })) } };
    },
    terminate: async () => { letGo++; },
  }),
};

const spoken = [];
const fakeSpeech = { speak(u) { spoken.push(u.text); }, cancel() {} };
win.speechSynthesis = fakeSpeech;
globalThis.speechSynthesis = fakeSpeech;
win.SpeechSynthesisUtterance = class { constructor(text) { this.text = text; } };
globalThis.SpeechSynthesisUtterance = win.SpeechSynthesisUtterance;

// open opens a document whose pages carry `text` (one string a page; '' is a page with none) and whose
// scans the server lists as `unread`. After a read the reloaded document carries `then`.
async function open(name, text, unread, then = null, extra = {}) {
  nextOpen = meta('test-epoch:' + name, name, extra);
  pdfjs.setNextDocument({ numPages: text.length, renders: true, text });
  $('pathInput').value = '/tmp/nib-harness/' + name;
  $('openGo').click();
  await settle(30);
  told = { layered: [], own: [], unread };
  toldStatus = 200; ocrStatus = 200;
  afterRead = then && { numPages: then.length, renders: true, text: then };
  reset();
}
function reset() {
  posted = null; asked = 0; recognised.length = 0; spoken.length = 0; seenWhileReading = '';
  h.confirms.length = 0; h.setConfirmAnswer(true);
  words = ['word']; gate = null;
  if ($('toast')) $('toast').textContent = '';
}
const pagesOf = (body) => [...new Set(body.words.map((w) => w.page))].sort();
// The page the commands act on, set through the page-number field — the control a user uses.
function onPage(n) {
  const field = doc.querySelector('.pageNum');
  field.value = String(n);
  field.dispatchEvent(new win.Event('change'));
}

// ── P7 — no document open ────────────────────────────────────────────────────
test('with no document open, a quick stamp and Read aloud say so and open the Open dialog', async () => {
  for (const press of [() => doc.querySelector('.quickstamps button').click(), () => $('readAloudBtn').click()]) {
    $('openModal').hidden = true;
    if ($('toast')) $('toast').textContent = '';
    press();
    await settle();
    assert.match(toastText(), /Open a PDF first/, 'the command did not say what is missing');
    assert.equal($('openModal').hidden, false, 'the Open dialog was not opened: the user is told to open a PDF and left to find how');
  }
  $('openCancel').click();
});

// ── P1 / P3 — a per-page command reads ONE page, and only a scan ─────────────
test('Read aloud on a scanned page reads that page and no other, then reads it aloud', async () => {
  await open('a.pdf', ['typed', '', '', 'layered'], [2, 3], ['typed', 'now read', '', 'layered']);
  // Page 4 carries a layer Nib added — the page the OCR BUTTON would offer to read again. A command never does.
  told = { layered: [4], own: [4], unread: [2, 3] };
  onPage(2);
  $('readAloudBtn').click();
  await settle(60);
  assert.ok(posted, 'the scanned page was not read: nothing was sent to /api/ocr');
  assert.deepEqual(pagesOf(posted), [2], 'the read was not of the one page the command is about');
  assert.equal(recognised.length, 1, 'more than the one page was recognised — page 3 is a scan too and was not asked for');
  assert.equal(posted.replace, undefined, 'a command\'s read asked the server to replace a text layer');
  assert.deepEqual(h.confirms, [], 'the user was asked a question — an unsigned document asks none, and replacing a layer is never a command\'s to offer');
  assert.equal(seenWhileReading, 'Reading this page first…', 'nothing on the page said a read was under way');
  assert.equal(note(), '', 'the reading note is still up after the read ended');
  assert.deepEqual(spoken, ['now read'], 'the page was read (OCR) and then not read aloud');
});

test('a page with text is read aloud with no read of the scan, and the server is not even asked', async () => {
  await open('b.pdf', ['typed', ''], [2]);
  onPage(1);
  $('readAloudBtn').click();
  await settle(30);
  assert.deepEqual(spoken, ['typed']);
  assert.equal(asked, 0, 'a page that has text asked which pages are scans');
  assert.equal(recognised.length, 0, 'a page that has text was recognised');
  $('readAloudBtn').click(); // stop
});

test('a blank page and a page that already has a layer are not read, and the answer no longer blames a scan', async () => {
  await open('c.pdf', ['', ''], []); // the server calls neither page unread: page 1 is blank
  onPage(1);
  $('readAloudBtn').click();
  await settle(30);
  assert.equal(asked, 1, 'setup: the door never asked the server about the page');
  assert.equal(recognised.length, 0, 'a blank page was recognised');
  assert.equal(toastText(), 'This page has no text to read.');
  // A page the server lists as BOTH unread and layered is layered: the door does not put a second layer on it.
  told = { layered: [2], own: [2], unread: [2] };
  onPage(2); reset();
  $('readAloudBtn').click();
  await settle(30);
  assert.equal(recognised.length, 0, 'a page that already has a text layer was read again');
  assert.equal(posted, null);
  assert.deepEqual(h.confirms, [], 'the door asked ADR-101\'s question — replacing a layer is the OCR button\'s to offer');
});

test('a read that finds no words answers plainly, once, and the next press does not read the page again', async () => {
  await open('d.pdf', [''], [1]);
  words = [];
  $('readAloudBtn').click();
  await settle(60);
  assert.equal(recognised.length, 1, 'setup: the scan was never read');
  assert.equal(posted, null, 'a read that found nothing sent a request');
  assert.equal(toastText(), 'This page has no text to read.');
  $('readAloudBtn').click();
  await settle(60);
  assert.equal(recognised.length, 1, 'the same page was read again on the next press, to find the same nothing');
  assert.equal(toastText(), 'This page has no text to read.');
});

test('a failed read is said once, by the door, and the command adds no sentence of its own', async () => {
  await open('e.pdf', [''], [1]);
  ocrStatus = 500;
  $('readAloudBtn').click();
  await settle(60);
  assert.ok(posted, 'setup: the read never reached the server');
  assert.equal(toastText(), 'This page is a scan and could not be read: Could not add the text layer.');
  assert.deepEqual(spoken, []);
});

// ── P4 — the refusals that stay ──────────────────────────────────────────────
test('a signed document is asked about first, and "no" reads nothing and says so', async () => {
  await open('signed.pdf', [''], [1], ['now read'], { signature: { state: 'valid' } });
  h.setConfirmAnswer(false);
  $('readAloudBtn').click();
  await settle(60);
  assert.equal(h.confirms.length, 1, 'a signed document was not asked about before its scan was read');
  assert.match(h.confirms[0], /This document is signed\. Editing it destroys the existing signature/);
  assert.equal(recognised.length, 0, 'the page was read after the user said no');
  assert.equal(toastText(), 'This page is a scan and was not read, so the signature stands.');
  // And "yes" reads it.
  reset();
  $('readAloudBtn').click();
  await settle(60);
  assert.equal(h.confirms.length, 1);
  assert.deepEqual(pagesOf(posted), [1], 'the user said yes and the page was not read');
});

test('a document locked for signing is refused with a sentence, and nothing is asked or read', async () => {
  await open('locked.pdf', [''], [1], null, { flags: [{}] });
  $('readAloudBtn').click();
  await settle(60);
  assert.equal(recognised.length, 0, 'a document locked for signing had a page read — a read is an edit');
  assert.equal(posted, null);
  assert.deepEqual(h.confirms, []);
  assert.equal(toastText(), 'This page is a scan and cannot be read here: this document is locked for signing, and reading a scan changes it.');
});

test('stopping the read sends nothing and runs nothing', async () => {
  await open('f.pdf', [''], [1], ['now read']);
  let release;
  gate = new Promise((r) => { release = r; });
  $('readAloudBtn').click();
  await settle(40);
  assert.equal(recognised.length, 1, 'setup: the read is not under way');
  assert.equal(note(), 'Reading this page first…');
  $('readingStop').click();
  await settle(40);
  release();
  await settle(40);
  assert.equal(posted, null, 'a stopped read still stamped its words on the document');
  assert.deepEqual(spoken, [], 'the command ran after its read was stopped');
  assert.equal(toastText(), 'Stopped reading — nothing was done.');
  assert.equal(note(), '');
});

test('moving to another document during the read stops it: nothing is sent and the command does not run', async () => {
  await open('g.pdf', [''], [1], ['now read']);
  let release;
  gate = new Promise((r) => { release = r; });
  $('readAloudBtn').click();
  await settle(40);
  assert.equal(recognised.length, 1, 'setup: the read is not under way');
  // Another document comes to the front while the page is being recognised.
  const tabs = [...doc.querySelectorAll('#tabstrip .tab')];
  assert.ok(tabs.length > 1, 'setup: there is no other document to move to');
  tabs[0].click();
  await settle(20);
  release();
  await settle(60);
  assert.equal(posted, null, 'the read went on to stamp a document the command will not run on');
  assert.deepEqual(spoken, [], 'the command ran against a document that is no longer in front');
  assert.equal(toastText(), 'The document is no longer in front, so reading stopped — nothing was done.');
});

test('moving away once the words are already on their way: the page is read, the command is not run, and that is what is said', async () => {
  await open('g2.pdf', [''], [1], ['now read']);
  let release;
  postGate = new Promise((r) => { release = r; });
  $('readAloudBtn').click();
  await settle(40);
  assert.ok(posted, 'setup: the words have not been sent');
  [...doc.querySelectorAll('#tabstrip .tab')][0].click();
  await settle(20);
  postGate = null;
  release();
  await settle(60);
  assert.deepEqual(spoken, []);
  assert.equal(toastText(), 'The page was read, but the document is no longer in front — nothing else was done.',
    'the read finished behind another document and nothing said what had and had not happened');
});

test('Escape stops a read; so does pressing Read aloud again; and a second command during a read is told to wait', async () => {
  await open('g3.pdf', [''], [1], ['now read']);
  let release;
  gate = new Promise((r) => { release = r; });
  $('readAloudBtn').click();
  await settle(40);
  assert.equal(recognised.length, 1, 'setup: the read is not under way');
  // Another command that needs this page's text, pressed while it is being read.
  $('exportTableBtn').click();
  await settle(30);
  assert.equal(toastText(), 'Nib is already reading a scan — try again when it has finished.');
  assert.equal(recognised.length, 1, 'a second read was started beside the first');
  doc.dispatchEvent(new win.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  await settle(40);
  release();
  await settle(40);
  assert.equal(posted, null, 'Escape did not stop the read');
  assert.equal(toastText(), 'Stopped reading — nothing was done.');
  // The button again, while the page is being read for it.
  reset();
  gate = new Promise((r) => { release = r; });
  $('readAloudBtn').click();
  await settle(40);
  assert.equal(recognised.length, 1, 'setup: the second read is not under way');
  $('readAloudBtn').click();
  await settle(40);
  release();
  await settle(40);
  assert.equal(posted, null, 'pressing Read aloud again did not stop the read it was waiting on');
  assert.equal(recognised.length, 1, 'pressing Read aloud again started a second read');
  assert.deepEqual(spoken, []);
});

// ── The recogniser between reads ─────────────────────────────────────────────
test('the recogniser is kept from one read to the next, and let go when stopped at work and when a document closes', async () => {
  await open('w.pdf', ['', ''], [1, 2], ['now', '']);
  const m0 = made;
  onPage(1);
  $('readAloudBtn').click();
  await settle(60);
  assert.deepEqual(pagesOf(posted), [1], 'setup: the first page was not read');
  $('readAloudBtn').click(); // stop speaking
  told = { layered: [1], own: [1], unread: [2] };
  afterRead = { numPages: 2, renders: true, text: ['now', 'read'] };
  posted = null;
  onPage(2);
  $('readAloudBtn').click();
  await settle(60);
  assert.deepEqual(pagesOf(posted), [2], 'setup: the second page was not read');
  assert.equal(made - m0, 1, 'the second read made a recogniser of its own instead of using the one kept from the first');
  $('readAloudBtn').click();
  // Stopped at work: that recogniser is busy with a page nobody wants, and is let go rather than kept.
  await open('w2.pdf', [''], [1], ['now read']);
  const g0 = letGo;
  let release;
  gate = new Promise((r) => { release = r; });
  $('readAloudBtn').click();
  await settle(40);
  $('readingStop').click();
  await settle(40);
  release();
  await settle(40);
  assert.equal(letGo - g0, 1, 'a recogniser stopped in the middle of a page was kept for the next read');
  // And the next read makes a new one; closing its document lets that one go.
  reset();
  const m1 = made;
  $('readAloudBtn').click();
  await settle(60);
  assert.equal(made - m1, 1, 'setup: the read after a stop did not make a recogniser');
  $('readAloudBtn').click();
  const g1 = letGo;
  const tabs = [...doc.querySelectorAll('#tabstrip .tab')];
  h.setConfirmAnswer(true);
  tabs[tabs.length - 1].querySelector('.tabclose').click();
  await settle(40);
  assert.equal(letGo - g1, 1, 'closing a document left the kept recogniser running');
});

// ── P1 — the other two per-page commands ─────────────────────────────────────
test('This page\'s table… reads the scanned page it is on, then offers the table', async () => {
  await open('h.pdf', ['', ''], [1, 2], ['', 'cell']);
  onPage(2);
  $('saveAsModal').hidden = true;
  $('exportTableBtn').click();
  await settle(60);
  assert.deepEqual(pagesOf(posted), [2], 'the table export did not read exactly its own page');
  assert.equal($('saveAsModal').hidden, false, 'the page was read and the table was not then offered');
  $('saveAsCancel').click();
  // A page with no text that is not a scan: one plain sentence.
  await open('h2.pdf', [''], []);
  $('exportTableBtn').click();
  await settle(40);
  assert.equal(recognised.length, 0);
  assert.equal(toastText(), 'No text on this page to extract');
});

test('Reflow paragraph reads the scanned page, then lists its paragraphs; a signed document is refused before any read', async () => {
  await open('i.pdf', [''], [1], ['a paragraph']);
  paragraphs = [{ index: 0, text: 'a paragraph' }];
  $('reflowBtn').click();
  await settle(80);
  assert.ok(posted, 'the scanned page was not read');
  assert.deepEqual(pagesOf(posted), [1]);
  assert.equal($('reflowModal').hidden, false, 'the page was read and its paragraphs were not then listed');
  $('reflowCancel').click();
  paragraphs = [];
  // Signed: the refusal that was always first is still first — the door is not reached.
  await open('i2.pdf', [''], [1], null, { signature: { state: 'valid' } });
  $('reflowBtn').click();
  await settle(40);
  assert.equal(asked, 0, 'Reflow asked about reading a signed document it was always going to refuse');
  assert.deepEqual(h.confirms, []);
  assert.match(toastText(), /signed/i);
});

// ── P2 — the whole-document commands ─────────────────────────────────────────
async function search(term) {
  $('redactTextBtn').click();
  $('rtTerm').value = term;
  $('rtFind').click();
  await settle(80);
}

test('Redact text… reads the scanned pages, then finds the word the read added — in one press', async () => {
  await open('j.pdf', ['typed', '', ''], [2, 3], ['typed', 'secret', '']);
  words = ['secret'];
  await search('secret');
  assert.deepEqual(pagesOf(posted), [2, 3], 'the search did not read exactly the scanned pages');
  assert.equal(seenWhileReading, 'Reading 2 scanned pages first…', 'the dialog did not say a read was under way');
  assert.equal($('redactTextModal').hidden, true, 'the search found nothing after the read: the dialog is still up');
  assert.match(toastText(), /^1 match\(es\) marked on 1 page\(s\)/);
  assert.doesNotMatch(toastText(), /NOT searched/);
});

test('a search with no match starts no read, and says only that nothing was found', async () => {
  await open('k.pdf', ['typed', 'more'], []);
  await search('absent');
  assert.equal(asked, 1, 'setup: the search never asked which pages are scans');
  assert.equal(recognised.length, 0, 'a search that found nothing started a read');
  assert.equal($('rtStatus').textContent, 'No matches found.');
  $('rtCancel').click();
});

test('a search over scans that were NOT read names the pages it did not search — refused, failed, and not known', async () => {
  // Refused: signed, and the user says no.
  await open('l.pdf', ['typed', '', ''], [2, 3], null, { signature: { state: 'valid' } });
  h.setConfirmAnswer(false);
  await search('typed');
  assert.equal(recognised.length, 0);
  assert.match(toastText(), /^1 match\(es\) marked on 1 page\(s\).* NOT searched: pages 2 and 3 — scans that were not read\. Check them by eye\./,
    'a search that marked matches did not say two scanned pages were never searched');
  // Refused, and nothing found: the dialog says it.
  reset(); h.setConfirmAnswer(false);
  await search('absent');
  assert.equal($('rtStatus').textContent, 'No matches found. NOT searched: pages 2 and 3 — scans that were not read. Check them by eye.');
  $('rtCancel').click();
  // Failed.
  await open('l2.pdf', ['typed', ''], [2]);
  ocrStatus = 500;
  await search('absent');
  assert.equal($('rtStatus').textContent, 'No matches found. NOT searched: page 2 — a scan that was not read. Check it by eye.');
  $('rtCancel').click();
  // Not known: the server could not say which pages are scans.
  await open('l3.pdf', ['typed', ''], [2]);
  toldStatus = 422;
  await search('absent');
  assert.equal(recognised.length, 0);
  assert.match($('rtStatus').textContent, /^No matches found\. Nib could not tell whether any page is an unread scan/);
  $('rtCancel').click();
});

test('Cancel in the search dialog stops the read: nothing is sent and nothing is marked', async () => {
  await open('m.pdf', ['typed', ''], [2], ['typed', 'typed']);
  let release;
  gate = new Promise((r) => { release = r; });
  $('redactTextBtn').click();
  $('rtTerm').value = 'typed';
  $('rtFind').click();
  await settle(40);
  assert.equal(recognised.length, 1, 'setup: the read is not under way');
  assert.equal($('rtFind').disabled, true, 'Find can be pressed again while the pages are being read — that search would run without them');
  $('rtCancel').click();
  await settle(20);
  release();
  await settle(60);
  assert.equal(posted, null, 'a cancelled search still stamped the words it had read');
  assert.doesNotMatch(toastText(), /match\(es\) marked/, 'a cancelled search marked matches');
});

test('Document text reads the scanned pages first; with no text at all it says so and saves nothing', async () => {
  await open('n.pdf', ['typed', ''], [2], ['typed', 'now read']);
  $('saveAsModal').hidden = true;
  $('exportTextBtn').click();
  await settle(80);
  assert.ok(posted, 'the text export did not read the scanned page: nothing was sent to /api/ocr');
  assert.deepEqual(pagesOf(posted), [2], 'the text export did not read the scanned page');
  assert.equal($('saveAsModal').hidden, false, 'the export was not offered after the read');
  $('saveAsCancel').click();
  await open('n2.pdf', ['', ''], []);
  $('exportTextBtn').click();
  await settle(40);
  assert.equal(recognised.length, 0);
  assert.equal($('saveAsModal').hidden, true, 'an empty text file was offered for a document with no text');
  assert.equal(toastText(), 'This document has no text to export');
  // Stopped: nothing is exported — not even the text the document already had.
  await open('n3.pdf', ['typed', ''], [2], ['typed', 'now read']);
  let release;
  gate = new Promise((r) => { release = r; });
  $('exportTextBtn').click();
  await settle(40);
  assert.equal(seenWhileReading, 'Reading 1 scanned page first…');
  $('readingStop').click();
  await settle(20);
  release();
  await settle(40);
  assert.equal($('saveAsModal').hidden, true, 'the export went ahead after its read was stopped');
  // Refused (signed, and the user says no): the text there is is still exported, and the door has said what was left out.
  await open('n4.pdf', ['typed', ''], [2], null, { signature: { state: 'valid' } });
  h.setConfirmAnswer(false);
  $('exportTextBtn').click();
  await settle(40);
  assert.equal(toastText(), 'Page 2 is a scan and was not read, so the signature stands.');
  assert.equal($('saveAsModal').hidden, false, 'a signed document\'s own text could not be exported because one page is a scan');
  $('saveAsCancel').click();
});

// ── P5 — Save as fillable form… runs Detect ──────────────────────────────────
test('Save as fillable form… with no fields runs Detect fields rather than telling the user to', async () => {
  await open('o.pdf', ['typed'], []);
  $('saveFillableBtn').click();
  await settle(40);
  // jsdom lays nothing out, so Detect gets as far as finding the page is not on screen and says so. That it
  // SAID so is the observable that it ran; what it finds on a real page is tier 3's.
  assert.equal(toastText(), 'Scroll the page into view, then try again', 'Detect fields did not run as the command\'s first step');
  assert.equal($('fieldNameModal').hidden, true, 'the naming dialog opened with no fields');
});

// ── P6 — Autofill opens the profile, and saving it fills ─────────────────────
test('Autofill with no profile opens the editor; saving fills the form the press was about; cancelling fills nothing', async () => {
  await open('p.pdf', ['typed'], []);
  const stored = [];
  pdfjs.lastDocument.getFieldObjects = async () => ({ fullName: [{ id: 'f1' }] });
  pdfjs.lastDocument.annotationStorage.setValue = (id, v) => stored.push([id, v.value]);
  profile = {};
  $('autofillBtn').click();
  await settle(30);
  assert.equal($('profileModal').hidden, false, 'the profile editor was not opened');
  assert.match(toastText(), /^No profile yet — add your details and Save/);
  // Cancel: nothing runs, and a later, unrelated Save of the profile fills nothing either.
  $('profileCancel').click();
  $('editProfileBtn').click();
  await settle(20);
  $('profileText').value = 'fullName = Jane Doe';
  $('profileSave').click();
  await settle(30);
  assert.deepEqual(stored, [], 'saving the profile from its own button filled a form nobody asked to fill');
  // From Autofill: the save runs the fill.
  profile = {};
  $('autofillBtn').click();
  await settle(30);
  $('profileText').value = 'fullName = Jane Doe';
  $('profileSave').click();
  await settle(40);
  assert.deepEqual(stored, [['f1', 'Jane Doe']], 'the profile was saved from Autofill and the form was not filled');
  assert.equal(toastText(), 'Filled 1 field(s) — review and Save');
  // Saved empty: said once, and the editor is not opened again.
  stored.length = 0; profile = {};
  $('autofillBtn').click();
  await settle(30);
  $('profileText').value = '';
  $('profileSave').click();
  await settle(40);
  assert.equal(toastText(), 'The profile is empty — nothing was filled');
  assert.equal($('profileModal').hidden, true, 'an empty profile reopened the editor: a loop');
});

// ── P8 — nobody pinned ───────────────────────────────────────────────────────
test('with nobody pinned, the co-sign hint opens Identity & peers, and closing it re-reads who is pinned', async () => {
  peers = [];
  $('cosignBtn').disabled = false;
  $('cosignBtn').click();
  await settle(20);
  assert.equal($('cosignNoPeers').hidden, false, 'setup: the hint is not showing');
  assert.equal($('cosignGo').disabled, true);
  const open = $('cosignNoPeers').querySelector('button');
  assert.ok(open, 'the hint names Identity & peers and gives no way to get there');
  open.click();
  await settle(20);
  assert.equal($('peersModal').hidden, false, 'Identity & peers did not open');
  assert.equal($('cosignModal').hidden, false, 'the co-sign dialog was closed to open it');
  peers = [{ fingerprint: 'bb'.repeat(32), name: 'Them' }];
  $('peersClose').click();
  await settle(20);
  assert.equal($('cosignNoPeers').hidden, true, 'someone is pinned and the hint still says nobody is');
  assert.equal($('cosignGo').disabled, false, 'someone is pinned and Go is still disabled');
  $('cosignCancel').click();
  // Opened from its own menu entry, closing it reopens nothing.
  $('managePeersBtn').click();
  await settle(10);
  $('peersClose').click();
  await settle(20);
  assert.equal($('cosignModal').hidden, true, 'closing Identity & peers opened the co-sign dialog');
});

test('every dialog that needs a pinned peer carries the button', () => {
  for (const id of ['cosignNoPeers', 'sinNoPeers', 'ssnNoPeers', 'srvNoPeers']) {
    const b = $(id).querySelector('button');
    assert.ok(b && typeof b.onclick === 'function', id + ' has no working button to Identity & peers');
    assert.doesNotMatch(b.id || '', /(Cancel|Close)$/, 'Escape would click it as the dialog\'s dismissal');
  }
});

// ── /pending 792 — one reflow at a time ──────────────────────────────────────
// The button was disabled for the request, and picking another paragraph enabled it again: a second press then sent
// a second edit built from the same page, and the later commit erased the earlier.
test('while a reflow is on its way, another paragraph cannot be picked and a second press sends nothing', async () => {
  await open('rf.pdf', ['one two'], []);
  paragraphsNow = [{ index: 0, text: 'first paragraph' }, { index: 1, text: 'second paragraph' }];
  $('reflowBtn').click();
  await settle(40);
  assert.equal($('reflowModal').hidden, false, 'setup: the reflow dialog did not open');
  reflows.length = 0;
  let release;
  reflowGate = new Promise((r) => { release = r; });
  $('reflowText').value = 'first paragraph, edited';
  $('reflowGo').click();
  await settle(20);
  assert.deepEqual(reflows, ['0'], 'setup: the reflow was not sent');
  assert.equal($('reflowPick').disabled, true, 'the paragraph picker can be used while the edit is on its way');
  // What a change of paragraph did, for a build that leaves the picker enabled.
  $('reflowPick').value = '1';
  $('reflowPick').onchange();
  assert.equal($('reflowGo').disabled, true, 'picking another paragraph re-enabled Reflow while the first edit is on its way');
  $('reflowGo').onclick();
  await settle(20);
  assert.deepEqual(reflows, ['0'], 'a second edit was sent before the first was answered — both are built from the same page, and the later commit erases the earlier');
  release();
  await settle(40);
  reflowGate = null;
  assert.equal($('reflowPick').disabled, false, 'the picker stayed held after the answer');
  assert.equal($('reflowGo').disabled, false, 'Reflow stayed disabled after the answer');
  assert.match($('reflowWhy').textContent, /more room/, 'the refusal the server answered was not shown');
  // W2: signed while the dialog was open. The commit door's 409 asks the user to confirm, which this dialog cannot do.
  docsNow = { docs: [nextOpen], activeId: nextOpen.id };
  reflowStatus = 409;
  reflowError = 'this document carries a signature, and this operation would rebuild it in a way that leaves no record it was ever signed — Confirm that you want that, and Nib will do it';
  $('reflowGo').click();
  await settle(40);
  assert.doesNotMatch($('reflowWhy').textContent, /Confirm/, 'the dialog asks the user to confirm something it has no control for');
  assert.match($('reflowWhy').textContent, /^This document is signed/, 'a document signed meanwhile was not said as the signed refusal');
  // Any other 409 is still the server's own sentence.
  reflowError = 'that paragraph has changed since it was read — read the page again';
  $('reflowGo').click();
  await settle(40);
  assert.equal($('reflowWhy').textContent, reflowError);
  reflowStatus = 200;
  docsNow = null;
  $('reflowCancel').click();
  paragraphsNow = null;
});

// ── /pending 834 — a refused redaction says why ──────────────────────────────
test('a redaction the server refuses is told in the server\'s words; a declined signature question is not repeated back', async () => {
  await open('rd.pdf', ['a secret here'], []);
  await search('secret');
  assert.match(toastText(), /^1 match\(es\) marked/, 'setup: nothing is marked, so Apply has nothing to send');
  // The flatten builds a real form: jsdom's FormData takes jsdom's Blob only, for the document and for each page picture.
  const nodeBlob = globalThis.Blob, stubToBlob = win.HTMLCanvasElement.prototype.toBlob;
  globalThis.Blob = win.Blob;
  win.HTMLCanvasElement.prototype.toBlob = (cb) => cb(new win.Blob(['png']));
  redactAnswer = { status: 422, body: { error: 'page 9 is not in this document, which has 1 page', cause: 'page-not-in-document' } };
  $('applyRedactBtn').click();
  await settle(80);
  assert.equal(toastText(), 'page 9 is not in this document, which has 1 page');
  // The user has just said No to losing the signature: the sentence that asks them to confirm is not the answer.
  redactAnswer = { status: 409, body: { error: 'this document carries a signature, and this operation would rebuild it in a way that leaves no record it was ever signed — Confirm that you want that, and Nib will do it' } };
  h.setConfirmAnswer(true);
  const answers = [true, false]; // yes to redacting, no to losing the signature
  const was = globalThis.confirm;
  globalThis.confirm = () => answers.shift();
  $('applyRedactBtn').click();
  await settle(80);
  globalThis.confirm = was;
  assert.equal(toastText(), 'redaction failed');
  redactAnswer = { status: 200, body: {} };
  globalThis.Blob = nodeBlob;
  win.HTMLCanvasElement.prototype.toBlob = stubToBlob;
});

// ── /pending 791 — an operation that loads the document back asks before it discards an edit ──
// Reflow, the OCR read, the removals, both unlocks and attaching a file act on the SERVER's copy and load
// it back: what was typed into the form, placed on the pages or marked for redaction went with the old
// document, and nothing was said. `confirmOverlayLoss` is the one door; the routing — that every reload is
// behind it — is pinning.test.mjs's. Here: that it asks, that No sends nothing and keeps the edit, that it
// says what is at stake and how to keep it, and that it stays silent when nothing is.
//
// Cannot be driven here: an item PLACED on a page (it needs layout). A typed form value and a redaction
// box are the two kinds jsdom can make; the placed kind is counted by the same function and is tier 3's.
const typeIntoForm = () => pdfjs.lastDocument.annotationStorage.set('field-1', { value: 'typed' }); // pdf.js tells the app, as for a real fill

test('a reflow asks before it discards what was typed into the form: No sends nothing and keeps it, Yes sends it', async () => {
  await open('ov.pdf', ['one two'], []);
  paragraphsNow = [{ index: 0, text: 'first paragraph' }];
  const typedIn = pdfjs.lastDocument;
  typeIntoForm();
  $('reflowBtn').click();
  await settle(40);
  assert.equal($('reflowModal').hidden, false, 'setup: the reflow dialog did not open');
  reflows.length = 0; reflowDone = true;
  $('reflowText').value = 'first paragraph, edited';
  h.confirms.length = 0; h.setConfirmAnswer(false);
  $('reflowGo').click();
  await settle(40);
  assert.equal(h.confirms.length, 1, 'the reflow was sent with a typed form value unsaved, and nothing was asked: the value goes with the document that is replaced');
  assert.match(h.confirms[0], /what you typed into the form/, 'the question does not say what would be lost');
  assert.match(h.confirms[0], /choose Cancel, then save the document first/, 'the question does not say how to keep it');
  assert.deepEqual(reflows, [], 'the user said No and the reflow was sent');
  assert.equal(pdfjs.lastDocument, typedIn, 'the user said No and the document was loaded again');
  assert.equal(typedIn.annotationStorage.size, 1);
  assert.equal($('reflowModal').hidden, false, 'a No closed the dialog: what the user typed there is gone');
  assert.equal($('reflowText').value, 'first paragraph, edited');
  h.setConfirmAnswer(true);
  $('reflowGo').click();
  await settle(40);
  assert.deepEqual(reflows, ['0'], 'the user said Yes and the reflow was not sent');
  assert.notEqual(pdfjs.lastDocument, typedIn, 'setup: an applied reflow did not load the document back');
  reflowDone = false; paragraphsNow = null;
});

test('with nothing unsaved the removal asks nothing; after a Save the typed values are the document\'s and it still asks nothing', async () => {
  await open('ov2.pdf', ['one two'], []);
  sent.length = 0;
  $('scanStripBtn').click();
  await settle(40);
  assert.deepEqual(h.confirms, [], 'an unedited document was asked about edits it does not have');
  assert.deepEqual(sent, ['sanitize']);
  // Typed, then saved: Save posts the values and keeps the pdf.js document, so its storage still holds them.
  typeIntoForm();
  $('saveBtn').click();
  await settle(40);
  assert.deepEqual(sent, ['sanitize', 'save'], 'setup: the save was not sent');
  assert.equal(pdfjs.lastDocument.annotationStorage.size, 1, 'setup: the storage was emptied by the save, so this proves nothing');
  $('scanStripBtn').click();
  await settle(40);
  assert.deepEqual(h.confirms, [], 'saved values were called edits that would be discarded');
  assert.deepEqual(sent, ['sanitize', 'save', 'sanitize']);
});

test('a redaction box not yet applied is asked about by a removal, and by a page operation — which is asked about nothing else', async () => {
  await open('ov3.pdf', ['a secret here'], []);
  await search('secret');
  assert.match(toastText(), /^1 match\(es\) marked/, 'setup: nothing is marked');
  typeIntoForm();
  sent.length = 0; h.confirms.length = 0; h.setConfirmAnswer(false);
  $('scanStripBtn').click();
  await settle(40);
  assert.equal(h.confirms.length, 1, 'a removal with a redaction box pending asked nothing');
  assert.match(h.confirms[0], /1 redaction box not applied yet/);
  assert.match(h.confirms[0], /what you typed into the form/);
  assert.match(h.confirms[0], /then save the document and apply the redactions first/);
  assert.deepEqual(sent, [], 'the user said No and the removal was sent');
  // A page operation sends the baked document: what is typed is in it, and only the box is lost.
  h.confirms.length = 0;
  $('rotateRightBtn').click();
  await settle(40);
  assert.equal(h.confirms.length, 1, 'a page operation with a redaction box pending asked nothing');
  assert.match(h.confirms[0], /1 redaction box not applied yet/);
  assert.doesNotMatch(h.confirms[0], /typed|placed/, 'a page operation bakes what is typed and placed, and said it would be discarded');
  assert.match(h.confirms[0], /choose Cancel, then apply the redactions first/);
  assert.deepEqual(sent, [], 'the user said No and the page operation was sent');
  // The box is still there to apply: Apply does not say there is nothing to redact.
  h.confirms.length = 0;
  $('applyRedactBtn').click();
  await settle(20);
  assert.match(h.confirms[0] || '', /^Permanently redact/, 'the redaction box did not survive the two refusals');
});

test('a command that would read a scan first asks before the read discards an edit, and No reads nothing and says so', async () => {
  await open('ov4.pdf', [''], [1], ['now read']);
  typeIntoForm();
  h.confirms.length = 0; h.setConfirmAnswer(false);
  $('readAloudBtn').click();
  await settle(60);
  assert.equal(h.confirms.length, 1, 'the scan was read for a command with a typed value unsaved, and nothing was asked');
  assert.match(h.confirms[0], /what you typed into the form/);
  assert.equal(posted, null, 'the user said No and the page was read');
  assert.equal(toastText(), 'This page is a scan and was not read, so your edits are kept.');
  // The OCR button asks the same question.
  $('ocrBtn').click();
  await settle(60);
  assert.equal(h.confirms.length, 2, 'the OCR button read a scan with a typed value unsaved and asked nothing');
  assert.equal(posted, null);
  h.setConfirmAnswer(true);
});

// ── /pending 778 — a page is selected from the keyboard ──────────────────────
// A thumbnail was a canvas with a click handler and nothing else, and the click was the only way into the
// selection — so the selection bar's rotate, move and delete could not be reached without a pointer.
// Cannot be seen here: the focus ring (tier 3 — it is CSS), and a screen reader's reading of the button.
test('a thumbnail is a tab stop that says what it is; Enter goes to the page, Space selects, Shift+Space selects a range', async () => {
  await open('th.pdf', ['a', 'b', 'c', 'd'], []);
  const thumbs = [...doc.querySelectorAll('.thumbgrid:not([hidden]) .thumb')];
  assert.equal(thumbs.length, 4, 'setup: the four thumbnails were not built');
  const key = (el, k, extra = {}) => {
    const e = new win.KeyboardEvent('keydown', { key: k, bubbles: true, cancelable: true, ...extra });
    el.dispatchEvent(e);
    return e;
  };
  const selected = () => thumbs.map((t, i) => (t.parentNode.classList.contains('selected') ? i + 1 : 0)).filter(Boolean);
  for (const t of thumbs) {
    assert.equal(t.tabIndex, 0, 'a thumbnail cannot be reached with Tab');
    assert.equal(t.getAttribute('role'), 'button');
  }
  assert.equal(thumbs[2].getAttribute('aria-label'), 'Page 3');
  // Space selects, and says so; the bar with the page actions appears.
  const sp = key(thumbs[1], ' ');
  assert.deepEqual(selected(), [2], 'Space on a thumbnail did not select its page');
  assert.equal(sp.defaultPrevented, true, 'Space also scrolls the panel');
  assert.equal(thumbs[1].getAttribute('aria-pressed'), 'true', 'the selection is shown by colour alone');
  assert.equal($('thumbSelBar').hidden, false);
  assert.equal($('thumbSelCount').textContent, '1 selected');
  // Shift+Space: from the last page chosen to this one.
  key(thumbs[3], ' ', { shiftKey: true });
  assert.deepEqual(selected(), [2, 3, 4], 'Shift+Space did not select the range');
  // Space again takes a page out.
  key(thumbs[2], ' ');
  assert.deepEqual(selected(), [2, 4]);
  assert.equal(thumbs[2].getAttribute('aria-pressed'), 'false');
  // The arrow keys move between thumbnails.
  thumbs[0].focus();
  key(thumbs[0], 'ArrowDown');
  assert.equal(doc.activeElement, thumbs[1], 'ArrowDown did not move to the next thumbnail');
  key(thumbs[1], 'End');
  assert.equal(doc.activeElement, thumbs[3]);
  key(thumbs[3], 'ArrowUp');
  assert.equal(doc.activeElement, thumbs[2]);
  // Enter is the plain click: the page, and the selection cleared.
  key(thumbs[2], 'Enter');
  assert.deepEqual(selected(), [], 'Enter did not clear the selection as a plain click does');
  assert.equal(doc.querySelector('.pageNum').value, '3', 'Enter on a thumbnail did not go to its page');
  assert.equal($('thumbSelBar').hidden, true);
});
