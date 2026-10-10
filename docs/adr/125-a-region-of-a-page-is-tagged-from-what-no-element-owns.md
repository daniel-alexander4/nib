# ADR-125 — a region of a page is tagged from what no element owns

**Status:** accepted
**Date:** 2026-10-09
**Context:** /pending 855 item 2. Content the tagger missed, content a reviewer ignored (the commit wrote it as
an artifact), and content a producer left untagged could not be brought into the structure tree at all: every
edit acted on an element the tree already had. And /pending 860 decided, the same day, that a DRAWN graphic —
vector paths — is never proposed as a Figure automatically (ADR-122). Something had to be where a person makes
one.
`internal/pdfops/structregion.go` (`regionElement`, `ReadUntagged`, `pieceRect`), `textrun.go`
(`readPageDrawings`, `artifactSeq`), `flowregion.go` (`drawnPath`, `keepPath`), `grouping.go` (`groupUnowned`);
`/api/tags/edit`, `/api/tags/untagged`, `nib tag edit`, `nib tag untagged`, the Review Structure Tree panel in
`web/app.js`.
**Applies:** every edit that brings untagged page content into an existing structure tree.

## Decision

**1. One edit kind, `region`, through the batch that already exists.** Same request shape, route, signed-refusal
door (`tagwrite.Edit`), undo and before/after consistency check as ADR-124's `create`, of which it is a
superset: `Value` is the new element's standard type, `Parent` and `Index` place it by the one placement rule
(`placeAmongElements`; 0 or -1 is the root), and `Element` must be 0. It adds `Page`, `Rect`, `Pieces` and
`Alt`.

**2. A region takes what no element owns, by the centre of its box.** Every piece of the page whose box centre
is inside the rectangle and that is under no MCID: a text run, an image the page's own stream draws, a painted
path — each either bare or inside an `/Artifact` sequence. Content an element owns is left alone and is not an
error. Nothing taken is `ErrTagsReview`: *nothing untagged is in that region*.

**3. The rectangle is in the page map's space** (ADR-088): `[left, top, right, bottom]` as fractions of the
page AS DISPLAYED — crop box, `/Rotate`, origin top left — through `displaySpace`, the one converter. A piece
is placed on the displayed page by `pieceRect`, which the region edit and the reader of untagged content both
call, so what one lists the other takes. A piece is judged by the part of it the page shows: its box is cut to
the page before its centre is taken.

**4. `Pieces` names an explicit set, and takes what lies INSIDE it.** When present it replaces `Rect`: the
element takes every unowned piece whose box lies inside ANY of those rectangles. The list in the panel sends the
rectangles of the ticked pieces, so ticking a paragraph and a picture far apart does not also take what lies
between them. A listed piece's rectangle is the box of everything grouped into it, so its own members are inside
it by construction — and a ground painted under the whole page, whose CENTRE may well fall inside a ticked
drawing, is not. `Rect` is a rectangle somebody drew and keeps the centre rule. At most 4096 rectangles an
edit.

**5. One element, one MCID a sequence, in content-stream order.** `addMarkedElementUnder` for the first
sequence and `addMCIDTo` for the rest, so the page's `/StructParents` is created when it has none and the
`/ParentTree` row grows by the rule every other writer uses. The element is then moved from the end of its
parent's `/K`, where that writer lists it, to the place the edit names.

**6. A bare piece is bracketed where the commit writer brackets one.** A run at its own show operator; an image
through `drawingBrackets.around` (ADR-122 — an inline image with its `q … cm` and `Q`); a painted path from the
first operand of its first construction operator through its painting operator, because a marked-content
operator may enclose a path object and never split one.

**7. A piece inside an `/Artifact` sequence is never bracketed.** A marked sequence inside an artifact one is
content the page calls decoration and the tree calls content. Instead, when EVERY piece of the sequence is in
the region, the sequence's opener is rewritten to the marked opener — the artifact edit's inverse, which
touches the opener and nothing else, property list included. One MCID covers the whole sequence. When only part
of it is in the region the edit is refused, with a sentence saying the region cuts through content marked as
one piece of decoration and to widen it.

**8. Brackets are all that changes in the page.** With each added `BDC`/`EMC` pair taken out and each rewritten
opener put back, the stream is byte for byte what it was
(`TestAMixedRegionIsOneElementOwningEveryPieceInContentOrder` and its neighbours assert it).

