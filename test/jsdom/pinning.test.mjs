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
  // text-reflow P06.S05: re-sets a paragraph and commits it through commitMutation.
  '/api/reflow',
  // P07.S02a (v1.117.155). Convene commits new bytes into the open document — a readme,
  // N signature pages, a ceremony page and the embedded record — so it is a mutating route
  // like any other and must be driven by the misaddressed-document guard below.
  //
  // It is also the one where the pin matters most: convene is multi-second, and docFor
  // falls back to the ACTIVE document when no X-Nib-Doc is present, so an unpinned convene
  // would commit a ceremony record into whichever tab the user switched to while it ran.
  '/api/ceremony/convene',
  // /pending 333's remedy half. It commits the file's bytes into the open document
  // through commitMutation, so it is a mutating route by the rule above.
  //
  // **It is also the only entry here that fires without the user doing anything** — the
  // return-to-foreground check reloads a clean document by itself — so docFor's fallback
  // to the ACTIVE document is not a theoretical mis-address for this route: an unpinned
  // call would reload the file underneath whichever tab the user had switched to.
  '/api/reload',
  // /pending 498. Both commit through `commitMutation` (tags.go) and were missing from this list
  // since P09 added them — so a fourth call site dropping its pin would have passed. Nothing was
  // broken (both sites pass `docId`); what was missing is the reverse check below that makes the
  // NEXT committing route impossible to leave out.
  '/api/tags/commit', '/api/tags/edit', '/api/tags/remove',
  // **`/api/ceremony/accept` is deliberately NOT here (P07.S02b).** The membership rule is
  // "commits into or destroys a document", and accept does neither: it parses an invitation and
  // writes a vault pin, carries no X-Nib-Doc, and cannot touch any document's bytes. Listing it
  // would put it in front of the misaddressed-document guard, which would then be asserting a
  // pin rule about a route with nothing to pin — and it would owe the ceremony freeze a routing
  // it has no reason to have. Named here rather than merely absent, because `/api/assemble` was
  // left out of this list on a reason that turned out to be false, and an absence with no
  // sentence is indistinguishable from an oversight.
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

