// The three destination dialogs (Save As, bookmark split, page split) open their folder browser on the
// open document's own folder, through ONE door (ADR-214), and on ~/nib when the document names none.
//
// ## What this cannot see
//
// - Whether the server honours `start=1` (a folder that is gone, unreadable or Nib's own ceremony store
//   answers ~/nib): `/api/listdir` is stubbed here. That half is `TestAStartFolderIsVettedAndFallsBackQuietly`.
// - The folder a CONVERTED document names: the stub's `/api/open` answers whatever `dir` the test gives it.
//   The server half is `TestADocumentNamesTheFolderItCameFrom`.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

let dir = '';
const h = await boot({
  routes: {
    '/api/listdir': { path: '/home/someone/nib', parent: '/home/someone', dirs: [], files: [] },
    '/api/open': (opts) => {
      const { path: p } = JSON.parse(opts.body);
      const name = p.split('/').pop();
      const meta = { id: 'sf:' + name, name, path: p, canSave: true, signature: { state: 'unsigned' }, canUndo: false, canRedo: false };
      if (dir) meta.dir = dir;
      return meta;
    },
    '/api/close': { name: '', path: '', canSave: false, signature: { state: '' }, canUndo: false, canRedo: false },
  },
});
const { document: doc, settle } = h;
const $ = (id) => doc.getElementById(id);

async function open(name) {
  setNextDocument({ numPages: 3, text: 'Invoice', data: [...'%PDF-1.7 x %%EOF'].map((c) => c.charCodeAt(0)), outline: [{ title: 'One', dest: null, items: [] }] });
  $('pathInput').value = '/tmp/nib-harness/' + name;
  $('openGo').click();
  await settle();
}
const lastListdir = () => h.calls.filter((c) => c.url.includes('/api/listdir')).at(-1)?.url ?? '';

test('Save As starts in the folder the document named, and says it is a suggestion', async () => {
  dir = '/home/someone/Barn';
  await open('load-sheet.pdf');
  $('saveAsBtn').click();
  await settle();
  const u = lastListdir();
  assert.match(u, /path=%2Fhome%2Fsomeone%2FBarn$/, 'the dialog did not ask for the document\'s own folder: ' + u);
  assert.match(u, /start=1&/, 'the folder was not marked as a suggestion the server may replace');
  $('saveAsCancel').click();
  await settle();
});

test('a document that names no folder starts on the default, not a suggestion', async () => {
  dir = '';
  await open('upload.pdf');
  $('saveAsBtn').click();
  await settle();
  const u = lastListdir();
  assert.match(u, /start=0&path=$/, 'a pathless document asked for something other than the default: ' + u);
  $('saveAsCancel').click();
  await settle();
});

test('both split dialogs start where Save As does', async () => {
  dir = '/home/someone/Barn';
  await open('split-me.pdf');
  $('exportPageSplitBtn').click();
  await settle();
  assert.match(lastListdir(), /start=1&path=%2Fhome%2Fsomeone%2FBarn$/, 'the page split did not start in the document\'s folder');
  $('exportBookmarkSplitBtn').click();
  await settle();
  assert.match(lastListdir(), /start=1&path=%2Fhome%2Fsomeone%2FBarn$/, 'the bookmark split did not start in the document\'s folder');
});

test('no destination dialog opens its browser on a bare browseDir(\'\') any more (one door, ADR-009)', () => {
  const src = readFileSync(new URL('../../web/app.js', import.meta.url), 'utf8');
  const bare = src.split('\n').filter((l) => /browseDir\(''/.test(l) && !l.trim().startsWith('//'));
  assert.deepEqual(bare, [], 'a dialog bypasses startFolder: ' + bare.join(' | '));
});
