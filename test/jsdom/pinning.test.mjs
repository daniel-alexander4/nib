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
//
// Corrected, and then pinned against the server so it cannot drift again. Three of
// the fifteen entries this replaces — /api/export, /api/sign, /api/stamp — existed
// in neither the mux nor web/app.js, so the list read 20% more complete than it was.
// One more named route that does not mutate the stored document (/api/flags is
// byte-in/byte-out: it writes the result into the response and never touches the
// registry), and
// '/api/attachments' was the read route; the mutating one is '/api/attachments/add'.
//
// Membership is "a misaddressed request DAMAGES the addressed document" — which is
// mostly "the handler commits into it" (commitMutation, commitBarrier, or a direct
// doc.data write under the lock), and as of P06.S02 also `/api/close-view`, which
// destroys it outright. The old wording said "commits into", and a close commits
// nothing; it is nonetheless the route where getting the address wrong costs the user
// the most, so the membership rule is stated by consequence rather than by mechanism.
//
// **`/api/assemble` was excluded on a reason that was false, and it is in the list now**
// (/pending 261, 2026-08-23). The exclusion said it "never reaches commitMutation" — true, and
// beside the point: it reaches `commitBarrier` (export.go), on the `reload=1` branch that loads
// the flattened result back as the open document. The membership rule two paragraphs up already
// names commitBarrier, so the exclusion contradicted the rule it sat under. All three client call
// sites happen to pass a docId today, so nothing was broken — what was missing is the guard that
// makes a FOURTH one fail. `/api/flags`, which the same sentence excluded, genuinely is
// byte-in/byte-out and stays out.
const MUTATING = [
  '/api/save', '/api/pages', '/api/redact', '/api/outline', '/api/ocr',
  '/api/sanitize', '/api/decrypt', '/api/attachments/add', '/api/undo', '/api/redo',
  '/api/close-view', '/api/assemble',
];

test('every route in the MUTATING inventory is a real POST route on the server', () => {
  // The V2 shape: a hand-kept inventory reconciled against nothing polices nothing.
  // Three of this list's entries used to name routes that existed nowhere — not in
  // the mux, not in web/app.js — and no assertion could notice, because a scan for
  // a route that is never called finds no unpinned call sites and reports clean.
  // Pinned against the server's own mux, which is the external source the count
  // has to come from; a typo or a renamed route now fails by name.
  const mux = fs.readFileSync(path.join(REPO, 'internal', 'server', 'server.go'), 'utf8');
  for (const route of MUTATING) {
    assert.ok(mux.includes(`"POST ${route}"`),
      `${route} is in the MUTATING inventory but is not a POST route in server.go — the list has drifted from the server`);
  }
});

