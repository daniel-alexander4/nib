# PLAN — true text reflow

**Dateline.** Seeded 2026-09-06 from `/grill "the Edit existing text with reflow capability"`, whose
measurements are this plan's factual base. The feature is `/pending 44`, declined 2026-06-24 and
reinstated to the backlog by Dan on 2026-09-06.

**Where this plan and the original brief differ, the plan wins.** Where this plan and `/pending 44`'s
declined entry differ, the plan wins — **two of that entry's four stated prerequisites do not
survive measurement**, and they are corrected here rather than quietly dropped.

**Status: building.** P01 CLOSED (v1.128.69); P02–P04 CLOSED (v1.129.73–.75, built as `PLAN-accessibility.md` P08.S01–S03 per D10); P05 CLOSED (v1.167.9 — the walker's evidence, and ADR-056's page-content door); P06 CLOSED (v1.168.1 — one paragraph on one page); P07 CLOSED (v1.173.1 — several paragraphs, and flow across pages); P08 — typographic fidelity — is building (slices firmed 2026-09-30).

---

## What this is

Nib edits text by **cover-and-replace**: read the runs under a drawn box, stamp an opaque cover, draw
a replacement on top. The original text stays in the content stream; making the edit permanent
rasterises the page. True reflow is the other thing — change a word in the middle of a paragraph and
have the paragraph re-wrap, in the document's own font, with the original text actually gone.

**There is no P00.** The repo needs no bootstrap; product work starts at P01.

## The measured starting point

Every figure here was produced by running something on 2026-09-06.

- **Advance widths are in the file, not the font program.** Across **256 fonts in 18 PDFs from 15
  producers**: `/Widths` 150, `/W` 95, `/DW`-only 1 — **96.1%** — and the remaining 3.9% are
  standard-14 core fonts. **Zero of 256 required parsing the embedded font program.**
- **The dictionary is authoritative, and often the only record left.** Where dictionary and embedded
  `hmtx` both exist they agreed on **5,282 of 5,282** CIDs. A further **18,472** CIDs are glyphs the
  subsetter stripped entirely — `glyf` length 0 where the dictionary still carries the correct
  advance. Parsing the subset there returns nothing.
- **Nothing measures a text edit today.** `internal/pdfops/pdfops.go:1012` emits a fixed-offset
  watermark; `internal/pdfops` references `CoreWidth`/`TextWidth` **zero** times. A replacement
  longer than its cover runs off the box, a shorter one leaves a gap, and neither is detected.
- **The metrics already exist in-process.** `mdpdf.CoreWidth` (`mdpdf/layout.go:61`) wraps pdfcpu's
  exported `font.TextWidth`/`font.CharWidth` over the real Base-14 AFM tables, byte-encoding-correct.
  It is used by `internal/p2p` and by nothing in the edit path.
- **A line-break engine already exists.** `wrapWords` (`mdpdf/layout.go:233`), with `splitWord`,
  paragraph and page-break handling — greedy and ragged-right, emitter-side only.
- **No content stream is ever parsed.** Named greps over `--include=*.go`: no tokenizer, no operator
  table, no text-state machine. The three sites that touch content treat it as opaque bytes —
  `wrapPageToBox` wraps it, the content digest hashes it, OCR only emits.
- **pdfcpu will not supply one.** Its single tokenizer, `parseContent` (`model/parseContent.go:415`),
  is unexported, exists to collect resource names, and `skipTJ` (`:122`) deliberately discards
  show-text operands.
- **`github.com/digitorus/pdf v0.1.2` is already a direct dependency** (signing) and returns
  positioned runs with widths — but it has **36 panic sites and 0 recovers**, and no `/W` support, so
  `Font.Width()` returns a **silent zero** for every CID font, which is 95 of the 256 measured.
- **Font identity never crosses the wire.** `classifyFont` (`web/app.js:8261`) is a substring search
  over a name string that defaults to Helvetica; the server's `coreFont` coerces anything unlisted to
  Helvetica too. Only a 12-way core-font guess reaches Go — no BaseFont, no resource reference.
- **Colour is sampled from pixels**, not read from the text object: darkest pixel in the box is the
  text colour, lightest is the cover (`web/app.js:8236-8241`).
- **Reflow on a signed document is already blocked at the UI door** — `editTextBtn` is in
  `EDITING_TOOLS` (`web/app.js:7770`), which signing mode disables.

## What `/pending 44` got wrong, recorded here because it will be read again

The decline named four requirements. Measured today:

1. **Paragraph/reading-order reconstruction** — still missing, and still real. But `detect.js`'s
   `buildTextRows` already clusters runs into rows, and `PLAN-accessibility`'s P08 autotagger is the
   same problem on the same input. **Shared, not duplicated** (D12).
2. **Original-font glyph-advance metrics** — **refuted.** 96.1% of fonts carry them in the
   dictionary; 0 of 256 need the font program. The entry's premise, that Nib cannot reuse an
   embedded subset and therefore cannot measure, conflated *measuring existing glyphs* with
   *rendering new ones*. Only the second is hard, and D8 avoids needing it.
3. **Content-stream text removal and re-emission** — **survives, and is now the whole difficulty.**
4. **A line-break/justification engine on top** — **the line-break half already exists** and is
   unwired. Justification does not, and is P08.

## Repo laws this plan establishes

1. **A rewrite that changes nothing changes nothing.** The content-stream walker must round-trip a
   page byte-identically when no edit is applied. This is the plan's central guard: a walker that
   silently drops an operator it did not understand corrupts documents in a way no test of the
   *edited* path can see.
2. **Nothing measures zero silently.** A width lookup that cannot find an advance reports `none` and
   triggers the fallback; it never returns 0 and lets a line pack unboundedly.
3. **Every fallback names its cause, to the user and to a counter.** Falling back to
   cover-and-replace is a legitimate outcome; falling back without saying why is how a user comes to
   believe text reflowed when it did not.
4. **One rule, one door.** Width measurement, row grouping and font classification each exist once
   and are called; a second implementation of any of them is the ADR-009 defect this repo has already
   paid for twice.
5. **A third-party PDF reader on a request path sits behind `internal/safe.Recover`**, and the
   containment is probed non-zero with a deliberately malformed document before any zero is read as
   safety.

---

## Decisions

### D1 — The fit defect is a defect, and it ships before the feature *(settled 2026-09-06 via /grill)*
Nothing measures a replacement against its box, so today's shipped edit silently overruns or leaves a
gap. That is a bug in a released feature, it is independent of reflow, and the metrics it needs are
already in the process. It is P01 and it is worth shipping alone.

### D2 — Widths come from the document's own dictionary; the font program is never parsed *(settled 2026-09-06 via /grill)*
Measured across 256 fonts: `/Widths`, `/W` (all three range forms) and `/DW` cover 96.1%, the rest are
core fonts, and none needs the program. This is also the spec position — for CID fonts the advance is
defined by `/W`/`/DW` and the program's metrics are not consulted — so dictionary arithmetic
reproduces exactly what a viewer does.

### D3 — Core-font metrics have one door, and it already exists *(settled 2026-09-06 via /grill)*
`mdpdf.CoreWidth` is the door, because it already encodes the byte-vs-rune rule that pdfcpu's core
fonts require and that `internal/p2p` got wrong once before ADR-009 homed it. The reader-side width
path calls it rather than calling `font.TextWidth` directly, which is the bug that rule exists to
prevent.

### D4 — The read side moves to Go *(settled 2026-09-06 via /grill)*
The rewrite happens where the bytes are written. A reader in the browser and a writer in Go disagree
by construction, and that disagreement is exactly the seam where a reflow silently mangles a page.
The client keeps pdf.js for what it is for — rendering and selection — and a cross-reader agreement
assertion pins the two together (seam S7).

### D5 — Nib writes its own width reader; `digitorus/pdf` is not adopted for it *(settled 2026-09-06 via /grill)*
It is already in `go.mod` and superficially perfect — positioned runs with widths. But it has no `/W`
support, so `Font.Width()` returns a silent zero for 95 of the 256 fonts measured, which is law 2's
exact failure; and 36 panic sites with no recover. Dictionary arithmetic is small enough that
inheriting those two properties to save it is a bad trade. If it is later used for anything, D6
governs.

### D6 — Any third-party PDF reader on a request path is contained and probed *(settled 2026-09-06 via /grill)*
`internal/safe.Recover` exists for this and says so in its own doc comment. Containment that has
never been made to fire is a claim, not a guard, so a malformed-PDF fixture drives it non-zero.

### D7 — The walker is nib's own, and its law is the identity round-trip *(settled 2026-09-06 via /grill)*
pdfcpu's `parseContent` is unexported and drops text, so it cannot be called — but its *shape* is
worth following: it already handles inline images (`skipBI`/`lookupEI`), string and hex literals, and
dicts, which are where a naive tokenizer breaks. Law 1 is the acceptance test.

### D8 — Reflow re-positions existing glyphs; new glyphs are a fallback trigger, not a subsetting project *(settled 2026-09-06 via /grill)*
Re-wrapping a paragraph moves glyphs the document already carries. Typed characters usually exist in
the subset too — inserting "the" into English prose needs no new glyph. When a needed glyph is
absent, the edit falls back to cover-and-replace with the cause stated (law 3), rather than growing
an embedded subset, which is the genuinely hard problem the decline correctly identified.

### D9 — Font identity crosses the wire *(settled 2026-09-06 via /grill)*
The BaseFont name and the page's font-resource reference travel with the edit. `classifyFont`'s
12-way guess stays only as the last-resort fallback for documents with nothing better, and stops
being the only thing Go ever learns. Without this, reflow measures the wrong font by construction.

### D10 — Row grouping is shared with `PLAN-accessibility`, not written twice *(settled 2026-09-06 via /grill)*
That plan's P08 autotagger must group positioned runs into lines and paragraphs; so must this one.
Two implementations of one rule is the ADR-009 defect, and here it would be two *different* opinions
about where a paragraph begins, in the same product. Whichever plan reaches it first builds it; the
other calls it. **This is a real cross-plan dependency and is stated in both plans.**

### D11 — Reflow is refused on a signed document, at the server door *(settled 2026-09-06 via /grill)*
The UI already disables the tool in signing mode. A UI-only gate is not the door — the server must
refuse too, per this repo's own repeated finding that a guard checking eight sites says nothing about
a ninth.

### D12 — The 18-PDF corpus is guard-test law *(settled 2026-09-06 via /grill)*
The fatal-bug category is *silently mangling a page*. Its oracle is the corpus this grill built — 15
producers, 256 fonts, every width form including the one honest counterexample (an Identity-H font
with no `/W` and a wrong `/DW`). Every width and walker slice runs against it.

### D13 — Scope grows in one direction, and each step is shippable *(settled 2026-09-06 via /grill)*
One paragraph on one page → several paragraphs → cross-page flow → justification. Each step ships;
none is a prototype for the next.

---

## Build order

