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

**Status: unbuilt.** No slice has started. **(2026-09-28, P01 phase-open)** `/deepdive` and `/plan-review` of the
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

### D7 — Five named refusal causes, not one error *(settled 2026-09-08 via /grill)*
`no-signature` / `kids-hidden` / `resaved` / `not-your-signature` / `prefix-failed-reverify`. A
lumped refusal reads backwards exactly when it matters, and each cause is a different sentence to
the user. Attribution beats aggregation.

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

### P01 — The revision index
**Goal.** One walk over a document's signatures that knows, per signature, *who* signed and *how
far their signature reaches*.

**Exit criteria.** Exactly one ByteRange walk exists in the tree and a guard asserts it; a
signature nested under `/Kids` is seen; a multi-gap ByteRange resolves to its last pair; an
out-of-range ByteRange is refused rather than sliced; every existing `trailing_test.go` test is
green and `AddedAfter` keeps its fail-closed arm. **(amended 2026-09-28, phase-open, /pending 661 and 687 folded
per Dan's /discuss on /pending 389)**: `AddedAfter` measures only verified, well-formed signatures; a signature whose
ByteRange covers anything but the whole revision less its own `/Contents` is refused and is not a valid signer.

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

#### P01.S02 — one door: `AddedAfter` over verified, well-formed revisions
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

#### P01.S03 — a refused signature is reported, and is not a signer
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

#### P01.S04 — the red proofs
Scope: a proof per new refusal and per gap-down, registered in `docs/red-proofs.md` — one per structural conjunct (the
eleven above, each weakened separately), the join's shift detection (delete one refused record before the join; the
count check must fire), and the K-pair gate. The inventory's G1-G3 (`memory/instruments/returned-document.md`) are
refreshed against this PIN at S01's grill and gain rows for the conjuncts and the 661/687 fixtures. Proofs that need
deleted code were taken in S01/S02. Refs: D7.
Acceptance: every conjunct and G1–G3 have a proof that goes red for its own assertion; the `/Kids` proof fails against
the pre-S01 walk (taken before S02).

### P02 — The route
**Goal.** The bytes of the version you signed, over the wire, or a refusal that says which of five
things went wrong.

**Exit criteria.** `GET /api/document/revision` returns a prefix that parses and re-verifies for the
requested signer, or one of five named refusals; nothing calls it on document open.

**(plan-review pin: the user's signature can be absent from the latest xref, architect — 2026-09-28)** Both sweeps
see only the newest definition of each object number, so a later revision that reuses the object number of the user's
signature dictionary hides it from `Revisions` while its signed prefix is still in the file — and D8 forbids reporting
that as "not inside this file". `SignedRevisionFor` therefore re-runs `Revisions` on each earlier revision's prefix
(each `startxref`/`%%EOF` boundary) when the latest has no record for the fingerprint, reusing the one door; a fixture
has a later revision redefine the user's dictionary's number. Remedy unverified until P02.S01.
**(plan-review pin: the causes the route can return, consistency + architect — 2026-09-28)** The record causes are
internal; each maps to exactly one D7 cause at this route. D7's `kids-hidden` is unreachable under the xref sweep — **a
decision question parked for Dan** (retire it, or redefine it), recorded in the closing batch; P02's "each of the five
causes is reachable" is restated after his answer. A hybrid-reference file must not be told "unsigned" by P02/P03 while
/pending 733 is open. The record type stays unexported; P02 exports a narrow projection (signers in coverage order, each
with fingerprint, coverage end, verified).

#### P02.S01 — `sign.SignedRevisionFor(pdf, fingerprint)`
Scope: select by fingerprint, slice, then require the prefix to parse **and** that signer to
re-verify standalone inside it. Refs: D1, D4.
Acceptance: the reproduced spoof (a stranger co-signing to EOF) does not return that stranger's
prefix for the user's fingerprint; a prefix that fails standalone re-verification is refused.

#### P02.S02 — the route
Scope: `GET /api/document/revision?signer=<fp>` through the existing mux block and `docFor`, served
with `sendDownload`'s discipline, reached from the client only via `apiFetch` (so `X-Nib-Doc` is
carried per ADR-004). Refs: D7, D10.
Acceptance: each of the five causes is reachable and distinguishable at the client; the route is
called from no boot or open path.

### P03 — The surface
**Goal.** The page the request asked for.

**Exit criteria.** One command opens one sheet showing the document, the verdict in dispute
language, every signature with its position relative to yours and whether this machine knows the
signer, and the changes; the three terminal states each render their own sentence; the fallback
chain is offered in order.

#### P03.S01 — the command and the sheet
Scope: `"Check a document that came back…"` in the `secure` tab's *Sign & Timestamp* card, with a
`data-forward` alias into *Send & Receive*; the sheet itself. Refs: D12.
Acceptance: reachable in both the sidebar and the collapsed toolbar without a second declaration;
the id gates (`ids`, `modes`, `doccontrols`) pass; keyboard-only operation and focus restore hold.

#### P03.S02 — the verdict, told for a dispute
Scope: the three terminal states, and the signer list ordered around yours. Refs: D8, D9.
Acceptance: a document co-signed by a stranger after your signature never reads as unqualified
`✓ Untampered` on this surface; a re-saved file says the signed version is not inside it; an
unsigned file says so without implying you never signed.

#### P03.S03 — "see what changed"
Scope: hand the recovered prefix to the shipped compare pipeline as bytes. Refs: D11.
Acceptance: the diff renders from server-supplied bytes with no change to the differ; the fallback
chain — kept copy → ceremony mirror → pick your own file — is offered in that order.

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
