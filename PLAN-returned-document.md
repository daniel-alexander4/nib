# PLAN — the document that came back changed

**Dateline.** Seeded 2026-09-08 from `/grill "I've cryptographically signed a document and sent it
off. Someone else has modified it and sent it back. They say they haven't changed it."` Every
number and every verdict below was produced by running something on 2026-09-08, not by reading
code. Dan settled the retention question the same day (option C — opt-in, `/discuss`-style choice
at the gate).

**Where this plan and the request differ, the plan wins**, and it differs in one load-bearing
place. The request asks for *"a notice that the sig is no longer valid"*. Measured, the shipped
badge already says that in three of four tamper classes — and in the fourth, the one that matters
most, it says **`✓ Untampered`**. The plan is built against what was measured, not against the
premise.

~~**Status: unbuilt.** No slice has started.~~ **(2026-10-02)** P01 is done (v1.179.2). Paused 2026-09-30 and resumed on Dan's option A on 2026-10-02. P02 is next. **(2026-09-28, P01 phase-open)** `/deepdive` and `/plan-review` of the
firmed P01 ran; their pins are in P01 and several "What is already true" bullets and standing caveats are superseded by
them — marked below where they are. **There is no P00** — nib is twelve hundred commits old
and needs no bootstrap.

---

## What this is

The dispute surface. One command that opens one page and answers, for a signed document that has
come back from a counterparty: *what does this file prove, what changed since I signed it, and who
else touched it.*

Nib already ships both halves and has never joined them. The missing piece is not a diff engine
and not a verdict — it is **the version you signed**, recovered from the returned file's own bytes
and handed to the differ that already exists.

## What is already true, and was read rather than assumed

- **A signature verdict is computed on every install and mutation** and rides on the document
  metadata as `Signature sign.Status` (`internal/server/server.go:516`). There is **no
  `/api/verify`** — named search across `.go`, `.js`, `.html`, `.mjs`: zero hits.
- **The badge already distinguishes three states and a caution** — `✓ Untampered` /
  `⚠ Modified since signing` / `Unsigned`, plus `· content added after signing`
  (`web/app.js:4302-4314`). The details modal renders per-signer rows with an honest time backing
  — TSA / self-asserted / none (`web/app.js:4330-4372`).
- **A full comparison surface shipped at v1.72.0**: word diff, side-by-side, per-pixel difference
  map, page auto-align across insertions and deletions, two independent pagers
  (`web/index.html:964-999`, `web/app.js:3837`). It is entirely client-side and the second document
  never leaves the machine.
- **The differ takes bytes, not a file.** `pdfjsLib.getDocument({ ...PDFJS_OPTS, data: buf })`
  (`web/app.js:3856`) — the `File` picker is the only thing coupling it to a user's disk.
- **The byte offset this whole plan needs is already computed and thrown away as a bool.**
  `trailingContentAfterLastSignature` walks AcroForm `/Fields`, takes
  `max(ByteRange[2] + ByteRange[3])`, and returns only `len(pdf) > maxEnd`
  (`internal/sign/verify.go:200-233`).
- **That walk sees top-level fields only.** No `/Kids` recursion; named search
  `grep -rn "Kids" internal/sign/` returns **zero hits**. `signatureBlobPresent`
  (`verify.go:167-187`) has the same blindness, so both of nib's walks agree wrongly.
- *(superseded in part by P01's PIN 2026-09-28: the xref sweep in `signercert.go` already maps signer to dictionary
  by position; the line cites above have drifted)* **The verify library cannot map a signer to a byte range.** `verify.Signer`
  (`digitorus/pdfsign@…/verify/types.go:63-80`) carries name, validity, certificates and times —
  **no ByteRange, no field name, no object pointer.** It computes the range internally and discards
  it. This is the single fact that decides the cost of this plan.
- **A destroyed signature does not always report itself as invalid.**
  `internal/p2p/l3.go:222-226` records it as measured: *"it VANISHES."*
- **Solo signing retains nothing.** `nib sign` (`internal/cli/commands.go:856`) and the GUI
  Finalize (`internal/server/finalize.go:118`) hand the bytes back and write no record, no hash and
  no copy. The ceremony path is the opposite: `~/nib/ceremonies/<id>/document.pdf` holds the exact
  bytes this machine signed, rewritten every hop, moved intact to `~/nib/ended/<id>/` at close-out
  (ADR-012) and consumed today only as a `bytes.HasPrefix` test
  (`internal/server/delivery.go:251`).
- **`~/nib/signed/` already exists and already means "the document IS signed"** — its own comment
  says so (`internal/server/delivery.go:288`), and `receivedSubdir` routes there too
  (`internal/server/session.go:1879-1887`).
- **There will never be a Go-side differ.** `/pending 46` declined it on a measured probe: pdfcpu
  has no text extraction, and `digitorus/pdf` returns empty or garbage on embedded-CID text where
  poppler recovers it correctly. The text layer exists only in the browser.

## The measured starting point

Five tamper classes, run against a nib-signed fixture at v1.128.29. **This table is the reason the
plan differs from the request.**

| What the counterparty did | `nib verify` reports |
|---|---|
| nothing | `valid`, 1 signer |
| appended an incremental update (annotated it) | `valid`, 1 signer, **`addedAfter=true`** |
| edited a byte in the body in place | `invalid`, 1 signer, `valid:false` |
| re-saved wholesale in another tool (pdfcpu) | `invalid`, 1 signer, `valid:false` |
| corrupted the signature blob | `invalid`, 0 signers |
| truncated back to a prior revision | **`unsigned`** |

- **Prefix recovery works, exactly.** For a signed-then-appended document,
  `pdf[:ByteRange[2]+ByteRange[3]]` is **byte-identical** to the signed original, parses via
  `digitorus/pdf`, and `sign.Verify` reports it `valid` with 1 signer.
- **And the naive form of it is spoofable — built and reproduced.** A stranger appends changed
  content *plus their own self-signed signature covering to EOF*. Result: `state=valid`,
  `addedAfter=false`, **2 signers, both `valid=true`**, `maxEnd == len(file)`, and the recovered
  "signed revision" is the whole file — so a diff renders **nothing changed**. The attacker needs
  no key of yours. This is already true of the shipped badge.
- **Cost.** `nib verify` end to end (2× `dpdf` parse + fork/exec): **0.25–0.33 s** at 140 KB,
  **3.6–3.8 s** at 1.4 MB. Recovery adds one `dpdf` parse, not a `ContentDigest`. Nothing above
  1.4 MB was measured; ADR-013 records `DocumentHash` at 8.7 s on 4.4 MB, so growth is not flat.

---

## The law this establishes

**The anchor is your own signature, and nothing later may move it.** Every question this surface
answers is of the form *"since **I** signed"*. A rule keyed on "the last signature" or "the largest
byte range" answers a different question, and the difference is not academic — it is the
reproduced attack, in which the counterparty's own signature silently becomes the thing the user
is shown as their own. Anywhere this plan compares, slices, or reports, the subject is the
signature whose certificate fingerprint is the user's.

---

## Decisions

### D1 — The anchor is the user's signature, selected by fingerprint *(settled 2026-09-08 via /grill — OVERTURNS the pre-grill shape)*
The prefix is taken at the coverage end of the signature whose PKCS#7 leaf SPKI fingerprint matches
the user's identity, never at `max` over all fields. The pre-grill plan used `max` because that is
the number `trailingContentAfterLastSignature` already computes; the spoof above is what that costs.
`SignerInfo.Fingerprint` (`internal/sign/verify.go:55`) already carries the identity on the read
side, so the vocabulary exists.

### D2 — The revision walk is ONE door, and it learns identity *(settled 2026-09-08 via /grill — ADR-009)*
`sign.Revisions(pdf) ([]Revision, error)` is the only ByteRange walk in the tree.
`trailingContentAfterLastSignature` is re-expressed over it rather than kept beside it. A guard
asserts the single implementation, in the shape `watchLink`'s guard already uses. **A second walk
is what ADR-009 exists to refuse** — one review found a single predicate with three
implementations that disagreed, and nib's two current walks (`verify.go:200` and `verify.go:167`)
are already a pair that share a blind spot.

### D3 — Identity comes from parsing `/Contents`, because the library will not give it *(settled 2026-09-08 via /grill)*
`verify.Signer` has no ByteRange and no field name, and the two enumerations run in different
orders — the library scans `rdr.Xref()` for `/Adobe.PPKLite`, nib walks `AcroForm/Fields` — so
index-matching the two lists is unsound. That is the same two-enumerations hazard
`addedAfterVerdict` already names in its own comment (`verify.go:98-120`). `Revisions` therefore
parses each field's `/Contents` itself and fingerprints the leaf with the existing `fingerprintOf`
(`internal/sign/identity.go:46-49`). `github.com/digitorus/pkcs7` moves indirect → direct in
`go.mod`; no new module enters the graph.

### D4 — A prefix is rendered only if it parses AND that signer re-verifies inside it *(settled 2026-09-08 via /grill)*
The observation that made this plan possible — the prefix parses and verifies — is promoted from a
measurement to a **precondition**. A ByteRange is attacker-controlled data in a file a counterparty
sent; a prefix that does not stand up on its own is an attacker-chosen byte range under a badge
nobody reads. Bounds-check before slicing: an out-of-range value panics a raw slice.

### D5 — The walk takes the LAST ByteRange pair, not elements 2 and 3 *(settled 2026-09-08 via /grill — PDF-format seat)*
ISO 32000-1 permits an even-length array of pairs and the verify library loops over all of them;
nib hard-codes `Index(2)+Index(3)`. On a multi-gap signature that truncates mid-file. Coverage ends
at `Index(n-2)+Index(n-1)`.

### D6 — A signature under `/Kids` is REPORTED, never skipped *(settled 2026-09-08 via /grill)*
Hierarchical field names are legal and are what other producers emit. Today such a document reaches
`AddedAfter`'s fail-closed arm and warns for the right answer by the wrong route; after this plan
the walk sees it. **The fail-closed arm stays** — fixing the walk must not remove the guard that
covered for it, which is exactly the "a warning goes quiet independently of the verdict" trap
`verify.go:98-120` was written against.

### D7 — ~~Five~~ Four named refusal causes, not one error *(settled 2026-09-08 via /grill; `kids-hidden` RETIRED 2026-09-29 by Dan — option A)*
`no-signature` / ~~`kids-hidden`~~ / `resaved` / `not-your-signature` / `prefix-failed-reverify`. A
lumped refusal reads backwards exactly when it matters, and each cause is a different sentence to
the user. Attribution beats aggregation.
**(superseded in part, 2026-09-29 — Dan, option A)** `kids-hidden` is retired: P01's revision index is an xref sweep, which
sees a `/Kids`-nested signature by construction, so the cause can never fire. The four remaining causes are the route's
vocabulary; P01's record causes (`malformed-byterange`, `byterange-outside-file`, `contents-elsewhere`,
`unsupported-filter`, `unparseable-contents`) are internal and each maps to exactly one of them at P02's route.

