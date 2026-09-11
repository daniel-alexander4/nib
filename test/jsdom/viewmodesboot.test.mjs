// The saved view layout, applied at boot (Dan, 2026-09-10).
//
// **A file of its own because `boot()` runs once per process** (see boot.mjs), and this is the only
// assertion in the set that needs a DIFFERENT `/api/status` — one carrying a layout the user chose
// on a previous run. Its sibling `viewmodes.test.mjs` boots with none, so the two arms of
// `st.viewLayout || 'pages'` cannot both be driven in one file.
//
// **It exists because the other file could not make that line go red.** Replacing
// `applyViewLayout(st.viewLayout || 'pages', false)` with a hard-coded `'pages'` left
// viewmodes.test.mjs entirely green: its harness sends no layout, so both branches produce the
// same answer. The failure that would ship is the plainest one there is — the setting does not
// survive a restart.
//
// The Go side proves `/api/status` CARRIES the value (`viewlayout_test.go`); this proves the
// client acts on it.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

const h = await boot({
  routes: {
    '/api/status': {
      state: 'ready', csrf: 'test-csrf', version: 'test',
      autoUpdate: false, updateCheckLocked: false, ghostscript: false, libreoffice: false,
      viewLayout: 'continuous',
    },
  },
});
const { document: doc } = h;

test('a layout saved on a previous run is applied before anything is opened', () => {
  const stack = doc.querySelector('.viewerContainer .pdfViewer');
  assert.ok(stack, 'the boot view built no page stack');
  assert.ok(stack.classList.contains('nibJoined'),
    'the boot view is on the standard layout although the vault holds "continuous" — the setting '
    + 'does not survive a restart, which is the whole of what persisting it is for');
});

test('and the View group says so, rather than the buttons disagreeing with the document', () => {
  assert.equal(doc.getElementById('viewContinuousBtn').getAttribute('aria-pressed'), 'true',
    'the document is joined and the Continuous button reads as unpressed — the control and the '
    + 'thing it controls disagree, so the next click appears to do nothing');
  assert.equal(doc.getElementById('viewStandardBtn').getAttribute('aria-pressed'), 'false');
});

test('applying a saved layout does not write it straight back', async () => {
  // `applyViewLayout(..., false)` — the persist flag. A boot that saved on every start would turn
  // a read into a write, and on a vault opened read-only or mid-migration that is a failure toast
  // on every launch for a setting nobody touched.
  const posts = h.calls.filter((c) => c.url.includes('/api/settings'));
  assert.equal(posts.length, 0,
    `boot posted ${posts.length} settings write(s) for a layout it only READ`);
});
