# ADR-210 — A table repeats its header on each page as header cells, and a picture in a cell is the cell's figure

Status: accepted, 2026-11-05 (/pending 938, v2.49.1). Closes ADR-183's declared "a table that crosses a page does not repeat its header" and
ADR-191's "an image inside a table cell is a figure after the table" for Markdown. Extends ADR-188, ADR-033.

1. **The header row repeats at the top of every page its table continues onto** (`mdpdf/table.go`). Every cell is wrapped and every row measured
   before any is drawn, so the header's height is known when a body row forces a break. The repeat is skipped when the header and the row that
   forced the break do not fit one page together (`headH+h <= avail`): it is an aid and never a refusal, and a table that was laid out before is laid
   out now.
2. **The repeat is header cells (TH, Scope Column), not an artifact.** The tagger writes one `Table` element PER PAGE (its state is a page's), so a
   continuation page's Table with its header drawn as an artifact would be a table of data cells with no header at all; the page's own header cells
   make each Table element read by itself. veraPDF's ua1 agrees (`TestADocumentWithARepeatedHeaderPassesUA1`). **The cost, declared:** a reader that
   walks the tree hears the header once per page. Joining a table across pages into one `Table` element is the change that would make an artifact the
   right answer, and it is the tagger's, not the layout's; at that change the repeat becomes `/Artifact` and `Role` gains the kind it would need.
3. **A picture in a table cell is set under the cell's text, in the cell** (`Options.ImageDir`, otherwise skipped as before). It goes through the
   same door as a paragraph's picture (`resolveImage`, `maxImages`, the same refusals by name), is scaled down to the cell's width and to half a
   page's height, and grows the row: a row's height is its tallest cell's text plus each picture and a gap. A row still never splits, and a row taller
   than a page is still `ErrTableRowTooTall`.
4. **The Figure is the cell's child.** `Figure.Cell` carries the cell's `Role.Block`; the tagger records each table cell's element by block and
   writes the Figure with that element as its parent, and refuses (the document falls back to an untagged render, said in the log) when the cell has
   no element rather than tag the picture by position. A picture with no description is still an artifact. `TestAPictureInATableCellIsAFigureInsideItsCell`
   asserts the parent is a `TD` and is red against a Figure parented to the root.
5. **Declared, not fixed:** a header repeat does not apply to a second header row group (GFM has one); a picture in a cell is never placed beside the
   text, and a cell of many pictures stacks them; a cell picture is not centred or aligned with the cell's text alignment.