// scanUnpinned finds every apiFetch on a mutating route that is preceded by an `await`
// in its own function and does not pass an explicit `docId`. That is the corruption
// channel, defined mechanically rather than by inspection.
function scanUnpinned(src) {
  const funcs = [];
  const header = /^(?:async function (\w+)|const (\w+) = async|\s*async (\w+)\()/gm;
  let m;
  while ((m = header.exec(src)) !== null) {
    const name = m[1] || m[2] || m[3];
    // The body's opening brace, which is NOT simply the next `{`. A default
    // parameter puts one inside the parameter list — `async function pageOp(op,
    // extra = {})` — and taking the first brace captured `{}` as the entire body.
    //
    // That is not a hypothetical: pageOp is the single entry point for twenty
    // document operations and was unpinned, and this scanner reported clean,
    // because it never read a line of it. The guard whose whole job is to catch an
    // unpinned mutating call could not see the most dangerous one in the file.
    // Walk the parameter list to its matching close paren first, then take the
    // brace after it.
    const lp = src.indexOf('(', m.index);
    if (lp === -1) continue;
    let pd = 0, afterParams = -1;
    for (let j = lp; j < src.length; j++) {
      if (src[j] === '(') pd++;
      else if (src[j] === ')') { pd--; if (pd === 0) { afterParams = j; break; } }
    }
    if (afterParams === -1) continue;
    const open = src.indexOf('{', afterParams);
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

  // The shape that defeated it: an object default parameter. This is pageOp's
  // signature, and while the body-finder took the first `{` the scanner read `{}`
  // as the whole function and reported clean over twenty unpinned operations.
  const defaulted = `
async function withDefault(op, extra = {}) {
  const form = await bakedForm();
  const res = await apiFetch('/api/pages', { method: 'POST', body: form });
}`;
  assert.deepEqual(scanUnpinned(defaulted).map((f) => f.name), ['withDefault'],
    'a function with an object default parameter is invisible to the scanner — its body was never read');

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

// ---------------------------------------------------------------------------
// The RELOAD half of the same law (ADR-001), and the half that had no guard.
//
// scanUnpinned above asks which document a request is ADDRESSED to. This asks which
// view the response is INSTALLED into — `setDocumentFromServer(meta, target)` writes
// target.docMeta, bumps target.docGen, runs resetSharedDocState (wiping that view's
// overlays, redact marks and undo stack) and repoints its viewer. A call that omits
// the target writes into whatever view is active when the round-trip RETURNS.
//
// The two halves fail independently, which is why one guard could not cover both and
// why this gap survived the pass that closed the other: /api/sanitize, /api/decrypt
// and /api/assemble are each called with no preceding await, so the request half is
// correct by construction and scanUnpinned rightly said nothing — while the reload
// landing seconds later was unpinned. Sixteen of twenty-one call sites were fixed in
// v1.105.14/15 and five were left, with nothing able to notice.
//
// Pinned against the mux for the same reason MUTATING is: a route list reconciled
// against nothing polices nothing.
const INSTALL_ROUTES = [
  '/api/open', '/api/open-url', '/api/upload', '/api/combine', '/api/office',
];

// scanUntargetedReloads finds every setDocumentFromServer call that passes no explicit
// target and whose response did not come from a document-INSTALLING route.
//
// Route rather than function name, deliberately. Five of these live in anonymous
// `els.x.onclick = async () => {}` handlers that a function-header scan cannot name,
// and a hand list of excused names is the V2 shape: it would have to be updated
// whenever a handler is renamed, and it fails silently when it is not. What actually
// makes a call legitimate is what the response IS — an Open replaces the document you
// are looking at, by definition — and that is a property of the route.
function scanUntargetedReloads(src) {
  const out = [];
  const call = /(?<![.\w$])setDocumentFromServer\(/g;
  let m;
  while ((m = call.exec(src)) !== null) {
    // The declaration is not a call site.
    if (/function\s+$/.test(src.slice(Math.max(0, m.index - 24), m.index))) continue;
    const lp = m.index + 'setDocumentFromServer'.length;
    let d = 0, end = -1;
    for (let j = lp; j < src.length; j++) {
      if (src[j] === '(') d++;
      else if (src[j] === ')') { d--; if (d === 0) { end = j; break; } }
    }
    if (end === -1) continue;
    // A comma at argument depth means a target was passed. Depth-aware, because the
    // first argument is routinely a call or a ternary carrying commas of its own.
    const args = src.slice(lp + 1, end);
    let depth = 0, targeted = false;
    for (const ch of args) {
      if (ch === '(' || ch === '[' || ch === '{') depth++;
      else if (ch === ')' || ch === ']' || ch === '}') depth--;
      else if (ch === ',' && depth === 0) { targeted = true; break; }
    }
    if (targeted) continue;
    // The lookback stops at the enclosing function, not at a fixed character count.
    //
    // A bare 1500-character window reaches into the function ABOVE and attributes its
    // route to this call — observed: installOpened's reload was blamed on the
    // `/api/close` in requestClose, thirty lines earlier. A guard that names the wrong
    // route is worse than one that names none, because the exemption list is keyed on
    // the route: the next such misattribution could land on an install route and
    // silently excuse a real defect.
    const window = src.slice(Math.max(0, m.index - 1500), m.index);
    const fnStart = Math.max(
      window.lastIndexOf('function '),
      window.lastIndexOf('=> {'),
      window.lastIndexOf('async () => {'),
    );
    const before = fnStart === -1 ? window : window.slice(fnStart);
    const routes = [...before.matchAll(/apiFetch\(\s*'([^']+)'/g)];
    const route = routes.length ? routes[routes.length - 1][1].split('?')[0] : '(none)';
    if (INSTALL_ROUTES.includes(route)) continue;
    out.push({ route, line: src.slice(0, m.index).split('\n').length });
  }
  return out;
}

test('every install route in the exemption list is a real POST route on the server', () => {
  const mux = fs.readFileSync(path.join(REPO, 'internal', 'server', 'server.go'), 'utf8');
  for (const route of INSTALL_ROUTES) {
    assert.ok(mux.includes(`"POST ${route}"`),
      `${route} exempts a reload site but is not a POST route in server.go — the exemption list has drifted from the server`);
  }
});

test('the reload scan detects an untargeted reload — its own stimulus', () => {
  const bad = `
els.sanitizeBtn.onclick = async () => {
  const res = await apiFetch('/api/sanitize', { method: 'POST' });
  await setDocumentFromServer(await res.json());
};`;
  assert.equal(scanUntargetedReloads(bad).length, 1,
    'the scanner does not detect an obviously untargeted reload, so its silence on the real source means nothing');

  // And it must not flag the two shapes that are correct, or it is unusable.
  const targeted = `
  const res = await apiFetch('/api/sanitize', { method: 'POST' });
  await setDocumentFromServer(await res.json(), owner);`;
  assert.deepEqual(scanUntargetedReloads(targeted), [],
    'a call that names its target is flagged — the false positive that would force the exemption list to grow');

  const install = `
  const res = await apiFetch('/api/open', { method: 'POST' });
  await setDocumentFromServer(await res.json());`;
  assert.deepEqual(scanUntargetedReloads(install), [],
    'an Open is flagged — installing a new document into the active view is what Open MEANS');

  // The definition itself must not read as a call site, or the scan reports a
  // permanent finding nobody can fix and gets suppressed.
  assert.deepEqual(scanUntargetedReloads('async function setDocumentFromServer(meta, target = view) {}'), [],
    'the declaration is being counted as a call site');
});

test('every reload names the view it lands in', () => {
  // Stimulus: there must BE call sites, or the emptiness below is a scan reading nothing.
  //
  // The floor was 20 and is now 15, and the number moved for a reason rather than
  // because the guard was widened until it passed — which is the failure mode a floor
  // invites. P06.S01 folded the five user-open paths (open, upload, office, open-url,
  // combine) into installOpened, and the arrival's build-load-activate body into
  // openInNewView, so 21 call sites became 17. The floor is what NOTICED: it went red on
  // the first run after the refactor, which is what a population probe is for.
  const sites = APP.match(/(?<![.\w$])setDocumentFromServer\(/g) || [];
  assert.ok(sites.length >= 15,
    `only ${sites.length} setDocumentFromServer sites found — the scan is not reading app.js properly`);

  const found = scanUntargetedReloads(APP);
  assert.deepEqual(found, [],
    `a reload with no target: it installs the response into whatever view is active when the round-trip returns, wiping that view's overlays, redact marks and undo stack — ${found.map((f) => `${f.route} at app.js:${f.line}`).join(', ')}`);
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
