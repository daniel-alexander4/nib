# ADR-192 — the model reads only what a reader sees, and a page takes the author's tree only when the tree accounts for its words

Status: accepted, 2026-11-03 (PLAN-import-export P02.S03c, v2.36.0). Extends ADR-088, ADR-190, ADR-191; ADR-031.

**One reader.** `internal/docmodel` holds the types; `docmodel/read.Read` is the only reader of them. Writers import the
model and no PDF reader: `TestNoWriterReachesAPDFReader` walks the transitive closure (`go list -deps`) of the model and
of `internal/docwrite/...`, with a positive control (the reader must be seen reaching `pdfops`) and a planted-import red proof.

**What may be read (D30, X6).** The walker gives runs, painted pieces and images one 1-based draw sequence
(`runWalker.draws`) and each the facts a cover test needs (`translucent`, `clipped`, `layerOff`). `visibleRuns`
(exportvis.go) drops, and counts by cause, text that is off the page, in optional content OFF in `/OCProperties /D`
(`/OC` on a `BDC` and on an XObject; groups and `/OCMD` by `/P`), wholly inside a LATER opaque unclipped rectangle or
upright unmasked image, or hidden (OCR) text that visible text covers more than half of. A mark that might cover but is
translucent or clipped keeps the text and is counted `maybeCovered`; an exhausted cover budget is `coverUntested`, never
`coveredText`. `/api/pagemap` is untouched (the new fields are not in `MapText`/`MapShape`).

**Tree or proposal, per page (D17).** A page takes the tree's blocks only when the tree's words equal the page's visible
words as a multiset (running headers and footers aside); otherwise it takes the proposal and is flagged
`tree-incomplete`. Equality both ways is the safe test: a word the tree holds that the page does not show is the
covered/redacted case. The tree route borrows style and links from the page's paragraph with identical words.

**A page that is only a picture (D20)** exports as that picture, "Scanned page N", and is listed in `ScannedPages`.

**Declared:** `/AS`, `/Usage /View` and an OCMD `/VE` are not read (a layer they hide is read as visible); one covering
mark must hold a run on its own; annotation text and `/Redact` rectangles are not read; the structure tree is a second
read of the file (`ReadStructure`) beside the validated one; the per-page wall time on a large corpus (W7) and the
tree-incomplete rate on real tagged documents were NOT measured here (no corpus in this slice).
