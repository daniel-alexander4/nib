// Clear form (/pending 849 (b)): every field goes back to the value the form gives it (/DV), or empty.
//
// What each test can catch: a reset that blanks a field the form pre-fills (the DV case), a check box
// or radio group that comes back with the wrong widget on (two radio buttons on would be a form no
// reader saves cleanly — ADR-172 is the neighbour), a push button or signature written to, a clear that
// runs although the person said no, and a field on screen left showing what was typed.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import * as pdfjs from './stub-pdfjs.mjs';

const meta = (name) => ({
  id: 'test-epoch:' + name, name, path: '/tmp/nib-harness/' + name, canSave: true,
  signature: { state: 'unsigned' }, canUndo: false, canRedo: false,
});
let nextOpen = meta('form.pdf');
const h = await boot({
  routes: {
    '/api/open': () => nextOpen,
    '/api/docs': () => ({ docs: [], activeId: '' }),
    '/api/close-view': () => nextOpen,
    '/api/listdir': { path: '/home/someone/nib', parent: '/home/someone', dirs: [], files: [] },
  },
});
const { document: doc, settle } = h;
const $ = (id) => doc.getElementById(id);
const toastText = () => ($('toast') || {}).textContent || '';

const FIELDS = {
  name: [{ id: 'w1', type: 'text', defaultValue: '' }],
  county: [{ id: 'w2', type: 'text', defaultValue: 'Multnomah' }],
  agree: [{ id: 'w3', type: 'checkbox', defaultValue: 'Yes', exportValues: 'Yes' }],
  optout: [{ id: 'w4', type: 'checkbox', defaultValue: 'Off', exportValues: 'On' }],
  plan: [
    { id: 'w5', type: 'radiobutton', defaultValue: 'B', buttonValue: 'A' },
    { id: 'w6', type: 'radiobutton', defaultValue: 'B', buttonValue: 'B' },
  ],
  state: [{ id: 'w7', type: 'combobox', defaultValue: 'OR' }],
  submit: [{ id: 'w8', type: 'button' }],
  sig: [{ id: 'w9', type: 'signature' }],
};

let stored;
async function open(name, fields) {
  nextOpen = meta(name);
  pdfjs.setNextDocument({ numPages: 1, renders: true, text: 'form', fieldObjects: fields });
  $('pathInput').value = '/tmp/nib-harness/' + name;
  $('openGo').click();
  await settle(30);
  stored = [];
  pdfjs.lastDocument.annotationStorage.setValue = (id, v) => stored.push([id, v.value]);
  h.confirms.length = 0; h.setConfirmAnswer(true);
  if ($('toast')) $('toast').textContent = '';
}

test('Clear form puts each field back to the form\'s own default, and leaves buttons and signatures alone', async () => {
  await open('form.pdf', FIELDS);
  // A typed name and an unticked box on screen, so the reset has to reach the element as well as storage.
  const host = [...doc.querySelectorAll('.viewerContainer')].pop();
  const typed = doc.createElement('input'); typed.type = 'text'; typed.value = 'Jane Doe'; typed.dataset.elementId = 'w1';
  const box = doc.createElement('input'); box.type = 'checkbox'; box.checked = false; box.dataset.elementId = 'w3';
  host.append(typed, box);
  $('clearFormBtn').click();
  await settle(30);
  assert.equal(h.confirms.length, 1, 'it cleared without asking');
  assert.match(h.confirms[0], /^Clear 6 field\(s\)/, 'the question did not count the fields it would clear');
  assert.deepEqual(Object.fromEntries(stored), {
    w1: '', w2: 'Multnomah', w3: true, w4: false, w5: false, w6: true, w7: 'OR',
  }, 'a field did not go back to the form\'s own default');
  assert.ok(!stored.some(([id]) => id === 'w8' || id === 'w9'), 'a push button or signature field was written');
  assert.equal(typed.value, '', 'the field on screen still shows what was typed');
  assert.equal(box.checked, true, 'the check box on screen did not go back to its default');
  assert.equal(toastText(), 'Cleared 6 field(s) — review and Save');
});

test('Clear form does nothing when the person says no', async () => {
  await open('form2.pdf', FIELDS);
  h.setConfirmAnswer(false);
  $('clearFormBtn').click();
  await settle(30);
  assert.equal(h.confirms.length, 1, 'setup: the question was not asked');
  assert.deepEqual(stored, [], 'a refused clear still cleared');
});

test('Clear form on a document with no fields says so and asks nothing', async () => {
  await open('flat.pdf', null);
  $('clearFormBtn').click();
  await settle(30);
  assert.equal(h.confirms.length, 0, 'it asked to clear a form that has no fields');
  assert.equal(toastText(), 'This PDF has no form fields to clear');
});