// The REVERSE direction, and the one that let two routes sit outside MUTATING (/pending 498).
//
// The test above says every name in the list is a real route; nothing said every route that
// commits into a document is in the list. `/api/tags/commit` and `/api/tags/edit` both commit
// through `commitMutation` and were absent, so an unpinned call to either would have passed.
// Membership is read from the server: a POST route whose handler calls `commitMutation`,
// `commitBarrier`, or writes `doc.data` directly (handleSave, undo/redo) must be listed.
//
// Ceiling, stated: one level deep. A handler that commits through a helper of its own is not seen;
// every committing handler today calls the door directly, and the floor below is what notices if
// that stops being true (the count drops).
function committingRoutes(mux, goFiles) {
  const bodies = new Map();
  for (const src of goFiles) {
    for (const m of src.matchAll(/^func \(s \*Server\) (handle\w+)\([^]*?^\}$/gm)) bodies.set(m[1], m[0]);
  }
  const out = [];
  for (const m of mux.matchAll(/"POST (\/api\/[^"]+)",[^\n]*?s\.(handle\w+)\)/g)) {
    const body = bodies.get(m[2]) || '';
    if (/\bs\.commitMutation\(|\bs\.commitBarrier\(|\bdoc\.data = /.test(body)) out.push(m[1]);
  }
  return out;
}

test('every route whose handler commits into a document is in the MUTATING inventory', () => {
  const dir = path.join(REPO, 'internal', 'server');
  const mux = fs.readFileSync(path.join(dir, 'server.go'), 'utf8');
  const goFiles = fs.readdirSync(dir).filter((f) => f.endsWith('.go') && !f.endsWith('_test.go'))
    .map((f) => fs.readFileSync(path.join(dir, f), 'utf8'));

  // Stimulus first: a planted committing handler must be found, or silence below means nothing.
  const planted = committingRoutes(
    'mux.HandleFunc("POST /api/planted", s.requireUnlocked(s.handlePlanted))',
    ['func (s *Server) handlePlanted(w http.ResponseWriter, r *http.Request) {\n\tif err := s.commitMutation(doc, a, b, false); err != nil {\n\t}\n}'],
  );
  assert.deepEqual(planted, ['/api/planted'], 'the reverse check cannot find a committing handler it was handed');

  const committing = committingRoutes(mux, goFiles);
  assert.ok(committing.length >= 14,
    `only ${committing.length} committing routes found — the handler scan is not reading internal/server`);
  const missing = committing.filter((r) => !MUTATING.includes(r));
  assert.deepEqual(missing, [],
    `these routes commit into a document and are not in MUTATING, so an unpinned call to them passes the guard: ${missing.join(', ')}`);
});

// scanUnpinned finds every apiFetch that MUTATES a document without naming which one.
//
// **The `await` condition is gone, and removing it is /pending 410.** The scan used to require an
// await that completes before the call is built — "capture, then await, then post" — and that is
// one corruption channel of two. The other is a HUMAN: the user opens a form, fills it in for
// minutes, and clicks Submit, with `#tabstrip` live the whole time. No scanner shaped around
// control flow can represent that window, and `/api/ceremony/convene` proved it — the route sat in
// the inventory below since P07.S02a with a comment naming this exact defect, and was unpinned
// anyway until P03.S04, because in `conveneFromPanel` the convene POST IS the first await.
//
// **So the rule is now unconditional: a mutating POST names its document.** That is strictly
// stronger, and it was affordable — measured against HEAD before it was written, the whole file
// holds 15 apiFetch calls on mutating routes, 14 POSTs and one GET, and every one of the 14 already
// passes `docId`. A rule with no exemptions needs no argument about which windows are wide enough.
//
// **Two scanner defects had to go with it, and both were masked by the await condition.**
// The `docId` test read a fixed 400-character tail, which `conveneFromPanel`'s own pin sits past —
// so relaxing the condition alone would have reported a pinned site. And a mutating ROUTE is not a
// mutating CALL: `openOutlineEditor` GETs `/api/outline`. The options object is now brace-matched
// to its close, and a call is mutating when it carries a `method:` that is not literally `'GET'` —
// no `method:` at all is `apiFetch`'s GET default, and a method the scan cannot read is treated as
// mutating rather than waved through.
//
// **Every `apiFetch(` in the file, not every one inside a recognised function header (/pending
// 498).** The scan used to find functions by three header shapes — `async function X`, `const X =
// async`, `async X(` — and read only their bodies. `els.x.onclick = async () => {` matches none of
// them, and that is the shape of Flatten (two `/api/assemble` posts), Attach a file, and Save
// outline: 35 of 118 call sites, four of them mutating posts, were never read. All four happen to
// carry `docId`; nothing would have said so the day one did not. The header is now used only to
// NAME the site in a report — the call sites themselves are found by the call, wherever it is.
//
// And the route is read from any string in the first argument, not only a leading `'literal'`:
// `apiFetch(gs ? '/api/pdfa?engine=gs' : '/api/pdfa', …)` and a template both hide their route
// from a quote-anchored match. A mutating POST whose first argument holds NO literal the scan can
// read is reported, because "I could not tell which route" must not read as "not a mutating one".
function siteName(src, index) {
  const header = /^\s*(?:export\s+)?(?:async\s+)?function\s+(\w+)|^\s*(?:const|let)\s+(\w+)\s*=\s*(?:async\s*)?\(|^\s*async\s+(\w+)\(|^\s*els\.(\w+)\.(\w+)\s*=/gm;
  let name = '(top level)', m;
  while ((m = header.exec(src)) !== null && m.index < index) {
    name = m[1] || m[2] || m[3] || `${m[4]}.${m[5]}`;
  }
  return name;
}

function apiFetchSites(src) {
  const sites = [];
  const call = /(?<![\w$.])apiFetch\(/g;
  let c;
  while ((c = call.exec(src)) !== null) {
    if (/function\s+$/.test(src.slice(Math.max(0, c.index - 24), c.index))) continue; // the definition
    // The call's OWN arguments, paren-matched from `apiFetch(` to its close. The predecessor read a
    // fixed 400-character tail, which runs off the end of a call whose options carry a comment —
    // `conveneFromPanel`'s pin sits at character 700 of its own call — and runs INTO the next call
    // on a short one.
    let pd = 0, args = '', firstEnd = -1;
    const open = c.index + 'apiFetch'.length;
    for (let j = open; j < src.length; j++) {
      const ch = src[j];
      if (ch === '(' || ch === '[' || ch === '{') pd++;
      else if (ch === ')' || ch === ']' || ch === '}') { pd--; if (pd === 0) { args = src.slice(c.index, j + 1); break; } }
      else if (ch === ',' && pd === 1 && firstEnd === -1) firstEnd = j;
    }
    if (!args) args = src.slice(c.index);
    const first = src.slice(open + 1, firstEnd === -1 ? c.index + args.length - 1 : firstEnd);
    const routes = [...first.matchAll(/(['"`])(\/api\/[^'"`?$]*)/g)].map((x) => x[2]);
    sites.push({ index: c.index, args, routes, line: src.slice(0, c.index).split('\n').length });
  }
  return sites;
}

function scanUnpinned(src) {
  const out = [];
  for (const site of apiFetchSites(src)) {
    const name = siteName(src, site.index);
    // Exact, now that the query string is cut at `?` by the route reader: a prefix match would let
    // a future `/api/pages-preview` inherit `/api/pages`'s membership by spelling alone.
    const mutating = site.routes.filter((route) => MUTATING.includes(route));
    const opaque = site.routes.length === 0;
    if (!mutating.length && !opaque) continue;
    const route = mutating[0] || '(unreadable route)';
    {
      let args = site.args;
      // **Comments stripped, and this was found by a mutation probe going green.** Replacing the
      // pin with `// docId removed` left the site reading as pinned, because the shorthand test is
      // a word match and the word was in a comment. Every options object in this file carries
      // comments, so it is not a contrived shape — and a scan that a comment can satisfy is one a
      // future author disables by explaining what they took out.
      args = args.replace(/\/\*[\s\S]*?\*\//g, ' ').replace(/\/\/[^\n]*/g, ' ');
      // A mutating ROUTE reached by a GET is not a mutating CALL — `openOutlineEditor`
      // reads `/api/outline`. `apiFetch` defaults to GET, so no `method:` key is a read;
      // anything else, INCLUDING a method this scan cannot resolve to a literal, is
      // treated as a write, because "I could not tell" must not read as "it is safe".
      const method = /method:\s*'([A-Z]+)'/.exec(args);
      if (!/method\s*:/.test(args)) continue;
      if (method && method[1] === 'GET') continue;
      // Both forms count: `docId: expr` and the ES6 shorthand `docId`. Missing the
      // shorthand made three pinned helpers read as unpinned.
      if (/\bdocId\s*[,:}\s]/.test(args)) continue;
      out.push({ name, route, line: site.line });
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

  // **/pending 410's own shape, and the one the scan was blind to.** No await completes before
  // this call — the window is a HUMAN holding a form open — and that is exactly how
  // `/api/ceremony/convene` stayed unpinned for two phases while sitting in the inventory with a
  // comment naming the defect. Under the old `await`-preceded condition this fixture read as safe.
  const userPause = `
async function submitHandler() {
  const res = await apiFetch('/api/ceremony/convene', { method: 'POST', body: form });
}`;
  assert.deepEqual(scanUnpinned(userPause).map((f) => f.name), ['submitHandler'],
    'a mutating POST whose only window is the user filling in a form reads as safe — which is how '
    + 'the convene route sat in this inventory, with a comment naming this exact defect, and was '
    + 'unpinned anyway until P03.S04');

  // And it must NOT flag what is actually safe, or it would be unusable and the frozen list would
  // fill with functions that are fine. Two shapes, because the rule now has two ways to be met.
  const pinned = `
async function safe() {
  const res = await apiFetch('/api/save', { method: 'POST', docId: captured });
}`;
  assert.deepEqual(scanUnpinned(pinned), [],
    'a call that names its document is being reported, so the rule cannot be satisfied at all');

  const read = `
async function reader() {
  const res = await apiFetch('/api/outline');
}`;
  assert.deepEqual(scanUnpinned(read), [],
    'a GET on a mutating route is being reported as an unpinned write — openOutlineEditor reads '
    + '/api/outline, and a mutating ROUTE is not a mutating CALL');

  // A method the scan cannot resolve to a literal is treated as a WRITE. "I could not tell" must
  // not read as "it is safe" — that is how a scan reports clean over a site it never understood.
  const opaque = `
async function opaque() {
  const res = await apiFetch('/api/save', { method: verb, body: bytes });
}`;
  assert.deepEqual(scanUnpinned(opaque).map((f) => f.name), ['opaque'],
    'a call whose method the scan cannot read is being waved through, so any future site can '
    + 'become invisible by building its options a little differently');

  // A COMMENT must not satisfy the pin. Found the way these things are found: a mutation probe
  // that removed the real pin and wrote `// docId removed` in its place went green.
  const commented = `
async function commented() {
  const res = await apiFetch('/api/save', { method: 'POST', /* docId dropped on purpose */ body: b });
}`;
  assert.deepEqual(scanUnpinned(commented).map((f) => f.name), ['commented'],
    'a comment mentioning docId satisfies the pin test, so a site is exempted by explaining '
    + 'itself — and every options object in this file carries comments');

  // **/pending 498's shape: the handler no header matched.** Flatten, Attach and Save outline are
  // all written like this, and the header-bounded scan read none of them.
  const handler = `
els.flattenBtn.onclick = async () => {
  const pages = await render();
  const res = await apiFetch('/api/assemble', { method: 'POST', body: form });
};`;
  assert.deepEqual(scanUnpinned(handler).map((f) => f.name), ['flattenBtn.onclick'],
    'an unpinned mutating POST inside `els.x.onclick = async () =>` is invisible — 35 of the file\'s '
    + 'call sites are written that way, four of them mutating');

  // A route chosen by a ternary, or built in a template, hides from a quote-anchored match.
  const ternary = `
async function pick() {
  const res = await apiFetch(fast ? '/api/redact?fast=1' : '/api/redact', { method: 'POST', body: b });
}`;
  assert.deepEqual(scanUnpinned(ternary).map((f) => f.route), ['/api/redact'],
    'a mutating route selected by a ternary is not read, so the call is waved through');
  const tmpl = `
async function tmpl() {
  const res = await apiFetch(\`/api/pages?op=\${op}\`, { method: 'POST', body: b });
}`;
  assert.deepEqual(scanUnpinned(tmpl).map((f) => f.route), ['/api/pages'],
    'a mutating route written as a template literal is not read');

  // And a POST whose route the scan cannot read at all is reported, not assumed harmless.
  const opaqueRoute = `
async function opaqueRoute() {
  const res = await apiFetch(url, { method: 'POST', body: b });
}`;
  assert.deepEqual(scanUnpinned(opaqueRoute).map((f) => f.route), ['(unreadable route)'],
    'a POST to a route the scan cannot read is waved through');
});

test('no mutating call is unpinned', () => {
  // The population, asserted before the verdict: an empty report is also what a scan reading
  // nothing produces. Three floors, one per way this scan has already been blind.
  const sites = apiFetchSites(APP);
  assert.ok(sites.length >= 100,
    `only ${sites.length} apiFetch call sites found — the file has well over a hundred, so the scan is not reading it`);
  const mutatingPosts = sites.filter((s) => s.routes.some((r) => MUTATING.includes(r)) && /method\s*:/.test(s.args)
    && !/method:\s*'GET'/.test(s.args));
  assert.ok(mutatingPosts.length >= 20,
    `only ${mutatingPosts.length} mutating POST sites found — the scan is not seeing the file's document operations`);
  // …and specifically the ones the header-bounded scan could not see (/pending 498).
  const inHandlers = mutatingPosts.filter((s) => siteName(APP, s.index).includes('.'));
  assert.ok(inHandlers.length >= 4,
    `only ${inHandlers.length} mutating POST sites inside \`els.x.y = async () =>\` handlers — the scan has gone back to reading function headers only`);

  const found = scanUnpinned(APP);
  const unexpected = found.filter((f) => !KNOWN_UNPINNED.includes(f.name));
  assert.deepEqual(unexpected.map((f) => `${f.name} → ${f.route} at app.js:${f.line}`), [],
    'an unpinned mutating call — its payload predates the id it is addressed with, so it acts on whatever document is current when the request goes out');
});

// ---------------------------------------------------------------------------
// The READ half of the request law (/pending 806, 625, 607).
//
// MUTATING is "a misaddressed request DAMAGES the addressed document", and that left every route
// that merely READS one outside any guard — and a read can do the damage one step later. An
// attachment id from document A's list (`page:1:0` repeats across documents) extracted with B's
// header saved B's embedded file under A's name; `/api/split-pages` commits nothing, but its only
// guard against a part overwriting the source is the ADDRESSED document's path. Both were sent with
// whatever was current at send time.
//
// **So membership is read from the server, not listed:** a route is document-scoped when its handler
// resolves a document (`s.resolveDoc(` or `s.docFor(`) itself or through one `s.helper(` of its own
// (`hopTarget`). Every apiFetch to such a route names its document with `docId`, GETs included, or
// is a named exemption below with its reason. `unpinned: true` does not satisfy it — on a document
// route that IS the unaddressed request — except where the exemption says so.
function documentRoutes(mux, goFiles) {
  const bodies = new Map();
  for (const src of goFiles) {
    for (const m of src.matchAll(/^func \(s \*Server\) (\w+)\([^]*?^\}$/gm)) bodies.set(m[1], m[0]);
  }
  const resolves = (name, depth) => {
    const body = bodies.get(name);
    if (!body) return false;
    if (/\bs\.(?:resolveDoc|docFor)\(/.test(body)) return true;
    return depth > 0 && [...body.matchAll(/\bs\.(\w+)\(/g)].some((m) => m[1] !== name && resolves(m[1], depth - 1));
  };
  const out = new Set();
  for (const m of mux.matchAll(/"(?:GET|POST|DELETE|PUT|HEAD) (\/api\/[^"]+)",[^\n]*?s\.(handle\w+)\)/g)) {
    if (resolves(m[2], 1)) out.add(m[1]);
  }
  return [...out];
}

// route → [site, reason]. A site is the name `siteName` gives it, so a rename fails here by name.
const DOCUMENT_ROUTE_EXEMPT = [
  ['/api/close', 'requestClose',
    'Close is CLOSE ALL (handleClose: setDoc(nil)); the header only lets a stale id 409, and the current id is the one the confirm was about'],
  ['/api/doc', 'openArrivalInNewView',
    'the one session question apiFetch\'s `unpinned` exists for — "what is active NOW" — which a pinned call cannot ask'],
];

function scanUnpinnedDocumentReads(src, routes) {
  const out = [];
  for (const site of apiFetchSites(src)) {
    const hit = site.routes.filter((r) => routes.includes(r));
    if (!hit.length) continue;
    const args = site.args.replace(/\/\*[\s\S]*?\*\//g, ' ').replace(/\/\/[^\n]*/g, ' ');
    if (/\bdocId\s*[,:}\s]/.test(args)) continue;
    const name = siteName(src, site.index);
    if (DOCUMENT_ROUTE_EXEMPT.some(([r, n]) => r === hit[0] && n === name)) continue;
    out.push({ name, route: hit[0], line: site.line });
  }
  return out;
}

test('every call to a document-scoped route names its document — reads included', () => {
  const dir = path.join(REPO, 'internal', 'server');
  const mux = fs.readFileSync(path.join(dir, 'server.go'), 'utf8');
  const goFiles = fs.readdirSync(dir).filter((f) => f.endsWith('.go') && !f.endsWith('_test.go'))
    .map((f) => fs.readFileSync(path.join(dir, f), 'utf8'));

  // Stimulus: a planted handler resolving through a helper, and an unpinned read of it, are found.
  const planted = documentRoutes(
    'mux.HandleFunc("GET /api/planted", s.requireUnlocked(s.handlePlanted))',
    ['func (s *Server) handlePlanted(w http.ResponseWriter, r *http.Request) {\n\ts.target(w, r)\n}',
      'func (s *Server) target(w http.ResponseWriter, r *http.Request) {\n\tdoc, ok := s.resolveDoc(w, r)\n}'],
  );
  assert.deepEqual(planted, ['/api/planted'], 'the route reader cannot find a handler that resolves through a helper');
  assert.deepEqual(
    scanUnpinnedDocumentReads("async function r() {\n  const res = await apiFetch('/api/planted', { unpinned: true });\n}", planted)
      .map((f) => f.name), ['r'],
    'an unpinned read of a document route is waved through — `unpinned: true` must not count as a pin');

  const routes = documentRoutes(mux, goFiles);
  // The population first: an empty report is also what a reader that found no routes produces.
  for (const r of ['/api/attachments', '/api/attachments/extract', '/api/split-pages', '/api/ceremony/hop', '/api/save']) {
    assert.ok(routes.includes(r), `${r} resolves a document but the route reader did not find it — the scan is blind`);
  }
  for (const [r, n] of DOCUMENT_ROUTE_EXEMPT) {
    assert.ok(routes.includes(r), `the exemption for ${r} names a route that is no longer document-scoped — remove it`);
    assert.ok(apiFetchSites(APP).some((s) => s.routes.includes(r) && siteName(APP, s.index) === n),
      `the exemption ${r} @ ${n} names a call site that no longer exists — remove it`);
  }
  const found = scanUnpinnedDocumentReads(APP, routes);
  assert.deepEqual(found.map((f) => `${f.name} → ${f.route} at app.js:${f.line}`), [],
    'a call to a document-scoped route that does not name its document — it is answered about whichever tab is current when it goes out');
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

// ── /pending 791 — every reload is behind the overlay-loss door, or is named ──────────────────────
// `setDocumentFromServer` empties the view's placed items, its redaction boxes and (with the old pdf.js
// document) what was typed into the form. Nine operations did that with no question. The rule is ONE door,
// `confirmOverlayLoss`, and this checks ROUTING (ADR-009): the function a reload sits in calls the door
// before it reloads, or is in the list below with its reason. A tenth operation added without the door is
// a name this test has not been told about.
//
// It cannot see: whether the door is called on the right view, or early enough to stop the REQUEST (a call
// after the request would pass). The behaviour is held in autoprereq.test.mjs.
const NO_OVERLAY_DOOR = {
  openInNewView: 'a new view: nothing was there to lose',
  installOpened: 'the first document into an empty view',
  reloadFromDisk: 'its one caller is the automatic reload, taken only when hasUnsavedWork is false',
  save: 'bakes everything and writes it: nothing placed or typed is lost',
  doUndo: 'declined #47: Undo discarding overlay edits is deliberate',
  doRedo: 'declined #47, as Undo',
  'els.staleRetry.onclick': 'loads again a document that failed to load',
  'els.applyRedactBtn.onclick': 'applies the boxes, bakes the rest, and asks its own question',
  sessionInit: 'bakes before it signs; a redaction box not yet applied still goes (residue, in the ceremony)',
};
// Owed, not exempt: the tag editor's three senders lose the same edits and were left alone the night the
// door was built (another change was in those functions). Each needs `if (!confirmOverlayLoss(owner)) return;`
// before its request; when one has it, this test says to take its name out.
const OVERLAY_DOOR_OWED = ['commitTags', 'sendTagEdits', 'removeAllTags'];

function reloadSites(src) {
  const code = src.split('\n').map((l) => (l.trim().startsWith('//') ? '' : l.replace(/\s\/\/ .*$/, ''))).join('\n');
  const heads = [...code.matchAll(/^(?:async function|function|els\.|const |let )[^\n]*/gm)];
  const out = [];
  const call = /(?<![.\w$])setDocumentFromServer\(/g;
  let m;
  while ((m = call.exec(code)) !== null) {
    if (/function\s+$/.test(code.slice(Math.max(0, m.index - 24), m.index))) continue; // the declaration
    const head = heads.filter((h) => h.index <= m.index).pop();
    const line = head ? head[0] : '';
    const named = /function\s+(\w+)/.exec(line) || /^(els\.\w+\.\w+)/.exec(line) || /^(?:const|let)\s+(\w+)/.exec(line);
    out.push({
      name: named ? named[1] : '(top level)',
      door: /(?<![.\w$])confirmOverlayLoss\(/.test(code.slice(head ? head.index : 0, m.index)),
      line: code.slice(0, m.index).split('\n').length,
    });
  }
  return out;
}

test('the overlay-door scan reports a reload with no door, and not one behind it — its own stimulus', () => {
  const tenth = 'async function runTenth() {\n  const owner = view;\n  const res = await apiFetch(\'/api/tenth\', { method: \'POST\' });\n  await setDocumentFromServer(await res.json(), owner);\n}\n';
  assert.deepEqual(reloadSites(tenth).map((s) => [s.name, s.door]), [['runTenth', false]]);
  const behind = tenth.replace('const res', 'if (!confirmOverlayLoss(owner)) return;\n  const res');
  assert.deepEqual(reloadSites(behind).map((s) => [s.name, s.door]), [['runTenth', true]]);
  // A door named only in a comment is not a door, and the function ABOVE having one does not excuse this one.
  const said = tenth.replace('const res', '// confirmOverlayLoss(owner) belongs here\n  const res');
  assert.equal(reloadSites(behind + said)[1].door, false);
});

test('every reload is behind confirmOverlayLoss, or is named with its reason', () => {
  const sites = reloadSites(APP);
  assert.ok(sites.length >= 15, `only ${sites.length} reload sites found — the scan is not reading app.js properly`);
  const bare = sites.filter((s) => !s.door && !(s.name in NO_OVERLAY_DOOR) && !OVERLAY_DOOR_OWED.includes(s.name));
  assert.deepEqual(bare.map((s) => `${s.name} at app.js:${s.line}`), [],
    'an operation loads the document back without asking: what is placed on the pages, typed into the form or marked for redaction is discarded with no question. Call confirmOverlayLoss(owner) before the request (pass true if it sends bakedBytes), or name it in NO_OVERLAY_DOOR with the reason');
  const names = new Set(sites.map((s) => s.name));
  for (const name of [...Object.keys(NO_OVERLAY_DOOR), ...OVERLAY_DOOR_OWED]) {
    assert.ok(names.has(name), `${name} is excused from the overlay-loss door and no longer reloads a document — the list has drifted`);
  }
  for (const s of sites) {
    assert.ok(!(s.door && (s.name in NO_OVERLAY_DOOR || OVERLAY_DOOR_OWED.includes(s.name))),
      `${s.name} now calls confirmOverlayLoss and is still listed as not doing so — take its name out`);
  }
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
  assert.match(APP, /if \(hasPin\) opts\.headers\['X-Nib-Doc'\] = pinned;/,
    'apiFetch ignores the captured id in favour of the current one — which is the defect, not the fix');
  assert.match(APP, /if \(hasPin && !pinned\) throw /,
    'a present-but-falsy docId is omitted rather than refused, and on the wire an absent header IS the current document (/pending 652)');
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
// D7's rule is "capture the export name at operation entry". Every export scope but two obeys it
// by declaring `const exportName = exportBase();` at the top of the handler that uses it.
//
// **The count that used to sit here has been taken out, and so has app.js's (/pending 513).** Both
// read "nineteen" against a file that had grown to twenty-one — a literal in prose that nothing
// executes is drift the moment the next export lands, and resetting it to 21 only buys until the
// next one. The floor below is the assertion; the population is whatever app.js holds.
//
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

  // Stimulus: the scan must actually be reading a population. A score of scopes declare
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
//
// stripNonCode takes comments and literals out of a source text, leaving code. ONE pass, so a comment or a literal is
// read from where it STARTS: whichever opens first owns everything up to its own close. It was five passes — block
// comments, line comments, then '…', "…" and `…` in turn — and each pass read the whole file as though the kinds
// after it did not exist. An apostrophe inside a template (`it's`) then opened a '…' string that ran to the next
// apostrophe on the line, swallowing the code between and laying a later string's contents open as code. Measured on
// a probe: a call inside a quoted string was reported undeclared (the guard accusing the product), and a real call
// after 'a /* b' or '//x' was not read at all (the guard blind, which is the direction it exists to prevent).
//
// What it still cannot read: a regular-expression literal holding a quote (/['"]/ opens a string), and code inside
// a template's ${…} (taken out with the template, so a call there is not checked). Both need a tokenizer.
const stripNonCode = (text) => text.replace(
  /\/\*[\s\S]*?\*\/|(?<!:)\/\/[^\n]*|'(?:[^'\\\n]|\\.)*'|"(?:[^"\\\n]|\\.)*"|`(?:[^`\\]|\\.)*`/g,
  (m) => (m[0] === '/' ? '' : '""'),
);
const bareCalls = (code) => [...code.matchAll(/(?<![.\w$])([a-z_$][\w$]*)\s*\(/g)].map((m) => m[1]);

test('the scan reads a comment or a literal from where it starts, whatever it holds', () => {
  const calls = (text) => bareCalls(stripNonCode(text));
  // An apostrophe in a template is not the start of a string: the quoted text after it stays a string.
  assert.deepEqual(calls("const a = `it's here`; const b = 'ghostCall(1)';\n"), []);
  assert.deepEqual(calls('const a = `say "hi`; const b = "ghostCall(1)";\n'), []);
  // …and the code between two such templates stays code.
  assert.deepEqual(calls("t(`don't`); u(`won't ghostCall(`);\n"), ['t', 'u']);
  // A comment mark inside a string is not a comment: the call after it is still read.
  assert.deepEqual(calls("const u = 'a /* b'; realOne(); const v = 'c */ d';\n"), ['realOne']);
  assert.deepEqual(calls("const p = '//x'; realTwo();\n"), ['realTwo']);
  assert.deepEqual(calls('const p = `//x`; realTwo();\n'), ['realTwo']);
  // A quote inside a comment is not a string, and a URL's slashes are not a comment.
  assert.deepEqual(calls("// it's a comment\nrealThree('x'); /* don't */ realFour();\n"), ['realThree', 'realFour']);
  assert.deepEqual(calls('go(http://x); after();\n'), ['go', 'after']);
  // An escaped quote does not close its string.
  assert.deepEqual(calls("const q = 'it\\'s ghostCall('; realFive();\n"), ['realFive']);
});

test('every bare function call resolves to something app.js declares', () => {
  const src = stripNonCode(APP);

  const declared = new Set();
  const add = (n) => { const t = String(n).trim().split(/[\s=[\]{}:.]/)[0]; if (t) declared.add(t); };
  // `export` is part of both prefixes. app.js had no exports at all until `dhashFromGrid` and
  // `hamming` were exported for the tier-3 hash instrument, and the omission read as two
  // undeclared calls — the guard accusing the product of a defect that was its own blind spot.
  for (const m of src.matchAll(/(?:^|\n)\s*(?:export\s+(?:default\s+)?)?(?:async\s+)?function\s+([A-Za-z_$][\w$]*)/g)) add(m[1]);
  for (const m of src.matchAll(/(?:^|\n)\s*(?:export\s+)?(?:const|let|var)\s+([^=\n;]+)/g)) m[1].split(',').forEach(add);
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
  for (const n of bareCalls(src)) {
    if (declared.has(n) || KEYWORDS.has(n) || BROWSER.has(n) || KNOWN.has(n)) continue;
    unresolved.add(n);
  }

  assert.deepEqual([...unresolved].sort(), [],
    `called but never declared — a rename that missed a call site throws at run time, and neither node --check nor any tier sees it: ${[...unresolved].sort().join(', ')}`);

  // Stimulus: the scan must be reading a real population, or the green above is over an
  // empty set — which is what a broken strip step would silently produce.
  assert.ok(declared.size > 400, `only ${declared.size} declarations found — the scan is not reading app.js properly`);
});

// /pending 652, behaviourally: the REAL apiFetch, its text taken from app.js, over a stubbed fetch. A present-but-falsy
// docId used to send no header — which `docFor` answers with the ACTIVE document — and now throws before any request.
test('a present-but-falsy docId throws before a request goes out (/pending 652)', async () => {
  const at = APP.indexOf('async function apiFetch(');
  const text = APP.slice(at, APP.indexOf('\n}\n', at) + 2);
  const sent = [];
  const fetchStub = async (url, opts) => { sent.push({ url, headers: opts.headers }); return new Response('{}', { status: 200 }); };
  const apiFetch = new Function('fetch', 'csrf', 'view', 'refreshStatus', 'showLaunchOverlay', 'reconcileWithServer', 'reconciling',
    `${text}\nreturn apiFetch;`)(fetchStub, 't', { docMeta: { id: 'test-epoch:7' } }, () => {}, () => {}, async () => {}, false);
  for (const bad of [null, undefined, '', 0]) {
    await assert.rejects(apiFetch('/api/x', { docId: bad }), /no open document/, `docId ${String(bad)} did not throw`);
  }
  assert.equal(sent.length, 0, 'a request went out for a falsy pin');
  // Controls: a real pin is sent as given, and an absent key still means the current document.
  await apiFetch('/api/x', { docId: 'test-epoch:3' });
  await apiFetch('/api/x', {});
  assert.deepEqual(sent.map((r) => r.headers['X-Nib-Doc']), ['test-epoch:3', 'test-epoch:7']);
});
