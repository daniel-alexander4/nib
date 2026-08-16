// Tier 2 smoke: the real web/app.js boots under the stubs, and the launch state
// it produces is the one P01 defined.
//
// This is the test that proves the harness reaches anything at all. Until it
// passes, every other tier-2 test would be asserting against a document nothing
// had run over — green because nothing happened, which is the failure mode this
// whole phase exists to prevent.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

// One boot per file — see the note in boot.mjs.
const { document, calls, rejections } = await boot();

test('the real app.js boots with no unhandled rejection', () => {
  assert.deepEqual(rejections.map((r) => r?.message ?? String(r)), [],
    'app.js threw during boot; the fetch stub shapes or the module stubs are wrong');
});

test('boot asks the server for its status', () => {
  // The non-zero probe for the fetch stub itself: if this is empty, the harness
  // installed a fetch nothing ever called, and every route assertion below it
  // would be vacuous.
  assert.ok(calls.length > 0, 'app.js made no fetch calls at all during boot');
  assert.ok(calls.some((c) => c.url.endsWith('/api/status')),
    `expected a /api/status call, got: ${calls.map((c) => c.url).join(', ')}`);
});

test('the launch state is the one P01 defines', () => {
  // Each of these is a P01 acceptance value, asserted here for the first time by
  // machine rather than by a human driving a browser.
  assert.equal(document.getElementById('empty').textContent, 'Open a PDF to begin.');
  assert.equal(document.getElementById('viewerWrap').className, '',
    'has-doc must not be set with nothing open');
  assert.equal(document.getElementById('sigBadge').textContent, 'no document');
  assert.equal(document.getElementById('sigBadge').className, 'badge badge-none');
  assert.equal(document.getElementById('sigDetailsBtn').hidden, true);
  assert.equal(document.querySelector('.pageCount').textContent, '/ 0');
});

test('document-requiring controls are disabled with nothing open', () => {
  // Includes closeBtn, the control P01.S04 added — so the greying-out clause is
  // now covered mechanically instead of only by a live drive.
  for (const id of ['closeBtn', 'saveBtn', 'printBtn', 'redactBtn', 'compareBtn']) {
    const el = document.getElementById(id);
    assert.ok(el, `${id} missing from index.html`);
    assert.equal(el.disabled, true, `${id} should be disabled with no document open`);
  }
});
