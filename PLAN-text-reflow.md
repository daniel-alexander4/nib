# PLAN — true text reflow

**Dateline.** Seeded 2026-09-06 from `/grill "the Edit existing text with reflow capability"`, whose
measurements are this plan's factual base. The feature is `/pending 44`, declined 2026-06-24 and
reinstated to the backlog by Dan on 2026-09-06.

**Where this plan and the original brief differ, the plan wins.** Where this plan and `/pending 44`'s
declined entry differ, the plan wins — **two of that entry's four stated prerequisites do not
survive measurement**, and they are corrected here rather than quietly dropped.

**Status: building.** P01 slices all done; the phase close is next. `/createcode` drives it from P01.

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

### P01 — Measure the edit *(the shipped defect)*
**Goal.** An edit that does not fit its box is detected and handled, using metrics already in the
process. No content stream is parsed and no reflow happens; this is the released feature's **fit**
behaviour working as users already believe it does.

**Exit criteria.**
- A replacement wider than its box is shrunk to fit, wrapped, or reported — never silently overrun.
- Every outcome that CHANGES what is baked is visible on the field, not only in a message that can
  be missed; and a shrink stops while the result still reads as the line it replaced.
- The overflow measurement is asserted at tier 1 and visible at tier 3.

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
  degenerate-input guard, not a legibility floor. `stampFloorPt` is now the floor and the clamp is
  written against it.
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

### P02 — The width reader
**Goal.** Given a page and a font, return the advance for any code, from the document's own
dictionary, with an honest `none` when it cannot.

**Exit criteria.** `/Widths`, all three `/W` range forms and `/DW` are read correctly across the
corpus; core fonts fall back through D3's single door; **no lookup ever returns a silent zero**
(law 2); the corpus census is a guard, so a regression in coverage is a red test.

### P03 — Positioned runs, in Go
**Goal.** Read a page into runs carrying text, font, size, position and width — the input every later
phase consumes.

**Exit criteria.** Run text and count agree with pdf.js on the corpus (seam S7); image-only pages
return zero runs and say so structurally; a malformed document is contained per D6 and the
containment is probed non-zero.

### P04 — Lines and paragraphs
**Goal.** Group runs into lines and lines into paragraphs, once, shared with `PLAN-accessibility`
P08 (D10).

**Exit criteria.** Paragraph counts match hand-checked expectations on the corpus; the grouping rule
exists in exactly one place, asserted by a guard; columns and multi-column pages are either handled
or explicitly reported as out of the tool's competence.

### P05 — The content-stream walker
**Goal.** Parse a content stream into operators and write it back. The new capability the whole plan
rests on, and the one this repo has never had.

**Exit criteria.** Law 1 holds — an unedited page round-trips byte-identically across the corpus;
inline images, string and hex literals, dicts, marked content and nested XObjects all survive; the
cost of a walk is **measured** on a real page, not estimated.

**Note.** This is the same surface `PLAN-accessibility` P05 needs in order to wrap content in
BDC/EMC. Whichever plan reaches it first builds it; the other extends it.

### P06 — Reflow one paragraph on one page
**Goal.** The feature, at its smallest honest scope: edit a word, re-wrap the paragraph in its own
font, remove the original text.

**Exit criteria.** The reflowed paragraph's line breaks match a re-layout using the document's own
advances; the original text is genuinely gone rather than covered; a missing glyph falls back with
its cause stated (D8, law 3); a signed document is refused at the server door (D11).

### P07 — Several paragraphs, and flow across pages
**Goal.** An edit that overflows its paragraph pushes the ones below it, and eventually onto the next
page.

**Exit criteria.** Content that moves takes its annotations, links and form widgets with it, or the
operation refuses; nothing anchored to a position is silently orphaned.

**Standing caveat.** This is where reflow stops being local. Every object anchored by absolute
position on a page — and, once `PLAN-accessibility` lands, every MCID in the tag tree — is a thing
that must move or break.

### P08 — Typographic fidelity
**Goal.** Justification, kerning from `TJ` arrays, and the text-state parameters the current edit
path reads none of — `Tc`, `Tw`, `Tz`.

**Exit criteria.** A justified paragraph re-wraps justified; kerned text keeps its kerning; a
document using character or word spacing is not silently re-spaced.

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
