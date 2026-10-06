// Drag to pan, and Resume last session (ADR-085).
//
// ── What this tier can reach ─────────────────────────────────────────────────
// WHOSE drag it is. The pan is the eleventh `pointerdown` on #viewerWrap, and every refusal it
// makes — a tool is armed, the press landed on text, on a field, on an overlay, the page does not
// overflow — is a decision taken from the event and the DOM, which is what this tier has. So is
// the whole of Resume: what the server recorded, what the button says, what it asks to be opened.
//
// ── What it cannot ───────────────────────────────────────────────────────────
// LAYOUT. Every scroll dimension here is 0, so "the page overflows" is written onto the scroll box
// by hand, and jsdom implements no `setPointerCapture` and no ResizeObserver — the hand cursor's
// `.can-pan` is never set at this tier. That the page really moves under a real mouse, by the
// distance the mouse moved, is tier 3's (test/ui/pan.test.mjs).
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const A = { id: 'test-epoch:1', name: 'alpha.pdf', path: '/tmp/alpha.pdf', canSave: true, signature: { state: 'unsigned' }, canUndo: false, canRedo: false };
const B = { id: 'test-epoch:2', name: 'beta.pdf', path: '/tmp/beta.pdf', canSave: true, signature: { state: 'unsigned' }, canUndo: false, canRedo: false };
const byPath = { [A.path]: A, [B.path]: B };

let recorded = [{ path: A.path, name: 'alpha.pdf' }, { path: B.path, name: 'beta.pdf' }];
const opened = [];

setNextDocument({ numPages: 3, outline: null });

const h = await boot({
  routes: {
    '/api/lastopen': () => recorded,
    '/api/close': { name: '', path: '', canSave: false, signature: { state: '' }, canUndo: false, canRedo: false },
    '/api/open': (opts) => {
      const { path } = JSON.parse(opts.body);
      opened.push(path);
      return byPath[path];
    },
  },
});
const { document: doc, window: win, settle } = h;
const wrap = doc.getElementById('viewerWrap');
const resume = doc.getElementById('resumeBtn');

test('the launch state offers what was open, by count and by name', async () => {
  await settle();
  assert.equal(wrap.className, '', 'setup: a document is already open, so this is not the launch state');
  assert.equal(resume.hidden, false, 'the server recorded two documents and nothing offers them back');
  assert.equal(resume.textContent, 'Resume last session — 2 documents');
  assert.equal(resume.title, 'alpha.pdf, beta.pdf');
  // The launch sentence is three tiers' definition of "nothing is open"; the offer sits beside it.
  assert.equal(doc.getElementById('empty').textContent, 'Open a PDF to begin.');
});

test('Resume reopens every recorded file, in order', async () => {
  assert.deepEqual(opened, [], 'setup: something was opened before anyone asked');
  resume.click();
  await settle(); await settle(); await settle();
  assert.deepEqual(opened, [A.path, B.path]);
  assert.equal(wrap.className, 'has-doc');
  assert.equal(doc.querySelectorAll('#tabstrip .tab').length, 2);
});

// ── the pan ──────────────────────────────────────────────────────────────────

const active = () => doc.querySelector('.viewerContainer:not([hidden])');

// overflow writes the geometry jsdom does not compute. `by` 0 is a page that fits.
function overflow(box, by) {
  for (const [k, v] of [['clientWidth', 500], ['clientHeight', 400], ['scrollWidth', 500 + by], ['scrollHeight', 400 + by]]) {
    Object.defineProperty(box, k, { configurable: true, value: v });
  }
  box.scrollLeft = 100; box.scrollTop = 100;
}
const ev = (type, x, y, button = 0) => new win.MouseEvent(type, { bubbles: true, cancelable: true, clientX: x, clientY: y, button });

// drag presses on `target`, moves by (dx, dy) and releases; reports how far the box scrolled.
function drag(target, dx, dy, button = 0) {
  const box = active();
  const before = { left: box.scrollLeft, top: box.scrollTop };
  target.dispatchEvent(ev('pointerdown', 200, 200, button));
  wrap.dispatchEvent(ev('pointermove', 200 + dx, 200 + dy, button));
  const during = box.classList.contains('panning');
  wrap.dispatchEvent(ev('pointerup', 200 + dx, 200 + dy, button));
  return { dx: box.scrollLeft - before.left, dy: box.scrollTop - before.top, during, after: box.classList.contains('panning') };
}

