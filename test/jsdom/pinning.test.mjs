// P04.S01 — operation pinning at the transport (D7).
//
// The defect this phase exists for, stated once: `apiFetch` stamps the CURRENT
// `docMeta.id`, and a mutating call whose payload was built before an `await` was
// therefore addressed to whatever document is current by the time the request goes
// out. For `/api/save` that is not a mislabel — the server writes the posted bytes to
// the *addressed* document's path, so document A's contents land in document B's file,
// past the signature guard, with a "Saved" toast and no error anywhere.
//
// It is fixable only because of P03: ADR-001 makes ids monotonic and never reused, so a
// captured id whose document is gone gets a 409 and the operation is REFUSED. Under a
// recycled id the same request would be silently redirected at whatever inherited the
// number — worse than the bug.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { REPO } from './boot.mjs';

const APP = fs.readFileSync(path.join(REPO, 'web', 'app.js'), 'utf8');

// The mutating routes: those where a misaddressed request REWRITES the addressed
// document rather than merely reporting on it. Kept as a literal list rather than a
// pattern, for the reason S03's bypass guard keeps one — a pattern can only say a route
// is not one it knows about.
const MUTATING = [
  '/api/save', '/api/pages', '/api/redact', '/api/outline', '/api/attachments',
  '/api/ocr', '/api/flags', '/api/assemble', '/api/export', '/api/sanitize',
  '/api/decrypt', '/api/undo', '/api/redo', '/api/sign', '/api/stamp',
];

// scanUnpinned finds every apiFetch on a mutating route that is preceded by an `await`
// in its own function and does not pass an explicit `docId`. That is the corruption
// channel, defined mechanically rather than by inspection.
function scanUnpinned(src) {
  const funcs = [];
  const header = /^(?:async function (\w+)|const (\w+) = async|\s*async (\w+)\()/gm;
  let m;
  while ((m = header.exec(src)) !== null) {
    const name = m[1] || m[2] || m[3];
    const open = src.indexOf('{', m.index);
    if (open === -1) continue;
    let depth = 0, end = open;
    for (let j = open; j < src.length; j++) {
      if (src[j] === '{') depth++;
      else if (src[j] === '}') { depth--; if (depth === 0) { end = j; break; } }
    }
    funcs.push({ name, body: src.slice(open, end + 1) });
  }

  const out = [];
  for (const { name, body } of funcs) {
    const call = /apiFetch\(\s*'([^']+)'/g;
    let c;
    while ((c = call.exec(body)) !== null) {
      // Is there an await that COMPLETES before this call is built? The naive test —
      // "does the word `await` appear earlier in the function" — is wrong and was
      // wrong here: in `const res = await apiFetch(...)` the word `await` sits before
      // `apiFetch`, so the call's OWN await counted as a preceding one and eight
      // functions were reported as corrupting that structurally cannot be. Strip the
      // introducer that belongs to this very call, then look.
      const pre = body.slice(0, c.index)
        .replace(/(?:(?:const|let|var)\s+\w+\s*=\s*|return\s+)?await\s+$/, '');
      if (!pre.includes('await')) continue;
      const route = c[1].split('?')[0];
      if (!MUTATING.some((r) => route.startsWith(r))) continue;
      // Read the call's own argument object — from the call to the matching close —
      // and look for an explicit docId.
      // Both forms count: `docId: expr` and the ES6 shorthand `docId`. Missing the
      // shorthand made three pinned helpers read as unpinned.
      const tail = body.slice(c.index, c.index + 400);
      if (/\bdocId\s*[,:}\s]/.test(tail)) continue;
      out.push({ name, route });
    }
  }
  return out;
}

// The frozen set — EMPTY as of P04.S02. It stays as a list rather than a count so that
// a fix and a new defect cannot cancel out; a count would sit still while one site was
// pinned and another introduced.
const KNOWN_UNPINNED = [];

