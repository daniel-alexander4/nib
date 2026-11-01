# ADR-181 — Pictures are shrunk in place, and nothing else changes

**Status:** accepted
**Date:** 2026-10-31
**Context:** `/pending 908`. Optimize is lossless and does little for a scan; Compress re-renders every page as a JPEG and
loses text, search and links. Three requests in a month wanted a third thing: a smaller file with the text and colours kept.
**Applies:** [ADR-009](009-one-door-per-rule.md) (one door: `pdfops.ShrinkPictures`), [ADR-032](032-a-pdfua-identification-survives-only-what-nib-verified-any-change-drops-it.md)
(it rides the same rewrite, so a PDF/UA identification is dropped like any other change's).

## Decision

**An image XObject is replaced by a smaller JPEG of itself, and only when that is safe; no other object is read or written.**
The pass is Optimize's read-and-write with one step added (`shrinkImages`); text, vectors, links, forms and structure are
whatever that rewrite leaves.

- **The colours are the picture's own colour-space entry, and that entry is not touched.** An ICC profile, a CalRGB or a
  Gray stays in the dictionary; only the samples' number and precision change. "Without changing the colours" is therefore a
  property of the construction. Pictures in CMYK, indexed, Separation/DeviceN, JPEG 2000, JBIG2, CCITT, LZW, a filter chain or
  anything but 8 bits a sample are **left alone** — no encoder here keeps them honest.
- **A picture is skipped when its sample values are tied to something the re-encode does not carry:** a `/Decode` array, a
  colour-key `/Mask`, a stencil, `/SMaskInData`, a soft mask with `/Matte`. A soft mask keeps its own size (the specification
  lets it differ from its picture).
- **A flat-colour graphic is left exact.** An 8-bit Flate picture with 256 or fewer distinct colours in a sample is a logo,
  a chart or a screenshot, and a JPEG would blur its edges and shift its flat colours. Only a photograph is converted.
- **A picture is replaced only if the result is at least 10 % smaller,** and **a document that comes out larger than it went
  in is returned as the bytes that went in** (Optimize's own rewrite grew one real document 2 %).
- **A target size is a search, not a promise.** `ShrinkToSize` bisects a six-rung ladder (longest side 3200 → 700 px,
  JPEG quality 88 → 52) for the gentlest rung that fits, in at most three passes. Where none fits it returns the hardest
  rung's output and says so (`Met` false); the surface then names Compress as the way to go further and what that costs.
- **Every skip is counted by reason** (`ShrinkReport.Left`), so a pass that changed little can say why in words.

## Measured (2026-10-31, eight real documents of 2-23 MB from the local corpus, level 3)

7.6 → 3.2 MB (87 of 296 pictures), 18.7 → 6.4, 22.7 → 3.5, 2.3 → 1.3; one image-heavy file of 1,229 small pictures moved
5 % and one grew 2 % before the never-larger rule. Time 0.1-4.5 s each; every output read back through the same door.

## Consequences

Declared: the pass works from the bytes the server holds, like Optimize, so unsaved edits are not in the copy; a signed
document loses its signature as with every rewrite; colours are exact for the colour-space entry and approximate for the
samples (a JPEG), which is what the quality rungs trade; JBIG2 and CCITT scans — a large share of office scans — are not
touched and are the next thing a heavier slice would take.
