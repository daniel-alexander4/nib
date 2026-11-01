# PLAN — import and export for every type but PDF

**Dateline.** Seeded 2026-11-01 from Dan's `/discuss` of `/pending 906` (read other formats) and `/pending 912` (export a
PDF as Markdown, DOCX or EPUB), which it replaces as the plan of record: **where those two entries and this plan differ, the plan
wins.** The foundational choices below (D1–D3, D6) are Dan's words; D4, D5 and D7 are tagged `assumed`. Facts are from the entries,
`instruments/open-as-is.md` (906's seam inventory, written for the model this plan reverses, reused where it still holds) and
the code at v2.16.1, read for this plan.

**Stage 2 grill, 2026-11-01 — amended.** Five fresh agents (browser render, open doors and Open Recent, Export card and ADR-103, document-model
cost, Group 2/3 tools) checked the plan against the code and this machine; four claims cracked and four slices were mis-scoped. Pins below are
tagged `(grill pin, 2026-11-01)`. Seam inventory: `memory/instruments/import-export.md`.

**Status: unbuilt. P01.S01 is next.** Stages 1–7 have run (the dateline's Passes list); the planning arc is complete for this add-on plan, and `/createcode` takes it from P01.S01. There is no P00: nib is a shipped repository, so Stage 8's bootstrap does not apply. Code facts below were read at v2.16.1 and the `file:line` cites carry that date.
**Passes:** Stage 2 whole-plan `/grill` 2026-11-01 (amended); Stage 3 round 1, the import door (D8–D15); round 2, the document model (D16–D23); round 3, the Export card and the File menu (D24–D28), all 2026-11-01; Stages 4–6, 2026-11-01 (D29–D35, L8: flat spots, standards alignment, security / privacy, availability / scale / Go / verification; four slice cuts, and the browser door redesigned after a measured local-file disclosure); Stage 7 `/plan-review` 2026-11-01 (R1–R9, W1–W35, five slice merges, 19 → 14 headed slices; every finding adopted). **Stage 8 does not apply** (nib is shipped; there is no P00). The review was written by the session that wrote the plan, so the external pass is still owed: `memory/plan-reviews/2026-11-01-external-brief.md`.

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
- **L8 — Hot paths and conventions (Stages 4–6, STANDARDS §12).** The hot paths are the open routes, the OS hand-off and the window stream: nothing here adds work to them beyond atomic counters (`import.ok`, `import.refused`) and the one function call; conversion runs only in the operation that
  asked for it. The import and export routes are long operations, not hot paths. New refusals are sentinels read with `errors.Is`; every new goroutine starts with `safe.Recover`; the open and export paths log nothing. Each slice states its gate: it touches `internal/server`'s open and hand-off paths, not session, ceremony, delivery or discovery, `p2p` or `rendezvous`,
  so tiers 4 and 6 do not fire and the slice says so in one line; tiers 2 and 3 fire on every slice that changes the front end. `VERSION` is bumped in the slice's own commit (minor for a user-facing feature); `build/gen-notices.sh` runs if a module is added, and any new module is permissive and cgo-free; documentation is updated in the same change, not at P06. `ContentDigestVersion` is not touched by this plan.

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

### D2 — The format set *(Dan, 2026-11-01; Group 3 chosen by Dan)*
**Import (→ PDF).**
- **Group 1, native:** PNG, JPEG, TIFF (all pages), WebP, GIF, BMP; Markdown; plain text; CSV (as a table page); CBZ (a zip of images).
- **Group 2, through an optional tool:** LibreOffice — Word, Excel, PowerPoint, OpenDocument, RTF, Pages / Numbers / Keynote, Visio,
  Publisher; Ghostscript — PostScript, EPS; a HEIF/ImageMagick tool — HEIC, HEIF (`/pending 911`); a Chromium-family browser — saved web
  pages (HTML, MHTML) and EPUB; email — `.eml` and Outlook `.msg`, parsed by Nib, the body through the browser path.
- **Group 3:** SVG, rendered by a sandboxed renderer — no script, no external fetch.

**Export (PDF →).**
- **Group 1, Nib's own writers:** images (PNG, JPEG, TIFF, WebP — written by the window's canvas, Go has no WebP encoder —, GIF, BMP; multi-page TIFF; CBZ), plain text, Markdown, HTML, EPUB, DOCX,
  ODT, and tables as XLSX / ODS / CSV and form data as CSV / JSON / XFDF (both exist).
- **Group 3:** PowerPoint (PPTX) and SVG.

**(grill pin, 2026-11-01)** HEIC/HEIF: `heif-dec` or `heif-convert` first, `magick` second, and the door probes that the tool can READ HEIC
(many ImageMagick builds cannot); its PNG output re-enters `ImageToDocument` and takes the 200 Mpx cap. None is installed on the dev machine.
`.eml` is standard library; Outlook `.msg` is an OLE compound file with no parser in `go.mod`, so a hardened reader or a dependency is decided at
P04.S05's grill. SVG import renders through the browser door (D5), not a separate renderer.

**Dropped by Dan, 2026-11-01:** DjVu, XPS, FictionBook, Mobipocket, CBR; and, from the "left out" list, editable Word from a scan, CAD, Kindle's
own formats. Nothing here is a promise to add them later.

### D3 — Export editable documents wherever possible *(Dan, 2026-11-01)*
- **DOCX / ODT:** flowing text — real paragraphs, headings, lists and tables — from the document model (D4). **EPUB / HTML / Markdown:** reflowable.
- **PPTX (Group 3):** one slide per page, positioned editable text boxes and the pictures placed, from the page map.
- Where an export has both an editable form and an exact-layout form, the Format line offers both and names the trade-off. *(Narrowed by D21: one form per format, PPTX being the positioned one.)*
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
**(grill pin, 2026-11-01; mechanism superseded by D29 — the CSP-meta sentence and the single host rule below no longer stand; the Chromium-only door, the flag rejections and the fixture duty do) The browser question is settled.** A Chromium-family browser renders a local HTML file to PDF headless in 0.7–1.5 s
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

### Stage 3, round 2 — the document model *(via /discuss, autonomous, 2026-11-01; facts read from `tagreview.go`, `structview.go`, `pagemap.go`, `doctext.go`, `textrun.go`, `anchors.go`, `pdfops.go:2755`, `reflowdoor.go`)*

### D16 — The model is a package of data, and the writers cannot reach a PDF *(assumed)*
`internal/docmodel` holds `Document{Blocks}`: Heading(level), Paragraph, ListItem(level, marker), Table(rows of cells with column and row span, header flag), Figure(bytes or none, alt), each with its
page and place, and a paragraph's text as `Spans{Text, Bold, Italic, Link}`. `docmodel.Read(pdf, opts)` is the ONE reader; the writers (`internal/docwrite/<format>`) import the model and nothing
that reads a PDF, held by an AST census as `TestTheReadersRouteThroughThisDoor` holds ADR-052. *Assumed:* a separate package rather than `pdfops` keeps the writers testable on hand-built models.

### D17 — The author's tree wins; the proposal is the fallback, and the export says which *(auto-adopted)*
`ReadStructure(...).Tagged` with text on its elements → the tree is the structure (ADR-031: a document's own tagging is the author's statement); otherwise `ProposeTags` and reflow's grouping. The
result carries `Source: "tree" | "proposal"` so the export can say what it did (L6). A tagged document whose tree holds no text falls to the proposal.

### D18 — The four missing inputs, each an additive read *(assumed)*
- **Style.** Bold and italic come from the font descriptor (`/FontWeight` ≥ 700, `/ItalicAngle` ≠ 0, `/Flags` ForceBold / Italic), the base-font name as the fallback, looked up once per font. Exposed as `bold` / `italic` (`omitempty`) on
  `MapText`, an additive change to ADR-088's door — the detection readers ignore them and the JSON-name test is extended in the same slice.
- **Links.** A new `pdfops.PageLinks` reads `/Link` annotations with a `/URI` action or a page destination, as boxes; a span takes the link whose box holds the run's centre (ADR-090's rule, never overlap). An external URI is carried; an internal
  destination becomes plain text and is counted as dropped.
