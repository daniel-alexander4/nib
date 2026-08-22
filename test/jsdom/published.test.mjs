// Every shape the server publishes has a reader, and the reader is NAMED.
//
// **The defect class.** `historyEvicted` was set, serialized, and asserted by six Go
// tests, and nothing read it — so ADR-003's "eviction is observable or it is not
// eviction" was undischarged while every test in the tree was green. A produced-and-
// never-consumed field is not a wire nit: the whole point of publishing it is that
// somebody is told.
//
// P05's close-out built a scan for that, and it read `docResponse` ONLY. P06 then added
// `docsResponse` and P07 added `handoffResponse` and `instance.Record`, none of which it
// could see — the same gap, one type over, three times. So this file generalises it, and
// generalising means two assertions, not one:
//
//   1. every shape in the table has a reader for each of its fields, and
//   2. **the table is complete** — every json-tagged struct in the scanned packages is
//      either in the table or excluded here with a reason.
//
// (2) is the half that makes this close a class rather than three instances, and it is
// the P05 lesson written as a test: a walk over an inventory cannot see a field that was
// never entered into it. Without it, the fourth new shape is invisible exactly the way
// the first three were.
//
// **The reader is not always app.js.** `handoffResponse`'s reader is `instance.HandOff`
// in Go — the client never sees it, because a second launch is a process, not a browser.
// A scan that only ever looked in `web/app.js` would have to special-case it or report a
// false positive. So each entry names its own readers, and "a reader somewhere named" is
// the property, not "a reader in the client".
//
// ── What this scan can and cannot prove ──────────────────────────────────────
// It proves a NAME is read in a named file. It does NOT prove that the read is of THIS
// shape: `.valid` in app.js satisfies `pendingView.valid` even if the only `.valid` in
// the file belongs to something else entirely. So a green here means "no field is
// obviously orphaned", not "every field is correctly consumed" — the failure it closes
// is historyEvicted's (published, and the string appears nowhere), which is the one that
// actually shipped. A stronger check would need to follow the value from its fetch, and
// that is a different tool; saying so is better than letting the next reader assume it.
//
// Pure source scan: no boot(), so this file is cheap and has no DOM ceiling to declare.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { REPO } from './boot.mjs';

const read = (...p) => fs.readFileSync(path.join(REPO, ...p), 'utf8');

// The packages that serialize to something outside their own process. Both are scanned
// whole rather than file-by-file, so a shape added in a new file is in scope by default.
const PACKAGES = ['internal/server', 'internal/instance'];

// ── The table ────────────────────────────────────────────────────────────────
// One entry per published shape: where it is declared, and every file that must contain
// a read of each of its fields. A field counts as read when some reader mentions it as a
// property — `.historyEvicted` in JS, `.HistoryEvicted` in Go.
const PUBLISHED = [
  { type: 'docResponse', readers: ['web/app.js'] },
  { type: 'docsResponse', readers: ['web/app.js'] },
  { type: 'statusResponse', readers: ['web/app.js'] },
  { type: 'outlineResponse', readers: ['web/app.js'] },
  { type: 'attachmentsResponse', readers: ['web/app.js'] },
  { type: 'attestationsResponse', readers: ['web/app.js'] },
  { type: 'attestationView', readers: ['web/app.js'] },
  { type: 'keysResponse', readers: ['web/app.js'] },
  { type: 'peersResponse', readers: ['web/app.js'] },
  { type: 'peer', readers: ['web/app.js'] },
  { type: 'listDirResponse', readers: ['web/app.js'] },
  { type: 'dirEntry', readers: ['web/app.js'] },
  { type: 'stampsResponse', readers: ['web/app.js'] },
  { type: 'sanitizeResponse', readers: ['web/app.js'] },
  { type: 'decryptResponse', readers: ['web/app.js'] },
  { type: 'updateResponse', readers: ['web/app.js'] },
  { type: 'imageMeta', readers: ['web/app.js'] },
  { type: 'recentEntry', readers: ['web/app.js'] },
  { type: 'sessionStatus', readers: ['web/app.js'] },
  { type: 'receivedInfo', readers: ['web/app.js'] },
  { type: 'pendingView', readers: ['web/app.js'] },
  { type: 'verifyView', readers: ['web/app.js'] },
  { type: 'sendResult', readers: ['web/app.js'] },
  { type: 'externalSignerInfo', readers: ['web/app.js'] },

  // Read in Go, not in the client — the whole reason this scan names its readers per
  // shape instead of always looking in app.js.
  { type: 'handoffResponse', readers: ['internal/instance/instance.go'] },
  { type: 'Record', readers: ['internal/instance/instance.go', 'internal/server/handoff.go'] },
];

