// The File card's Open Recent list — /pending 485.
//
// ── What only this tier can see ──────────────────────────────────────────────
// The Go tests prove the server records an open and serves `/api/recent`. They cannot see whether the
// sidebar card ever SHOWS it. It did not: the list moved from a dropdown into the File card, and the
// only thing that filled it was opening a `.menu`, which never happens in the sidebar. So what is read
// here is the card's slot itself, with no menu opened at any point.
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
const { document: doc, settle } = h;
await settle();

const slot = () => doc.querySelector('[data-tab="file"] .recentSlot');
const entries = () => [...slot().querySelectorAll('button')].map((b) => b.textContent);

test('the list is filled at start-up, in the card, with no menu opened', () => {
  assert.ok(slot(), 'the File card has no recent slot');
  assert.equal(doc.querySelector('.menu.open'), null, 'setup: a menu is open, so this cannot tell the card from the menu path');
  assert.deepEqual(entries(), ['lease.pdf'],
    `the card shows ${JSON.stringify(entries())} — a recent list only a dropdown fills is empty in the sidebar`);
});

test('the list has a label a screen reader and a reader can both find', () => {
  const label = doc.getElementById(slot().getAttribute('aria-labelledby'));
  assert.ok(label, 'the slot names no label');
  assert.equal(label.textContent.trim(), 'Open Recent');
  assert.equal(slot().getAttribute('role'), 'group');
});

test('opening a document refreshes the list without a reload', async () => {
  recent = [{ name: 'deed.pdf', path: '/tmp/nib-harness/deed.pdf' }, ...recent];
  setNextDocument({ numPages: 1, outline: null });
  doc.getElementById('pathInput').value = '/tmp/nib-harness/deed.pdf';
  doc.getElementById('openGo').click();
  await settle();
  assert.deepEqual(entries(), ['deed.pdf', 'lease.pdf'],
    `after an open the card shows ${JSON.stringify(entries())} — it is still the list from start-up`);
});

test('an entry opens its file', async () => {
  opened.length = 0;
  setNextDocument({ numPages: 1, outline: null });
  [...slot().querySelectorAll('button')].find((b) => b.textContent === 'lease.pdf').click();
  await settle();
  assert.deepEqual(opened, ['/tmp/nib-harness/lease.pdf']);
});

test('an empty list says so', async () => {
  recent = [];
  setNextDocument({ numPages: 1, outline: null });
  doc.getElementById('pathInput').value = '/tmp/nib-harness/other.pdf';
  doc.getElementById('openGo').click();
  await settle();
  assert.equal(entries().length, 0);
  assert.match(slot().textContent, /No recent files/);
});
