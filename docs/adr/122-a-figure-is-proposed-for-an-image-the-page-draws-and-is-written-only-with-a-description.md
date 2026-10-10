# ADR-122 — a figure is proposed for an image the page draws, and is written only with a description

**Status:** accepted
**Date:** 2026-10-09
**Context:** /pending 854 item 2. The tagger proposed no figure: a picture on a committed page was bracketed as
an artifact with every other uncovered drawing (`/pending 514`), which tells a reader a photograph is
decoration, and the only way to a described picture was to tag the document in another tool. `/pending 514`
had measured why nothing was proposed then — a `/Figure` nobody fills fails ua1 7.3 t1 exactly as the unmarked
image fails 7.1 t3 — so a figure can only be offered together with the question that fills it.
`internal/pdfops/flowregion.go` (`markImage`, `drawnImage`), `grouping.go` (`placeFigures`, `figureRefusal`),
`proposer.go` (`pageBlocks`), `tagreview.go`, `tagcommit.go`, `claimtagging.go` (`drawingBrackets`);
`/api/tags/propose`, `/api/tags/commit`, `nib tag propose|commit`, the Tags review card in `web/app.js`.
**Applies:** everything that proposes, reviews or commits inferred structure, and every writer that brackets a
drawing.

## Decision

**1. A figure is proposed for each IMAGE the page draws**: an image XObject painted by `Do` in the page's own
content, and an inline image. Detection is deterministic and comes from the one read the proposer already
makes (`readPageShapes` through `readPageTableLayout`): the walk that keeps a page's shapes keeps each image
with its box, its operator's span and what is open around it (`drawnImage`). No second walk.

**2. Not proposed** (`figureRefusal`), each for its own reason:

- an image drawn **inside a form XObject** — its operator is in the form's stream, the commit writes the
  page's, and the form may be drawn more than once: the reason text in a form is refused;
- an image **smaller than 8pt** either way as drawn (`figureMinSide`) — a bullet or an ornament, which stays
  the artifact it is today;
- an image already inside an **`/Artifact`** sequence, or inside a sequence with an **MCID**;
- an image drawn **turned or slanted**: under any matrix but a whole number of quarter turns it fills a
  parallelogram, and the box that bounds it is not where it is. This one is REPORTED on its page and not
  guessed at. A quarter turn and a mirrored image fill an upright rectangle and are proposed.

**3. A figure is one element**: role `Figure`, at the top level, no text, its rect the image as drawn. It sits
where its top-left corner would be read (`tablePlace`'s rule); where a figure and a table have the same
paragraphs before them, the higher top is first and then the further left (`pageBlocks`). The tables keep the
order and the places they had. A figure moves no text, ends no paragraph and ends no list.

**4. A figure is written only with a description.** `TagReview` gains `Alt` (`"alt"` in the review). A Figure
is KEPT — then its description must hold something other than white space — or IGNORED, which leaves its image
the artifact it would have been. Its role may only be `Figure`; no other element may be made one; a
description on anything else is refused. Each is `ErrTagsReview` with its own sentence. The writer holds the
same law itself (`commitProposal` writes no Figure whose description is blank), so no caller reaches a
described-by-nobody figure. The proposal's own JSON is therefore a valid review for everything EXCEPT a
proposed figure, which needs its `"alt"` or `"ignore": true`.

**5. The commit brackets the image's own operator** — `Do` with its operand — as
`/Figure <</MCID n>> BDC … EMC` through `addMarkedElementUnder`, and writes `/Alt` as the structure editor
writes one (`types.EscapedUTF16String`). The operator must be one a fresh read of the page finds uncovered
(`uncoveredDrawingSpans`, matched by where the operator ends) or the proposal is stale. A kept figure is not
ALSO bracketed as an artifact; an ignored one is.

**6. An inline image is bracketed with its own drawing: `q a b c d e f cm BI … EI Q`** (`drawingBrackets`, the
one door for every writer that brackets a drawing — the commit's figures and artifacts, and the OCR door).
pdfcpu's content scan (`model/parseContent.go`, `lookupEI`) ends an inline image only at an `EI` followed by
white space and `Q`, or by the end of the stream, so `EI`, a new line and `EMC` is a corrupt image to it.
Measured here: a page with an inline image could not be committed AT ALL — kept or ignored, `pdfcpu: corrupt
BI expression` — because the artifact pass has bracketed an inline image at its own span since `/pending 514`
and no test had committed one through pdfcpu.

**7. The review card** shows a Figure row with its type as text, a labelled field for the description, Ignore,
the moves and Show. The summary counts the figures still waiting; Commit stays enabled and a refusal shows the
server's sentence.

## Consequences

- **A changed picture is not seen.** A Figure's text is empty, so the echo that catches a changed paragraph
  cannot see an image swapped for another in the same place by the same operator: the description is committed
  to whatever is drawn there. What the commit holds is where the image is.
- **Vector drawings are not proposed.** A drawing made of painted paths stays an artifact. Declared gap.
- **A page with no text proposes nothing**, as before: it is reported as a scan's page, its picture is in no
  proposal, and a commit does not touch it. A photograph alone on a page cannot be made a Figure here.
- **An image inside a kept table's grid is a Figure of its own**, after the table — not inside the cell.
- **A description needs a language.** In a document whose catalog names none, a kept Figure fails ua1 7.2 t22
  (measured on a hand-built page) as that document's text already fails 7.2 t34. The commit sets no language.
- **An inline image drawn any other way** than `q … cm BI … EI Q` is bracketed at its own span as before, and
  pdfcpu refuses that page — the state every inline image was in.
- **Measured on the local corpus** (51 documents, 505 pages, 197 images): 16 figures proposed, in 8
  documents; not proposed — 73 inside marked content with an id, 69 inside an artifact, 12 inside a form, 27 on
  pages with no text, none too small and none turned. Every element that is not a figure — 25,670 — is
  proposed identically before and after.
- **veraPDF**, on LibreOffice's own image with its tags removed and on a hand-built page with both kinds of
  image: committed with a description, neither 7.3 t1 nor 7.1 t3 fails, and nothing fails that the same
  document with the figure ignored does not.
- **Covered by** `TestAnImageThePageDrawsIsProposedAsAFigure`, `TestWhatIsNotAPictureToDescribeIsNotAFigure`,
  `TestAnImageTurnedAQuarterIsProposedWhereItIs`, `TestFiguresSitInReadingOrder`, `TestAFigureEndsNoList`,
  `TestAPageOfPicturesAloneProposesNoFigure`, `TestAFigureIsReviewedAsAFigureWithADescriptionOrIgnored`,
  `TestAKeptFigureIsContentAndAnIgnoredOneAnArtifact`, `TestAKeptFigureBetweenItemsPartsTheirList`,
  `TestAFigureThatIsNotWhereItWasProposedIsStale`, `TestTheWriterWritesNoFigureWithoutADescription`,
  `TestAnInlineImageIsBracketedWithItsOwnDrawing`, `TestAnIgnoredInlineImageCommitsAsAnArtifact`,
  `TestAProducersImageIsProposedAndCommitsAsAFigure`, `TestAHandBuiltFigureFailsNoFigureClause`,
  `TestTheCommitRouteWritesAFigureOnlyWithItsDescription`,
  `TestTagProposeNamesAFigureAndCommitWantsItsDescription` and `test/jsdom/tagsreview.test.mjs`.