// ── The exclusions ───────────────────────────────────────────────────────────
// Every one carries its reason. An unexplained entry here is how a real produced-and-
// unread shape gets parked and forgotten, which is the failure this file exists for.
const EXCLUDED = {
  // Client → server. These are DESERIALIZED by the handler beside them, so their reader
  // is that handler and the direction of the check is mirrored. The mirror class — a
  // field the client sends and the server never reads — is NOT covered by this file.
  // Named rather than left silent: it is a real gap, and it is a different scan.
  enrollRequest: 'request body, read by its handler',
  settingsRequest: 'request body, read by its handler',
  handoffRequest: 'request body, read by its handler',
  addKeyRequest: 'request body, read by its handler',
  removeKeyRequest: 'request body, read by its handler',
  pinPeerRequest: 'request body, read by its handler',
  removePeerRequest: 'request body, read by its handler',
  armRequest: 'request body, read by its handler',
  stampReq: 'request body, read by its handler',
  cosignParams: 'request body, read by its handler',
  finalizeParams: 'request body, read by its handler',
  watermarkParam: 'request body, read by its handler',
  pendingReq: 'request body, read by its handler',
  sessionDecision: 'request body, read by its handler',
  openRequest: 'request body, read by its handler',
  urlRequest: 'request body, read by its handler',
  pathRequest: 'request body, read by its handler',
  saveAsRequest: 'request body, read by its handler',
  listDirRequest: 'request body, read by its handler',
  ocrRequest: 'request body, read by its handler',
  tableRequest: 'request body, read by its handler',
  timestampRequest: 'request body, read by its handler',

  // Third-party wire formats. Their shape is GitHub's, not Nib's, and a field Nib does
  // not read is a field GitHub publishes rather than one Nib leaks.
  release: "GitHub's release JSON, not a shape Nib publishes",
  asset: "GitHub's release JSON, not a shape Nib publishes",

  // Peer-to-peer envelopes exchanged between two Nib processes over the co-sign
  // transport. Both ends are Go, and internal/p2p owns that contract.
  cosignQuote: 'p2p envelope between two Nib processes, not a client response',
};

// ── Published, and NOT read ──────────────────────────────────────────────────
// The findings this scan produced on its first run, kept VISIBLE rather than excluded.
// An exclusion says "this is not the scan's business"; these are exactly its business —
// each is a `historyEvicted` — so they sit in their own bucket with a reason and, where
// there is one, the follow-up that will clear them. Deleting an entry from here is how
// one gets fixed; a NEW unread field cannot be parked here without someone writing a
// line, which is the intended cost.
const UNREAD_KNOWN = {
  'updateResponse.managed': 'read in Go by assetURL (internal/server/update.go) BEFORE serialization, to pick a .deb over a raw binary. It is on the wire for no client that wants it. Not counted as read: a field consumed by the code that sets it has no consumer at the far end, which is the whole property here.',
  'imageMeta.mime': 'the image library lists id/name/builtin and the client shows an <img src="/api/images/{id}">, which needs no mime. Informational in a listing API; harmless, and named so it is a decision rather than an oversight.',
};

