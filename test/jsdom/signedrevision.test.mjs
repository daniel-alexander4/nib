// PLAN-returned-document P02.S03: the client's half of GET /api/document/revision.
//
// `fetchSignedRevision` has no caller yet — the surface for a document that came back is P03's — so these tests run the
// REAL function, its text taken from web/app.js, against a stubbed apiFetch, and boot the app to prove that nothing
// asks for a signed version when the app starts or a document opens (D10).
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { boot, REPO } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const SRC = fs.readFileSync(path.join(REPO, 'web', 'app.js'), 'utf8');
const ROUTE = '/api/document/revision';

// The function's own text, from its declaration to the first line that closes it at column 0.
function functionText(name) {
  const at = SRC.indexOf(`async function ${name}(`);
  assert.ok(at >= 0, `app.js declares no ${name}`);
  const end = SRC.indexOf('\n}\n', at);
  return SRC.slice(at, end + 2);
}

const h = await boot({
  routes: {
    '/api/open': {
      id: 'test-epoch:3', name: 'signed.pdf', path: '/tmp/nib-harness/signed.pdf', canSave: true,
      // A document that CAME BACK CHANGED — signed, then added to — which is the obvious trigger for asking (the P02
      // phase-close review): an open that fetched only for this shape would pass a guard that opened a clean one.
      signature: {
        state: 'valid', addedAfter: true, addedAfterCause: 'appended',
        signers: [{ name: 'Alice', valid: true, fingerprint: 'a'.repeat(64) }],
      },
      canUndo: false, canRedo: false,
    },
  },
});
const { document, calls, settle } = h;

test('nothing asks for a signed version when the app starts or a changed signed document opens (D10)', async () => {
  setNextDocument({ numPages: 2 });
  document.getElementById('pathInput').value = '/tmp/nib-harness/signed.pdf';
  document.getElementById('openGo').click();
  await settle();
  assert.ok(calls.some((c) => c.url.includes('/api/open')), 'stimulus: the open never went out, so an empty list proves nothing');
  const asked = calls.filter((c) => c.url.includes(ROUTE));
  assert.deepEqual(asked.map((c) => c.url), [], 'the app asked for a signed version on boot or open');
});

test('the route is named in exactly one place, the pinned helper', () => {
  const sites = SRC.split(ROUTE).length - 1;
  assert.equal(sites, 1, `${ROUTE} appears ${sites} times in app.js; every request must go through fetchSignedRevision`);
  assert.ok(functionText('fetchSignedRevision').includes(ROUTE), 'the one mention is not inside fetchSignedRevision');
});

test('the helper tells each of the five causes apart, pins the document, and reads the facts', async () => {
  const seen = [];
  const reply = { status: 200, body: null, headers: {} };
  const apiFetch = async (url, opts) => {
    seen.push({ url, opts });
    return new Response(reply.body, { status: reply.status, headers: reply.headers });
  };
  const errText = async (res, fallback) => fallback;
  const fetchSignedRevision = new Function('apiFetch', 'errText',
    `${functionText('fetchSignedRevision')}\nreturn fetchSignedRevision;`)(apiFetch, errText);

  // Read from Go's own declarations, so a sixth cause — or a renamed one — reaches this test (the P02 phase-close
  // review: the list was typed here and nothing compared it with internal/sign/signedrevision.go).
  const GO = fs.readFileSync(path.join(REPO, 'internal', 'sign', 'signedrevision.go'), 'utf8');
  const causes = [...GO.matchAll(/^\s*Revision\w+\s+RevisionCause\s*=\s*"([a-z-]+)"/gm)].map((m) => m[1]);
  assert.deepEqual([...causes].sort(),
    ['could-not-check', 'no-signature', 'not-your-signature', 'prefix-failed-reverify', 'resaved'],
    'the Go side declares a different set of causes; the client and P03\'s wording must cover each');
  const got = [];
  for (const cause of causes) {
    reply.status = 422;
    reply.body = JSON.stringify({ cause, refused: [{ obj: 9, cause: 'contents-elsewhere' }], attributed: cause === 'resaved' });
    reply.headers = { 'Content-Type': 'application/json' };
    const r = await fetchSignedRevision('test-epoch:3', 'b'.repeat(64));
    assert.equal(r.ok, false, `${cause}: read as a version`);
    got.push(r.cause);
    assert.equal(r.refused.length, 1, `${cause}: the refused records were dropped`);
    assert.equal(r.attributed, cause === 'resaved', `${cause}: attributed`);
  }
  assert.deepEqual(got, causes, 'the five causes did not come back as five distinct values');

  reply.status = 200;
  reply.body = new Uint8Array([37, 80, 68, 70]);
  reply.headers = { 'Content-Type': 'application/pdf', 'X-Nib-Revision': JSON.stringify({ obj: 7, end: 4, earlierRevision: true, history: 'none' }) };
  const ok = await fetchSignedRevision('test-epoch:3', 'b'.repeat(64));
  assert.equal(ok.ok, true);
  assert.equal(ok.bytes.byteLength, 4, 'the signed version did not come back as bytes');
  assert.equal(ok.facts.earlierRevision, true, 'the X-Nib-Revision facts were not read');

  for (const s of seen) {
    assert.equal(s.opts.docId, 'test-epoch:3', 'a request was not pinned to its document (ADR-001)');
    assert.ok(s.url.startsWith(`${ROUTE}?signer=`), `unexpected URL ${s.url}`);
  }

  reply.status = 500;
  reply.body = '{}';
  await assert.rejects(fetchSignedRevision('test-epoch:3', 'b'.repeat(64)), 'a 500 was read as a refusal or a version');
});

// D10 holds by construction only while nothing calls the helper: the boot above drives ONE open, and a caller wired to
// some other open-time event would pass it. The allow-list is empty until P03 builds the surface that asks on demand.
const FETCH_CALLERS_ALLOWED = [];
test('fetchSignedRevision has no caller but the ones named here (D10)', () => {
  const calls = SRC.split('fetchSignedRevision(').length - 1 - 1; // less the declaration
  assert.equal(calls, FETCH_CALLERS_ALLOWED.length,
    `fetchSignedRevision is called ${calls} time(s) in app.js; a caller is an on-demand action named here, never open or boot`);
});
