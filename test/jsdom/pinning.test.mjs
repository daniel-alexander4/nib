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

// The idiom changed in P05.S01/S02: `docMeta` became `view.docMeta` when document state
// moved onto the view record. This guard was RE-DERIVED to the new idiom rather than
// loosened until it passed — the distinction P03.S02 had to make when the registry
// changed the resolution idiom out from under its guard, and the reason that one is
// worth repeating: a regex widened to stop failing is a guard that has stopped guarding.
test('save() is pinned, and reads nothing about the document after its first await', () => {
  const body = APP.slice(APP.indexOf('async function save()'));
  const end = body.indexOf('\n}\n');
  const fn = body.slice(0, end);

  assert.match(fn, /const doc = view\.docMeta;/,
    'save() does not capture its document before awaiting');
  assert.match(fn, /docId: doc\.id,/,
    'save() posts without naming the captured document — /api/save writes to the ADDRESSED document, so the bytes would land in another file');
  assert.match(fn, /if \(!doc\.canSave\)/,
    'save() reads canSave off the live docMeta after awaiting — it would download a file the user asked to overwrite, or overwrite one they asked to download');
  assert.match(fn, /if \(!view\.docMeta \|\| view\.docMeta\.id !== doc\.id\) \{ toast\('Saved'\); return; \}/,
    'save() updates the badge and reloads without checking the document is still the one it saved — and it must compare IDS, since a fresh meta object for the same document is not the same object');

  // And the capture must come BEFORE the first await, which is the whole property.
  //
  // Comments are stripped first. Without that this reads the word "await" out of the
  // comment explaining the capture and reports the capture as too late — the check
  // failing on its own documentation, which is the same class of error as the guard in
  // registry_test.go that once flagged a doc comment quoting the idiom it policed.
  const code = fn.split('\n').filter((l) => !l.trim().startsWith('//')).join('\n');
  const capture = code.indexOf('const doc = view.docMeta;');
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

// P04.S03 — an export is named for the document it came from.
//
// Not corruption but mislabeling, and it fails in a place that matters: in
// `openSaveAs(await res.blob(), exportBase() + '-cosigned.pdf')` the arguments evaluate
// LEFT TO RIGHT, so the blob resolves before exportBase() runs and the name is taken
// from whatever document is current by then. Worst on the signing names, where the
// filename is how a user tells two documents apart in a workflow whose entire subject
// is which document was signed.
test('every export names its document at operation entry, not at save time', () => {
  // The stimulus: there must BE export sites, or "none of them are late" is a green
  // over an empty population.
  const captures = APP.match(/const exportName = exportBase\(\);/g) || [];
  assert.ok(captures.length >= 15,
    `only ${captures.length} export scopes capture a name — the scan is not reading what it thinks`);

  // And no site may call exportBase() at the point of use. One call is the definition
  // itself; anything else is a name taken after the operation finished.
  //
  // Comments are stripped first. The doc comment ON exportBase quotes the very pattern
  // it warns against, so counting raw occurrences makes this check fail on its own
  // documentation — the third time in this plan that a guard has read prose as code
  // (registry_test.go's idiom scan, and the capture-position check above).
  const CODE = APP.split('\n').filter((l) => !l.trim().startsWith('//')).join('\n');
  const calls = (CODE.match(/exportBase\(\)/g) || []).length;
  const defs = (CODE.match(/function exportBase\(\)/g) || []).length;
  assert.equal(defs, 1, 'exportBase is defined more than once');
  assert.equal(calls - defs, captures.length,
    'an exportBase() call outside a capture — it would name the file for whatever document is current when the export resolves, not the one it came from');
});

test('the signing exports are covered by name', () => {
  // Named explicitly rather than trusted to the count above, because these are the two
  // where a wrong filename is a wrong claim about which document was signed.
  for (const suffix of ['-cosigned.pdf', '-for-signing.pdf']) {
    const line = APP.split('\n').find((l) => l.includes(suffix) && l.includes('openSaveAs'));
    assert.ok(line, `no export site produces ${suffix}`);
    assert.ok(line.includes('exportName'),
      `${suffix} is named from a live read rather than a captured one`);
  }
});

// P05.S04 — the other half of P04's export-name rule, and the half that shipped broken.
//
// D7's rule is "capture the export name at operation entry". Nineteen scopes obey it by
// declaring `const exportName = exportBase();` at the top of the handler that uses it.
// TWO could not, because their flow is split across two handlers: the one that produces
// the artifact (`reduceGo`, `tvFile`) and the one that saves it (`reduceSave`, `tvSave`).
// The rewrite gave the second handler no local entry to capture at, and it read the first
// handler's `const` anyway — a sibling arrow function at module scope, so the identifier
// simply does not resolve.
//
// Both threw `ReferenceError` on every click from P04 until 2026-08-16: "Save reduced PDF"
// and "Save complete proof" did nothing at all. Nothing caught it — `node --check` passes
// on a scope error, tier 2 never drove either flow, and tier 3 drives neither.
//
// The fix carries the captured name forward on a module binding alongside the artifact
// (`reduceName`, `upgradedProofName`) rather than re-deriving it at save time, which would
// name the document that is active when the user clicks rather than the one the bytes came
// from — the same defect P04 exists to close, one step later.
test('no handler reads an export name it did not capture', () => {
  const lines = APP.split('\n');
  // Approximate a handler as the region from the nearest preceding module-level binding.
  // Coarse, deliberately: it over-reports rather than under-reports, and a false positive
  // here is a loud question about a real scope while a false negative is a broken button.
  const starts = lines.reduce((acc, l, i) => {
    if (/^(els\.\w+\.\w+ = |function |const \w+ = (async )?\(|let )/.test(l)) acc.push(i);
    return acc;
  }, [0]);

  const unscoped = [];
  lines.forEach((l, i) => {
    if (!l.includes('exportName') || l.trim().startsWith('//')) return;
    if (/const exportName/.test(l)) return;
    const prev = Math.max(...starts.filter((s) => s <= i));
    if (!lines.slice(prev, i + 1).join('\n').includes('const exportName')) {
      unscoped.push(`${i + 1}: ${l.trim()}`);
    }
  });

  assert.deepEqual(unscoped, [],
    `an export name is read outside the handler that captured it — this throws ReferenceError at click time and node --check cannot see it:\n  ${unscoped.join('\n  ')}`);

  // Stimulus: the scan must actually be reading a population. Nineteen-odd scopes declare
  // it; if that count collapses, the green above is over nothing.
  const declared = (APP.match(/const exportName = exportBase\(\);/g) || []).length;
  assert.ok(declared >= 15,
    `only ${declared} export scopes declare a captured name — the scan is not reading what it thinks`);
});

// P05.S04 — every function called in app.js is one app.js declares.
//
// Written because this slice introduced exactly this defect and carried it past two gates:
// `reloadOpenDoc` was renamed to `openArrivalInNewView` and its one call site was not,
// leaving `await reloadOpenDoc()` on the arrival path. `node --check` passed (a scope error
// is not a syntax error) and all 44 tier-2 tests passed, because nothing drives that path —
// which is precisely the ceiling arrival.test.mjs declares. It was found by reading, which
// is not a process.
//
// This is a HEURISTIC, and it is worth being plain about that: it strips comments and
// string literals, collects declarations and parameters by pattern, and flags calls to bare
// identifiers left over. A real `no-undef` linter would do it properly; this repo has none,
// and the alternative was nothing. Its false-positive direction is a loud named question
// about a real identifier; the false negative it replaces was a dead code path on the one
// flow this slice exists to fix. When a false positive appears, add the name to KNOWN below
// with a reason rather than loosening the scan.
test('every bare function call resolves to something app.js declares', () => {
  let src = APP.replace(/\/\*[\s\S]*?\*\//g, '');       // block comments
  src = src.replace(/(?<!:)\/\/[^\n]*/g, '');              // line comments (not URLs)
  src = src.replace(/'(?:[^'\\\n]|\\.)*'/g, '""');       // string literals
  src = src.replace(/"(?:[^"\\\n]|\\.)*"/g, '""');
  src = src.replace(/`(?:[^`\\]|\\.)*`/g, '""');          // templates

  const declared = new Set();
  const add = (n) => { const t = String(n).trim().split(/[\s=[\]{}:.]/)[0]; if (t) declared.add(t); };
  for (const m of src.matchAll(/(?:^|\n)\s*(?:async\s+)?function\s+([A-Za-z_$][\w$]*)/g)) add(m[1]);
  for (const m of src.matchAll(/(?:^|\n)\s*(?:const|let|var)\s+([^=\n;]+)/g)) m[1].split(',').forEach(add);
  for (const m of src.matchAll(/import\s*\{([^}]*)\}/g)) m[1].split(',').forEach((n) => add(n.split(' as ').pop()));
  for (const m of src.matchAll(/import\s+([A-Za-z_$][\w$]*)\s+from/g)) add(m[1]);   // default imports
  for (const m of src.matchAll(/import\s*\*\s*as\s+([A-Za-z_$][\w$]*)/g)) add(m[1]);
  for (const m of src.matchAll(/\(([^()]*)\)\s*=>/g)) m[1].split(',').forEach(add);
  for (const m of src.matchAll(/function\s*\w*\s*\(([^()]*)\)/g)) m[1].split(',').forEach(add);
  for (const m of src.matchAll(/([\w$]+)\s*=>/g)) add(m[1]);
  for (const m of src.matchAll(/catch\s*\(\s*([\w$]+)/g)) add(m[1]);
  for (const m of src.matchAll(/for\s*\(\s*(?:const|let|var)\s+([\w$]+)/g)) add(m[1]);

  const KEYWORDS = new Set(['if', 'for', 'while', 'switch', 'catch', 'return', 'typeof', 'await',
    'function', 'super', 'new', 'of', 'in', 'do', 'else', 'try', 'throw', 'delete', 'void',
    'yield', 'case', 'async']);
  const BROWSER = new Set(['window', 'document', 'console', 'Math', 'JSON', 'Object', 'Array',
    'String', 'Number', 'Boolean', 'Date', 'Set', 'Map', 'Promise', 'Error', 'RegExp',
    'parseInt', 'parseFloat', 'isNaN', 'isFinite', 'setTimeout', 'clearTimeout', 'setInterval',
    'clearInterval', 'fetch', 'alert', 'confirm', 'prompt', 'encodeURIComponent',
    'decodeURIComponent', 'Uint8Array', 'Blob', 'File', 'FileReader', 'FormData', 'URL',
    'URLSearchParams', 'DOMParser', 'XMLSerializer', 'Image', 'atob', 'btoa', 'structuredClone',
    'requestAnimationFrame', 'cancelAnimationFrame', 'matchMedia', 'getComputedStyle',
    'createImageBitmap', 'Intl', 'BigInt', 'Symbol']);
  // Names the heuristic cannot see, each with why. Not a suppression list to grow casually.
  const KNOWN = new Set([
    'eq',     // a destructured callback parameter in alignPages' options object
    'onFile', // likewise, in the drag-and-drop wiring
  ]);

  const unresolved = new Set();
  for (const m of src.matchAll(/(?<![.\w$])([a-z_$][\w$]*)\s*\(/g)) {
    const n = m[1];
    if (declared.has(n) || KEYWORDS.has(n) || BROWSER.has(n) || KNOWN.has(n)) continue;
    unresolved.add(n);
  }

  assert.deepEqual([...unresolved].sort(), [],
    `called but never declared — a rename that missed a call site throws at run time, and neither node --check nor any tier sees it: ${[...unresolved].sort().join(', ')}`);

  // Stimulus: the scan must be reading a real population, or the green above is over an
  // empty set — which is what a broken strip step would silently produce.
  assert.ok(declared.size > 400, `only ${declared.size} declarations found — the scan is not reading app.js properly`);
});