### P01 — Measure the edit *(done 2026-09-09, v1.128.69)*
**Goal.** An edit that does not fit its box is detected and handled, using metrics already in the
process. No content stream is parsed and no reflow happens; this is the released feature's **fit**
behaviour working as users already believe it does.

**Exit criteria.**
- A replacement wider than its box is shrunk to fit, wrapped, or reported — never silently overrun.
- Every outcome that CHANGES what is baked is visible on the field, not only in a message that can
  be missed; and a shrink stops while the result still reads as the line it replaced.
- The overflow measurement is asserted at tier 1 and visible at tier 3.

**Closed 2026-09-09 at v1.128.69 — 6 clauses, all met, none credited to a path chosen because it
worked.** C1a–d (shrunk / wrapped / reported / never silently overrun) by four outcomes each with a
fixture that cannot reach the others. C2a by two distinct markers — `ovl-refit` where Nib ALTERED
what is baked, `ovl-misfit` where it did not — with the computed outline asserted non-zero at tier 3.
**C2b and C3b were partial at the first reconciliation and are now met**: tier 3 driving a real Save
found that `clearOverlays` destroys the marked overlay and `"Saved"` overwrites the fit sentence, so
on the commonest path the user was told nothing; `#fitNotice` fixed it, and the guard that proves it
was written before the fix and was red against that build (`/pending 460`). C2c by the ratio bound.
C3a by 7 trap strings held to pdfcpu's own emitted BBox rather than to a hand-copied AFM figure.

**Gates at the close:** tier 0 build, tier 1 `go test ./...` serially, tier 2 **272/272**, tier 3
**111/111**, red proofs **403**. **Tiers 4 and 6 did NOT fire** — P01 touched `internal/pdfops`,
`mdpdf`, `internal/server/overlay.go` and `web/*`, none of the session/ceremony/delivery/discovery
paths and nothing crossing two processes.

**What the close-out review found that four slice reviews could not:** two doc comments that were
correct when written and were falsified by a LATER slice — `stampFloorPt`'s *"the smallest point
size shrink-to-fit will produce"* and `FitShrunk`'s *"down to at most stampFloorPt"*, both untrue
once the ratio bound landed. Corrected, along with this plan's own S02 pin.

**Amended 2026-09-09 — one criterion left this phase, and one word of another changed.**

- **The cover-fidelity criterion is now `/pending 459`.** It read *"a replacement narrower than the
  original no longer leaves an unexplained band of cover-colour"*, and it is out of this phase
  because **D13's one direction does not pass through it**: cover-and-replace is kept permanently by
  this plan's own Out of scope (D8), as the fallback for `missing-glyph`, `no-widths`,
  `unsupported-encoding`, `signed`, tables, columns and scanned text — so no phase here will ever
  satisfy it, and P06 does not dissolve it. Measured while re-filing: over a ruled line the flat
  fill is `rgb[255 255 255]` and the rule is **erased** rather than banded, on same-length edits as
  much as short ones, so the criterion was also narrower than its own defect. **The first proposal
  was to strike it as dissolved by P06; that was refuted at the line by its own grill.**
- **"refused" became "reported"** in the first criterion, per P01.S02's pin: `/api/bake` is what
  every save, print, flatten, export and both signature paths run through — 24 call sites — so an
  HTTP refusal makes an over-long edit impossible to save at all.
- **The second criterion is new, and it is the one the S02 grill earned.** A shrink rewrites the
  user's page, and it shipped with the weakest signal of the three outcomes: a toast, no marker, and
  no bound below the 6pt floor, so 12pt→6pt was reachable silently. It is now bounded at three
  quarters of the asked-for size and marked on the element.

#### P01.S01 — wire the metrics to the bake path *(done 2026-09-09, v1.128.64)*
Scope: `StampFields` measures each field's text before emitting, through `mdpdf.CoreWidth` (D3).
Refs: D1, D3, law 4.

**Pinned 2026-09-09 by the slice's grill — the scope holds and the prescription does not.**
Measured against pdfcpu's own emitted form-XObject `BBox`, which is what actually decides where
the glyphs land:

- **`mdpdf.CoreWidth` alone is the wrong measurement**, 2 of 5 trap strings. It counts `\n` as a
  character, so `"one\ntwo"` measures 50.69pt against an emitted 20.02pt. The rule is **split on
  newline, widest line wins** — and this is not a future concern, because S02's *wrap* outcome
  works by inserting newlines.
- **Measure the RAW text, never `stampText`'s output.** `stampText` doubles `%` for pdfcpu's
  format string and pdfcpu undoubles it; measuring the escaped form reads `"50% of the time"` as
  94.04pt against an emitted 83.38pt.
- **`mdpdf.CoreWidth` PANICS on a non-core font name** (`pdfcpu: user font not loaded: Arial`), so
  the prescription as written puts a panic on `/api/bake`. The fix is structural, not defensive:
  measure `coreFont(f.Font)` — the value that already decides what is stamped — so the allowlist
  bounds the measurement and emission and measurement cannot diverge.
- **The `+2` anchor inset is worth 2pt of silent error** in every verdict and becomes a named
  constant read by both sites.
- **Four sites bake text; only this one is the "must fit a drawn box" rule.** Page numbers anchor
  to a page corner, the watermark is centred and relatively scaled, the OCR layer is invisible and
  sized from OCR's own boxes. Established by a grep over `TextWatermark` call sites, not assumed.

Tasks:
- T01 — `stampStyle(Field) (font string, pts int)`: one door for what is actually emitted,
  extracting the font coercion and size clamp already inline in `StampFields`.
- T02 — `stampInsetPt`: the `+2` anchor inset as a named constant, read by the stamp description
  and by the fit.
- T03 — `stampWidth(text, font string, pts int)`: one door for the emitted width — newline-split,
  widest line, `mdpdf.CoreWidth` per line, allowlisted font by contract.
- T04 — `Fit{Field, Page, WidthPt, BoxPt, OverrunPt}`; `StampFields` returns `([]byte, []Fit, error)`;
  `internal/server/overlay.go` updated.
- T05 — the assertions, the red proof, and the `recorded` count bump.

Acceptance:
- The measured width equals the `BBox` width pdfcpu actually emits, across the trap corpus
  (ε ≤ 0.01pt) — the oracle is pdfcpu's own layout, not a hand-copied AFM figure.
- A field carrying a non-core font name neither panics nor measures the wrong face.
- The overrun is computed against the same inset the stamp uses, proven by a fixture where a 2pt
  error flips the verdict.
- A red proof: removing the measurement turns the overflow assertion red.
- No second width implementation is introduced — the guard greps for one.

#### P01.S02 — decide and implement the overflow behaviour *(done 2026-09-09, v1.128.65)*
Scope: shrink-to-fit down to the existing 6pt floor, then wrap within the box, then refuse with a
sentence naming what happened. Refs: D1, law 3.

**Pinned 2026-09-09 by the slice's grill — the slice's purpose holds and ALL THREE of its named
outcomes were wrong.** Two of the three read as obviously correct, which is why they are recorded
rather than quietly changed:

- **"Wrap within the box" is unavailable in the common case.** Measured: two lines of 12pt
  Helvetica occupy 27.74pt against the 18pt a 20pt-tall box leaves after the anchor inset, and
  pdfcpu's `position:bl` anchors the block at the BOTTOM, so extra lines grow **upward** over
  whatever is above — on a text document, the previous line. A box drawn around one line of text
  has room for one line. Wrap is now **gated on measured vertical room**, and is the fallback for
  what shrinking cannot reach rather than the second rung for everything.
- **"The existing 6pt floor" did not exist.** `stampStyle` turned 5, 4 and 1 into **8**, so a
  shrink loop stepping down from 7 got a LARGER size back and never converged. It is a
  degenerate-input guard, not a legibility floor. `stampFloorPt` is now the ABSOLUTE floor and the
  clamp is written against it. **Superseded in part at the phase close:** the ratio bound added
  after this slice is what actually stops a shrink above 8pt, so `stampFloorPt` binds only under it
  — see `shrinkFloorFor`. This sentence read "`stampFloorPt` is now the floor" until P01's close-out
  review caught that a later slice had falsified it.
- **"Refuse" cannot mean refusing the request.** `/api/bake` is what every save, print, flatten,
  export, PDF/A conversion and both signature paths run through — **24 call sites** in
  `web/app.js` — and the client's own rule there is that a bake which is not OK **aborts the whole
  operation**. A 409 would make a document carrying one over-long edit impossible to save, print or
  sign at all. The outcome is **stamp and report**, on an `X-Nib-Fit` response header, because that
  route's body is the PDF.
- **The comparisons needed a tolerance.** A box built as `y0 + inset + h` measures back as
  41.619999999999997 for an h of 41.62, so text occupying exactly its box reported as overrunning
  and which way it fell depended on how the caller reached the number. `fitTolerancePt` is 0.01pt —
  1/7000 inch, below anything visible and above double-precision noise.

Tasks:
- T01 — `stampFloorPt`; `stampStyle`'s clamp written against it so a shrink can land on the floor.
- T02 — `mdpdf.CoreLineHeight`, guarding pdfcpu's **opposite** failure: `font.LineHeight` returns a
  silent **0** on an unlisted font where `font.CharWidth` panics. A zero is law 2's exact defect —
  `N * 0 <= height` is true for every N, so any number of lines reports as fitting.
- T03 — `mdpdf.WrapCore`: the ONE line-breaking door, adapting a plain string into the existing
  `wrapWords` engine rather than growing a second greedy wrapper (law 4, ADR-009).
- T04 — `FitOutcome` (as-drawn / shrunk / wrapped / overran), `resolveFit`'s ladder, `Fit.Outcome`
  and `Fit.StampedPt`.
- T05 — `X-Nib-Fit` on the bake response; `Fit`'s JSON tags arrive WITH their marshaller.
- T06 — the assertions, 9 mutations, 2 red proofs, `recorded` 401 → 403.

Acceptance:
- Each of the four outcomes is reachable and asserted with a fixture that produces it **and cannot
  produce the others**.
- Wrap fires only where the wrapped lines fit the box's measured height, driven from both sides of
  the boundary with the same text.
- Shrink stops at the floor and never jumps up, probed at every size from 1 to 20.
- A bake whose text cannot be made to fit still returns **200 with a whole PDF**, and names the
  cause per field.
- The cause is per-field and per-reason, never a lumped count.

#### P01.S03 — the client agrees with the server *(done 2026-09-09, v1.128.66)*
Scope: the on-screen overlay reflects the same fit decision, so the preview stops disagreeing with
the bake. Refs: D1.

**Pinned 2026-09-09 — the client READS the decision rather than reaching it.**
A live, per-keystroke fit would need width measurement in the browser, and that is a second
implementation of the rule law 4 says exists once. It would also be a *different* one: the browser's
font metrics are not pdfcpu's AFM tables, so the two would disagree in exactly the cases that
matter. The server decides; the client consumes `X-Nib-Fit`. What "agrees" means is therefore the
same **decision** — the same point size, the same verdict — not pixel-identical rasterisation, which
cover-and-replace can never have.