// A page with one run of text on it, as pdf.js lays one out.
function pageWithText() {
  const box = active();
  let page = box.querySelector('.page[data-pan-fixture]');
  if (!page) {
    page = doc.createElement('div');
    page.className = 'page'; page.dataset.panFixture = '1';
    page.innerHTML = '<div class="textLayer"><span class="run">some text</span></div>'
      + '<div class="annotationLayer"><section><input class="field"></section></div>'
      + '<div class="ovl"><b class="handle"></b></div>';
    box.querySelector('.pdfViewer').appendChild(page);
  }
  return { box, page, blank: page.querySelector('.textLayer'), run: page.querySelector('.run'),
    field: page.querySelector('.field'), handle: page.querySelector('.handle') };
}

test('a drag on the page moves the view against the pointer', () => {
  const { box, blank } = pageWithText();
  overflow(box, 1000);
  const got = drag(blank, 60, -40);
  assert.deepEqual({ dx: got.dx, dy: got.dy }, { dx: -60, dy: 40 },
    'the page did not follow the hand: dragging right must reveal what is to the LEFT');
  assert.equal(got.during, true, 'the closed hand was never shown during the drag');
  assert.equal(got.after, false, 'the closed hand outlived the drag');
});

test('a press that does not travel is a click, not a drag', () => {
  const { box, blank } = pageWithText();
  overflow(box, 1000);
  const got = drag(blank, 2, 2);
  assert.deepEqual({ dx: got.dx, dy: got.dy, during: got.during }, { dx: 0, dy: 0, during: false });
});

test('a drag that starts on text is a selection, and the middle button pans even there', () => {
  const { box, run } = pageWithText();
  overflow(box, 1000);
  assert.deepEqual(drag(run, 60, 60).dy, 0, 'a drag on a run of text scrolled the page, so text can no longer be selected');
  assert.equal(drag(run, 60, 60, 1).dy, -60, 'the middle button did not pan from text — on a page of text there is then nowhere to take hold');
});

test('a field and an overlay keep their own drags', () => {
  const { box, field, handle } = pageWithText();
  overflow(box, 1000);
  assert.equal(drag(field, 60, 60).dy, 0, 'a drag in a form field scrolled the page');
  assert.equal(drag(handle, 60, 60).dy, 0, 'a drag on an overlay handle scrolled the page under it');
  assert.equal(drag(handle, 60, 60, 1).dy, 0, 'the middle button took a drag that belongs to an overlay');
});

test('a page that fits has nothing to pan', () => {
  const { box, blank } = pageWithText();
  overflow(box, 0);
  const got = drag(blank, 60, 60);
  assert.deepEqual({ dy: got.dy, during: got.during }, { dy: 0, during: false });
});

test('an armed tool owns the pointer, and the hand is withdrawn while it does', async () => {
  const { box, blank } = pageWithText();
  overflow(box, 1000);
  assert.equal(drag(blank, 60, 60).dy, -60, 'setup: the pan does not work before the tool is armed, so its refusal below proves nothing');
  assert.equal(wrap.hasAttribute('data-no-pan'), false, 'setup: the hand is already withdrawn');

  doc.getElementById('redactBtn').click();
  await settle();
  assert.equal(wrap.style.cursor, 'crosshair', 'setup: Redact did not arm');
  assert.equal(wrap.hasAttribute('data-no-pan'), true, 'Redact is armed and the page still offers the hand');
  overflow(box, 1000);
  assert.equal(drag(blank, 60, 60).dy, 0, 'a drag with Redact armed scrolled the page instead of drawing');

  doc.getElementById('redactBtn').click();
  await settle();
  assert.equal(wrap.hasAttribute('data-no-pan'), false, 'the tool was put down and the hand did not come back');
  assert.equal(wrap.className, 'has-doc', 'the pan wrote a class onto the wrap, whose className two tiers read whole');
});

test('closing everything brings the offer back, and an empty record offers nothing', async () => {
  recorded = [{ path: A.path, name: 'alpha.pdf' }];
  doc.getElementById('closeAllBtn').click();
  await settle(); await settle();
  assert.equal(wrap.className, '', 'setup: Close did not return to the launch state');
  assert.equal(resume.hidden, false);
  assert.equal(resume.textContent, 'Resume last session — 1 document');

  recorded = [];
  resume.click(); // reopens alpha
  await settle(); await settle();
  doc.getElementById('closeAllBtn').click();
  await settle(); await settle();
  assert.equal(resume.hidden, true, 'nothing is recorded and the button is still offered');
  assert.equal(resume.textContent, 'Resume last session', 'the count of a session that is gone is still on the button');
});
