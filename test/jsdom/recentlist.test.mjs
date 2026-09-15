// Open Recent — a button like Open… that opens a dialog (Dan, 2026-09-14, after /pending 485).
//
// ── What only this tier can see ──────────────────────────────────────────────
// The Go tests prove the server records an open and serves `/api/recent`. They cannot see whether the
// File card offers it: the list lived inline in the card, where nothing ever filled it (/pending 485),
// and then inline under a label. Dan's shape is a button beside Open… and a dialog, so what is driven
// here is that button and what the dialog shows.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

let recent = [{ name: 'lease.pdf', path: '/tmp/nib-harness/lease.pdf' }];
const opened = [];
const h = await boot({
  routes: {
    '/api/recent': () => recent,
    '/api/open': (opts) => {
      const path = JSON.parse(opts.body).path;
      opened.push(path);
      return {
        name: path.split('/').pop(), path, canSave: true,
        signature: { state: 'unsigned' }, canUndo: false, canRedo: false,
      };
    },
  },
});
const { document: doc, window: win, settle } = h;
await settle();

const button = () => doc.getElementById('openRecentBtn');
const modal = () => doc.getElementById('recentModal');
const entries = () => [...doc.querySelectorAll('#recentList button')].map((b) => b.textContent);

async function openRecent() {
  button().click();
  await settle();
}

test('Open Recent is a button in the File card beside Open…, and the card holds no inline list', () => {
  const card = doc.getElementById('openMenuItem').closest('.tbgroup');
  assert.ok(button(), 'there is no Open Recent button');
  assert.equal(button().closest('.tbgroup'), card, 'Open Recent is not in the same card as Open…');
  assert.match(button().textContent, /Open Recent/);
  assert.equal(card.querySelector('.recentSlot, #recentLabel'), null, 'the inline recent list is still in the card');
  assert.equal(modal().hidden, true, 'the dialog is open before anyone asked for it');
});

test('the button opens a dialog listing the recent files, focused on the first', async () => {
  await openRecent();
  assert.equal(modal().hidden, false, 'clicking Open Recent did not open the dialog');
  assert.deepEqual(entries(), ['lease.pdf']);
  assert.equal(doc.activeElement && doc.activeElement.textContent, 'lease.pdf',
    'focus is not on the first entry, so a keyboard user starts outside the list');
});

test('an entry opens its file and closes the dialog', async () => {
  opened.length = 0;
  setNextDocument({ numPages: 1, outline: null });
  [...doc.querySelectorAll('#recentList button')].find((b) => b.textContent === 'lease.pdf').click();
  await settle();
  assert.deepEqual(opened, ['/tmp/nib-harness/lease.pdf']);
  assert.equal(modal().hidden, true, 'the dialog stayed open over the document it opened');
});

test('the list is read again each time the dialog opens', async () => {
  recent = [{ name: 'deed.pdf', path: '/tmp/nib-harness/deed.pdf' }, ...recent];
  await openRecent();
  assert.deepEqual(entries(), ['deed.pdf', 'lease.pdf'],
    `the dialog shows ${JSON.stringify(entries())} — the list from an earlier opening`);
});

test('Cancel and Escape both close it', async () => {
  doc.getElementById('recentCancel').click();
  await settle();
  assert.equal(modal().hidden, true, 'Cancel did not close the dialog');
  await openRecent();
  doc.dispatchEvent(new win.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  await settle();
  assert.equal(modal().hidden, true, 'Escape did not close the dialog');
});

test('an empty list says so', async () => {
  recent = [];
  await openRecent();
  assert.equal(entries().length, 0);
  assert.match(doc.getElementById('recentList').textContent, /No recent files/);
  doc.getElementById('recentCancel').click();
  await settle();
});