**9. Painted paths are recorded whole, by the walk that already tracks the CTM.** `uncoveredDrawingSpans` has
spans and no boxes; `pageShape` has boxes for straight axis-aligned pieces only. `runWalker` gains
`keepDrawings`: each painted path is kept as one `drawnPath` — its bounding box (a curve by its control points,
whose hull holds it; half the line width added for a stroke), its span, and what is open around it — with the
images, and every `/Artifact` sequence with its opener, its close and how many pieces it holds. It is set by
ONE reader, `readPageDrawings`. `keepShapes` stays the page map's (ADR-088) and the proposer's walk records
nothing new (`TestOnlyTheRegionReaderKeepsDrawings`). A path that only clips (`W n`) paints nothing and is
never one.

**10. Refusals, each `ErrTagsReview` with its own sentence:** a type that is not standard; an element named; a
page the document does not have; a rectangle that is empty, inverted or outside 0..1; more than 4096
rectangles; a `Figure` (by standard role) with a blank `Alt` — ADR-122's law, here as at the commit; a taken
piece drawn inside a form XObject (`errCommitInForm`'s reason: the operator is in another stream, which may be
drawn more than once); part of an artifact sequence; an artifact sequence that also draws a shading or a form;
one marked inside content an element owns, or holding such content or another artifact; one the page never
closes. A document with no structure tree is the stale answer every edit gives. `Alt`, when given, is written
for any type, as the alt edit writes it.

**11. A reader of what is untagged: `GET /api/tags/untagged?page=N`, `nib tag untagged IN [--page N]
[--json]`, `pdfops.ReadUntagged`.** Read-only, behind the session door, pinned to its document (ADR-004); the
CLI's JSON is the route's shape. It lists a page's unowned pieces GROUPED: text into paragraphs by the one
grouping door (`groupUnowned` → `groupRuns`), each image, painted paths whose boxes touch as one `drawing` — a
ruled grid is one, a chart is one — and, last, each thin `rule` and plain `box` that touches no other path.
Two paths are never joined THROUGH a plain rectangle that holds the other whole: a page's ground touches
everything drawn on it and would make one drawing of the page.
Each piece carries its page, kind, text, its rectangle in displayed-page fractions, `decoration` (the page marks
it an artifact) and `inForm` (a form XObject draws it — listed so nothing untagged is hidden, and not taggable).
Pieces of different standing — bare and decoration, the page's and a form's — are never grouped together, so a
listed piece can always be taken without cutting through another's sequence.

**12. The panel: *Tag untagged content*, under the tree.** A page number and *Show untagged content* list the
page's pieces as labelled tick boxes — a real list, one tab stop a piece; the piece the keyboard is on, and
every ticked one, is outlined on its page with the tag review's outline. A type chooser, a description field
labelled as required for a figure, and *Tag selected*, which sends ONE region edit naming the ticked pieces and
places the new tag as *Add tag* does: after the selected element under its parent, else last at the top.
Nothing happens on a tick or on the select's change but the outline. After the edit the tree is read again, the
new tag is selected and focused, and the page's list is read again. The section sits AFTER the edit bar, so it
adds no Tab stop between the tree and the controls for the selected tag. A signed document is refused by the
door, and the server's sentence is shown, as for every other control of the card.

## What changed on the way

- **The pointer path is not built.** The design allowed for leaving it out if reusing the drag mechanism
  needed more than about 150 lines or disturbed another tool, and it does both: a dragged rectangle on the page
  is a placement TOOL here — a per-view `…Mode` flag, a `pointerdown`/`pointermove`/`pointerup` triple on
  `#viewerWrap`, an entry in `PLACEMENT_TOOLS` (the keyboard-placement door) and in `disarmEditingTools` (the one
  door for "nothing is armed", which every other arm calls), and the guards that census both. A twelfth tool
  changes what arming each of the other eleven does. The list is the complete path; `Rect` is on the wire and
  on the command line for a caller that has a rectangle.
- **Go's JSON decoder pads a short array and cuts a long one.** `"rect": [0, 0, 1]` decoded as `[0 0 1 0]`
  with no error. `tagwrite.DecodeEdits` reads rectangles as slices and refuses one that is not four numbers as
  malformed (`TestDecodingReadsARegion`).
