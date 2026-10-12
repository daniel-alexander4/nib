# ADR-069 — the reference door mirrors pdfcpu's unguarded recursion, and bounds its paths and its depth as well as its loops

**Status:** accepted. /pending 764. Extends `/pending 675`'s `/UseCMap` check, which becomes one row of it.

## Context

pdfcpu v0.13.0's validator recurses through a document's references. A Go stack overflow is fatal — no `recover`
catches it — so a reference loop on a chain the validator follows without a guard kills nib with every open
document's unsaved work. `/pending 675` refused one such loop (`/UseCMap`); `/pending 764` measured a second, an
802-byte tiling pattern whose `/Resources` names itself, reached through any open.

The validator's call graph was traced from source (every cycle over `validate/`, each one's guard read, and whether
the mark comes before the recursion). Of the chains a first sweep had listed as unguarded, seven are not (Type 3
`/Resources` and `/CharProcs`, annotation `/AP`, `/SMask /G`, name and number trees, thread beads, page `/Kids`), and
four it missed are (`/Properties` → `/Resources`, type 5 halftones, image `/Mask` and `/Alternates`, renditions and
media clips). The same unguarded chains keep no memo either, so SHARING is walked once per path: tiling patterns each
naming the next twice took 2.8 s to validate at depth 20 from 4.8 KB, doubling per level, and a structure every page
shares is walked again on every page (measured ×70 from one page to fifty, on each of seven resource entries).

## Decision

