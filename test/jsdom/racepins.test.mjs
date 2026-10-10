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
import { setNextDocument, setDataGate } from './stub-pdfjs.mjs';

const APP = fs.readFileSync(path.join(REPO, 'web', 'app.js'), 'utf8');

const SIGNED = { state: 'valid', signers: [{ name: 'Alice', valid: true }] };
const meta = (id, name) => ({
  id, name, path: '/tmp/nib-harness/' + name, canSave: true,
  signature: SIGNED, canUndo: false, canRedo: false,
});

// A queue of held responses for /api/attestations, released one at a time by the test.
const held = [];
const heldLists = []; // /api/attachments, likewise
const h = await boot({
  routes: {
    '/api/open': (opts) => {
      const { path: p } = JSON.parse(opts.body);
      return p.endsWith('a.pdf') ? meta('rp:1', 'a.pdf') : meta('rp:2', 'b.pdf');
    },
    '/api/attestations': (opts) => new Promise((resolve) => {
      held.push({ doc: opts.headers['X-Nib-Doc'], release: resolve });
    }),
    // /pending 806 and 625: a list held until the test releases it, and the two routes whose request
    // must name the document the operation began on.
    '/api/attachments': (opts) => new Promise((resolve) => {
      heldLists.push({ doc: opts.headers['X-Nib-Doc'], release: resolve });
    }),
    '/api/attachments/extract': () => new Response('bytes', { status: 200 }),
    '/api/split-pages': () => ({ count: 1, dir: '/tmp/nib-harness/out' }),
    '/api/listdir': () => ({ path: '/tmp/nib-harness', parent: '', dirs: [], files: [] }),
  },
});
const { document: doc, calls, settle } = h;
const tabs = () => [...doc.querySelectorAll('#tabstrip .tab')];
const activeName = () => doc.querySelector('#tabstrip .tab.active .tabname').textContent;
const ID_OF = { 'a.pdf': 'rp:1', 'b.pdf': 'rp:2' }; // what the /api/open stub answers

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

// /pending 806. An attachment id means something only against the document that listed it — the
// two lists below give the SAME id, which is the real shape (`page:1:0` on every document) — so
// a list answered late must not render into the other document's dialog, and an Extract must name
// the document its row came from.
test('an attachment list answered after a switch never feeds the other document\'s Extract', async () => {
  assert.equal(tabs().length, 2, 'setup: two documents are not open');
  const body = doc.getElementById('attachBody');
  const first = activeName();
  doc.getElementById('attachBtn').click();
  await settle();
  assert.equal(heldLists.length, 1, 'setup: the dialog did not ask for the first document\'s list');

  tabs().find((t) => t.querySelector('.tabname').textContent !== first).click();
  await settle();
  doc.getElementById('attachBtn').click();
  await settle();
  assert.equal(heldLists.length, 2, 'setup: the reopened dialog did not ask for the second document\'s list');
  assert.notEqual(heldLists[0].doc, heldLists[1].doc, 'setup: the two lists did not ask about two documents');

  // The second document answers first; the first document's answer lands after it.
  heldLists[1].release({ attachments: [{ id: 'page:1:0', name: 'second.txt' }] });
  await settle();
  heldLists[0].release({ attachments: [{ id: 'page:1:0', name: 'first.txt' }] });
  await settle();
  assert.ok(!/first\.txt/.test(body.textContent),
    'the first document\'s late list rendered into the second document\'s dialog — its ids now extract from the wrong file');
  assert.match(body.textContent, /second\.txt/, 'the dialog\'s own list was dropped too');

  // And the Extract names the document whose list it came from.
  const before = calls.length;
  body.querySelector('.attachrow button').click();
  await settle();
  const ex = calls.slice(before).find((c) => c.url.includes('/api/attachments/extract'));
  assert.ok(ex, 'setup: Extract sent nothing');
  assert.equal(ex.headers['X-Nib-Doc'], heldLists[1].doc, 'Extract is not addressed to the document that listed the id');
  doc.getElementById('saveAsModal').hidden = true;
  doc.getElementById('attachmentsModal').hidden = true;
});

test('an Extract names the document its row was listed from, even after a switch', async () => {
  const body = doc.getElementById('attachBody');
  doc.getElementById('attachBtn').click();
  await settle();
  const listed = heldLists[heldLists.length - 1];
  listed.release({ attachments: [{ id: 'page:1:0', name: 'listed.txt' }] });
  await settle();
  const btn = body.querySelector('.attachrow button');
  assert.ok(btn, 'setup: the list rendered no Extract button');
  // An arrival switches the tab while the row is on screen; the click is already on its way.
  const active = activeName();
  tabs().find((t) => t.querySelector('.tabname').textContent !== active).click();
  await settle();
  const before = calls.length;
  btn.click();
  await settle();
  const ex = calls.slice(before).find((c) => c.url.includes('/api/attachments/extract'));
  assert.ok(ex, 'setup: Extract sent nothing');
  assert.equal(ex.headers['X-Nib-Doc'], listed.doc,
    'Extract sent another document\'s id with this list\'s attachment id — `page:1:0` names a file in every document');
  doc.getElementById('saveAsModal').hidden = true;
});

