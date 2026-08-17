// P03.S05, client side: a refusal body is not a document, and the refresh that runs
// after an out-of-band arrival asks about the SESSION rather than about a document.
//
// The regression these guard was live at v1.103.12 and was silent in both directions.
// `reloadOpenDoc` was the ONE document-route call site in app.js with no `res.ok`
// check — the other twenty have it — so a 409 body reached `setDocumentFromServer`,
// which assigned `{error: "..."}` to docMeta. That left `docMeta.id` undefined, so
// every later request stopped sending the header and the session reverted to the
// unpinned path P03.S03 exists to close; meanwhile the document still rendered,
// because /api/pdf with no id falls back to the active one. Nothing looked wrong
// except that Save stopped working.
//
// The fix is deliberately NOT "apiFetch throws on 409". That was tried and reverted in
// this slice's own diff review: fifteen call sites already handle a refusal as
// `if (!res.ok) { toast('…'); return; }`, and throwing would convert fifteen clear
// failures into unhandled rejections saying nothing. The guard belongs at the sink,
// where the mistake actually does damage and where one check covers every caller.
//
// ── What this tier can and cannot reach ──────────────────────────────────────
// `reloadOpenDoc` and `apiFetch` are module-scope, and the arrival that calls
// reloadOpenDoc originates in a p2p session — so the failing SEQUENCE cannot be driven
// from jsdom. Driven here: the pin is really in force, the refusal really carries no
// id, and the session really stays pinned after one. Asserted at the source: which
// discriminator the sink uses, and that apiFetch does not throw. The end-to-end drive
// is `not exercised` at this tier and carried to P06, when an arrival becomes
// reachable through the UI.
//
// The same ceiling P01.S03 hit with `closeDocument`, named rather than papered over —
// a source assertion dressed as a behavioural one is worse than no test, because it
// reads as coverage.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { boot, REPO } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const DOC = '/tmp/nib-harness/doc.pdf';
const DOC_ID = 'test-epoch:3';

let docConflict = false;

const h = await boot({
  routes: {
    '/api/open': {
      id: DOC_ID, name: 'doc.pdf', path: DOC, canSave: true,
      signature: { state: 'unsigned' }, canUndo: false, canRedo: false,
    },
    '/api/scan': { hidden: [] },
    '/api/doc': () => (docConflict
      ? { error: 'that document is no longer open' }
      : { id: 'test-epoch:9', name: 'arrived.pdf', path: '', canSave: false,
          signature: { state: 'untampered' }, canUndo: false, canRedo: false }),
  },
});
const { document, calls, settle } = h;

test('the pin is in force, so the unpinned case is a difference and not a default', async () => {
  setNextDocument({ numPages: 2 });
  document.getElementById('pathInput').value = DOC;
  document.getElementById('openGo').click();
  await settle();

  assert.ok(calls.some((c) => c.url.includes('/api/open')), 'the open call never went out');
  assert.equal(document.getElementById('viewerWrap').className, 'has-doc');

  calls.length = 0;
  document.getElementById('scanBtn').click();
  await settle();
  const scan = [...calls].reverse().find((c) => c.url.includes('/api/scan'));
  assert.equal(scan?.headers['X-Nib-Doc'], DOC_ID,
    'nothing is pinned, so "this one call is unpinned" would be vacuous');
});

test('the post-arrival refresh asks its question unpinned', () => {
  const app = fs.readFileSync(path.join(REPO, 'web', 'app.js'), 'utf8');

  assert.match(app, /apiFetch\('\/api\/doc', \{ unpinned: true \}\)/,
    'reloadOpenDoc pins its refresh — it would report the document the client already holds, and the arrival would never appear');

  // And the option is HONOURED, not merely passed. A rename on either side would
  // otherwise leave the call pinned with this file still green — the exact shape of
  // failure that makes a source assertion dangerous.
  assert.match(app, /const unpinned = opts\.unpinned === true;/,
    'apiFetch does not read the unpinned option the call site passes');
  assert.match(app, /if \(!unpinned && view\.docMeta && view\.docMeta\.id\)/,
    'apiFetch attaches the id without consulting the unpinned option');
});

