// The icon toolbar (ADR-087): every control in the fixed bar is an icon whose word is still in the
// markup, the page position and Undo/Redo are back in the bar, and one group appears only while a
// tool is armed.
//
// ── What this tier can reach ─────────────────────────────────────────────────
// STRUCTURE and STATE: that each button carries an icon and a word and no bare text; what the page
// buttons, Undo/Redo and the armed-tool group do and when they are available; that both page
// readouts are written together; the standing marks on Undo and Reload.
//
// ── What it cannot ───────────────────────────────────────────────────────────
// Whether the word is actually out of sight in the bar and in sight inside More — that is layout,
// and jsdom has none — and the zoom level, which the real viewer computes. test/ui/toolbaricons.test.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';
import { viewersMade } from './stub-viewer.mjs';

const DOC = '/tmp/nib-harness/toolbar.pdf';
let reply = { id: 'test-epoch:1', name: 'toolbar.pdf', path: DOC, canSave: true, signature: { state: 'unsigned' }, canUndo: false, canRedo: false };
const undone = [];

const h = await boot({
  routes: {
    '/api/open': () => reply,
    '/api/undo': () => { undone.push('undo'); return reply; },
    '/api/redo': () => { undone.push('redo'); return reply; },
    '/api/close': () => ({}),
    '/api/lastopen': () => [],
  },
});
const { document: doc, settle } = h;
const $ = (id) => doc.getElementById(id);
const bar = () => [...doc.querySelectorAll('#toolbar .tbfixed .tbgroup > button')];

async function open(meta) {
  reply = { ...reply, ...meta };
  setNextDocument({ numPages: 3, outline: null });
  $('pathInput').value = DOC;
  $('openGo').click();
  await settle(); await settle();
}

test('every control in the fixed bar is an icon that still carries its word', () => {
  const buttons = bar();
  assert.ok(buttons.length >= 18, `only ${buttons.length} buttons found in the fixed bar — the selector has stopped matching`);
  for (const b of buttons) {
    const name = b.id || b.dataset.forward;
    const word = b.querySelector('.tblabel');
    assert.ok(word && word.textContent.trim(), `${name} has no .tblabel word: folded into More it is a picture with no name`);
    const face = b.querySelector('svg') || b.querySelector('#zoomPct');
    assert.ok(face, `${name} has neither an icon nor the zoom readout`);
    const bare = [...b.childNodes].filter((n) => n.nodeType === 3 && n.textContent.trim());
    assert.deepEqual(bare.map((n) => n.textContent.trim()), [], `${name} shows a word in the bar`);
    assert.ok(b.title, `${name} has no tooltip`);
  }
  // The word is the WHOLE text, so a test that finds a button by its text still finds it.
  assert.equal($('zoomInBtn').textContent, 'Zoom in');
  assert.equal($('saveBtn').textContent, 'Save');
});

test('nothing in the client writes a bar button\'s text over its icon', () => {
  const src = fs.readFileSync(new URL('../../web/app.js', import.meta.url), 'utf8');
  for (const id of ['readAloudBtn', 'saveBtn', 'undoBtn', 'redoBtn', 'reloadBtn']) {
    assert.doesNotMatch(src, new RegExp(id + "\\)?\\.textContent\\s*="), `${id}.textContent is assigned: that replaces the icon with a word`);
  }
});

test('with nothing open, the page buttons and Undo/Redo are off and no tool is shown', () => {
  for (const id of ['prevPageBtn', 'nextPageBtn', 'undoBtn', 'redoBtn']) {
    assert.equal($(id).disabled, true, `${id} is clickable with no document open`);
  }
  assert.equal($('armedGroup').hidden, true);
});

