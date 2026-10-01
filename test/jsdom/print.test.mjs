// /pending 788 — Print in a browser whose PDF frame this page may not script.
//
// Measured in Firefox 157 (deepdives/2026-09-30-the-print-flow.md): the PDF.js viewer in the hidden print frame is
// CROSS-ORIGIN to nib's page, so `contentWindow.addEventListener` throws a SecurityError, and the handler used to die
// there — `print()` after it never ran and the button did nothing. Stock Ubuntu opens nib that way (no Chromium-family
// browser found → xdg-open → Firefox).
//
// ## What this cannot see
//
// - A real print dialog, or a real cross-origin frame: jsdom loads no PDF and has no print. The frame's window is
//   stubbed to behave as Firefox's measurably did — `focus` and `addEventListener` throwing, `print` allowed — and as a
//   browser refusing `print` too. Tier 3 runs Chromium only, where the handler always worked.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const h = await boot({
  routes: {
    // The save the refusal offers browses a folder.
    '/api/listdir': { path: '/home/someone/nib', parent: '/home/someone', dirs: [], files: [] },
    '/api/open': (opts) => {
      const { path: p } = JSON.parse(opts.body);
      const name = p.split('/').pop();
      return { id: 'pr:' + name, name, path: p, canSave: true, signature: { state: 'unsigned' }, canUndo: false, canRedo: false };
    },
  },
});
const { document: doc, settle } = h;
globalThis.URL.createObjectURL = () => 'blob:nib-print';
globalThis.URL.revokeObjectURL = () => {};
const toastText = () => (doc.getElementById('toast') || { textContent: '' }).textContent;

setNextDocument({ numPages: 1 });
doc.getElementById('pathInput').value = '/tmp/nib-harness/contract.pdf';
doc.getElementById('openGo').click();
await settle();

const securityError = (prop) => Object.assign(new Error(`Permission denied to access property "${prop}" on cross-origin object`), { name: 'SecurityError' });

// clickPrint presses Print with the frame's window replaced by win, fires the frame's load, and returns what was called.
async function clickPrint(win) {
  const before = new Set(doc.querySelectorAll('iframe'));
  doc.getElementById('printBtn').click();
  await settle();
  const frame = [...doc.querySelectorAll('iframe')].find((f) => !before.has(f));
  assert.ok(frame, 'Print appended no frame — the stimulus never happened');
  Object.defineProperty(frame, 'contentWindow', { value: win, configurable: true });
  frame.onload();
  await settle();
  return frame;
}

test('a frame whose window cannot be listened to still prints — Firefox, the stock Ubuntu path', async () => {
  let printed = 0;
  const win = {
    focus() { throw securityError('focus'); },
    addEventListener() { throw securityError('addEventListener'); },
    print() { printed++; },
  };
  await clickPrint(win);
  assert.equal(printed, 1, 'print() was not reached after the frame refused addEventListener');
  assert.doesNotMatch(toastText(), /would not print/, 'a print that worked was reported as refused');
  assert.equal(doc.getElementById('saveAsModal').hidden, true, 'a print that worked offered a save');
});

test('a browser that refuses print() says so and offers the PDF to print elsewhere', async () => {
  const win = {
    focus() {},
    addEventListener() {},
    print() { throw securityError('print'); },
  };
  const frame = await clickPrint(win);
  assert.match(toastText(), /would not print from inside Nib/, 'the refusal was not reported');
  assert.equal(doc.getElementById('saveAsModal').hidden, false, 'no save was offered');
  assert.equal(doc.getElementById('saveAsName').value, 'contract.pdf', 'the save does not name the document printed');
  assert.equal(frame.isConnected, false, 'the refused frame was left in the page');
  doc.getElementById('saveAsModal').hidden = true;
});

test('a frame that can be listened to registers afterprint and prints', async () => {
  const events = [];
  const win = {
    focus() {},
    addEventListener(type) { events.push(type); },
    print() { events.push('print'); },
  };
  await clickPrint(win);
  assert.deepEqual(events, ['afterprint', 'print']);
});

test('a frame nobody could listen to stays for its preview and goes when the next print starts', async () => {
  const silent = { focus() {}, addEventListener() { throw securityError('addEventListener'); }, print() {} };
  const first = await clickPrint(silent);
  await new Promise((r) => setTimeout(r, 50));
  assert.equal(first.isConnected, true, 'the frame was removed while its preview may still be open');
  await clickPrint(silent);
  assert.equal(first.isConnected, false, 'the last print\'s frame was not removed when the next one started');
});
