// ADR-103 — an export with several formats is ONE button, and the Save dialog's Format line chooses.
//
// Three families go through it: this page's table (xlsx / csv / ods), form data (csv / json / xfdf)
// and pages as images (this page PNG / every page ZIP). What is held here, per family where this
// tier can drive it:
//
//   * the bytes written are the SELECTED format's, never another's under its name — including when
//     the format is changed after the dialog opened;
//   * the name's extension follows the format only while the name is the dialog's own;
//   * a maker that takes time says so, holds Save, and a failure stays in the dialog;
//   * a refusal that used to come before the dialog still does (no text on the page, no form);
//   * what the export acts on is what was open at the PRESS, not at Save;
//   * a save with no formats is the dialog it always was.
//
// ## What this cannot see
//
// - A picture actually rendered: jsdom has no canvas, so the two "Pages as images" makers always
//   FAIL here after the bake. That is used — it is the failure path — and the success path (a PNG
//   and a ZIP written under their own names) is tier 3's, `test/ui/filemenu.test.mjs`.
// - Layout: that the Format line lines up with the name field, and focus rings.
// - A real screen reader hearing "Preparing…". The live region's attributes are asserted, not speech.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument, setDataGate } from './stub-pdfjs.mjs';

const written = [];        // every /api/write: { dir, name, bytes, overwrite }
let existing = new Set();  // names /api/write answers 412 for until `overwrite` is sent
let formAnswer = null;     // a Response the form-data route gives instead of the format's bytes
let tableAnswer = null;    // likewise for /api/table
let calls = [];
const formatOf = () => new URL('http://x' + calls.at(-1).url.replace(/^https?:\/\/[^/]+/, '')).searchParams.get('format');

const h = await boot({
  routes: {
    '/api/listdir': { path: '/home/someone/nib', parent: '/home/someone', dirs: [], files: [] },
    '/api/open': (opts) => {
      const { path: p } = JSON.parse(opts.body);
      const name = p.split('/').pop();
      return { id: 'sf:' + name, name, path: p, canSave: true, signature: { state: 'unsigned' }, canUndo: false, canRedo: false };
    },
    // Each format answers bytes that NAME it, so "which bytes were written" is readable.
    '/api/table': () => tableAnswer || new Response('TABLE-AS-' + formatOf(), { status: 200 }),
    '/api/form-data': () => formAnswer || new Response('FORM-AS-' + formatOf(), { status: 200 }),
    '/api/close': { name: '', path: '', canSave: false, signature: { state: '' }, canUndo: false, canRedo: false },
    '/api/write': async (opts) => {
      const f = opts.body;
      const name = f.get('name');
      if (existing.has(name) && !f.get('overwrite')) return new Response(JSON.stringify({ error: 'exists' }), { status: 412 });
      written.push({ dir: f.get('dir'), name, bytes: await textOf(f.get('data')), overwrite: !!f.get('overwrite') });
      return { path: f.get('dir') + '/' + name };
    },
  },
});
const { document: doc, settle } = h;
calls = h.calls;