- **Rules and plain boxes were first listed one by one, and a real page made that unusable.** Measured on
  page 48 of a 67-page InDesign report in the local corpus (893 runs, 832 painted paths, none of the paths
  marked by its producer): 834 pieces, of which 832 were single rules and boxes of one chart. Every touching
  path now joins a drawing, plain or not, and the same page lists 5 pieces: 2 of text, 1 drawing, 2 rules.
- **`Pieces` first took what was centred in a rectangle, like `Rect`.** A ground under the page is centred in
  whatever piece sits at the page's middle, and went into the tag with it. Decision 4.
- **A content the reader lists had to say when it is inside a form.** Listing it without that sent a person to
  a refusal; hiding it hid untagged content. `inForm` was added, and its tick box is disabled with the reason
  in its label.

## Declared limits

- **A run is the unit.** One show operator cannot be split, so a region through the middle of a line takes the
  runs whose centres it holds, whole.
- **Part of an artifact sequence is refused**, never split.
- **Content a form XObject draws is refused.**
- **A shading (`sh`) is not taken**: it paints its clip, which the walk does not track, so it has no box.
- **No reading order is guessed.** The person places the new tag; it defaults to after the selected one.
- **A run of white space is taken when its centre is in the region and is not listed on its own**: the grouping
  door drops blank runs, and a blank run inside a paragraph's box goes with the paragraph.
- **A piece wholly off the displayed page is neither listed nor taken.**
- **A rotated run is boxed as if upright**, as everywhere `runBox` is used.
- **Clustering is bounded**: past 4,000,000 comparisons on a page the paths left are each listed alone.
- **A drawing of N paths is N marked-content sequences**, one MCID each, on one element: each path is bracketed
  at its own span, as the commit writer brackets a drawing. The chart above is 830. Bracketing a run of
  adjacent paths as one sequence is not built.
- **The region is chosen from the list, not by dragging on the page** (above).

## Consequences

- **This is where a drawn graphic becomes a Figure** (ADR-122, /pending 860): the reader lists it as a
  `drawing`, and a region typed `Figure` with a description owns its paths.
- **veraPDF, measured** (`TestAVectorDrawingTaggedAsAFigureFailsNoFigureClause`): on a page nib committed with
  one paragraph ignored and a three-path drawing, tagging the drawing as a Figure with a description fails
  neither 7.3 t1 nor 7.1 t3 and nothing the committed page did not already fail; tagging the ignored paragraph
  by region as well fails exactly what the page fails when the commit keeps the paragraph. Without a
  description the edit is refused before anything is written.
- **Cost, measured on 2026-10-09** on that 67-page, 2.7 MB document, five runs each, on a machine also running
  another project's fleet: the reader for its heaviest page 449–574 ms and for all 67 pages 644–824 ms — so
  reading and parsing the file is most of either, where the tree route's own read takes 829–1094 ms; a region
  edit tagging the 830-path chart as a Figure 1.61–1.71 s, against about 1.5 s for a retype of one element (the
  parse and the write are the cost of any edit). `ProposeTags` over the 36 corpus documents, eight alternating
  rounds of five passes: fastest pass 1.69–2.08 s before and 1.56–2.19 s after (one round disturbed to 4.7 s) —
  no difference outside the noise, as expected of a walk that sets no new flag.
- **Covered by** `internal/pdfops/structregion_test.go` — each kind of piece, the mixed region, `Pieces`, every
  refusal, placement, a page with no `/StructParents`, a turned and cropped page, the reader's list against
  what a region takes, nib's own commit and a LibreOffice document round-tripped through the artifact edit;
  `TestARegionThroughTheEditRouteTagsWhatWasIgnoredAndOneUndoTakesItBack`,
  `TestTheUntaggedRouteListsWhatNoTagOwnsAndChangesNothing`, `TestARegionIsRefusedOnASignedDocumentAtTheDoor`;
  `TestTagUntaggedListsWhatNoTagOwnsAndARegionTagsIt`; `TestDecodingReadsARegion`;
  `test/jsdom/taguntagged.test.mjs`; and `test/ui/tagregion.test.mjs`, the round trip by keyboard alone.
