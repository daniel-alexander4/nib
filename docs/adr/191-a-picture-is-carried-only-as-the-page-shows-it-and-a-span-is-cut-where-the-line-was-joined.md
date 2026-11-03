# ADR-191 — a picture is carried only as the page shows it, and a span is cut where the line was joined

Status: accepted, 2026-11-02 (PLAN-import-export P02.S03b, v2.35.0). Extends ADR-122, ADR-155, ADR-190.

**Pictures.** `pdfops.ExportPages` (exportpage.go) gives each picture a page draws a Figure with its box in the map's
fractions and its place among the paragraphs (`tablePlace`). Its bytes are carried only when nothing about the dictionary or
the placement changes how the page shows it; otherwise it is a **placeholder with a named cause**: `stencil-mask`,
`masked`, `soft-masked`, `filter` (JPX, CCITT, JBIG2), `colour-space` (anything but Gray/RGB/Cal/ICC/Indexed), `inline-image`,
`turned-or-mirrored` (only an upright, unmirrored placement is the stored picture), `not-extracted`, `too-big`,
`over-budget`. **The cause is read from the image dictionary by the walker** (`imageCause`, flowregion.go), not from
pdfcpu's answer, because pdfcpu's extraction refuses a page over one stencil ("ColorSpace missing") and says nothing of
which picture. `drawnImage` gains `objNr`, `plain`, `cause` (additive; the proposers ignore them). Artifact images and
those under 8 pt are not figures and are counted (`SkippedImages`). A form's picture is found (ADR-122 refuses it for
TAGGING only). One object drawn twice is two figures over one buffer, extracted once. Caps: 32 MiB a picture, 256 MiB a
document, `maxImagePixels`; the size is read from the header, since pdfcpu's renderer gives bytes and no size.

**Declared:** a stencil on a page costs that page's other pictures (`not-extracted`) — pdfcpu cannot hand them over; no
`/OC` resolution (layers are the model's, S03c); an image inside a table cell is a figure after the table.

**Spans.** `ExportParagraph.Spans` are cut from the grouped line at the offsets `joinRuns` recorded as it joined each run
(`textLine.starts`), so no reader re-derives where a space went; their concatenation is `Paragraphs`' text. A line whose
cuts do not reproduce its text is one plain span, counted in `UnstyledLines`: the words are never wrong, only the style is
lost, and it says so. A span carries the link whose box holds the centre of the RUN it came from (ADR-090's rule); a link
over part of one run is therefore the whole run's or none — declared.