- **Images.** A Figure is joined to the image object drawn at its box and takes that object's bytes through `pdfread.ExtractImages` (the existing door, `pdfops.go:2766`), one object once. An image the writers cannot carry (a mask, JBIG2, an exotic colour space)
  becomes a placeholder with its description and is counted. Per-image and total byte caps are named constants (D14's shape).
- **Paragraph grouping.** Exposed as an exported read returning runs per paragraph with the bold / italic / link spans above; `Paragraphs` (text only, for the reflow editor) is unchanged.

### D19 — Reading order is the grouping's, and what it could not read is reported, never refused *(assumed)*
Columns and gutters come from reflow's grouping (ADR-155, ADR-161); a page the grouping reported (`TagProposal.Unsupported`) is exported in its best order and **named in the result** with its reason, so a two-column page that
came out wrong is a stated limit rather than a silent one (L6). No export is refused for it.

### D20 — A page with no text exports as its own picture, and the window reads it first when asked *(assumed)*
The server cannot rasterise a page (the browser does that), so a page with no text and no OCR layer becomes a Figure of the picture it already holds, described "Scanned page N". The window runs `ensureText` (ADR-106) on the
`unread` pages before the request, as it does for the table and text exports, and the document then carries real text. This replaces P02's "or is refused": an export of a scan is a document of pictures, and it says so.

### D21 — Flowing for every writer but PPTX, which is the exact one *(assumed; narrows D3's "both" to where both exist)*
DOCX, ODT, EPUB, HTML and Markdown are flowing only. PPTX is the positioned form (one slide per page, boxes from the page map). A DOCX of positioned frames is a third family for no reader who asked for it, so the Format line offers one form per
format and names it. P03.S04 (the choice) folds into the three writer slices as their labels.

### D22 — Every writer has a text oracle, or it is not done *(auto-adopted; the oracle's reference is superseded by D34 and R1 — the SOURCE PDF minus an independent visibility filter, not the model's text; without `soffice` the round trip is skipped and reported "not run", never the writer)*
Fidelity is judged by the words in order, not by looking: LibreOffice converts each exported DOCX / ODT / PPTX back to PDF, and `DocumentText` of the result must equal the model's text modulo whitespace; EPUB / HTML / Markdown are read back by their own parsers.
`build/exportfidelity.sh` (local corpus, counts only, beside `accuracy.sh`) prints it, and each writer's slice names its red proof (drop a table cell, reorder two paragraphs). An instrument, not a tier.

### D23 — One route, one pass, and the facts ride in a header *(auto-adopted)*
`POST /api/export/document?format=` is pinned to the document by `X-Nib-Doc` (ADR-004), reads the model in ONE pass (the 559-page file measured 2–5 s; a `MapPage` loop would be minutes) on the request's context, and answers the bytes with
`X-Nib-Export` carrying compact JSON — `source`, `droppedLinks`, `placeholderImages`, `flaggedPages`, `scannedPages` — as `X-Nib-Revision` does (ADR-072). A refusal is a 422 with a cause.

### Stage 3, round 3 — the Export card and the File menu *(via /discuss, autonomous, 2026-11-01; facts read from `web/index.html:276-362,466`, `web/app.js:9269-9330,10485-10540`, ADR-103)*

### D24 — Open & Import is one group, and a missing tool is said once per tool *(assumed)*
The File card's group "Open a Document" becomes **Open & Import**: *Open…*, *Open Recent…*, and **Import…** in place of *Open & convert to PDF…* (`officeOpenBtn`, same id, new label and title), with the language field beside it as now. The picker's `accept` comes from the
format table (P01.S01), not the literal at `index.html:280`. The single LibreOffice paragraph (`#officeMissing`) becomes one static paragraph per tool the table names (LibreOffice, Ghostscript, a HEIF decoder, a Chromium-family browser), each shown only
when that tool is absent AND the format table lists a format it would unlock; each carries its own static `href` authored in `index.html` (ADR-039/040: the remedy URL is never on the wire) and its own recheck.

### D25 — The Export card, final: one button per thing, formats on the Format line *(auto-adopted, ADR-103)*
*Print…*; *Pages as images…* — This page (PNG, JPEG, WebP, GIF, BMP, SVG), Every page (ZIP of PNGs, multi-page TIFF, CBZ); *Pictures in the document (ZIP)*; **Document as…** — Markdown, HTML, EPUB, Word (.docx), OpenDocument (.odt), PowerPoint (.pptx), Text (.txt);
*This page's table…*; *Form data…*. It replaces *Document text (.txt)*. Eight controls become six; the card's label stays **Export & Print**.

### D26 — The two splits move to Split Pages, with their ids *(auto-adopted, D1)*
`exportBookmarkSplitBtn` and `exportPageSplitBtn` move into the Page Functions group "Split Pages" (`index.html:466`) **keeping their ids**, so the `apiFetch`-gated and document-required id sets (`app.js:14452-14462`), `racepins.test.mjs:192` and the handlers are untouched; what changes is the group-membership pin
(`toolbargroups.test.mjs:170,214`) and the label order in `filemenu.test.mjs:79-89`.

### D27 — A format that cannot run is listed with its reason, never dropped *(auto-adopted, L4)*
A Format line entry whose tool is absent (SVG without `pdftocairo`) or whose encoder the browser lacks (WebP) stays on the line, `disabled`, its label ending in the reason ("needs pdftocairo", "this browser cannot write WebP") — text, not colour (WCAG 1.4.1).
Selecting nothing else changes. *Document as…* runs `ensureText` (ADR-106) before the request, as the text and table exports do.

### D28 — `.txt` comes from the model, and the dialog says what each format keeps *(assumed)*
Text (.txt) is written from the document model in reading order (D16), not from pdf.js's content-stream order (`documentText`, `app.js:10531`), which scrambles columns; `documentText` stays for Compare, which shares it, and is no longer an export. Below the Format line a note says what the
selected format keeps in one sentence (L6), and after the export the status line reports `X-Nib-Export`'s counts in words ("3 links to other pages became plain text; 1 picture could not be carried; page 4 was read as best it could"). *Assumed wording*, mechanism fixed: the note and the report are one text per format and one function, so they cannot disagree.

### Stages 4–6 — flat spots, standards, dimension reviews *(2026-11-01; four fresh reviewers read the plan cold — flat spots, STANDARDS.md, security / privacy, availability / scale / Go / verification; every remedy below that makes a claim about the browser was RUN before it was written)*

### D29 — The browser renders from a single-use loopback listener, not from `file:`, and D5's CSP-meta-only is superseded *(auto-adopted — measured)*
The reviewer's finding held when re-run here: a page under D5's exact CSP that carries `<meta http-equiv=refresh content="0;url=file:///…/secret.txt">` printed the sibling file's text into the PDF (`virtual-time-budget=3000`;
the CSP blocks subresources, not top-level navigation) — a local-file disclosure INTO the output. Three renders of the same hostile page: from `file:` the secret was printed; from a `data:` URL it was not; from a loopback HTTP origin it was not.
**The shape that makes it unrepresentable:** Nib serves the one sanitised document from a single-use listener on an ephemeral port under a random path (any other path is 404, and it closes after one request), the browser gets
`--host-resolver-rules="MAP nib-render.test 127.0.0.1:<port>, MAP * ~NOTFOUND"` — measured: the page loads, and a refresh to `http://127.0.0.1:<port>/…` is refused (an IP literal is not mapped) — and the CSP is a **response header**,
with `<meta http-equiv=refresh>` and `<base>` removed by an HTML parse as defence in depth. Flags: a fresh 0700 `--user-data-dir`, `--disable-extensions`, `--no-first-run`, `--disable-background-networking`, `--allow-file-access-from-files` never set, and **no `--no-sandbox`**
(snap-packaged Chromium may refuse a profile under `/tmp`: the door PROBES a real render before it offers a format, as D2 does for HEIC). MHTML / EPUB unpack **in memory**, rewrite resources to `data:` URLs and reject every other reference; no file is written under an attacker's name.
Fixtures: (a) the refresh-to-`file:` page — the secret must be absent; (b) the **positive control**, the same page with the protections removed, must print it, or the probe is dead; (c) a loopback canary that the CSP must keep silent and that fires without it. A source census over the
browser-argument door holds the flags (`TestTheBrowserIsLaunchedOnlyThroughTheDoor`), so the host rule and the header cannot be dropped. STANDARDS §1.1 A/B: **A** (a standing guard with a fails-if-dropped fixture), *assumed*; B would drop SVG import, MHTML and EPUB.

