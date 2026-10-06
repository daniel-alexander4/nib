// A launch into a running Nib arrives in the window that is already open (ADR-086).
//
// ── What this tier can reach ─────────────────────────────────────────────────
// What the page DOES with a hand-off pushed on its window stream: it asks the server what is open
// and shows the new document as the front tab, says the launch's notice when there is no document
// to show, and marks its title while the user is somewhere else. And that a launch's own message on
// the URL (`open`, `notice`) is spent once and removed.
//
// ── What it cannot ───────────────────────────────────────────────────────────
// Whether a real window comes to the front, and whether a second one opens: those are the
// launch's and the window manager's, and tier 3 and the Go tests hold them.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const A = { id: 'test-epoch:1', name: 'alpha.pdf', path: '/tmp/alpha.pdf', canSave: true, signature: { state: 'unsigned' }, canUndo: false, canRedo: false };
const B = { id: 'test-epoch:2', name: 'beta.pdf', path: '/tmp/beta.pdf', canSave: true, signature: { state: 'unsigned' }, canUndo: false, canRedo: false };

// What the server holds. The launch's own document is opened by the page from `?open=`.
let held = { docs: [], activeId: '' };
const opened = [];

setNextDocument({ numPages: 2, outline: null });

const h = await boot({
  search: '?open=' + encodeURIComponent(A.path) + '&notice=handoff-queued',
  routes: {
    '/api/docs': () => held,
    '/api/lastopen': () => [],
    '/api/open': (opts) => {
      opened.push(JSON.parse(opts.body).path);
      held = { docs: [A], activeId: A.id };
      return A;
    },
  },
});
const { document: doc, window: win, settle } = h;
const tabs = () => [...doc.querySelectorAll('#tabstrip .tab')];
const toastText = () => (doc.getElementById('toast') || {}).textContent || '';
const docsCalls = () => h.calls.filter((c) => c.url.split('?')[0].endsWith('/api/docs')).length;

test('a launch\'s message on the URL is spent once and removed', async () => {
  await settle(); await settle();
  assert.deepEqual(opened, [A.path], 'setup: the launch\'s own document was not opened from ?open=');
  assert.equal(toastText(), 'That document will open once you unlock Nib.', 'setup: the notice on the URL was not said');
  assert.equal(win.location.search, '', 'open and notice are still in the address: a reload replays them, and brings back a document the user closed');
});

test('a hand-off pushed to this window shows the document as the front tab', async () => {
  assert.equal(tabs().length, 1, 'setup: one document is open');
  const before = docsCalls();
  held = { docs: [A, B], activeId: B.id }; // the server installed it
  assert.ok(h.pushWindowEvent('handoff', JSON.stringify({ result: 'opened' })), 'no listener took the hand-off: the open window never hears a launch');
  await settle(); await settle();
  assert.equal(docsCalls(), before + 1, 'the window did not ask the server what is open');
  assert.equal(tabs().length, 2, 'the handed-off document did not arrive as a tab');
  assert.equal(doc.querySelector('#tabstrip .tab[aria-selected="true"]')?.textContent.includes('beta.pdf'), true, 'the new document is not the front tab');
});

test('a refused or queued hand-off says so and asks for nothing', async () => {
  const before = docsCalls();
  h.pushWindowEvent('handoff', JSON.stringify({ result: 'refused' }));
  await settle();
  assert.equal(toastText(), 'That document could not be opened here — Nib may be full.');
  h.pushWindowEvent('handoff', JSON.stringify({ result: 'queued' }));
  await settle();
  assert.equal(toastText(), 'That document will open once you unlock Nib.');
  assert.equal(docsCalls(), before, 'a hand-off that opened nothing made the window re-read the documents');
});

test('the title is marked until the window is next focused', async () => {
  // jsdom's document.hasFocus() is false: this window is one the user is not in.
  assert.equal(doc.hasFocus(), false, 'setup: jsdom reports focus, so the unfocused branch is not what ran');
  const base = doc.title.replace(/^● /, '');
  assert.equal(doc.title, '● ' + base, 'a window the user is not looking at took a document and shows no sign of it');
  win.dispatchEvent(new win.Event('focus'));
  assert.equal(doc.title, base, 'the mark outlived the user coming back');
});
