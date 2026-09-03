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
  // The ceremonies listing and its rows (P06.S02). It left EXCLUDED when the panel started reading
  // it, which is what an exclusion is for: a promise that somebody will, redeemed.
  { type: 'ceremoniesResponse', readers: ['web/app.js'] },
  // Whose turn it is (P06.S03). The panel renders every branch of its three states, and `meKnown`
  // beside `isMe` because a machine that does not know its position must not be told it is
  // somebody else's turn.
  { type: 'ceremonyNextResponse', readers: ['web/app.js'] },
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
  // The sticky session-failure surface (P08.S08, C03). It sat in NEITHER table from v1.117.243
  // until v1.117.262 — so tier 2 was red for five commits and this scan was the thing saying so.
  // All three fields have a reader now: `summary`/`at` render the sentence and key its
  // re-announcement, and `what` selects the recovery action, which is the half that makes it a
  // surface rather than a label over an unrescuable state.
  { type: 'noticeView', readers: ['web/app.js'] },
  { type: 'pendingView', readers: ['web/app.js'] },
  // Who has already signed the arriving document (P07.S07c, D27 item 3). Rendered by
  // `renderConsentSigners`, which draws a row per signer and marks an invalid one rather than
  // dropping it — so all three fields have a reader on the consent screen.
  { type: 'pendingSigner', readers: ['web/app.js'] },
  { type: 'verifyView', readers: ['web/app.js'] },
  { type: 'sendResult', readers: ['web/app.js'] },
  // P05.S11's D19 diagnosis surface. Published now; RENDERED by P06's ceremony panel, which is built
  // last — so the fields carry a reader only once that lands. Named here (shape) and in UNREAD_KNOWN
  // (fields) so the gap is a recorded decision, not the historyEvicted-shaped oversight this scan exists for.
  // `deferredFields` is not scanned AT ALL, which is the difference from UNREAD_KNOWN and the
  // reason it exists: a shape whose consumer is unbuilt has no reader for any of these, and
  // parking them one at a time let `detail` — whose name happens to appear elsewhere — be
  // laundered as covered while `cause` and `summary` sat parked beside it. `error` is genuinely
  // read (errText) and stays scanned. When P06 lands, delete the deferral; nothing textual can
  // notice for you, and that limit is real rather than hidden.
  { type: 'diagnosisResponse', readers: ['web/app.js'], deferredFields: ['cause', 'summary', 'detail'] },
  { type: 'diagnosisView', readers: ['web/app.js'], deferredFields: ['cause', 'summary', 'detail'] },
  { type: 'externalSignerInfo', readers: ['web/app.js'] },
  // Moved out of EXCLUDED, whose reason for it was factually WRONG: it was excluded as "a p2p
  // envelope between two Nib processes, not a client response", and it is the body of
  // POST /api/cosign/quote, fetched twice in app.js. An exclusion is a stronger laundering than
  // a coincidence — no regex is even attempted — and it was hiding an unread `page`, since
  // deleted (/pending 254).
  { type: 'cosignQuote', readers: ['web/app.js'] },

  // Read in Go, not in the client — the whole reason this scan names its readers per
  // shape instead of always looking in app.js.
  { type: 'handoffResponse', readers: ['internal/instance/instance.go'] },
  { type: 'Record', readers: ['internal/instance/instance.go', 'internal/server/handoff.go'] },

  // `/api/lan/heard`'s two shapes. Their reader is the tier-4 HARNESS, not the client — the
  // route is a browse-level instrument built because /pending 300 could not be diagnosed
  // without one, and `link_report` is what reads it. Entered in PUBLISHED rather than EXCLUDED
  // on purpose: it IS published, it does have a reader, and excluding it would launder a shape
  // that a future client change could quietly stop matching. This scan names readers per shape
  // precisely so a non-client reader is expressible.
  { type: 'lanHeard', readers: ['build/pairrepro.sh'] },
  { type: 'lanHeardResponse', readers: ['build/pairrepro.sh'] },

  // `/api/ceremony/deliver`'s per-party report (P08.S05g), whose reader is the tier-4 harness
  // for the same reason `lanHeard`'s is: the round is a route a convener triggers and P06 builds
  // the panel that would show it. `decline_round` grades every one of the five fields — two
  // parties delivered, and the party that ended the proceeding named rather than silently absent.
  //
  // **This shape shipped in NEITHER table at v1.117.320, so tier 2 was red at that commit and the
  // slice closed over it** — the fifth time this scan has caught a shape entered nowhere, and the
  // second time it was red across a slice close. Entered in PUBLISHED rather than EXCLUDED
  // because it IS published and it DOES have a reader.
  //
  // It does not close `/pending 353`, which is about the round having no reader *in the product*:
  // a harness assertion is evidence for this scan and not a surface a convener can look at.
  { type: 'deliveryOutcome', readers: ['build/pairrepro.sh'] },
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
  // P07.S02a's convene route (v1.117.155). The two request shapes are the ordinary
  // client-to-server case above.
  conveneRequest: 'request body, read by its handler',
  convenePartyRequest: 'request body, read by its handler',
  ceremonyInvitesRequest: 'request body, read by its handler',
  // P07.S02b's accept route (v1.117.157).
  acceptRequest: 'request body, read by its handler',
  listDirRequest: 'request body, read by its handler',
  ocrRequest: 'request body, read by its handler',
  tableRequest: 'request body, read by its handler',
  timestampRequest: 'request body, read by its handler',

  // Third-party wire formats. Their shape is GitHub's, not Nib's, and a field Nib does
  // not read is a field GitHub publishes rather than one Nib leaks.
  release: "GitHub's release JSON, not a shape Nib publishes",
  asset: "GitHub's release JSON, not a shape Nib publishes",


  // **Server → client, and GENUINELY UNREAD today (P07.S02a, v1.117.155).**
  //
  // Excluded rather than listed with `web/app.js` as a reader, because nothing in the client
  // reads them: P06 builds the convene panel and is built LAST. Naming a reader that does not
  // read would be this scan asserting coverage against a filename — the false green it exists
  // to prevent — and `historyEvicted` is what happens when a shape is simply left out.
  //
  // Delete these four entries when P06.S04's panel renders the invitations. The Go-side twin of
  // this parking is in observables_test.go for ceremony.Convened and vault.CeremonySecret.
  conveneResponse: 'P07.S02a ships the route before P06 builds its panel; no client reader yet',
  conveneInvite: 'P07.S02a ships the route before P06 builds its panel; no client reader yet',
  // Same parking, same panel, one slice later (P07.S02b, v1.117.157). `/api/ceremony/accept`
  // is the invitee's door and P06 builds the screen that shows a party who invited them, who
  // else is on the roster, and in what capacity. Delete these two with the convene pair.
  acceptResponse: 'P07.S02b ships the route before P06 builds its panel; no client reader yet',
  acceptedParty: 'P07.S02b ships the route before P06 builds its panel; no client reader yet',
  // **`ceremoniesResponse` was here and is GONE (P06.S02, v1.117.335).** The ceremony panel reads
  // it — every field, which is what deleting an entry from this map costs: `primary` as well as
  // its `note`, and `expires` as the one deadline this phase shows in human units. It went alone;
  // the four convene/accept entries above are P06.S04's and stay until that panel lands. The
  // parking said "delete it with the four above", and deleting one of five is the honest version.
};

