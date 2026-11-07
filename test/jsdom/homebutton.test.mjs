// The destination dialogs (Save As, both splits) offer the default folder, ~/nib, one press away wherever they are
// (ADR-215): the button's text is the folder's whole path, it has no tooltip, and it is hidden where the dialog
// already is. Whether the dialog is there is the server's answer (`atDefault`).
//
// ## What this cannot see
//
// - What the server answers for `defaultDir` and `atDefault`: `/api/listdir` is stubbed. That half is
//   `TestListDirNamesTheDefaultFolder` (internal/server/startfolder_test.go).
// - Whether a long path wraps instead of being cut: jsdom has no layout. `.dirhome` in style.css says so.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const DEF = '/home/someone/nib';
const h = await boot({
  routes: {
    // The stub is handed only the options, so it reads the URL of the call boot just recorded.
    '/api/listdir': () => {
      const p = new URL(h.calls.at(-1).url, 'http://x').searchParams.get('path') || DEF;
      return { path: p, parent: '/', dirs: [], files: [], defaultDir: DEF, atDefault: p === DEF };
    },
    '/api/open': (opts) => {
      const { path: p } = JSON.parse(opts.body);
      const name = p.split('/').pop();
      return { id: 'hb:' + name, name, path: p, canSave: true, dir: '/home/someone/Barn', signature: { state: 'unsigned' }, canUndo: false, canRedo: false };
    },
  },
});
const { document: doc, settle } = h;
const $ = (id) => doc.getElementById(id);

async function open(name) {
  setNextDocument({ numPages: 3, text: 'Invoice', data: [...'%PDF-1.7 x %%EOF'].map((c) => c.charCodeAt(0)), outline: [{ title: 'One', dest: null, items: [] }] });
  $('pathInput').value = '/home/someone/Barn/' + name;
  $('openGo').click();
  await settle();
}

test('away from ~/nib, each destination dialog shows the default folder by its whole path, with no tooltip', async () => {
  await open('sheet.pdf');
  for (const [btn, home, cancel] of [['saveAsBtn', 'saveAsHome', 'saveAsCancel'], ['exportPageSplitBtn', 'psHome', null], ['exportBookmarkSplitBtn', 'bsHome', null]]) {
    $(btn).click();
    await settle();
    const b = $(home);
    assert.ok(b, `no #${home} in index.html`);
    assert.equal(b.hidden, false, `${home}: the dialog is in /home/someone/Barn and the way back to ~/nib is hidden`);
    assert.equal(b.textContent, DEF, `${home} does not carry the folder's whole path`);
    assert.equal(b.getAttribute('title'), null, `${home} has a tooltip; its text is the path`);
    if (cancel) $(cancel).click();
    await settle();
  }
});

test('pressing it lists ~/nib, and there the button hides', async () => {
  $('saveAsBtn').click();
  await settle();
  assert.equal($('saveAsHere').textContent, '/home/someone/Barn', 'setup: the dialog did not start in the document\'s folder');
  $('saveAsHome').click();
  await settle();
  assert.equal($('saveAsHere').textContent, DEF, 'the button did not take the dialog to ~/nib');
  assert.equal($('saveAsHome').hidden, true, 'at ~/nib the button still shows');
  $('saveAsCancel').click();
  await settle();
});

test('the Open dialog has no such button (only destinations were asked for)', () => {
  assert.equal(doc.querySelector('#openModal .dirhome'), null);
});
