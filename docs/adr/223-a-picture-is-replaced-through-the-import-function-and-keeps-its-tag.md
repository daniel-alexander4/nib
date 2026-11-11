# ADR-223 — A picture is replaced through the import function, and keeps its tag and its place

Status: accepted, 2026-11-10 (PLAN-image-edit P03.S01; D9). Extends ADR-219, ADR-221, ADR-185.

## Context

A replacement picture arrives as a file the user picked. `pdfops.Import` is the one function a file becomes a PDF
through (ADR-185), and it returns a PDF it is given as it came, before it classifies anything, so a PDF named
`x.png` would carry every filter and key its author chose into the document. No function in the tree copied an
object between two documents: `copyObject` copies within one and refuses a stream.

## Decision

1. **The format is admitted inside Import** (`ImportOptions.Only`, two checks: the PDF passthrough and after
   classification), with `Single` (one page, from the frame count before any decode) and `MaxPixels`. A replacement
   is imported with `ReplacementImport`: `RasterFormatKeys()` (an image row with no outside tool), one page,
   `ReplaceMaxPixels` = 50 MP (measured: a 48 MP PNG costs 6.5 s and 1.28 GB through the producer, a JPEG
   milliseconds). The route reaches Import through `importOrRefuse`; the CLI through `pdfops.Import`.
2. **The document's own refusal runs before the import** (`objedit.Check`), at the route and the CLI, so a signed
   document's upload is never read.
3. **The door lifts one image** (`liftImage`, `copyImageStream`): Import's one-page result must name exactly one
   image; it is copied as new objects with the kept entries, the raw bytes and at most one `/SMask`, and any other
   reference refused. The old operator becomes `/NibRn Do` at its span; the new name is entered after `pruneOnePage`
   gave the page a fresh `/Resources` of its own, so a page sharing the old dictionary never sees it.
4. **Placement**: a movable picture (ADR-221) is fitted inside its old box keeping the new proportion, centred, with
   the EXIF turn Import put on its page's `/Rotate` folded in (`placementTurned`); otherwise the old `cm` is kept,
   the answer says `stretched` past 1%, and a turned photo is refused `orientation`.
5. **The tag stays.** `Replaceable` is `objectEditability` without the tag causes; a described Figure needs a new
   description (written on the element in the same rewrite) or `keepAlt`, else `alt-unanswered`. A stencil
   (`image-mask`) and an image the structure refers to (`object-referenced`) are refused.
6. **The after-check** holds the replaced slot to its box, to a `Do` of the new name (`painter`) and that name to the
   lifted bytes; each refuses a wrong picture alone.
7. **One request shape**: `POST /api/page/objects/replace`, multipart `file` and `edits` (the JSON `DecodeEdits`
   reads, one `replace`); the request-field census follows `r.FormValue` into the decoder.

## Consequences

- Five causes join the table (22): `image-mask`, `object-referenced`, `orientation`, `alt-unanswered`, `not-one-image`.
- The corpus instrument gains `OBJECTREPLACE` (one replace a page, inside the old box, the page otherwise unchanged)
  and a floor; `TestNoNewImageDecoder` pins every picture decoder by file.
- Declared: the stretch notice compares the box's proportion, exact for a picture drawn straight and approximate for
  a turned one; a turned photo on a picture that cannot be moved is refused rather than re-encoded.