Tasks:
- T01 — `collectFieldsWithSources`: one walk yielding the posted array AND the overlay objects at
  matching indices, because the report addresses fields by index and a second filter would map a
  report onto the wrong overlay (ADR-009).
- T02 — `applyFitReport`: shrunk fields take `stampedPt` and re-lay out; overruns get `ovl-misfit`.
  Pinned per ADR-001 — a field deleted mid-bake must not be written to by its stale index.
- T03 — `tellFitReport`: one count per cause, never lumped, with the measured overrun in the
  sentence.
- T04 — the `ovl-misfit` rule; dashed, not filled, so the text underneath stays readable.
- T05 — tier-2 assertions, 12 mutations, and the `unreadKnown` park retired.

Acceptance:
- A field the server shrank shows the size the PAGE carries, not the size the client asked for.
- A stale index cannot write to a field that is no longer open.
- An absent or malformed header changes nothing and does not fail the save.
- Each cause is named separately with its own count, and an overrun says by how much.
- Exactly one walk decides which overlay fields bake.

#### P01.S04 — carry font identity across the wire *(done 2026-09-09, v1.128.67)*
Scope: BaseFont and the font-resource reference travel with the edit; `classifyFont` becomes the
fallback rather than the only path. Refs: D9.

**Pinned 2026-09-09 — D9's mechanism was measured in a real browser, and half of it does not exist.**

- **`tc.styles[fontName].fontFamily` is a CSS GENERIC, not a font name** — measured `"monospace"`
  for a Courier document and `"sans-serif"` for a Helvetica-Bold one. On its own it can only ever
  produce a family guess, which is what `classifyFont` already does.
- **`commonObjs.get(fontName).name` IS the document's BaseFont** — `"Courier"`, `"Helvetica-Bold"` —
  and `addEdit` has been computing it all along, then discarding it once `classifyFont` had read it.
  It lives on the `FontFaceObject` **prototype**, so `Object.keys` does not show it; and it is
  populated by **rendering**, not by `getTextContent`, which is fine because a user drags an edit
  box over a page they are looking at. **A first probe that skipped the render reported it absent
  and I briefly concluded the slice could not be built; that was wrong and is recorded here because
  the pre-render reading is the one a future reader will reproduce.**
- **The font-RESOURCE reference does not reach the client at all**, and is not carried. Nothing in
  P01 could consume it: pdfcpu stamps Base-14 faces only. When the server needs the real font dict
  it will read it from the document it already holds, which is P02/P03's work — so the wire never
  needs to carry it, and D9's second half is answered by not needing it.

**What P01 can actually consume, which is what this slice builds.** `classifyFont` cannot fail, only
be wrong — a document set in a display face is measured with Helvetica's widths and reports its
verdict with the same confidence as one measured exactly. `Fit.Fidelity` separates them: `exact`
(the document's own face is the one stamped), `alias` (metrically identical by design — Arial and
Helvetica share advance widths), `guess` (a stand-in), `unknown` (nothing was sent). A fit measured
on a guess now says so, in the sentence the user reads.

Acceptance:
- The server receives a real font identity for a document that has one.
- A document with nothing better still works through the old guess, asserted.
- Every fidelity arm is reachable, and a subset prefix of a metric-compatible face reads as `alias`
  rather than `guess` — the case that proves the prefix is stripped at all.
- `BaseFont` is advisory: what is stamped is unchanged by it, asserted against the emitted BBox.

**(2026-09-14, v1.129.72 — P02, P03 and P04 are being built by `PLAN-accessibility.md` P08.S01–S03**,
under D10's *"whichever plan reaches it first builds it; the other calls it"*. Each is held to the
exit criteria below as written, and its marker is written here as well as there. **The 18-PDF corpus
D12 names is not on disk** — no `*corpus*` directory under `~`, and no PDF under `~` or `/tmp` dated
2026-09-06 — so those slices measure against a generated corpus and say so.)

### P02 — The width reader *(done 2026-09-14, v1.129.73 — as `PLAN-accessibility.md` P08.S01; `internal/pdfops/fontwidth.go`)*
**Goal.** Given a page and a font, return the advance for any code, from the document's own
dictionary, with an honest `none` when it cannot.

**Exit criteria.** `/Widths`, all three `/W` range forms and `/DW` are read correctly across the
corpus; core fonts fall back through D3's single door; **no lookup ever returns a silent zero**
(law 2); the corpus census is a guard, so a regression in coverage is a red test.

### P03 — Positioned runs, in Go *(done 2026-09-14, v1.129.74 — as `PLAN-accessibility.md` P08.S02; `internal/pdfops/textrun.go`. Agreement with pdf.js is asserted on page text and baselines, not item count — see that slice's pin)*
**Goal.** Read a page into runs carrying text, font, size, position and width — the input every later
phase consumes.

**Exit criteria.** Run text and count agree with pdf.js on the corpus (seam S7); image-only pages
return zero runs and say so structurally; a malformed document is contained per D6 and the
containment is probed non-zero.

### P04 — Lines and paragraphs *(done 2026-09-14, v1.129.75 — as `PLAN-accessibility.md` P08.S03; `internal/pdfops/grouping.go`, one door `readPageLayout`)*
**Goal.** Group runs into lines and lines into paragraphs, once, shared with `PLAN-accessibility`
P08 (D10).

**Exit criteria.** Paragraph counts match hand-checked expectations on the corpus; the grouping rule
exists in exactly one place, asserted by a guard; columns and multi-column pages are either handled
or explicitly reported as out of the tool's competence.

### P05 — The content-stream walker *(done 2026-09-28, v1.167.9 — two slices; ledger 9 clauses, all met, E1′ as pinned; gates below)*
**Goal.** Parse a content stream into operators and write it back. The new capability the whole plan
rests on, and the one this repo has never had.

**Exit criteria.** Law 1 holds — an unedited page round-trips byte-identically across the corpus;
inline images, string and hex literals, dicts, marked content and nested XObjects all survive; the
cost of a walk is **measured** on a real page, not estimated.

**Note.** This is the same surface `PLAN-accessibility` P05 needs in order to wrap content in
BDC/EMC. Whichever plan reaches it first builds it; the other extends it.

**PIN 2026-09-28 (phase-open) — the walker exists; this phase owes its EVIDENCE, not its code.**
`PLAN-accessibility.md` P05 built `internal/contentstream` (tokenizer `tokenize.go:23`, writer
`write.go:29`, splice `write.go:63-109`) and `setPageContent` (`internal/pdfops/tagemit.go:79`) as the
page write door; `textrun.go:543` already walks into form XObjects through `Do` and records each
show operator's token span. What is missing is measured at phase-open: the byte-identity test covers
~30 **hand-written strings** (`tokenize_test.go:58`) and no real document; nothing measures a
walk's cost (no `Benchmark`, no timing, in the package or its callers).

**PIN 2026-09-28 — "byte-identically" means the DECODED content stream, never the file.** Law 1 as
written would fail on its first run through pdfcpu's writer: the 2026-09-09 deepdive measured a
no-op re-encode at `bytes_equal=false, digest_equal=true` (checklist item 7). So law 1 is two
assertions at two levels — the decoded stream round-trips byte-identically through the walker, and
the document written back through the page write door keeps its `ContentDigest`.

**"Across the corpus" names three populations**, since D12's 18-PDF corpus is not on disk (see the
P02–P04 note above): the generated corpus the P08 slices used, the real-producer corpus
(`$NIB_UA_PRODUCERS`, else `~/nib/producers` — 36 files, 9 producers on this machine), and veraPDF's
PDF/UA-1 corpus (`$NIB_DIGEST_CORPUS`, else `~/nib/verapdfs` — 297 files). An absent external corpus
is a SKIP that says so, never a pass.

#### P05.S01 — law 1 over real documents *(done 2026-09-28, v1.167.7)*
Every page content stream **and every form XObject stream a page reaches through `Do`** in the three
corpora tokenizes with complete, non-overlapping coverage and writes back byte-identically.
Acceptance:
1. The round trip holds on every stream of every document in each present corpus; a failure names
   the document, page, stream and first differing offset.
2. A construct census asserts each construct the exit criterion names was **seen** at least once —
   inline image, literal string, hex string, dict operand, marked content (`BMC`/`BDC`/`EMC`), and a
   `Do` into a form XObject — so a green round trip over a corpus lacking one cannot read as
   coverage of it.
