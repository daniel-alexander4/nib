# PLAN — import and export for every type but PDF

**Dateline.** Seeded 2026-11-01 from Dan's `/discuss` of `/pending 906` (read other formats) and `/pending 912` (export a
PDF as Markdown, DOCX or EPUB), which it replaces as the plan of record: **where those two entries and this plan differ, the plan
wins.** The foundational choices below (D1–D3, D6) are Dan's words; D4, D5 and D7 are tagged `assumed`. Facts are from the entries,
`instruments/open-as-is.md` (906's seam inventory, written for the model this plan reverses, reused where it still holds) and
the code at v2.16.1, read for this plan.

**Stage 2 grill, 2026-11-01 — amended.** Five fresh agents (browser render, open doors and Open Recent, Export card and ADR-103, document-model
cost, Tier 2/3 tools) checked the plan against the code and this machine; four claims cracked and four slices were mis-scoped. Pins below are
tagged `(grill pin, 2026-11-01)`. Seam inventory: `memory/instruments/import-export.md`.

**Status: unbuilt. P01.S01 is next.** Stage 3 (subsystem rounds: the import door, the document model, the Export card) is the next pass.
**Passes:** Stage 2 whole-plan `/grill` 2026-11-01 (amended); Stage 3 round 1, the import door, 2026-11-01 (D8–D15; the other two rounds — the document model, the Export card — remain).

**SME packs:** `go`, `verification` (inferred from `go.mod` and the repo's census guards).

---

## What this is

Nib opens PDFs, four image formats and, through *Open & convert to PDF…*, Markdown (natively) and Office files (through
LibreOffice). It exports pages as images, a text dump, one page's table as a spreadsheet, and form data. It has **no way to
write a PDF's content back out as a document someone can edit**, and it reads few of the formats people are handed.

**The model, Dan 2026-11-01:** a PDF is the only document type Nib works on. Everything else comes in through **Import** and goes out
through **Export**; **Save** and **Save As** write PDF and nothing else. Where it can, Export writes a document the recipient can
**edit**, not a picture of one.

## Repo laws for this plan

- **L1 — The original is never written.** Importing reads a file and makes a PDF; no route writes, replaces or attaches the source
  (Dan 2026-10-19: "No attachment"). A census hashes the source before and after driving every route.
- **L2 — Save writes PDF.** Any other type leaves through Export. A tab made by Import has no path, so Save is Save As.
- **L3 — Export takes a PDF, and only a PDF.** One button per THING exported, the format chosen on the Save dialog's Format line
  (ADR-103); never a button per extension.
- **L4 — A format that needs an outside tool lights up only when the tool is found** (ADR-040's door, `toolpath.go`), and says
  what is missing in its words. Nothing is bundled. A format with no tool never silently degrades.
- **L5 — One door per rule (ADR-009).** One format table, one import door, one export door; a census holds each.
- **L6 — An export says what it keeps.** A scanned page is only editable after OCR (`ensureText`, ADR-106); fidelity is best-effort and
  the menu says so rather than the recipient finding out.
- **L7 — Conversion is never on a request's hot path.** It runs in the operation that asked for it, bounded by caps (pixels,
  archive entries, decoded bytes, pages, nesting) and a timeout on every subprocess.

## What exists today (read at v2.16.1)

- The open door: `asOpenableDocument` (`internal/server/imageopen.go`) is the one decision four routes share (path, upload, fetch-by-URL,
  OS hand-off); an image becomes a pathless one-page PDF (`pdfops.ImageToDocument`), Markdown renders natively and Office goes through
  LibreOffice (`pdfops.ConvertDocToPDF`, `SupportedDocExt`).
- The export surface: the **Export & Print** card (`web/index.html:351`): Print, Pages as images, Pictures in the document (ZIP),
  Document text, This page's table (XLSX / ODS / CSV, written by `internal/pdfops/spreadsheet.go`), Form data, and the two splits.
- The readers a document model composes: reflow's paragraph grouping (`grouping.go`, ADR-155/161), the tagger's heading / list / table /
  figure proposal (`ProposeTags`, ADR-121..123), the page map's text runs and ruled grids (ADR-088), and a tagged document's own
  structure tree.
- **Absent, by search:** no DOCX, ODT, PPTX, EPUB, HTML, Markdown or RTF *writer*; no EPUB, HTML, SVG, PostScript, HEIC, email or comic-archive
  *reader* (`grep -rli epub internal` finds none; the office list is LibreOffice's).

---

## Decisions

### D1 — Open, Import, Export; PDF is the only document *(Dan, 2026-11-01, /discuss)*
Opening, dragging or double-clicking a **PDF** opens it. Any other supported type — images included — is **imported**: it becomes a
PDF tab. Save and Save As write PDF only. Every other output is an Export. This supersedes 906's "every format opens as itself",
its read-only non-PDF tab and `FOREIGN_ALLOWED` allowlist, and its "Save As never changes the type"; it keeps 906's *Import* /
*Export* naming and its move of *Split by bookmarks* / *Split by page range* from Export to Page Functions (they write PDFs).

### D2 — The format set *(Dan, 2026-11-01; Tier 3 chosen by Dan)*
**Import (→ PDF).**
- **Tier 1, native:** PNG, JPEG, TIFF (all pages), WebP, GIF, BMP; Markdown; plain text; CSV (as a table page); CBZ (a zip of images).
- **Tier 2, through an optional tool:** LibreOffice — Word, Excel, PowerPoint, OpenDocument, RTF, Pages / Numbers / Keynote, Visio,
  Publisher; Ghostscript — PostScript, EPS; a HEIF/ImageMagick tool — HEIC, HEIF (`/pending 911`); a Chromium-family browser — saved web
  pages (HTML, MHTML) and EPUB; email — `.eml` and Outlook `.msg`, parsed by Nib, the body through the browser path.
- **Tier 3:** SVG, rendered by a sandboxed renderer — no script, no external fetch.

**Export (PDF →).**
- **Tier 1, Nib's own writers:** images (PNG, JPEG, TIFF, WebP, GIF, BMP; multi-page TIFF; CBZ), plain text, Markdown, HTML, EPUB, DOCX,
  ODT, and tables as XLSX / ODS / CSV and form data as CSV / JSON / XFDF (both exist).
- **Tier 3:** PowerPoint (PPTX) and SVG.

**(grill pin, 2026-11-01)** HEIC/HEIF: `heif-dec` or `heif-convert` first, `magick` second, and the door probes that the tool can READ HEIC
(many ImageMagick builds cannot); its PNG output re-enters `ImageToDocument` and takes the 200 Mpx cap. None is installed on the dev machine.
`.eml` is standard library; Outlook `.msg` is an OLE compound file with no parser in `go.mod`, so a hardened reader or a dependency is decided at
P04.S05's grill. SVG import renders through the browser door (D5), not a separate renderer.

**Dropped by Dan, 2026-11-01:** DjVu, XPS, FictionBook, Mobipocket, CBR; and, from the "left out" list, editable Word from a scan, CAD, Kindle's
own formats. Nothing here is a promise to add them later.

### D3 — Export editable documents wherever possible *(Dan, 2026-11-01)*
- **DOCX / ODT:** flowing text — real paragraphs, headings, lists and tables — from the document model (D4). **EPUB / HTML / Markdown:** reflowable.
- **PPTX (Tier 3):** one slide per page, positioned editable text boxes and the pictures placed, from the page map.
- Where an export has both an editable form and an exact-layout form, the Format line offers both and names the trade-off.
- A scanned page is exported as an image unless it was read first (L6).

### D4 — One document model, three writers' worth of readers *(assumed, 2026-11-01)*
The editable exports share **one reader** that composes what already exists: a tagged document's structure tree first, else the tagger's
proposal and reflow's grouping, with the page map's grids for tables. The writers (Markdown, HTML, EPUB, DOCX, ODT) consume the model and
never read a PDF themselves. *Why assumed:* five writers each reading the page differently would give five different documents of one PDF.
Merged cells and multi-column reading order are the declared hard cases.
**(grill pin, 2026-11-01)** The model is NOT only a composition of what exists. `ReadStructure`, `ProposeTags` and `DocumentText` compose, but four
inputs are missing and are P02.S03's own content: font style (`MapText` drops `font`/`baseFont`), links (no exported reader), image bytes
(only `ExtractImagesZip` exists) and paragraph grouping (`Paragraphs` returns text only; `groupRunsAndTables` is unexported). It reads in ONE pass
(`mapPageIn`, `DocumentText`'s shape) — a `MapPage` loop measured 0.63 s/page, ~6 min on 559 pages, against 2.0–5 s for the one-pass forms.
Scans and image-heavy files are unmeasured. Until P02.S03 supplies links and font style, an export says what it drops (L6).

### D5 — Outside tools sit behind the one door *(assumed, 2026-11-01)*
`magick` / a HEIF decoder, Ghostscript, LibreOffice, `pdftocairo` and a Chromium-family browser join `toolpath.go`, `missingtool.go` and ADR-040's
wording.
**(grill pin, 2026-11-01) The browser question is settled.** A Chromium-family browser renders a local HTML file to PDF headless in 0.7–1.5 s
(`--headless --print-to-pdf --no-pdf-header-footer`, a fresh `--user-data-dir`, `--host-resolver-rules="MAP * ~NOTFOUND"`). Scripts are made inert by a
CSP `<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; img-src data:">` that Nib injects; it also
stopped the network fetches (a loopback listener saw none). `--blink-settings=scriptEnabled=false` wrote NO PDF — do not use it. Firefox has no
headless PDF flag, so the door is Chromium-only (`browser.chromiumCandidatesFor` already excludes Firefox) and a Firefox-only machine gets the
missing-tool sentence (L4). HTML is built first; MHTML and EPUB need Nib to unpack and rewrite before rendering and are decided at P04.S04's grill
with a probe. A render fixture proves the script and the fetch do NOT happen, and fails if the CSP is dropped.

### D6 — Opening a non-PDF imports it, once, with a notice *(Dan, 2026-11-01)*
Open, drag-drop, double-click and the OS hand-off run the import and say so: *"Imported report.docx as a PDF. Save writes a PDF; use Export for
other formats."* The **Import…** entry stays for the choices an import has (document language, ADR-033's `LabelUA`). The result is a
pathless PDF tab (L2). *Assumed with it:* Open Recent records the source path and reopening imports again (906 P15); this couples to
`/pending 490` (typed entries).
**(grill pin, 2026-11-01)** Already true for a path-opened image (`handleOpen` records the source after the `converted` branch, `server.go:980`); the
other doors do not record and `/pending 490` owns that one door. This plan makes NO vault `Contents` change. Resume (`recordLastOpen`,
`lastopen.go:32`) skips pathless documents, so an imported tab is not resumed — declared gap. Open now refuses Office at the upload and URL doors
(`convertibleRefusal`); the import function removes that.

### D7 — ODT and SVG export are in the set *(assumed, 2026-11-01)*
ODT mirrors the XLSX / ODS pairing the table export already has and costs one writer over D4's model. SVG export (a page as SVG) follows Dan's rule
that every type but PDF goes both ways. Either is dropped on a word from Dan.
~~through Ghostscript's SVG device~~ **(grill pin, 2026-11-01)** Ghostscript 10.02.1 has no SVG device (`gs -h` lists none; `-sDEVICE=svg` is unknown).
SVG export runs `pdftocairo -svg` (poppler; present on the dev machine, not yet run for this plan) as a tool behind the one door (D5, L4), one page
per file; with it absent the format is not offered.

### Stage 3, round 1 — the import door *(via /discuss, autonomous, 2026-11-01; facts read from `imageopen.go`, `office.go`, `pdfops/office.go`, `handoff.go`, `instance.go`)*

### D8 — One import function, classified by content first *(auto-adopted)*
`pdfops.Import(ctx, data, name, ImportOptions) (Result, error)` replaces `asOpenableDocument`'s `openability` enum, `handleOffice`'s body and `cmdOffice`'s. PDF, image
and archive kinds are decided by their bytes; the extension decides only what bytes cannot say (Markdown, text, CSV, the Office family). Every door (path, upload,
URL, hand-off, `combine.go:36`, `diskstate.go:257` through `readInstallablePDF`) calls it and no door keeps its own refusal; `convertibleRefusal` goes. One refusal
sentence per cause, from the function. *Why:* ADR-009 — today the same Office file is converted at one door and refused at four, and `handleOffice` and `cmdOffice` each carry their own copy of the language and PDF/UA handling.

### D9 — An import runs on the request's context, two at a time *(context auto-adopted; the cap assumed)*
`ConvertOfficeToPDF` runs on `context.Background()` (`office.go:203`), so a closed window leaves LibreOffice running for up to its 2-minute timeout, and nothing
bounds how many run at once (eight double-clicked `.docx` files start eight). The import takes the request's context, which kills the process tree through
`runConvert`'s existing path, and a process-wide gate admits two tool conversions at a time, the rest waiting visibly ("Importing…"). `roomToOpen` is asked before the
conversion, as `handleOffice` already does. *Cap of two assumed:* LibreOffice's per-run profile (`office.go:207`) makes more than one safe; two is a judgment, to be measured at P01.S02.
The hand-off client's 30 s wait (`instance.go:447`) is unchanged: an import that outlasts it is already served to completion there (`instance.go:438`) and the window shows the tab.

### D10 — Plain text and CSV are native; Markdown's renderer sets them *(Dan's tier in D2; mechanism assumed)*
`txt` and `csv` leave `OfficeExtensions` (they go through LibreOffice today). Text is set as preformatted wrapped lines and CSV as a table across pages, both by `mdpdf`
with Nib's faces (`ConvertDocToPDF`'s Markdown path), the CSV delimiter sniffed (comma, semicolon, tab). *Assumed:* reusing mdpdf over a second renderer keeps one set of fonts, `embeddedFontsAreHonest` and tagging.

### D11 — An imported tab is named `<base>.pdf`, titled from the source *(auto-adopted)*
Images keep `scan.png` as a name today (`server.go:969`, `handoff.go:204`) and Office becomes `.pdf` (`office.go:104`); the Save As suggestion then differs by door. One rule in the function:
base + `.pdf`, `TitleFromName` from the source name, best-effort as now. `importedFrom` (the source basename) rides on the document.

### D12 — The notice is said once per window per document *(auto-adopted)*
The window announces an imported document the first time it sees a document id carrying `importedFrom` (ids are never reused, ADR-001), remembered in that window's `sessionStorage`.
A reload, a tab switch or a second window does not repeat it; a hand-off arrival (ADR-086) says it the same way, because the keying is the document, not the door.

### D13 — Options travel in `ImportOptions`, and the function applies them *(auto-adopted)*
`Lang`, `LangChosen` move from `handleOffice` (`office.go:82-100`) and `cmdOffice` into the function: `SetLang`, then `LabelUA` only for Markdown with a CHOSEN language (ADR-033), unchanged. The
automatic open sends neither, so it declares no language and claims no label, as `/api/office` without `lang` does today. *Import…* keeps the language field.

### D14 — One limits struct, every cap a named constant with its test *(starting values assumed)*
`importLimits` holds: input bytes (`maxPDFBytes`, 200 MiB, existing), image pixels (`maxImagePixels`, 200 Mpx, existing), and new ones — archive entries 2,000, decoded
bytes 1 GiB, pages 1,000 (TIFF and CBZ), text 20 MiB, CSV cells 1,000,000, with a ratio bound on any archive. Each refuses by name (L7, P5 in the inventory). *The new values are
unmeasured guesses* and are tuned at P01.S03 against a measured run; the output must still pass the byte cap (ADR-008) at `addDocCapped`.

### D15 — `nib import` is the command, `nib office` its alias *(auto-adopted)*
`nib import IN -o OUT [--lang TAG]` calls the same function and prints the same missing-tool sentence with the vendor URL (`cmdOffice`'s today); `nib office` stays as an alias so no script breaks.
The unprintable-character warning (`UnprintableMarkdown`) moves into the function and is returned, so the window and the command both show it.

---

## Out of scope

- Everything in D2's "dropped" list. Pixel editing of an imported image (mark-up and redaction work on the PDF it became).
- Round-tripping: an exported DOCX re-imported is a new import, not a recovery of the original PDF.
- Printing to PDF from other programs (`/pending 902`) and scanning (`/pending 903`) — separate items.
- PDF/A and PDF/UA claims for an *imported* document beyond what ADR-032/033 already decide.

## Standing caveats

- Fidelity of any PDF → editable export is **inferred from layout**; no output is "the original document". Judge DOCX / ODT by opening them in
  LibreOffice (an instrument, not a tier; Word itself cannot be driven here), EPUB with `epubcheck`, PPTX by LibreOffice Impress.
- LibreOffice, Ghostscript, a HEIF tool and a browser are not bundled and may be absent on any machine; every Tier 2 row has a missing-tool test.

---

## Build order

### P01 — The import door
Goal: one format table and one import door, so every type but PDF arrives as a PDF tab the same way, and Tier 1 works with no outside tool.
Exit criteria:
- Every Tier 1 import type opens (drag, path, upload, hand-off) as a pathless PDF tab with the notice; the original is byte-identical afterwards (L1's census).
- No list of extensions exists outside the format table (a source census, L5).
- `nib import IN -o OUT.pdf` converts the same set (`nib office` becomes its alias).

#### P01.S01 — The format table
Scope: one table in `pdfops` — sniff, extensions, MIME, producer, tool, import and export flags — read by the open door, `listDir` filters, the picker's `accept`, and the `.desktop` MimeType. Refs: D2, L5.
Acceptance:
- `SupportedOfficeExt`, `SupportedMarkdownExt` and the image sniff lists are views of the table; `TestTheFormatTableIsTheOnlyList` fails on a second list.
- A format with no producer on this machine is listed with its missing tool, not omitted.
- **(grill pin, 2026-11-01)** The table is served to the page and replaces the three `accept` literals (`app.js:597-599`, `index.html:280`, `:1986`),
  `listDir`'s PDF-only filter (`saveas.go:116`) and `build/nib.desktop`'s `MimeType` (generated); the census scans `web/` and `build/` as well as Go.
  Nine sites list extensions today (`office.go:55,75`, `imagedoc.go:83` by magic, the three literals, `listDir`, `.desktop`).
Tasks: *(written at slice-grill time)*

#### P01.S02 — Open imports
Scope: the four doors route a non-PDF through one import function; the notice; the Import… entry keeps options; Open Recent records the source (D6). Refs: D1, D6, D8, D9, D11, D12, D13, D15, L1, L2.
Acceptance:
- Each door imports the same file to the same bytes; the notice names the file; Save on the result is Save As.
- A census drives every route against an imported tab and hashes the source file before and after.
- **(grill pin, 2026-11-01)** `document` gains an `importedFrom` marker (it has none, `server.go:111-123`) carried on `docResponse` for the notice;
  ONE import function serves all callers of `asOpenableDocument` (`server.go:914,1014`, `sources.go:77`, `handoff.go:176`, `combine.go:36`;
  `diskstate.go:257` reaches it through `readInstallablePDF`), with a per-caller census; ONE naming rule (images keep `scan.png` today, Office
  becomes `.pdf`). Counters: `import.ok{door,format}` and `import.refused{cause}`, atomics only — the open routes are request handling and this
  approval is the hot-path check-in.
Tasks: *(written at slice-grill time)*

#### P01.S03 — Tier 1 native producers
Scope: GIF, BMP, multi-page TIFF, plain text, CSV, CBZ producers, with caps (pixels, entries, decoded bytes, pages) and one test per cap. Refs: D2, D10, D14, L7. Tunes D14's unmeasured values against a measured run.
Acceptance:
- Each producer's output opens and reads back its content; each cap refuses by name.
Tasks: *(written at slice-grill time)*

#### P01.S04 — `nib import`
Scope: the command, generalising `nib office`. Refs: D2, D15.
Acceptance:
- Same set as the window; a missing tool exits non-zero with the missing-tool sentence.
Tasks: *(written at slice-grill time)*

### P02 — The Export surface and the document model
Goal: the Export card reshaped to L3, image exports widened, and the one document model (D4) built, with the first two writers (Markdown, HTML) as its proof.
Exit criteria:
- Export takes a PDF only; Split by bookmarks / page range live in Page Functions; one button per thing exported, the format on the Format line.
- Pages as images offers PNG, JPEG, TIFF (multi-page), WebP, GIF, BMP and CBZ.
- Markdown and HTML export a tagged and an untagged document with headings, lists and tables kept; a scanned page goes through `ensureText` first or is refused with its reason.
*Slices sketched:* S01 Export card + Split move (ADR superseding 906's definitions); S02 image formats; S03 the document model; S04 Markdown + HTML writers.
**(grill pin, 2026-11-01)** S01 is half done: ADR-103 already gives one button per thing and a Format line. What remains is moving the two splits and
widening the image row; a new ADR supersedes ADR-103's "Pages as images" row and its "Export & Print is now eight" sentence, and the same slice edits
the tests that pin the card (`filemenu.test.mjs:79-89`, `toolbargroups.test.mjs:170,214`, `saveformats.test.mjs:326-507`, `racepins.test.mjs:192`,
`app.js:14452-14462`). S02: the browser renders PNG as now (pdf.js, `app.js:10027`); `/api/assemble` transcodes to GIF, BMP, CBZ and multi-page TIFF
(a hand-built IFD chain — `x/image/tiff` writes one image per file); Go has no WebP encoder, so WebP uses canvas `toBlob('image/webp')`, offered only
where the browser can encode it and said so on the Format line. S03 carries the four missing inputs named in D4.

### P03 — Editable documents
Goal: DOCX, ODT and EPUB writers over the model, judged by instruments, with flowing and exact-layout forms where both make sense.
Exit criteria: each opens in LibreOffice with headings, lists, tables and pictures intact on the fixture corpus; EPUB passes `epubcheck` where it is installed and Nib's structural test (stored-first `mimetype`, `container.xml`, nav) everywhere — `epubcheck` is absent on the dev machine; the menu states what each keeps (L6). One zip helper (`GridToODS`'s stored-first `CreateRaw`, `spreadsheet.go:64`) serves ODT, EPUB, DOCX, PPTX and CBZ, held by a census.
*Slices sketched:* S01 minimal OOXML writer; S02 ODT; S03 EPUB; S04 flowing-versus-exact choice and the honest labels.

### P04 — Tier 2 imports
Goal: the formats that need a tool, each behind the one door.
Exit criteria: each Tier 2 type imports where its tool is present and names the missing tool where it is not; HEIC, saved web pages / EPUB, PostScript / EPS and email each have a fixture and a cap.
*Slices sketched:* S01 LibreOffice breadth; S02 Ghostscript PS/EPS; S03 HEIC/HEIF (`/pending 911`); S04 the browser as renderer (D5 settled; HTML first, MHTML / EPUB route decided at its grill by probe); S05 `.eml` / `.msg` (`.msg` needs an OLE reader, see D2). S03's HEIC tool order and capability probe are in D2.

### P05 — Tier 3
Goal: SVG in and out, PPTX out.
Exit criteria: an SVG with a script or an external reference imports with both inert (through the browser door); a page exports as SVG where `pdftocairo` is present; a PPTX opens in LibreOffice Impress with one slide per page, editable text boxes and the pictures placed.
*Slices sketched:* S01 sandboxed SVG; S02 SVG export; S03 PPTX writer from the page map.

### P06 — Close
Goal: documentation and the censuses that keep the set honest.
Exit criteria: README, the web page and the CLI table describe the set; one census asserts every row of the format table has an import or export test, a cap and a missing-tool test; `/pending 906`, `912`, `911` closed or re-pointed here.
