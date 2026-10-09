// The release download's popup (ADR-039, ADR-102).
//
// Dan, 2026-09-16: *"a popup with download progress and a way to open/execute/install the new
// version. Download location should be clearly displayed."* And 2026-10-07: *"nib download folder
// should always be the browser default folder unless configured differently in settings. It should
// not ask me where I want to download it. It should pop up a message saying Downloading to
// <folder>."*
//
// ── The half this tier owns ──────────────────────────────────────────────────
// What the user SEES: that clicking the pill starts the download with nothing asked, that the
// request names no folder, that the popup says the folder the SERVER answered with, that progress
// from the window stream reaches it, that a terminal state stops it, and that Cancel tells the
// server rather than merely hiding the popup. And the Settings field: what it shows, what it sends,
// and where a refusal is said. Which folder the server picks, and what a browser's preference file
// may say, are `internal/server/downloaddir_test.go` and `internal/browser/downloads_test.go`.
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
let settings = [];
let settingsRefusal = '';
let status = {
  state: 'ready', version: 'test', autoUpdate: false, updateCheckLocked: false, ghostscript: false, libreoffice: false,
  advanced: { ceremony: true, discovery: true, rendezvous: true, timestamp: true },
  downloadDir: '/home/someone/Saved', downloadDirFrom: 'browser', downloadDirBrowser: 'Chrome',
};

const FOLDER = '/home/someone/Saved';
const FILE = `${FOLDER}/nib-2.0.0-linux-amd64`;
const refusal = (code, error) => new Response(JSON.stringify({ error }),
  { status: code, headers: { 'Content-Type': 'application/json' } });

const h = await boot({
  routes: {
    '/api/status': () => status,
    '/api/update/check': {
      current: '1.0.0', latest: '2.0.0', updateAvailable: true,
      url: 'https://example.test/r', downloadUrl: 'https://example.test/d/nib-2.0.0-linux-amd64',
    },
    '/api/update/download': (opts) => {
      posted.push(opts);
      if (startStatus === 412) return refusal(412, `nib-2.0.0-linux-amd64 is already in ${FOLDER}. Nothing was downloaded.`);
      if (startStatus !== 200) return refusal(startStatus, 'a download is already running');
      return { status: 'started', name: 'nib-2.0.0-linux-amd64', path: FILE, dir: FOLDER };
    },
    '/api/update/download/cancel': (opts) => { posted.push(opts); return { cancelled: true }; },
    '/api/update/reveal': (opts) => { posted.push(opts); return { status: 'ok' }; },
    '/api/settings': (opts) => {
      settings.push(JSON.parse(opts.body));
      if (settingsRefusal) return refusal(400, settingsRefusal);
      return { status: 'ok' };
    },
  },
});
const { document: doc, settle } = h;

const modal = () => doc.getElementById('downloadModal');
const progress = () => doc.getElementById('dlProgress').textContent;
const where = () => doc.getElementById('dlWhere').textContent;
const push = (ev) => h.pushWindowEvent('download', JSON.stringify(ev));

async function clickPill() {
  doc.getElementById('updateGet').click();
  await settle();
}

test('clicking the pill downloads at once: nothing is asked, and the request names no folder', async () => {
  posted = [];
  await clickPill();
  assert.equal(modal().hidden, false, 'the pill did not open the download popup');
  assert.equal(h.confirms.length, 0, `the pill raised ${h.confirms.length} confirm() call(s) — it must not ask`);
  assert.match(doc.getElementById('dlWhat').textContent, /2\.0\.0/, 'the popup does not say which version it is fetching');

  // It does not ask: the one click is the request. ADR-039's dialog waited for a Download button.
  assert.equal(posted.length, 1,
    `clicking the pill sent ${posted.length} download request(s), want exactly one — the popup is waiting to be asked`);
  // And it does not choose: the server resolves the folder (ADR-102). A body here is a folder
  // the page made up.
  assert.equal(posted[0].body == null, true,
    `the download request carries a body (${String(posted[0].body)}) — the page is naming a folder again`);

  // Nothing in the popup is a way to pick a folder or a second step to press.
  assert.equal(modal().querySelectorAll('input, select, textarea').length, 0, 'the popup has something to fill in');
  const labels = [...modal().querySelectorAll('button')].filter((b) => !b.hidden).map((b) => b.textContent.trim());
  assert.deepEqual(labels, ['Cancel'], `while downloading the popup offers ${JSON.stringify(labels)}, want only Cancel`);
});