test('Previous and Next turn the page, and both readouts say the same', async () => {
  await open({});
  const nums = () => [...doc.querySelectorAll('.pageNum')].map((i) => i.value);
  assert.equal(doc.querySelectorAll('#toolbar .tbgroup[data-label="Page"] .pageNum').length, 1, 'the bar has no page field');
  assert.equal(doc.querySelectorAll('#sbPages .pageNum').length, 1, 'the Pages tab lost its page field');
  assert.equal($('nextPageBtn').disabled, false, 'setup: Next is off with a three-page document open');
  assert.deepEqual(nums(), ['1', '1']);
  $('nextPageBtn').click();
  assert.deepEqual(nums(), ['2', '2'], 'Next did not move both readouts to page 2');
  $('prevPageBtn').click();
  assert.deepEqual(nums(), ['1', '1'], 'Previous did not move both readouts back');
  assert.deepEqual([...doc.querySelectorAll('.pageCount')].map((s) => s.textContent), ['/ 3', '/ 3']);
});

test('Undo is off with nothing to undo, on when the server has history, and asks the server', async () => {
  assert.equal($('undoBtn').disabled, true, 'a freshly opened document offers Undo');
  assert.equal($('redoBtn').disabled, true);
  await open({ canUndo: true, canRedo: true });
  assert.equal($('undoBtn').disabled, false, 'the server has history and Undo is off — the mouse has no way to undo');
  assert.equal($('redoBtn').disabled, false);
  $('undoBtn').click(); await settle();
  $('redoBtn').click(); await settle();
  assert.deepEqual(undone, ['undo', 'redo']);
});

test('released history is said on the Undo button, and only while there is nothing to undo', async () => {
  await open({ canUndo: false, canRedo: false, historyEvicted: true });
  assert.equal($('undoBtn').disabled, true);
  assert.equal($('undoBtn').classList.contains('attn'), true, 'history was released and the Undo button looks the same as "no edits yet"');
  assert.match($('undoBtn').title, /released/);
  await open({ canUndo: true, historyEvicted: true });
  assert.equal($('undoBtn').classList.contains('attn'), false, 'the mark stayed although there is something to undo');
  assert.equal($('undoBtn').title, 'Undo (Ctrl+Z)');
});

test('an armed tool is named in the bar, and its button puts it down', async () => {
  await open({ canUndo: false, historyEvicted: false });
  assert.equal($('armedGroup').hidden, true, 'setup: a tool is already armed');
  $('noteBtn').click(); await settle();
  assert.equal($('armedGroup').hidden, false, 'Note is armed and the bar says nothing');
  assert.equal($('armedName').textContent, 'Note');
  $('armedOffBtn').click(); await settle();
  assert.equal($('armedGroup').hidden, true, 'the tool was put down and the bar still names it');
  assert.equal($('noteBtn').getAttribute('aria-pressed'), 'false', 'the bar\'s button hid the chip and left the tool armed');
});

test('the reload icon is marked while the file on disk differs', async () => {
  assert.equal($('reloadBtn').classList.contains('attn'), false, 'setup: reload is already marked');
  await open({ diskChanged: true });
  assert.equal($('reloadBtn').classList.contains('attn'), true, 'the file changed on disk and the control that answers it is unmarked');
});

test('the zoom level follows the focused document, and no other', async () => {
  // Two documents, so one of them is in the background. Different paths: the same path twice is
  // activated, not opened again.
  reply = { ...reply, id: 'test-epoch:2', name: 'second.pdf', path: '/tmp/nib-harness/second.pdf' };
  setNextDocument({ numPages: 2, outline: null });
  $('pathInput').value = reply.path;
  $('openGo').click();
  await settle(); await settle();
  assert.ok(viewersMade.length >= 2, `setup: ${viewersMade.length} viewer(s) exist, so nothing is in the background`);
  const front = viewersMade.at(-1), back = viewersMade[0];
  front.eventBus.dispatch('scalechanging', { scale: 2.5 });
  assert.equal($('zoomPct').textContent, '250%', 'the focused document zoomed and the bar did not say so');
  back.eventBus.dispatch('scalechanging', { scale: 0.5 });
  assert.equal($('zoomPct').textContent, '250%', 'a document in the background re-scaled and the bar now shows ITS zoom');
});
