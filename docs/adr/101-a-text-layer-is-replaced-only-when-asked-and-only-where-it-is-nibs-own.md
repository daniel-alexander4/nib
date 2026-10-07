# ADR-101 — a text layer is replaced only when asked, and only where it is Nib's own

**Status:** accepted (2026-10-07). Supersedes ADR-094's first declared gap ("a layer cannot be REPLACED") and nothing
else of it: a plain second OCR still leaves every layered page as it is. /pending 851 part 4, remainder.

## Context

ADR-094 stopped a second OCR from doubling a layer by leaving a layered page alone. That left three files nobody
could put right: one OCR'd before ADR-092, whose words are a median 0.72 of their scanned width; one OCR'd before
ADR-097, whose Hebrew and Arabic words other readers read backwards; and one read in the wrong language. Undo is
the only way back and it lasts a session.

The entry's sketch was "recognise the stamped forms, remove them with their optional-content group and the structure
`TagOCRLayer` built, re-stamp". Read from what the stamp actually writes (two words stamped and tagged, every object
dumped), a layer is, per word:

- on the page: `/Artifact <</Subtype /Watermark /Type /Pagination >>BDC q a b c d e f cm /GS0 gs /Fm0 Do Q EMC`,
  the marker replaced by `/P <</MCID n>> BDC` once the layer is described — **and a `q … Q` round everything the
  page held before**, one level per word, because pdfcpu wraps the page each time it stamps;
- a form of the word's own, `q BT /F1 12 Tf ET … cm BT … 3 Tr (…) Tj ET Q` (inside `/ReversedChars BMC … EMC` for
  ADR-097), with its own resources and a subset of one of the faces Nib stamps in;
- a name in the page's `/XObject` and one in its `/ExtGState`;
- over a described layer, a `P` per paragraph owning the words' MCIDs, a `Sect` per block, the page's row of the
  parent tree, `/MarkInfo`, and `/Artifact BMC … EMC` round the scan's image.

The optional-content group is not the layer's: it is the one group named "Watermark" that **every** pdfcpu stamp in
the document shares, a visible DRAFT watermark included. Removing "the layer's group" would have been removing a
user's.

## Decision

1. **Replacing is asked for, never implied.** `POST /api/ocr` takes `replace: true`; without it ADR-094 is the whole
   behaviour. The window asks the user — "N pages already have a searchable text layer that Nib added. Run OCR on them again?" —
   before it spends a recognition pass, and only says yes on their behalf when they do.