### D8 — Three terminal states, rendered honestly, never degraded *(settled 2026-09-08 via /grill — forensic-examiner seat)*
Append-only → the full diff. Re-saved wholesale → **"the version you signed is not inside this
file"**, because it is definitively not recoverable: one revision was emitted and the surviving
ByteRange numbers index a byte layout that no longer exists. No signature at all → say so. A
surface that degrades one of these into a weaker version of another is worse than no surface,
because its whole value is being quotable.

### D9 — Later signers are named, with whether this machine knows them *(settled 2026-09-08 via /grill)*
`AddedAfter`'s own doc says content between signatures *"is expected"* — true on a roster, **false
in a dispute**, and the difference is the reproduced attack. The ceremony path already models this
(`unrostered`, `Pinned` on `attestationView`); the solo path does not, so the available signal is
the pinned-peer list, and "not known to this machine" is the honest phrasing.

### D10 — Recovery is on demand; never on document open *(settled 2026-09-08 via /grill)*
It is one more `dpdf` parse on a path that already parses twice, and the measured cost is
superlinear in document size. Wiring it to open would put it on the request path CLAUDE.md's
hot-path rule guards. Inventory row P1 exists to police this.

### D11 — The diff engine stays client-side, and there will be no Go differ *(settled 2026-09-08 via /grill)*
`/pending 46` measured this and Dan declined it on the product-thesis line. The recovered prefix is
handed to the shipped compare pipeline as bytes. This plan adds no diffing code of any kind.

### D12 — The surface is a sheet *(settled 2026-09-08 via /grill)*
A reading surface shown in place of the viewer, the `#ceremonySheet` pattern — not a modal (too
small for a document plus a verdict plus a signer list) and not a document tab (it is about a
document, not another one).

### D13 — Retention at signing is opt-in and defaults OFF *(settled 2026-09-08 — Dan's call, option C)*
A **"Keep a copy for my records"** tick in the Finalize & sign flow. When ticked, the signed output
is also written to `~/nib/signed/`, named on `receivedName`'s existing 8-hex-digest shape
(`internal/server/session.go:1925`). No new directory and no new naming convention.
**Opt-in rather than always** is the decision: nib currently persists nothing on the solo path, and
turning that into unconditional retention is a posture change on a local-first tool that handles
PII by design. A **new persisted artifact plus a retention rule is architectural and gets an ADR in
the same change** (STANDARDS §11).

### D14 — A failed retention write surfaces at signing, not at the dispute *(settled 2026-09-08 via /grill — local-first SRE reasoning)*
If the copy silently fails to land, the user believes they hold evidence and does not — discovered
months later, in the one conversation where it mattered. That outcome is worse than never offering
the tick, so the write's failure is a signing-time error, not a log line.

### D15 — The retained copy is listable and deletable in the UI *(settled 2026-09-08 via /grill — privacy seat)*
The redaction presets are SSN, email, phone and card: nib handles PII by design. A feature that
starts persisting signed documents and offers no way to see or remove them fails on its own terms.

### D16 — The retained copy earns a real checklist tick *(settled 2026-09-08 via /grill)*
ADR-027: a Simple Sign step is ticked only where Nib can observe it, and eight steps are honestly
untracked. A file on disk **is** observable, so this is a step with a genuine probe rather than a
`—`.

### D17 — OpenTimestamps is NOT folded into retention *(settled 2026-09-08 via /grill)*
Stamping the retained copy at the moment of signing would make it far stronger evidence — a copy
you kept proves what you had, not what you sent. It is deliberately out of scope because the OTS
path has an unfinished half: nothing upgrades a pending proof once Bitcoin anchors it, so a stamp
taken here would report `pending` forever. That is `/pending 388`, and folding it in would widen
this plan into that one.

---

## Build order

### P01 — The revision index *(done 2026-10-02, v1.179.2)*
**Goal.** One walk over a document's signatures that knows, per signature, *who* signed and *how
far their signature reaches*.

