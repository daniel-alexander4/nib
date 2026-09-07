# PLAN — true text reflow

**Dateline.** Seeded 2026-09-06 from `/grill "the Edit existing text with reflow capability"`, whose
measurements are this plan's factual base. The feature is `/pending 44`, declined 2026-06-24 and
reinstated to the backlog by Dan on 2026-09-06.

**Where this plan and the original brief differ, the plan wins.** Where this plan and `/pending 44`'s
declined entry differ, the plan wins — **two of that entry's four stated prerequisites do not
survive measurement**, and they are corrected here rather than quietly dropped.

**Status: unbuilt.** No slice has started. `/createcode` drives it from P01.

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
process. No content stream is parsed and no reflow happens; this is the released feature working as
users already believe it does.

**Exit criteria.**
- A replacement wider than its box is shrunk to fit, wrapped, or refused — never silently overrun.
- A replacement narrower than the original no longer leaves an unexplained band of cover-colour.
- The overflow measurement is asserted at tier 1 and visible at tier 3.

#### P01.S01 — wire the metrics to the bake path
Scope: `StampFields` measures each field's text before emitting, through `mdpdf.CoreWidth` (D3).
Refs: D1, D3, law 4.
Acceptance:
- The measured width of a known string in a known core font matches the AFM value.
- A red proof: removing the measurement turns the overflow assertion red.
- No second width implementation is introduced — the guard greps for one.

#### P01.S02 — decide and implement the overflow behaviour
Scope: shrink-to-fit down to the existing 6pt floor, then wrap within the box, then refuse with a
sentence naming what happened. Refs: D1, law 3.
Acceptance:
- Each of the three outcomes is reachable and asserted with a fixture that produces it.
- The refusal names the cause; it does not toast a generic failure.

#### P01.S03 — the client agrees with the server
Scope: the on-screen overlay reflects the same fit decision, so the preview stops disagreeing with
the bake. Refs: D1.
Acceptance: a string that shrinks server-side shrinks in the overlay; asserted at tier 2.

#### P01.S04 — carry font identity across the wire
Scope: BaseFont and the font-resource reference travel with the edit; `classifyFont` becomes the
fallback rather than the only path. Refs: D9.
Acceptance:
- The server receives a real font identity for a document that has one.
- A document with nothing better still works through the old guess, asserted.

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
