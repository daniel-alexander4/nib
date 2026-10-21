// Save As… (File → Save a Copy) — the document saved under its own name, in a folder the user
// chooses, unchanged if the user has not changed it (Dan, 2026-10-21: a PDF that came by email is
// often to be kept exactly as it arrived).
//
// What is held here:
//
//   * the dialog offers the file's own name, exactly as it was opened, and a typed name is used;
//   * with nothing edited, the bytes written are the bytes loaded — nothing baked, nothing posted to
//     the bake or the signing-flags route, even for a document that carries signing flags;
//   * a name that is not a PDF's (a photo opened as a PDF) is offered with ".pdf" in its place;
//   * Save on a document opened with no folder of its own opens this dialog under its own name,
//     where it used to download "document.pdf".
//
// ## What this cannot see
//
// - That the server writes those bytes as posted: `/api/write` is stubbed. The server half is
//   `holdsBytes` in internal/server/saveas.go, held by the Go tests.
// - A real pdf.js `getData()`: the stub answers the bytes the test gave it.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const written = []; // every /api/write: { dir, name, bytes }
let pathless = false;
let flags = null;

const h = await boot({
  routes: {
    '/api/listdir': { path: '/home/someone/nib', parent: '/home/someone', dirs: [], files: [] },
    '/api/open': (opts) => {
      const { path: p } = JSON.parse(opts.body);
      const name = p.split('/').pop();
      const meta = { id: 'sa:' + name, name, path: pathless ? '' : p, canSave: !pathless, signature: { state: 'unsigned' }, canUndo: false, canRedo: false };
      if (flags) meta.flags = flags;
      return meta;
    },
    '/api/close': { name: '', path: '', canSave: false, signature: { state: '' }, canUndo: false, canRedo: false },
    '/api/write': async (opts) => {
      const f = opts.body;
      written.push({ dir: f.get('dir'), name: f.get('name'), bytes: await bytesOf(f.get('data')) });
      return { path: f.get('dir') + '/' + f.get('name') };
    },
  },
});
const { document: doc, settle } = h;
const $ = (id) => doc.getElementById(id);

// The same harness seam saveformats.test.mjs states: jsdom's FormData takes only its own Blob.
globalThis.FormData = class {
  #f = new Map();
  append(k, v) { if (!this.#f.has(k)) this.#f.set(k, v); }
  get(k) { return this.#f.has(k) ? this.#f.get(k) : null; }
};
function bytesOf(b) {
  if (typeof b.arrayBuffer === 'function') return b.arrayBuffer().then((a) => [...new Uint8Array(a)]);
  return new Promise((resolve, reject) => {
    const r = new h.window.FileReader();
    r.onload = () => resolve([...new Uint8Array(r.result)]);
    r.onerror = () => reject(r.error);
    r.readAsArrayBuffer(b);
  });
}

const RECEIVED = [...'%PDF-1.7 as it came by email %%EOF'].map((c) => c.charCodeAt(0));

async function open(name, opts = {}) {
  setNextDocument({ numPages: 1, text: 'Invoice', data: RECEIVED, ...opts });
  $('pathInput').value = '/tmp/nib-harness/' + name;
  $('openGo').click();
  await settle();
}
async function close() {
  if (!$('saveAsModal').hidden) { $('saveAsCancel').click(); await settle(); }
}
const callsTo = (route, from) => h.calls.slice(from).filter((c) => c.url.includes(route)).length;

test('Save As offers the file its own name and writes it byte for byte when nothing was changed', async () => {
  await open('Invoice March.PDF');
  const from = h.calls.length;
  $('saveAsBtn').click();
  await settle();
  assert.equal($('saveAsModal').hidden, false, 'Save As did not open the Save dialog');
  assert.equal($('saveAsName').value, 'Invoice March.PDF', 'the dialog did not offer the name the file was opened with');
  $('saveAsGo').click();
  await settle();
  assert.equal(written.length, 1, 'Save wrote nothing');
  assert.equal(written[0].name, 'Invoice March.PDF');
  assert.deepEqual(written[0].bytes, RECEIVED, 'the bytes written are not the bytes the document was opened with');
  assert.equal(callsTo('/api/bake', from), 0, 'an unchanged document was sent to be baked');
  written.length = 0;
  await close();
});

test('the name can be changed in the dialog, and the typed name is the one written', async () => {
  await open('statement.pdf');
  $('saveAsBtn').click();
  await settle();
  $('saveAsName').value = 'Bank statement October.pdf';
  $('saveAsGo').click();
  await settle();
  assert.equal(written.length, 1);
  assert.equal(written[0].name, 'Bank statement October.pdf', 'the typed name was not used');
  assert.deepEqual(written[0].bytes, RECEIVED);
  written.length = 0;
  await close();
});

test('a document carrying signing flags is saved unchanged — the flags are not stripped from a file nobody changed', async () => {
  flags = [{ kind: 'sign', page: 1, x: 0.1, y: 0.1, w: 0.2, h: 0.05 }];
  try {
    await open('to-sign.pdf');
    const from = h.calls.length;
    $('saveAsBtn').click();
    await settle();
    $('saveAsGo').click();
    await settle();
    assert.equal(callsTo('/api/flags', from), 0, 'Save As rewrote the file to strip its signing flags');
    assert.equal(written.length, 1);
    assert.deepEqual(written[0].bytes, RECEIVED);
  } finally {
    flags = null;
    written.length = 0;
    await close();
  }
});

test('a name that is not a PDF\'s is offered with .pdf in place of its extension', async () => {
  await open('receipt photo.png');
  $('saveAsBtn').click();
  await settle();
  assert.equal($('saveAsName').value, 'receipt photo.pdf');
  await close();
});

test('Save on a document with no folder of its own opens Save As under its own name, not a "document.pdf" download', async () => {
  pathless = true;
  try {
    await open('From the bank.pdf');
    $('saveBtn').click();
    await settle();
    assert.equal($('saveAsModal').hidden, false, 'Save on a pathless document did not open the Save dialog');
    assert.equal($('saveAsName').value, 'From the bank.pdf');
  } finally {
    pathless = false;
    await close();
  }
});