test('a refusal body is refused rather than becoming the document', async () => {
  const app = fs.readFileSync(path.join(REPO, 'web', 'app.js'), 'utf8');

  // The sink guard. Asserted at the source AND driven below — the source assertion
  // pins which discriminator is used (`error`, not "no id"), because the empty
  // docResponse a closed document produces also has no id and is legitimate.
  assert.match(app, /if \(!meta \|\| meta\.error\) \{/,
    'setDocumentFromServer accepts a refusal body — assigning it to docMeta drops the id and silently unpins every later request');

  // reloadOpenDoc's missing ok-check, which was the whole defect: every other
  // document-route site in the file already had this line.
  assert.match(app, /if \(!res\.ok\) \{ toast\('co-signed, but could not refresh the view'\); return; \}/,
    'reloadOpenDoc parses the body without checking res.ok');

  // And apiFetch must NOT throw on 409: fifteen call sites handle it as
  // `if (!res.ok) { toast(...) }`, and throwing would turn each into an unhandled
  // rejection that tells the user nothing.
  //
  // **Re-derived at P06.S03, not loosened.** This used to assert that the string
  // `res.status === 409` appeared NOWHERE in app.js — a proxy for "does not throw",
  // and a proxy that stopped tracking its property the moment apiFetch grew a 409
  // branch that reconciles instead of throwing. Widening a guard until it passes is
  // how a guard stops guarding, so this now reads the branch and asserts the thing
  // the proxy stood for.
  const fnStart = app.indexOf('async function apiFetch');
  assert.ok(fnStart !== -1, 'apiFetch is gone — this guard has nothing to read');
  const fn = app.slice(fnStart, app.indexOf('\n}', app.indexOf('return res;', fnStart)));
  // The stimulus: the 401 branch DOES throw, so a scan that finds no `throw` anywhere
  // is reading the wrong region rather than reporting health.
  // Same LINE, not `.*` under the s flag: that version matched the word "throw" in a
  // comment forty lines below and so could not fail. The stimulus for a stimulus.
  assert.match(fn, /res\.status === 401[^\n]*throw/,
    'the 401 branch no longer throws — this scan is not reading apiFetch');
  const at409 = fn.indexOf('res.status === 409');
  if (at409 !== -1) {
    const branch = fn.slice(at409, fn.indexOf('\n  }', at409) + 4);
    assert.doesNotMatch(branch, /\bthrow\b/,
      'apiFetch throws on 409, converting fifteen handled failures into silent ones');
    assert.doesNotMatch(branch, /\bawait\b/,
      'apiFetch awaits its 409 reconciliation, so every refused call now waits on a second round-trip before the caller can report it');
  }

  // The behavioural half: the refusal really is refused. docMeta keeps its id, so
  // the session stays pinned — observable through the header on a later call.
  docConflict = true;
  const before = [...calls].reverse().find((c) => c.headers['X-Nib-Doc'])?.headers['X-Nib-Doc'];
  assert.equal(before, DOC_ID, 'setup: the session is not pinned, so staying pinned proves nothing');

  const body = await (await globalThis.fetch('/api/doc', { headers: { 'X-Nib-Doc': DOC_ID } })).json();
  assert.ok('error' in body, 'the harness did not produce a refusal body');
  assert.equal(body.id, undefined, 'a refusal carries no id — this is what must not reach docMeta');
  docConflict = false;

  // The session is still pinned after the refusal was seen.
  calls.length = 0;
  document.getElementById('scanBtn').click();
  await settle();
  const after = [...calls].reverse().find((c) => c.url.includes('/api/scan'));
  assert.equal(after?.headers['X-Nib-Doc'], DOC_ID,
    'the session lost its pin — this is the silent unpinning the guard exists to prevent');
});