**Exit criteria.** Exactly one ByteRange walk exists in the tree and a guard asserts it; a
signature nested under `/Kids` is seen; a multi-gap ByteRange resolves to its last pair; an
out-of-range ByteRange is refused rather than sliced; every existing `trailing_test.go` test is
green and `AddedAfter` keeps its fail-closed arm. **(amended 2026-09-28, phase-open, /pending 661 and 687 folded
per Dan's /discuss on /pending 389)**: `AddedAfter` measures only verified, well-formed signatures; a signature whose
ByteRange covers anything but the whole revision less its own `/Contents` is refused and is not a valid signer.

**Phase close 2026-10-02 (v1.179.2) — acceptance ledger, every clause split on `and`, checked at HEAD a403ac12:**
- ✅ Exactly one ByteRange walk exists — `sweep`, `internal/sign/revisions.go`; a repo-wide grep outside `internal/sign` finds only the test helper `internal/testpdf/copiedsig.go`.
- ✅ …and a guard asserts it — `TestEverySignatureEnumerationIsTheSweep` PASS.
- ✅ A `/Kids`-nested signature is seen — `TestAKidsNestedSignatureIsSeen` PASS.
- ✅ A multi-gap ByteRange resolves to its last pair — **as amended by D5's PIN**: an abutting six-element array ends at its last pair (`TestAnAbuttingSixElementByteRangeEndsAtItsLastPair` PASS), and a true second gap is refused by conjunct (5) (`TestEachStructureConjunctRefusesItsOwnFixture`, "(5) two gaps").
- ✅ An out-of-range ByteRange is refused rather than sliced — conjunct (6), `CauseByteRangeOutsideFile`: `TestEachStructureConjunctRefusesItsOwnFixture` "(6) one byte past EOF" and `TestAByteRangeThatReadsNothingIsARefusedRecordNotARefusedDocument` (length 999,999,999) PASS.
- ✅ Every existing `trailing_test.go` test is green — all five PASS (`go test ./internal/sign/` ok).
- ✅ `AddedAfter` keeps its fail-closed arm — `TestAddedAfterFailsClosed` PASS.
- ✅ (amended) `AddedAfter` measures only verified, well-formed signatures — `coverage` over `bounds()` (ADR-059); S02's tests in the package run.
- ✅ (amended) A signature whose ByteRange covers anything but the whole revision less its own `/Contents` is refused — conjuncts (1)-(11); the one shape the review found unrefused (the signer's own number redefined) is refused since v1.169.48 `cda73c99`, `TestADictionaryRedefinedUnderTheSignersNumberIsRefused` PASS.
- ✅ (amended) …and is not a valid signer — `countsAsSigner` (ADR-060); the same test asserts it.

Review: `code-reviews/v1.169.47-p01-phase-close-2026-09-30.md`, fully dispositioned. Its six criticals were fixed in
`cda73c99`; the re-review of that commit found one critical it introduced (the Scan action walk, plus the field walk's
sibling), fixed in this close. Out-of-phase findings filed as /pending 771-778 (770 merged into 735). Graduation pass:
`instruments/returned-document.md` (68 rows: keep-live 46, deleted 7). Required-run gates: tiers 0-3, plus tiers 4 and 6,
which fire because P01.S02 touched `internal/server/ceremonyid.go`. Results are recorded in the close commit.

**PIN 2026-09-28 (phase-open, read at the lines — `deepdives/2026-09-29-p01-the-revision-index-and-the-byterange.md`, which
is misdated in its filename).** Premises the code has moved past, amended here; no decision is struck.
- **D3's step is already built.** `pkcs7` is a direct dependency (`go.mod:11`, ADR-051) and the `/Contents` re-parse with
  `fingerprintOf` exists (`signercert.go:83-105`, `signerFingerprintsByBag`) — over the LIBRARY's own xref sweep, same
  predicate, same skip rule. D3's "the two enumerations run in different orders" is true of the `/Fields` walk
  (`verify.go:407-423`) and false of this one. So `Revisions` EXTENDS that sweep; it builds no third walk, and a
  `/Fields` walk would reopen ADR-051's hole (a decoy entry writing both sides of the key).
- **D2 is implemented by DELETING the `/Fields` ByteRange walk** — its "re-expressed over `Revisions`" is met because the
  walk's one job, `maxEnd`, becomes a reader of the records. `signatureBlobPresent` and `hasCertificationSignature` stay
  as named exemptions (see S02); D2's pairing of `signatureBlobPresent` with the ByteRange walk is answered there.
- **D6 is met by construction**: an xref sweep never walks fields, so a `/Kids`-nested signature is seen without
  recursion; the fail-closed arm stays (D6's own rule), guarded to its one caller.
- **D5 holds, and needs /pending 687's structure rule UNDER it** — the full conjunct list is S01's. Measured: a new
  dictionary carrying the victim's `/Contents` and `/ByteRange [victim's four, 999999999, 0]` verifies as a second signer
  under the victim's fingerprint; a last-pair rule alone would let it end coverage anywhere.
- **/pending 661 reproduced** (two decoy shapes, `valid addedAfter=false` over an appended revision): `AddedAfter`'s
  `maxEnd` is taken over VERIFIED, well-formed revisions only.
- **The join is positional, not a re-hash** (rung 2, hot path — **measured** by the plan-review's performance seat: the
  library is ~47 MB and 13-23 ms per signature at 10 MB, the sweeps under 3% of `Verify` past ~1 MB, so a per-signature
  `p7.Verify` would roughly double the dominant cost). Deleting the `/Fields` walk saves at most ~1.8 ms and is not
  quoted as offsetting anything.
- **The standing caveats moved**: the stranger-co-signs spoof now reads `⚠ 2 signatures · 1 from someone you have not
  verified` with the vault open (`/pending 390`'s `unverifiedSigners`); `/XRefStm` is settled — digitorus/pdf never reads
  it, so a hybrid-reference file verifies `unsigned` (a `State` defect filed as /pending 733, not P01's).

**PIN 2026-09-28 (plan-review of the firmed P01, hand-off `plan-reviews/2026-09-28-p01-returned-document.md`)** — ten
seats; every critical below is folded into the slice it concerns, marked **(plan-review pin: …)**.

#### P01.S01 — `sign.Revisions`: the library's own sweep, with identity, coverage and structure *(done 2026-09-29, v1.168.4)*
**Ledger** (acceptance split, each with its evidence): `/Kids` seen — met (`TestAKidsNestedSignatureIsSeen`, red against
a `/Fields`-only sweep); 6-element abutting → `Index(4)+Index(5)` — met; eleven conjuncts, each its own fixture asserting
WHICH fired — met (16 cases); past-EOF refused, not sliced — met (record refused; a read past the file never reaches the
library); both copied dictionaries refused — met (tests + **live, `nib verify`**: the old binary named the victim twice,
the new one leaves the copy unnamed); 661 decoys named with causes — met; fingerprint oracle = the fixture's signing key
incl. ADR-051's forged bag — met; parse-failing-before-valid — met; K-pair never reaches the library — met (**live**: a
negative-length K=2,000 file 284 MB/0.41 s → 23 MB/0.04 s; an indirect K=4,000 file 9.65 s `valid` → 0.01 s `invalid`);
hybrid yields no record naming /pending 733 — met; **cost < 5% at ≥ 10 MB — not measurable at this granularity by wall
clock** (39 MB × 30, 36 alternated samples each: median +5.15%, fastest −5.46%, so the noise is ±5%) and **met by the
finer instrument**: the added work is the sweep, 3.2 ms against the old walk's 2.9 ms inside a 2.4 s `Verify` (0.01%).
Review `code-reviews/v1.168.3-p01s01-returned-document-2026-09-29.md`: three rounds, four criticals fixed — two of them
PRE-EXISTING live defects (an indirect `/ByteRange` made `Verify` quadratic; a negative length made the library read the
whole file per pair). The gate refuses exactly what over-reads; a shape that reads nothing is a refused record and the
document keeps its signers (the S03 pin). Carried to S03's grill: a refused record the library FAILS still makes a
document `Invalid` (`verify.go`); a planted token can refuse a genuine signature (/pending 736).
Scope: extend `signerFingerprintsByBag`'s sweep into one record per signature-shaped dictionary — object number,
`/Type`, `/Filter`, `/SubFilter`, the raw `/ByteRange`, a structural cause, the coverage end (D5), the leaf fingerprint,
and whether the library's signer at that position verified. `signerFingerprintsByBag` is deleted; the record is the
**one home** of "who signed", "how far" and "well-formed". Refs: D2, D3 (pinned), D5 (pinned), D6, /pending 687.
- **(plan-review pin: the record set, forensic + data)** A record for EVERY xref dictionary carrying `/ByteRange` or
  `/Type /Sig`/`/DocTimeStamp` — including a non-PPKLite `/Filter` (cause `unsupported-filter`) and a `/Contents`
  `pkcs7.Parse` rejects (cause `unparseable-contents`) — so an examiner can name "object 31 claims to be a signature and
  is not one". The /pending 661 decoys are records with causes, never silent skips.
- **(plan-review pin: the join, data + crypto + security + consistency)** The positional join runs over EXACTLY the
  library's subsequence — records that pass its predicate (`/Filter /Adobe.PPKLite`, `pkcs7.Parse` succeeds) and the
  `SigFlags` gate — in xref order, BEFORE any structural refusal or filter is applied; ordinal i of that subsequence is
  the library's signer i. Every other record is `verified=false` by definition, never by index. The count must agree and
  the bag must agree at EVERY position (both empty agrees only as both empty); either disagreement is an error routed
  fail-closed. "Verified" means the library's `ValidSignature` for that position. A verified record with an empty
  fingerprint still bounds coverage (the hash was checked whoever made it) and never anchors D1, the roster or a signer
  count.
- **(plan-review pin: one home for "who signed", data)** `SignerInfo.Fingerprint` is taken from the record at its
  position, and the bag survives only as the per-position cross-check — **this supersedes ADR-051's "the join key is the
  bag", so S01 lands ADR-058 in the same change** (ADRs are never edited). The ambiguous-bag blanking disappears with the map.
- **(plan-review pin: the structure rule, security + crypto + PDF-format + QA — its conjuncts are FIXED at eleven)** A
  record is well-formed only if: (1) `/ByteRange` is a direct array of integers, even length ≥ 4; (2) the first start is
  0; (3) every length is > 0; (4) starts strictly ascend; (5) every pair begins where the previous one ended, EXCEPT
  exactly one gap; (6) the last pair ends inside the file (never past EOF — `processByteRange` truncates silently); (7)
  the gap is exactly one hex-string token: `file[gapStart] == '<'`, `file[gapEnd-1] == '>'`, only hex digits and PDF
  white space between, no `>` before the last byte; (8) its strict decoding equals this dictionary's `/Contents`
  `RawString`; (9) `/Contents` is a direct string, not an indirect reference; (10) the dictionary is not held in an
  object stream (`x.Stream()` is zero — a compressed `/Contents` has no file offset); (11) **the gap belongs to THIS
  object**: scanning back from `gapStart` through bytes the signature itself covers, bounded (64 KiB), the nearest
  `N G obj` header equals the record's own object number, and only `/Contents` and white space sit immediately before the
  `<`. (11) is what refuses a verbatim copy with the victim's own four numbers: the bytes before the gap are the victim's
  signed bytes, so a copy under any other object number fails it, and a copy reusing the victim's number REPLACES the
  victim in the sweep (one signer, harmless). The digitorus reader exposes no offsets (`read.go:118-131`), which is why
  (11) is a backward scan of signed bytes and not an offset lookup; nothing searches the whole file (`bytes.Index` over
  file bytes is refused by the guard). Causes: `malformed-byterange` (1-5, 7-10), `byterange-outside-file` (6),
  `contents-elsewhere` (11). The revision a signature covers is `[0, end of its last pair)` — every signature but the
  last legitimately ends before EOF. **Remedy status**: (1)-(10) are local checks; **(11) is unverified until S01's
  first task measures it** on nib's own output and both real signed producers.
- **(plan-review pin: before the library, performance)** The sweep runs BEFORE `verify.Verify`, beside the `pdfcpuCanRead`
  gate: a PPKLite record with an odd pair count, or pair lengths summing past `len(data)`, returns `Invalid` without
  calling the library — which today copies every named pair into memory (`signature.go:70-84`), so `[0 S 0 S … ×K]`
  allocates K×S and a Go out-of-memory is not recoverable. **This closes a live defect on the upload, install and undo
  paths**, not only a P01 one.
- **(plan-review pin: DocTimeStamp, PDF-format)** A `/Type /DocTimeStamp` record reads `ValidSignature=false` from the
  library (inferred: pkcs7 fills `p7.Content` with the TSTInfo before the ranges are appended). S01 records `/Type` and
  `/SubFilter`; S02 decides its coverage (below).
Acceptance:
- A hierarchical (`/Kids`) signature field appears in the result; today's `/Fields` walk returns nothing for it.
- A well-formed 6-element (abutting) ByteRange resolves to `Index(4)+Index(5)`.
- **Each of the eleven conjuncts has its own named negative fixture**, refused with its cause — including a key injected
  into the gap after an early `>` (security's `/Reason` injection), an overlapping pair, a zero-length pair, and a first
  start of 1.
- A ByteRange exceeding the file length is refused, not sliced.
- Both copied-dictionary fixtures — the victim's four numbers plus `999999999 0`, AND the victim's exact four numbers
  under a new object number — yield a REFUSED record.
- Both /pending 661 decoy shapes yield a record naming their object number and cause.
- **The fingerprint oracle is outside the code (plan-review pin: QA)**: each verified record's fingerprint equals the
  fingerprint of the key the fixture was SIGNED with (the test's own certificate) — including the ADR-051 forged-bag
  shape, which must yield the attacker's fingerprint, not the victim's. Never "equals what `Verify` reports", which after
  this slice is the same record.
- A parse-failing PPKLite dictionary placed BEFORE a valid one leaves the valid signer's `verified` and fingerprint on
  the right record.
- A K-pair ByteRange (K = 10,000, lengths summing past the file) returns `Invalid` with the library never called —
  allocation measured and bounded.
- The hybrid-reference fixture yields no record, its assertion naming /pending 733 so fixing that item flips it on purpose.
- **Cost (plan-review pin: performance)**: `Verify` measured over signatures {1, 10, 30} × signed bytes {1, 10, 50 MB},
  before and after, time and `TotalAlloc`; P01 adds under 5% to whole-`Verify` at 10 MB and above — a threshold that can
  fail. The library's copy-per-signature cost is filed separately (/pending 735), not P01's.

**Tasks** (slice grill 2026-09-28, `memory/grills/2026-09-28-p01s01-revisions.md`; conjunct (11) MEASURED on 7
signatures in 5 files — Finalize's `Sign`, `SignExternal`, three stacked `SignApproval`s, `irs-f1040`, `irs-fw9` — the
nearest header was the dictionary's own object every time, 199 B back on nib's output and 501 B on the IRS files; a
verbatim copy under a new number found the victim's header and was refused; (7) held on all 7, padding included):
T01 — `internal/sign/revisions.go`: the `Revision` record and `sweepRevisions` over `r.Xref()`, each record's library
ordinal (none when `SigFlags` is null), `unsupported-filter` / `unparseable-contents`. T02 — `structureOf`: conjuncts
(1)-(10), then (11) as `gapOwner`, a LINEAR backward keyword scan bounded below by the previous distinct gap's end (a
regex over the window measured 3.2 ms per IRS signature; the linear scan 2-7.5 µs); no regex, no `bytes.Index` over the
file. T03 — `revisions_test.go`: one fixture per conjunct, each asserting WHICH conjunct fired (the cause lumps nine);
both copied dictionaries (xref-stream appends); both 661 decoys; `/Kids`; the 733 hybrid; a scan-budget fixture;
per-source refusal counts. T04 — `Verify`: sweep and K-pair gate after `pdfcpuCanRead`, a `libraryVerify` seam proving
the library is never called on the K = 10,000 fixture, allocation bounded. T05 — `joinLibrary`; `signerInfo` takes the
record's fingerprint; a join error feeds `addedAfterVerdict`'s error arm; `signerFingerprintsByBag` and `recordSigner`
deleted; `forgedbag_test` rewritten (`TestOneBagWithTwoSignersNamesNeither` becomes "names each"). T06 — ADR-058 +
`_index.md`, the CLAUDE.md ADR line, stale comments. T07 — the cost matrix, `oldtree` of `16da8b58` vs the new tree,
the slice failing at ≥ 5% over 10 MB. T08 — `corruptwalk_test.go` drives `sweepRevisions` too.
**(grill pin: the join on a failed signature, 2026-09-28)** The library returns an EMPTY bag on every failure path
(`signature.go:36-63`; filled only at `certificate.go:345`), so "both empty agrees only as both empty" would make every
tampered document a join error and blank its valid co-signers. The rule is: an empty library bag requires
`ValidSignature=false` at that position; a non-empty one must equal the record's. **Defaults (rung 2)**: a join error
makes `AddedAfter` fail closed already in S01; the scan's lower bound is the previous gap's end; the cost matrix is a
recorded probe, not a standing test; the conjunct field is unexported and read by tests only. **Accepted residual**:
where a producer writes attacker-influenced `/Reason` or `/Name` BEFORE `/Contents` (the IRS files do; nib does not),
header-shaped text there can move which object a gap is attributed to (/pending 736). **(diff-review correction,
2026-09-28)** Coverage and identity cannot move; three things CAN: a planted hex token or header puts a victim's own
signature below the scan floor, so it is REFUSED — after S03 that LOWERS the signer count, and after S02 it moves
`maxEnd` (fail-closed, an `AddedAfter` warning); and a planted header naming a copy's object hands the copy the
attribution, so its unsigned `/Reason`, `/M` and `/Name` are the ones reported. S03's grill owns the first; the second
fails safe; the third is /pending 736.

#### P01.S02 — one door: `AddedAfter` over verified, well-formed revisions *(done 2026-09-29, v1.169.0)*
**Ledger**: `trailing_test.go` green and unchanged (its prose corrected) — met; `TestAddedAfterFailsClosed` red with the
error arm cut — met; both 661 decoys (LISTED in `/Fields`, the shape that fooled the old walk) and both copied dictionaries
read `addedAfter=true` with cause `refused-signature-present` — met in Go and through `cmdVerify` (exit 2, the refused line);
**live**, the built `nib verify`: honest exit 0, copied exit 2 naming `object 22 … contents-elsewhere`, both hostile K-pair
files exit 2 with their refused line; the guard fails on a second walk — met, and on 14 bypass shapes the first cut missed
(`TestTheSigwalkGuardSeesEveryBypass`); the pre-S02 red proof — TAKEN on `11490690` before the walk was deleted
(`docs/red-proofs.md`). Review `code-reviews/v1.168.4-p01s02-returned-document-2026-09-29.md`, three rounds.
**(review pins, 2026-09-29 — supersede the T03 order and the zero-signer grill pin above)** Cause order: error or NO
bounding record → `could-not-check` first (a refusal must not claim an append nobody measured); then any refused →
`refused-signature-present`; else `appended`; the badge names both facts. The zero-signer rule exempts ONLY a well-formed
signature the catalog's `/Perms` names that the library never enumerated (a `/Perms /UR3` form without `/SigFlags`) —
exempting everything outside the library's enumeration let an appended revision strip `/SigFlags` and UNSIGN a signed
document. The DocTimeStamp imprint check is OFF the verdict path entirely (11% of `Verify` at 10 MB per stamp; an honest
RSASSA-PSS token is refused by `timestamp.Parse`): on demand in P02/P03 (D10). Placeholders (empty `/Contents`) are never
listed as refused. ADR-059.
Scope: delete the `/Fields` ByteRange walk; `maxEnd` over records that verified and are well-formed; a count or bag
disagreement is an error (fail-closed). Guards: `Key("ByteRange")` / `Xref()` in exactly one non-test function of
`internal/sign` plus a named allow-list checked at site markers (ADR-009 shape: `signatureBlobPresent` — re-expressing it
over a PPKLite-only sweep would NARROW it towards `Unsigned` — and `hasCertificationSignature`, whose `/Kids` blindness is
declared at its site and filed as /pending 734); `addedAfterVerdict` has exactly one caller. Stale comments corrected
(`verify.go:5-6`, `:227-229`, `:380-383`, `ceremonyid.go:802-806`). Refs: D2, D6, /pending 661.
- **(plan-review pin: DocTimeStamp coverage, PDF-format)** A DocTimeStamp record bounds coverage only when its RFC 3161
  `messageImprint` matches the hash of its ranges (`digitorus/timestamp` is already a dependency); otherwise it is a
  refused record with its own cause. Without this, "verified only" reads `addedAfter=true` on every PAdES B-LTA document.
  **Remedy unverified until built**; a B-LTA fixture is built for it, or the case is declared if none can be produced.
- **(plan-review pin: zero-signer path, data)** On the zero-signer path `Verify` answers `Invalid` when
  `signatureBlobPresent(data) || len(records) > 0` — a `/Kids`-nested signature whose PKCS#7 fails no longer reads
  `Unsigned` while `Revisions` holds a record. Widening, the safe direction.
- **(plan-review pin: say why it moves, PDF-format)** After S02, non-PPKLite and `adbe.x509.rsa_sha1` dictionaries stop
  bounding `maxEnd` (the `/Fields` walk never read `/Filter`); `AddedAfter` moves the safe way, and that is stated in the
  change so it is not read as a regression.
- **(plan-review pin: say which fact, forensic)** `Status` gains `Refused` — every refused record (object number,
  `/Filter`, cause) — and the `AddedAfter` bit is joined by `AddedAfterCause`: `appended` | `refused-signature-present` |
  `could-not-check`. The bit stays fail-closed. **`AddedAfter` is never rendered or quoted as "unchanged since you
  signed"** — that answer is P02's fingerprint-selected prefix.
Acceptance:
- Every test in `internal/sign/trailing_test.go` is green, unchanged.
- `TestAddedAfterFailsClosed` still goes red when the error arm is removed.
- Both /pending 661 decoys and both copied dictionaries read `addedAfter=true` with cause `refused-signature-present`;
  `nib verify` exits 2 on them and prints the refused object and its cause.
- The guard fails if a second ByteRange walk is introduced, and if `bytes.Index` reads file bytes in `Revisions`.
- **The pre-S02 red proof is TAKEN INSIDE THIS SLICE (plan-review pin: QA)**, before the walk is deleted, or against
  `oldtree` of the pre-S02 commit: the 661 fixtures defeat the `/Fields` walk.

**Tasks** (slice grill 2026-09-29, `memory/grills/2026-09-29-p01s02-addedafter.md` — verdict AMENDED): T01 — a
LISTED-decoy fixture and the recorded pre-S02 red proof (S01's `appendDecoy661` never lists the decoy in `/Fields`, so on
HEAD it already warns and a proof built on it is vacuous; the listed variant reads `valid, addedAfter=false` on HEAD for
both filters — that is the proof). T02 — `Revision.hasContents`, `Timestamp`, the imprint check. T03 — delete the
`/Fields` walk; `coverage` and an `addedAfter` wrapper as `addedAfterVerdict`'s one caller (its body untouched for the
replay script); cause order: error/join → `could-not-check`, any refused → `refused-signature-present`, no valid signer →
`could-not-check`, else `appended`; the zero-signer rule; the `Status` fields. T04 — tests (incl. `TestTheJoinTolerates…`
now asserting cause ≠ `could-not-check`, and the two tests that called the deleted walk). T05 — the guard: `.Xref()`,
`.Key("ByteRange")`, `.Key("Fields")` only in `sweep`, a `//sigwalk:exempt <name>` marker per other site each matching
exactly one call (the shape of `pdfread/pagecontent_guard_test.go`); no `regexp`/`bytes.Index*`/`Contains*` in
`revisions.go`. T06 — the readers: `describeStatus` and the CLI line per refused record (`/Filter` printed `%q`, capped
at 64 bytes — attacker-typed), the badge suffix by cause, the details modal listing refused records, `app.js`'s "not
covered by any signature" note → "by any VALID signature"; jsdom tests; `observables_test.go` and `published.test.mjs`
learn the new fields. T07 — the imprint cost at 10 MB. T08 — ADR-059, stale comments, minor version bump (refused
signatures are a new thing a user is shown).
**(grill pin: DocTimeStamp, 2026-09-29 — AMENDS the plan-review pin above)** A DocTimeStamp NEVER bounds coverage: it names
no signer, a public TSA stamps any hash (/pending 708 showed a self-minted one), so "imprint matches → bounds" would let
anyone hide appended content behind a timestamp. The imprint only decides whether the record is refused
(`timestamp-unverified`) or not. Measured: the library reads a DocTimeStamp `ValidSignature=false`; the imprint check
works (`timestamp.Parse` + a hash over the ranges → MATCH). An honest B-LTA document therefore reads `AddedAfter=true,
appended` — accurate: the stamp's revision was added after the last signature. **Found live**: every B-LTA document
reads `Invalid` TODAY, the stamp counted as a failed signer (/pending 737, carried to S03).
**(grill pin: zero-signer rule)** `Invalid` when any record has non-empty `/Contents` (the rule `signatureBlobPresent`
uses) — not `len(records) > 0`, which would turn a prepare-for-signing placeholder (`/Type /Sig`, empty `/Contents`)
from `Unsigned` into `Invalid`. It overlaps S01's join branch and does not conflict.
**(grill pin: the guard)** After S02 neither `signatureBlobPresent` nor `hasCertificationSignature` calls `.Xref()` or
`.Key("ByteRange")`, so the planned allow-list excuses nothing; the guard looks for the three calls with a marker per site.
**Defaults (rung 2)**: three causes, the note reworded rather than a fourth; the readers land in S02; minor bump.

#### P01.S03 — a refused signature is reported, and is not a signer *(done 2026-09-29, v1.169.1)*
**Ledger**: both copied-dictionary fixtures verify with ONE signer and one `Refused` entry, `State` as for the untouched
file — met (Go + **live**, `nib verify`: `valid (1 signer(s))`, the copy named `contents-elsewhere`; before S03 the binary
said 2 signers); the tier-1 table tests — `NextPlacement` [40 136 320 220] (was 232), `ContributionProgress` done=1 (was
`ErrPrefixMismatch` — **a copied dictionary halted a ceremony**), `ReadAttestations` 1 (was 2), `Completeness` and
`confirmCoSigned` unchanged, `unverifiedSigners` 0, `signersSoFar` 1 row — each EQUAL to the untouched file (`internal/p2p`,
`internal/server`); badge and `nib verify` show the refused line (jsdom + CLI); B-LTA: `invalid, 2 signers` → `valid, 1
signer`, timestamp named (/pending 737's `State` half); precondition: zero structural refusals per source (measured at the
grill), and the two real signed producers still exit 0; tiers 4 (+ `-n 4`) and 6 at the slice gate. Review
`code-reviews/v1.169.0-p01s03-returned-document-2026-09-29.md`: **(review pin)** a record is a TIMESTAMP only when labelled
one AND its PKCS#7 encapsulates content — relabelling a failed genuine signature (13 bytes in its own coverage) had turned
`invalid` into `valid` with "a document timestamp is present"; now it stays a failed signer. `digitorus/pkcs7` does not
expose the eContentType OID (ADR-060 says why the weaker test suffices).
Scope: `Verify` does not count a refused record as a signer. **(plan-review pin: SRE + forensic + data + architect)** A
structural refusal is NOT expressed as `SignerInfo.Valid=false` — that would turn the whole document `Invalid`
(`verify.go:165-170`), read as "the victim's signature was tampered with", and halt a ceremony (`l3.go:287`,
`p2p/session.go:1210-1222`). Instead: refused records live only in `Status.Refused` (S02) and are excluded from
`Signers`; `State` is computed over signers as today; the badge, the details modal and `nib verify` say **"a signature
Nib refused is present"** with the cause and object number, and a refused row never shows the victim's name or
fingerprint as its signer. So `len(Signers)` counts real signers and the co-sign placement index (`p2p/cosign.go:48`) and
`signersSoFar` (`session.go:2291-2298`) are right with no change at those sites. **Declared**: a document carrying a
refused copy places the next co-sign block differently under an old build and a new one — only in that attack shape.
**Precondition, measured first (plan-review pin: QA)**: counted PER SOURCE — the generated corpus, each of nib's own
signing paths by name (Finalize, `nib sign`, the co-sign, the ceremony hop), and the real-producer corpus (≥ 2:
`designer/irs-f1040.pdf`, `designer/irs-fw9.pdf`, both `[0 a b c]`), with zero structural refusals; a missing corpus
prints `SKIP (not a pass)`, and a producer that fails the rule stops the slice and is parked. Refs: /pending 687.
Acceptance:
- Both copied-dictionary fixtures verify with ONE signer and one `Refused` entry, `State` as for the untouched file.
- A tier-1 table test feeds the copied-dictionary document through `Completeness`, `l3` `Progress`, `confirmCoSigned`,
  `NextPlacement` and `unverifiedSigners` and asserts each outcome — tiers 4 and 6 run the same binary on every side and
  cannot see this, so they are the regression backstop, not the acceptance.
- The badge and `nib verify` show the refused line on the fixture (jsdom + CLI test).
- Tiers 4 and 6 green.

**Tasks** (slice grill 2026-09-29, `memory/grills/2026-09-29-p01s03-refused-not-signer.md` — verdict AMENDED: a signer is
a record that is well-formed AND NOT A TIMESTAMP, or /pending 737 stays open — no DocTimeStamp is ever refused). Measured on
`939c7986`: the copied dictionary reads `valid, 2 signers` and **halts a ceremony today** (`ContributionProgress` →
`ErrPrefixMismatch`), moves `NextPlacement` (136 → 232), adds an `unverifiedSigners` row and a victim-named `signersSoFar`
row; approval + DocTimeStamp reads `invalid, 2 signers`. **Precondition measured: zero structural refusals** — Finalize 1/1,
`SignExternal` 1/1, the co-sign / ceremony hop 3/3, `irs-f1040` 1/1, `irs-fw9` 1/1; 2 of the 36 real-producer files carry
signatures, 0 refused. T01 — one `countsAsSigner()` predicate (`Cause == "" && !Timestamp`), `bounds()` built on it; a
library signer whose joined record does not count is skipped. T02 — zero counted signers: `Invalid` if `anyCheckableBlob`,
else `Unsigned` (a lone signer relabelled `ETSI.RFC3161` must not read unsigned). T03 — on a join error nothing is excluded
(declared). T04 — `addedAfter`'s fourth argument stays `len(resp.Signers) > 0`, never `len(st.Signers)` (red-proved). T05 —
`Status.Timestamps []uint32`, on the CLI and in the details modal, in both censuses. T06 — the badge shows a refusal whenever
there is one, whatever `AddedAfter` says; Invalid with no signer says "no signature Nib could check". T07 — `nib verify`
lines; exit 2 whenever a signature is refused. T08 — `testpdf.CopiedSignatureDictionary` (byte surgery; `testpdf` must not
import `sign`). T09 — table test in `internal/p2p`: `NextPlacement`, `Progress`, `Completeness`, `confirmCoSigned`,
`ReadAttestations`, each EQUAL to the untouched file. T10 — `internal/server`: `unverifiedSigners`, `signersSoFar`. T11 —
sign-level cases (`revisions_test.go`'s `len(st.Signers) != 2` becomes 1). T12 — jsdom + CLI tests. T13 — ADR-060: a
signer is a well-formed record that is not a timestamp. T14 — tiers 4 (`-n 4`) and 6 at slice close.
**Carries settled (rung 2)**: a refused record's failed library verdict does not set `State` — every byte inside a counted
signer's coverage is hash-bound, so a refused record can hide only a change past the last counted signer, which
`AddedAfter` reports. /pending 736 accepted: a refusal can REMOVE a signer, never add or rename one, and a removed signer
fails closed at `Completeness`, `confirmCoSigned` and `Progress`; counting a refused record whose fingerprint matches the
roster is rejected — that is exactly how the copy would count. **Defaults**: a timestamp-only document stays `Invalid` with
the stamp named; `nib verify` exits 2 whenever a signature is refused; B-LTA reads valid + "content added after signing" +
the timestamp line (a smarter cause is 737's residue); a join error keeps today's counting.

#### P01.S04 — the red proofs *(done 2026-09-30, v1.169.47)*
**Ledger**: every conjunct has a proof red for its own assertion — met per ARM (18 replayable rows, `recorded` 507 → 525):
(1) direct, integer, length and parity arms, (2)-(4), (5) overlap and count, (6), (7) token, (8) bytes, (9), (10), (11); **(7)'s delimiter arm and (8)'s length arm are
equivalent mutations** (the delimiter arm only while `pkcs7.Parse` precedes the structure rule) (argued in `docs/red-proofs.md`, not replayable), and **(8)'s bytes arm had no fixture** — one
added. The join's shift detection — met (`join-count-unchecked`; the plan's "a refused record" is the same path, since
`joinLibrary` reads only positions). The K-pair gate — **proved red, recorded as prose, not a row**: the mutated test
allocates ~15 GB. G1 — met, red at `Revisions`' join (fail-closed) before the test's own message; G2 — met; G3 — the (6)
row plus the K-pair proof. "The /Kids proof fails against the pre-S01 walk" — taken at S01/S02 (ledger above).
**Tasks**: T01 — the rows in `test/redproofs/` and the section in `docs/red-proofs.md`; T02 — the same-length (8)
fixture; T03 — the inventory's re-check (`instruments/returned-document.md`).
Scope: a proof per new refusal and per gap-down, registered in `docs/red-proofs.md` — one per structural conjunct (the
eleven above, each weakened separately), the join's shift detection (delete one refused record before the join; the
count check must fire), and the K-pair gate. The inventory's G1-G3 (`memory/instruments/returned-document.md`) are
refreshed against this PIN at S01's grill and gain rows for the conjuncts and the 661/687 fixtures. Proofs that need
deleted code were taken in S01/S02. Refs: D7.
Acceptance: every conjunct and G1–G3 have a proof that goes red for its own assertion; the `/Kids` proof fails against
the pre-S01 walk (taken before S02).

### P02 — The route *(done 2026-10-02, v1.179.6)*
**Goal.** The bytes of the version you signed, over the wire, or a refusal that says which of four (D7 as amended 2026-09-29)
things went wrong.

**Exit criteria.** `GET /api/document/revision` returns a prefix that parses and re-verifies for the
requested signer, or one of four named refusals; nothing calls it on document open. **(amended 2026-10-02, plan-review W6)**: where a later
revision redefined the user's signature dictionary, the user's signed version is returned byte-identical. (The refusal set
is four D7 causes plus `could-not-check`, pending Dan — C2.)

**Phase close 2026-10-02 (v1.179.6) — acceptance ledger, every clause split on `and`, checked at the close's tree:**
- ✅ `GET /api/document/revision` returns a prefix that parses — `TestTheRouteReturnsTheSignedVersionOrNamesWhyNot`: four
  200 rows (returned untouched, a stranger co-signed to EOF, the signer's dictionary replaced later, upper-case
  fingerprint), each byte-identical to the signed version, `application/pdf`, `no-store`.
- ✅ …and re-verifies for the requested signer — `holder` returns only a record `Revisions(prefix)` VERIFIED, well-formed,
  not a timestamp, with this fingerprint, at exactly the cut (`signedrevision.go`); `TestTheSignedVersionAcrossDocumentShapes`
  (the spoof cannot be returned for the user; a forged SignerInfo reads `resaved`, never a version).
- ✅ …or one of four named refusals — **as built, five** (C2's `could-not-check` is parked for Dan, below): the route test's
  five 422 rows, one per cause; `signedrevision.test.mjs` tells the five apart, reading the list from Go's declarations.
- ✅ Nothing calls it on document open — jsdom boot over a CHANGED signed open (`addedAfter`) sends no request to the
  route; `fetchSignedRevision` has no caller (census, allow-list empty until P03); Go census
  `TestTheSignedVersionIsWalkedOnlyOnDemand` (one site, in `handleDocumentRevision`). All three red-proved at the close.
- ✅ (W6) Where a later revision redefined the user's signature dictionary, the user's signed version is returned
  byte-identical — the route row "the signer's dictionary replaced later" (`earlierRevision`, `redefinedObj` = the
  object), and the shape rows for the P01 attack, with `/SigFlags` dropped, with copies claiming later ends.
- ⏸ (parenthetical) "The refusal set is four D7 causes plus `could-not-check`, pending Dan — C2": **parked**, built under
  that name; in the closing batch.

Review: `code-reviews/v1.179.5-p02-phase-close-2026-10-02.md` — 6 critical, all fixed in this close (two in-phase:
a forged bare SignerInfo crowding out the version; the walk's uncharged scans), plus two re-review rounds. Out-of-phase
findings filed as /pending 800 and 802–808 (801 fixed here); 578, 600, 631, 652 amended. Graduation pass:
`instruments/returned-document.md` (63 rows: keep-live 47, deleted 3, declared gaps 13). Required-run gates: tiers 0–3;
tiers 4 and 6 do not fire by `slicegate_test.go`'s list (no session/ceremony/delivery/discovery/p2p/rendezvous file),
and tier 6 was run anyway because `HasSignatureBlob`, on the arrival gate, changed. Results are recorded in the close commit.

**(plan-review pin: the user's signature can be absent from the latest xref, architect — 2026-09-28)** Both sweeps
see only the newest definition of each object number, so a later revision that reuses the object number of the user's
signature dictionary hides it from `Revisions` while its signed prefix is still in the file — and D8 forbids reporting
that as "not inside this file". `SignedRevisionFor` therefore re-runs `Revisions` on each earlier revision's prefix
(each `startxref`/`%%EOF` boundary) when the latest has no record for the fingerprint, reusing the one door; a fixture
has a later revision redefine the user's dictionary's number. Remedy unverified until P02.S01.
**(plan-review pin: the causes the route can return, consistency + architect — 2026-09-28)** The record causes are
internal; each maps to exactly one D7 cause at this route. D7's `kids-hidden` is unreachable under the xref sweep — **ANSWERED
2026-09-29 by Dan: retired (option A)**; P02's acceptance now counts four causes. A hybrid-reference file must not be told "unsigned" by P02/P03 while
/pending 733 is open. The record type stays unexported; P02 exports a narrow projection (signers in coverage order, each
with fingerprint, coverage end, verified).

**PIN 2026-10-02 (phase-open, firmed against HEAD ad036fb7 / v1.179.2 — read at the lines, not from the plan).**
- **The record type is already exported** (`sign.Revision`, `revisions.go:57`; `sign.Revisions`, `:714`) — the 2026-09-28 pin's
  "stays unexported" was written before P01 built it. What P02 adds is selection and recovery, not a new projection type:
  `Revision` already carries `Fingerprint`, `CoverageEnd`, `Verified`, `Cause` and `Timestamp`, which is the projection the
  pin asked for. P03 reads it; P02 does not invent a second shape of it.
- **/pending 733 is CLOSED** (v1.169.5, `d714ab58`): a hybrid-reference file is no longer told "unsigned" by the blob check.
  The pin's "while /pending 733 is open" condition is discharged; 740/741 (its residue) are not on this path.
- **P01's phase close changed the redefinition case** (`cda73c99`, conjunct (11) now requires the owning header at the
  xref's offset): a later revision that redefines the user's object number is now REFUSED `contents-elsewhere` in the latest
  sweep, not silently read as the user. So the earlier-revision recovery in the first pin is no longer an edge case: it is
  how the user's own signed version is found when a later party rewrote their dictionary. It gets its own slice (S02).
- **There is no revision-boundary helper and no standalone prefix verifier** (grep of non-test `internal/sign` for
  `startxref`/`%%EOF`: no hits; only `Verify(data)` over a whole buffer). Both are built here, the second by calling
  `Revisions` on the prefix — the one door — never a second verifier.
- **No existing route serves document-derived bytes through `sendDownload`**: `/api/pdf` (`handlePDF`, `server.go:1023`)
  writes raw bytes with `Cache-Control: no-store` and is fetched by pdf.js, not `apiFetch` (ADR-004's named exception). The
  new route is fetched by `apiFetch` (so `X-Nib-Doc` pins it) and answers bytes inline for the compare pipeline
  (`getDocument({data: buf})`, `app.js:4417`), so it follows `handlePDF`'s headers, not `sendDownload`'s attachment
  disposition — the route slice's "sendDownload's discipline" is read as *its filename quoting where a name is sent*, and no name is.
- **Firmed: three slices**, S02 split out of S01 for the reason above, the route renumbered S03.

**PIN 2026-10-02 (plan-review of the firmed P02, hand-off `plan-reviews/2026-10-02-p02-returned-document.md` — 3 critical,
10 warning, 9 info; citations spot-checked at the lines by the arc).** These pins govern the slices below. Where a pin and a
slice's text disagree, the pin wins.
- **(C1) A named identity selects, never asserts.** `named` (unverified, `revisions.go:250`) may choose which prefix to
  re-verify; D4's re-verification is the proof. A refusal built from it says *"a signature naming your certificate does not
  verify against this file"*, never "your signature", and carries `attributed: false` unless the SignerInfo's own signature
  over its signed attributes checks against the named certificate's key (remedy unrun — S01's grill reads whether
  `digitorus/pkcs7` exposes those bytes).
- **(C2) An error on the whole file is not a refusal.** `Revisions(full)` errors with no records in five places
  (`verify.go:275`, `:291`, `:302`, `:351`, `:422`); measured, one appended negative-length copy of the user's `/Contents`
  does it while `full[:end]` is the user's original and re-verifies. Candidates then come from `sweepRevisions(full)`: the
  last-pair end of every record whose `named` is the fingerprint, refused ones included. Each is accepted only through
  S01's re-verification. A whole-file error with no verifying candidate returns **`could-not-check`**, never
  `no-signature` or `not-your-signature` — **parked for Dan as an amendment to D7 (a fifth cause); built under that name
  until he answers.**
- **(C3) The cause is a precedence over the record set, not a map over record causes.** In order: (1) a verified,
  well-formed record for the fingerprint → its prefix; (2) else C2's candidates and S02's walk → the first that re-verifies;
  (3) else a record naming the fingerprint → `resaved`, worded per C1 and W10; (4) else any signature-shaped record →
  `not-your-signature`, with every refused record attached (`RefusedSignature`, `verify.go:167-196`); (5) else
  `no-signature`. **S01's acceptance table iterates over DOCUMENT SHAPES** (the probe's six rows plus the spoof), not over
  cause constants — the "each record cause maps to exactly one D7 cause" clause is struck as unsatisfiable; the
  redefinition row must not read `resaved`, the corrupted-blob row must not read "you did not sign this".
- **(W1) S02's boundaries end after the EOL.** A nib signature covers `%%EOF\n` (`pdfsign/sign/pdftrailer.go:64`), so a cut
  at the end of `%%EOF` misses it by one byte (measured). Candidate ends: (a) the last-pair ends of sweep records naming the
  fingerprint, raw `ByteRange` included; then (b) each `%%EOF` plus an optional `\r`, `\n` or `\r\n`, kept only where
  `startxref`'s offset lands on `xref` or `N G obj`. Deduped, newest first. A prefix that errors is SKIPPED, never mapped.
- **(W2) Screen before verifying.** Each S02 candidate is screened with `sweepRevisions` and only a prefix holding a record
  naming the fingerprint pays `Revisions`; capped at 16 candidates and 16×len bytes, measured on a hostile fixture.
- **(W3) "Last" is the largest `CoverageEnd`** (records come in object-number order, `read.go:452`). A later record naming
  the user that fails is reported beside the prefix returned; the user's earlier signatures are listed too.
- **(W4) The route is `requireUnlocked`**, as `/api/pdf` is (`server.go:452`) — not `requireSession`.
- **(W5) A refusal is `422` with JSON `{cause, refused[]}`** — 409 triggers `apiFetch`'s reconcile (`app.js:429`). Facts
  about returned bytes ride in an `X-Nib-Revision` header (object, end, found-in-earlier-revision, later-failed).
- **(W6) Exit criteria gain:** where a later revision redefined the user's signature dictionary, the user's signed version
  is returned byte-identical. **(W7)** S02's acceptance asserts that recovery and that the cause is neither `resaved` nor
  `not-your-signature`.
- **(W8)** The response names the object number a later revision redefined, so P03 can say so.
- **(W9)** The response says whether the working copy has history (undo non-empty, or dropped per ADR-003), because the route
  reads `doc.data`, not the file as it arrived; P03 words it. Which nib operations rewrite a signed document: unverified.
- **(W10)** `resaved` stays the internal name; no surface may say "re-saved" — it alleges an act nib did not observe.
- **Info folded:** I2 (the check that can fail is the record's, not the length), I3 (an unrelated record erroring in a prefix
  gives `prefix-failed-reverify`, worded so), I4 (D10's "no boot or open path" gets a static guard), I5 (single-flight per
  document id), I6 (reuse `RefusedSignature`), I7 (the 2026-09-28 "unverified until P02.S01" is S02's now), I8 (a
  timestamp-only document is `no-signature`, never worded "nothing signed this"), I9 (counterparts — P03's wording).

#### P02.S01 — `sign.SignedRevisionFor(pdf, fingerprint)` *(done 2026-10-02, v1.179.3)*
Scope: select the requested signer's record by fingerprint among the latest sweep's well-formed, verified, non-timestamp
records (the user's LAST signature where they signed more than once — the one "since I signed" means); slice
`pdf[:CoverageEnd]` (bounds already proven by conjunct (6)); then require the prefix to pass `Revisions` with no error AND
hold a record for that fingerprint that is verified, well-formed, and whose `CoverageEnd` is the prefix's length — the
signature covers the whole prefix less its own `/Contents`. Return the prefix or one of D7's four causes as a typed error,
with every P01 record cause mapped to exactly one of them. Refs: D1, D4, D7, D8.
Acceptance: the reproduced spoof (a stranger co-signing to EOF) does not return that stranger's prefix for the user's
fingerprint, and returns the user's own; a prefix that fails standalone re-verification is refused
`prefix-failed-reverify`; an unsigned document is `no-signature`; a signed document the user never signed is
`not-your-signature`; a document re-saved wholesale (the user's signature present and failing) is `resaved`; each P01
record cause reaches exactly one D7 cause, asserted by a table over the cause constants.
**(grill 2026-10-02, `grills/2026-10-02-p02s01-signedrevisionfor.md` — verdict AMENDED)** The last clause above is struck
(plan-review C3). Three corrections to the pins, each measured in a goprobe prototype over 18 document shapes: **(a)** C2's
candidates come from the records `verifyIndexed` returns BESIDE its error (it does at the overread, join and unseen sites),
never from a second `sweepRevisions(full)`, which would run the digitorus reader on a file the pdfcpu gate refused
(ADR-041); a pdfcpu or sweep failure is `could-not-check`. **(b)** The P01 redefinition attack keeps the user's ByteRange,
and its refused record still verifies, so **S01 recovers it byte-identical**; S02 keeps the two redefinitions S01 gets wrong
(ByteRange changed → reads `resaved`; replaced by a non-signature → reads `no-signature`). **(c)** C1's `attributed` IS
buildable — `pkcs7`'s SignerInfo exposes its signed attributes and `EncryptedDigest`; measured true on the original and on a
pdfcpu resave, false on a forged SignerInfo. A timestamp authority's fingerprint is never a candidate nor `resaved`.
Tasks:
- T01 — `RevisionCause` (five constants) and the `SignedRevision` result value (prefix, cause, obj, end, `RedefinedObj`,
  `Later`, `Earlier`, `Refused`, `Attributed`).
- T02 — `SignedRevisionFor` runs `verifyIndexed` once and takes candidates from what it returns.
- T03 — `revisionCandidates`: the seam S02 extends — records naming the fingerprint, not timestamps, verified unless the
  whole file errored; last-pair ends bounds-checked, deduped, newest first, capped at 16.
- T04 — the prefix predicate: `Revisions(prefix)` with no error, holding a verified signer record for the fingerprint whose
  `CoverageEnd` is the prefix's length.
- T05 — the precedence: a candidate that holds, else `could-not-check`, `prefix-failed-reverify`, `resaved`,
  `not-your-signature`, `no-signature`; every refusal carries the refused records.
- T06 — `attributed`, computed only on the `resaved` branch.
- T07 — the W3/W8 facts (`Later`, `Earlier`, `RedefinedObj`).
- T08 — the acceptance table over document shapes; S02's two rows marked for S02 to flip.
- T09 — the cost of 16 hostile candidates at ≥10 MB, measured.
- T10 — guard: `sweepRevisions` has exactly one non-test caller.
- T11 — `/redproof` over every predicate conjunct, candidate filter, bound and the cap.
**Built 2026-10-02 (v1.179.3) — where the code went beyond or against the tasks, each measured:**
- **T09 found a defect, not a cost.** 40 copies of the signer's own blob claiming later ends filled all 16 verify slots
  ahead of the genuine version: `could-not-check` for a version in the file, 13 s at 40 MB. Added a SCREEN before any
  re-verify — the SignerInfo must check against the named key and its ranges must hash to its signed `messageDigest` —
  with a hashing budget of 16×len charged before hashing, and well-formed proposers tried first. Now 1.4 s and the
  version. Declared residual: enough WELL-FORMED copies covering the whole file spend the budget → `could-not-check`.
- **T10 re-scoped.** `sweepRevisions` has three non-test callers (`verifyIndexed`, `signedAsIntended` ×2), not one; the
  guard asserts what the grill meant — every caller runs pdfcpu's read before its first sweep (ADR-041).
- **Grill default 5 reversed** (P02.S01 review C1, measured +804 MB on a 4 MB file): `Revision` keeps a small COPIED
  `signerProof`, never the parsed PKCS#7.
- **Unchecked ≠ failed** (review C2): a record the library never enumerated (a later revision dropped `/SigFlags`) is
  screened, not dropped as failed — it had read a false `resaved`.
- **The signature check mirrors the library** (`pkcs7` `getSignatureAlgorithm`): SHA-1, Ed25519, curve OIDs, RSA OIDs
  taking their hash from the digest algorithm, and SignerInfos with no signed attributes (checked over the ranges' bytes).
  ByteRanges must be ascending and non-overlapping for the screen (783 ranges on disk: the 8 that are not are this
  repo's own adversarial fixtures).
- Review: `code-reviews/v1.179.3-p02s01-2026-10-02.md` (2 critical, 2 warning, 5 info; re-review 1 warning; all
  dispositioned). Red-proof: targeted + blind pass; final 50+ mutations red, survivors declared equivalent in the
  inventory. Live: every PDF on disk (361; 5 library-verified signers) — each signer's version returned byte-identical.
**Acceptance ledger:** the spoof does not return the stranger's prefix for the user ✅ and returns the user's own ✅; a
prefix failing standalone re-verification is `prefix-failed-reverify` ✅ (I3 row); unsigned is `no-signature` ✅; a
document the user never signed is `not-your-signature` ✅; a document re-saved wholesale is `resaved` ✅ (worded per C1,
`Attributed`); the per-record-cause table ~~✅~~ STRUCK (C3) — replaced by the 25-shape table ✅. Pins owned here: C1 ✅,
C2 ✅ (the negative-length copy returns the original), C3 ✅, W3 ✅ (`Earlier`, `Later`), W8 ✅ (`RedefinedObj`). W1/W2/W6/W7
are S02's; W4/W5/W9 are S03's.

#### P02.S02 — the user's signed version when a later revision rewrote their dictionary *(done 2026-10-02, v1.179.4)*
Scope: when the latest sweep has no verified, well-formed record for the fingerprint, walk the file's earlier revision
boundaries (each `%%EOF` that ends a cross-reference section, newest first, bounded) and run S01's selection on each prefix
through the same door; the first that yields the user's verified record is the answer, and the route says it was found in
an earlier revision. Refs: D1, D4, D8; the 2026-09-28 architect pin.
Acceptance: a fixture where a later revision redefines the user's signature object (the P01 phase-close attack) recovers
the user's original signed version byte-identical, not `not-your-signature`; a fixture with no such revision costs one
sweep; the boundary walk is bounded in count and in bytes on a hostile file with many `%%EOF` markers (measured).
**(grill 2026-10-02, `grills/2026-10-02-p02s02-boundary-walk.md` — verdict AMENDED, each change measured)** W1 as written
found no nib boundary: pdfsign's `startxref` points one byte early, so the header is reached past white space; a header must
be `xref` or an object whose dictionary says `/XRef`; markers naming one offset collapse to the earliest (5,000 appended
markers otherwise queue ahead of the version); and only a boundary a literal ByteRange ends at is worth a look. W2's
"screen with `sweepRevisions`" would be the first ungated sweep — the screen is pdfcpu's read then the sweep, in one
function. The walk also runs when the whole file errored: two pdfcpu-refused shapes recover byte-identical. "Costs one
sweep" is re-worded to the measured truth: no screen and no verify unless a ByteRange ends at an earlier boundary.
Tasks: T01 `revisionBoundaries`; T02 `rawByteRangeEnds` (the intersection); T03 `boundaryCandidate` (gated); T04 the walk
after S01's candidates, both paths, sharing the verify cap and budget, 16 screens; T05 `EarlierRevision`, `RedefinedObj`
only where the whole file was read, a cut walk → `could-not-check`; T06 flip S01's two rows; T07 new shapes (two
pdfcpu-refused files, 5,000 same-xref markers, more fakes than the cap, signed twice and both replaced); T08 unit tests of
both scans; T09 screen counts; T10 the live pass; T11 `/redproof`.
**Built (v1.179.4):** as tasked. `nameReaches` (a W8 refinement) was removed as dead — a latest record still reaching
the found end would have been an S01 candidate. Measured on the built code: a 12 MB walk recovery 72 ms (one `Verify`
47 ms); 20 distinct fake xref sections 1.2 s → exactly 16 screens, cut, `could-not-check`; 5,000 same-xref markers → one
screen and the version; an untouched document → zero screens. Live: 361 PDFs, every verified signer still returned
directly (no walk).
**Review (`code-reviews/v1.179.4-p02s02-2026-10-02.md`; 3 critical, 1 warning, then a re-review critical), all fixed and
red-proved:** the boundary scan is linear (a bounded white-space look — 33.8 s → < 2 s on 2,000 markers over 4 MiB); the
walk also runs PAST a version S01 found, for the signer's LAST one (W3), and a version found while that search was cut
carries `LaterUnchecked` (the certificate is public, so anyone can spend the walk's screens ahead of the last version);
only ends actually re-verified count as tried; every bounded exit reads `could-not-check`. The "zero cost on an honest
document" claim was false: signer 1 of N pays N-2 screens, ~3.5 ms each at 50 KB, ≤ 16 — results unchanged.
Declared: a non-signer asking about an honest document with 17+ signed revisions reads `could-not-check`; the ByteRange
regex misses `#`-escaped names and comments inside the array, and the boundary scan misses a comment before `%%EOF` or
> 64 bytes after `startxref` — each makes the walk miss a version, which then reads a refusal; no file on disk shows one.
**Acceptance ledger:** the P01 phase-close attack (a later revision redefines the signer's dictionary) recovers the
original byte-identical ✅ (S01 already did; the ByteRange-changed and non-signature redefinitions now do too ✅, cause
neither `resaved` nor `not-your-signature` ✅ — W7); a document with no such revision costs **no screen and no verify**
when no ByteRange ends at an earlier boundary ✅ (re-worded from "one sweep"; zero screens on an untouched document,
asserted) — and N-2 screens for signer 1 of N, measured; the walk is bounded in count (16 screens, the shared 16
verifies) ✅ and in bytes (the shared 16×len budget, the linear scan) ✅ on hostile files with many `%%EOF` markers,
measured (5,000 same-xref markers → one screen and the version; 20 distinct fakes at 12 MB → 1.2 s, 16 screens,
`could-not-check`) ✅. W6's exit-criterion amendment ✅. W8 ✅ (`RedefinedObj` where the whole file was read).

#### P02.S03 — the route *(done 2026-10-02, v1.179.5)*
Scope: `GET /api/document/revision?signer=<fp>` through the existing mux block and `docFor`, behind `requireSession`
(ADR-054), answering the prefix as `application/pdf` with `Cache-Control: no-store` (`handlePDF`'s headers) or a JSON
refusal naming its cause, reached from the client only via `apiFetch` (so `X-Nib-Doc` is carried per ADR-004). Refs: D7,
D10.
Acceptance: each of the four causes is reachable and distinguishable at the client; the route is called from no boot or
open path; `TestEveryRouteIsBehindTheSessionOrNamed` covers it without a new exemption.
**(grill 2026-10-02, `grills/2026-10-02-p02s03-route.md` — deepdive + grill, verdict AMENDED)** The route keeps W4/W5's
shape (`requireUnlocked` wraps `requireSession`, so the census holds with no exemption), but **a client caller lands in
this slice**: the request-field census refuses a server-read `signer` no `apiFetch` sends, and its product-gap
exemption is full. **W9 cracked**: a barrier clears undo without marking it evicted and a save replaces the bytes with no
history, so the fact is RECORDED history and "none" never means "as it arrived". The route projects every field (a
marshal names none, so the census would still see eleven unread), and its single-flight key carries the bytes' identity.
Tasks: T01 handler (400 on a malformed fingerprint, lowercased first; bytes and history in one hold); T02 single-flight;
T03 200 + `X-Nib-Revision`; T04 422 `{cause, refused, attributed}`; T05 `history`; T06 `fetchSignedRevision` (pinned,
no caller); T07 nothing calls it on boot or open; T08 the route's Go test, every cause; T09 the client tells them apart;
T10 the census rows; T11 cost; T12 ADR-072; T13 `/redproof`.
**Built (v1.179.5):** as tasked, plus one thing T11 found. **At 100 MB the boundary walk cost 2 min 6 s** on 20 fake xref
sections — pdfcpu fell into a slow full read of every fake prefix. The walk now PRESCREENS each boundary from its own
bytes before any pdfcpu read (`prescreen`, in `internal/sign`): the ByteRange literal's object must hold a literal
`/Contents` naming the signer, checked against the named key, over ranges hashing to the signed digest — which a genuine
holder always satisfies (conjuncts 9 and 10). 2 min 6 s → 53 ms; honest 100 MB recovery 0.54 s (one Verify 0.22 s).
With rejection cheap, the bounds split: 256 prescreens, 16 pdfcpu reads — crowding a version out now takes hundreds of
fake sections, not seventeen. `internal/testpdf` gains `AppendRevision` and `SignatureDictionary` (test-support) for the
server's fixtures. ADR-072.
**Review (`code-reviews/v1.179.5-p02s03-2026-10-02.md`; 2 critical, 2 warning, 4 info), all fixed or declared and
red-proved:** the prescreen charges every byte it scans to the budget (64,000 literals in one object had cost 3 min
uncharged) and examines each object once, with no 1 MiB window; blobs are parsed untrimmed (trimming the padding cut a
genuine signature ending in 0x00, one in 256); the single-flight key gains the length. Declared: a walk outlives its
client (every bound is linear in the file); HEAD pays the walk. **Live (real binary, headless, `/api/open` then the
route over HTTP):** the signer's version byte-identical with its facts on the untouched, spoofed and redefined files
(`earlierRevision`, `redefinedObj` on the last); 422 `not-your-signature` / `no-signature` as the file stands; 403
with no session. Found live and worded, not changed: a stranger asking about the redefined file reads `no-signature`
— true of the file as it stands, while an earlier revision holds someone else's signature (P03 words it).
**Acceptance ledger:** each cause reachable and distinguishable at the client ✅ — **five**, not four (C2's parked
`could-not-check`): the route's Go test returns each as a distinct 422 through the real mux, and the jsdom test runs the
real `fetchSignedRevision` over each; the route is called from no boot or open path ✅ (jsdom spy: zero requests through
boot and a signed open; one literal in app.js, inside the helper); `TestEveryRouteIsBehindTheSessionOrNamed` covers it
without a new exemption ✅ (`requireUnlocked` wraps `requireSession`; live 403). Pins: W4 ✅, W5 ✅ (422 + JSON, the
header), W9 ✅ (recorded history, the gap declared), I4 ✅, I5 ✅ (single-flight, proved by count and by an edit
mid-walk), I6 ✅.

### P03 — The surface
**Goal.** The page the request asked for.

**Exit criteria.** One command opens one sheet showing the document, the verdict in dispute
language, every signature with its position relative to yours and whether this machine knows the
signer, and the changes; the three terminal states each render their own sentence; the fallback
chain is offered in order.

**PIN 2026-10-02 (phase-open, firmed against HEAD `8766fdf4` / v1.179.6 — read at the lines, not from the plan).**
- **The fallback chain's first link is P04's, by the plan's own inventory.** No kept copy exists to offer: `~/nib/signed/`
  is the ceremony DELIVERY folder (`internal/server/delivery.go:371`), not retention, and the inventory already files the
  link under retention (`instruments/returned-document.md` S5, "retained copy → the dispute surface's fallback chain").
  So P03.S03 builds the chain from the two links that exist — the ceremony mirror, then pick your own file — in that
  order, with the kept copy's slot named and empty; **P04.S02 puts the kept copy at its head**, and P04's acceptance
  gains that clause. The exit criterion's "offered in order" is checked on P03's two links at P03's close and on all
  three at P04's.
- **No route hands the mirrored document to the client.** Every `ceremony.ReadMirror` caller
  (`ceremonynext.go:246`, `delivery.go:1395`, `ceremonystop.go:73`, …) uses the bytes server-side. The mirror link
  needs a read route, pinned (ADR-004), and a way to find the ceremony a document belongs to — S03's grill.
- **"Known to this machine" is server-side only, and only as a count.** `unverifiedSigners` (`server.go:1784`) counts
  signers whose fingerprint is neither a pinned peer nor this machine's identity; no per-signer flag reaches the client,
  and `renderConsentSigners` (`app.js:1917`) shows fingerprints with no known/unknown mark. D9 needs it per signer:
  S02 adds a per-signer `known` beside the count, from the same one `PinnedPeers()` copy (one rule, ADR-009).
- **"You" is `selfFingerprint`** (`app.js:1084`, from `/api/peers`), which exists only with an unlocked vault and an
  identity; an external signer's certificate (`/api/identity/external`) is a second "you". S02 decides how the surface
  picks among them and what it says with neither (the route itself refuses a missing fingerprint, 400).
- **The pieces S01 and S03 reuse exist:** the `secure` tab's Sign & Timestamp card (`index.html:488`), the one
  `data-forward` door (`app.js:11547`; precedent `index.html:538`, a Send & Receive twin), the `#ceremonySheet` pattern
  (`showCeremonySheet`/`parkCeremonySheet`/`resumeCeremonySheet`, `app.js:15672/15783/15906`), Compare's bytes-in
  path (`openCompare`, `getDocument({data: buf})` at `app.js:4433`), and the id gates (`ids`, `modes`, `doccontrols`).
- **/pending 652 is S01's first task**: `apiFetch` drops `X-Nib-Doc` for a falsy `docId`, which on this route answers
  the ACTIVE document's signed version — S01 is the first caller of `fetchSignedRevision`.
- **Firmed: three slices, as sketched**, with the amendments above folded into their acceptance. `/plan-review` does
  not fire: P03 is a UI surface — no security, migration or egress change; the mirror read route is a same-user,
  session-gated read of the user's own files, which S03's grill attacks.

#### P03.S01 — the command and the sheet *(done 2026-10-02, v1.179.7)*
Scope: `"Check a document that came back…"` in the `secure` tab's *Sign & Timestamp* card, with a
`data-forward` alias into *Send & Receive*; the sheet itself. Refs: D12.
Acceptance: reachable in both the sidebar and the collapsed toolbar without a second declaration;
the id gates (`ids`, `modes`, `doccontrols`) pass; keyboard-only operation and focus restore hold.

Grill (2026-10-02, light — a UI slice over a pinning seam; deepdive of `apiFetch`'s 36 `docId` sites: 35 guarded, the
convene's deliberate `null` the one falsy path). Conclusions: the sheet is ABOUT one document, captured at open
(ADR-001) — it closes when the active tab changes rather than describe another document; it never shares the screen
with the ceremony sheet (opening either hides the other); S01 does NOT fetch — the verdict is S02's, and a sheet that
showed a raw cause would be D8's degraded surface; the D10 caller census stays empty until S02 names its caller.
- T01 — /pending 652: `apiFetch` throws on a present-but-falsy `docId` (a toastable sentence); `conveneFromPanel` refuses
  before the call with no bound document (what `renderCeremonySetupDoc` already promises); `pinning.test.mjs`'s source
  pin updated; a test that a falsy id throws; the convene tests load a document.
- T02 — the command `#returnedBtn` in *Sign & Timestamp* and its `data-forward` twin in *Send & Receive*; in
  `DOC_REQUIRED`; not an editing tool (it only reads).
- T03 — `#returnedSheet` on the `#ceremonySheet` pattern: shown in place of the viewer; heading, the document's name,
  an empty verdict region for S02, one way out ("Back to the document"); Escape leaves; focus to the heading on open and
  back to the OPENER (either twin) on close; closes on an active-view change; hides an open ceremony sheet and is hidden
  by one.
- T04 — jsdom: both entry points open it, the forward resolves through the one door, focus in and back, Escape, a tab
  switch closes it, the ceremony exclusion, disabled with no document; the id gates.

**Built (v1.179.7):** T01–T04 as tasked, plus what they found. The throw exposed seven jsdom tests (five files) whose
stubs installed a document with no `id` — the server never does (`docResponse` always sets one) — and passed with an
`undefined` pin; their stubs now carry ids, and one setup-sheet test opens a document for the convene it is about.
**Added beyond the Ts:** a tier-3 test (`test/ui/returnedsheet.test.mjs`) — where focus LANDS after the sheet goes is
invisible to jsdom — and, from the review (`code-reviews/v1.179.6-p03s01-…`, 0 critical, 3 warning, 7 info, all
dispositioned): the sheet closes on any mode change and on any tool arming (`disarmEditingTools`, the one door), is a
`role="region"`, and Escape works wherever focus is unless a modal is up. **Live:** the real binary in a real browser,
keyboard only — Enter on the command and on its twin opens the sheet with focus on its heading, Escape and "Back to the
document" return focus to whichever opened it, and the reader's scroll survives (tier 3, 2/2, red without the restore).
**Acceptance ledger:** reachable in the sidebar card and through the Send & Receive twin without a second declaration ✅
(one `data-forward`, resolved by the one door); the id gates pass ✅ (`ids` 2/2, `modes` 2/2, `doccontrols` 5/5);
keyboard-only operation holds ✅ (tier 3, Enter/Tab/Escape); focus restore holds ✅ (tier 3, both openers, and jsdom).
/pending 652 closed (T01).

#### P03.S02 — the verdict, told for a dispute
Scope: the three terminal states, and the signer list ordered around yours. Refs: D8, D9.
Acceptance: a document co-signed by a stranger after your signature never reads as unqualified
`✓ Untampered` on this surface; a re-saved file says the signed version is not inside it; an
unsigned file says so without implying you never signed.

#### P03.S03 — "see what changed"
Scope: hand the recovered prefix to the shipped compare pipeline as bytes. Refs: D11.
Acceptance: the diff renders from server-supplied bytes with no change to the differ; the fallback
chain — kept copy → ceremony mirror → pick your own file — is offered in that order. **(phase-open PIN 2026-10-02)**: the
kept copy's link is P04.S02's; P03 offers mirror → own file in that order, the kept copy's slot named and empty.

### P04 — Retention at signing *(Dan's option C)*
**Goal.** Make the dispute answerable in the case the file itself cannot answer.

**No dependency on P01–P03** — it can move earlier if the re-saved case turns out to be the common
one in practice.

**Exit criteria.** A tick at signing, default off, writes the signed output to `~/nib/signed/`; a
failed write fails the signing; the copy is listed and deletable; the Simple Sign checklist gains a
probed step; an ADR records the retention decision.

#### P04.S01 — the tick and the write
Scope: the checkbox, the write, the naming. Refs: D13, D14.
Acceptance: unticked signs and writes nothing; ticked writes exactly one file; a write failure is
surfaced at signing and the signature is not silently returned as if the copy existed.

#### P04.S02 — the copy is visible and removable
**(P03 phase-open PIN 2026-10-02)**: and the dispute surface offers the kept copy FIRST in its fallback chain, ahead of
the ceremony mirror (inventory S5).
Scope: listing and deletion. Refs: D15.
Acceptance: a kept copy appears in a surface the user can reach and can be deleted from it.

#### P04.S03 — the checklist step, and the ADR
Scope: the Simple Sign row with a real probe; the ADR. Refs: D16, D13.
Acceptance: the row ticks from a filesystem probe and shows `—` for documents signed before this
shipped; the ADR is in the same commit.

---

## Out of scope

- **A Go-side differ, or any text extraction in Go.** D11; measured and declined at `/pending 46`.
- **Upgrading a pending OpenTimestamps proof.** D17; that is `/pending 388`.
- **Trust-chain or CA validation.** `internal/sign/verify.go:6-9` drops `TrustedIssuer`
  deliberately — nib cares about integrity, not third-party trust. "Known to this machine" (D9) is
  the pinned-peer list, and is not a claim about identity.
- **Changing `ContentDigest` or `DocHash`.** ADR-013 refuses it and the reasons are unchanged here.
- **Making the badge itself say something different.** ADR-013 binds its per-signature language as
  accurate about what it says; this plan adds a surface that asks a different question rather than
  re-wording one that answers its own correctly.

## Standing caveats

- *(superseded by P01's PIN 2026-09-28: the badge now withholds "Untampered" for an unknown signer)* **The spoof is live in shipped code today.** A document modified and co-signed by a stranger
  reads `✓ Untampered · 2 signers` in the toolbar right now, before any of this is built. This plan
  fixes the *new* surface; whether the existing badge should also change is a separate call and is
  not decided here.
- *(settled by P01's PIN 2026-09-28: digitorus never reads `/XRefStm`; /pending 733)* **Hybrid-reference files (`/XRefStm`) were reported as a gap by the format pass and not
  re-verified at the line.** If `digitorus/pdf` genuinely ignores `/XRefStm`, a signature dict in
  such a file may be unfindable and the refusal must be `no-signature` rather than a wrong prefix.
  Settle it with a fixture before P01.S01 closes.
- *(answered by P01's PIN 2026-09-28: no recursion — an xref sweep; the deepdive ran)* **`/Kids` recursion changes a shipped, red-proved, fail-closed guard.** The one flagged unmeasured
  surface: a `/deepdive` on `internal/sign` would settle what else reads `AddedAfter` and whether
  its fail-closed arm can go quiet once the walk stops disagreeing with the library. Dan's to call.
- **Nothing above 1.4 MB was measured.** The cost figures are a range, not a curve.
- **A copy you kept is your own artifact.** It proves what you had, not what you sent. D17 names
  what would strengthen it and why it is not here.

## Seam inventory

`~/.claude/projects/-home-dan-repos-nib/memory/instruments/returned-document.md` — 8 paths,
5 seams, 7 gap-downs, written against this plan before any code. All class 1. **No row on an
enumerated hot path**; one row (P1) flagged HOT-ADJACENT because the naive placement would put a
`dpdf` parse on the document-open path, which D10 forbids and that row polices. No
`diagnostic, no standing reader` rows.
