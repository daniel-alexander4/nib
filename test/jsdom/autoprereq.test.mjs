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
    '/api/paragraphs': () => ({ paragraphs: posted ? paragraphs : [] }),
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