test('the popup says "Downloading to <folder>", and the folder is the server\'s answer', () => {
  assert.equal(where(), `Downloading to ${FOLDER}`,
    `the popup says "${where()}" — Dan: "It should pop up a message saying Downloading to <folder>"`);
  assert.equal(doc.getElementById('dlWhere').getAttribute('aria-live'), 'polite', 'the folder line is not announced');
});

test('the popup carries the contract every dialog here carries', () => {
  const m = modal();
  assert.equal(m.getAttribute('role'), 'dialog', 'the download popup is not a dialog to a screen reader');
  assert.equal(m.getAttribute('aria-modal'), 'true', 'the download popup does not trap the reader');
  const labelled = m.getAttribute('aria-labelledby');
  assert.ok(labelled && doc.getElementById(labelled)?.textContent.trim(),
    'the download popup has no resolvable accessible name');
  // Escape CLICKS a dialog's Cancel rather than hiding it, so this button is what makes the popup
  // dismissable from the keyboard AND what makes the abort run.
  assert.ok(m.querySelector('button[id$="Cancel"]'),
    'the download popup has no Cancel for the Escape handler to reach, so it cannot be dismissed from the keyboard');
  assert.equal(doc.getElementById('dlProgress').getAttribute('aria-live'), 'polite',
    'the progress line is not announced, or is announced assertively');
});

test('progress from the window stream reaches the popup, and an unchanged line is not rewritten', async () => {
  assert.equal(push({ status: 'running', done: 10, total: 100, percent: 10 }), true,
    'no window stream was listening for a download event — progress could never arrive');
  await settle();
  const first = progress();
  assert.match(first, /10%/, `the popup does not show progress: "${first}"`);
  assert.equal(where(), `Downloading to ${FOLDER}`, 'progress arriving wiped the folder line');

  // The same percent again must not rewrite the region. Asserted on the NODE, because rewriting
  // identical text is exactly what re-announces an aria-live region — invisible to a text compare
  // taken after the fact.
  const before = doc.getElementById('dlProgress').firstChild;
  push({ status: 'running', done: 10, total: 100, percent: 10 });
  await settle();
  assert.equal(doc.getElementById('dlProgress').firstChild, before,
    'an unchanged progress line was written again, which re-announces it to a screen reader');

  push({ status: 'running', done: 40, total: 100, percent: 40 });
  await settle();
  assert.match(progress(), /40%/, 'a changed percentage did not reach the popup');

  // No stated length: no percent exists, so the bytes so far are the progress (/pending 646).
  push({ status: 'running', done: 7 * 1048576 + 5, total: 0, percent: 0 });
  await settle();
  assert.match(progress(), /7 MB so far/,
    'a transfer whose server stated no length shows no progress for its whole run');
});

test('a finished download names the file and offers the folder, never a way to run it', async () => {
  push({ status: 'done', percent: 100, path: FILE });
  await settle();
  assert.equal(progress(), `Downloaded to ${FILE}`, `the finished popup does not say where the file went: "${progress()}"`);
  assert.equal(where(), '', `a finished download still says "${where()}"`);
  assert.equal(doc.getElementById('dlReveal').hidden, false, 'Show in folder is not offered after a download');

  // The refusal that is the whole posture: nothing in this popup runs or installs the artifact.
  const labels = [...modal().querySelectorAll('button')].map((b) => b.textContent.toLowerCase());
  for (const banned of ['run', 'install', 'execute', 'launch']) {
    assert.ok(!labels.some((l) => l.includes(banned)),
      `the popup offers "${banned}" — nothing verifies these bytes, so Nib does not run what it downloaded (ADR-039)`);
  }

  posted = [];
  doc.getElementById('dlReveal').click();
  await settle();
  assert.equal(posted.length, 1, 'Show in folder asked the server nothing');
  assert.equal(posted[0].body == null, true, 'Show in folder sent the server a path — reveal takes none (ADR-039)');
});