// ── The scan ─────────────────────────────────────────────────────────────────
// Struct name -> [json field names], for every json-tagged struct in the packages.
// declaredStructs collects every struct declaration under `internal/`, keyed by
// `pkg.Type` AND by bare `Type`, so an embed can be resolved from either spelling.
//
// It reaches wider than PACKAGES on purpose: an embedded type does not have to live in a
// scanned package to contribute published fields, and `attestationView` embeds
// `p2p.SignerAttestation` from a package this file has no other reason to read.
function declaredStructs() {
  const byQualified = new Map(); // "p2p.SignerAttestation" -> {body, file}
  const byBare = new Map(); // "SignerAttestation" -> [{qualified, body, file}, …]
  const root = path.join(REPO, 'internal');
  for (const pkg of fs.readdirSync(root)) {
    const dir = path.join(root, pkg);
    if (!fs.statSync(dir).isDirectory()) continue;
    for (const f of fs.readdirSync(dir)) {
      if (!f.endsWith('.go') || f.endsWith('_test.go')) continue;
      const src = fs.readFileSync(path.join(dir, f), 'utf8');
      for (const m of src.matchAll(/type\s+([A-Za-z_][A-Za-z0-9_]*)\s+struct\s*\{/g)) {
        // The struct's body: from the opening brace to the first line that is exactly
        // "}". Anchored to a line start so a nested literal cannot end it early — the
        // P05 lesson about scanUnpinned taking the first `{` it found, in the other
        // direction.
        const from = m.index + m[0].length;
        const end = src.indexOf('\n}', from);
        if (end === -1) continue;
        const rec = { body: src.slice(from, end), file: `internal/${pkg}/${f}`, name: m[1] };
        byQualified.set(`${pkg}.${m[1]}`, rec);
        if (!byBare.has(m[1])) byBare.set(m[1], []);
        byBare.get(m[1]).push(rec);
      }
    }
  }
  return { byQualified, byBare };
}

const DECLARED = declaredStructs();

// resolveEmbed finds the struct an embed line names, or null.
//
// **An ambiguous bare name is a hard failure, not a guess.** Two packages under
// `internal/` may both declare `Record`, and picking one silently would merge the wrong
// fields — a scan that reports the wrong shape is worse than one that reports none,
// because it reads as coverage. A qualified embed (`p2p.SignerAttestation`) is exact and
// never ambiguous; only an unqualified one can be, and that means same-package, so the
// declaring file's own package is preferred before anything is called ambiguous.
function resolveEmbed(name, fromFile) {
  if (name.includes('.')) return DECLARED.byQualified.get(name) ?? null;
  const all = DECLARED.byBare.get(name) ?? [];
  const samePkg = all.filter((r) => path.dirname(r.file) === path.dirname(fromFile));
  if (samePkg.length === 1) return samePkg[0];
  if (all.length === 1) return all[0];
  if (all.length > 1) {
    throw new Error(
      `the embedded type ${name} (in ${fromFile}) is declared in ${all.length} packages ` +
        `(${all.map((r) => r.file).join(', ')}) — this scan cannot tell which is embedded, ` +
        `and guessing would merge another shape's fields into this one. Qualify the embed ` +
        `or teach resolveEmbed the package.`,
    );
  }
  return null;
}

// EMBED matches a line that is a struct EMBED and nothing else: no field name, an
// optional `*`, an optional package qualifier, an optional json tag.
//
// Deliberately narrow. `Foo Bar` is a named field, `[]Foo` is a named field's type, and
// an anonymous `struct{…}` is neither — none of them may be read as an embed, because a
// false embed pulls in a shape this struct does not publish.
const EMBED = /^\s*\*?([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)?)\s*(?:`[^`]*`)?\s*$/;

// fieldsOf returns every json tag a struct publishes, INCLUDING those it reaches through
// an embedded type.
//
// **This is the gap that let `oneProceeding` survive the scan built to find exactly it.**
// An embedded type contributes its fields with no tag on the embed line, so a scan that
// reads `json:"…"` out of a struct's own body sees them not at all: `attestationView`
// embeds `p2p.SignerAttestation`, publishes nine fields, and this file checked ONE
// (`pinned`). `SignerAttestation.OneProceeding` — computed, serialized as
// `oneProceeding`, rendered nowhere — was invisible here and had to be found by
// `observables_test.go` on the Go side. The two scans were complementary by accident;
// this closes the seam from the JS side.
//
// Recursive, because an embedded type may itself embed one. `seen` is per-call and
// guards a cycle: Go forbids a struct embedding itself by value, but a scan does not get
// to rely on the compiler having run.
function fieldsOf(rec, seen = new Set()) {
  if (seen.has(rec.file + '#' + rec.name)) return [];
  seen.add(rec.file + '#' + rec.name);
  const own = [...rec.body.matchAll(/`json:"([^",]+)/g)].map((x) => x[1]).filter((x) => x !== '-');
  const inherited = [];
  for (const line of rec.body.split('\n')) {
    const bare = line.replace(/\/\/.*$/, '');
    const m = bare.match(EMBED);
    if (!m) continue;
    const target = resolveEmbed(m[1], rec.file);
    if (target) inherited.push(...fieldsOf(target, seen));
  }
  return [...new Set([...own, ...inherited])];
}

