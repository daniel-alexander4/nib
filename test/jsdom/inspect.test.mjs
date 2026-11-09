// Inspect this file (/pending 406): what the open document says about itself.
//
// What each test can catch: a value the document chose rendered as markup (an Author of `<b>x</b>` becoming
// an element), a cut or cleaned value shown without saying so, the tagging line ignoring the document's own
// answer (849 (e) lives here), and a refusal swallowed into an empty description.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import * as pdfjs from './stub-pdfjs.mjs';

const meta = (name, extra = {}) => ({
  id: 'test-epoch:' + name, name, path: '/tmp/nib-harness/' + name, canSave: true,
  signature: { state: 'unsigned' }, canUndo: false, canRedo: false, ...extra,
});
let nextOpen = meta('a.pdf');
let answer = () => ({});
const h = await boot({
  routes: {
    '/api/open': () => nextOpen,
    '/api/docs': () => ({ docs: [], activeId: '' }),
    '/api/close-view': () => nextOpen,
    '/api/inspect': () => answer(),
    '/api/listdir': { path: '/home/someone/nib', parent: '/home/someone', dirs: [], files: [] },
  },
});
const { document: doc, settle } = h;
const $ = (id) => doc.getElementById(id);
const toastText = () => ($('toast') || {}).textContent || '';

async function open(name, extra) {
  nextOpen = meta(name, extra);
  pdfjs.setNextDocument({ numPages: 1, renders: true, text: 'x' });
  $('pathInput').value = '/tmp/nib-harness/' + name;
  $('openGo').click();
  await settle(30);
  if ($('toast')) $('toast').textContent = '';
}

const DESCRIPTION = {
  info: [
    { key: 'Author', value: '<b>Jane</b>', removed: 2 },
    { key: 'Subject', value: 'long', cut: true },
    { key: 'Trapped', value: '', notText: true },
  ],
  xmp: { present: true, text: '<x:xmpmeta><dc:creator>Jane</dc:creator></x:xmpmeta>', elsewhere: 3 },
  fonts: [{ name: 'Helvetica', subtype: 'Type1', embedded: false }],
  scan: { findings: [{ severity: 'high', detail: 'Document-level JavaScript', page: 0 }] },
};

test('Inspect shows each value the file chose as text, and says what was taken out or cut', async () => {
  await open('a.pdf', { tagged: true });
  answer = () => DESCRIPTION;
  $('inspectBtn').click();
  await settle(30);
  const body = $('inspectBody');
  assert.equal($('inspectModal').hidden, false, 'the dialog did not open');
  assert.equal(body.querySelector('b'), null, 'a value the file chose was rendered as markup');
  const text = body.textContent;
  assert.ok(text.includes('<b>Jane</b>'), 'the Author value is not shown as the text it is');
  assert.ok(text.includes('2 hidden character(s) taken out'), 'the characters cleaning took are not said');
  assert.ok(text.includes('shown in part'), 'a cut value is not said to be cut');
  assert.ok(text.includes('(not text, so not shown)'), 'a value that is not text is not said to be');
  assert.ok(body.querySelector('pre').textContent.includes('<dc:creator>'), 'the XMP packet is not shown as text');
  assert.ok(text.includes('XMP on 3 page(s)'), 'the other packets are not counted');
  assert.ok(text.includes('Helvetica') && text.includes('not embedded'), 'the font row is missing');
  assert.ok(text.includes('The file says it is tagged'), 'the tagging line ignored the document\'s own answer');
  assert.ok(text.includes('Document-level JavaScript'), 'the scan findings are missing');
});

test('Inspect on an untagged file says it does not claim tagging', async () => {
  await open('b.pdf', {});
  answer = () => ({ ...DESCRIPTION, info: [], fonts: [] });
  $('inspectBtn').click();
  await settle(30);
  const text = $('inspectBody').textContent;
  assert.ok(text.includes('The file does not say it is tagged'), 'an untagged file was described as tagged');
  assert.ok(text.includes('The file has no document properties.'), 'an empty property list is not said');
});

test('Inspect says the refusal, never an empty description', async () => {
  await open('c.pdf', {});
  answer = () => new Response(JSON.stringify({ cause: 'document-unreadable', error: 'Nib could not read this document\'s own description: x' }), { status: 422, headers: { 'Content-Type': 'application/json' } });
  $('inspectBtn').click();
  await settle(30);
  assert.equal($('inspectModal').hidden, true, 'the dialog stayed open over a refusal');
  assert.match(toastText(), /^Nib could not read this document's own description/);
});