test('a failure stops the popup, says why, and says how to try again', async () => {
  await clickPill();
  push({ status: 'failed', problem: 'the download stopped before it finished' });
  await settle();
  const err = doc.getElementById('dlError');
  assert.equal(err.hidden, false, 'a failed download reported nothing to the user');
  assert.match(err.textContent, /stopped before it finished/i, `the failure does not say what happened: "${err.textContent}"`);
  // There is no Download button to press again (ADR-102), so the popup has to say what to do.
  assert.match(err.textContent, /click the version number again/, `the failure does not say how to retry: "${err.textContent}"`);
  assert.equal(where(), '', `a failed download still says "${where()}"`);

  // And the pill does start a fresh one: the old error is gone and a new request went out.
  posted = [];
  await clickPill();
  assert.equal(posted.length, 1, 'clicking the pill after a failure did not start another download');
  assert.equal(err.hidden, true, 'the previous failure is still showing over a new download');
  assert.equal(doc.getElementById('dlReveal').hidden, true, 'Show in folder from an earlier download is still offered');
});

test('a file already in the folder is said plainly, names the folder, and offers to show it', async () => {
  startStatus = 412;
  await clickPill();
  startStatus = 200;
  const err = doc.getElementById('dlError');
  assert.equal(err.hidden, false, 'a refused download reported nothing');
  assert.ok(err.textContent.includes(FOLDER) && /already in/.test(err.textContent),
    `the refusal does not say the file is already in ${FOLDER}: "${err.textContent}"`);
  assert.equal(progress(), '', `a refused download still shows "${progress()}"`);
  assert.equal(where(), '', `a refused download says "${where()}" — nothing is downloading`);
  assert.equal(doc.getElementById('dlReveal').hidden, false,
    'the popup says the file is in a folder and offers no way to see it');
  assert.equal(modal().querySelectorAll('input, select, textarea').length, 0, 'the refusal asks the user to choose something');

  // Any other refusal has no file to show.
  startStatus = 409;
  await clickPill();
  startStatus = 200;
  assert.match(err.textContent, /already running/, `a 409 is not said: "${err.textContent}"`);
  assert.equal(doc.getElementById('dlReveal').hidden, true, 'Show in folder is offered for a refusal that names no file');
});

test('a download that ends before its own answer is read does not go back to "Downloading to"', async () => {
  // A small file: the stream's `done` can arrive before the POST's answer is parsed.
  let release;
  const gate = new Promise((r) => { release = r; });
  const real = globalThis.fetch;
  globalThis.fetch = async (url, opts) => {
    const res = await real(url, opts);
    if (String(url).includes('/api/update/download') && !String(url).includes('cancel')) await gate;
    return res;
  };
  try {
    doc.getElementById('updateGet').click();
    await settle();
    push({ status: 'done', percent: 100, path: FILE });
    await settle();
    release();
    await settle();
    assert.equal(where(), '', `after finishing, the popup went back to saying "${where()}"`);
    assert.equal(progress(), `Downloaded to ${FILE}`);
  } finally {
    globalThis.fetch = real;
  }
});

test('Cancel tells the server, and does not merely hide the popup', async () => {
  posted = [];
  doc.getElementById('dlCancel').click();
  await settle();
  assert.equal(modal().hidden, true, 'Cancel left the popup open');
  assert.equal(posted.length, 1,
    'Cancel hid the popup without telling the server, so a ~95 MB transfer would keep running with '
    + 'nothing on screen and no way to stop it');
});

// ── Settings → Updates → Download folder ─────────────────────────────────────

