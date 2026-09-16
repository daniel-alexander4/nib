// The release download's dialog (ADR-039). Dan, 2026-09-16: *"a popup with download progress and a
// way to open/execute/install the new version. Download location should be clearly displayed."*
//
// ── The half this tier owns ──────────────────────────────────────────────────
// What the user SEES: that the dialog opens instead of the old `confirm()`, that progress from the
// window stream reaches it, that the destination is named, that a terminal state stops it, and that
// Cancel tells the server rather than merely hiding the dialog. The transfer itself — what is
// written, what is refused, what a failure leaves behind — is
// `internal/server/updatedownload_test.go`, which has a filesystem.
//
// ── Why the progress assertions read the RENDERED text ───────────────────────
// The line is written into an `aria-live` region, so the property is not "a number arrived" but
// "what a screen reader would announce changed". Asserting the rendered string is the only form
// that can catch a re-announcement of an unchanged line.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './boot.mjs';

let posted = [];
let startStatus = 200;

const h = await boot({
  routes: {
    '/api/update/check': {
      current: '1.0.0', latest: '2.0.0', updateAvailable: true,
      url: 'https://example.test/r', downloadUrl: 'https://example.test/d/nib-2.0.0-linux-amd64',
    },
    '/api/listdir': { path: '/home/someone/nib', parent: '/home/someone', dirs: [], files: [] },
    '/api/update/download': (opts) => {
      posted.push(opts);
      if (startStatus !== 200) {
        return new Response(JSON.stringify({ error: 'a download is already running' }),
          { status: startStatus, headers: { 'Content-Type': 'application/json' } });
      }
      return { status: 'started', name: 'nib-2.0.0-linux-amd64', path: '/home/someone/nib/nib-2.0.0-linux-amd64' };
    },
    '/api/update/download/cancel': (opts) => { posted.push(opts); return { cancelled: true }; },
    '/api/update/reveal': (opts) => { posted.push(opts); return { status: 'ok' }; },
  },
});
const { document: doc, settle } = h;

const modal = () => doc.getElementById('downloadModal');
const progress = () => doc.getElementById('dlProgress').textContent;
const push = (ev) => h.pushWindowEvent('download', JSON.stringify(ev));

async function openDialog() {
  doc.getElementById('updateGet').click();
  await settle();
}

test('clicking the pill opens the dialog instead of a confirm, and names the destination', async () => {
  await openDialog();
  assert.equal(modal().hidden, false,
    'the pill did not open the download dialog — the old path was a native confirm() the user could '
    + 'only answer yes or no to, which is what ADR-039 replaced');
  // The confirm() is gone from this path: a dialog that ALSO confirmed would ask twice.
  assert.equal(h.confirms.length, 0, `the pill still raised ${h.confirms.length} confirm() call(s)`);
  assert.match(doc.getElementById('dlWhat').textContent, /2\.0\.0/, 'the dialog does not say which version it offers');
  // The destination comes from the server's own resolution of the folder, never a guess here.
  assert.match(doc.getElementById('dlWhere').textContent, /\/home\/someone\/nib/,
    `the dialog does not name where the file will go: "${doc.getElementById('dlWhere').textContent}"`);
});

test('the dialog carries the contract every dialog here carries', () => {
  const m = modal();
  assert.equal(m.getAttribute('role'), 'dialog', 'the download dialog is not a dialog to a screen reader');
  assert.equal(m.getAttribute('aria-modal'), 'true', 'the download dialog does not trap the reader');
  const labelled = m.getAttribute('aria-labelledby');
  assert.ok(labelled && doc.getElementById(labelled)?.textContent.trim(),
    'the download dialog has no resolvable accessible name');
  // Escape CLICKS a dialog's Cancel rather than hiding it, so this button is what makes the dialog
  // dismissable from the keyboard AND what makes the abort run.
  assert.ok(m.querySelector('button[id$="Cancel"]'),
    'the download dialog has no Cancel for the Escape handler to reach, so it cannot be dismissed from the keyboard');
  assert.equal(doc.getElementById('dlProgress').getAttribute('aria-live'), 'polite',
    'the progress line is not announced, or is announced assertively');
});

test('progress from the window stream reaches the dialog, and an unchanged line is not rewritten', async () => {
  doc.getElementById('dlGo').click();
  await settle();
  assert.equal(posted.length >= 1, true, 'pressing Download posted nothing to the server');

  assert.equal(push({ active: true, status: 'running', done: 10, total: 100, percent: 10 }), true,
    'no window stream was listening for a download event — progress could never arrive');
  await settle();
  const first = progress();
  assert.match(first, /10%/, `the dialog does not show progress: "${first}"`);

  // The same percent again must not rewrite the region. Asserted on the NODE, because rewriting
  // identical text is exactly what re-announces an aria-live region — invisible to a text compare
  // taken after the fact.
  const before = doc.getElementById('dlProgress').firstChild;
  push({ active: true, status: 'running', done: 10, total: 100, percent: 10 });
  await settle();
  assert.equal(doc.getElementById('dlProgress').firstChild, before,
    'an unchanged progress line was written again, which re-announces it to a screen reader');

  push({ active: true, status: 'running', done: 40, total: 100, percent: 40 });
  await settle();
  assert.match(progress(), /40%/, 'a changed percentage did not reach the dialog');
});

test('a finished download names the file and offers the folder, never a way to run it', async () => {
  push({ active: false, status: 'done', percent: 100, path: '/home/someone/nib/nib-2.0.0-linux-amd64' });
  await settle();
  assert.match(progress(), /\/home\/someone\/nib\/nib-2\.0\.0-linux-amd64/,
    `the finished dialog does not say where the file went: "${progress()}"`);
  assert.equal(doc.getElementById('dlReveal').hidden, false, 'Show in folder is not offered after a download');

  // The refusal that is the whole posture: nothing in this dialog runs or installs the artifact.
  const labels = [...modal().querySelectorAll('button')].map((b) => b.textContent.toLowerCase());
  for (const banned of ['run', 'install', 'execute', 'launch']) {
    assert.ok(!labels.some((l) => l.includes(banned)),
      `the dialog offers "${banned}" — nothing verifies these bytes, so Nib does not run what it downloaded (ADR-039)`);
  }

  posted = [];
  doc.getElementById('dlReveal').click();
  await settle();
  assert.equal(posted.length, 1, 'Show in folder asked the server nothing');
});

test('a failure stops the dialog and says why, rather than spinning', async () => {
  push({ active: false, status: 'failed', problem: 'the download stopped before it finished' });
  await settle();
  const err = doc.getElementById('dlError');
  assert.equal(err.hidden, false, 'a failed download reported nothing to the user');
  assert.match(err.textContent, /stopped before it finished/, `the failure does not say what happened: "${err.textContent}"`);
  assert.equal(doc.getElementById('dlGo').disabled, false, 'after a failure the user cannot try again');
});

test('Cancel tells the server, and does not merely hide the dialog', async () => {
  posted = [];
  doc.getElementById('dlCancel').click();
  await settle();
  assert.equal(modal().hidden, true, 'Cancel left the dialog open');
  assert.equal(posted.length, 1,
    'Cancel hid the dialog without telling the server, so a ~95 MB transfer would keep running with '
    + 'nothing on screen and no way to stop it');
});
