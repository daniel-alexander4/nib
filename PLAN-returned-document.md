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

**Status: unbuilt.** No slice has started. **There is no P00** — nib is twelve hundred commits old
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
- **The verify library cannot map a signer to a byte range.** `verify.Signer`
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
green and `AddedAfter` keeps its fail-closed arm.

#### P01.S01 — `sign.Revisions`, with identity
Scope: the recursive walk (`/Kids`, inherited `/FT`), last-pair coverage end, bounds check, and the
`/Contents` PKCS#7 leaf-SPKI fingerprint per field. `pkcs7` indirect → direct. Refs: D2, D3, D5, D6.
Acceptance:
- A hierarchical (`/Kids`) signature field appears in the result; today's walk returns nothing for it.
- A 6-element ByteRange resolves to `Index(4)+Index(5)`.
- A ByteRange exceeding the file length is refused, not sliced.
- Each returned revision's fingerprint matches the fingerprint `sign.Verify` reports for the same signer.

#### P01.S02 — one door: `AddedAfter` re-expressed over `Revisions`
Scope: delete the second walk; `trailingContentAfterLastSignature` becomes a reader of `Revisions`.
A guard asserts a single implementation. Refs: D2, D6.
Acceptance:
- Every test in `internal/sign/trailing_test.go` is green, unchanged.
- `TestAddedAfterFailsClosed` still goes red when the error arm is removed.
- The guard fails if a second ByteRange walk is introduced.

#### P01.S03 — the red proofs
Scope: a proof per new refusal and per gap-down, registered in `docs/red-proofs.md`. Refs: D7.
Acceptance: G1–G3 each have a proof that goes red for its own assertion; the `/Kids` proof fails
against the pre-S01 walk.

### P02 — The route
**Goal.** The bytes of the version you signed, over the wire, or a refusal that says which of five
things went wrong.

**Exit criteria.** `GET /api/document/revision` returns a prefix that parses and re-verifies for the
requested signer, or one of five named refusals; nothing calls it on document open.

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

- **The spoof is live in shipped code today.** A document modified and co-signed by a stranger
  reads `✓ Untampered · 2 signers` in the toolbar right now, before any of this is built. This plan
  fixes the *new* surface; whether the existing badge should also change is a separate call and is
  not decided here.
- **Hybrid-reference files (`/XRefStm`) were reported as a gap by the format pass and not
  re-verified at the line.** If `digitorus/pdf` genuinely ignores `/XRefStm`, a signature dict in
  such a file may be unfindable and the refusal must be `no-signature` rather than a wrong prefix.
  Settle it with a fixture before P01.S01 closes.
- **`/Kids` recursion changes a shipped, red-proved, fail-closed guard.** The one flagged unmeasured
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