// /pending 625. The split's only guard against a part overwriting its SOURCE is the addressed
// document's path, so a switch during the bake must not move the request to the other document.
test('a page split whose bake straddles a switch is addressed to the document it baked', async () => {
  const startName = activeName();
  const startId = ID_OF[startName];
  assert.ok(startId, `setup: no id known for ${startName}`);
  doc.getElementById('exportPageSplitBtn').click();
  await settle();
  doc.getElementById('psDir').value = '/tmp/nib-harness/out';
  let release;
  setDataGate(new Promise((r) => { release = r; }));
  const before = calls.length;
  try {
    doc.getElementById('psGo').click();
    await settle();
    // Mid-bake: an arrival activates the other tab.
    tabs().find((t) => t.querySelector('.tabname').textContent !== startName).click();
    await settle();
    assert.notEqual(activeName(), startName, 'setup: the switch did not happen');
    assert.ok(!calls.slice(before).some((c) => c.url.includes('/api/split-pages')), 'setup: the bake was not held');
  } finally {
    setDataGate(null);
    release();
  }
  await settle(20);
  const sp = calls.slice(before).find((c) => c.url.includes('/api/split-pages'));
  assert.ok(sp, 'setup: the split was never sent');
  assert.equal(sp.headers['X-Nib-Doc'], startId,
    'the split was addressed to the document active when the bake FINISHED — its bytes are the other one\'s, and the overwrite guard checked the wrong path');
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
  // /pending 625: the two folder splits, whose server-side overwrite guard is the addressed path.
  'async function pageSplitGo() {',
  'async function bookmarkSplitGo() {',
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

// ── The census (/pending 671) ──────────────────────────────────────────────────────────────
//
// SITES above is a list somebody keeps, so a new async handler that awaits and then reads the live view
// was checked by nothing: two such sites were live when this was written, neither listed. This walks
// EVERY `async` body in app.js instead — any line that opens one, nested or not — and a body that reads
// the live view on a line after its first await must be named here with the reason that read is right.
// A read on the first await's own line is not counted: `await bakedForm()` and
// `await pageOp(…, { page: view.viewer.currentPageNumber })` evaluate it before anything is awaited.
//
// Weaker than SITES on purpose: it does not ask for `const owner = view;`, because most async bodies
// never touch a document. What it cannot see: a live read reached through a helper not in LIVE_READS,
// and a body whose header line does not end in `{` (none today — the floor below would not notice one).
const LIVE_BY_DESIGN = {
  'async function openSessionInit() {': 'opens a dialog whose Go acts on the ACTIVE document, and words it for that one — "the document that is open NOW"',
  'async function reconcileWithServer() {': 'a session question, not a document operation: it compares the server\'s tab set with whatever is on screen when it answers',
  'async function openURL(url) {': 'reads the view only after installOpened succeeded, which made the opened document the active one',
  'async function ensureAlignment() {': 'every await is followed by `if (seq !== cmpSeq) return;`, and a switch closes Compare, which bumps cmpSeq',
  'async function renderCompareVisual(mode) {': 'as ensureAlignment — the cmpSeq token is its pin',
  'async function save() {': 'captures `owner`; the two later reads are the check itself (`view.docMeta.id !== doc.id`) and one made only once that check has passed',
  'async function runOCR(cmd = null) {': 'the one late read restores the OCR BUTTON, which belongs to whichever document is in front when the read ends — its lock, not the lock of the document that was read (/pending 830); the words themselves go to the captured owner',
  'async function loadImages() {': 'the read is inside a card\'s click handler, which runs at the click — placing an image on the document then in front is the intent',
};
function asyncBodies(src) {
  const out = [];
  let off = 0;
  for (const line of src.split('\n')) {
    if (/\basync\b.*\{\s*$/.test(line) && !/^\s*\/\//.test(line)) {
      let d = 0;
      for (let j = off + line.lastIndexOf('{'); j < src.length; j++) {
        if (src[j] === '{') d++;
        else if (src[j] === '}' && --d === 0) { out.push([line.trim(), src.slice(off + line.lastIndexOf('{'), j + 1)]); break; }
      }
    }
    off += line.length + 1;
  }
  return out;
}
function lateLiveReads(body) {
  const lines = stripComments(body).split('\n');
  const first = lines.findIndex((l) => /\bawait\b/.test(l));
  if (first === -1) return [];
  return lines.slice(first + 1).filter((l) => LIVE_READS.some((re) => re.test(l))).map((l) => l.trim());
}
test('the census detects a late live read in a body nobody listed — its own stimulus', () => {
  const src = 'function a() {}\nels.x.onclick = async () => {\n  const r = await thing(view.id);\n  view.marks.push(r);\n};\n'
    + 'async function fine() {\n  const owner = view;\n  await thing();\n  owner.x = 1;\n}\n';
  const found = asyncBodies(src).map(([h, b]) => [h, lateLiveReads(b)]);
  assert.deepEqual(found, [['els.x.onclick = async () => {', ['view.marks.push(r);']], ['async function fine() {', []]]);
});
test('no async body reads the live view after awaiting, unless it is named with its reason', () => {
  const bodies = asyncBodies(APP);
  assert.ok(bodies.length >= 200, `the census found ${bodies.length} async bodies in app.js, where there were 231 — it has stopped seeing them`);
  const failures = [];
  const hit = new Set();
  for (const [header, body] of bodies) {
    const late = lateLiveReads(body);
    if (!late.length) continue;
    hit.add(header);
    if (!LIVE_BY_DESIGN[header]) failures.push(`${header} — ${late[0]}`);
  }
  for (const header of Object.keys(LIVE_BY_DESIGN)) {
    if (!hit.has(header)) failures.push(`${header} — named as reading the live view by design, and it no longer does (or was renamed): drop the row`);
  }
  assert.deepEqual(failures, [],
    `an operation acts on whichever document is active when its await returns — capture \`const owner = view;\` first, or name the body in LIVE_BY_DESIGN with why the live read is right\n  ${failures.join('\n  ')}`);
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