- **One walk, before the validator, over pdfcpu's unguarded edges** (`internal/pdfread/refgraph.go`), inside the one
  door every validating read already routes through (`Validated`, `Reader`; ADR-009's guard). Each edge is keyed on
  the ROLE pdfcpu reached the object in, because pdfcpu recurses on the role, not the shape. Objects pdfcpu marks
  before recursing — form and image XObjects, fonts, `validateStreamDictEntry` targets, ICCBased streams — stop the
  walk, so a loop pdfcpu handles is not refused. Name and number trees are depth-capped by pdfcpu (at 100), so a
  loop through `/Kids` is not refused either — nib's checker reads such trees as veraPDF does — but their sharing is
  counted, because the cap bounds depth and not breadth.
- **Three refusals, one door:** a loop (`ErrReferenceCycle`, replacing `ErrUseCMapCycle`); more paths than
  2^18 + 16 per object (`ErrReferencePaths`), counted as pdfcpu walks them, once per page for what pages share; and a
  chain deeper than 8,192 (`ErrReferenceDepth`), because depth alone grows the stack.
- **Tested in-process**: one loop per edge, each paired with the same document ending (which must validate), found by
  the walk on the unvalidated read before any loop is handed to the validator; every edge and entry red-proved by
  mutation. No child-process harness.
- `TestTheReferenceDoorNamesThePdfcpuItRestates` fails on a pdfcpu upgrade: the table is re-traced, not assumed.

## Consequences

- 0 of 333 real documents (36 producers, 297 veraPDF corpus) refused; the heaviest used 1.9 paths per object of the
  16 allowed. The walk costs 1.6% of a validated read over that corpus, 2.9% at worst (~0.2 ms on a 7 ms read).
- **Declared:** a quadratic chain pdfcpu really does walk quadratically — a reply thread of roughly 700+ annotations
  (`/IRT`), each validated from the page and each walking the thread — exceeds the budget and is refused.
- **Declared:** recursion inside pdfcpu's `model` package not reached from `validate/` was not traced.
- Six entry edges only feed the path count and have no mutation proof (page `/Annots`, `/HT`, an image's
  `/ColorSpace`, the transfer keys after `/TR`, a shading pattern's `/ExtGState`, a Rendition action's `/R`).

### Note, 2026-10-11 (`/pending 867`, `/pending 767`) — a dated record of what the door has become; the decision above stands

- **The door reads object streams.** pdfcpu's unvalidated read leaves an object held in an object stream undecoded
  (63,297 of the objects in the 36 producer documents), and the walks read the table as it stands, so every refusal
  above was passed by putting one link of a chain in an object stream. `resolveCompressed` decodes them all before the
  first walk (`/pending 840`). **The cost figures above were measured on the door that could not see those objects
  and no longer hold.**
- **A third pass** (`refall.go`, `refuseDeepReferences`, `/pending 840`) bounds, at 2^19 levels, the depth of EVERY
  reference under a font or an image, whatever its key: pdfcpu's optimize pass follows them all (`traverse`,
  `model.EqualObjects`). And pdfcpu's free-reference walk from the catalog is made by nib first, without recursion
  (`fixFreeReferences`, held against pdfcpu's own pass on fifteen shapes), so pdfcpu's finds every object entered.
- **Cost, re-measured** over the same 333 documents, best of three a document, on a machine under load (load average
  8 to 13 — the ratios are the figures, not the milliseconds): the door 727 ms of a 2,032 ms validated and optimized
  read, 36%. Of it: decoding object streams 507 ms (work the validator did itself as it met them; that it is moved
  rather than added was not re-measured), the path count 95 ms (4.7% of the read), the guarded depth pass 129 ms
  (6.4%), the every-reference pass 25 ms (1.2%). Worst: census-p60-280.pdf (2.7 MB), 419 ms of 909 ms; the largest,
  fda-170357.pdf (6.1 MB), 60 ms of 188 ms.
- **A loop is kept from the optimize pass where it could start, not refused at the door** (`duplicateLoop`,
  `optimize.go`). `traverse` marks nothing, and two equal fonts naming `10 0 obj [10 0 R]` under `/Foo` were a fatal
  stack overflow (measured in a throwaway process). The pass is skipped — refused for a reader that depends on it —
  only for a font or image that reaches a loop AND that pdfcpu could find equal to another, judged by a fingerprint
  of `EqualObjects` four containers deep that errs towards equal. A Type 3 font that names itself still optimizes.
  Of 2,547 validated documents (36 producer, 295 veraPDF, 1,576 from the Go module cache, 640 from TeX Live and
  `/usr/share/doc`; 23,007 fonts and images) **none has a font or image that reaches a loop at all**, so the rule
  skips nothing there; it costs 22 ms of the 2,032 ms above (the every-reference walk again, after validation).
- **An XObject whose `/Subtype` is a reference** skips the pass the same way: pdfcpu's relaxed validator reads it
  through, and its pass dereferences a nil (`optimize.go:697`, `:801`) — 2 of the module cache's 1,577 PDFs, both
  SHA-1 collision files. `fault.Catch` would not have held it: it holds pdfcpu's own `fault.Fail` and re-panics the rest.
- **An action is counted once per naming the walk does not follow** (`sharedActions`, `/pending 767` part 5). Outline
  items, the catalog, form fields and name-tree leaves name actions; the walk follows none of them and pdfcpu
  validates the action's whole `/Next` graph at each. Measured: an action over 32,798 paths validated in 0.88 s from
  one outline item, 6.2 s from eight, 59.5 s from sixty-four (28 KB), its count unmoved. Only an action with paths
  under it is charged, so a document without one is not scanned; no document of the 2,547 changed its answer.
- **The `/IRT` refusal stays declared.** A single reply chain of 739 annotations reads and 740 is refused; the 739
  validated for 10.7 s (under the same load), so the budget is generous there, not tight. None of 960 documents
  examined (the two corpora, TeX Live, `/usr/share/doc`; 29,051 annotations) carries an `/IRT` at all — those corpora
  hold no review documents and cannot say whether a real one reaches it.
- **`model`'s recursion, traced** (every self- or mutually-recursive function of the package, found by name over its
  call graph — a call through a function value is not seen): the page-tree walkers and `processRefCounts` check
  `CheckRecursionDepth`; the parser bounds its nesting; `EqualObjects` is bounded for forms by `Unaffordable` and for
  fonts and images by the third pass; name-tree `Value` and `Remove` recurse over nodes in memory (named here, not read
  line by line). **`DeleteObject` (xreftable.go:573) is not bounded by anything:** it frees an object and recurses through
  every reference it holds, one frame a link (a loop ends at the freed object). nib reaches it from
  `pdfops/scan.go`'s key removal. Unmeasured; filed with the report that closed this note.