// With the frozen list empty, "no unpinned sites" is the pass — and it is also what a
// broken scanner reports. So the scanner is checked against a KNOWN-BAD input rather
// than against the real source: a synthetic function that is unmistakably corrupting.
// This is the stimulus test the empty list makes necessary.
test('the scan detects a corrupting site — its own stimulus', () => {
  const bad = `
async function synthetic() {
  const bytes = await bakedBytes();
  const res = await apiFetch('/api/save', { method: 'POST', body: bytes });
}`;
  const found = scanUnpinned(bad).map((f) => f.name);
  assert.deepEqual(found, ['synthetic'],
    'the scanner does not detect an obviously corrupting site, so its silence on the real source means nothing');

  // And it must NOT flag the shape that is safe, or it would be unusable and the
  // frozen list would fill with functions that are fine.
  const good = `
async function safe() {
  const res = await apiFetch('/api/save', { method: 'POST' });
}`;
  assert.deepEqual(scanUnpinned(good), [],
    'the scanner flags a call that IS its own first await — the false positive that put eight safe functions on the frozen list');
});

test('no mutating call is unpinned', () => {
  const found = scanUnpinned(APP).map((f) => f.name);
  const unexpected = [...new Set(found)].filter((n) => !KNOWN_UNPINNED.includes(n));
  assert.deepEqual(unexpected, [],
    'an unpinned mutating call — its payload predates the id it is addressed with, so it acts on whatever document is current when the request goes out');
});

test('save() is pinned, and reads nothing about the document after its first await', () => {
  const body = APP.slice(APP.indexOf('async function save()'));
  const end = body.indexOf('\n}\n');
  const fn = body.slice(0, end);

  assert.match(fn, /const doc = docMeta;/,
    'save() does not capture its document before awaiting');
  assert.match(fn, /docId: doc\.id,/,
    'save() posts without naming the captured document — /api/save writes to the ADDRESSED document, so the bytes would land in another file');
  assert.match(fn, /if \(!doc\.canSave\)/,
    'save() reads canSave off the live docMeta after awaiting — it would download a file the user asked to overwrite, or overwrite one they asked to download');
  assert.match(fn, /if \(!docMeta \|\| docMeta\.id !== doc\.id\) \{ toast\('Saved'\); return; \}/,
    'save() updates the badge and reloads without checking the document is still the one it saved — and it must compare IDS, since a fresh meta object for the same document is not the same object');

  // And the capture must come BEFORE the first await, which is the whole property.
  //
  // Comments are stripped first. Without that this reads the word "await" out of the
  // comment explaining the capture and reports the capture as too late — the check
  // failing on its own documentation, which is the same class of error as the guard in
  // registry_test.go that once flagged a doc comment quoting the idiom it policed.
  const code = fn.split('\n').filter((l) => !l.trim().startsWith('//')).join('\n');
  const capture = code.indexOf('const doc = docMeta;');
  const firstAwait = code.indexOf('await');
  assert.ok(capture !== -1, 'the capture line is not in save()');
  assert.ok(firstAwait !== -1, 'save() contains no await — the property under test does not apply, so this is not a pass');
  assert.ok(capture < firstAwait,
    'the capture happens after an await, so it captures whatever the switch already changed');
});

test('apiFetch honours an explicit docId over the current document', () => {
  assert.match(APP, /const pinned = opts\.docId;/,
    'apiFetch does not read the docId its callers pass');
  assert.match(APP, /const hasPin = Object\.prototype\.hasOwnProperty\.call\(opts, 'docId'\);/,
    'apiFetch decides pinning by truthiness, so a captured-but-missing id silently falls back to the CURRENT one — the exact request pinning exists to prevent, arriving through the option meant to stop it');
  assert.match(APP, /if \(hasPin\) \{ if \(pinned\) opts\.headers\['X-Nib-Doc'\] = pinned; \}/,
    'apiFetch ignores the captured id in favour of the current one — which is the defect, not the fix');
  assert.match(APP, /delete opts\.docId;/,
    'docId is forwarded into fetch() as a request option');
});

// The behavioural proof that a captured id actually protects a file lives at tier 1
// (internal/server/pinning_test.go): it posts A's bytes addressed to A while B is
// active and asserts B's file is untouched on disk. That needs a real filesystem and a
// real server, which is exactly the delegation this tier's ceiling describes — jsdom
// can see which header went out, not which file got written.