// **Two harness seams, neither of them the app's.** A stubbed route answers a NODE `Response`, so
// `res.blob()` is a Node Blob, and jsdom's `FormData.append` refuses anything but its own Blob; and
// jsdom's Blob has no `.text()`. In a browser there is one Blob. So the form the save posts is a
// plain field list here, and the bytes are read whichever kind they are.
globalThis.FormData = class {
  #f = new Map();
  append(k, v) { if (!this.#f.has(k)) this.#f.set(k, v); }
  get(k) { return this.#f.has(k) ? this.#f.get(k) : null; }
};
function textOf(b) {
  if (typeof b.text === 'function') return b.text();
  return new Promise((resolve, reject) => {
    const r = new h.window.FileReader();
    r.onload = () => resolve(r.result);
    r.onerror = () => reject(r.error);
    r.readAsText(b);
  });
}
const $ = (id) => doc.getElementById(id);
const toastText = () => ($('toast') || { textContent: '' }).textContent;
const tabs = () => [...doc.querySelectorAll('#tabstrip .tab')];
const optionLabels = () => [...$('saveAsFormat').options].map((o) => o.textContent);
const requests = (frag, from = 0) => calls.slice(from).filter((c) => c.url.includes(frag));

async function open(name, opts = {}) {
  setNextDocument({ numPages: 3, text: 'Name Amount', ...opts });
  $('pathInput').value = '/tmp/nib-harness/' + name;
  $('openGo').click();
  await settle();
}
async function choose(i) {
  const sel = $('saveAsFormat');
  sel.selectedIndex = i;
  sel.dispatchEvent(new h.window.Event('change', { bubbles: true }));
  await settle();
}
async function save() { $('saveAsGo').click(); await settle(); }
async function cancel() { if (!$('saveAsModal').hidden) { $('saveAsCancel').click(); await settle(); } }

await open('ledger.pdf');

test.afterEach(async () => {
  setDataGate(null);
  formAnswer = null; tableAnswer = null; existing = new Set();
  await cancel();
});

// ── A save with no formats is the dialog it always was ──────────────────────
test('an export with one format shows no Format line and writes its bytes', async () => {
  written.length = 0;
  $('exportTextBtn').click();
  await settle();
  assert.equal($('saveAsModal').hidden, false, 'stimulus: the text export opened no save dialog');
  assert.equal($('saveAsFormatRow').hidden, true, 'a save with one format shows a Format line');
  assert.equal($('saveAsName').value, 'ledger.txt');
  assert.equal($('saveAsStatus').textContent, '');
  await save();
  assert.equal(written.length, 1, 'the save wrote nothing');
  assert.equal(written[0].name, 'ledger.txt');
  assert.equal(written[0].dir, '/home/someone/nib');
  assert.match(written[0].bytes, /Name Amount/, 'the bytes written are not the document\'s text');
  assert.equal($('saveAsModal').hidden, true, 'the dialog stayed open after a save that worked');
});

test('a Format line left over from the last export is gone at the next ordinary save', async () => {
  $('exportTableBtn').click();
  await settle();
  assert.equal($('saveAsFormatRow').hidden, false, 'stimulus: the table export showed no Format line');
  await cancel();
  written.length = 0;
  $('exportTextBtn').click();
  await settle();
  assert.equal($('saveAsFormatRow').hidden, true, 'the Format line survived into a save that has one format');
  await save();
  assert.equal(written.length, 1);
  assert.match(written[0].bytes, /Name Amount/, 'an ordinary save wrote the last export\'s bytes');
});

// ── The dialog's shape ──────────────────────────────────────────────────────
test('the Format line is labelled, sits between name and folder, and the status is a polite live region', async () => {
  $('exportTableBtn').click();
  await settle();
  const sel = $('saveAsFormat');
  assert.equal(sel.closest('label')?.textContent.trim().startsWith('Format'), true, 'the Format select has no visible label');
  const order = [...$('saveAsModal').querySelectorAll('input, select')].map((e) => e.id);
  assert.deepEqual(order, ['saveAsName', 'saveAsFormat', 'saveAsDir'], 'the dialog\'s focus order is not name → format → folder');
  const status = $('saveAsStatus');
  assert.equal(status.getAttribute('role'), 'status');
  assert.equal(status.getAttribute('aria-live'), 'polite');
  assert.equal(doc.activeElement, $('saveAsName'), 'focus did not land on the name field');
});

// ── This page's table ───────────────────────────────────────────────────────
test('the table export offers three formats and writes the selected one under its extension', async () => {
  written.length = 0;
  const from = calls.length;
  $('exportTableBtn').click();
  await settle();
  assert.deepEqual(optionLabels(), ['Excel workbook (.xlsx)', 'Comma-separated values (.csv)', 'OpenDocument spreadsheet (.ods)']);
  assert.equal($('saveAsTitle').textContent, 'Export table');
  assert.equal($('saveAsName').value, 'ledger-p1-table.xlsx');
  assert.equal(requests('/api/table', from).length, 0, 'a format was serialised before anyone asked for it');

  // Switched TWICE after the dialog opened: the bytes must be the last selection's.
  await choose(1);
  assert.equal($('saveAsName').value, 'ledger-p1-table.csv', 'the name did not follow the format');
  await choose(2);
  assert.equal($('saveAsName').value, 'ledger-p1-table.ods', 'the name did not follow the format the second time');
  await save();
  assert.equal(written.length, 1, 'nothing was written');
  assert.deepEqual({ name: written[0].name, bytes: written[0].bytes }, { name: 'ledger-p1-table.ods', bytes: 'TABLE-AS-ods' },
    'the file written is not the selected format\'s bytes under the selected format\'s name');
  assert.deepEqual(requests('/api/table', from).map((c) => c.url.split('format=')[1]), ['ods'],
    'formats nobody saved were serialised');
  assert.equal($('saveAsModal').hidden, true);
});

test('the default table format is xlsx, written without touching the Format line', async () => {
  written.length = 0;
  $('exportTableBtn').click();
  await settle();
  await save();
  assert.deepEqual(written.map((w) => [w.name, w.bytes]), [['ledger-p1-table.xlsx', 'TABLE-AS-xlsx']]);
});

test('a page with no text is refused before any dialog', async () => {
  await open('scan.pdf', { text: '' });
  const from = calls.length;
  $('exportTableBtn').click();
  await settle();
  assert.match(toastText(), /No text on this page to extract/);
  assert.equal($('saveAsModal').hidden, true, 'a page with nothing to extract opened the save dialog');
  assert.equal(requests('/api/table', from).length, 0);
  tabs().find((t) => t.textContent.includes('scan.pdf')).querySelector('.tabclose').click();
  await settle();
  assert.equal(tabs().length, 1, 'cleanup: the scan is still open');
});

test('a spreadsheet the server could not build is said in the dialog, which stays open', async () => {
  written.length = 0;
  $('exportTableBtn').click();
  await settle();
  tableAnswer = new Response('nope', { status: 500 });
  await save();
  assert.equal(written.length, 0, 'a failed export wrote a file');
  assert.equal($('saveAsModal').hidden, false, 'the dialog closed on a failure');
  assert.equal($('saveAsStatus').textContent, 'Could not build the spreadsheet');
  assert.equal($('saveAsGo').disabled, false, 'Save was left disabled after a failure, so there is no retry');
  assert.equal($('saveAsFormat').disabled, false);
  // And the retry works, with the bytes of the format selected THEN.
  tableAnswer = null;
  await choose(1);
  assert.equal($('saveAsStatus').textContent, '', 'the failure is still shown after the format changed');
  await save();
  assert.deepEqual(written.map((w) => [w.name, w.bytes]), [['ledger-p1-table.csv', 'TABLE-AS-csv']]);
});

// ── The name follows the format only while it is the dialog's own ────────────
test('a typed name is kept: a known extension is swapped, anything else is left alone', async () => {
  written.length = 0;
  $('exportTableBtn').click();
  await settle();
  const name = $('saveAsName');

  name.value = 'March figures.xlsx';
  await choose(1);
  assert.equal(name.value, 'March figures.csv', 'a typed name with the old format\'s extension did not take the new one');

  name.value = 'March figures';
  await choose(2);
  assert.equal(name.value, 'March figures', 'a second extension was appended to a name typed without one');

  name.value = 'March.figures.final';
  await choose(0);
  assert.equal(name.value, 'March.figures.final', 'a typed name with an extension of its own was rewritten');

  // The bytes are still the selected format's, whatever the name says.
  await save();
  assert.deepEqual(written.map((w) => [w.name, w.bytes]), [['March.figures.final', 'TABLE-AS-xlsx']]);
});

// ── Form data ───────────────────────────────────────────────────────────────
test('the form export offers three formats; csv is fetched at the press and the others when saved', async () => {
  written.length = 0;
  const from = calls.length;
  $('exportFormBtn').click();
  await settle();
  assert.deepEqual(optionLabels(), ['Comma-separated values (.csv)', 'JSON (.json)', 'XFDF — for Acrobat and Foxit (.xfdf)']);
  assert.equal($('saveAsName').value, 'ledger-form.csv');
  assert.deepEqual(requests('/api/form-data', from).map((c) => c.url.split('format=')[1]), ['csv']);

  await choose(2);
  assert.equal($('saveAsName').value, 'ledger-form.xfdf');
  await save();
  assert.deepEqual(written.map((w) => [w.name, w.bytes]), [['ledger-form.xfdf', 'FORM-AS-xfdf']],
    'the file written is not the selected format\'s — the csv fetched at the press went out under the xfdf name, or the reverse');
  assert.equal(requests('/api/form-data', from).every((c) => c.headers['X-Nib-Doc'] === 'sf:ledger.pdf'), true,
    'a form-data request did not name the document');
});

test('switching back to the default writes the default\'s bytes, not the last one made', async () => {
  written.length = 0;
  $('exportFormBtn').click();
  await settle();
  // Make json fail-free but UNSAVED is impossible — a maker runs only at Save — so save json to a
  // name that exists and decline the replace: json's bytes are now made and held, the dialog open.
  await choose(1);
  existing = new Set(['ledger-form.json']);
  h.setConfirmAnswer(false);
  await save();
  h.setConfirmAnswer(true);
  assert.equal(written.length, 0, 'setup: the declined replace wrote the file anyway');
  assert.equal($('saveAsModal').hidden, false, 'setup: declining the replace closed the dialog');
  await choose(0);
  await save();
  assert.deepEqual(written.map((w) => [w.name, w.bytes]), [['ledger-form.csv', 'FORM-AS-csv']]);
});

test('a document with no form is refused before any dialog, in the server\'s words', async () => {
  const from = calls.length;
  formAnswer = new Response(JSON.stringify({ error: 'this document has no form fields' }), { status: 422, headers: { 'Content-Type': 'application/json' } });
  $('exportFormBtn').click();
  await settle();
  assert.match(toastText(), /no form fields/);
  assert.equal($('saveAsModal').hidden, true, 'a document with no form opened the save dialog');
  assert.equal(requests('/api/form-data', from).length, 1);
});

test('the 412 replace flow still asks, and writes the selected format when the answer is yes', async () => {
  written.length = 0;
  $('exportFormBtn').click();
  await settle();
  await choose(1);
  existing = new Set(['ledger-form.json']);
  const asked = h.confirms.length;
  await save();
  assert.equal(h.confirms.length, asked + 1, 'a name already in the folder was not asked about');
  assert.match(h.confirms.at(-1), /ledger-form\.json already exists/);
  assert.deepEqual(written.map((w) => [w.name, w.bytes, w.overwrite]), [['ledger-form.json', 'FORM-AS-json', true]]);
});

test('the export is of the document open at the press, whatever is active at Save', async () => {
  await open('other.pdf');
  const [ledger, other] = ['ledger.pdf', 'other.pdf'].map((n) => tabs().find((t) => t.textContent.includes(n)));
  ledger.click();
  await settle();
  written.length = 0;
  const from = calls.length;
  $('exportFormBtn').click();
  await settle();
  other.click();                      // the user looks at the other document with the dialog open
  await settle();
  assert.equal(doc.querySelector('#tabstrip .tab.active').textContent.includes('other.pdf'), true, 'setup: the switch did not happen');
  await choose(1);
  await save();
  assert.deepEqual(requests('/api/form-data', from).map((c) => c.headers['X-Nib-Doc']), ['sf:ledger.pdf', 'sf:ledger.pdf'],
    'a format fetched at Save named the document active THEN, not the one the export was pressed for');
  assert.equal(written[0]?.name, 'ledger-form.json', 'the file is named for the document active at Save');
  other.querySelector('.tabclose').click();
  await settle();
  assert.equal(tabs().length, 1, 'cleanup: the second document is still open');
});

// ── Pages as images ─────────────────────────────────────────────────────────
// A thenable the stub's getData() awaits: it counts each render that STARTED and holds them all.
function renderGate() {
  const waiting = [];
  return { started: () => waiting.length, release: () => waiting.splice(0).forEach((r) => r()), then(r) { waiting.push(r); } };
}

test('pages as images offers this page or every page, named for each', async () => {
  const from = calls.length;
  $('exportPagesBtn').click();
  await settle();
  assert.deepEqual(optionLabels(), ['This page (PNG)', 'Every page (ZIP)']);
  assert.equal($('saveAsTitle').textContent, 'Export pages as images');
  assert.equal($('saveAsName').value, 'ledger-page1.png');
  await choose(1);
  assert.equal($('saveAsName').value, 'ledger-pages.zip', 'the name did not become the every-page name');
  await choose(0);
  assert.equal($('saveAsName').value, 'ledger-page1.png');
  assert.equal(requests('/api/assemble', from).length, 0, 'the every-page render ran before Save was pressed');
});

test('a render in progress says Preparing once, holds Save and the format, and a second press starts nothing', async () => {
  written.length = 0;
  $('exportPagesBtn').click();
  await settle();
  await choose(1);
  const gate = renderGate();
  setDataGate(gate);
  await save();
  assert.equal(gate.started(), 1, 'stimulus: Save started no render');
  assert.equal($('saveAsStatus').textContent, 'Preparing…');
  assert.equal($('saveAsGo').disabled, true, 'Save is still pressable while the pictures are being made');
  assert.equal($('saveAsFormat').disabled, true, 'the format can be changed under the bytes being made');
  // The button is disabled, so drive the handler itself: the guard must hold without the attribute.
  // Not awaited: with the guard gone this call would wait on the held render, and the test must
  // fail on the count below rather than hang.
  $('saveAsGo').onclick();
  await settle();
  assert.equal(gate.started(), 1, 'a second press started a second render');

  // jsdom cannot draw, so the render FAILS once released — which is the failure path.
  gate.release();
  setDataGate(null);
  await settle(30);
  assert.equal(written.length, 0, 'a render that failed wrote a file');
  assert.equal($('saveAsModal').hidden, false, 'the dialog closed on a failed render');
  assert.notEqual($('saveAsStatus').textContent, 'Preparing…', 'the dialog still says it is working after the render failed');
  assert.notEqual($('saveAsStatus').textContent, '', 'a failed render said nothing');
  assert.equal($('saveAsGo').disabled, false, 'Save was left disabled after the failure');
  assert.equal($('saveAsFormat').disabled, false, 'the format was left disabled after the failure');
});

test('a render that outlives a cancelled dialog writes nothing and does not touch the next one', async () => {
  written.length = 0;
  $('exportPagesBtn').click();
  await settle();
  const gate = renderGate();
  setDataGate(gate);
  await save();
  assert.equal(gate.started(), 1, 'stimulus: Save started no render');
  $('saveAsCancel').click();
  await settle();
  // A different export opens while the first render is still held.
  $('exportTableBtn').click();
  await settle();
  assert.equal($('saveAsGo').disabled, false, 'the new dialog opened with Save disabled by the old render');
  assert.equal($('saveAsStatus').textContent, '', 'the new dialog opened saying Preparing…');
  gate.release();
  setDataGate(null);
  await settle(30);
  assert.equal(written.length, 0, 'the cancelled export wrote a file');
  assert.equal($('saveAsStatus').textContent, '', 'the old render\'s failure was written into the new dialog');
  assert.equal($('saveAsGo').disabled, false);
  assert.equal($('saveAsName').value, 'ledger-p1-table.xlsx', 'the new dialog\'s name was disturbed');
});

test('a document closed with the dialog open is not exported — and neither is the one opened in its place', async () => {
  written.length = 0;
  $('exportPagesBtn').click();
  await settle();
  // The last document: closing it resets the view record, and the next open REUSES that record.
  doc.querySelector('#tabstrip .tab.active .tabclose').click();
  await settle();
  assert.equal($('viewerWrap').className, '', 'setup: the document did not close');
  await open('stranger.pdf');
  assert.equal(tabs().length, 1, 'setup: the second document did not open into the first one\'s place');
  const gate = renderGate();
  setDataGate(gate);
  await save();
  assert.equal(gate.started(), 0, 'a render started for a document that was not the one the export was pressed for');
  assert.equal($('saveAsStatus').textContent, 'That document is no longer open');
  assert.equal($('saveAsModal').hidden, false);
  assert.equal(written.length, 0);
});
