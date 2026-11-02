# ADR-184 — The format table is the only list of the formats Nib imports

**Status:** accepted
**Date:** 2026-11-01
**Context:** `PLAN-import-export.md` P01.S01 (D2, L5). Nine places listed extensions or MIME types for what Nib opens or converts —
`office.go`'s `OfficeExtensions` and the Markdown test, `imagedoc.go`'s magic list, the Import picker's `accept` twice
(`index.html`, `app.js`), the Combine picker's `accept`, `listDir`'s PDF-only filter and `build/nib.desktop`'s `MimeType` — and they
had already disagreed: the picker's literal and `OfficeExtensions` were the same fourteen words typed twice, and the Open dialog
listed PDFs only while the routes behind it opened four image types.

## Decision

- **`pdfops.Formats()` (`internal/pdfops/formats.go`) is the one table**: a key, a label, extensions, MIME types, the tool the
  producer needs (`MissingTool`; ADR-040's door), and two flags — `Image` (read by magic bytes on every route) and `Door`
  (opened by every route as it stands, with no second step). `SupportedOfficeExt`, `SupportedMarkdownExt` and `SupportedDocExt` are
  views of it; `OfficeExtensions` is gone.
- **A row is added in the slice that builds its producer**, never ahead of it. Today's rows are what Nib converts today; GIF, BMP, CBZ,
  the text-native types and Group 2 arrive with P01.S03 and P04, each with its row. A format is never listed that a door then refuses.
- **The page reads it from `GET /api/status` (`formats`)** and builds both pickers' `accept` from it. The HTML carries no `accept` on
  either input. A tool is a key (`libreoffice`), never a link (ADR-039). The Import picker drops a format whose tool is absent; the
  table keeps the row, so a later notice can name the tool.
- **`listDir` offers PDF and every `Door` format**, so the Open dialog shows the images the routes open. `build/nib.desktop`'s
  `MimeType` is PDF plus every `Door` MIME type, held equal by `TestTheDesktopFileMimeTypeIsTheTable` (`NIB_REGEN_DESKTOP=1` rewrites it).
  Office and Markdown are not `Door` until P01.S02 routes them through the import door, so the desktop file does not advertise a type
  the hand-off refuses. **Linux only**: macOS and Windows file association ship nothing today, a declared gap.
- **The census is mechanical and has a red proof.** `TestTheFormatTableIsTheOnlyList` walks the Go sources, the page and `build/` and
  fails on three or more table tokens inside the string literals of six consecutive lines, on an `accept=` naming one, and on a
  `MimeType=` outside the desktop file; `TestTheFormatCensusFindsAPlantedList` plants each shape and requires it found. Six sites
  are named exemptions, each a different list on purpose: the two picture pickers (PNG and JPEG are what a page can embed), the CSV
  that fills a form, `embed.go`'s Content-Types for Nib's own static files, `mdpdf/packet.go` (a standalone package that cannot
  import `pdfops`), and the table export's output labels.

## Not decided here

- Magic-byte classification of every row (D32) is P01.S02a's; `sniffImage` stays the image sniffer until then.
- The per-tool missing notices (D24) and the Format-line reasons are the slices that own those surfaces.