// ── Published, and NOT read ──────────────────────────────────────────────────
// The findings this scan produced on its first run, kept VISIBLE rather than excluded.
// An exclusion says "this is not the scan's business"; these are exactly its business —
// each is a `historyEvicted` — so they sit in their own bucket with a reason and, where
// there is one, the follow-up that will clear them. Deleting an entry from here is how
// one gets fixed; a NEW unread field cannot be parked here without someone writing a
// line, which is the intended cost.
const UNREAD_KNOWN = {
  // **`updateResponse.managed` was here and the FIELD is gone (/pending 350, v1.117.309).** This
  // entry had the argument already written — "a field consumed by the code that sets it has no
  // consumer at the far end, which is the whole property here" — and it sat as a park rather than
  // as a fix. It is now a local in the handler, so there is nothing on the wire to read.
  'imageMeta.mime': 'the image library lists id/name/builtin and the client shows an <img src="/api/images/{id}">, which needs no mime. Informational in a listing API; harmless, and named so it is a decision rather than an oversight.',
  // **`sessionStatus.diagnosis` was here and is GONE — read since /pending 349 (v1.117.309).**
  //
  // Its entry read "rendered by P06 (the ceremony panel), unbuilt", and that reason was wrong
  // rather than merely stale: the surface the field's own doc names — "the polling UI" — has
  // existed since P05.S11, which is the slice that added the field. Nothing was waiting on P06;
  // the reader was simply never written, and a plausible phase to blame is what kept anyone from
  // noticing for as long as it did.
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
//
// Each field carries `declaredIn` — the struct that actually declares it — because two
// shapes reaching the SAME field through an embed are one field with one reader, while two
// shapes declaring their own identically-named field are two, and the exclusive-evidence
// check below is meaningless unless it can tell those apart.
function fieldsOf(rec, seen = new Set()) {
  const key = rec.file + '#' + rec.name;
  if (seen.has(key)) return [];
  seen.add(key);
  const own = [...rec.body.matchAll(/`json:"([^",]+)/g)]
    .map((x) => x[1])
    .filter((x) => x !== '-')
    .map((name) => ({ name, declaredIn: key }));
  const inherited = [];
  for (const line of rec.body.split('\n')) {
    const bare = line.replace(/\/\/.*$/, '');
    const m = bare.match(EMBED);
    if (!m) continue;
    const target = resolveEmbed(m[1], rec.file);
    if (target) inherited.push(...fieldsOf(target, seen));
  }
  const out = [], names = new Set();
  for (const f of [...own, ...inherited]) {
    if (names.has(f.name)) continue;
    names.add(f.name);
    out.push(f);
  }
  return out;
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
  assert.ok(doc.fields.some((f) => f.name === 'historyEvicted'),
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
  assert.ok(view.fields.some((f) => f.name === 'oneProceeding'),
    `attestationView publishes ${view.fields.length} field(s) (${view.fields.map((f) => f.name).join(', ')}) — oneProceeding is missing, so embedded types are contributing nothing and every field this shape reaches through p2p.SignerAttestation is unchecked`);
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

// lineIndex lets a match be attributed to a LINE rather than to a boolean. Which line is the
// whole of the coincidence check below: "some line in this file mentions .detail" and "the line
// that mentions .detail belongs to a different shape entirely" are the same green.
function lineIndex(src) {
  const starts = [0];
  for (let k = 0; k < src.length; k++) if (src[k] === '\n') starts.push(k + 1);
  return starts;
}
function lineAt(starts, idx) {
  let lo = 0, hi = starts.length - 1;
  while (lo < hi) {
    const mid = (lo + hi + 1) >> 1;
    if (starts[mid] <= idx) lo = mid; else hi = mid - 1;
  }
  return lo + 1;
}

// evidenceFor returns the set of `file:line` sites that count as a read of `field`, using the
// same three spellings the scan has always used: JS property access, JS destructuring, and the
// Go exported name.
function evidenceFor(src, starts, rel, field) {
  const out = new Set();
  const pats = [
    new RegExp(`\\.${field}(?![\\w$])`, 'g'),
    new RegExp(`\\{[^{}]*\\b${field}\\b[^{}]*\\}\\s*=`, 'g'),
    new RegExp(`\\.${field[0].toUpperCase()}${field.slice(1)}(?![\\w$])`, 'g'),
    // **Bracket and .get() access, because a non-JS reader was invisible to the three above.**
    // The table has always said a reader need not be `web/app.js`, and two of them are Go — but
    // `/api/lan/heard`'s reader is a shell harness driving Python, which reaches every field as
    // `d["heard"]` and `d.get("note")`. Against the dotted patterns alone that reader matched
    // NOTHING, so six fields with a real consumer reported as unread. That failure runs in the
    // dangerous direction as well as this one: a matcher that cannot see a whole reader style
    // cannot tell "nobody reads it" from "I cannot read the reader", and both come out as a
    // finding somebody will eventually park.
    new RegExp(`\\[\\s*['"\`]${field}['"\`]\\s*\\]`, 'g'),
    new RegExp(`\\.get\\(\\s*['"\`]${field}['"\`]`, 'g'),
  ];
  for (const re of pats) for (const m of src.matchAll(re)) out.add(`${rel}:${lineAt(starts, m.index)}`);
  return out;
}

// ── Nested shapes ────────────────────────────────────────────────────────────
//
// `fieldsOf` follows EMBEDS. It does not follow a NAMED struct-typed field, so a whole shape
// rides onto the wire unchecked one type over: `docResponse.Signature sign.Status` carries
// `sign.Status` and `sign.SignerInfo`, and `sanitizeResponse.Residual pdfops.ScanReport` carries
// `pdfops.Finding` — the very type whose `detail` was the coincidental reader that laundered
// D19's `detail` at /pending 254. Same class as `oneProceeding`, one level of indirection out.
//
// **Where the recursion stops, stated as a rule rather than left incidental:**
//  1. Only types `declaredStructs()` found — structs declared under `internal/`. Nothing from
//     the module cache is ever followed: `time.Time` has no json tags (0 fields, silently) and
//     `netip.AddrPort` is unexported throughout, so walking them produces confident nonsense.
//  2. Stop at any type that has its own PUBLISHED entry — it is checked there, with its own
//     readers. Without this `docsResponse.docs` re-expands every docResponse field.
//  3. Memoize per type: `sign.SignerInfo` is reached through three shapes and is one field set.
//  4. A depth cap AND a cycle guard, because the cycle is not hypothetical — `xfdfField.Fields
//     []xfdfField` exists in `internal/pdfops`, the same package this walk already reaches, and
//     it cycles through a SLICE, which a pointers-only argument would miss.
//  5. An ambiguous bare type name is TERMINAL, not a throw. `resolveEmbed` throws for embeds and
//     must keep throwing; four bare names (`Record`, `Stats`, `result`, `Server`) are already
//     declared in two packages each, and a new field of one must not break the suite with a
//     parse error that reads as a scan failure.
//
// Nested keys are `pkg.Type.field`, NEVER a bare field name. `fieldsOf` dedups by name, so a
// flattened `sign.SignerInfo.reason` would collide with `decryptResponse.Reason` and be
// laundered inside its own shape — the failure this file already documents one field over.
const NESTED_DEPTH = 4;

// FIELD_DECL matches `Name Type `json:"x"`` — a NAMED field with a type, which is exactly what
// EMBED is written to exclude.
const FIELD_DECL = /^\s*[A-Z][A-Za-z0-9_]*\s+([\[\]\*A-Za-z0-9_.\]]+)\s+`json:"([^",]+)/;

// baseTypeName strips slice, pointer, array and map decoration down to the type being named.
function baseTypeName(t) {
  let out = t.replace(/^\**/, '').replace(/^(\[\d*\])+/, '').replace(/^\*+/, '');
  const map = out.match(/^map\[[^\]]*\](.*)$/);
  if (map) out = map[1].replace(/^\**/, '');
  return out;
}

// resolveNamed is resolveEmbed's non-throwing sibling: null on unknown OR ambiguous.
function resolveNamed(name, fromFile) {
  if (name.includes('.')) return DECLARED.byQualified.get(name) ?? null;
  const all = DECLARED.byBare.get(name) ?? [];
  const samePkg = all.filter((r) => path.dirname(r.file) === path.dirname(fromFile));
  if (samePkg.length === 1) return samePkg[0];
  if (all.length === 1) return all[0];
  return null; // unknown, or ambiguous — terminal either way
}

// nestedTypes walks a shape and returns every internal struct type reachable through a named
// struct-typed field, keyed `pkg.Type`.
function nestedTypes(rec, published, out = new Map(), seen = new Set(), depth = 0) {
  if (depth >= NESTED_DEPTH) return out;
  const key = rec.file + '#' + rec.name;
  if (seen.has(key)) return out;
  seen.add(key);
  for (const line of rec.body.split('\n')) {
    const m = line.replace(/\/\/.*$/, '').match(FIELD_DECL);
    if (!m) continue;
    const target = resolveNamed(baseTypeName(m[1]), rec.file);
    if (!target || published.has(target.name)) continue; // rule 2
    const id = `${path.basename(path.dirname(target.file))}.${target.name}`;
    if (!out.has(id)) out.set(id, target);
    nestedTypes(target, published, out, seen, depth + 1);
  }
  return out;
}

const EVIDENCE = (() => {
  const cache = new Map();
  const source = (rel) => {
    if (!cache.has(rel)) {
      const src = codeOnly(read(rel));
      cache.set(rel, { src, starts: lineIndex(src) });
    }
    return cache.get(rel);
  };
  const out = new Map(); // "shape.field" -> {declaredIn, field, lines:Set, deferred:bool}
  const publishedTypes = new Set(PUBLISHED.map((p) => p.type));
  const nested = new Map(); // "pkg.Type" -> {rec, readers:Set}
  for (const entry of PUBLISHED) {
    const st = STRUCTS.get(entry.type);
    if (!st) continue;
    const rec = [...DECLARED.byQualified.values()].find((r) => r.file === st.file && r.name === entry.type);
    if (rec) {
      for (const [id, target] of nestedTypes(rec, publishedTypes)) {
        if (!nested.has(id)) nested.set(id, { rec: target, readers: new Set() });
        for (const r of entry.readers) nested.get(id).readers.add(r);
      }
    }
    const deferred = new Set(entry.deferredFields ?? []);
    for (const f of st.fields) {
      const lines = new Set();
      if (!deferred.has(f.name)) {
        for (const rel of entry.readers) {
          const { src, starts } = source(rel);
          for (const site of evidenceFor(src, starts, rel, f.name)) lines.add(site);
        }
      }
      out.set(`${entry.type}.${f.name}`, {
        declaredIn: f.declaredIn, field: f.name, lines, deferred: deferred.has(f.name),
      });
    }
  }
  // The nested shapes, keyed by DECLARING type so a name cannot be laundered across shapes.
  for (const [id, { rec, readers }] of nested) {
    for (const f of fieldsOf(rec)) {
      const lines = new Set();
      for (const rel of readers) {
        const { src, starts } = source(rel);
        for (const site of evidenceFor(src, starts, rel, f.name)) lines.add(site);
      }
      out.set(`${id}.${f.name}`, { declaredIn: f.declaredIn, field: f.name, lines, deferred: false });
    }
  }
  return out;
})();

test('every field of every published shape has a reader in the file that declares it as one', () => {
  const unread = [], fixed = [], doubleParked = [];
  for (const [key, e] of EVIDENCE) {
    if (e.deferred) continue; // not scanned at all — see deferredFields
    // A coincidental field is PARKED AS UNREAD, which is the whole point of that list: it
    // looks read and is not, so counting its matches would re-launder it here. It is a park
    // like UNREAD_KNOWN, with a stronger claim — "the reader you would find is somebody
    // else's" — and the two lists must not both hold the same field, or one of them is
    // describing a state that is not the field's.
    if (COINCIDENTAL[key]) {
      if (UNREAD_KNOWN[key]) doubleParked.push(key);
      continue;
    }
    const found = e.lines.size > 0;
    if (found && UNREAD_KNOWN[key]) {
      fixed.push(key); // a known-unread field that now HAS a reader: delete its entry
    } else if (!found && !UNREAD_KNOWN[key]) {
      unread.push(`${key} (declared ${e.declaredIn})`);
    }
  }
  assert.deepEqual(unread, [],
    `the server publishes these and no declared reader reads them — produced and never consumed, which for a status field means the user is never told:\n  ${unread.join('\n  ')}`);

  // The other direction, and it is not symmetry for its own sake: a UNREAD_KNOWN entry
  // that has since gained a reader is a stale excuse, and a stale excuse is where the
  // NEXT unread field gets parked without anyone noticing.
  assert.deepEqual(fixed, [],
    `these are listed in UNREAD_KNOWN but now HAVE a reader — delete their entries, or the list stops describing anything: ${fixed.join(', ')}`);

  assert.deepEqual(doubleParked, [],
    `these fields are parked in BOTH UNREAD_KNOWN and COINCIDENTAL: ${doubleParked.join(', ')}. They say different things — "nothing reads it" and "what looks like a reader is another shape's" — and a field cannot be both.`);

  // And the third direction, which this file did NOT have until statusResponse.toolbarStyle
  // was deleted and nothing noticed. Both loops above only ever walk fields that exist, so
  // an entry for a field that has been REMOVED is visited by neither and sits there
  // forever — describing a shape the server no longer publishes, in a list whose whole
  // purpose is to be an accurate account of what is unread.
  const vanished = Object.keys(UNREAD_KNOWN).filter((k) => !EVIDENCE.has(k)).sort();
  assert.deepEqual(vanished, [],
    `UNREAD_KNOWN names fields that no longer exist on any published shape: ${vanished.join(', ')}. They were removed or renamed and the excuse outlived them.`);

  // The stimulus for the whole file. An empty PUBLISHED table, or a source read that
  // returned nothing, produces the same two empty arrays as a clean tree.
  const checked = [...EVIDENCE.values()].filter((e) => !e.deferred).length;
  assert.ok(checked >= 120,
    `only ${checked} fields were checked across ${PUBLISHED.length} shapes — the scan is not reading what it thinks it is. The figure was 117 before the nested walk and 138 after it (/pending 259); a floor left at the old number would tolerate losing the whole nested population.`);
});

// ── The coincidence door ─────────────────────────────────────────────────────
//
// **The scan above proves a NAME is mentioned in a named file. It cannot tell whose name.**
// Not a theoretical hole: `diagnosisResponse.detail` and `diagnosisView.detail` are rendered by
// P06, which is unbuilt, and both passed as consumed on the strength of one `f.detail` in
// `renderScanReport` — a `pdfops.Finding`, nothing to do with D19 — while their siblings `cause`
// and `summary` sat correctly parked one line above. A P06-only field reading as covered is the
// historyEvicted class this file exists to close, one field over. /pending 254.
//
// **Two precise fixes were built and measured, and both are refused. Recorded so the next
// reader does not re-derive them:**
//
//  1. *Attribute a match to the shape by proximity or by tracking the variable.* Measured: no Go
//     type name appears anywhere in app.js (`grep -c docResponse` is 0), so the only anchor is
//     the route string — and `/api/doc` is fetched at app.js:2757 while `m.historyEvicted` is
//     read at 8309, through `setDocumentFromServer(out, owner)` → `meta` → `target.docMeta` →
//     `m`. Anything short of alias tracking across a property stash reports historyEvicted
//     ITSELF as unread. A guard that cries wolf on the field it is named for gets relaxed.
//  2. *Require evidence to be EXCLUSIVE — no line may be the sole evidence for two shapes.*
//     Built, run, and it flagged 20+ fields on its first pass, all of them genuinely read. The
//     reason is structural: a file-wide `.name` regex hands EVERY shape publishing `name` the
//     identical set of lines, so set algebra cannot separate "both read, on different lines"
//     from "one borrows all of the other's". Exclusivity needs per-shape attribution, which is
//     (1), which is refused.
//
// So the door that shuts is the one that is decidable: **a shape whose consumer is unbuilt is
// deferred WHOLE**, via `deferredFields`, and its fields are not scanned at all — no coincidence
// can reach them. That is what makes `.detail` unlaunderable rather than luckily-caught. The
// fields below are the ones a manual pass found already laundered; each names what its matches
// actually belong to, so the list is an account rather than an excuse.
const COINCIDENTAL = {
  'pendingView.valid': 'the consent card does not show whether the peer\'s incoming signature validated — /pending 252, Dan\'s call to render it or declare it omitted. Its only matches are `s.valid` in openSigDetails, a sign.SignerInfo.',
  'pendingView.acceptedPeer': 'same card, same decision (/pending 252). Its only match is `a.acceptedPeer` in augmentSigDetails, an attestationView.',
  'attestationView.signer': 'read in Go by the side that SETS it (internal/p2p), never at the far end; the client renders signer identity from sign.Status instead. Its only match is `pending.signer`, a pendingView.',
  'attestationView.fingerprint': 'as above. Every match is a peer / peersResponse / pendingView fingerprint; augmentSigDetails reads acceptedPeer, reason, matched, pinned, rosterHash and oneProceeding, not this.',
  'attestationView.when': 'as above. Matches are `q.when` (cosignQuote) and `s.when` (sign.SignerInfo).',
  'attestationView.valid': 'as above. Matches are `s.valid`, a sign.SignerInfo.',
  // Reached through docResponse.Signature (and its two embedders) once the nested walk landed —
  // /pending 259. Both are read in Go only by the side that SETS them (internal/p2p builds every
  // attestation from them), which by this file\'s own updateResponse.managed doctrine is no
  // consumer at the far end; the client renders signer identity from attestationView instead.
  'sign.SignerInfo.reason': 'its ten matches are `pending.reason` (pendingView), `a.reason` (attestationView), `out.reason` (decryptResponse), `info.reason` (listDirResponse) and `ev.reason` (a PromiseRejectionEvent). openSigDetails never reads it.',
  'sign.SignerInfo.fingerprint': 'the exact twin of attestationView.fingerprint, one type over. Every match is a peer / peersResponse / pendingView fingerprint.',
};

test('the deferred and coincidental lists describe fields that still exist', () => {
  // A deferral for a field the shape no longer has is a check that silently stopped running —
  // the same third direction UNREAD_KNOWN needed after toolbarStyle was deleted and nothing
  // noticed. Both lists get it, because both are ways of NOT scanning something.
  const badDeferrals = [];
  for (const entry of PUBLISHED) {
    const st = STRUCTS.get(entry.type);
    if (!st) continue;
    for (const f of entry.deferredFields ?? []) {
      if (!st.fields.some((x) => x.name === f)) badDeferrals.push(`${entry.type}.${f}`);
    }
  }
  assert.deepEqual(badDeferrals, [],
    `these are listed in deferredFields and are not fields of their shape any more: ${badDeferrals.join(', ')}. The shape changed and the deferral outlived it.`);

  const vanished = Object.keys(COINCIDENTAL).filter((k) => !EVIDENCE.has(k)).sort();
  assert.deepEqual(vanished, [],
    `COINCIDENTAL names fields that no longer exist on any published shape: ${vanished.join(', ')}`);

  // STIMULUS, and it is the assertion that makes the whole deferral mechanism honest: a
  // deferred field must actually be skipped. If deferredFields were silently ignored, every
  // test here would still pass and `.detail` would be laundered again by the same line.
  // STIMULUS for the nested walk. Asserting on `signature` itself would pass with the recursion
  // deleted — it is docResponse's OWN field. `timeBacking` is reachable ONLY through
  // docResponse.Signature -> sign.Status.Signers -> sign.SignerInfo, so it is false the moment
  // the walk stops walking.
  assert.ok(EVIDENCE.has('sign.SignerInfo.timeBacking'),
    'sign.SignerInfo.timeBacking is not in the inventory, so named struct-typed fields are being followed nowhere and every shape they carry is unchecked');
  const nestedKeys = [...EVIDENCE.keys()].filter((k) => k.split('.').length === 3);
  assert.ok(nestedKeys.length >= 18,
    `only ${nestedKeys.length} fields were reached through a named struct field; the census at v1.117.122 was 21 across sign.Status, sign.SignerInfo, pdfops.ScanReport, pdfops.Finding, pdfops.OutlineItem, pdfops.AttachmentInfo and vault.KeyInfo`);

  const deferred = [...EVIDENCE.values()].filter((e) => e.deferred);
  assert.ok(deferred.length >= 6,
    `only ${deferred.length} fields are deferred; P06's two diagnosis shapes defer three each, so the mechanism is not being applied`);
  assert.ok(deferred.every((e) => e.lines.size === 0),
    'a deferred field collected evidence, so deferral is not actually skipping the scan');
});
