// Scan from a scanner (/pending 903, ADR-213).
//
// ── What only this tier can see ──────────────────────────────────────────────
// The Go tests drive the routes against SANE's own virtual scanner. They cannot see whether the window offers the
// button only where scanimage was found, fills the dialog from the list, sends what was chosen, and installs the
// answer as a document. One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const scans = [];
let devices = [{ id: 'test:0', label: 'Noname frontend-tester' }, { id: 'airscan:w1:Brother', label: 'Brother HL-L2395DW' }];
const h = await boot({
  routes: {
    '/api/status': { state: 'ready', version: 'test', autoUpdate: false, updateCheckLocked: false, scanner: true, formats: [], advanced: {} },
    '/api/scanner/devices': () => ({ devices, modes: ['Color', 'Gray', 'Lineart'], resolutions: [100, 150, 200, 300, 600] }),
    '/api/scanner/scan': (opts) => {
      scans.push(JSON.parse(opts.body));
      return { name: 'Scan 2026-11-05 20.41.pdf', path: '', canSave: false, signature: { state: 'unsigned' }, canUndo: false, canRedo: false };
    },
  },
});
const { document: doc, window: win, settle } = h;
await settle();

const button = () => doc.getElementById('scannerBtn');
const modal = () => doc.getElementById('scannerModal');
const go = () => doc.getElementById('scannerGo');
const options = (id) => [...doc.getElementById(id).options].map((o) => o.textContent);

async function openScan() {
  button().click();
  await settle();
}

test('the Scan… button is in Open & Import and shows when the server found scanimage', () => {
  assert.ok(button(), 'there is no Scan… button');
  assert.equal(button().closest('.tbgroup'), doc.getElementById('openMenuItem').closest('.tbgroup'));
  assert.equal(button().hidden, false, 'scanimage was found and the button is hidden');
  assert.equal(modal().hidden, true, 'the dialog is open before anyone asked for it');
});

test('the dialog lists the scanners and the choices, and names modes in words', async () => {
  await openScan();
  assert.equal(modal().hidden, false);
  assert.deepEqual(options('scannerDevice'), ['Noname frontend-tester', 'Brother HL-L2395DW']);
  assert.deepEqual(options('scannerMode'), ['Color', 'Grayscale', 'Black and white']);
  assert.equal(doc.getElementById('scannerDpi').value, '300', 'the resolution does not start at 300');
  assert.equal(go().disabled, false);
});

test('Scan sends what was chosen and installs the answer as a document', async () => {
  doc.getElementById('scannerDevice').value = 'airscan:w1:Brother';
  doc.getElementById('scannerMode').value = 'Gray';
  doc.getElementById('scannerDpi').value = '150';
  doc.getElementById('scannerSource').value = 'feeder';
  setNextDocument({ numPages: 2, outline: null });
  go().click();
  await settle();
  assert.deepEqual(scans, [{ device: 'airscan:w1:Brother', mode: 'Gray', dpi: 150, feeder: true }]);
  assert.equal(modal().hidden, true, 'the dialog stayed open over the scan it made');
  assert.match(doc.getElementById('tabstrip').textContent, /Scan 2026-11-05 20\.41\.pdf/, 'the scan has no tab');
});

test('Cancel and Escape both close it', async () => {
  await openScan();
  doc.getElementById('scannerCancel').click();
  await settle();
  assert.equal(modal().hidden, true, 'Cancel did not close the dialog');
  await openScan();
  doc.dispatchEvent(new win.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  await settle();
  assert.equal(modal().hidden, true, 'Escape did not close the dialog');
});

test('no scanner found says so and leaves Scan off', async () => {
  devices = [];
  await openScan();
  assert.equal(go().disabled, true, 'Scan is on with nothing to scan with');
  assert.match(doc.getElementById('scannerStatus').textContent, /No scanner found/);
  doc.getElementById('scannerCancel').click();
  await settle();
});