### D30 — The model exports only text a reader could see *(auto-adopted)*
`DocumentText` emits every run with only a `Hidden` flag (`doctext.go:66-105`), so a white-on-white run, a run off the crop box, or text beneath a box drawn over it would travel into an editable file. The model reads a run only if it is not off the page and is not
covered. A hidden (OCR) run is read **only on a page with no visible text** — there it IS the picture's reading and D20 depends on it — and dropped on a page that has visible text. Where the map cannot say a run is uncovered (shapes and text are separate lists), the run is
**excluded** and counted as `coveredText` in `X-Nib-Export`: the safe error leaves text out, never leaks it. A redacted-then-exported fixture is the red proof (the redaction's removed text must not appear; an overlay-only "redaction" is reported, not exported).

### D31 — A converter lives and dies with Nib, and an import has a memory budget *(auto-adopted for the mechanisms; the numbers assumed)*
`killTreeOnCancel` (`convertproc_unix.go:20`) is reached only by a context cancel that process exit can beat, so Quit (ADR-169), idle exit, SIGKILL and a crash leave `soffice.bin` running and its `nib-office-*` directory (holding the user's source in `in.<ext>`) behind.
(1) A registry of live converter process groups is killed **synchronously** on the shutdown path, before exit; the window shows the launch overlay's `quit` words. (2) A startup sweep removes `nib-*` temp directories by age, named with the pid and its start time — never by a marker file — and covers the browser profile, Ghostscript's and the multipart spill.
(3) `ParseMultipartForm(maxPDFBytes)` keeps 200 MiB in RAM and `io.ReadAll` copies it again (`server.go:59`, `office.go:33`): the body spills to disk with a small `maxMemory`; a converter's output is size-checked by `stat` before it is read; the export takes a total-bytes cap (*assumed* 256 MiB with the per-image
caps D18 left unvalued). The peak RSS of a 200 MiB import and a 1,000-page export is **measured at P01.S03 and quoted**; the estimate of 1.5–3 GiB is unmeasured. (4) **D9's gate, as code:** a buffered-channel acquire in a `select` against `ctx.Done()`, release deferred before any panic point, a queue bound (*assumed* 16, then a named refusal),
the 2-minute budget **started at acquire** (not at the request), `roomToOpen` asked again after acquire, `safe.Recover` first in every new goroutine, and a test that cancels a queued import and then lets a third acquire. The export takes the same gate. (5) The export is pinned to its document by `X-Nib-Doc` (ADR-004) and answers 409 if it is gone (ADR-001); the model read checks its context per page —
pdfcpu's validation itself cannot be cancelled, and that is declared.

### D32 — The format table classifies by magic bytes, and no attacker string reaches a command line or a path *(auto-adopted)*
An extension that disagrees with the sniffed type is a **named refusal**, except where bytes cannot say (Markdown, text, CSV); a tool is handed only the sniffed type, with the input written to a fixed name `in.<sniffedExt>`. CBZ entry names are sort keys, never paths: `..`, absolute names and symlinks are refused; the TIFF walk stops at the page cap
and detects an IFD loop; zip64 and nested archives are refused. This is what D8's "classified by content first" must mean, and `imageopen.go:60-66` (image sniff, then extension) is the shape it replaces.

### D33 — The outside tools are contained, and each has a row *(auto-adopted for the mechanisms; the numbers assumed)*
LibreOffice's profile is pre-seeded with macro security at its highest level and links never updating (a DOCX `INCLUDEPICTURE` and an ODT external link are the fixtures; whether LibreOffice fetches them today is **unverified**, so the loopback canary decides). Ghostscript runs only through one `runGS` door with `-dSAFER` pinned by a census (`pdfa_gs.go:99`).
Every tool has a timeout, a memory ceiling and a lowered priority in the format table (*assumed* starting values: `gs` 2 min, browser 60 s, `pdftocairo` 60 s, HEIF 60 s) and a per-OS candidate row. **Declared gaps:** `pdftocairo` is not on stock Windows or macOS, so SVG export lights up there only where poppler is installed (D7);
the Windows `taskkill` branch is compiled and argv-tested only (`convertproc_windows.go:12-15`) and every new tool inherits that. The egress guard is the root `egress_test.go` extended with a census of every `exec` site; the README's "every editing feature works with no network" (`README.md:69`) is what it keeps true.

### D34 — An export is judged against the source PDF, and a silent drop is the fatal bug *(auto-adopted)*
D22's oracle compares the round-tripped words to the **source PDF's** `DocumentText` (a multiset of words plus order within a paragraph), **not** to the model's own text: a reader that drops a paragraph passes the second and fails the first. `X-Nib-Export` adds `completeness` (source rune count against model rune count) and a loud flag below a stated
ratio (*assumed* 0.98). Red proofs: drop one table cell, swap two paragraphs, drop a footnote, truncate the last page — each must go red. **Metadata:** writers set a fixed Producer / Creator ("Nib"), no author, no source path, no timestamp finer than the day, a random EPUB id; the PDF's Info and XMP are never copied; a test opens `core.xml`, `meta.xml` and `content.opf`.
Every census names its enumeration source and the mutation that adds a row with no test (a name-grep passes an empty test body). **Failure playbook, stated:** a LibreOffice crash mid-run is a named refusal with the missing-output sentence; a machine that sleeps resumes Go's monotonic timer (it does not fail it), so a conversion past its wall budget is reported, not retried.

### D35 — The source is never written, and a recorded path is only a path *(auto-adopted)*
L1's census gains its red proof: a mutation making one door call `os.WriteFile(srcPath)`, and an assertion on the source's mtime, inode and directory listing. *Save As* refuses a destination equal to `importedFrom`. Open Recent records the path and nothing of the content (`vault.go:1746`; `/api/recent` is behind `requireUnlocked`); a recorded entry whose tool is now absent shows the missing-tool
sentence when reopened (`/pending 490` coupling stays). Imports add nothing else to the vault, and **Resume skips an imported tab** (`lastopen.go:32`, declared).

### Stage 7 — plan-review pins *(2026-11-01; a ten-seat panel read the plan cold — seven dimensions, packs `go` and `verification`, the PDF-format, privacy / accessibility / small-practice-user seats; Dan adopted every recommendation. Four claims were checked before they were pinned: `MapShape` carries no paint order (`pagemap.go:66-74`); `socketOwner` is the existing peer check (`launchpeer_linux.go:38`); D9's request-context cancel breaks the hand-off client's "the route does not stop when its caller does" (`instance.go:437-440`); no pagemap JSON-name test exists. The seats' two run results — Chromium dumped core under `ulimit -v`, and an inconclusive proxy variant — were NOT re-run here and are marked below.)*

Each pin is tagged `(plan-review pin: <topic>, 2026-11-01)` and names what it attaches to. A pin that adds a clause names the observation that discharges it.

**R1 — the export oracle (D34, D22, P02.S04, P03 exit) *(plan-review pin: oracle)*.** The oracle's reference is `DocumentText` minus an **independently written** visibility filter in the test tree (never the model's predicate); `X-Nib-Export`'s `coveredText`, `hiddenText`, `offPage` and `unreadGlyphs` must equal the difference. `completeness` is per-page **word-multiset** equality with exclusions counted by cause, loud on any unexplained loss — the 0.98 rune ratio is struck (it is blind to reordering and to a one-cell drop). `PageMap.Unread` rides into the model per page; those pages go in `flaggedPages` with `unreadGlyphs`. A second, LibreOffice-independent oracle parses the written package (`w:t` runs in `document.xml` order, `office:text` in `content.xml`; EPUB and HTML with `x/net/html`, Markdown with goldmark — all in `go.mod`) over the **committed generated** fixture set in `go test`. Red proofs name their layer: reader mutations (tree vs proposal, a grouping drop, D30's filter, a truncated last page) run against `docmodel.Read`'s output directly, never through `~/nib/export-corpus.txt`. *Observation:* each mutation turns a named test red with its assertion message. `exportfidelity.sh` with no `soffice` or an empty corpus exits non-zero "not run".

