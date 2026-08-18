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

  // Drive SEVERAL document routes, not one. The generalised assertion below is only
  // worth its name if the window it inspects holds more than a single call — with
  // one, "every document-route call carried the id" and "the one call this test
  // happened to trigger carried the id" are the same statement, which is what the
  // original single headerOn('/api/scan') check amounted to.
  calls.length = 0;
  document.getElementById('scanBtn').click();
  await settle();
  document.getElementById('attachBtn').click();
  await settle();

  // EVERY document-route call in the window, not just the one this test happened to
  // trigger. The name promises "every apiFetch call names it" and the body used to
  // read a single /api/scan entry — so a second call going out unpinned, which is
  // exactly the regression the pin exists to stop, left this green. `calls` already
  // records headers per call, so the general form costs nothing.
  //
  // The route list is the same membership question MUTATING answers in
  // pinning.test.mjs: a route that resolves a document must name one. Routes that
  // legitimately do not (pre-unlock, vault, image library) are excluded by prefix.
  //
  // `/api/docs` is on it as of P06.S03, and the entry is a CORRECTION rather than an
  // addition: the route is a question about the SESSION — what does the server hold? —
  // which by construction cannot name a document, and it is the reason apiFetch has an
  // `unpinned` option at all. It was passing this test only because the reconcile runs
  // at boot and `calls` is cleared before the drive below, so a future reconcile firing
  // inside the window would have failed the guard for a correct call. A guard that
  // passes by timing is one that fails by timing.
  const NOT_DOCUMENT_SCOPED = ['/api/status', '/api/ssh/', '/api/vault/', '/api/images',
    '/api/update/', '/api/identity', '/api/peers', '/api/settings', '/api/profile',
    '/api/recent', '/api/browse', '/api/roots', '/api/session/', '/api/cosign/',
    '/api/docs'];
  const documentCalls = calls.filter((c) =>
    c.url.startsWith('/api/') && !NOT_DOCUMENT_SCOPED.some((p) => c.url.startsWith(p)));

  assert.ok(documentCalls.length > 0,
    'no document-route call went out at all — the assertion below would pass over an empty list');

  for (const c of documentCalls) {
    assert.equal(c.headers['X-Nib-Doc'], DOC_ID,
      `${c.method} ${c.url} carried no document id (or the wrong one) — it would silently take the active-document default`);
  }
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
test('nothing bypasses apiFetch to reach a document route', () => {
  const app = fs.readFileSync(path.join(REPO, 'web', 'app.js'), 'utf8');

  // Scans for /api/ LITERALS that are not the first argument of an apiFetch call,
  // rather than for the shape `fetch('/api/...')`.
  //
  // The narrower form missed the whole class. `const url = cond ? '/api/a' : '/api/b';
  // await fetch(url)` has no literal inside the fetch call, so two real bypasses
  // (/api/ssh/enroll and /api/ssh/migrate, app.js) were invisible to a test whose
  // own comment claimed "a fourth bypass fails this test whatever it fetches". It
  // also could not see a route reached by NAVIGATION — window.location — which is
  // how /api/form-data was exporting the server's active document instead of the
  // view's, because a navigation carries no X-Nib-Doc header at all.
  const stripped = app.split('\n').filter((l) => !l.trim().startsWith('//')).join('\n');
  const found = new Map();
  for (const m of stripped.matchAll(/['"`](\/api\/[a-zA-Z0-9/_-]+)/g)) {
    const pre = stripped.slice(Math.max(0, m.index - 40), m.index);
    // The first argument of an apiFetch call, including the ternary form
    // apiFetch(gs ? '/api/pdfa?engine=gs' : '/api/pdfa', …), which is still pinned.
    if (/apiFetch\(\s*$/.test(pre) || /apiFetch\([^()]*\?[^()]*$/.test(pre) || /apiFetch\([^()]*:\s*$/.test(pre)) continue;
    found.set(m[1], (found.get(m[1]) || 0) + 1);
  }

  // Allowed, each for a stated reason — an unexplained entry is how a real bypass
  // gets parked here and forgotten.
  const allowed = new Set([
    // Pre-unlock and vault-scoped: no document exists yet, or the route is not
    // about one. These cannot carry a document id and must not.
    // /api/ssh/repoint is the key-missing recovery (v1.108.14): it runs BEFORE the vault
    // opens, so there is no CSRF token for apiFetch to attach and no document to name.
    // Guarded server-side by requirePublicLoopback, like its three neighbours here.
    '/api/status', '/api/ssh/unlock', '/api/ssh/enroll', '/api/ssh/migrate', '/api/ssh/repoint',
    '/api/update/check', '/api/vault/export', '/api/identity',
    // pdf.js issues these fetches itself, so the id rides in the URL rather than a
    // header — D15, decided rather than overlooked.
    '/api/pdf', '/api/session/pending-pdf',
    // An <img> src, not a fetch, and the image library is not document-scoped.
    '/api/images/',
  ]);

  // The stimulus: an empty result would read as "no bypasses" forever, including
  // after apiFetch itself was deleted.
  assert.ok(found.size > 0, 'the bypass scan matched nothing — it is not reading what it thinks');

  const unexpected = [...found.keys()].filter((u) => !allowed.has(u)).sort();
  assert.deepEqual(unexpected, [],
    'a document route is reached without apiFetch, so it carries no document id and resolves against whatever the SERVER thinks is active');
});

// The `docResponse` reader scan that used to live here is now
// test/jsdom/published.test.mjs, which covers EVERY shape the server publishes rather
// than that one — and, more to the point, checks that its own table is complete. This
// one read docResponse only, so P06's docsResponse and P07's handoffResponse and
// instance.Record were invisible to it: the same gap, one type over, three times. Left
// as a pointer rather than deleted silently, because "where did that check go" is a
// question the next reader will otherwise answer by writing a second one.
