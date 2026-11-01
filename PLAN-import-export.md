# PLAN — import and export for every type but PDF

**Dateline.** Seeded 2026-11-01 from Dan's `/discuss` of `/pending 906` (read other formats) and `/pending 912` (export a
PDF as Markdown, DOCX or EPUB), which it replaces as the plan of record: **where those two entries and this plan differ, the plan
wins.** The foundational choices below (D1–D3, D6) are Dan's words; D4, D5 and D7 are tagged `assumed`. Facts are from the entries,
`instruments/open-as-is.md` (906's seam inventory, written for the model this plan reverses, reused where it still holds) and
the code at v2.16.1, read for this plan.

**Status: unbuilt. P01.S01 is next.** No pass beyond the seed has run: the whole-plan `/grill` (Stage 2) is the next one.
**Passes:** *(none yet)*

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

### D5 — Outside tools sit behind the one door *(assumed, 2026-11-01)*
`magick` / a HEIF decoder, Ghostscript, LibreOffice and a Chromium-family browser join `toolpath.go`, `missingtool.go` and ADR-040's
wording. **Open question for the grill:** whether the browser can be driven as a renderer (headless print-to-PDF of a local file, scripts
off, network off) on the user's own browser, and what happens when the user's browser is Firefox.

### D6 — Opening a non-PDF imports it, once, with a notice *(Dan, 2026-11-01)*
Open, drag-drop, double-click and the OS hand-off run the import and say so: *"Imported report.docx as a PDF. Save writes a PDF; use Export for
other formats."* The **Import…** entry stays for the choices an import has (document language, ADR-033's `LabelUA`). The result is a
pathless PDF tab (L2). *Assumed with it:* Open Recent records the source path and reopening imports again (906 P15); this couples to
`/pending 490` (typed entries).

### D7 — ODT and SVG export are in the set *(assumed, 2026-11-01)*
ODT mirrors the XLSX / ODS pairing the table export already has and costs one writer over D4's model. SVG export (a page as SVG, through
Ghostscript's SVG device) follows Dan's rule that every type but PDF goes both ways. Either is dropped on a word from Dan.

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
Tasks: *(written at slice-grill time)*

#### P01.S02 — Open imports
Scope: the four doors route a non-PDF through one import function; the notice; the Import… entry keeps options; Open Recent records the source (D6). Refs: D1, D6, L1, L2.
Acceptance:
- Each door imports the same file to the same bytes; the notice names the file; Save on the result is Save As.
- A census drives every route against an imported tab and hashes the source file before and after.
Tasks: *(written at slice-grill time)*

#### P01.S03 — Tier 1 native producers
Scope: GIF, BMP, multi-page TIFF, plain text, CSV, CBZ producers, with caps (pixels, entries, decoded bytes, pages) and one test per cap. Refs: D2, L7.
Acceptance:
- Each producer's output opens and reads back its content; each cap refuses by name.
Tasks: *(written at slice-grill time)*

#### P01.S04 — `nib import`
Scope: the command, generalising `nib office`. Refs: D2.
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

### P03 — Editable documents
Goal: DOCX, ODT and EPUB writers over the model, judged by instruments, with flowing and exact-layout forms where both make sense.
Exit criteria: each opens in LibreOffice with headings, lists, tables and pictures intact on the fixture corpus; EPUB passes `epubcheck`; the menu states what each keeps (L6).
*Slices sketched:* S01 minimal OOXML writer; S02 ODT; S03 EPUB; S04 flowing-versus-exact choice and the honest labels.

### P04 — Tier 2 imports
Goal: the formats that need a tool, each behind the one door.
Exit criteria: each Tier 2 type imports where its tool is present and names the missing tool where it is not; HEIC, saved web pages / EPUB, PostScript / EPS and email each have a fixture and a cap.
*Slices sketched:* S01 LibreOffice breadth; S02 Ghostscript PS/EPS; S03 HEIC/HEIF (`/pending 911`); S04 the browser as renderer (the D5 question, decided at its grill); S05 `.eml` / `.msg`.

### P05 — Tier 3
Goal: SVG in and out, PPTX out.
Exit criteria: an SVG with a script or an external reference imports with both inert; a page exports as SVG where Ghostscript is present; a PPTX opens in LibreOffice Impress with one slide per page, editable text boxes and the pictures placed.
*Slices sketched:* S01 sandboxed SVG; S02 SVG export; S03 PPTX writer from the page map.

### P06 — Close
Goal: documentation and the censuses that keep the set honest.
Exit criteria: README, the web page and the CLI table describe the set; one census asserts every row of the format table has an import or export test, a cap and a missing-tool test; `/pending 906`, `912`, `911` closed or re-pointed here.