**R2 — what the model reads (D30, D20, D18) *(plan-review pin: visibility)*.** The walker gives runs and shapes a **shared draw-sequence index** (additive, in D18's `MapText` change; `MapShape` has none today). A run is *covered* only when a LATER opaque path or image box contains it. A hidden (OCR) run is read **unless visible runs cover most of its box or the page's ink** — this supersedes D30's "only on a page with no visible text", which would drop the OCR body of a scan with a typed header (ADR-094 keeps that page OCR-able). A mixed page exports its picture and its visible text. Where order is unknowable the run is excluded and counted. *Observation (fixtures, each with its expected outcome):* a shaded table header (kept), a white box over text (dropped), an image over text (dropped), a scan with a typed header (body present; the oracle red if absent), text on an ADR-096 white ground (kept).

**R3 — the D29 listener *(plan-review pin: render origin)*.** One door, `renderOrigin`, serves exactly one body: bound to `127.0.0.1` only; a path of ≥128 bits from `crypto/rand`; ADR-157's `socketOwner` peer check, and a wrong-uid or wrong-`Host` request (`Host` must be `nib-render.test:<port>`) answered 404 **without consuming** the use; consumed on the first GET of the token path only, everything else 404 without closing from inside the handler; `ReadHeaderTimeout`, a hard 30 s lifetime, `Close` deferred on every exit path (cancel, panic, a browser that never connects); `safe.Recover` first in the serve goroutine. A source census fails on any `net.Listen` / `http.Serve` outside `cmd/nib/main.go` and this door, with an `egress_test.go` row. The browser takes `--no-proxy-server` and the child environment drops `HTTP(S)_PROXY` / `ALL_PROXY` — *a proxy bypass of `MAP * ~NOTFOUND` is unconfirmed (one variant, inconclusive)*; the hostile-env-proxy fixture decides. D29's fixture (a) becomes a table of **at least eight vectors** (meta refresh, `<iframe>`, `<object>`, `<embed>`, `<link rel=prefetch>`, SVG `<use href>`, CSS `@import` / `url()`, an anchor), **each with a positive control** (protections off must print the secret, or the canary must fire). The controls log the browser version and **skip loudly** (a named count); P04.S04 and P05.S01 are not marked done on a machine where they skipped. A stub browser that exits 1 must make the format `disabled` with its reason (D27). The browser-launch census names **two** doors (the window launch at `browser.go:45,75` and the headless render, which lives in `internal/browserrender`) and holds only the render door to D29's flags.

**R4 — temporary directories and orphans (D31(1)(2)) *(plan-review pin: temp lifecycle)*.** Every temp creator goes through ONE `tempdir.New(kind)` door naming `nib-<kind>-<pid>-<starttime>-<rand>` (0700, asserted). The startup sweep — run on every start, the CLI included — removes a directory only when that pid **and start time** (from `/proc` or the platform equivalent) are no longer live, never on age alone, skips any name that does not parse, and **kills a still-live converter group** recorded in the directory (verified by pid + start time) before removing it. Spill files go to a `nib-spill-…` directory from the same door; `nib-rendezvous-*` (`cli/rendezvous.go:117`) is declared exempt or migrated. Every import and export `defer`-removes its own directory on success, refusal, cancel and timeout (test: none left after each); ENOSPC on a temp write is a named refusal. `Pdeathsig` is per-thread in Go and unix-only, so it is not relied on: the window between a crash and the next start is a **declared gap**. *Observation:* a second Nib process holding a directory is untouched; a killed converter's group is found and killed on the next start.

**R5 — containment is in S02a, not later (P01.S02a, S03, D31, D32, D33, D14) *(plan-review pin: containment order)*.** S02a owns D31(1)(2)(3) (the `tempdir` door, the sweep, the registry kill, the multipart spill to disk with a small `maxMemory`, the `stat` check on converter output), D33's LibreOffice profile (macro security at its highest, links never updating; fixtures: a DOCX `INCLUDEPICTURE`, an ODT external link, a loopback canary that decides — *whether LibreOffice fetches them today is unverified*) and the `exec` census, D32's sniffer with `importLimits` carrying **provisional** starting values, and the measured peak RSS at gate = 2 with two 200 MiB inputs — S02a does not close without the number. S03 owns D32's CBZ / TIFF rules and tunes D14. D9's "measured at P01.S02" reads S02a.

**R6 — the gate (D9, D31(4), D23) *(plan-review pin: gate semantics)*.** The hand-off door runs its import on a context **detached from the connection** (its own budget only), because the client's 30 s wait (`instance.go:447`) would otherwise cancel the context and kill the converter. The gate is acquired **once at the top** of `Import` / `Export`; a tool called inside never re-acquires (a `.eml` whose body goes through the browser would deadlock at width 2) — test: two nested imports at width 2 complete. The export has its **own gate**, budgeted by pages with a per-page context check and a reported (not retried) wall clock. Multi-file behaviour: a hand-off backlog is admitted in order with one window notice ("Importing 12 of 50"); past the queue bound the refusal says "N files waiting, try fewer", is idempotent (nothing left behind) and is counted `import.refused{queue-full}`; a queued item has a maximum wait (*assumed* 60 s) and a visible per-item Cancel. Admission is by `Content-Length` **before** `parseMultipart`; queued spill bytes count against a total cap. The install is one step that rechecks `ctx.Err()` under `s.mu`, all or none. *Observations:* a test fires 50 hand-offs; one cancels at queued, at converting and at converted-not-installed and finds zero documents and zero directories; a converter that outlasts 30 s still installs its document.

**R7 — memory ceilings (D33) *(plan-review pin: ceilings)*.** The ceiling is an **RSS watchdog** (or a cgroup where there is one), never an address-space limit for the browser or LibreOffice — *a seat ran headless Chromium under `ulimit -v 4000000` and it dumped core and wrote no PDF; that run was not repeated here.* The format table declares per tool which OSes enforce it; an unenforced OS is a **declared gap named in that tool's missing-tool wording** (Windows and macOS have no `Setrlimit` / `nice`; a `//go:build` sibling that silently does nothing is the Go pack's no-op). *Observation:* a test kills a tool that exceeds a small ceiling on Linux.

