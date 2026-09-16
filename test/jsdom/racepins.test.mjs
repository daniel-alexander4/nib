// /pending 498 — the document-switch races that sat behind the pinning guard.
//
// `pinning.test.mjs` asks which document a REQUEST names. It cannot see the other half of ADR-001:
// an operation that captures nothing, awaits, and then reads the live `view` — to bake, to write
// marks, to open a dialog, or to append a verdict. Every site below did that. Two are driven here,
// because their failure is a thing a user sees or loses:
//
//   * two opens overlapping, where the second used to load into the view the first had claimed,
//     orphaning the first document server-side with no tab;
//   * the signature-details panel, which appended one document's completeness verdict under
//     another document's signers.
//
// The rest are held to one shape by a scan — capture before the first await, and no live-view read
// after it — because driving each needs a canvas, a text layer or a live session this tier does not
// have. The scan is the weaker instrument and says so: it proves the SHAPE, not the behaviour.
//
// ## What this cannot see
//
// - Any timing a real browser produces and jsdom does not: pdf.js load times, a real co-sign
//   arrival poll. The gates below MAKE the overlap; tier 3 is where it happens by itself.
// - The co-sign bake (`cosign`) behaviourally: its attestation render needs a canvas. Held by the
//   shape scan only.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { boot, REPO } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const APP = fs.readFileSync(path.join(REPO, 'web', 'app.js'), 'utf8');

const SIGNED = { state: 'valid', signers: [{ name: 'Alice', valid: true }] };
const meta = (id, name) => ({
  id, name, path: '/tmp/nib-harness/' + name, canSave: true,
  signature: SIGNED, canUndo: false, canRedo: false,
});

// A queue of held responses for /api/attestations, released one at a time by the test.
const held = [];
const h = await boot({
  routes: {
    '/api/open': (opts) => {
      const { path: p } = JSON.parse(opts.body);
      return p.endsWith('a.pdf') ? meta('rp:1', 'a.pdf') : meta('rp:2', 'b.pdf');
    },
    '/api/attestations': (opts) => new Promise((resolve) => {
      held.push({ doc: opts.headers['X-Nib-Doc'], release: resolve });
    }),
  },
});
const { document: doc, calls, settle } = h;
const tabs = () => [...doc.querySelectorAll('#tabstrip .tab')];

test('two opens overlapping each get their own tab — neither loads over the other', async () => {
  let release;
  setNextDocument({ numPages: 2, gate: new Promise((r) => { release = r; }) });
  const input = doc.getElementById('pathInput');
  input.value = '/tmp/nib-harness/a.pdf';
  doc.getElementById('openGo').click();
  await settle();
  input.value = '/tmp/nib-harness/b.pdf';
  doc.getElementById('openGo').click();
  await settle();
  // Stimulus: both opens reached the server and both loads are held open together.
  assert.equal(calls.filter((c) => c.url.includes('/api/open')).length, 2, 'setup: both opens did not go out');
  release();
  await settle(30);
  setNextDocument({ numPages: 2 });

  const names = tabs().map((t) => t.querySelector('.tabname').textContent).sort();
  assert.deepEqual(names, ['a.pdf', 'b.pdf'],
    'the second open loaded into the view the first was still loading — one document has no tab, and the server still holds it');
});

test('a signature-details panel never shows another document\'s verdicts', async () => {
  assert.equal(tabs().length, 2, 'setup: the previous test did not leave two documents open');
  const body = doc.getElementById('sigDetailsBody');
  const activeName = () => doc.querySelector('#tabstrip .tab.active .tabname').textContent;

  // Open the panel on the active document; its verdict request is held.
  const first = activeName();
  doc.getElementById('sigDetailsBtn').click();
  await settle();
  assert.equal(held.length, 1, 'setup: the panel did not ask the server for its verdicts');

  // Switch to the other document (this closes the panel) and open the panel there.
  tabs().find((t) => t.querySelector('.tabname').textContent !== first).click();
  await settle();
  assert.notEqual(activeName(), first, 'setup: the switch did not happen');
  doc.getElementById('sigDetailsBtn').click();
  await settle();
  assert.equal(held.length, 2, 'setup: the second panel did not ask for its verdicts');
  // A setup check, not the pin's test: the verdict request leaves synchronously while its document
  // is still active, so the implicit header is already right, and removing the explicit `docId`
  // was probed and stays green here. The defect this file drives is the APPEND, below.
  assert.notEqual(held[0].doc, held[1].doc, 'setup: the two panels did not ask about two different documents');

  // The FIRST document's answer arrives late, into the second document's panel.
  held[0].release({ attestations: [], obliged: 3, signed: 1 });
  await settle();
  assert.ok(!/Incomplete/.test(body.textContent),
    'the first document\'s "Incomplete — 1 of 3" verdict was appended under the second document\'s signers');

  // And the second document's own answer still lands — the guard must not drop everything.
  held[1].release({ attestations: [], obliged: 2, signed: 2 });
  await settle();
  assert.match(body.textContent, /Complete — all 2 obliged/,
    'the panel\'s own verdict was dropped too, so the guard above passes by showing nothing');
});

