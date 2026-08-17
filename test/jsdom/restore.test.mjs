// P06.S03 — the client learns what the server holds.
//
// Its own file because the restore runs at module-evaluation time, inside applyStatus,
// and this tier's standing rule is one boot per file: `web/app.js` wires itself when it
// is imported and ES modules are cached per process, so a second boot gets the module
// bound to the FIRST document.
//
// ── What this tier can reach ─────────────────────────────────────────────────
// Everything about the reconciliation, because all of it is client behaviour driven by
// what the server answers, and answering is what a stub does. That includes the
// all-tabs-stale case carried from P03: the pin says to observe it "by restarting the
// server under a client holding ≥2 ids", and a restart's client-visible effect is
// exactly this — every id 409s (docFor refuses a foreign epoch before it compares
// anything else) and /api/docs reports a different set. **The restart is the pin's
// SETUP, not its subject.**
//
// ── What it cannot ───────────────────────────────────────────────────────────
// The restart itself. Tier 3's nib process is shared across test files and owned by
// build/uirepro.sh, so restarting it mid-run would break the sibling files. What is
// therefore unobserved is the Go side of a restart — that a fresh process really does
// mint a new epoch and refuse every old id — and that half lives in the Go suite
// (docid_test.go), not in a browser.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const A = { id: 'test-epoch:1', name: 'alpha.pdf', path: '/tmp/alpha.pdf', canSave: true, signature: { state: 'unsigned' }, canUndo: false, canRedo: false };
// B is PATH-LESS on purpose — an upload, a combine, an office conversion or an arrival.
// It is the case the acceptance calls out, and the one that mattered most before this
// slice: with no boot restore, a document with no path was unreachable for the rest of
// the process's life, because the only way back to a document was to open it by path.
// It also exercises the strip's name fallback, since docResponse.Name is "." —
// filepath.Base("") — for every path-less document.
const B = { id: 'test-epoch:2', name: '.', path: '', canSave: false, signature: { state: 'unsigned' }, canUndo: false, canRedo: false };

// Mutable, so a test can change what the server holds under a running client — which is
// the whole of the stale case.
let held = { docs: [A, B], activeId: A.id };
let scanStatus = 200;

setNextDocument({ numPages: 3, outline: null });

const h = await boot({
  routes: {
    '/api/docs': () => held,
    '/api/scan': () => (scanStatus === 409
      ? new Response(JSON.stringify({ error: 'that document is no longer open' }), { status: 409 })
      : { findings: [] }),
  },
});
const { document: doc, settle } = h;

test('a reload restores every document the server holds, and the one it says is active', async () => {
  await settle();

  // Two views, two tabs. Before this slice the client asked "what is the ACTIVE
  // document" from exactly one place — the co-sign arrival poll — and never asked what
  // else was open, so this came back showing NOTHING while the server held two.
  const containers = doc.querySelectorAll('.viewerContainer');
  assert.equal(containers.length, 2,
    `restored ${containers.length} views, want 2 — the client is not reading what the server holds`);
  const tabs = doc.querySelectorAll('#tabstrip .tab');
  assert.equal(tabs.length, 2, `the strip shows ${tabs.length} tabs, want 2`);

  // ORDER, from the server's list rather than from whichever adoption finished first.
  const names = [...tabs].map((t) => t.querySelector('.tabname').textContent);
  assert.deepEqual(names, ['alpha.pdf', 'Untitled'],
    'the tabs are not in the server\'s registry order, or the path-less document did not come back');

  // And the ACTIVE one is the one the server named — A, which is NOT the one adopted
  // last. Without that the test would pass against a client that simply activates
  // whatever it finished loading.
  const active = doc.querySelector('#tabstrip .tab.active');
  assert.ok(active, 'no tab is active after a restore');
  assert.equal(active.querySelector('.tabname').textContent, 'alpha.pdf',
    'the restore activated the last document adopted rather than the one the server named as active');
});

test('a 409 drops the tab for a document the server no longer holds', async () => {
  // The ordinary case, and the one that is not a restart: one document is gone, the
  // rest are fine.
  assert.equal(doc.querySelectorAll('#tabstrip .tab').length, 2, 'setup: two tabs are needed to lose one');
  held = { docs: [B], activeId: B.id };
  scanStatus = 409;

  doc.getElementById('scanBtn').click();
  await settle();
  await settle(); // the reconcile is fired but not awaited, by design — the caller's own refusal handling runs first

  // Counted as CONTAINERS, not tabs. The strip is hidden below two documents (S01's
  // appear-at-two rule), so at one document the tab count is 0 whether the
  // reconciliation dropped one view or both — the observable the first draft chose
  // could not tell the right outcome from the worst one.
  assert.equal(doc.querySelectorAll('.viewerContainer').length, 1,
    'the reconciliation did not leave exactly the one document the server still holds');
  assert.ok(doc.querySelector('.viewerContainer:not([hidden])'),
    'no view is visible after the reconciliation');
  assert.equal(doc.getElementById('viewerWrap').className, 'has-doc',
    'the app fell back to the launch state while the server still held a document');
  assert.equal(doc.getElementById('tabstrip').hidden, true,
    'the strip is still showing with one document open');
  scanStatus = 200;
});

test('when the server holds nothing at all, the app resolves to the launch empty state', async () => {
  // **P03's all-tabs-stale pin.** A server restart makes every id stale at once, and the
  // resolution must be the launch state — not N tabs that each error.
  //
  // The stimulus is the client-visible effect of a restart: the ids the client holds are
  // refused, and the server reports an empty set. Asserted BEFORE, so "the launch state"
  // is a state the app arrived at rather than one it never left.
  assert.equal(doc.getElementById('viewerWrap').className, 'has-doc', 'setup: nothing is open, so collapsing proves nothing');

  held = { docs: [], activeId: '' };
  scanStatus = 409;
  doc.getElementById('scanBtn').click();
  await settle();
  await settle();

  assert.equal(doc.getElementById('viewerWrap').className, '',
    'the app did not return to the launch state — the user is left holding tabs that every request refuses');
  assert.equal(doc.getElementById('empty').textContent, 'Open a PDF to begin.');
  assert.equal(doc.getElementById('tabstrip').hidden, true, 'the tab strip is showing with nothing open');
  assert.equal(doc.getElementById('closeBtn').disabled, true, 'Close is still enabled with nothing open');
});
