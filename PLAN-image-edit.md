# PLAN — edit the images and drawings a page already draws

*Dateline: opened 2026-11-06 via `/createplan` (Dan's call in `/discuss`, same day: "plan it, delete one picture as slice one, move/resize and replace after it, in the same plan"). Replaces `/pending 901`. Stage 1 (seed) written from a read of the code on 2026-11-06 at v2.50.1; Stage 2 (a measurement of the load-bearing claim, D3) is recorded below. Stages 3-7 of the playbook are proportioned to a feature added to a shipped repo, see "Which stages ran". Where this plan and `/pending 901` differ, the plan wins.*

Nib edits text (Edit text, reflow, ADR-071/155) and adds stamps and pictures, but cannot select a picture or a drawing the page already draws and delete, move, resize or replace it. Acrobat, Foxit, PDF-XChange, Xodo PDF Studio and PDFgear can (README comparison table, *Where the others are stronger*).

## Repo laws this plan stands under

- **L1. One door writes page content** (ADR-009, ADR-056, ADR-129). A page object is edited through `pdfops.EditPageObjects` and nothing else; the server, the CLI and the window never build content bytes. A guard names the door.
- **L2. An edit acts on the object that was shown** (ADR-001, ADR-146). The window names an object by an address from a read; the write re-reads the page and refuses an address that no longer names the same operator. Never act on position alone.
- **L3. Nothing claims what it did not verify** (ADR-031, ADR-032). Every edit drops a PDF/UA identification inside the rewrite it already performs (`writeMutated`); a tagged document's structure is never left describing content that is gone.
- **L4. A signed document is not edited** (tagwrite's predicate, `tagwrite.go:29-48`; reflow `reflow.go:79-81`). The edit is refused at the door, not only greyed in the window.
- **L5. A page object that a form XObject draws is not edited here** (ADR-125's `InForm`). Editing the form changes every page that draws it.

## Decisions

Each is tagged by how it was reached: `auto-adopted` (correctness settled it), `assumed` (a defensible judgment on Dan's behalf, to read), `Dan` (his call). Facts below were read in the source on 2026-11-06; `file:line` are pointers to re-read at slice-grill time.

### D1 — One reader returns every object, owned or not, with an address *(auto-adopted)*
`readPageDrawings` (`textrun.go:307`) already keeps each image's own operator span (`drawnImage.span`, "`Do` with its operand, or the whole of `BI … EI`", `flowregion.go:86-90`), its box, `inForm`, `artifact`, `marked`, `objNr`, `clipped`, `translucent` and draw order. `ReadUntagged` lists only unowned pieces and cannot be the reader for this: a picture inside a Figure is exactly what a person wants to delete. A new reader `ReadPageObjects(pdf, page)` over the same walk returns every picture (later every drawing, via `groupUnowned`'s grouping) with the flags above and `Editable`/`Why`.

### D2 — An address is (page, draw-order ordinal, operator digest) *(auto-adopted)*
An ordinal alone shifts when anything before it is edited; a span alone shifts when a rewrite moves bytes. The write re-reads the page, finds the ordinal's object and compares the digest of its operator bytes (`Do` plus operand name, or the whole inline image) to the one it was given; a mismatch is 409 `object-changed` (ADR-146's way back is a reload). The request also carries `baseRev` like every posted operation.

### D3 — Delete removes the painting operator and nothing around it *(assumed; proven by the S01 probe before S02 builds on it)*
`q … cm /Im0 Do Q` becomes `q … cm  Q`: the state operators stay, so the stream stays balanced whatever surrounds the picture, and draw order of everything else is untouched. This avoids the one thing `drawingBrackets.around` (`claimtagging.go:315-340`) exists to handle (pdfcpu ends an inline image only at `EI`+space+`Q`) because the whole inline image is removed. The edit is `contentstream.Edit.Replace(span.start, span.end, nil)` then `setPageContent` (`tagemit.go:79`), the pair `region` already uses (`structregion.go:262-267`). The assumption to falsify: a page whose stream, once an operator is removed, pdfcpu's relaxed validator or a viewer reads differently. S01 measures that over the local corpora before any route exists.

### D4 — A picture inside a tagged sequence takes its structure with it, or the edit refuses *(assumed)*
A `marked` image sits inside `/P <</MCID n>> BDC … EMC`. Deleting only its `Do` leaves an empty marked-content sequence and a Figure element whose content is gone, which the PDF/UA checker and a screen reader both read as a defect Nib wrote. So: P01.S02 refuses a `marked` object (`Why: "tagged"`); P01.S03 makes it work by removing the element's reference to that content through ADR-124's `delete` when the Figure holds only this content, and by refusing when the element holds other content. Nothing is left describing what is not drawn (L3).

### D5 — The signed-document refusal is the shared predicate *(auto-adopted)*
`tagwrite.ErrSigned` / `sign.HasSignatureBlob` is "the one door for is-this-signed" (`reflow.go:79-81`). The new door calls it. A new route is caught by `TestEveryStructureWriteReachesTheSignedDocumentDoor`'s shape; this plan adds the same census for content edits.

### D6 — The window selects with a click and acts with a button or the Delete key *(assumed)*
ADR-125 refused a dragged-rectangle path ("a twelfth placement tool"); clicking a picture is not placement. ADR-105 puts a tool that acts on the open document in the menu: **Page Functions → Edit Objects**, one `.tbgroup[data-entry]` button in the existing group, armed-tool chip as the other tools. The hit-test reads the page map (`GET /api/pagemap`, `web/app.js:16010`) plus the new objects read; no pdf.js annotation editor is used (`web/app.js:13919`), because these are page content, not annotations.

### D7 — Undo is the document's own install history *(auto-adopted)*
The edit installs through `document.install` (ADR-146) so Undo/Redo and the global history budget (ADR-003) apply unchanged. No second undo.

### D8 — Move and resize rewrite the `cm` that places the picture, and refuse where it cannot *(assumed)*
The picture is the unit square under the CTM, so a move or resize is a new matrix. It is rewritten only where one `cm` sits straight before the `Do` inside the same `q … Q` (the `drawingBrackets` shape, `claimtagging.go:312-314`). A nested or composite placement, a form-drawn picture (L5), a rotated or sheared picture in P02.S01, and any picture another object's clip depends on are refused with a named cause. Nothing is "approximately moved".

### D9 — Replace uses the one import function and keeps the structure element *(assumed)*
A replacement picture arrives as a file; `pdfops.Import` is the one door a non-PDF becomes a PDF through (ADR-185), so the new picture is imported to a one-page PDF and its image XObject is taken from that, never decoded by a second reader. The old image is removed (D3), the new is drawn at the old box, and a tagged Figure keeps its element and `/Alt` (D4). The image library in the vault (`/api/images`) is a stamp library and is not the source.

### D10 — Vector drawings last, grouped as ADR-125 groups them *(assumed)*
`groupUnowned` (`grouping.go:474`) is the grouping door: paths whose boxes touch are one drawing. P03 offers a drawing as one object to delete and move; it never splits a path and never joins through a plain box that holds the other whole (ADR-125). A drawing inside an `/Artifact` sequence is whole-sequence or refused, as `region` does.

## Out of scope
Editing a picture's pixels (crop, recolour, rotate-in-place), editing a form XObject's contents, shading and pattern objects, text (Edit text and reflow own it), image annotations, a pointer-drawn rectangle select (D6), and any batch "delete every picture" command (the existing extract and shrink paths own whole-document picture work).

## Which stages ran
Stage 1 seed: this document. Stage 2 grill: below. Stages 3-6 (subsystem rounds, flat-spot passes, standards alignment, dimension reviews): proportioned to a feature in a shipped repo with its own ADRs, STANDARDS already adopted and no name to choose; each phase opens with `/createcode`'s phase-open firming, which re-grills its slices against the code as it stands. Stage 7 `/plan-review` and Stage 8 bootstrap: not run (no scaffolding to make; the repo exists). If Dan wants the plan-review gate before P01.S01, it is one command.

## Build order

### P01 — Delete one picture
Goal: select a picture the page draws and delete it, safely, on an untagged, then a tagged, document.
Exit criteria: a picture on an untagged page is deleted from the window and Undo restores it; a Figure's picture is deleted with its element or refused with a named cause; a signed, ceremony or form-drawn page is refused; `nib` has the same operation on the command line; the corpus probe shows no page read differently by pdfcpu or the checker.

#### P01.S01 — The objects reader and the delete probe
Scope: `ReadPageObjects` for pictures (D1, D2), and an instrument that deletes each picture of each corpus page in memory and compares what pdfcpu and the checker read before and after (D3's falsification). No route, no window. Refs: D1, D2, D3, L1, L2.
Acceptance:
- The reader returns every image of a page with address, box fractions (the page map's space, ADR-088), `Editable` and a `Why` for each refusal (form-drawn, tagged until S03, signed).
- `build/` instrument (local list, counts only, like `formshape.sh`) prints how many pictures were deleted cleanly and how many pages read differently afterwards; any difference is a finding that changes D3 before S02.
- Hostile fixtures: inline image, image in a form, image under an `/Artifact` sequence, image with a soft mask, a page whose `/Contents` is an array, 10,000 images (cost measured, not argued).
Tasks: *(written at slice-grill time)*

#### P01.S02 — The delete door, the route and the refusals
Scope: `pdfops.EditPageObjects(pdf, edits)` with kind `delete`; `POST /api/page/objects`; the CLI verb; refusals for signed (D5), form-drawn (L5), tagged (D4 first half), stale address (D2). Refs: D2-D5, D7, L1-L5.
Acceptance:
- The door is the only code that changes a page's content for an object; a census names it (`TestEveryPageObjectEditRoutesThroughTheDoor`) and a second writer fails it.
- Each refusal is a 4xx with a `cause`; a stale address is 409 and the window reloads (ADR-146); a signed document is refused at the door with the route's own test.
- The rewrite drops a PDF/UA identification inside the write (ADR-032); the operation joins `TestNoOperationCarriesAnIdentificationItDidNotVerify`'s census.
- Each condition is mutation-red.
Tasks: *(at slice-grill)*

#### P01.S03 — A tagged picture takes its Figure with it, or refuses
Scope: D4's second half. A Figure that holds only this content is deleted through ADR-124's `delete`; one that holds other content refuses. Refs: D4, ADR-122, ADR-124, ADR-138.
Acceptance:
- After the edit the checker finds no empty Figure and no `/ParentTree` slot naming the deleted element; the completeness gate agrees.
- A Figure with `/Alt` and one picture deletes; a Figure with a picture and a caption refuses and says so.
Tasks: *(at slice-grill)*

#### P01.S04 — Select and delete in the window
Scope: **Edit Objects** in Page Functions (D6): click selects, the outline shows the box, Delete key or button deletes, Undo restores, a refused object says why in the status line. Refs: D6, D7, ADR-104/105/087.
Acceptance:
- jsdom tier covers the state; tier 3 covers the real click on a real picture; the toolbar-width census and `foldThresholds` are re-measured if the bar changes.
- Keyboard-only: Tab reaches the objects of the page in draw order, Enter/Delete acts (accessibility reviewer seat).
Tasks: *(at slice-grill)*

### P02 — Move and resize
Goal: change where a picture is drawn, on the cases where one `cm` places it, and refuse the rest by name (D8).
Exit criteria: a plain upright picture moves and resizes from the window with Undo; nested, rotated, form-drawn and clip-dependent pictures are refused with a cause; a tagged Figure keeps its element and `/Alt`; the structure's reading order is unchanged or the edit is refused (ADR-071's law for what moves).
Slices (sketch): S01 the placement reader (the one `cm`, `upright`/`plain`); S02 move/resize door and route; S03 window handles and keyboard nudge; S04 reading order and the tagged case.

### P03 — Replace, and drawings
Goal: replace a picture keeping its place and `/Alt` (D9), and offer a vector drawing as one object to delete and move (D10).
Exit criteria: replace goes only through `pdfops.Import`; a replaced Figure keeps its element; a drawing deletes and moves whole or refuses; accuracy of the drawing grouping is judged by a `build/` instrument before the window offers it.
Slices (sketch): S01 replace door; S02 replace in the window; S03 drawings reader and delete; S04 drawings move.

## Stage 2 — recon of the load-bearing claims *(run 2026-11-06)*

**D3 (delete the painting operator only) was measured, not argued.** A throwaway probe (never committed) took every picture of the first two pages of each local-corpus document (first two pictures a page), removed its recorded span with `contentstream.Edit.Replace`, wrote through `setPageContent` and `pdfread.Write`, re-read the result and compared. Population: the four local lists (`accuracy`, `formrt`, `export`, `population`), 2,165 listed, 2,159 documents read (15 unreadable), 3,437 pages, 1,814 pictures, of which 179 are drawn by a form (refused by L5, not edited) and 236 are tagged (edited at the byte level to test the stream; D4 is about their structure).

| Result | Count |
|---|---|
| deleted, re-read by pdfcpu, one picture fewer on the page, page text-run count unchanged | 1,633 |
| write or re-read failed | 2 (one document, page 1, both pictures) |
| re-read with the wrong number of pictures | 0 |
| page text-run count changed | 0 |

**The two failures are the document, not the deletion.** A control that rewrites the same document with the page's content unchanged (`setPageContent` with the original bytes) fails to re-read in exactly the same way. A document that does not survive a rewrite through this path is a separate defect, and S01's instrument counts such documents as "not editable" rather than as delete failures.

**What this does not show.** It is 1,633 deletions on one machine's files, the first two pages and two pictures a page; it is not every picture. It checks pdfcpu's reading and the page's text-run count, not a rendered comparison, not the PDF/UA checker and not a viewer other than pdfcpu. It says nothing about a tagged picture's structure (D4) or about moving a picture (D8). S01 re-runs it as the committed instrument with a rendered comparison added.

**Claims read in the source and not yet re-read at the line:** the signed-document predicate (`tagwrite.go:29-48`), ADR-124's `delete` on a Figure that holds one content item, and the window's tool and hit-test seams (`web/app.js` lines in D6). They are pointers from a search, and each slice's grill opens them before relying on them.