// ── The shape scan ──────────────────────────────────────────────────────────────────────────────
//
// Each named site must capture `const owner = view;` before its first await, and after that await
// read nothing that resolves the live view: `view.`, and the helpers that default to it when called
// without an owner (`bakedForm()`, `exportBase()`, one-argument `scanTextMatches`/`pageTextItems`).
// `owner !== view` is the one permitted mention — it is the check, not a read.
const LIVE_READS = [/\bview\./, /\bbakedForm\(\)/, /\bexportBase\(\)/, /\bscanTextMatches\([^,()]*\)/, /\bpageTextItems\([^,()]*\)/];

function bodyOf(src, header) {
  const at = src.indexOf(header);
  if (at === -1) return null;
  const open = src.indexOf('{', at + header.length - 1);
  let d = 0;
  for (let j = open; j < src.length; j++) {
    if (src[j] === '{') d++;
    else if (src[j] === '}') { d--; if (d === 0) return src.slice(open, j + 1); }
  }
  return null;
}
const stripComments = (s) => s.replace(/\/\*[\s\S]*?\*\//g, ' ').split('\n').map((l) => l.replace(/(^|[^:])\/\/.*$/, '$1')).join('\n');

function liveReadsAfterAwait(body) {
  const code = stripComments(body);
  const cap = code.indexOf('const owner = view;');
  const firstAwait = code.search(/\bawait\b/);
  const problems = [];
  if (firstAwait === -1) problems.push('no await — the site is not the shape this scan is about');
  if (cap === -1 || (firstAwait !== -1 && cap > firstAwait)) problems.push('no `const owner = view;` before the first await');
  const tail = code.slice(firstAwait === -1 ? code.length : firstAwait);
  for (const re of LIVE_READS) {
    const m = re.exec(tail);
    if (m) problems.push(`reads the live view after an await: ${m[0]}`);
  }
  return problems;
}

const SITES = [
  'async function cosign() {',
  'async function openOutlineEditor() {',
  'async function openSplit() {',
  'async function openBookmarkSplit() {',
  'els.saveFillableBtn.onclick = async () => {',
  'els.autofillBtn.onclick = async () => {',
  'els.rtFind.onclick = async () => {',
  'els.applyBoxSplitBtn.onclick = async () => {',
];

test('the shape scan detects a live-view read after an await — its own stimulus', () => {
  const bad = [
    'async function late() {\n  const x = await thing();\n  view.redactMarks.push(x);\n}',
    'async function late() {\n  const owner = view;\n  await thing();\n  const f = await bakedForm();\n}',
    'async function late() {\n  await thing();\n  const owner = view;\n}',
  ];
  for (const b of bad) {
    assert.ok(liveReadsAfterAwait(bodyOf(b, 'async function late() {')).length > 0,
      `the shape scan passed a site that reads the live view after awaiting:\n${b}`);
  }
  const good = 'async function ok() {\n  const owner = view;\n  const f = await bakedForm(owner);\n  if (owner !== view) return;\n  owner.x = f;\n}';
  assert.deepEqual(liveReadsAfterAwait(bodyOf(good, 'async function ok() {')), [],
    'the shape scan flags a correctly captured site, so it cannot be satisfied');
});

test('every listed site captures its document before awaiting and never reads the live view after', () => {
  const failures = [];
  for (const header of SITES) {
    const body = bodyOf(APP, header);
    if (!body) { failures.push(`${header} — not found in app.js (renamed? update SITES)`); continue; }
    for (const p of liveReadsAfterAwait(body)) failures.push(`${header} — ${p}`);
  }
  assert.deepEqual(failures, [], `a document-switch race: the operation acts on whichever document is active when its await returns\n  ${failures.join('\n  ')}`);
});

// A dialog opener reads nothing of the live view after its await — and is still wrong if it opens:
// the dialog it unhides acts on the ACTIVE document (a switch closes doc-bound dialogs, so the one
// open at the click is the active one). So each must check the switch between its last await and
// the unhide. Found by probing the scan above: deleting openSplit's check left it green.
const OPENERS = [
  ['async function openOutlineEditor() {', 'els.outlineModal.hidden = false'],
  ['async function openSplit() {', 'els.splitModal.hidden = false'],
  ['async function openBookmarkSplit() {', 'els.bookmarkSplitModal.hidden = false'],
  ['els.saveFillableBtn.onclick = async () => {', 'els.fieldNameModal.hidden = false'],
];
test('every dialog opener that awaits checks for a switch before it opens', () => {
  const failures = [];
  for (const [header, unhide] of OPENERS) {
    const body = stripComments(bodyOf(APP, header) || '');
    const open = body.indexOf(unhide);
    if (!body || open === -1) { failures.push(`${header} — not found, or it no longer opens ${unhide}`); continue; }
    const before = body.slice(0, open);
    const lastAwait = [...before.matchAll(/\bawait\b/g)].pop();
    if (!lastAwait) { failures.push(`${header} — no await before it opens; not this shape`); continue; }
    if (!before.slice(lastAwait.index).includes('if (owner !== view) return;')) {
      failures.push(`${header} — opens after an await with no switch check`);
    }
  }
  assert.deepEqual(failures, [],
    `a dialog built for one document opens over another, and its Go acts on the one on screen\n  ${failures.join('\n  ')}`);
});

// Split-by-box hands off to pageOp, which captures `view` at ITS entry and confirms signature loss
// against it — so the check must sit between this handler's await and that call.
test('split-by-box stops before pageOp when the document changed during its await', () => {
  const body = stripComments(bodyOf(APP, 'els.applyBoxSplitBtn.onclick = async () => {'));
  const firstAwait = body.search(/\bawait\b/);
  const guard = body.indexOf('if (owner !== view) return;');
  const op = body.indexOf('pageOp(');
  assert.ok(firstAwait !== -1 && op !== -1, 'setup: split-by-box has no await or no pageOp call');
  assert.ok(guard > firstAwait && guard < op,
    'split-by-box calls pageOp after an await with no switch check — pageOp then splits the active document with this one\'s regions');
});

// The two sites whose await is not in a capturing function: the verdicts append and the rescue.
test('the late verdict append and the co-sign rescue are tied to their own document', () => {
  const aug = stripComments(bodyOf(APP, 'async function augmentSigDetails(rows, owner = view, seq = sigDetailsSeq) {') || '');
  assert.ok(aug, 'setup: augmentSigDetails is not found under its pinned signature');
  const firstAwait = aug.search(/\bawait\b/);
  const guard = aug.search(/if \(seq !== sigDetailsSeq \|\| owner !== view \|\| els\.sigDetailsModal\.hidden\) return;/);
  const firstAppend = aug.search(/appendChild/);
  assert.ok(guard > firstAwait && guard < firstAppend,
    'augmentSigDetails appends verdicts after its await without checking the panel is still the one it was building');

  const notice = stripComments(bodyOf(APP, 'function reflectNotice(n) {'));
  assert.match(notice, /const rescuable = n\.what === 'signed-not-saved';/,
    'a notice with no document in any tab offers "Save a copy…", which saves the user\'s own document instead');
  const rescue = stripComments(bodyOf(APP, 'els.sessionNoticeAction.onclick = async () => {'));
  assert.ok(!/\bview\b(?!s)/.test(rescue.replace(/\bviews\b/g, '')),
    'the rescue reads the active view, so after a tab switch it saves the wrong document under -cosigned.pdf');
  assert.match(APP, /if \(meta\.id\) rescueDocId = meta\.id;/, 'the arrival never records which document the rescue should save');
});