2. **A word is Nib's own only when all of it is the stamp's shape** (`ownOCRWords`, internal/pdfops/ocrreplace.go):
   the marker (pdfcpu's watermark marker by `watermarkArtifactSpans`, or an MCID), the placement and the draw by the
   readers the fit already pairs words with (`placementAfter`, `formDrawnAfter`), `Q EMC` straight after, and a form
   that sets ONE string, invisibly, in an OCR face, with no operator that paints. The face test is one function
   (`ocrFaceName`), which the page map's ink reader now calls too.
3. **A page is Nib's own only when taking those words out leaves no invisible text on it**, read by ADR-094's own
   rule (`hiddenTextOn`, which `PagesWithTextLayer` now calls). Another program's layer, a mix of the two, or a word
   that departs from the shape anywhere: the page is put back exactly as it was and named `not-nibs-layer`. A page
   half replaced is a page whose words are found twice.
4. **The structure goes with the content, or the page is refused.** What leaves the tree is worked out before
   anything is written: each tagged word's MCID must be owned by exactly one element that a reference can name, and
   that element's `/K` must give up exactly the words the tree's own reading says it owns. Then the words leave
   their elements, an element left with nothing leaves its parent (and so on up), and the parent-tree slots are
   emptied. Anything else is `structure`, and the page keeps its old layer. A tree the layer was the whole of is
   taken off the catalog with `/MarkInfo` and every page's `/StructParents`: an empty tree under a claim of tagging
   is what ADR-031 forbids, and before the layer the document claimed nothing.
5. **What the layer brought goes with it**: the words' spans; the stamp's `q … Q` wrappers, at most one per word
   removed and only while the pair is the outermost thing on the page (a pair beyond that may be the page's own);
   and the words' names in `/XObject` and `/ExtGState`, where the remaining content no longer uses them — out of a
   resources dictionary the page is given for itself, because resources are shared and inherited.
6. **A page is only ever replaced by words.** One the request carries no word for keeps the layer it has — a
   recognition that found nothing must not cost the page its text.
7. **A signed document is refused**, 422 `{cause: "signed"}` (ADR-072), by `sign.HasSignatureBlob` at the route;
   `GET /api/ocr/pages` reports no page of one as replaceable. (A plain OCR of a signed document is unchanged: the
   window warns and the commit door decides, as before.)
8. **One commit.** Removal and stamp are one `commitMutation`: one undo step back to the old layer, the byte cap
   (ADR-008), the ceremony freeze. `X-Nib-OCR` gains `replaced` and, for a replace, `causes` by page.
9. **The window is told by the code that does it.** `GET /api/ocr/pages` gains `own`, from `pdfops.TextLayers`,
   which takes the layer out of a copy that is thrown away — so "can be read again" is the route's own answer and
   not a second recogniser's.

## Consequences

- Measured on two real scans Nib OCR'd (3 pages / 1,717 words / 1.8 MB, and 14 pages / 13,271 words / 10.7 MB), with
  the engine's own words stamped back: every page read as Nib's own; invisible runs 1,717 → 1,717 and 13,271 →
  13,271, no page differing; no consistency or completeness defect and no parent-tree slot naming a removed element;
  both validate. Taking the layer out took 0.20 s and 1.39 s (the files fell to 1.04 MB and 5.02 MB); stamping the
  same words onto the stripped file took 3.5 s and 23.4 s, which is what a first OCR's stamp costs.
- `GET /api/ocr/pages` on the same files: 0.13 s → 0.13 s and 0.91 s → 1.35 s. Once per press of OCR.
- **An old short stamp cannot be brought up to date without reading the page again.** Its form keeps the text, a
  whole-point size and the box's bottom-left corner; the scanned box's right edge was never written, and the fit is
  that width over the advance. (A layer in the pre-ADR-097 order could be turned round in place — the letters and
  the fit are all there — and is not: see the gaps.)
- With every page layered and nothing to read again, the window no longer loads the engine to say so.

## Declared gaps

- **The optional-content group stays.** It is shared with every other stamp; after a replace the new words use it
  again. Only a replace whose every word turned out unrepresentable leaves it naming nothing.
- **`/Lang` and the `/Artifact BMC` round the scan's image stay** when a layer goes. Nothing says who set the first
  (the route sets it again from the language chosen now), and the second means nothing without a claim of tagging
  and is not written twice by the next one.
- **A layer a reviewer has since edited in the Tags panel** is replaced if its words are still owned one-to-one —
  a paragraph retyped as a heading loses that with its words — and refused (`structure`, or `not-nibs-layer` where a
  word was made an artifact, which changes its marker) otherwise.
- **An empty element written inline** in its parent is left empty: no reference names it to take it out by.
- **Turning a pre-ADR-097 right-to-left layer round without reading the page again** is not built.
- **The window's half is driven in jsdom with the recogniser stubbed** — which pages are read, what is asked and
  what is sent. A real recognition followed by a real replace in a browser is not run by any tier.

## Guards

`internal/pdfops/ocrreplace_test.go`: `TestOnlyNibsOwnLayerIsEverTakenOut` (six ways a page is not Nib's own, each
left byte for byte), `TestTakingALayerOutLeavesTheDocumentAsItWasBeforeIt`,
`TestReplacingATaggedLayerLeavesOneRunPerWordAndNoStructurePointingNowhere`,
`TestAPageNotAskedForKeepsItsLayerAndItsStructure`, `TestALayerWhoseStructureCannotBeTakenOutWithItIsLeftAlone`,
`TestAShortStampIsBroughtUpToDateByReplacingIt`, `TestAWatermarkStampedAfterTheLayerStaysWhenTheLayerGoes`.
`internal/server/ocr_test.go`: `TestATextLayerIsReplacedOnlyWhenAskedAndOnlyWhereItIsNibsOwn`,
`TestASignedDocumentsTextLayerIsNeverReplaced`. `test/jsdom/ocrhierarchy.test.mjs`: the driven flow.
