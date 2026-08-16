// P03.S03: the client names the document on every request.
//
// This is where the plan-review's critical finding is discharged. The server
// accepts a missing id and falls back to "whatever is active" — a compatibility
// path the CLI and the older Go tests need — so a call site that simply *forgets*
// the header gets the active document during exactly the switch operation-pinning
// exists to survive, having passed no check.
//
// The reason it needs a guard rather than a convention: **a pinned call and an
// unpinned one differ by an ABSENT header.** Nothing in code review can see that,
// which is why enforcement lives in apiFetch and why this test exists to prove it
// is still there.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { boot, REPO } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const DOC = '/tmp/nib-harness/doc.pdf';
const DOC_ID = 'test-epoch:7';

const h = await boot({
  routes: {
    '/api/open': {
      id: DOC_ID, name: 'doc.pdf', path: DOC, canSave: true,
      signature: { state: 'unsigned' }, canUndo: false, canRedo: false,
    },
    '/api/scan': { hidden: [] },
  },
});
const { document, calls, settle } = h;

const headerOn = (substr) => {
  const c = [...calls].reverse().find((x) => x.url.includes(substr));
  return c ? c.headers['X-Nib-Doc'] : undefined;
};

// P3 in the inventory, and it runs FIRST only in file order — its assertion is
// meaningful only because the presence case below passes. Stated so the ordering
// is not mistaken for laziness: "no header before a document is open" would be
// satisfied by a build that never attaches one at all.
test('before a document is open, no id is sent', () => {
  const boots = calls.filter((c) => c.url.includes('/api/status') || c.url.includes('/api/images'));
  assert.ok(boots.length > 0, 'the boot made no calls — nothing below would mean anything');
  for (const c of boots) {
    assert.equal(c.headers['X-Nib-Doc'], undefined,
      `${c.url} carried an id before any document was open`);
  }
});

test('opening a document, then every apiFetch call names it', async () => {
  setNextDocument({ numPages: 3 });
  document.getElementById('pathInput').value = DOC;
  document.getElementById('openGo').click();
  await settle();

  // The stimulus: the open itself must have happened, or every assertion below
  // reads an empty list and passes.
  assert.ok(calls.some((c) => c.url.includes('/api/open')), 'the open call never went out');
  assert.equal(document.getElementById('viewerWrap').className, 'has-doc');

  // Now drive a document route through apiFetch and read the header off it.
  calls.length = 0;
  document.getElementById('scanBtn').click();
  await settle();

  const sent = headerOn('/api/scan');
  assert.equal(sent, DOC_ID,
    'a document-route call did not carry the id the server issued — it would silently take the active-document default');
});

test('the id sent is the one the server issued, not one the client invented', () => {
  // ADR-001 makes the server the sole issuer: ids are monotonic, epoch-prefixed
  // and never reused, and none of that is a guarantee the client could make. If
  // the client ever derived an id rather than echoing one, the law would stop
  // being the server's to keep.
  const sent = headerOn('/api/scan');
  assert.equal(sent, DOC_ID);
  assert.match(sent, /^[^:]+:\d+$/, 'the id is not in the epoch:seq form the server issues');
});

// D15's exception: pdf.js issues the /api/pdf fetch itself, so the id rides the
// URL rather than a header — a header would mean opting into pdf.js's httpHeaders
// plumbing for a uniformity nobody reads.
test('/api/pdf carries its id as a query parameter', () => {
  const pdfCall = [...calls, ...h.calls].find((c) => c.url.includes('/api/pdf'));
  if (!pdfCall) {
    // The stubbed pdf.js resolves without fetching, so this may not appear at
    // tier 2. Assert the SOURCE instead of pretending the call was observed.
    const app = fs.readFileSync(path.join(REPO, 'web', 'app.js'), 'utf8');
    assert.match(app, /'\/api\/pdf\?t=' \+ Date\.now\(\) \+ docParam/,
      'the pdf.js URL does not append the document id');
    assert.match(app, /const docParam = meta\.id \? '&doc=' \+ encodeURIComponent\(meta\.id\)/,
      'the doc parameter is not built from the served id');
    return;
  }
  assert.match(pdfCall.url, /[?&]doc=/, '/api/pdf did not carry a doc parameter');
});

// P5: the property the whole enforcement rests on. apiFetch can be perfect and
// still be bypassed by a bare fetch, and such a call would look entirely ordinary.
//
// The guard freezes the exact set rather than trying to classify routes: a regex
// cannot prove a route is non-document, only that a literal is not one it knows
// about. So a fourth bypass fails this test whatever it fetches, and adding one
// deliberately means updating this list deliberately — which is the intended cost.
test('nothing bypasses apiFetch to reach an API route', () => {
  const app = fs.readFileSync(path.join(REPO, 'web', 'app.js'), 'utf8');
  const bare = [];
  for (const line of app.split('\n')) {
    if (line.trim().startsWith('//')) continue;
    for (const m of line.matchAll(/(?<!api)[Ff]etch\(\s*['"`](\/api\/[a-z0-9/-]+)/g)) {
      bare.push(m[1]);
    }
  }
  const allowed = ['/api/status', '/api/ssh/unlock', '/api/update/check'];

  // The stimulus: if the scan found nothing at all, an empty result would read as
  // "no bypasses" forever, including after apiFetch itself was deleted.
  assert.ok(bare.length > 0, 'the bypass scan matched nothing — it is not reading what it thinks');

  const unexpected = bare.filter((u) => !allowed.includes(u));
  assert.deepEqual(unexpected, [],
    'a route is fetched without going through apiFetch, so it carries no document id and nothing in review would show it');
});
