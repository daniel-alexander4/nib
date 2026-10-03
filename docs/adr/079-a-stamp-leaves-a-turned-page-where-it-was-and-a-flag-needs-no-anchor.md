# ADR-079 — a stamp leaves a turned page where it was, and a flag needs no anchor while every rewrite keeps its place

**Status:** accepted. `/pending 457` (2026-10-03). Applies ADR-009 to pdfcpu's stamp.

## Context

A signing flag (`NibFlags`, `internal/pdfops/flags.go`) is nib's one persisted, document-travelling page
coordinate: `{page, frac, type}`, a fraction of the page as the viewer shows it, with nothing saying what it was
placed beside. Every pdfcpu write carries the Info dictionary through, so a rewrite of a flagged document keeps
the flag at its fraction whatever the page now draws there. /pending 457 asked whether that needs a content
anchor — a format change to a blob in documents already sent — or whether the rewrites that reach a flag simply
move nothing.

Measured: the three routes the item named (`/api/ocr`, `/api/sanitize`, `/api/attachments/add`) kept the boxes and
rotation and rendered pixel-identical over twelve documents, with a crop, a turn and a visible stamp as controls
that all differed. A structural census over the same rewrites then found one shape that moved: **a page with a
`/Rotate` whose box does not start at the origin.** pdfcpu (v0.13.0, `stamp.go` `addPageWatermark`) folds the
rotation into the content to stamp a page — deletes `/Rotate`, swaps the box's sides, prefixes
`model.ContentBytesForPageRotation`'s matrix — and that matrix turns about the origin, not the box's corner.
Every pdfcpu stamp nib makes does this: the OCR layer, the bake's fields and images (every save), the watermark
and the page numbers. On a turned page cropped at (29.75, 84.2) the drawn text left the page.

## Decision

- **No anchor in the flag format.** Instead, every route that commits a rewrite is classified for its flags —
  keeps the drawn geometry, carries the flags (the reflow door, `anchormove.go`), never receives them (it rewrites
  posted bytes, and the client's bake strips `NibFlags`), or installs the file's own —
  `internal/server` `TestEveryRewriteOfAHeldDocumentIsClassifiedForItsFlags`; and every keeps-geometry rewrite is
  driven over flagged documents, a turned and cropped page among them, by `internal/pdfops`
  `TestEveryRewriteOfAHeldDocumentKeepsWhatAFlagWasPlacedBeside`. Its check reads the page's old operators in the
  new stream and maps three corners through the added matrices and the new box and rotation to where the viewer
  shows them, so a folded rotation passes and a misplaced one fails.
- **One stamp door, `stampInPlace`** (`internal/pdfops/stamprotation.go`). Every `pdfcpu.AddWatermarks*` runs inside
  it, and it rewrites pdfcpu's matrix as `T(ll) · R · T(-ll)`, turning the page about its own box's corner. It
  rewrites only a matrix it finds byte for byte, and refuses the stamp otherwise: a page pdfcpu could not patch has
  already lost its `/Rotate`. The two stamps that went through pdfcpu's `api` package (`StampImages`,
  `StampTextLayer`) now read and write through `addWatermarks`, the same configuration in one rewrite, because `api`
  stamps where the door cannot reach. `TestEveryPdfcpuStampGoesThroughStampInPlace` holds the routing.

## Consequences

- The anchor is owed the day a rewrite on a keeps-geometry route moves drawn content — the census turns red then.
  It is then an optional field, an old blob without it read as "unknown" (never invalid), every writer and the
  round-trip guard updated, and an ADR superseding this one.
- **Declared:** pdfcpu moves no ANNOTATION when it folds a rotation, so a widget, link or note on a turned page keeps
  a `/Rect` written for the old user space — at any box corner, the origin included (measured: a form's widgets at
  y=700 on a page now 595 high). `stampInPlace` does not repair it; it is filed separately.
- **Declared:** `StripMetadata` (`/api/sanitize?method=metadata`) drops `NibFlags` with the rest of the Info
  dictionary. That loses a flag rather than misplacing one, and is filed separately.