const box = () => doc.getElementById('downloadDirInput');
const line = () => doc.getElementById('downloadDirWhere').textContent;
const boxError = () => doc.getElementById('downloadDirError');

async function type(v) {
  box().value = v;
  box().dispatchEvent(new h.window.Event('change'));
  await settle();
}

test('Settings shows the folder in use and where it came from, in plain words', async () => {
  assert.equal(box().closest('.apppage')?.id, 'settingsUpdatesPage', 'the Download folder field is not on the Updates page (ADR-104)');
  assert.equal(box().value, '', 'with nothing set in Nib the box is not empty');
  assert.equal(line(), `Updates download to ${FOLDER} — Chrome’s download folder.`);
  for (const jargon of [/XDG/i, /profile/i, /preferences/i, /default_directory/]) {
    assert.doesNotMatch(line(), jargon, `the line uses a word the user has no reason to know: "${line()}"`);
  }
});

test('each source of the folder is worded, and a set folder that is gone is said', async () => {
  const say = async (st) => { status = { ...status, downloadDirSet: undefined, downloadDirBrowser: undefined, ...st }; await type(''); return line(); };
  assert.equal(await say({ downloadDir: '/h/Downloads', downloadDirFrom: 'system' }),
    'Updates download to /h/Downloads — your Downloads folder.');
  assert.equal(await say({ downloadDir: '/h/nib', downloadDirFrom: 'nib' }),
    'Updates download to /h/nib — Nib’s own folder, because no Downloads folder was found.');
  assert.equal(await say({ downloadDir: '/h/x', downloadDirFrom: 'browser', downloadDirBrowser: 'Firefox' }),
    'Updates download to /h/x — Firefox’s download folder.');
  assert.equal(await say({ downloadDir: '/h/mine', downloadDirFrom: 'setting', downloadDirSet: '/h/mine' }),
    'Updates download to /h/mine — set here.');
  assert.equal(box().value, '/h/mine', 'the box does not show the folder that is set');
  // Set, and since removed: the folder in use is not the one in the box, and the line says why.
  assert.equal(await say({ downloadDir: '/h/Downloads', downloadDirFrom: 'system', downloadDirSet: '/h/gone' }),
    '/h/gone is no longer there, so updates download to /h/Downloads — your Downloads folder.');
});

test('a typed folder is sent to the server, and its refusal is shown beside the box', async () => {
  settings = [];
  status = { ...status, downloadDir: '/h/typed', downloadDirFrom: 'setting', downloadDirSet: '/h/typed' };
  await type('  ~/typed ');
  assert.deepEqual(settings, [{ downloadDir: '~/typed' }], `the field sent ${JSON.stringify(settings)}`);
  // What is shown afterwards is the server's answer — the whole path — never the text typed.
  assert.equal(box().value, '/h/typed', 'the box kept the typed text instead of the folder the server stored');
  assert.equal(line(), 'Updates download to /h/typed — set here.');
  assert.equal(boxError().hidden, true, 'a saved folder shows an error');

  settingsRefusal = 'Nib could not find that folder. Check the spelling, or create it first.';
  await type('/no/such');
  assert.equal(boxError().hidden, false, 'a refused folder shows nothing');
  assert.equal(boxError().textContent, settingsRefusal, 'the refusal shown is not the server\'s sentence');
  assert.equal(line(), 'Updates download to /h/typed — set here.', 'a refused folder changed the line saying which folder is in use');

  // Clearing the box returns to the browser's folder, and clears the error.
  settingsRefusal = '';
  settings = [];
  status = { ...status, downloadDir: FOLDER, downloadDirFrom: 'browser', downloadDirBrowser: 'Chrome', downloadDirSet: undefined };
  await type('');
  assert.deepEqual(settings, [{ downloadDir: '' }], 'clearing the box did not tell the server');
  assert.equal(boxError().hidden, true, 'the old refusal is still showing');
  assert.equal(line(), `Updates download to ${FOLDER} — Chrome’s download folder.`);
});
