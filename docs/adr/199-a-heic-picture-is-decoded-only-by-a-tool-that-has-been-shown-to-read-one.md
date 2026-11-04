# ADR-199 — A HEIC picture is decoded only by a tool that has been shown to read one, and its own declared size is read before any decoder runs

Status: accepted, 2026-11-03 (PLAN-import-export P04.S02b, v2.41.0). Extends ADR-185, ADR-198; D2, W2.

**The door.** One table row, `heic` (extensions `heic`, `heif`; MIME `image/heic`, `image/heif`; Tool `ToolHEIF`;
`FamilyHEIF`), no `Door` flag. The family is sniffed from an ISO base-media `ftyp` box: a major or compatible brand in
`heic heix hevc hevx heim heis mif1 msf1` and none of `avif avis` (the minor version in bytes 12–15 is a number and not a
brand; an AV1 picture is not claimed, though libheif can decode it). A picture is a picture whatever it is called, so a
HEIF under `.jpg`, or under no name, opens as HEIC; under any other document's name, or `.heic` over bytes that are not
HEIF (MVG text included), it is `ErrExtensionDisagrees` and no tool is started. `convertImport` calls `heicToDocument`:
decode to a PNG, then `ImageToDocumentCtx`, so the PNG takes the same 200 Mpx cap and one-page layout as every picture.

**Three layers, none trusting the others.**

1. *The tool is asked, once per binary, whether it reads HEIC* (`heifProbe`, cached by path, size and modification time,
   refusals included). `magick` on PATH is no evidence: many ImageMagick builds have no HEIC coder. The probe converts a
   1 KB HEIC Nib carries (`heifprobe.heic`, 64×48 shown, coded 64×64) with the same arguments as the conversion
   (`heifArgs`) and requires a 64×48 PNG back. The tools are tried in the plan's order, `heif-dec`, `heif-convert`,
   `magick`; one that fails is skipped for the next, and when none passes the refusal is `ErrHEIFCannotRead` (422,
   counter cause `tool-cannot-read`), not "could not convert" and not a missing tool. `/api/status` runs no probe
   (Y2): `HEIFAvailable` only finds a tool on PATH, and the window's `#heifMissing` notice is tied to it.
2. *The container's own sizes are read before a decoder runs* (`heifDeclaredSizes`, `heifWithinCaps`). Every `ispe`
   (image spatial extents) in the `meta`/`iprp`/`ipco` boxes is charged against the pixel cap through the raster job's
   `charge`, so a 65535×65535 picture is `ErrImagePixels` without a process started. The size is the CODED size, which
   is at least the size shown (the probe picture is coded 64×64). 20,000 × 10,000 is accepted and one row more is not.
3. *The tool runs with nothing to reach.* The input is alone in a directory Nib made under a fixed name, the output goes
   to another, standard input is the null device, the environment is `toolEnv`'s, and the gate's time and memory watchdog
   apply (`runConvertPick`, which `runConvert` now calls: the name of the output is asked after the process has ended,
   because libheif writes `out-1.png` where it was given `out.png` for a file with several pictures). ImageMagick is given
   the coder by prefix on both sides (`heic:in` `png:out`), so neither the name nor the bytes choose another one, and a
   `policy.xml` of Nib's (`MAGICK_CONFIGURE_PATH`) denies every delegate, coder, filter, module and `@`/`|` path, then
   allows the HEIC and HEIF coders to read and PNG to write. Later rules win, so the order is the point and a test
   holds it.

**Refusals, each named.** `ErrHEIFMissing` (the table's missing-tool refusal, classified by `MissingToolFor`),
`ErrHEIFCannotRead`, `ErrHEIFUnreadable` (the decoder ran and failed; its own last words are kept, 415 as a picture),
`ErrImagePixels`, `ErrConverterOverMemory`.

**Measured.** On this machine `heif-convert` 1.17.6 with libde265 1.0.15 reads the probe picture, and a file of two
pictures writes `out-1.png` and `out-2.png` and no `out.png`. 16 mutations of `heifimport.go` and `importsniff.go` were run
and every one turned a test red.

**Declared:**
- `magick` is not installed here. Its argument form and its policy are tested on a stand-in that records them, never
  against ImageMagick itself, and whether a given ImageMagick honours `{HEIC,HEIF}` braces in `pattern` is unmeasured;
  the capability probe is what would catch a build whose policy forbids the read.
- A file of several pictures is read at the first libheif lists, which is not necessarily the primary one; the rest are
  ignored. Rotation and mirroring are libheif's (`irot`, `imir` are applied by `heif-convert`); a stand-in cannot show
  it and no rotated fixture was made.
- Auxiliary images (depth maps), EXIF and XMP are not carried to the PDF.
- `ispe` is read from the container's header only; a file whose metadata cannot be walked reaches the decoder, which has
  its own limits and the memory watchdog behind it.
- Windows and macOS: `magick` has a Windows candidate glob; neither platform was run. HEIF and AVIF decoders may ship
  without the HEVC plugin (patent-encumbered): that is the probe's refusal, and the sentence says to install libheif's
  `heif-dec`.