**R8 — leaf packages (D16, P02.S03c, P03, P01.S03, P02.S01) *(plan-review pin: package graph)*.** `docmodel` holds the types only; the reader is `docmodel/read`. The census uses `go/packages` over the **transitive closure** and fails if any `docwrite/*` reaches `pdfops`, `pdfread` or `uacheck`, with a planted-import red proof. `internal/ziputil` (a stored-first entry from an explicit ordered-entries spec, D32's entry-name safety, D14's ratio and entry caps) is built in **P01.S03** — CBZ import and export need it before P03 — and its census (callers of `zip.NewWriter`) gates CBZ from the start; per-format structural tests: ODF `mimetype` at byte 38, stored; OOXML content-types present; EPUB stored-first.

**R9 — the source-never-written census (L1, D35, P01.S02a) *(plan-review pin: L1 census)*.** After every route **and every failure path** (cancel, cap refusal, tool crash) the census asserts the source's mtime, inode, size, hash **and its parent directory listing**; a static check forbids any write-capable call whose path derives from the import input. Mutations named: `os.WriteFile(srcPath)`, `os.Rename(tmp, srcPath)`, `Save` to `importedPath`. The census is **parametrised over the format table** so each later producer fails it on arrival.

**Warnings, adopted** (each tagged `(plan-review pin: W<n>, 2026-11-01)`; number = the review's):
- **W1** (D18 links) the model carries only `http`, `https`, `mailto`; any other scheme is plain text counted in `droppedLinks`; writers escape every text and attribute value; HTML-writer fixtures for `<script>`, `"onerror=` and `javascript:`. **W2** (D32 / D2 `magick`) `magick heic:in png:out` under a restrictive `policy.xml` via `MAGICK_CONFIGURE_PATH`; `heif-dec` first; a `.heic` carrying MVG text is the fixture. **W3** (D8 / D11 / D23) the notice is `textContent`; names are stripped of control characters, separators and bidi overrides; `X-Nib-Export` values come only from enums and integers; page lists are capped (`revision.go:18`).
- **W4** (D31(1)) the registry kill is SIGKILL on the group with a 1 s cap, called from ONE `shutdown()` that Quit, idle exit and SIGTERM all reach; a test each. **W5** (L8, D14) the counters (`import.ok`, `import.refused`, gate depth, queue-full, sweep removals, orphan kills) are readable on the status or diagnostics copy, each with a non-zero probe fixture and a zero-meaning line. **W6** (D29) the browser's per-render cost is measured at P04.S04, its profile is removed in the launching function's `defer`, the sweep is the backstop, and profile + temp total is capped.
- **W7** (D23, D34) P02.S03c measures wall time per page on the large corpus and sets any page ceiling from that figure — **no number is pinned now**; the completeness check reuses the single map read. **W8** (D23) the export captures `doc.data` and `rev` under `s.mu`, parses outside it, holds the slice by reference, answers 409 if `rev` moved, and the Format-line note says unbaked overlay marks and field values (ADR-002) are not exported. **W9** (D11, D35) a server-side `importedPath` (path and hand-off doors only, never on the wire) is compared cleaned and symlink-resolved with Save As's destination; `importedFrom` stays the display basename; the pin states whether the refusal is vacuous when the destination must end `.pdf`.
- **W10** (P01.S01) the table row gains `exports []{thing, label, needs}`; every Format line is generated from it; the census fails on any format literal in `app.js` outside the table and counts magic-byte tables as lists (red mutation: add `.xyz` to `listDir`). **W11** (D23, D28) one `ExportFacts` Go struct (P02.S04) owns every `X-Nib-Export` key — `source`, `droppedLinks`, `placeholderImages`, `flaggedPages`, `scannedPages`, `coveredText`, `hiddenText`, `offPage`, `unreadGlyphs`, `completeness`, `signed` — with a field-by-field wording table; the guard derives left from the struct by reflection and right from the JS. **W12** (D16, D3) `Document.Meta{Title, Lang, Dir}` and `Block.Dir`: Title from the imported name (never a source path), Lang from the catalog `/Lang` only else `und`, writers emit `w:bidi` / `dir=rtl` / `xml:lang`; a structural test fails on a missing `dc:title` / `dc:language`.
- **W13** (D18) there is no pagemap JSON-name test today: create it in the same commit as the additive `MapText` fields, measure payload size and `MapPage` time before and after on the accuracy corpus, and consider a model-only read path so `/api/pagemap` is untouched. **W14** (D6, D35, `/pending 490`) S02b does not depend on typed Recent entries: the missing-tool sentence is computed at reopen from the sniffed type and the table; 490 owns any schema bump and its older-build behaviour. **W15** (D13, D15) `LabelUA` and `SetLang` are reachable only from `Import` (source census); the CLI and the route produce byte-identical output for the same options.
- **W16** (verification) a skip for an absent tool prints a named count ("N present-path tests not run: heif"); a slice does not close on a skipped present-path test without naming the machine that ran it. **W17** (D14, P01.S03) each cap test reads the named constant and builds constant + 1; the generator is modelled on `build/genpdf.go` and asserts each fixture really exceeds its target; CSV, Markdown and text imports get an output-page cap so one import cannot exceed the 512 MiB cap after conversion. **W18** every census names its enumeration source and its red mutation; the P06 census runs a registry the tests populate at run time (executed, not grepped; a `t.Skip`-only test fails) with the row and guard counts pinned as constants **outside** the table, as `verify_test.go` does.
- **W19** P01's exit gains a negative: each cap and each sniff mismatch refuses by a named sentinel and leaves no `nib-*` directory and no vault entry (mutation: raise the cap, the boundary test goes red). **W20** (L8) the per-commit tier for this plan is `go test ./internal/pdfops/ ./internal/server/ ./internal/docmodel/... ./internal/docwrite/...` — whole packages, never `-run`, because the census guards live there — plus `./build/jsdomtest.sh`; the full suite runs once at each slice close and the done marker names its result. **W21** (sequencing) S02a keeps `cmdOffice` working through `Import` with its CLI tests green; P01.S01's table ships the schema (tool id, candidates per OS, timeout, ceiling, probe) with ALL tool rows, an exported `browser.Find` and a `pdftocairo` row, probes "unprobed" until their door lands; the `.txt` writer is P02.S04's.
- **W22** the standing oracle and red-proof fixtures are generated in-test through `internal/testpdf` (committed code, no binaries); the local `~/nib/export-corpus.txt` is an instrument only. **W23** (D18 images) the Figure-to-image join is **new work**: no pdfcpu API gives an image at a box (`model.Image` has `Name` / `ObjNr` / `PageNr`, no placement), so it walks forms, keys placements by `ObjNr` with a rect per use, counts pictures with no Figure, and has fixtures for an image in a form and one drawn twice (`extractImages`' `seen[ObjNr]` shows a twice-drawn image once). **W24** (D18 links) `PageLinks` follows pdfcpu's population rules (`ctx.PageAnnots` holds indirect annotations only; direct `/Annots`, indirect `/A` / `/Dest`, named destinations, ADR-168's set-aside entries) and converts through `placeOnPage` only; fixture: a rotated-90 page with direct-dictionary links.
- **W25** (D17 / D18) `MapText` exposes `Replaced` (the walker computes `textRun.replaced`, `textrun.go:100,186`) and the model emits the replacement once per sequence, compared by D34 under the same rule. **W26** a signed document's export carries `signed` (+ count) in `X-Nib-Export`, the dialog says "signatures are not carried; the file is a new, unsigned document", and text from signature appearances is excluded. **W27** (P01.S03) orientation is JPEG-only today (`imagedoc.go:307`): each producer defines "apply the rotation or refuse a mirrored one by name" with a fixture per orientation, the text / CSV page size and margins, and whether TIFF DPI is honoured.
- **W28** (accessibility) the import notice goes to a `role="status"` / `aria-live="polite"` region (not `#sessionNotice`'s `role="alert"`), moves no focus, and a batch is coalesced ("Imported 3 files"); the queued state is announced ("Importing report.docx, 2 ahead"), `aria-busy`, with a keyboard-reachable Cancel; a disabled Format option keeps its reason through `aria-describedby` (a native disabled option is skipped) and a jsdom test asserts it; the per-format note and the export result use the polite region; the per-tool missing paragraphs are static text revealed on the card's open and only a recheck result is live; a keyboard-order test covers Page Functions; imports set a `/Lang` default and say plainly they are untagged. **W29** (small-practice user) missing-tool and disabled-format messages say in plain words what they cannot do, what to install with a link and what still works, naming the feature ("SVG pictures") before the tool; each export flag carries one action and says the file was saved, and "flag" leaves the UI; the notice also says "Your original is unchanged"; Save on a pathless imported tab opens Save As with `<name>.pdf` beside the source; Resume says "1 imported document was not restored"; `closeWouldLose` treats a never-saved imported tab as unsaved. **The reviewer's rename of "Document as…" is rejected** (it collides with the Save a Copy group's name); the button keeps its label and gains a title naming the formats.
- **W30** the README says Recent lists the paths of imported files; an export's title is never a source path or filename. **W31** (D12) the notice is keyed on the `(epoch, id)` pair ADR-004 already carries (ids are monotonic per process, not across restarts). **W32** (D34) the sentence about a sleeping machine resuming Go's monotonic timer is dropped (true on Linux; unverified elsewhere). **W33** (D33) Ghostscript runs through one `runGS` door (`pdfa_gs.go`'s), `-dSAFER` pinned by a census, never a second invocation. **W34** (P04.S04) MHTML / EPUB unpack in memory, resources are rewritten to `data:` URLs, every other reference is rejected, and no file is written under an attacker's name. **W35** the consistency edits above (D3, D5, D22, the Status line, "Group" for format groups so "tier" means test tier only, the S02a / S02b Refs, D8's callers, below).
- **D8's caller list (W35):** the direct callers of `asOpenableDocument` are `server.go:914,1014`, `sources.go:77` and `combine.go:36`; `handoff.go:176` and `diskstate.go:257` reach it through `readInstallablePDF` (indirect). `pdftocairo` has no import notice; its Format-line reason carries the `href` authored in `index.html`.

**Info, adopted:** a written OOXML / ODF conformance subset in `docs/` naming the styles used (Heading 1–6, List Paragraph, Table Grid); the bold / italic hit rate measured on the corpus at P02.S03a; every `X-Nib-Export` key and counter has one fixture that drives it above zero; no write-door refusal (ADR-136 / 145 / 032) can fire on a freshly built import.

**Phase 5 merges, applied mechanically (19 → 14 headed slices; 22 → 17 counting today's two cuts):** P01.S04 → P01.S03; P02.S02 → P02.S01; P03.S02 → P03.S01; P04.S03 → P04.S02; P05.S02 → P05.S01. Earliest coordinate kept; nothing renumbered; the one absorbed slice that carried acceptance (P01.S04) had one bullet and it is in P01.S03 verbatim (before: S03 1 + S04 1 = 2; after: 2). Not merged, with the reason: P01.S01 | S02a | S02b | S03 (different seams and risk; S02 is cut once); P02.S03a / b / c (cut once, today); P02.S04 (needs S03c); P03.S03 EPUB (a third structural format with its own conformance oracle — borderline, kept); P04.S01 | S04 | S05 (a different tool and parser risk each; S05 needs S04); P05.S03 PPTX (own writer, own oracle); and every phase boundary.

---

## Out of scope

- Everything in D2's "dropped" list. Pixel editing of an imported image (mark-up and redaction work on the PDF it became).
- Round-tripping: an exported DOCX re-imported is a new import, not a recovery of the original PDF.
- Printing to PDF from other programs (`/pending 902`) and scanning (`/pending 903`) — separate items.
- PDF/A and PDF/UA claims for an *imported* document beyond what ADR-032/033 already decide.

## Standing caveats

- Fidelity of any PDF → editable export is **inferred from layout**; no output is "the original document". Judge DOCX / ODT by opening them in
  LibreOffice (an instrument, not a tier; Word itself cannot be driven here), EPUB with `epubcheck`, PPTX by LibreOffice Impress.
- LibreOffice, Ghostscript, a HEIF tool and a browser are not bundled and may be absent on any machine; every Group 2 row has a missing-tool test.

---

## Build order

### P01 — The import door
Goal: one format table and one import door, so every type but PDF arrives as a PDF tab the same way, and Group 1 works with no outside tool.
Exit criteria:
- Every Group 1 import type opens (drag, path, upload, hand-off) as a pathless PDF tab with the notice; the original is byte-identical afterwards (L1's census).
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
  **(Stages 4–6 pins)** The table classifies by magic bytes (D32). `.desktop`'s `MimeType` is generated; macOS and Windows file association ship nothing today (`build/nib.desktop:9`, `nfpm.yaml:34`) and are a **declared gap**, so "double-click imports" is a Linux behaviour until scheduled. Guards, one per sync rule and written in the same commit as the rule: table → page `accept`, table → per-tool notices, table → `.desktop`, Format-line reasons, `X-Nib-Export` keys → status wording, `MapText` style fields in the JSON-name test. The owners of the three lists (formats, tools, Format-line labels) are named in `CLAUDE.md` in the same commit.
Tasks: *(written at slice-grill time)*

#### P01.S02 — Open imports *(cut once, 2026-11-01, into S02a and S02b)*
**Refs (plan-review pin W35):** S02a — D8, D9, D11, D13, D14, D31, D32, D33, D35, L1; S02b — D1, D6, D12, D24, D35, L2. (D15 is P01.S03's.)
**New slice because:** the reviewers found this was a rewrite of the open door plus a UI feature plus two censuses, which cannot be one commit (the cut rules apply: a slice is cut once, never again).
**S02a** — the function and the doors: D8, D9 (as code, D31), D11, D13, D32; the per-caller census; the L1 census with its mutation (D35); the counters; `nib import` is the command P01.S03 adds.
**S02b** — the window: `importedFrom` on the document and `docResponse`, the once-per-window notice (D12), the Import… relabel and per-tool missing notices (D24), Open Recent for imports (D35), the Save As refusal. The Scope, Acceptance and Tasks below are the pair's together until each is grilled.
Scope: the four doors route a non-PDF through one import function; the notice; the Import… entry keeps options; Open Recent records the source (D6). Refs: see the pair's lines above.
Acceptance:
- Each door imports the same file to the same bytes; the notice names the file; Save on the result is Save As.
- A census drives every route against an imported tab and hashes the source file before and after.
- **(grill pin, 2026-11-01)** `document` gains an `importedFrom` marker (it has none, `server.go:111-123`) carried on `docResponse` for the notice;
  ONE import function serves all callers of `asOpenableDocument` (`server.go:914,1014`, `sources.go:77`, `handoff.go:176`, `combine.go:36`;
  `diskstate.go:257` reaches it through `readInstallablePDF`), with a per-caller census; ONE naming rule (images keep `scan.png` today, Office
  becomes `.pdf`). Counters: `import.ok{door,format}` and `import.refused{cause}`, atomics only — the open routes are request handling and this
  approval is the hot-path check-in.
Tasks: *(written at slice-grill time)*

#### P01.S03 — Group 1 native producers *(absorbs P01.S04, `nib import`, 2026-11-01)*
Absorbs: P01.S04 (`nib import`) — the CLI is the second door to the same producers; S02a keeps `cmdOffice` working through `Import` meanwhile and this slice adds the `import` name and the `office` alias.
Scope: GIF, BMP, multi-page TIFF, plain text, CSV, CBZ producers, with caps (pixels, entries, decoded bytes, pages) and one test per cap; the command `nib import`, generalising `nib office`. Refs: D2, D10, D14, D15, L7. Tunes D14's unmeasured values against a measured run.
**(Stages 4–6 pins)** Builds the hostile-input generator in-test (`build/genfixtures.go` shape; nothing binary committed): zip bomb and ratio archive, huge-dimension and many-page TIFF with an IFD loop, a `..` CBZ entry, a malformed docx. Measures and quotes peak RSS for a 200 MiB import, a 250k-row CSV through `mdpdf` (its scaling measured linear to 16k rows only) and a 1,000-page model read, and sets D9's gate of two on them. Adds the startup temp sweep and registry kill (D31). Each native producer has a wall-clock cap.
Acceptance:
- Each producer's output opens and reads back its content; each cap refuses by name.
- Same set as the window; a missing tool exits non-zero with the missing-tool sentence.
Tasks: *(written at slice-grill time)*

### P02 — The Export surface and the document model
Goal: the Export card reshaped to L3, image exports widened, and the one document model (D4) built, with the first two writers (Markdown, HTML) as its proof.
Exit criteria:
- Export takes a PDF only; Split by bookmarks / page range live in Page Functions; one button per thing exported, the format on the Format line.
- Pages as images offers PNG, JPEG, TIFF (multi-page), WebP, GIF, BMP and CBZ.
- Markdown and HTML export a tagged and an untagged document with headings, lists and tables kept; a scanned page goes through `ensureText` first, and one still unread exports as its own picture and says so (D20).
*Slices sketched:* S01 Export card + Split move + image formats (ADR superseding 906's definitions; **Absorbs: P02.S02**, image formats — one seam, `exportPagesBtn`, `assemble`, `saveformats`); S03 the document model; S04 Markdown + HTML + Text writers (**plan-review pin W21:** the `.txt` writer is S04's).
**(round 3, 2026-11-01)** S01 builds D25–D28 (the six-control card, the splits' move with their ids, disabled-with-reason formats, the per-format note); the Import… relabel and per-tool missing notices of D24 belong to P01.S02, which already owns the entry.
**(grill pin, 2026-11-01)** S01 is half done: ADR-103 already gives one button per thing and a Format line. What remains is moving the two splits and
widening the image row; a new ADR supersedes ADR-103's "Pages as images" row and its "Export & Print is now eight" sentence, and the same slice edits
the tests that pin the card (`filemenu.test.mjs:79-89`, `toolbargroups.test.mjs:170,214`, `saveformats.test.mjs:326-507`, `racepins.test.mjs:192`,
`app.js:14452-14462`). The image-format part of S01 (was S02): the browser renders PNG as now (pdf.js, `app.js:10027`); `/api/assemble` transcodes to GIF, BMP, CBZ and multi-page TIFF
(a hand-built IFD chain — `x/image/tiff` writes one image per file); Go has no WebP encoder, so WebP uses canvas `toBlob('image/webp')`, offered only
where the browser can encode it and said so on the Format line. S03 carries the four missing inputs named in D4 and D18, the package and census of D16, the source rule D17 and the scan rule D20; S04's route is D23 and its oracle D22 (Refs: D4, D16–D23, D25–D28, D30, D34).
**S03 is cut once, 2026-11-01** (**New slice because:** four new readers, a package and a census are not one commit): **S03a** style and links (D18) plus `~/nib/export-corpus.txt` with its sha256 sidecar and a generated hand-built-model set — tagged, untagged, two-column, merged-cell, scanned, right-to-left, redacted; the writers are judged by D34's oracle and are **not** golden-tested;
**S03b** images and paragraph grouping (D18), with the Figure-to-image join and the unembeddable-image placeholder; **S03c** `docmodel.Read`, D17, D20, D30 and the import-graph census (`go/packages`, not text matching). S04 owns `.txt` from the model and the removal of `documentText` as an export (D28); *Document text (.txt)* stays on the card until S03c lands and *Document as…* ships with S04, listing only formats that exist (an unbuilt format is not listed at all). ADRs, numbered from 183 and written in the slice that builds each: ADR-183 PDF is the only document, Import and Export, with the splits' move
(P02.S01, superseding ADR-103's image row and its "eight" sentence); the import door (P01.S02a); the document model (P02.S03c); the export route and `X-Nib-Export` (P02.S04); the outside tools and the browser door (P04.S04).

### P03 — Editable documents
Goal: DOCX, ODT and EPUB writers over the model, judged by D34's oracle against the source PDF, each flowing (D21).
Exit criteria: each written package's XML-structure oracle (R1) passes on the generated fixture set — heading, list and table counts equal the model's; LibreOffice and `epubcheck` are confirmatory runs whose absence is printed; EPUB also passes Nib's structural test (stored-first `mimetype`, `container.xml`, nav, `dc:title`, `dc:language`) everywhere; the menu states what each keeps (L6). One zip door (`internal/ziputil`, built in P01.S03 from `GridToODS`'s stored-first `CreateRaw`, `spreadsheet.go:64`) serves ODT, EPUB, DOCX, PPTX and CBZ, held by a census over the callers of `zip.NewWriter` (R8).
*Slices sketched:* S01 DOCX + ODT writers (**Absorbs: P03.S02**, ODT — both are word-processor packages over one block-to-paragraph mapper and the one zip helper); S03 EPUB (reuses P02.S04's HTML writer). Each writer carries its labels and its D22 / D34 oracle; the old S04, flowing-versus-exact, is folded in by D21.

### P04 — Group 2 imports
Goal: the formats that need a tool, each behind the one door.
Exit criteria: each Group 2 type imports where its tool is present and names the missing tool where it is not; HEIC, saved web pages / EPUB, PostScript / EPS and email each have a fixture and a cap.
*Slices sketched:* S01 LibreOffice breadth; S02 Ghostscript PS/EPS + HEIC/HEIF (`/pending 911`) (**Absorbs: P04.S03** — each is one external-converter wrapper behind the same door with one fixture family); S04 the browser as renderer (D5 settled by D29; HTML first, MHTML / EPUB route decided at its grill by probe; one exported browser lookup beside `toolpath.go`'s per-tool candidate lists — `findChromium` is unexported today, `browser.go:130`); S05 `.eml` / `.msg` (`.msg` needs an OLE reader, see D2). S02's HEIC tool order and capability probe are in D2.

### P05 — Group 3
Goal: SVG in and out, PPTX out.
Exit criteria: an SVG with a script or an external reference imports with both inert (through the browser door); a page exports as SVG where `pdftocairo` is present; a PPTX opens in LibreOffice Impress with one slide per page, editable text boxes and the pictures placed.
*Slices sketched:* S01 SVG in and out (sandboxed import through the browser door; export through `pdftocairo`; **Absorbs: P05.S02** — one SVG corpus); S03 PPTX writer from the page map.

### P06 — Close
Goal: documentation and the censuses that keep the set honest.
Exit criteria: README, the web page and the CLI table describe the set; one census asserts every row of the format table has an import or export test, a cap and a missing-tool test; `/pending 906`, `912`, `911` closed or re-pointed here. **(Stages 4–6 pin)** Also names `docs/release-notes/<version>.md` (ADR-175), the README feature-matrix row "Office → PDF conversion" (`README.md:48`) and its footnote saying Office needs LibreOffice "and nothing else" (`:79`), "What leaves your computer", and the `nfpm.yaml` `Recommends` decision (it lists LibreOffice only, `:19`; Ghostscript, poppler, a HEIF decoder and a browser are undecided). `instruments/open-as-is.md` is superseded by D1 except for its path inventory.
