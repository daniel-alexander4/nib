# ADR-187 — An archive is opened through one door, and a cap refuses

**Status:** accepted
**Date:** 2026-11-02
**Context:** `PLAN-import-export.md` P01.S03a (D14, D32, R8, Y6, W17); extends ADR-185, ADR-009.

## Decision

- **`internal/ziputil` is the one door a zip is read through, and the one a stored-first archive is written through.** `Open` holds an archive to its `Limits` before any member is decompressed — entries, summed declared size, the expansion ratio above a floor, duplicate and unsafe names — and treats an error from `zip.NewReader` as a refusal even when it returns a reader (`ErrInsecurePath`). `Read` is capped on the REAL stream with an `io.LimitReader`; a central directory's sizes are a claim. A `Limits` with any cap unset is refused: no door can forget one. A package an outside converter reads by itself (a DOCX written by an old tool with backslash names) sets `TolerateUnsafeNames`; nothing Nib reads by name does.
- **A census over `archive/zip` constructors** (`TestEveryArchiveIsReadAndWrittenThroughTheDoor`, parsed not grepped, with a planted-caller red proof) fails a file that opens an archive itself. Six writers predate the door and are named with reasons; the list can only shrink, and no non-fixture file may READ an archive except through `Open`.
- **A cap refuses and never truncates.** A 1,001-page TIFF, a 1,001-frame GIF and a 1,001-picture CBZ are refused by name (`ErrTooManyPages`), because one that opened as 1,000 would lose a page without saying. The pixel caps are a running sum charged from the header before each decode (`ErrImagePixels`).
- **A TIFF's directory chain is walked by Nib** (`walkTIFF`: bounds-checked, each offset visited once, stopped at the page cap) because `x/image/tiff` decodes the first directory only; each page is served to the decoder through a reader that overlays the header's first-directory offset. Reduced-resolution and mask directories are not pages. A compression with no decoder (JPEG-in-TIFF) is `ErrTIFFCompression`.
- **A CBZ page is known by its bytes, never its name**; members are ordered naturally (page2 before page10); non-pictures, `__MACOSX/` and dot-files are not pages; an unsafe or repeated name refuses the whole archive.
- **Native producers carry their own wall-clock budget** (2 minutes, checked between pages), since they never wait at the gate.

## Declared limits

The cap VALUES (1,000 pages, 1 Gpx total, 64 MiB a CBZ picture) are provisional until P01.S03b measures them. GIF frames are composited with disposal methods 0–3 but a frame's local colour-table transparency is the library's. A TIFF's `Orientation` and a BMP's/TIFF's colour profile are not read. `zip64` is not specially refused: the 200 MiB input cap and the 1 GiB decoded cap sit far below it.