function scanStructs() {
  const out = new Map();
  for (const pkg of PACKAGES) {
    for (const f of fs.readdirSync(path.join(REPO, pkg))) {
      if (!f.endsWith('.go') || f.endsWith('_test.go')) continue;
      const rel = `${pkg}/${f}`;
      for (const [, rec] of DECLARED.byQualified) {
        if (rec.file !== rel) continue;
        const fields = fieldsOf(rec);
        if (fields.length) out.set(rec.name, { fields, file: rel });
      }
    }
  }
  return out;
}


// codeOnly strips comments before anything looks for a read.
//
// **A mention is not a read**, and without this the check is satisfied by prose about the
// field. Measured: `Record.version` was declared read by internal/instance/instance.go,
// the read was deleted, and this scan still passed — because a doc comment two lines up
// says "see Record.Version". That is the same weakness the .deb dependency guard had in
// the previous sweep, in a second file, which is why it is fixed here rather than noted.
//
// Mangling a string that contains "//" (a URL) is acceptable: it can only ever REMOVE a
// candidate read, so the failure direction is a false alarm someone investigates, never a
// false pass nobody sees.
function codeOnly(src) {
  return src
    .replace(/\/\*[\s\S]*?\*\//g, ' ')
    .split('\n')
    .map((l) => l.replace(/\/\/.*$/, ''))
    .join('\n');
}

const STRUCTS = scanStructs();

test('the scan actually reads the packages it claims to', () => {
  // The stimulus for both tests below. A scan that parses nothing reports every shape
  // as covered and every reader as present — the green this whole file refuses.
  assert.ok(STRUCTS.size >= 30,
    `only ${STRUCTS.size} json-tagged structs found across ${PACKAGES.join(', ')} — the scan is not reading the source`);
  assert.ok(STRUCTS.has('docResponse'),
    'docResponse was not found, so this scan is not looking at the shape the class was discovered on');
  const doc = STRUCTS.get('docResponse');
  assert.ok(doc.fields.includes('historyEvicted'),
    'historyEvicted is not among docResponse\'s parsed fields — the field the whole defect class was named for is invisible to the scan');

  // And the EMBED half, which is a second way for the scan to go quietly blind.
  // `attestationView` declares one field of its own (`pinned`) and reaches nine more
  // through `p2p.SignerAttestation`. Before fieldsOf resolved embeds this scan checked
  // exactly one of the ten — which is how `oneProceeding` survived the very scan built
  // to catch a published-and-never-read field, and had to be found on the Go side by
  // observables_test.go instead.
  //
  // Asserted on a field that is ONLY reachable through the embed. Asserting on `pinned`
  // would pass with embed resolution deleted.
  const view = STRUCTS.get('attestationView');
  assert.ok(view, 'attestationView was not found, so the embed case is not being scanned at all');
  assert.ok(view.fields.includes('oneProceeding'),
    `attestationView publishes ${view.fields.length} field(s) (${view.fields.join(', ')}) — oneProceeding is missing, so embedded types are contributing nothing and every field this shape reaches through p2p.SignerAttestation is unchecked`);
  assert.ok(view.fields.length >= 10,
    `attestationView resolved to only ${view.fields.length} fields; it declares 1 and embeds 9`);
});

test('the table covers every shape the packages publish', () => {
  const known = new Set([...PUBLISHED.map((p) => p.type), ...Object.keys(EXCLUDED)]);
  const missing = [...STRUCTS.keys()].filter((t) => !known.has(t)).sort();
  assert.deepEqual(missing, [],
    `these json-tagged shapes are neither in PUBLISHED nor in EXCLUDED, so nothing checks that anything reads them — which is exactly how historyEvicted shipped: ${missing.join(', ')}. Add each to the table with its readers, or to EXCLUDED with the reason it is not published.`);

  // And the reverse: a table entry naming a struct that no longer exists is a check
  // that silently stopped running. `go test` cannot tell you a test stopped existing;
  // neither can this one, unless it is asked.
  const stale = PUBLISHED.map((p) => p.type).filter((t) => !STRUCTS.has(t)).sort();
  assert.deepEqual(stale, [],
    `the table names shapes that no longer exist, so their entries check nothing: ${stale.join(', ')}`);
});

test('every field of every published shape has a reader in the file that declares it as one', () => {
  const cache = new Map();
  const source = (rel) => {
    if (!cache.has(rel)) cache.set(rel, codeOnly(read(rel)));
    return cache.get(rel);
  };

  const unread = [], fixed = [];
  for (const entry of PUBLISHED) {
    const s = STRUCTS.get(entry.type);
    if (!s) continue; // reported by the staleness check above
    for (const field of s.fields) {
      // Three spellings, all of them a read. A JS reader may use property access OR
      // destructuring — `const { docs, activeId } = await res.json()` is how the client
      // reads docsResponse, and a property-access-only regex reported both fields
      // unread on this scan's first run. A Go reader spells it as the exported field.
      const js = new RegExp(`\\.${field}(?![\\w$])`);
      const destructured = new RegExp(`\\{[^{}]*\\b${field}\\b[^{}]*\\}\\s*=`);
      const go = new RegExp(`\\.${field[0].toUpperCase()}${field.slice(1)}(?![\\w$])`);
      const found = entry.readers.some((r) => {
        const src = source(r);
        return js.test(src) || go.test(src) || destructured.test(src);
      });
      const key = `${entry.type}.${field}`;
      if (found && UNREAD_KNOWN[key]) {
        fixed.push(key); // a known-unread field that now HAS a reader: delete its entry
      } else if (!found && !UNREAD_KNOWN[key]) {
        unread.push(`${key} (declared ${s.file}, readers: ${entry.readers.join(', ')})`);
      }
    }
  }
  assert.deepEqual(unread, [],
    `the server publishes these and no declared reader reads them — produced and never consumed, which for a status field means the user is never told:\n  ${unread.join('\n  ')}`);

  // The other direction, and it is not symmetry for its own sake: a UNREAD_KNOWN entry
  // that has since gained a reader is a stale excuse, and a stale excuse is where the
  // NEXT unread field gets parked without anyone noticing.
  assert.deepEqual(fixed, [],
    `these are listed in UNREAD_KNOWN but now HAVE a reader — delete their entries, or the list stops describing anything: ${fixed.join(', ')}`);

  // And the third direction, which this file did NOT have until statusResponse.toolbarStyle
  // was deleted and nothing noticed. Both loops above only ever walk fields that exist, so
  // an entry for a field that has been REMOVED is visited by neither and sits there
  // forever — describing a shape the server no longer publishes, in a list whose whole
  // purpose is to be an accurate account of what is unread.
  const known = new Set();
  for (const entry of PUBLISHED) {
    const st = STRUCTS.get(entry.type);
    if (st) for (const f of st.fields) known.add(`${entry.type}.${f}`);
  }
  const vanished = Object.keys(UNREAD_KNOWN).filter((k) => !known.has(k)).sort();
  assert.deepEqual(vanished, [],
    `UNREAD_KNOWN names fields that no longer exist on any published shape: ${vanished.join(', ')}. They were removed or renamed and the excuse outlived them.`);

  // The stimulus for the whole file. An empty PUBLISHED table, or a source read that
  // returned nothing, produces the same two empty arrays as a clean tree.
  const checked = PUBLISHED.reduce((n2, e) => n2 + (STRUCTS.get(e.type)?.fields.length || 0), 0);
  assert.ok(checked >= 60,
    `only ${checked} fields were checked across ${PUBLISHED.length} shapes — the scan is not reading what it thinks it is`);
});