3. ~~A page whose `/Contents` is an array is walked as the concatenation pdfcpu returns, and the test
   says how many such pages it saw.~~ **Amended 2026-09-28 at the slice grill** — pdfcpu's
   `PageContent` (`model/xreftable.go:1882`, v0.13.0) joins an array's streams with NO separator, and
   the spec lets a stream end on any token boundary, so `…(A) Tj` + `ET …` reads as the operator
   `TjET` and a stream ending in a comment swallows the next one's first line. Seventeen nib readers
   call it and seven writers (`setPageContent`'s six call sites, `wrapPageToBox`) write the join back as one stream. Measured over both
   real corpora: **0 fused joins of 77** (69 array pages) — spec-legal, not seen. So: a page whose
   `/Contents` is an array is read through ONE door that joins at token boundaries, the test says how
   many array pages and joins it saw, and a fixture of each fused shape is read correctly.

**Tasks** (from the slice grill, 2026-09-28):
- T01 — `pdfread.PageContent`: pdfcpu's per-stream decode, joined with `\n` only where two regular
  bytes meet or the earlier stream ends inside a comment; byte-identical to pdfcpu everywhere else.
- T02 — route every `internal/pdfops` reader through it; `ContentDigest` (ADR-013: its coverage is a
  format) and `internal/uacheck` (veraPDF's join unmeasured) are named exemptions at their sites.
- T03 — an AST census: nothing outside `pdfread` calls pdfcpu's `PageContent` but the named sites.
- T04 — the corpus round trip: every page stream and every form stream reachable from a page's
  resources, three corpora, with the construct census and the array-page count.
- T05 — ADR-056, and the seam inventory's P05.S01 rows.

**Closed 2026-09-28 — what the code did that the text above does not say.** Forms are walked from each page's
RESOURCES, recursively, not from its `Do` operators: a superset, since every form a `Do` can draw is in them. The
diff review added three things to T01/T04 (`code-reviews/v1.167.6-p05s01-2026-09-28.md`): the join decides each
separator from the previous STREAM (it had re-tokenized the whole join — 13.8 s for 1,000 streams), an element that
is not a stream fails the read as pdfcpu's does, and law 1 now asserts ISO 32000-1 Table A.1's operator vocabulary,
because a span-copying round trip cannot see a mis-scan. The empty name `/` at a stream's end is a third fusing shape,
found by executing the "two ways, and only two" claim. Ledger: 9 clauses, 9 met.

#### P05.S02 — the write-back, and what a walk costs *(done 2026-09-28, v1.167.8)*
A no-op walk written back through `setPageContent` and pdfcpu's writer leaves `ContentDigest`
unchanged, and the cost of a walk is measured on real pages.
Acceptance:
1. `ContentDigest` is unchanged after a tokenize → write → `setPageContent` → write-context round trip
   on every page of the generated and real-producer corpora.
2. A benchmark measures tokenize + write per page; the figure for the largest real page stream is
   recorded here with its population, machine and date.

**PIN 2026-09-28 (slice grill, measured) — clause 1 is FALSE of `ContentDigest`, and not because of the walk.** A
no-op walk written back keeps every page's decoded content byte-identical (356 pages, 35 real-producer documents) yet
moved the digest on **21 of 35** — exactly the documents whose `/Annots` graph references a page object. The digest
hashes a stream's dictionary with its body and follows `/P`, `/Dest` and `/D` back to the page, so it covers the page's
content-stream ENCODING there (`/pending 720`, `attachments.go:575-615`). The deepdive's `digest_equal=true` was a
document with no such annotation. So the clause is asserted as two: **each page's decoded content, re-read from the
written document, is the content walked** (the law, where it lives); and **the digest holds on every document whose
annotations reach no page, and moves on every one whose do** — 720's partition, pinned so closing it goes red on
purpose. The two fused fixtures move it too, by ADR-056's named exemption (`/pending 718`).

**Tasks** (slice grill, 2026-09-28):
- T01 — `TestANoOpWalkKeepsTheDigest`: tokenize → write → `setPageContent` → `api.WriteContext` over the generated and
  real-producer corpora; decoded content re-read per page; the digest partition by `annotsReachAPage`.
- T02 — `BenchmarkAWalkOfTheLargestRealPage`, and the figure below.

**Measured 2026-09-28** (i5-1155G7, Go 1.25, `-count=3`): the largest decoded page in the real-producer corpus
(`indesign/census-p60-280.pdf` p30, 134,220 bytes) walks — tokenize + write back — in **4.29–4.60 ms**
(29–31 MB/s), allocating 7.0 MB in 27 allocations. A real page's walk is milliseconds; it is not the cost that
decides whether reflow can sit on a request path.

**P05 closed 2026-09-28 at v1.167.9.** Acceptance ledger (9 clauses, split on `and`): E1 law 1 at the decoded stream ✓;
E1′ the document keeps its digest — met as pinned (content 356/356; two named, measured digest properties, /pending 720 and
718); inline images ✓, string ✓, hex ✓, dicts ✓, marked content ✓, nested XObjects ✓ (each SEEN, by census); the walk's cost
measured on a real page ✓ (4.29–4.60 ms). **The full-repo review found two regressions this phase made** — the n-up note
and tag carries compare with pdfcpu's own join and lost a divided page's notes and tags — fixed by ADR-057, each red-proved;
plus `wrapPageToBox`'s `Q` after a trailing comment. Out of phase: 23 already filed, 721–728 new
(`code-reviews/v1.167.8-p05-phase-close-2026-09-28.md`). **Graduation**: 8 rows, 1 actionable, 1 hot-path, 0 deleted.
**Closure sweep**: /pending 457 re-pointed to P06. **Required-run gates** (measured at v1.167.9): tier 0 `go build` ✓,
`go vet` ✓, `gofmt` ✓, tier 1 `go test ./...` ✓ (suiterun fp c734a5a6), tier 2 jsdom ✓ 467/467, tier 3 uirepro ✓ 160/160, 0 skipped; **tiers 4 and 6 did NOT
fire** — no slice touched `internal/server`'s session, ceremony, delivery or discovery paths, `internal/p2p` or
`internal/rendezvous`.

### P06 — Reflow one paragraph on one page *(done 2026-09-28, v1.168.1)*
**Goal.** The feature, at its smallest honest scope: edit a word, re-wrap the paragraph in its own
font, remove the original text.

**Exit criteria.** The reflowed paragraph's line breaks match a re-layout using the document's own
advances; the original text is genuinely gone rather than covered; a missing glyph falls back with
its cause stated (D8, law 3); a signed document is refused at the server door (D11).

**Phase close 2026-09-28 (v1.168.1), the four clauses checked:** ✅ line breaks match a re-layout in the document's own
advances — 76 real paragraphs and 29 with a NEW word, re-broken exactly as the breaker sets them (tier 1; the binary reads
the text back at tier 3); ✅ the original text gone, not covered — through the binary (pdf.js text layer); ✅ a missing glyph
falls back with its cause — through the binary, and now for SUBSET fonts too (the review's first critical: a subset's zero
width or undrawn code had been accepted); ✅ a signed document refused at the server door — **through the binary** (Dan's
/discuss decision): signed by Finalize, reopened, the Reflow click names the cause and sends nothing, and a direct POST is
refused `signed`. Review `code-reviews/v1.168.0-p06-phase-close-…`: 2 critical + 13 warnings in P06's code fixed (the second
critical: replacement text in a NAMED property list, and — found by the fix pass's re-review — in a nested `/ParentTree`),
out-of-phase filed /pending 729-732. Graduation: 25 keep-live, 1 gated, 0 deleted. Gates: tiers 0-3; tiers 4/6 do not fire.

**PIN 2026-09-28 (phase-open, read at the lines) — what exists, what does not, and one premise that is false.**
- **Exists**: a paragraph reaches its runs (`grouping.go:48`, `textParagraph.lines[].runs`), and a run its show operator's
  token span (`textrun.go:71` `span`, `:72` `inForm`) — `tagcommit.go:167-187` already keys runs by span to splice around
  them; the splice (`contentstream.Edit`) and the page write door (`setPageContent`); per-code advances with a `none`
  source (`fontwidth.go:162`); a signed-refusal door to copy (`tagwrite.ErrSigned`, `tagwrite.go:31`).
- **Does not exist**: per-glyph codes and advances on a run (summed and discarded in `show()`, `textrun.go:755-801`); any
  rune→code map ("does this font carry this character", D8's trigger — searched, none); a line breaker taking a width
  function (`mdpdf/layout.go:351` `wrapWords` is bound to mdpdf's own faces); deletion of an operator
  (`contentstream/write.go:61`, "Deletion is still not offered"); a committing text-edit route (today `/api/bake` returns
  bytes and `/api/save` commits).
- **D11's premise is false.** *"The UI already disables the tool in signing mode"* — `signLocked` is `docHadFlags`
  (`web/app.js:3463`), a document carrying NibFlags, not a signature; nothing in the UI or on the server stops a text edit
  on a SIGNED document today. D11's decision stands and is now the only guard, so its server door is built here and the
  UI gate is keyed on signature state as well.

#### P06.S01 — a run keeps its glyphs *(done 2026-09-28, v1.167.10)*
Each positioned run carries, per glyph, its code bytes, its advance and that advance's width source, beside the sums it
has today, read once in `show()`. Acceptance: the per-glyph advances sum to the run's width on every run of the generated
and real-producer corpora; a glyph whose width is `none` is carried as `none`, never as 0 (law 2); `TJ` kerns sit
between glyphs, not inside one.

**Tasks** (slice grill, 2026-09-28): T01 — `runGlyph` (code, text, font advance and its source, user-space advance, the
`TJ` kern before it) and `textRun.kernAfter`, recorded in `show()` only when the walker is asked (`keepGlyphs`), so every
existing reader pays nothing; `readPageGlyphRuns` is the asking door. T02 — the corpus law (glyph advances + kerns = run
width), a kerned fixture, and a `none` fixture. **Grill assumption (rung 2)**: opt-in rather than always-on — retention on
every tagging/grouping/checker read is memory nobody reads.

#### P06.S02 — which characters a font can draw *(done 2026-09-28, v1.167.11)*
One door answers, for a run's font, the code that draws rune r — or `absent` with its cause — by inverting the decoding
the reader already does (ToUnicode, or the simple font's encoding). Acceptance: every rune decoded from a corpus run maps
back to the code it came from; a rune the font does not carry answers `absent` (D8's trigger); a code shared by two runes
or unmapped is reported, never guessed.

**Tasks** (slice grill, 2026-09-28): T01 — `runFont.codesFor`, `textFor` inverted over every code the font can express
(ToUnicode keys of the font's code length; every byte of a simple font not already ToUnicode's), several codes returned as
several; `textRun.face` kept with the glyphs. T02 — the corpus round trip and four hand-built answers. **Grill assumption
(rung 2)**: built FROM `textFor` rather than beside it, so the inverse cannot disagree with the reader (law 4).

#### P06.S03 — the line breaker takes a width function *(done 2026-09-28, v1.167.12)*
`mdpdf`'s greedy wrap is re-expressed over a width function (law 4 — the existing engine, not a second one), and the
reflow path calls it with the document's own advances. Acceptance: mdpdf's output is byte-identical before and after on
its own tests; a paragraph re-wrapped at its own measure with no edit reproduces its original line breaks on the
hand-checked corpus.

**Tasks** (slice grill, 2026-09-28): T01 — `mdpdf.BreakGreedy[T]` with `BreakOps` (width, space, forced break, split);
`wrapWords` becomes one call of it, byte-identical. T02 — `pdfops/reflow.go`: `paragraphWords` (a word ends at a space
glyph, a run gap wider than 0.15 em, or a line end; the paragraph's space is the MEDIAN gap it draws; each refusal named),
`measureOf`, `rebreak`, and `readPageGlyphLayout`. **Measured**: 2,487 of 2,691 real-producer multi-line paragraphs already
re-break in place under the greedy rule; the other 204 are P08's justified/hyphenated text.

#### P06.S04 — the rewrite of one paragraph *(done 2026-09-28, v1.167.13)*
Given a paragraph and replacement text, delete its show operators and emit the re-wrapped lines in its own font, size
and colour, at its lines' baselines. Acceptance: the output's text for the page is the edited paragraph in place of the
original, and the original words are ABSENT from the content (not covered); an unedited reflow round-trips its decoded
content (law 1); a paragraph with a run in a form XObject, a rotated baseline, or a width `none` falls back with the
cause named (law 3).

**Tasks** (slice grill, 2026-09-28): T01 — each run keeps its text state (`runTextState`: text and line matrices, `Tf`
size, Tc, Tw, Tz, rise, scale). T02 — `reflowParagraph`: the paragraph's show operators DELETED; the re-broken lines
emitted in place of its last one, inside its own text object — spacing stated first, each line at its original line's
text matrix plus its lead, words separated by the font's own space glyph, the TJ array closed around a font change — then
`Tf`, `Tc`, `Tw` and the line matrix restored so later text lands unchanged. T03 — `contentAround`: only positioning and
`Tf` between its shows; a following show must reposition first.
**PIN (grill, rung 2)**: marked content BETWEEN a paragraph's lines is refused as `tagged` — moving text across per-line
MCIDs would mis-tag it, and re-anchoring them is P07's "nothing anchored is silently orphaned". Replacement text
(`/ActualText`, `/Alt`) around it is refused too, or the old words stay readable.
**PIN (review, measured)**: each line has its OWN room (`BreakOps.LineWidth`): 1,152 real multi-line paragraphs have a
first-line indent, and one measure for every line overflowed it.
**Measured**: 76 real-producer paragraphs (a declared sample) rewritten and read back exactly — text and line breaks — and
0 wrong; refused by cause: mixed-content 28, mixed-state 14, no-space-glyph 3, paragraph-grows 3, styled-word 2,
inline-follower 1, no-widths 1, rotated 1. Review `code-reviews/v1.167.12-p06s04-2026-09-28.md`.

#### P06.S05 — the route, the refusals, and the flags *(done 2026-09-28, v1.168.0)*
A committing route (`MUTATING` entry, `commitMutation` — undo is a feature for an edit, and the file itself no longer
holds the text), the signed-document refusal at the server door (D11, `tagwrite`'s shape), a UI gate keyed on signature
state, one counter per fallback cause surfaced to the user (P5), and the NibFlags decision `/pending 457` waits on.
Acceptance: each of P06's exit criteria, driven through the real binary.

**Tasks** (slice grill, 2026-09-28): T01 — `pdfops.Paragraphs` / `pdfops.ReflowParagraph` (pinned by the text shown →
`ErrReflowStale`; D11 by `sign.HasSignatureBlob`, tagwrite's door). T02 — `GET /api/paragraphs`, `POST /api/reflow` through
`commitMutation`, MUTATING entry. T03 — the dialog (`#reflowModal`), `reflowBtn` in `EDITING_TOOLS`, the cause sentences.
T04 — tier-3 `reflow.test.mjs`. **Decisions (rung 2)**: no separate per-cause counter — the response and the sentence are
the readers, and an unread counter is what `observables_test.go` refuses; NibFlags: nothing drawn outside the paragraph's
box, asserted, /pending 457 moved to P07. Ledger: clauses 1–3 met (2–3 through the binary), clause 4 met at the handler and
not through the binary.

### P07 — Several paragraphs, and flow across pages *(done 2026-09-30, v1.173.1)*
**Goal.** An edit that overflows its paragraph pushes the ones below it, and eventually onto the next
page.

**Exit criteria.** Content that moves takes its annotations, links and form widgets with it, or the
operation refuses; nothing anchored to a position is silently orphaned.

**PHASE CLOSE 2026-09-30 (v1.173.1) — exit criteria, split on `and`/`or`, each with its evidence:**
- [x] content that moves takes its **annotations** with it — notes and their popups move and cross (`TestWhatIsAnchoredMovesWithTheText`, `TestWhatIsAnchoredCrossesWithItsParagraph`); a kept anchor is never carried off its page (`TestAKeptAnchorStaysOnItsPage`, the phase-close review's C1).
- [x] …its **links** — link annotations and every destination naming what moves: links, bookmarks, named destinations, `/OpenAction`, `GoTo` in widget `/A`, `/AA` and `/Next` chains (`TestEveryDestinationToWhatLeavesCrosses`); tier 3 carries a link onto page 2 through the binary.
- [x] …its **form widgets** — a widget crosses with its `/P` re-pointed; a re-set line no longer runs under a widget beside it (C3, `TestAOneLineParagraphIsNotSetUnderAFieldBesideIt`).
- [x] **or the operation refuses** — every shape that cannot move exactly refuses by name (S03–S07, the phase-close W1–W4 refusals), and the user is told which paragraph stands in the way (`TestARefusalNamesTheParagraphBelow`, server door).
- [x] **nothing anchored to a position is silently orphaned** — NibFlags move with their text (S04/S06) or refuse; structure MCIDs and OBJRs follow a carried paragraph (S07); beads refuse; a flow never jumps content below it (C2, `TestAFlowOnlyLeavesAPageItsMarginBounds`).
**Amends S06's PIN**: "a running header there leaves no room, and the growth refuses" was FALSE (the review's probe4 landed
a paragraph in page 2's header slot); it is true now, by the margin rule (ADR-071 rule 2). **Repo law**: ADR-071.
**Required-run gates** (CLAUDE.md "the higher tiers are a SLICE gate"): tiers 0–3 run at this close (see the commit);
tiers 4/6 do NOT fire — no change to `internal/server`'s session, ceremony, delivery or discovery paths, `internal/p2p` or
`internal/rendezvous` (the server diff is `reflow_test.go` only). **Review**: `code-reviews/v1.173.0-p07-phase-close-…`
(3 critical, 7 warning, 14 info in-phase — all dispositioned; I12 reverted after its own re-review; 15 pre-existing filed
/pending 780–787). **Graduation pass**: 35 rows, 35 keep-live, 0 actionable (inventory P07 section).

**Standing caveat.** This is where reflow stops being local. Every object anchored by absolute
position on a page — and, once `PLAN-accessibility` lands, every MCID in the tag tree — is a thing
that must move or break.

**PIN 2026-09-30 (phase-open, read at the lines) — what exists, what does not.**
- **Exists**: P06's emitter re-sets a paragraph's lines at given text matrices inside its own text object and restores
  `Tf`/`Tc`/`Tw`/the line matrix after it (`reflow.go` `reflowParagraphIn`); the refusal a grown paragraph gets today,
  `causeGrows` (`reflow.go:205`); a paragraph's column (`grouping.go:49-52`) and the page's column count; the CTM the
  walker tracks (`textrun.go:266`); a matrix and a rect transform (`annotcarry.go:88-112`); painting operators classified,
  with no position (`claimtagging.go:162`, `:268`); destination readers for outlines and link actions
  (`outlinecarry.go`, `pageselect.go:957`).
- **Does not exist**: a bounding box for any non-text mark (path, image, inline image, shading, form); anything that
  moves an annotation, a destination or a NibFlag WITHIN a page by a translation; a notion of how much room lies below a
  paragraph.
- **Shape (rung 2, recorded)**: each step refuses what the next step moves, so every slice ships (D13) and the exit
  criterion's "or the operation refuses" holds at every commit — S03 refuses an anchored object in the band it moves, and
  S04 turns those refusals into moves. Whole paragraphs move; a paragraph is never split across a page (no widow/orphan
  logic) — a paragraph that does not fit whole is refused. A shrinking paragraph leaves its gap, as P06 does: the goal
  is overflow, and pulling text up is not in it. `/plan-review` did not fire: the phase is not security-, migration- or
  egress-heavy (no persisted format changes — a NibFlag keeps its `{page, frac}` shape).

#### P07.S01 — what lies below a paragraph *(done 2026-09-30, v1.169.49)*
One door, `flowRegion`, answers for paragraph p: the paragraphs below it in its column, down to the first vertical gap
clearly wider than the column's usual paragraph gap (that gap is the ROOM growth may consume; what lies past it — a
footer, a page number — stays put); the band those paragraphs occupy; and every non-text mark whose box meets the band,
from a new bounding-box reader over paths, images, inline images, shadings and forms (the walker's CTM, one door).
Acceptance: the room and the band are asserted on hand-built pages (a footer past the gap stays out of the region); every
non-text mark kind is boxed and a mark in the band names itself; a census over the generated and real-producer corpora
says how many paragraphs have room for one more line, and why the others do not.

**Tasks** (slice grill, 2026-09-30): T01 — `keepMarks` on the walker: every path paint, image `Do`, inline image and
`sh` recorded as a `pageMark` boxed under the CTM (a stroke grows by half its line width; a shading is unbounded — it
paints a clip the reader does not track). T02 — `flowRegion`: region paragraphs contiguous while each step is ≤
max(1.5 × the column's median paragraph step, 2.5 em); the room down to the highest obstacle below in the column's extent
less one em, or the page FLOOR — a bottom margin equal to the page's top margin; the band's marks and unowned text runs
listed. T03 — every mark kind boxed, the room bounded both ways, and the corpus census. **Grill assumptions (rung 2)**:
the floor mirrors the page's own top margin rather than reading every page for a document margin (one walk per page on a
request path); text no paragraph of the region owns — an artifact, another column — is an obstacle, never moved.
**Closed 2026-09-30 — what the code did that the text above does not say.** A mark that COVERS the whole band is a
backdrop (a page or cell fill: the text stays over it wherever it moves) and blocks nothing — except a shading, whose box is
unbounded because its clip is unknown. A run showing only white space draws no ink and is no obstacle (measured: 760 of 999
real-producer paragraphs read as blocked without that). The review added a sixth kind, `form`: a form the walk does not
enter (undecodable, nested past the depth bound, drawing itself) is boxed by its `/BBox`. **Census** (generated + hand-built,
real-producer corpus): of 999 real-producer paragraphs P06 can reflow, **194 have room for one more line**; blocked by a
mark in the band — path 581 (IRS form rules and vector-drawn glyphs dominate), image 34, inline image 21, text 36; no room —
content below 78, page margin 55. Review `code-reviews/v1.169.48-p07s01-2026-09-30.md`.

#### P07.S02 — move a paragraph *(done 2026-09-30, v1.169.50)*
A paragraph is re-set, unedited, `dy` lower, through P06's emitter (one door — a move is a reflow with its own text at
translated line matrices). Acceptance: the moved paragraph reads back with identical text and line breaks, each baseline
exactly `dy` lower; the decoded content outside its spans is byte-identical; a paragraph P06 would refuse refuses the
move with the same cause.

**PIN 2026-09-30 (slice grill, read at the lines) — a move is NOT a re-set, and two of the clauses above are amended.**
P06's emitter re-breaks words and spaces them at the paragraph's MEDIAN gap with its space glyph (`reflow.go`
`reflowParagraphIn`), so "moved through the emitter" would re-space every justified line and drop per-run state between
shows — a move that changes how the text is set. A move changes WHERE and nothing else: each run keeps its own show
operator (its codes, `TJ` kerning, `"`'s spacing), gains a `Tm` placing it `dy` lower in user space (tm′ = tm·CTM·T(0,−dy)·CTM⁻¹,
so a flipped or scaled CTM moves it down the PAGE), and its own line matrix is restored after it, so relative positioning
that follows lands unchanged; nothing between the runs is touched, so marked content keeps its glyphs. Its refusals are
therefore the ones that bear on a move — `text-in-form`, `inline-follower` (a show drawn straight after a moved run),
`degenerate-state` — not P06's word-level set (`styled-word`, `no-space-glyph` … say nothing about moving). **Clause 1
reads "every run reads back identical, exactly `dy` lower"; clause 3 reads "what a move cannot do refuses by name".** A
paragraph's blank runs (a show of white space only, P07.S01's 760) move with it, or the next word reads as a follower.

**Tasks** (slice grill, 2026-09-30): T01 — `runTextState.ctm`; `runMatrix.inverse`. T02 — `moveRuns` / `moveParagraph`
(`flowmove.go`): `'` and `"` rewritten as `Tj` with `"`'s spacing stated; the replacement separated on both sides;
`paragraphRunsWithBlanks`. T03 — the corpus read-back (every paragraph of the generated and real-producer corpora, every
run compared), the written round trip on `'`/`"`/`TJ`/flipped CTM/tagged, and each refusal.
**Closed 2026-09-30.** The review added a fourth refusal, `text-clips` (a run in render mode 4–7 carries the clip with
it), said to the user like every other cause. **Measured**: 1,379 paragraphs of the generated and real-producer corpora
moved and read back exactly — 171 of several lines, 630 kerned, 241 carrying blank runs; refused inline-follower 2,
text-in-form 2. Review `code-reviews/v1.169.49-p07s02-2026-09-30.md`.

#### P07.S03 — grow into the room below *(done 2026-09-30, v1.170.0)*
An edited paragraph takes the lines it needs at its own line pitch; the region below moves down by the growth, bounded by
the room; `paragraph-grows` gives way to `page-full` when the room is not enough. Anything anchored in the band — an
annotation, a destination, a NibFlag, a non-text mark — REFUSES (`anchored`) in this slice. Acceptance: the edited
paragraph reads back re-broken and every region paragraph reads back unchanged, `dy` lower; nothing outside the region
moves; each refusal is named, including which region paragraph refused and why.

**PIN 2026-09-30 (slice grill) — the edited paragraph's own anchored objects were unchecked, and are now.** A re-wrap moves
the paragraph's WORDS inside its box, so a link or widget laid over a word points at another word after any edit — true of
P06 already, and exactly what the exit criterion forbids. The `anchored` check covers annotations over the edited
paragraph's own box on every edit, growing or not, and everything anchored in the band when it grows. **Rung 2, recorded**:
a destination inside the edited paragraph's box is not checked (its first baseline does not move); a NibFlag inside it
stays allowed, as P06 decided; `paragraph-grows` is retired for `page-full`; a one-line paragraph's pitch is its column's
median line step at its size, else `no-pitch` — never an invented 1.2 em.

**Tasks** (slice grill, 2026-09-30): T01 — `shiftedTm`, the one user-space shift the move and the extra lines share.
T02 — `pageAnchors`: `/Annots` rects (widget / annotation), NibFlags on the page (top-left fractions of the crop box; a
rotated page's flag is unbounded), and positioned destinations to the page from outlines, links on every page,
`/OpenAction` and the `/Dests` name tree. T03 — grow in `reflowParagraphIn`: extra lines at the last line's matrix shifted
k × pitch; the region moved by `moveRuns` in the same edit; `no-pitch`, `page-full`, `anchored`; a region paragraph's
refusal carried with its text. T04 — `Refusal{Cause, Below}` from `ReflowParagraph`, on `/api/reflow`, said by the dialog.
T05 — hand-built grow and refusals, a corpus grow-and-read-back, tier 3 through the binary.
**Closed 2026-09-30.** The build found a critical the grill did not: a one-line paragraph carries no record of its
measure, so it is set to its COLUMN's — and stops an em short of anything drawn beside it on its line (irs-f1040's
"Apt. no." had been set straight into "Check here"). P06's corpus test now reads the edit by baseline, since a growth can
regroup the page. **Census**: 10 real paragraphs grew, 8 pushing a region; refused no-pitch 209, anchored 132, page-full
16. Tier 3 drives a growth through the binary. Review `code-reviews/v1.169.50-p07s03-2026-09-30.md`.

#### P07.S04 — what is anchored moves with the text *(done 2026-09-30, v1.171.0)*
Annotations (`/Rect` and every coordinate key its subtype carries), widgets, link and outline destinations naming a
position in the band, and NibFlags (`/pending 457`) move by the same `dy`; a non-text mark in the band still refuses, and
so does an object straddling the band's edge. Acceptance: each anchored kind is moved and read back at `dy`; each
straddling or unmovable kind refuses by name; the census from S01 is re-run and states what S04 unlocked.

**PIN 2026-09-30 (slice grill) — "in the band" is too narrow on a one-column page.** The band is the column's extent, so a
margin note, a signing flag or a change bar BESIDE a moved paragraph lies outside it and would stay behind — orphaned
silently, which is the exit criterion's own failure. The ZONE that moves is the region's own height, from the region's
bottom to the edited paragraph's, **page-wide on a one-column page** (nothing else can sit at that height there) and
column-wide on several; anything anchored inside it moves, and anything touching that height's full width without lying
inside the zone — straddling its edge, in the free room, in another column — refuses (rung 2: an object whose column is
ambiguous is refused, not guessed). The region's drawings are judged across the same page-wide band.

**Tasks** (slice grill, 2026-09-30): T01 — `anchorZone`. T02 — classify: inside moves, touching-not-inside refuses. T03 —
`shiftAnchors`: `/Rect` and `/QuadPoints` `/L` `/Vertices` `/CL` `/InkList`; destinations shifted IN PLACE and deduplicated
by array identity (one array shared by a link, a bookmark and a name moves once); NibFlags re-encoded with every other
field kept, `ctx.Properties` in step. T04 — drawings judged page-wide on a one-column page. T05 — each kind moved and read
back, each refusal, a shared destination moved once, the S01 census re-run.
**Closed 2026-09-30.** The review found the page-wide band made a COLUMN's fill stop reading as a backdrop; the backdrop is
judged on the column's band and the obstacle on the page's. **Census re-run, measured**: S04 unlocks **no** real-producer
paragraph — all 132 `anchored` refusals are a link over the edited paragraph's own words (25) or a drawing in the band
(107); none of the corpus's growable paragraphs has a note, flag or destination in its zone. S04's evidence is the
hand-built fixtures (every kind moved and read back, a shared destination moved once). Review
`code-reviews/v1.170.0-p07s04-2026-09-30.md`.

#### P07.S05 — a paragraph set on another page *(done 2026-09-30, v1.171.1)*
A paragraph re-set onto a DIFFERENT page, as its own text object: its fonts carried into that page's resources (a name
collision renamed, never overwritten), with the colour, render mode and graphics state it was drawn under. Acceptance:
the paragraph reads back on the target page with its text, font and size; the source page no longer draws it; a state
the walker cannot carry refuses by name.

**PIN 2026-09-30 (slice grill, read at the lines).** The walker tracks NO colour, no ExtGState and no clip, and nothing in
`pdfops` merges resources between pages. **Deleting from the source needs no restore**: each show is replaced by what it
does besides drawing — `Tj`/`TJ` by nothing, `'` by `T*`, `"` by `aw Tw ac Tc T*` — so every later state is exact, and only
a show relying on the text matrix refuses (as a move's does). **Rung 2, recorded**: colour is carried in DEVICE spaces only
(`g`/`rg`/`k`, `cs /Device…` + `sc`), anything else — an ExtGState, a colour space, a pattern — refuses `state-not-carried`;
a clip is not carried, and a run its clip would cut refuses the same way; a paragraph with marked content bound to its
page's structure refuses `tagged-across-pages`.

**Tasks** (slice grill, 2026-09-30): T01 — the walker records per run its fill and stroke as re-emittable device-colour
operators, whether any `gs` or non-device colour is in force, and its clip box. T02 — `deleteRuns`. T03 — `setRunsOn`: the
target's content wrapped `q … Q`, then `q <fill> <stroke> BT`, each run's `Tf`/`Tc`/`Tw`/`Tz`/`Ts`/`Tr` and its matrix in
user space translated with its own show, `ET Q`; fonts under the target's existing name for the same object, else a fresh
one — never overwritten. T04 — `state-not-carried`, `tagged-across-pages`, sentences. T05 — a two-page read-back (text,
font, size, colour, position), the source no longer drawing it, each refusal, a font-name collision.
**Closed 2026-09-30.** Deleting from the source, carrying to the target (the target's own unbalanced state closed first),
five named refusals. **Measured, and parked**: every multi-page real-producer document is tagged (14/14), so the carry
refuses all 641 of their page-1 paragraphs `tagged-across-pages` and reaches only untagged documents — `/pending 779` asks
Dan whether P07 carries the structure tree (recommended) or ships with the refusal. Review
`code-reviews/v1.171.0-p07s05-2026-09-30.md`.

#### P07.S06 — flow across pages *(done 2026-09-30, v1.172.0)*
When the room is not enough, the region's last paragraphs move to the top of the next page's region, which moves down in
turn — to the document's last page, where overflow refuses. Single-column pages only; anchored objects (S04) and NibFlags
move with their paragraph, across pages included. Acceptance: the phase's exit criterion, driven through the real binary
at tier 3 — content that moves takes its annotations, links and widgets, or the operation refuses, and nothing anchored
is silently orphaned.

**PIN 2026-09-30 (slice grill).** Landing and pushing are ONE rule, `pushDown`: a block of a column's paragraphs starting at
one of them moves down by P; what no longer fits above the page's floor leaves, whole, for the top of the next page, whose
block from its first paragraph is pushed in turn — and on the last page it refuses `page-full`. The edited page is its
first application. The carried block lands with its first baseline on the next page's first paragraph's, and pushes it by
the block's baseline span plus its column's usual paragraph step. Anchors inside what crosses CROSS: an annotation moves
from one page's `/Annots` to the next's (its `/P` re-pointed), a destination's page reference changes, a flag's page
number. **Rung 2, recorded**: the next page's body starts at its first paragraph — a running header there leaves no room,
and the growth refuses rather than guessing around it; the block keeps its own x (one document's pages share a column); a
blank next page refuses; tagged documents refuse at the carry (`/pending 779`), so the real corpus cannot reach this slice
and its evidence is generated documents.

**Tasks** (slice grill, 2026-09-30): T01 — `pushDown` over pages, returning each page's edits. T02 — the edited page
composes P06's emission, S02's moves and S05's deletions in one edit. T03 — `setRunsOn` over the target's already-edited
content. T04 — `carryAnchors`: annotations, destinations and flags across pages. T05 — two- and three-page cascades read
back, anchors carried, the last page refusing, a multi-column page refusing; tier 3 through the binary.
**Closed 2026-09-30.** Red-proofing found a popup treated as an anchor of its own — its window refused flows and was
moved twice — so a popup now moves only with its note. Tier 3 drives a flow onto page 2 and reads the carried LINK there.
Review `code-reviews/v1.171.1-p07s06-2026-09-30.md`.

#### P07.S07 — a tagged paragraph keeps its structure when it changes page *(done 2026-09-30, v1.173.0)*
**Added 2026-09-30 by Dan's decision (`/pending 779`, A)**: every multi-page real-producer document is tagged (14/14), so
without this slice S05/S06's carry refuses `tagged-across-pages` on all of them. A paragraph whose marked content belongs to
its page's structure tree is carried WITH that structure: its MCIDs renumbered free on the target page (the page's
`/StructParents` row created if it has none), each element's `/Pg` (or the MCR's) re-pointed, and its `/ParentTree`
entries moved from the source row to the target's — through the repo's existing structure writers (`structwrite.go`
`addMCIDTo`, `setParentTreeSlot`), never a second copy (ADR-009), and never claiming tagging it has not (ADR-031).
Acceptance: a tagged paragraph carried to another page reads back there with its text AND its structure element, whose
content resolves on the target page (the repo's structure reader, not a count); the source page's structure no longer
claims it; `tagged-across-pages` remains only for what cannot be carried, by name; the S05 corpus read-back reaches the
real-producer documents it could not before, and says how many.

**PIN 2026-09-30 (slice deepdive + grill, read at the lines).** The plan's writers cannot do this as named: `addMCIDTo`
cannot create a page's row (`structwrite.go:423`), appends an INTEGER kid (content on the element's `/Pg`, wrong when the
element keeps content on the source page) and overwrites a non-array `/K`; and `parentTreeDict` refuses a NESTED
`/ParentTree` (`structwrite.go:223`) — **measured: 7 of the 14 multi-page real-producer documents nest one**, so the slice
as written reaches at most half its population. **Rung 1, recorded**: (a) the write half writes nested number trees for
the two shapes nib ever writes — a key that exists (in place, in its leaf) and a new key above every key (appended to the
rightmost leaf, `/Limits` raised along its path); anything else still refuses. (b) The element's kid is REPLACED IN PLACE
by an MCR naming the target page — never an appended integer and never a re-pointed `/Pg`, so reading order and the
element's other content are untouched. (c) The page-row door is extracted from `addMarkedElementUnder` (ADR-009) and the
new MCID is above both the row and every MCID the target's own stream draws. (d) A carried sequence goes WHOLE: its
opener and `EMC` leave the source, its slot is cleared; a sequence holding anything but the leaving shows and
non-drawing state, one nested in another MCID, one drawing a form, an MCID no element claims, a property list with an
indirect value — `tagged-across-pages`. (e) **Found by the dive, same path**: `carryAnchors` moved an annotation and left
its OBJR's `/Pg` on the source page — live on any tagged form today; fixed here.

**Tasks** (slice grill, 2026-09-30): T01 — nested `/ParentTree` writing behind `parentTreeDict`'s successor
(`parentTreeLeaf`), `addMCIDTo` through `kidsArray`, the page-row door. T02 — `pageLayout.sequences`; `planTagCarry`
(source side: each leaving run's whole sequence, its element, the source edits). T03 — `setRunsOn` emits each sequence
under its own tag and property list with its new MCID, and writes the tree (slot moved, kid replaced by an MCR). T04 —
`carryAnchors` re-points a moved annotation's OBJR. T05 — tests: a tagged carry read back through `ReadStructure` and
`rowFor` both directions, `checkStructConsistency` clean, a nested tree, each refusal; the corpus read-back counts tagged
carries.
**PIN 2026-09-30 (build, measured — amends (d))**: the corpus read-back still refused 124 carries `tagged-across-pages`
after the whole-sequence cut — **90** a sequence that also draws text that stays, **26** a clipping path inside it, 8 a
sequence opened inside it. Clipping and path construction paint nothing and are now allowed; a SPLIT sequence is carried —
the source keeps the sequence, its slot and its kid, and the carried text becomes a new MCR of the same element right
after that kid, so the element reads in order across the pages. Also built from the review: the target's row is judged
before anything is written (`targetRefusal`), and a flat `/Nums` gains a row in key order (`insertNum`).
**Closed 2026-09-30.** A tagged paragraph carried or flowed to another page reads back there with its element
(`ReadStructure`: its text, and the element on the new page when nothing of it stays behind), both `/ParentTree` rows say
so, `checkStructConsistency` is clean, over flat and nested trees and onto a page that had no row; a carried link's OBJR
follows it. **The S05 corpus read-back now reaches the real producers: 58 carried, 56 of them tagged, from 7 documents**
(before: 0 — every page-1 paragraph of all 14 refused); `tagged-across-pages` 124 → **21**, and `state-not-carried` (162,
S05's device-colour rule) is now what binds. Tier 3 drives a tagged flow through the binary and reads the element on page 2
through `/api/tags/tree`. Review `code-reviews/v1.172.1-p07s07-2026-09-30.md` (3 warnings, 5 info; all dispositioned).

### P08 — Typographic fidelity
**Goal.** Justification, kerning from `TJ` arrays, and the text-state parameters the current edit
path reads none of — `Tc`, `Tw`, `Tz`.

**Exit criteria.** A justified paragraph re-wraps justified; kerned text keeps its kerning; a
document using character or word spacing is not silently re-spaced.

**PIN 2026-09-30 (phase-open, measured over the real-producer corpus `~/nib/producers`, 9 producers) — what exists, what
does not.**
- **Exists**: the reader folds `Tc`, `Tw` (single-byte code 32 only) and `Tz` into every glyph's advance and keeps each
  `TJ` adjustment as the next glyph's `kern` (`textrun.go` `show`); a word the paragraph already draws is re-emitted with
  its own codes AND kerns (`reflow.go` `known`), so kept text keeps its kerning today; a uniform `Tc`/`Tw` is restated
  before the lines and restored after them; moved paragraphs (P07) carry their operators verbatim (`flowmove.go`).
- **Does not exist**: any notion of alignment — every re-set line starts at its original line's matrix and is set at ONE
  space, the paragraph's median gap, so a justified paragraph re-wraps RAGGED (measured: of 1,126 readable paragraphs of
  three or more lines, **274 justified** — 230 by `TJ`/position, 44 by `Tw` — and AH's **75** that reflow today all come
  out ragged, silently); a typed word's kerning (new words get 0); `Tz`/`Ts` restored after the paragraph; a word whose
  runs differ in state, font or size.
- **The refusals P08 owns, on the rewrite path** (each multi-word paragraph given a one-word edit): of 11,880 multi-word paragraphs,
  **2,649 reflow**; `mixed-state` **1,463** (the largest — InDesign justifies with per-line `Tc`+`Tw`, Acrobat writes
  ±0.0005 `Tc`/`Tw` per run); `mixed-content` 1,211 — and over every readable multi-word paragraph the operators that
  `contentAround` calls foreign are text-state operators only in 472, blank shows only (/pending 732) in 276, both in 398;
  the rest (BT/ET between lines, marks, colour) are not P08's; `styled-word` 381.
- **Kerning is two populations**: per (face, size, pair), InDesign/AH/Ghostscript draw one consistent value (InDesign 2,304
  consistent nonzero pairs vs 545 inconsistent), Acrobat and Word draw positional noise (Acrobat 10,747 inconsistent vs
  2,792). A typed word can therefore borrow a pair's kern only where the page draws that pair one way.
- **Shape (rung 1/2, recorded)**: each slice turns a measured refusal or silent loss into an exact result or keeps a
  named refusal (law 3), state first because InDesign's justification IS state. Alignment is one door covering left,
  justified, centred and right (S03, S04) — the exit criterion names justified, and a centred heading re-set off its axis
  (**265** short paragraphs centred on the page in this corpus) is the same silent loss, so it is in (rung 2). Out:
  NEW hyphenation (nothing is hyphenated that was not — but a producer's break hyphen rejoins when its break moves: S07,
  added at S03 by measurement, which this pin's first wording ruled out unmeasured), and paragraphs split
  across text objects (BT/ET between lines — 2,900 refusals, not typographic). `/plan-review` did not fire: not security-,
  migration- or egress-heavy — no persisted or wire format changes (the cause list is the one wire surface, and only
  shrinks or gains a name).

#### P08.S01 — a word keeps the spacing it was drawn with *(done 2026-09-30, v1.174.0)*
A paragraph whose runs differ in `Tc`, `Tw`, `Tz` or `Ts` is re-set word by word, each kept word under the state it was
drawn in (the emitter states what changes before a word, and restores all four after the paragraph); text-state
operators between the paragraph's shows are part of it, not foreign content. A typed word takes the state of the word
before it (the first: the paragraph's first run's). Acceptance: hand-built mixed-`Tc`/`Tw`/`Tz`/`Ts` paragraphs reflow and
each word reads back at its own advance; the state after the paragraph is what it was; the corpus's `mixed-state` and
state-operator `mixed-content` refusals fall to what still cannot be carried, each named.

**Tasks** (slice grill, 2026-09-30 — **amended: per GLYPH, not per word**: in 585 of the corpus's 849 readable mixed-state
paragraphs a single word is drawn across runs of different state — Acrobat 522 of 586 — so one state per word would
re-space glyphs silently): T01 — `paragraphWords` records each glyph's spacing (`tc tw th ts tr`) from its run. T02 — the
emitter states what changes before the glyph that needs it (closing and reopening the `TJ`), converts a kern or a lead
with the `Tz` in force, sets a space's advance under the state of the glyph before it, opens with the first glyph's full
state and restores `Tf Tc Tw Tz Ts Tr Tm` after. T03 — `mixed-state` narrows to a text-matrix scale that differs (geometry,
not state); `contentAround` admits `Tc Tw Tz Ts Tr`; the edit path refuses a clipping mode (`Tr` 4–6, `text-clips`) as a
move and a carry already do. T04 — a typed word takes the spacing of the glyph before it; `REFLOW_CAUSES`' sentence for
`mixed-state` is made true; tests and the corpus census. **Grill defaults (rung 1/2)**: a word drawn under two spacings
is not `ambiguous-style` (a style is a font and a size) and its first occurrence is used; a differing scale stays refused.
**Closed 2026-09-30 — what the code did that the text above does not say** (four review rounds,
`code-reviews/v1.174.0-p08s01-2026-09-30.md`, every finding fixed). The grill default above was **overturned by the review's
critical**: a word drawn under two LOOKS — a font, a size, or a style (`Tz`, `Ts`, `Tr`: a stroked synthetic bold, a raised
figure) — silently took its first copy's look. Now the edit is ALIGNED to the paragraph word by word (`alignWords`, LCS): a
kept word keeps its own occurrence's codes, kerns and spacing, and a word in two looks is used only where EVERY longest
alignment keeps it in one look, else `ambiguous-style` (measured 0.15 ms at 100 words, 57 ms at the 2,000-word bound). A
typed word takes the paragraph's USUAL style (most glyphs; a tie to the plainer) and the fit (`Tc`, `Tw`) of the glyph
just before it. The space after a word is the font's own space under that word's spacing plus the paragraph's median
residual — a line under `Tw 2` or `Tz 110` keeps its wider spaces, one wide gap moves none (P07's "every space is the
paragraph's" became "the space the document drew after a word of that size"). The clip (`text-clips`, now refused on the
edit path too, as move and carry did — one `clipMode`) and scale refusals live in `paragraphWords`, asked before anything is
typed. **Corpus (one-word edit, real producers)**: `mixed-state` **1,463 → 206** (what remains is a size or stretch set by
`Tm`, S06's); paragraphs with only state operators between their shows no longer refuse (472 → 0); reflowed **2,649 →
2,886**. Tier 3 re-sets a mixed-spacing paragraph through the binary (red under a `Tc` refusal, on its own assertion).

#### P08.S02 — a space drawn as its own show *(done 2026-09-30, v1.174.1)*
A blank show between a paragraph's runs (`( ) Tj`, a `TJ` of spaces) is the paragraph's space and goes with it (/pending
732); a refusal that remains names what is actually there. Acceptance: N one-glyph runs with separate space shows reflow
at every N; the corpus's blank-show refusals (276, and 398 with state operators) fall; the cause sentence is true of what
it refuses.

**Tasks** (slice grill, 2026-09-30 — measured before it, at S01's close): T01 — a blank run joins the paragraph's spans
only when it is drawn in the page's own stream (a form's offsets index another stream) and its span lies BETWEEN the
paragraph's first and last show: over the corpus that unblocks 660 paragraphs and regresses none of the 5,110 that reflow;
taking every blank near the lines regresses 128. T02 — `contentAround` names what it finds over the whole range, not the
first foreign operator: marked content `tagged`, a show, paint or colour `mixed-content`, and lines drawn as separate text
objects a new `text-objects` with its own sentence. T03 — tests: one-glyph runs with separate space shows at several N, a
blank run outside the range untouched, a form's blank never deleted, each cause; the corpus census. **Grill defaults (rung
1/2)**: BT/ET between lines stays refused — allowed alone it unblocks ~0 (those paragraphs are tagged line by line); 732's
other half (the dialog's `Validated` read vs the staleness check's optimized one) is unmeasured and is filed on its own.
**PIN 2026-09-30 (build, measured — amends T01)**: the first cut exposed 97 `inline-follower` refusals, every one a blank
show straight after the paragraph's last (Acrobat 89): a blank there, with nothing but text state between, is the
paragraph's too — but only a `Tj`/`TJ` (a `"` sets the spacing what follows is drawn in, the review's W1) — 97 → 7. And a
run grouping drops is a space only if it DECODES to white space: a glyph that reads as nothing (a Dingbats mark with no
/ToUnicode) is ink, and was being cut (the review's C1). Narrowed again by round 2: a show is a space only if every code it
draws is the single-byte 32 or a code the paragraph's own lines draw as a space (a ToUnicode can call a check mark U+0020),
and never in a clipping mode.
**Closed 2026-09-30.** Corpus (one-word edit, real producers): reflowed 2,886 → **3,345**; `mixed-content` 1,691 → **124**;
`inline-follower` 97 → **7**; the line-by-line tagged producers now read `tagged` (1,233). Ledger: N one-glyph runs with
separate space shows reflow at N = 2, 3, 5 — met; the corpus's blank-show refusals fall — met (classes A and C of the
phase-open census are the 660 + 97 unblocked); the cause sentence is true of what it refuses — met (`text-objects` added,
`mixed-content` names other text, `tagged` wins over the stretch). Tier 3 re-sets a paragraph of space-shows through the
binary (red under the blank inclusion removed). Review `code-reviews/v1.174.1-p08s02-2026-09-30.md` (1 critical, 4 warnings,
2 info over two rounds; all fixed). /pending 732 closed; its unmeasured half is /pending 789.

#### P08.S03 — a justified paragraph re-wraps justified *(done 2026-09-30, v1.175.0)*
One door, `paragraphAlignment`, reads a paragraph's lines as left, justified, centred or right. A justified paragraph is
broken at its flush edge and each line but the last is set to it, the slack shared across its spaces by `TJ` adjustment
(the one mechanism every font has — `Tw` is inert on a multi-byte code); its last line is set at the paragraph's natural
space. Acceptance: hand-built and corpus justified paragraphs re-wrap with every line but the last flush to both edges;
an unedited justified paragraph still re-breaks in place; a ragged paragraph is untouched by the door.

**Tasks** (slice grill, 2026-09-30 — **amended by measurement: the breaker does not change**). Over the corpus's justified
paragraphs the current breaker at the current measure already re-breaks them in place (AH 225 of 232, InDesign 34 of 34,
Acrobat 16 of 16 and 12 of 12 two-liners); breaking at the flush edge, with natural or minimal spaces, does WORSE (AH 220,
151, 25). So justification is emission only. T01 — `paragraphAlignment`: justified when two or more lines before the last
end within 0.05 em of each other and the lines after the first start together, or — two lines — the first ends at its
column's right edge within 0.05 em, the last is shorter, and the first is set looser than the last (the evidence a single
line can give); the flush edge is where those lines end. T02 — the emitter sets every line but the last to the flush edge,
its slack (positive or negative) shared over its spaces as `TJ` adjustment; a line of one word, or one whose spaces would
fall under a quarter of their natural width, stays as set; the last line keeps its natural spaces. T03 — tests: hand-built
justified paragraphs re-wrap flush with a natural last line; a ragged one is untouched; the corpus read-back holds every
re-set justified line flush. **Grill defaults (rung 1/2)**: a word keeps the `Tc` its old line was letter-spaced with
(S01) — re-letter-spacing is not done; InDesign's per-line `Tw` reaches the space through the spacer and the slack corrects
it to the edge.
**PIN 2026-09-30 (build and review — amends T01–T02 and the defaults)**: the breaker does not change for a paragraph it
can set flush; where some line cannot reach the edge (a stretched median space puts the paragraph's own measure past its
edge — constructed: lines written to 368 against 271), the paragraph is broken again AT the edge at its natural spaces, and a
word still too wide refuses `word-too-wide`; a line within the paragraph's own tolerance (0.05 em) of the edge is at it
(real OID and URL lines end 0.01 and 0.33pt past). **The default above about `Tw` was wrong (review C3)**: carried with its
words, a line's justification `Tw` stretched every space after them to their OLD line's width — so in a justified
paragraph every glyph takes the last line's `Tw` (it moves only the space, code 32: no glyph moves). Detection needs even
spaces on the justified lines (a table row ends at the edge by one wide gap, review C2); the last line's natural space is
its usual gaps' median (within 1.5 em of its narrowest); the shrink floor is a quarter of the natural space, the spacer's
median stretch taken off. Round 2: a gap after sentence punctuation is left out of the evenness (it had vetoed 76 of 284 real
justified paragraphs), and a paragraph whose last line also ends at the edge CONTINUES past it — that line is set flush too.
**Closed 2026-09-30.** Ledger: hand-built justified paragraphs (by `Tw` and by `TJ`) re-wrap with every line but the last at
the flush edge — met, and with even spaces when stretched words land on the last line; corpus justified paragraphs re-wrap
flush — met (13 re-set, 50/50 lines flush and even; the door reads 283 of the corpus's 284 flush paragraphs as justified);
an unedited justified paragraph still re-breaks in place — met (the breaker is unchanged; the re-break at the edge fires only
on a line that cannot be set flush); a ragged paragraph is untouched by the door — met (ragged, right-aligned and table cases
read not justified, a ragged paragraph keeps its drawn spaces). Tier 3 re-wraps a Tw-justified paragraph through the binary
with every line but the last at one right edge (red with justification off). Review `code-reviews/v1.175.0-p08s03-…` (4
critical, 4 warnings, 5 info over two rounds; all dispositioned; one refinement registered as unproven — the tightest line's
`Tw` for a continuing paragraph matters only on the refit path).

#### P08.S04 — a centred or right-aligned paragraph keeps its axis
The door's centred and right paragraphs are re-set about their axis; a one-line paragraph reads as centred only on
evidence (its column or page centre, its indent). Acceptance: hand-built centred and right paragraphs re-wrap about their
axis; the corpus census of what reads as centred is recorded with its false-positive check.

#### P08.S05 — a typed word takes the document's kerning
A new word's glyph pairs take the kern the page draws for that pair in that face and size — only where the page draws it
one way; positional noise lends nothing. Acceptance: a typed word in a kerned paragraph reads back with the page's kerns;
a word the paragraph already draws keeps its own (asserted, already true); Acrobat/Word-style inconsistent kerning lends
none.

#### P08.S07 — a word broken at a line's end rejoins when the break moves
**Added 2026-09-30 at S03's grill, by measurement** — the phase-open pin put hyphenation out ("a justified producer's
hyphen is kept as the glyph it is") on no measurement, and it was wrong: a line ending in a hyphen whose next line opens in
lower case is in **301 of 909** AH and **106 of 436** InDesign multi-line paragraphs — the justifying producers — and a
re-wrap carries the fragment mid-line ("accom- modate"), silently. A broken word whose halves the edit keeps together joins
again (the hyphen glyph dropped, no space) wherever the re-wrap puts it mid-line, and keeps its hyphen only where it still
ends a line: the breaker measures the first half with its hyphen at a line's end and without it mid-line. Nothing new is
hyphenated. Acceptance: a hyphenated break moved mid-line reads back as one word; one left at a line's end keeps its hyphen;
an unedited paragraph still re-breaks in place; a hyphen that is part of a word ("well-known") is never dropped.

#### P08.S06 — a word in two styles
A word whose runs change font or size part-way through (`styled-word`, 381) is carried as its pieces, each in its own
font — and a paragraph whose runs differ in a size set by the text matrix rather than `Tf` (`mixed-state`, **190** left
after S01: InDesign's `Tf 1` with the size in `Tm`, measured at 1–60% apart) is the same problem one operator over. Acceptance: a mixed-style word reflows and reads back in both styles; a typed word that matches it is not
ambiguous; the corpus's `styled-word` refusals fall.

---

## Out of scope

- **Growing an embedded font subset** to render glyphs the document does not carry (D8). It is the
  genuinely hard problem the 2026-06-24 decline identified, and this plan routes around it.
- **Reflowing tables, columns and floats** as layout objects. P04 must *detect* multi-column text well
  enough to refuse it; rearranging it is a different feature.
- **Editing scanned text.** OCR produces an invisible layer over an image; reflowing it would reflow
  nothing the reader can see.
- **PDF/A and PDF/UA conformance of reflowed output** — that is `PLAN-accessibility`'s remit, and the
  two plans meet at P05.
- **Removing cover-and-replace.** It stays as the fallback (D8) and for the cases reflow refuses.

## Standing caveats

- **The walker's cost is unmeasured.** Widths, read APIs and existing capability were all measured;
  no page has been rewritten. P05's exit criteria require the measurement before anything is wired
  to a live save.
- **`/pending 44`'s decline is not fully overturned.** Its prerequisite 3 stands, and this plan's
  P05 is that prerequisite. What changed is that it is bounded engineering rather than the research
  problem the entry described.
- **Two plans meet at one surface.** P05 here and P05 of `PLAN-accessibility` are the same walker,
  and P04 here and P08 there are the same grouping. Built twice, they will disagree.

## Seam inventory

`~/.claude/projects/-home-dan-repos-nib/memory/instruments/text-reflow.md` — 21 rows (6 paths,
8 seams, 7 gap-downs), written against this plan before any code. Its hot-path rows are the
edit-overflow measurement on `/api/bake` and the panic-containment door of D6; it has no
`diagnostic, no standing reader` rows.
