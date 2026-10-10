# Nib

[![License: AGPLv3](https://img.shields.io/badge/license-AGPLv3-blue.svg)](LICENSE)
![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)
![Platforms](https://img.shields.io/badge/platforms-Linux%20%7C%20macOS%20%7C%20Windows-555)
![100% local](https://img.shields.io/badge/data-100%25%20local-success)
![cgo-free](https://img.shields.io/badge/build-pure%20Go%2C%20single%20binary-00ADD8)

**A complete PDF toolkit that runs entirely on your own machine.** Fill any form,
make scans searchable with on-device OCR in 41 languages, truly redact, compare
revisions visually, rewrite a paragraph and have it re-wrap in the document's own
font, tag a document for screen readers and check it against PDF/UA, sign and
timestamp — alone, with one peer, or in a ceremony of up to 32 — convert to and
from Office and PDF/A, pull tables out to spreadsheets, and reshape pages — all
from a desktop-style app *or* a scriptable command line.

And it's **yours**: free and open source under the AGPL, a single self-contained
binary with no installer, no account, no subscription, and no cloud. Nothing you
open, type, or sign ever leaves your computer, and your documents and signing
identity live in an encrypted vault only your SSH key can open.

---

## How Nib compares

How Nib's feature set lines up against the three best-known commercial PDF editors and two popular free ones.
**Legend:** ✅ built in · 🟡 partial or limited · ❌ not available.

| Capability | **Nib** | Adobe Acrobat Pro | Foxit PDF Editor | PDF-XChange Editor | PDF24 Creator | PDFgear |
| --- | :--: | :--: | :--: | :--: | :--: | :--: |
| Fill forms — including flat & scanned | ✅ | ✅ | ✅ | ✅ | 🟡 | ✅ |
| Detect fields, turn a scan into a fillable form | ✅ | ✅ | ✅ | ✅ | ❌ | 🟡 |
| On-device OCR *(41 languages)* | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| True redaction + pattern / PII search-and-redact | ✅ | ✅ | ✅ | ✅ | 🟡 | 🟡 |
| Visual **and** text document compare | ✅ | ✅ | ✅ | 🟡 | 🟡 | ❌ |
| Edit existing text with paragraph reflow ¶ | ✅ | ✅ | ✅ | ✅ | ❌ | 🟡 |
| Accessibility check against PDF/UA ◊ | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ |
| Tag an untagged document automatically ◊ | 🟡 *(headings, paragraphs, lists)* | ✅ | ✅ | ❌ | ❌ | ❌ |
| Review and correct the tag tree and reading order ◊ | 🟡 *(no new or deleted tags)* | ✅ | ✅ | ✅ | ❌ | ❌ |
| Digital signature with your own certificate | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| RFC-3161 trusted timestamp | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ |
| **OpenTimestamps** (Bitcoin) proof of *when* | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| **Peer-to-peer co-signing**, no server \* | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| **Multi-party signing ceremony** (3–32), no server or account \* | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| **Send a document out to be signed**, no cloud \* | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| PDF/A archival export | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Office ↔ PDF conversion † | ✅ | ✅ | ✅ | 🟡 | 🟡 | ✅ |
| Table → spreadsheet (XLSX / ODS / CSV) | ✅ | ✅ | ✅ | 🟡 | ❌ | 🟡 |
| Merge, split, rotate, crop, N-up, Bates, page labels | ✅ | ✅ | ✅ | ✅ | 🟡 | 🟡 |
| AES-256 encryption | ✅ | ✅ | ✅ | ✅ | ✅ | 🟡 |
| **Scriptable command line** / batch a folder | ✅ | ❌ | ❌ | 🟡 | 🟡 | 🟡 |
| Runs offline §, no account, no telemetry | ✅ | ❌ | 🟡 | ✅ | ✅ | 🟡 |
| **Free & open source** (AGPLv3) | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Single self-contained binary, cross-platform ‡ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| **Price** | 🟢 **Free** | 🔴 Subscription | 🔴 Paid | 🟡 Free tier | 🟢 Free | 🟢 Free |

<sub>† Office conversion uses LibreOffice if it's installed — optional, detected at runtime, never bundled. ‡ One portable binary for Linux / macOS / Windows (PDF-XChange Editor and PDF24 Creator are Windows-only; PDFgear has no Linux build). § No account, no telemetry, no analytics, and every editing feature works with no network at all. A few features do reach the network — timestamping, timestamp verification, opening a document by URL, remote co-signing, and the update check — each one started by you and never in the background. All of them are listed in [What leaves your computer](#what-leaves-your-computer).</sub>

<sub>¶ [Reflow a paragraph](#reflow-a-paragraph) re-sets changed words in the document's own font, size, spacing and justification, removes the old words rather than covering them, and carries the text below — and what is anchored to it — down the page and onto the next. It changes only what it can set exactly: a paragraph it cannot rewrite faithfully is refused with the reason, and the older cover-and-replace [Edit text](#edit-existing-text) is there for those.</sub>

<sub>◊ The three accessibility rows are summaries. Nib's checker covers 105 of the 106 PDF/UA-1 rules the reference validator veraPDF evaluates and never shows a clause it could not check as a pass; it has no WCAG check and offers no automatic fix. Its tagger proposes headings, paragraphs, list items and tables drawn as a regular ruled grid and a figure for each picture a page draws, for you to review before anything is written — a figure is written only once you describe what it shows; no drawing made of lines and shapes is proposed as a figure, and no table with merged cells or without ruled lines — and tags an already-tagged document again once you remove its tags. Its tree editor changes an element's type, alt text, header scope, a table cell's spans and header cells, and its place, marks decoration, adds and deletes tags, and tags what a page draws that no tag owns — text the tagger missed, a picture, a drawing — chosen from a list of the page's untagged content, not by dragging on the page; it gives a tag written inline in the file a number of its own so that it can be edited, and lists and edits the role map, where a document's own tag types are mapped to standard ones. It cannot yet edit a tag's title, or reorder by dragging on the page. For Acrobat Pro every one of these is laid out feature by feature in [docs/accessibility-parity.md](docs/accessibility-parity.md), each Acrobat claim quoted from Adobe's documentation, each Nib claim naming the test that shows it, and every gap listed. The Foxit and PDF-XChange columns are read from those vendors' own feature lists, not measured: PDF-XChange Editor Plus lists a tags pane, a reading-order pane and an accessibility check, and no automatic tagging.</sub>

<sub>‖ The PDF24 Creator and PDFgear columns are read from those projects' own feature pages, help forum and changelog, not measured, and ❌ there means *not listed*. PDF24 Creator: redaction and compare are listed with no pattern search and no text diff described; signing takes a certificate but a timestamp authority is on its to-do list; it has a command-line tool (`pdf24-DocTool`). PDFgear: text editing, redaction, OCR and certificate signatures are listed, a compare tool is not; it works offline except for its cloud AI assistant, and batch work is conversion only.</sub>

<sub>\* The three signing rows are about doing it *without a service*. Acrobat and Foxit both offer send-to-sign and multi-party workflows — through Acrobat Sign and Foxit eSign, which means their servers, their accounts, and your document on someone else's machine. Nib's equivalents run between the signers' own copies of Nib, pinned to keys the signers compared themselves.</sub>

Acrobat, Foxit and PDF-XChange are mature commercial editors that do plenty Nib
doesn't aim to — full WYSIWYG content editing, prepress, cloud collaboration. PDF24
Creator and PDFgear are free, capable everyday tools, though neither is open source.
The table is about the jobs Nib *does* cover, and where it works differently.

**Choose Nib** when you want to own your tools: work entirely offline with no
account or subscription, script PDF jobs from the command line, keep every
document and signature on your own machine, and stand on a transparent AGPLv3
codebase you can read and rebuild. Reach for a commercial editor when you need
full WYSIWYG layout editing or prepress.

---

## What you can do

### Fill any PDF — even ones without form fields
Nib fills normal interactive forms (AcroForm) directly. For flat, scanned, or
print-only forms with no fields, the **Text** tool lets you type anywhere on the
page.

### Smart field detection
Press **Detect fields** (on the **Mark Up** tab) and Nib drops fillable widgets where they belong. On a form made
by a program it reads the page's own ruled lines and text — so a table cell with
its label printed inside it becomes a field under the label, exactly as wide as
the cell — and on a scan it reads the picture of the page:

- **Blank lines** → a text box above every fill-in rule, including the faint
  light-gray lines on modern forms.
- **Boxes** → a field inside each empty box (boxes that already contain text are
  skipped).
- **Tables** → one input per cell, below or beside its printed label; a cell
  two rows deep is one field.
- **Typed blanks** → a field over every run of underscores.
- **Checkboxes** → click to check.
- **Circle-the-answer choices** → `Y / N`, option sets near a "(circle one)"
  note, pipe-separated lists on their own (`$5 | $10 | $25`), a labelled run of
  options (`Type of Membership: Youth Teen Adult …`), and repeats of a choice the
  "(circle one)" governs (every `Male / Female` on the page, not just the first).

It's a smart proposal, not magic — move, resize, retype, or ignore anything it
suggests. Measured on real forms that carry their own fields as the answer key,
it finds nearly all of them and places about ninety-nine in a hundred well; it does
least well where a form marks its blanks with shading alone.

### Turn a flat scan into a fillable form
On a flat or scanned form, **File → Save a Copy → Save as fillable form…** emits a real
interactive **AcroForm PDF**. If the page has no fields yet, it runs **Detect fields** on the
page in view as its first step (you can also run Detect fields yourself, page by page, and
adjust what it finds first): every detected text box and
checkbox becomes a live, fillable field (with proper appearance streams, so it
works in Adobe and any browser), dropped right onto the original page — the scan
itself is untouched. Need a **dropdown** or a **radio-button group**? The **Dropdown** and **Radio** tools
in **Mark Up → Annotate & Draw** let you draw one and type its choices; they're
authored as a real combobox / radio group. A radio group lays its buttons out to
match the box you draw — a wide box runs them across, a tall box stacks them down.
A quick step lets
you **name each field** so the collected data is meaningful — Nib pre-fills each
name from the field's own label on the page (e.g. a box beside "First Name:" is
named `first_name`), which you can edit (focus a row to highlight that field). On
an image-only scan there's no text to read, so fields stay `field_N` unless you've
run **OCR** first. The opposite of flattening: instead of baking your answers in,
you publish a blank form for others to fill.

### Mail-merge a form from a spreadsheet
Got a fillable form and a spreadsheet of records? **Mark Up → Fill Forms from Data → Fill from spreadsheet…**
picks a CSV and fills the form once per row, handing you a **ZIP with one PDF per
row**. The CSV's first row is the form's field names (export them with *File →
Export & Print → Form data…*, choosing CSV, to see the exact names); checkbox fields take
true/false/yes/1. It runs **entirely on your machine** — the same engine as the
`nib fill` command line, just point-and-click. (The form needs real fillable
fields; on a flat scan, run *Save as fillable form…* first.)

### Exchange form data as XFDF
Trading form data with Acrobat or Foxit? Nib reads and writes **XFDF**, the XML
form-data interchange format both speak. **File → Export & Print → Form data…**, with
XFDF chosen as the format in the Save dialog, saves the open form's values; **Mark Up → Fill Forms from Data → Import form data (XFDF)…** fills the form
from an `.xfdf` file and saves the result. On the command line it's
`nib export-xfdf IN -o OUT.xfdf` and `nib fill IN --data DATA.xfdf -o OUT`.
Hierarchical (dotted) field names are preserved as nested fields; like every Nib
operation it runs entirely on your machine.

### Check a document for accessibility (PDF/UA)
**Accessibility → Check accessibility (PDF/UA)…** reports the open document against the PDF/UA-1
rules Nib can verify itself — the accessibility standard screen readers rely on. Every clause is
marked **passes**, **fails**, **does not apply**, or **Nib could not check**, and a clause Nib could
not check is never shown as a pass. Each failure names what it found and where: which font on which
page, which operator in which content stream. The report also says where the document's structure
came from — written by Nib from what it knew, read from a scan by OCR, inferred from how the pages
look and reviewed by you, or not recorded at all. On the
command line it's `nib ua IN`, which exits 1 and prints every reason when a checked clause fails or
could not be checked.

**This is a checker, not a certificate.** Nib checks 105 of the 106 rules the reference validator
evaluates, so a document can pass every clause Nib checks and still fail one it does not — though no such document is
known today: veraPDF passes all 142 files of its own PDF/UA-1 test corpus on which every clause Nib checks passes, and
the one rule Nib does not check is one veraPDF has never been seen to fail. On the rules it does check, Nib agrees with
veraPDF on 105 of the 105 — the same verdict on every document that exercises the rule, wherever Nib reached one, in veraPDF's own test corpus, in
Nib's generated test documents and in documents from real producers (Word, Acrobat, LibreOffice and others). On those rules
no disagreement is known. Where a glyph or CharSet check needs a font program
Nib cannot read (an OpenType-wrapped one holding CFF outlines) it says "could not check" rather than
passing; the embedding check opens such a program far enough to see whether it holds a font at all, as veraPDF
does, and fails it when it does not. Nib's verdicts are tested, not proven, and that is also why Nib never
writes the PDF/UA identification on the strength of its own report. Nib's answers on the clauses it does check are tested
against [veraPDF](https://verapdf.org/) wherever the test suite runs with veraPDF and the test corpora installed (a
fresh clone and the release build skip those comparisons and say so); for a conformance verdict, use veraPDF.

**The one document Nib does label PDF/UA is its own Markdown conversion**, and only when you chose the
document's language — `nib office notes.md -o notes.pdf --lang en`, or picking a Document language in the
app rather than leaving the pre-filled one. That claim rests on veraPDF measuring every construct Nib's
Markdown renderer produces as conformant, not on Nib's checker, and any later change to the document removes it.

For the same reason, **when Nib changes a document that claims PDF/UA conformance, the claim is removed**
— whoever wrote it. Nib cannot tell whether an edit kept the document conformant, and a claim that
outlives the edit that broke it is worse than no claim. The one exception is a signed document: removing
the claim would break the signature, so it is left as it was.

### Tag an untagged document's structure
**Accessibility → Tag structure…** reads the open document's pages and proposes its headings,
paragraphs and list items from how they look — larger text as headings, drawn bullets and numbers as
list items — and a table wherever the page draws one as a regular grid of ruled lines, at least two
rows by two columns: a row for each row, a cell for each cell, the first row as header cells — and a
figure for each picture a page draws. Nothing
is written while you review: each proposed element is outlined on its page, and you can change its
type, ignore it, or move it earlier or later in the reading order, all from the keyboard. A table is
shown with its rows and cells under it; a cell can be made a header or a data cell, the table is
moved or ignored as a whole, and a grid that is not a table — a ruled form's boxes — can be marked
**Not a table**, which writes its boxes as paragraphs instead. A figure has a field for its
description — what the picture shows, which is what a screen reader says for it — and is written only
with one: describe it, or tick **Ignore** and it is marked as decoration; the review counts the figures
still waiting, and a commit with one left undescribed is refused with the reason. **Commit structure** writes it and records that it was inferred, so the accessibility report
says so; **Undo** takes it back. It refuses a document that is already tagged (it will not write a
second structure over the first — **Remove all tags…**, in Review Structure Tree, takes the first away
so you can tag the document again), a signed document (tagging would break the signature), and a page
whose text is drawn in a way it cannot mark without describing something else. Text on the page that
you ignore is marked as decoration rather than left unaccounted for. The proposal is a starting point,
not an answer — multi-column pages it cannot separate, and body paragraphs with no visible spacing
between them, are the known weak spots. A ruled table with merged cells is not read as a table (the
review says so on its page and its text is proposed as paragraphs), and a table with no ruled lines is
never recognized. A ruled grid with fewer than a quarter of its cells filled in — a blank worksheet, a
form's ruling — is not proposed as a table either: what text it holds is proposed as paragraphs. A figure is proposed for a picture — an image — and never for a drawing made of lines
and shapes, which stays decoration until you tag it yourself (**Tag untagged content**, in Review Structure Tree); not for a picture smaller than 8 points either way, one drawn
inside a form (a reusable block of page content), or one on a page with no text at all; a picture drawn
turned or slanted is reported on its page and not proposed. Nib cannot tell that a picture was swapped
for another in the same place between the proposal and the commit, so a description is committed to
whatever is drawn there.

### Review and correct an existing structure tree
**Accessibility → Review Structure Tree** shows the open document's tags as a tree — whoever wrote them,
Nib or another program — in the order a screen reader reads them. Arrow keys walk it (Down and Up in
reading order, Right into an element, Left out to its parent), and the element you are on is outlined on
its page. A figure with no alternative text says so, and so does a table header cell with no scope.

Select an element and change what is wrong with it: its **type** (a paragraph that should be a heading,
say), its **alt text** (what a screen reader says for a figure), a header cell's **scope** (whether it
heads its row, its column or both), how many **columns and rows** a table cell spans, which **header
cells** head a cell (tick them; Nib writes the identifiers), its **place** among its neighbours, or **Mark as decoration** for
something that is not content at all — a running header a program tagged as a paragraph. Every change is a
button or Enter, so a keyboard user can browse the choices without changing anything, and **Undo** takes
each one back. The accessibility report shows the result: set the missing alt text and the figure clause
passes. A signed document is refused, because a correction changes the bytes its signatures cover.

The tree itself can be changed too. **Add tag** makes a new, empty tag of the type you pick, directly after
the element you are on — or last, if you are on none — and puts you on it. **Move into the tag above** puts
an element inside the one before it, which is how a new tag is filled, and **Move out one level** takes it
back out, to just after the tag it was in. **Delete this tag — keep its content** removes a tag that should
not be there and nothing else: what it held, text and tags alike, moves up to the tag above and is read
exactly as before. That is the opposite of *Mark as decoration*, which keeps nothing for a screen reader. A
move that cannot be made is greyed out with the reason beside it, and Nib refuses a delete that would leave
content with no tag to belong to — a paragraph at the very top of the tree, say — and tells you to change
its type instead.

Content with no tag at all can be brought in from the same panel. **Tag untagged content** takes a page
number; **Show untagged content** lists what that page draws that no tag owns — text gathered into
paragraphs, pictures, drawings made of lines and shapes (lines and shapes that touch are listed as one drawing), and lone rules and plain boxes last — and says which of
it the page marks as decoration (text you ignored when the document was tagged is there, for one). Each
piece is a tick box, and the piece you are on is outlined on the page. Tick what belongs together, pick a
type, and **Tag selected** makes them one new tag, placed after the element you are on, and puts you on
it. This is how a drawn graphic becomes a figure: pick *Figure* and say what it shows — a figure is not
written without a description. A piece is taken whole or not at all: Nib will not take half of something
the page marks as one piece of decoration, and it lists but cannot tag content drawn inside a form (a
reusable block of page content). It does not guess where the new tag belongs in the reading order — move it
if it is in the wrong place — and you choose from the list, not by dragging on the page.

Some programs write a tag *inline* — inside the tag that holds it, with no number of its own — and nothing
in a PDF can point at a tag like that, so it cannot be changed, moved, deleted or given anything to hold.
Select one and the panel says so, and offers **Make inline tags editable**: one step that gives every such
tag in the document a number. Nothing a screen reader reads changes — the same tags, in the same order, with
the same text — and you are left on the tag you had selected, which can now be edited like any other.

A document may also use tag types of its own — *Heading 1*, *Body Text* — and say what each means in its
**role map**. Open **Role map**, at the foot of the panel, to see every such type, the standard type it
stands for, and how many tags use it. Pick another standard type and press **Change** and every tag of that
type is read as the new one; **Add** maps a type that has no mapping, which is what a screen reader needs
before it can announce a tag of that type at all; **Remove** takes away a mapping no tag uses any more. A
custom type is always mapped to a standard one, never to another custom type, and a standard type cannot be
mapped to something else.

**Show reading order** numbers each element on the page in the order a screen reader reads it, so you can
see where that differs from the order your eye reads.

### Convert to PDF/A for archiving
Need a document that archives will still open decades from now? **File → Save a Copy →
Archival PDF (PDF/A-2b)…** converts the open document to a **PDF/A-2b candidate**:
Nib embeds an sRGB output profile, writes the PDF/A identification, and removes
active content and attachments. On the command line it's `nib pdfa IN -o OUT`.

Nib is honest about what it can and can't do here. The fast built-in converter is
pure Go, but PDF/A requires every font embedded and device-independent colour, and
Nib can't add a missing font or convert colour itself — so a document with
non-embedded fonts, DeviceCMYK colour, or encryption is **refused with the specific
reason** rather than turned into a file that falsely claims conformance.

For those harder documents, Nib can use **[Ghostscript](https://www.ghostscript.com/)**
if it's installed on your system — it re-embeds fonts and converts colour, the
general conversion pure Go can't do. It's strictly optional: when the built-in
converter refuses a document and Ghostscript is installed, the dialog offers a
*"Convert with Ghostscript"* button (the CLI has a `--gs` flag); when Ghostscript isn't
found, Nib says so, and that it would have handled the document. Ghostscript is detected at runtime and never bundled, so Nib stays a
single pure-Go binary. (Note: that path runs Ghostscript over your PDF; it executes
under Ghostscript's sandbox, but it is processing the document, so only enable it
for files you'd open anyway.)

Either way, because no pure-Go PDF/A validator exists, Nib can't certify the result
itself — it produces a *candidate* you should **verify with
[veraPDF](https://verapdf.org/)** before relying on it for archival.

For the same reason, **any later change Nib makes to a PDF/A file removes its PDF/A identification** — an edit, a
page operation, adding or removing a password, or signing it — because Nib cannot tell whether the changed file
still meets the standard. Convert (and verify) again after the last change. A document that is already signed keeps its
identification, since rewriting it would destroy the signature.

### Open an image

Open a **PNG, JPEG, TIFF or WebP** the way you open a PDF — by path, by dragging it
onto the window, or by double-clicking it when Nib is your handler. It becomes a one-page
document you can mark up, annotate, redact and export like any other.

The page is the image's **physical size**: pixels divided by the density the file
declares (`pHYs` for PNG, JFIF for JPEG; TIFF and WebP are always taken at the
default), or **96 dpi** where it declares none — so
a 1920×1080 screenshot opens at about 1440×810 points, close to the size it had on
screen. A photo carrying an EXIF orientation opens **upright**; the four mirrored
orientations are not corrected, and open as an unrotated image.

An opened image is **not saved back over itself**. The file on disk is a PNG and the
document is a PDF built from it, so **Save** downloads a PDF copy to your browser's download folder rather than
replacing your original; use **File → Save a Copy** to choose its name and place.

### Open an office or Markdown document
**File → Open & convert to PDF…** opens a Markdown, Word, Excel, PowerPoint, or
OpenDocument file (`.md/.markdown`, `.docx/.doc/.odt/.rtf/.txt`,
`.xlsx/.xls/.ods/.csv`, `.pptx/.ppt/.odp`) by converting it to PDF — it then
becomes the open document, ready to mark up, fill, sign, or save.

**Markdown converts natively**, in pure Go, with no external tool: headings,
emphasis, lists, blockquotes, and code blocks render as crisp selectable text
(images and tables are skipped). Non-Latin text prints too — Cyrillic, Greek,
CJK, Korean, Arabic, Hebrew, Thai and the Indic scripts — using the same faces
Nib already ships for OCR, embedded only when the document actually needs one. For office formats, rendering
office layout is something pure Go can't do, so Nib shells out to
**[LibreOffice](https://www.libreoffice.org/)** in headless mode; like
Ghostscript it's strictly optional and **detected at runtime** (never bundled, so
Nib stays a single cgo-free binary). Nib looks for it on your `PATH` **and** in the
place each platform installs it by default — a macOS `.app` bundle and
`%ProgramFiles%` on Windows are not on `PATH`, and until v1.133.0 Nib reported
those machines as not having it at all. When Nib can't find it, the *Open a
Document* card says so, with a link and a **Check again** button (so installing it
takes effect without restarting Nib), the file picker narrows to Markdown, and the
CLI verb says the same thing:

```
nib office report.docx -o report.pdf
nib office notes.md -o notes.pdf
nib office vertrag.docx -o vertrag.pdf --lang de
```

In the app, the **Document language** list beside *Open & convert to PDF…* does the
same: it starts at this computer's language, and whatever it names is declared on
the converted document; set it to *not specified* to declare nothing new.

`--lang` declares the language the document is written in, so a screen reader
pronounces it correctly. Without it the CLI declares none for Markdown, and an
office conversion keeps LibreOffice's, which is the converting machine's locale
rather than the document's — a German contract converted on an English machine
is declared English.

Each conversion runs in its own temporary directory with an isolated LibreOffice
profile and a timeout. As with Ghostscript, LibreOffice *interprets* the document
(a large surface, and office files can carry macros — headless LibreOffice doesn't
auto-run them at the default security level), so only convert files you'd open
anyway. Fidelity is LibreOffice's: complex documents may not convert pixel-perfectly.

### Circles & pills for multiple choice
Choosing an option marks it the way a person would: a **circle** around a single
letter (or `Y`/`N`), or a **pill** around a whole word — baked cleanly into the
PDF on save.

### One-click stamps
Quick-stamps for the things you reach for most — **today's date**, **"Approved"**,
and a **checkmark**. Drop one on, drag to place, resize to fit.

### Signing flags — sign, date, initial, name, title, company
Filling a form with the same fields on every page? Run **Mark Up → Detect fields** to find the
blanks, then switch to the **Signing** tab and in the sidebar's **Place Signing Flags** panel pick **Sign**,
**Date**, **Initial**, **Name**, **Title**, or **Company** and click a blank to
flag it — the flag snaps to that line (or click anywhere to place one freehand).
Then click each flag to fill it and Nib **jumps to the next** one: a date flag
stamps today's date; a sign/initial flag drops your signature or initials, picked
from the Library once and reused; and a name/title/company flag fills from your
**autofill profile** (add the value once under *Mark Up → Edit autofill profile* and
every such flag reuses it). Each fill is sized to fit its flag. These place a
*visible* signature for filling out the form; the cryptographic signature is still
the separate **Finalize & sign** step.

**Send it to someone else to sign** (like DocuSign, without the cloud). Plant the
flags, then click **Signing marks completed** to lock the document: flag placement
and every editing tool switch off and the flags freeze, so the layout can't drift
before it goes out (toggle **Edit marks again** if you still need to change it).
Click **Save for signing…** and email the saved file. The flags travel *inside*
that one PDF — no sidecar to lose — so when the recipient opens it in Nib it opens
**locked in signing mode**: they can fill but not edit. A banner offers **Start**,
walks them flag-to-flag with **Next field**, and ends with **Mark complete &
sign** — one step that flattens the filled document and applies the recipient's
own tamper-evident certification signature, saved as `<doc>.signed.pdf`. A
**Finish & sign** button stays available the whole time, so they can complete
even if they filled a flag from the Library or left one blank. Send the file
as-is: printing it or re-exporting it through another app strips the flags.

**Skip email entirely — send it Nib-to-Nib.** Instead of mailing the file,
**Signing → Send & Receive → Send a document to a peer…** hands it straight to a pinned peer over
the same encrypted, no-cloud channel co-signing uses (both of you online; they
pick **Receive a document…** first). Received files save into `~/nib` — a flagged
document waiting for you under `to-sign/`, a finished signature under `signed/`, anything else under `incoming/` —
so the round trip is: you send the flagged file, they fill and **Mark complete &
sign**, they send it back, and the signed copy lands in your `~/nib/signed/`. Each
hop needs both peers online; nothing is stored on a server in between.

### Signatures & images, stored securely
- **Draw your signature** on a pad; it's saved as a clean **transparent PNG**, so
  it sits *on* the line instead of inside a white box.
- **Upload a photographed or scanned signature** and Nib knocks the white paper
  background out to transparency for you — preview it, tune the threshold, and it
  sits *on* the page instead of inside a white box. (Add logos and other images
  the same way; uncheck the box to keep an image's background as-is.)
- Everything lives in an **encrypted image library** inside your vault. Click to
  place, then drag and resize anywhere.

### Annotate
**Highlight** text, **draw** freehand, add free **text** boxes, and drag a
**border** — a colored outline with no fill — anywhere on a page; works on flat
PDFs too. Highlights and borders are **any color**: pick one from the swatch row
(your last five colors stay one click away) or open the picker for a new shade.
A border's thickness is yours to set, in points. Draw **Shapes** — a line, an
arrow, a rectangle, or an ellipse (rectangles and ellipses can be filled) — in any
colour and thickness; they bake into the page so they show in every viewer. Drop a
**Note** to leave a comment — a sticky note you place, type into, and drag; on save
it becomes a real clickable sticky-note annotation (an icon whose popup shows your
text) that any PDF viewer can read.

### Edit existing text
**Mark Up → Edit text**, then drag a box over baked-in text. Nib covers it with a
fill sampled from the background and drops an editable box prefilled in the
original's size, colour, and closest font (serif / sans / mono, bold, italic) —
so a fix reads like an edit, not a patch. The page stays sharp and vector. The
original text stays underneath (it's a visual edit) until you press **Remove
originals**, which flattens just the edited pages so the old text is gone for
good — or until you flatten / finalize the whole document.

### Reflow a paragraph
**Mark Up → Reflow paragraph** lists the paragraphs on the current page. Pick one,
change its words, and Nib re-sets it **in the document's own font and size**, on its
own lines and margins — a changed word pushes the rest of the paragraph along, the
way a word processor would. Unlike Edit text, the **old words are removed**, not
covered: they are gone from the page, from copy-and-paste and from Find. Every letter
keeps the spacing it was set with — letter spacing, word spacing, condensed or raised
type — so the words you did not change look exactly as they did, and a word you type
takes the spacing of the text just before it. A word you type is **kerned** the way the page
kerns: a pair of letters the document always sets closer (an *AV*, a *To*) is set that close in
your word too — and where the page places its letters inconsistently, nothing is borrowed. A word the
document **hyphenated** across a line end ("accom-" / "modate") becomes one word again when your edit moves it
inside a line — without the hyphen where the document writes the word whole elsewhere, with it where the document
writes it hyphenated ("full-time"). If nothing nearby says which, the edit is refused with the reason rather than
guessed. A word that changes **font** part-way — an italic word with a roman comma, a footnote
number in another face, set smaller and raised — keeps each letter in its own font, size and height, and
text whose size a publishing program set through its text matrix rather than its font size is re-set at
that size too. A **justified** paragraph stays justified: every line
but the last is set out to the right margin again. A **centred** heading stays centred when you change its length (as long as it stays one line).

When the new text needs **more lines**, the paragraph grows down, at its own line
spacing, and the paragraphs below it in its column move down with it into the free
space under them; a footer or page number past that space stays put. When the page
runs out of room, the last paragraphs move whole to the top of the next page, pushing
its text down in turn, page after page — on single-column pages, only where nothing but
margin lies below the text (a signature block or a footnote below it stops the flow
rather than being jumped), onto a next page that opens with body text rather than a
running header, and not past the last page. (A page number or running header drawn
apart from the column is read as just that, not as a second column.) Notes, links, form fields, signing flags and bookmarks on or beside the
paragraphs that move go with them, onto the next page too — and in a document tagged
for accessibility, so does each paragraph's tag, so a screen reader finds it on its new
page. If something cannot move with the text — a line or picture drawn there, a note or
link half in and half out of what moves, invisible search text over a scan, or tagged
text Nib cannot re-tag correctly on its new page — it is refused with the reason rather
than left pointing at the wrong words. (The same goes for any note, link or form field
laid over the paragraph itself, which the new wrapping would move under it.)

It changes only what it can set exactly. The new text must use only characters the
document's font carries; a
paragraph drawn in a way Nib cannot rewrite faithfully (text inside a stamp, turned
or vertical text, lines tagged one by one for accessibility, text a screen reader is
told to read differently, or an invisible search layer over a scan) is refused with
the reason, and **Edit text** is there for those. In a font that carries only the
letters the document used, a letter the page never shows counts as missing. Clearing
the whole paragraph is refused too — redact it instead. A **signed** document is never reflowed — rewriting
a page would change what its signatures cover. **Undo** restores the paragraph.

### OCR — make a scan searchable
Got a scanned PDF that's just images? **Mark Up → OCR** reads the text on every page
and adds an **invisible text layer** underneath the scan, so the page still looks
exactly the same but the text is now **selectable, copyable, and findable** (and
shows up in *Find*). The OCR runs **entirely on your machine** — the recognition
engine is built into Nib (no install, no cloud, nothing leaves your computer) —
so it works offline like everything else. A page that **already has a text layer** —
from an earlier OCR, in Nib or anywhere else — is left as it is and not read again, and
Nib says how many pages that was. Where the text layer is one **Nib itself added**, Nib offers to
**read those pages again**: say yes and the old layer is taken out and a new one put in its place, in the
language and quality chosen now — the way to fix a scan read in the wrong language, or to bring a file
OCR'd by an older Nib up to date. Another program's text layer is never replaced, and neither is one in a
signed document. Pick the scan's language from the
dropdown next to the button (**English, French, German, Spanish, Italian, Czech,
Dutch, Hungarian, Polish, Portuguese, Romanian, Swedish, Turkish, Vietnamese,
Russian, Ukrainian, Bulgarian, Serbian, Macedonian, Belarusian, Greek, Thai,
Hindi, Bengali, Marathi, Nepali, Sanskrit, Tamil, Telugu, Kannada, Malayalam,
Gujarati, Punjabi, Arabic, Persian, Urdu, Hebrew, Chinese
(Simplified and Traditional), Japanese, Korean**) for
best accuracy; full Unicode comes through either way (accents, quotes, dashes, and
non-Latin scripts — including right-to-left Arabic and Hebrew, which stay searchable
in logical order, and CJK). A **quality** selector next to
it trades speed for accuracy: **Fast** (200 DPI) is the quick default; **Best**
(300 DPI) renders the pages larger so small or faint text reads more reliably — a
bit slower, worth it when accuracy matters. The OCR'd document also gets its
**language tag** set (PDF `/Lang`) so a screen reader announces it in the right
voice. It's undoable, too.

### Real redaction
On the **Secure** tab, click **Redact**, draw your boxes and press **Apply redactions**. Nib re-renders those pages flat so the
content underneath is **actually gone** — not just hidden behind a black
rectangle. (Verified: a redacted page exposes no hidden text or form field.)

Don't want to hunt for every occurrence by hand? **Redact text…** finds them for
you: type a word or phrase, and/or tick a built-in pattern — **SSN, email, US phone
number, card number** — and Nib marks every match in the document as a redaction box.
Review the boxes (remove any you don't want), then press **Apply redactions** to flatten
them for real. Each box is drawn from the page's own letter positions, so it
covers the match and stops there instead of spilling onto the words either side;
where Nib can't read those positions it falls back to a deliberately wider box
and tells you how many. It reads the text layer, so it works on any text-based
PDF — and on a scan, whose pages it **reads (OCR) first, by itself**, before it
searches: you do not have to remember to. If those pages cannot be read — the
document is signed and you say no, it is locked for signing, or the read fails —
the result **names the pages that were not searched**, so a scan is never passed
over in silence. Each invisible word is set to the
width and height of the scanned word it stands for, so a box drawn from it covers
the word (in Nib, and in any other program that reads the layer). A scan OCR'd by
a Nib before 1.189.2 has narrower invisible words; there Nib carries each box out
to the start of the next word, but another program reading that older layer still
sees the narrow words. Matches split across the page's text runs are
still caught, and so is a match that wraps onto the next line — a name whose
first word ends one line and whose second begins the next, a number broken
after a hyphen, a word hyphenated at the line end: it gets one box on each
line. A match that runs over a **page** break, or over more than three lines,
is not found — search for its parts.

### Compare two versions
Wondering what changed between draft v3 and v4 of a contract? **Page Functions → Compare…**
lets you pick a second PDF and compare it against the open document three ways,
switchable from the toolbar at the top of the dialog:

- **Text** — diffs the two **text layers** word-by-word: removed text struck
  through in red, additions in green, inline. Tells you *what* changed. Works on
  any text-based PDF (a scan with no text shows nothing — run **OCR** on it
  first, which makes it diffable). Reading order follows each document's text
  stream, so it's most reliable comparing two versions produced by the same tool.
- **Side-by-side** — renders a page of each document next to each other.
- **Differences** — paints a per-pixel **difference map**: regions that changed
  are highlighted in red, with the percentage of the page that differs. Tells you
  *where* it changed — and because it compares rendered pixels, not text, it works
  on **scanned** PDFs too. Pages that differ in size between the two documents are
  shown side by side instead (normalise page sizes first for a difference map).

In the visual modes Nib **auto-aligns the pages**: it matches each page of one
document to its counterpart in the other, so if a version inserted or deleted a
page the comparison stays lined up instead of every later page reading as changed.
The outer ‹ › then step through the matched pairs, an added or removed page is shown
on its own with a banner, and the toolbar notes how many pages were added/removed.
A page that simply **changed position** is recognised as a *move* — its banner says
where it went (and where it came from) rather than reporting it as one page deleted
and another added, so a reorder reads as a reorder.
For text-based PDFs alignment is instant — it matches on the page text. For
**scanned documents** (or a scan compared against a digital original), where there's
no text to match on, Nib instead renders every page and aligns on a **visual
fingerprint**, so two scanned revisions line up too; lightly-edited pages stay
paired (the difference map then shows what changed) while a genuinely new or removed
page becomes a gap. That render pass shows brief progress; uncheck **Auto-align** at
any time to page the two sides manually. Like everything else it runs **entirely on
your machine** — the second PDF never leaves your computer.

### Scan for hidden content
**Secure → Scan for hidden content** lists what's lurking in a PDF that you can't
see on the page: auto-run hooks (OpenAction, additional actions), JavaScript,
risky link/widget actions (launch a program, submit a form, open a URL),
embedded files, media annotations (sound, video, rich media and 3D — the last
two can carry their own scripts), optional-content layers, XMP metadata, and the document's
identifying properties (author, title, creator…). Then remove it four
ways, strongest fidelity-preserving first:
- **Strip active content** — neutralises every auto-run hook, script and risky
  action while keeping the page text and layout intact. It re-scans its own result
  and refuses — leaving the document untouched, and `nib sanitize` exiting non-zero
  — if anything active would remain.
- **Strip identifying metadata** — clears the document properties (author, title,
  creator, subject, keywords), deletes the XMP metadata — the document's own and
  any an image, form or font carries (a camera's, an authoring tool's) — and
  regenerates the document's tracking identifier, leaving the visible content
  untouched. (pdfcpu re-stamps a generic producer and the current date on write,
  so the file names Nib, not you.) It re-scans its result and refuses if any
  identifying metadata would remain.
- **Remove files & media** — deletes only embedded files and media annotations
  (file attachments, sound, movie, screen, rich media, 3D), leaving all other
  interactivity untouched. It too re-scans its result and refuses if a file or
  media annotation would remain.
- **Flatten to images** — the guaranteed-inert floor: turns every page into an
  image so nothing active can remain (selectable text is lost).

If a strip can't produce a sound document it's reported and your open document
is left untouched, so you can step down to the next method safely. Any removal
produces a new, **unsigned** copy — save it to keep the cleaned version.

### Add password protection
**Secure → Add password protection…** saves a separate **AES-256 encrypted copy**
that needs the password to open (the same password opens and owns the file, so it
controls who can open the copy and restricts nothing else — printing, copying, editing
and screen readers all work for anyone who has it). You
type it twice — Nib can't recover a forgotten one. Your open document is left
unprotected and editable; the protection is a standalone export, never combined
with signing (encrypting rewrites the file, so the copy won't carry a signature).
Headless: `nib encrypt IN -o OUT --password-file FILE` (or `$NIB_PDF_PASSWORD`).

### Remove password protection
Open a password-protected PDF and Nib asks for the password, then **unlocks the
working copy** so you can view and edit it — saving keeps it unprotected. For a PDF
that opens but blocks editing/printing/copying (owner-password *restrictions*),
**Secure → Remove password protection…** strips those flags. Both produce a plain,
unrestricted document, the same as `qpdf --decrypt`. This is for a document you can
already open or are authorized to edit — Nib only tries the password you type and
never guesses or recovers one. Unlocking rewrites the file, so an existing digital
signature won't survive — and because an encrypted document can't be inspected
until it's unlocked, Nib tells you right after if unlocking invalidated a signature
it turned out to carry.

### Attachments
**Secure → Attachments** lists the files embedded inside a PDF. **Extract** any
one to save it out, or **Attach a file…** to embed a new one (a source file, a
README, anything). Adding a file replaces the open document with a new, unsigned
copy — save it to keep the attachment.

### Sign & finalize — tamper-evident
**Finalize & sign** seals the document with a certification signature from an
identity kept in your vault and bakes in a visible watermark — a preset like
**DRAFT**, **CONFIDENTIAL**, **FINALIZED**, **COPY**, or **VOID** (or your own
text), with adjustable opacity, colour, size, and angle and a live preview —
optionally with a trusted RFC-3161 timestamp. Any later edit breaks the signature
— that's the point. **Secure → Signing certificate** saves your public certificate so others can verify it's you.
A certification can only be a document's first signature, so Finalize (and `nib sign`)
refuses a document that is already signed rather than break the signature it carries.

**Keep a copy for my records.** Off unless you tick it, every time. When ticked, Finalize also saves the exact signed
file to `~/nib/signed/` (named `kept_<document>_<date-time>-<code>.pdf`) before handing it to you, so if a signed
document ever comes back changed and the file itself can no longer show what you signed, the copy you kept can. The
copy is saved **unencrypted**, outside Nib's vault — anyone who can read your files, and your backups, can read it. If
it cannot be saved, Finalize does not sign, and says why. A kept copy proves what you had when you signed, not what you
sent.
Your copies are listed under **Secure → Sign & Timestamp → Copies kept when you signed…**, where each can be removed — permanently,
and not from any backup. When a signed document comes back, **Check a signed document that came back…** offers the
matching kept copy first among the copies to compare against.

**Sign with your own certificate.** By default Finalize uses Nib's self-signed
identity (integrity, not third-party trust). If you have a CA-issued credential,
import it under **Settings → Identity & Keys → Identity & peers… → Signing certificate** (a PKCS#12
`.p12`/`.pfx` file + its passphrase); then Finalize offers a **Sign as** choice
and signs with that certificate and its chain, so a verifier who trusts the
issuing CA sees a trusted signature. The certificate is used only for solo
Finalize — your Nib identity and pinned peers are untouched, and co-signing always
uses the Nib identity. You enter the `.p12` passphrase at each signing; the key is
never stored unencrypted. (Hardware tokens / PIV / system keystores aren't
supported — they'd require cgo, and Nib ships as one pure-Go static binary.)

Every PDF you open also shows a **signature badge**: untampered, modified, or
unsigned. Click **details** for the full picture — every signer (not just the
first), and whether each signing time is backed by an independent timestamp
authority or merely stated by the signer. A timestamp counts as independent only
when its authority's certificate chains to a root your computer trusts; one from
an authority Nib cannot verify is shown as no better than the signer's own clock,
since anyone can make such a timestamp with any date. The badge describes the copy Nib
opened — so if the file changes on disk afterwards (another program, or `nib …
-w` in a terminal), Nib says so in a banner and offers to reload it, and Save
asks before replacing the changed file with what you have open.

**Undo is one list** — **Ctrl+Z** steps back through everything you have done to the
document, newest first, whether it was a drawing, a stamp, a note, or a page operation.
Drawing some lines and then some shapes leaves one history, not two. The **Undo** and
**Redo** arrows in the toolbar walk the same list; they are greyed out when there is
nothing to undo, and Undo is outlined when earlier history had to be released to save
memory.

**Start over** — the **↻ reload** button throws away everything you have done since
opening and re-reads the file from disk. It asks first when there is
unsaved work. Undo steps back one operation at a time; this abandons all of them.

**The sidebar has two tabs** — **Pages** is the thumbnail grid (drag to reorder, and the
page number lives here), and **Functions** is everything else: the current tab's command
cards and the other panels.

**Colour swatches always offer black and white**, alongside the five most recent — the plain
two are the ones a recent-list can never keep.

**The Simple Sign checklist** — *Signing → Simple Sign* opens a page of its own, with a tab beside
the document tabs, that lists the steps of signing a document in order under four headings (once
before your first signing, prepare the document, marks on the page, seal it and keep it). Each step
is a link to the tool that does it — following one shows the document again, and the page stays
open in its tab — marked **required** or *optional*, with a line saying what it is for, and ticked
when Nib can see it is done. Steps Nib can't observe — whether you ran a
hidden-content scan, where you saved an `.ots` — show a dash rather than a tick it can't back.
You can also tick any step off yourself — a hand tick is shown as your claim rather than as
something Nib saw. It's a checklist, not a wizard: nothing is enforced, but two of the steps
(applying a redaction, and signing) are one-way doors, and the order says so. One step, *A copy
kept when you signed*, is asked of Nib when the page comes forward: ✓ when a copy you kept matches the open
document (it is that copy, or begins with it), ○ when none does — with how many are kept and a link
to them — and a dash when the document carries no signature, which includes right after Finalize,
while the open document is still the unsigned one.

**Which things in a menu open a page, and which unfold.** Something you set up or are led through
— every Settings entry, the Simple Sign checklist, the explanation of a signing ceremony — opens a
page, in the one tab the pages share. A tool that acts on the open document stays in the sidebar and unfolds
there, beside the document it works on: in *Signing* that is **Place Signing Flags** and
**Send & Receive**. Entering *Signing* still lands on the flag tools and opens no tab.

**Settings** is its own tab, and each thing in it opens a page of its own rather than unfolding in
the sidebar: **Appearance** (the theme and the sidebar's colours), **Read Aloud**, **Toggle Features** (which tabs the menu shows, the
advanced features, and updates — three cards on one page that open and shut), **Identity & Keys** (your identity and pinned peers, and the keys that unlock
your vault), **Vault** (back up and restore) and **About** (the version, what a signature proves, and
the licence and third-party notices, each shown on the page when you press its button). A page
opens where the document is shown, in a tab beside the document tabs marked *Settings* (or
*Signing*) and named for the page. **There is one such tab**: open another page and it takes the
first one's place in that tab, rather than adding a tab for every entry you click. Click a
document's tab to go back to the document — it is exactly where you left it — and the × on the
page tab to close the page. A setting is saved as you change it, so there is
nothing to save or cancel — a download folder you had typed is saved as its page is replaced. **Appearance** also sets the sidebar's card colours: leave *Sidebar colours* on *All colours*
for the six-accent rotation, or pick one hue and the cards become that colour in six steps.

**Main menu**, the first card of *Toggle Features*, is where you cut the menu down to what you use. Untick a tab and it goes from the
top of the window — nothing about your documents changes, and the keyboard shortcuts still work,
so hiding **Secure** does not stop its shortcuts. **File**, **Mark Up**, **Page Functions** and **Settings** have no box and are
always there — Settings because it is where the switch lives, so hiding it would leave you no way back. This is a separate thing from the *Advanced
features* card below it, and the difference matters — unticking a tab here only takes it out of the
menu, while switching a feature off there stops the feature itself.

**The version pill** (top right) always shows the version you are running. It is yellow
until a check runs, green when you are on the latest release, and red when a newer one
exists — hover it to see which, and click to download.

### Advanced features — off until you ask
Nib is a PDF editor first. Four of its features reach the network or the local link — **signing
ceremonies**, **finding peers on this network**, **reaching peers over the internet**, and
**timestamping** — and every one of them is **off on a new installation**. Turn on what you want
under *Settings → Toggle Features → Advanced features*.

**Off means the function stops, not that its button is hidden.** Switching discovery off closes the
announcing socket; switching the rendezvous off means nothing contacts the DHT; switching ceremonies
off stops the sweep that listens for your turn, not just the panel that shows it. The panels and
buttons go too, so the app does not offer you a flow it will refuse.

Two things worth knowing. **A ceremony already under way keeps working** — an installation that
upgrades with a live proceeding on it comes up with the three features that proceeding needs already
on, because stopping it silently and hiding the panel that would explain why is not a thing this
program will do to you. And **you cannot switch ceremonies off while one is still running**: Nib
refuses and tells you what is going on, because there is no way to record "ended because the user
turned the feature off" that would be honest to the other parties.

The `nib timestamp` and `nib watch` subcommands are not affected — they run without your vault, so
there is no setting for them to read, and typing a command is already an explicit act.

**How the pages are laid out — the View controls**, at the right edge of the toolbar. Two
layouts: **Pages**, the default, which draws each page separately with a break between them, and
**Continuous**, which joins them into one strip so the document scrolls as a single piece. The
choice is remembered and applies to every document you have open, including ones you open later.
One thing worth knowing before you switch: joining the pages removes the break Nib draws, not the
margins the document itself has, so a page's own white footer and the next page's white header
still meet in the middle.

**Present** is the third layout: one page at a time, full screen, with the menubar, toolbar and
sidebar gone. Click or press → to advance, ← to go back, **P** for a pointer that follows your
cursor, and **Esc** to leave. A small clock in the corner counts up from the moment you started —
elapsed rather than a countdown, because the number a presenter wants is how long they have been
talking. Presentation is deliberately **not remembered**: it is something you are doing now, not how
you like to read, so Nib never reopens full screen because of a meeting last week.

**Full screen** is a separate control, and that is on purpose. Presenting puts you full screen;
being full screen does not put you in a presentation, so you can read an ordinary document with the
window filling the display and everything still where it was. Most PDF viewers make these one thing,
which is why Escape is unpredictable in them.

**Read aloud** speaks the page you are looking at. It uses your browser's default voice unless you
choose another under **Settings → Read Aloud**, where you can also set the speed. The list shows the
voices your browser has on this machine; if a saved voice is not there, Nib reads in the default voice
rather than going quiet. It stops when you turn the page, switch document, or press it again. **It is not a screen reader and
does not claim to be** — it reads the page's own text layer, not the document's tags. This is a
different thing, useful for proof-reading or for taking a document in by ear.
A scanned page has no text to read, so Nib reads it (OCR) first — that one page — and then reads it
aloud; press the button again to stop. A page with nothing on it to read says so rather than falling
silent.

**The toolbar is icons.** Hover over one for its name and shortcut; when the window is too narrow
for all of them, the ones that do not fit move into **⋯ More**, where each is shown with its name.
From the left: the sidebar toggle, the document's name, find, the page you are on with
previous/next, undo/redo — and, only while a drawing or placing tool is armed, that tool's name
with a button to put it down. On the right: the layouts above, the zoom controls, then save, print,
reload and quit.

The zoom controls are out, the current zoom as a percentage (click it for **Actual size**, 100%),
in, **Fit width** and **Fit page**. Fit
width and Fit page both measure the *whole* document and lock a single scale, so a file whose pages
are not all the same size does not resize under you as you scroll past the boundary.

**Where the file commands live** — the toolbar keeps what you reach for while reading:
the document's name with a dot showing whether it needs saving, find, the page position,
undo and redo, and — at the right edge — the View and zoom controls, Save, Print, Reload and
Quit. Opening, saving a copy and exporting are once-per-document acts, so they are cards in
the **File** tab's sidebar — *Open a Document*, *Save a Copy*, *Export & Print* (Print is in
both places). A document is closed with the **×** on
its tab. **Ctrl+O** opens,
**Ctrl+S** saves and **Ctrl+F** opens find without going near either.

### Accessibility structure — what Nib does and does not claim
A PDF can carry **tags**: a structure a screen reader uses to read it in the right order, tell a
heading from a paragraph, and navigate. Nib tags the documents it writes from Markdown, the forms it
authors and the scans it recognises, can propose tags for any other document for you to review
(**Tag structure…**, above), and lets you correct the tags a document already has (**Review Structure
Tree**, above). This section is about what happens to those tags when the document is edited.

**Nib will not claim tagging it has not got.** Most editing operations keep a document's tag
structure — rotate, optimise, set the language, add a note or an attachment, stamp a watermark or
page numbers, and the tags come through intact. **Printing 2-up keeps them too**, which is the one
you would expect to lose them: the pages are composed onto new sheets and the structure is carried
across with them. **Choosing which pages to keep now keeps the tags too** — extracting pages,
deleting pages, reordering, duplicating a page, splitting into separate files and making a booklet:
the structure of the pages you keep is carried onto them, and the structure of the pages you removed
goes with those pages.

**Cropping keeps them too**, and so it does the links, comments and form fields on the cropped
pages: a crop only moves the edge of the page, so everything it hides is still in the file and the
tags go on describing it — use **Redact** when the hidden part must go.

**Combining documents keeps them** where they can be kept. The tags of the first document — or of the
document you are inserting into — carry through, and a tagged document added to it brings its own
tags along, read where its pages are — or, if its tags cannot be joined to yours cleanly, arrives without
them while yours stay exactly as they were. An untagged document added to a tagged one leaves its own pages
without tags, and Nib does not start claiming a document is tagged just because a tagged page was
added to an untagged one. **Splitting one page into tiles** keeps the rest of the document's tags; the
tiles themselves have none, because each tile holds the whole page's content and tagging each one
would make a screen reader read that page once per tile.

**Co-signing and signing ceremonies keep them too**: the pages Nib adds — the note explaining what the
signatures prove, the ceremony page and the signature pages — are tagged as well, so a screen reader can
read them. (The signature boxes themselves are not yet tagged.)

**Redaction still loses them**, and removes the *claim* along with the structure rather than leaving
the file looking accessible: it destroys page content by design, so no structure describing it could
still be true. A screen reader told a document is tagged stops reaching for the fallbacks it would
otherwise use, so a false claim is worse than a visible loss.

Two honest limits on what a carried structure can promise. A description written about a section that
spanned pages you removed still describes that whole section, because there is no way to trim such a
description without making it wrong rather than merely incomplete. And duplicating a page gives the
copy its own structure, read item by item alongside the original's rather than as a second block.

**And it tells you when it happens.** If you open a document that arrived with accessibility
structure and an edit removes it, a notice stays on screen — not a message that flashes past — until
you dismiss it, so you find out while you can still do something about it. Nib cannot put the
structure back; what it can do is not let you ship the loss unknowingly.

### Timestamp with OpenTimestamps — prove *when*
**Timestamp (OpenTimestamps)** creates a small `.ots` proof that anchors your
document's hash to the Bitcoin blockchain, so anyone can later confirm the exact
file existed, unaltered, by that time — with no certificate, no account, and no
trust in Nib. Only a SHA-256 **hash** of the document is sent to the public
OpenTimestamps calendar servers; the document itself never leaves your machine.
The `.ots` is a sidecar — it never touches the PDF, so it can't disturb a
signature — keep it alongside that exact file. The proof becomes fully verifiable
a few hours after the next Bitcoin block confirms it. It proves *when* a document
existed, not *who* wrote or signed it — that's what signing and co-signing are for.

**Verify a timestamp** checks an `.ots` against the open document right inside Nib:
it confirms the proof is for that exact file and reports the Bitcoin block time it
was anchored at. Only a public block height is looked up over the internet — never
the document or its hash — and the block is confirmed against three public block
explorers run by independent operators, at least two of which must agree, so no
single explorer can spoof a result (point it at your own Esplora endpoint to
verify trustlessly). If the proof was still *pending* (stamped but not yet anchored
when you last saved it), verifying it once a Bitcoin block has confirmed it lets you
**save the now-complete proof** — a self-contained `.ots` that no longer needs any
calendar server to verify, ever. You can also verify with any other OpenTimestamps
tool (e.g. [opentimestamps.org](https://opentimestamps.org)) — the proof is standard.

### A signed document that came back
You signed a contract, sent it off, and it came back. **Sign & Timestamp → Check a signed document that came
back…** (also under **Send & Receive**) opens a sheet about the open document that answers the questions a dispute
asks, in words you could read out:

- **Is the version you signed inside this file?** Nib looks for the signature made with your identity (or the
  signing certificate you imported) and recovers the exact bytes it covers. *"This file is exactly the version you
  signed"*, *"… and N bytes were added after it — your signature does not cover what was added"*, or *"The version you
  signed is not inside this file"* when the file was rewritten wholesale and the signed bytes no longer exist in it.
  A file with no signature says so *as it stands* — which never means nobody signed it.
- **Who else signed, and when?** Every signature is listed in the order it covers the file, marked *yours*, *known to
  this machine* or *not known to this machine*, and *before* or *after* yours. The word "Untampered" never appears
  here: a stranger co-signing after you is exactly the case this sheet exists for.
- **What changed?** *Compare the version you signed with this file* opens **Compare** with what you signed as the
  starting point, so text in green is text that is in this file and was not in what you signed. When the signed
  version cannot be recovered, the sheet offers other copies in order: **a copy kept when you signed** (if you ticked
  "Keep a copy for my records"), **this machine's copy from the signing ceremony** the document belongs to (found from the ceremony
  record inside it, and only when the stored ceremony is the same proceeding — the copy this machine last stored, which
  is not necessarily the final copy everyone signed), or **a file you choose**.

Nothing is checked until you open the sheet; opening a document never runs it.

### Co-sign with a peer
Two people can sign the *same* document, each attesting — in a visible block and a
cryptographically-signed reason — that they accept the other's identity. Nib names
each identity with **six words** derived from its signing key. Read yours aloud once over a
channel you both trust — a call, or in the room — and pin theirs under **Identity & peers**.
The full key fingerprint is there too, under *Fingerprint (advanced)*, for pinning by hand.

There are two ways to exchange the document:

- **Pass the file** — you co-sign, then send the PDF to the other person (email, USB,
  Signal); they co-sign and send it back. Nothing but the file moves, and the result
  verifies on its own, with no server in between.
- **Live, over an encrypted channel** — co-sign in real time without passing a file.
  One person **arms to receive** (Signing → Send & Receive → *Receive a live co-signature…*), the other
  **dials in** (Signing → Send & Receive → *Co-sign live with a peer…*). The connection is mutually
  authenticated TLS, pinned to each other's identity key: an unpinned peer is dropped
  at the handshake, before any document bytes are exchanged. The receiver reviews the
  exact document and accepts or declines — nothing is signed without that consent —
  and the session tears down after a single exchange. **The co-signed document arrives
  alongside whatever you already had open, not on top of it** — your own document stays
  open, with anything you had typed or marked on it untouched.

**Reachability for live sessions.** Nib operates no relay, rendezvous, or NAT-traversal
infrastructure of its own — that would mean a server, and Nib has none. (It can *borrow*
someone else's: the peer-finding work uses the public BitTorrent DHT as a meeting point,
which is described under [What leaves your computer](#what-leaves-your-computer). Nothing
in it is run by us.) Leave the address blank and Nib picks a port and
looks for the other person itself — on the local network, and over the internet — once those are
switched on under **Settings → Toggle Features → Advanced features**. A typed `host:port` is the fallback, and then
the receiver makes it reachable one of two ways:

- **Port-forward** the chosen port on their router to their machine, or
- **Share a private network** you both already trust — a VPN such as **Tailscale**
  or **WireGuard** — and bind / dial the address it hands you.

If the receiver is behind **CGNAT** (common on mobile and some ISPs), port-forwarding
won't work — use the VPN path. The security model doesn't depend on *how* you reach
each other: the pinned-key handshake holds over any transport.

### Sign with several people — a ceremony

Co-signing above is two people. When a document needs **three or more** signatures — a
lease with a guarantor, a deed with witnesses, a resolution with a board — Nib runs it as
a **ceremony**: one named proceeding, one roster, one document, passed from party to party
in roster order until everyone has signed.

**Where it lives.** *Signing → Start or join a ceremony* opens a page that explains a ceremony — what it
will and will not do, what the finished document shows, what leaves your computer — and starts the
two ways in, *Convene a ceremony…* and *Accept an invitation…*. (Both the entry and the panel are
there only while **Signing ceremonies** is switched on under Settings → Toggle Features → Advanced features; with it
off, a page left open says so and offers the switch.) The running ceremony lives in the sidebar,
under *Signing → Signing Ceremonies*. The panel lists every ceremony on this machine with
its roster, your position in it, and a *"what happens next"* control that asks Nib whose turn it is
— the same question the software itself refuses out-of-order contributions with, rather than a
second answer computed for the screen.

**You can give a ceremony a name.** The recital — *"We agree to the lease of 14 Elm Row, Edinburgh,
for a term of five years"* — is the sentence every party signs, and it is the wrong thing to scan a
list of proceedings by. A name is the handle: it heads the card, the recital stays underneath it,
and it is **stored on your machine only**. Nobody else sees it, it is not part of what anyone signs,
and two parties are free to call one ceremony different things. Clearing the name puts the
agreement back at the top of the card, and a name given while a ceremony was running stays on it in
the finished list afterwards — where it is the only line a person recognises, since a closed-out row
otherwise carries an outcome and a date and nothing else.

**It renders with the vault locked**, deliberately: you can open Nib, see that a proceeding exists
and where it has got to, and be asked for your password at the moment you sign rather than at the
moment you look. Finished ceremonies stay listed, because a close-out **moves** a ceremony's folder
rather than deleting it — on every machine but the convener's, that folder holds the only copy of
that party's own signature.

**How it runs.** One party **convenes**: they choose the document, write a short recital of what is
being agreed, tick the parties out of their own peer list — **no fingerprint is typed and none is
shown**; a person is a name and, where it matters, a capacity like *as Director* — and set a
deadline. Nib produces one **invitation per party** — each different, each carrying that party's own
secret — and the convener sends them out however they like. Each party **accepts** their invitation
by pasting it into the same panel, which pins the convener's identity and nothing else: everyone
talks to the convener, not to each other, so a nine-party ceremony is nine pinned relationships and
not thirty-six.

**An invitation is a channel secret, not a signing credential.** Forwarding one lets somebody watch
for that party's turn; it does not let them sign as them, because signing needs their key.

Then the document travels. The convener passes it to the first signing party, who reviews
it and signs; it comes back; it goes out to the second; and so on. **One instrument, in
sequence.** Every hop is a mutually-authenticated encrypted connection to a party whose key
the invitation named, and every signature is checked against the ceremony's own record
before the next hop starts — including the document's own bytes, so a party cannot be handed
a different document under the same proceeding.

When the proceeding ends, the convener runs a **delivery round** from the Ceremony panel —
*Send everyone their copy* — and every party gets the finished document. A proceeding every party
has signed is **complete**, and the convener's Nib records that the moment the last signature
arrives: from then on it can be delivered and no longer stopped, because stopping tells every
party it ended before everyone signed. Nib reports each party
separately, because a round that reached three of four is not a failure: you are told who has
their copy, who already had it from an earlier run, and who could not be reached and why. Pressing
it again retries only the ones still missing; a party who already acknowledged is skipped rather
than sent a second copy. **The re-run is the remedy and it is yours to press** — Nib does not
retry on its own, so a party who was offline stays listed as unreached until you run the round
again. A leg to a party who is not listening can take a few minutes before it gives up.

**While a delivery round runs, Nib says who it is reaching.** A round is one attempt per party and
a party who is not listening can hold it for several minutes, so the panel names the party, its
place in the round, and how long it has been trying against the point where Nib gives up on them —
because a spinner that never changes cannot be told from one that has hung.

**A peer that keeps dropping cannot make Nib spin.** Both of the waiting side's retry paths pace
themselves — a small wait that grows to a two-second ceiling — so a party whose connection is
flapping is retried steadily rather than as fast as the network can fail, for as long as the
ceremony has left.

**When nothing happens, Nib can test the network itself.** A ceremony that fails on a local
network is silent by nature — a firewall, a VPN swallowing the group, a connection with no carrier
— and until now the only way to tell those apart was `nib discover` at a terminal. The waiting
screen has a **Check this network** button that announces and listens for three seconds and then
says which it is: nothing left this machine, nothing came back to us, or everything works and
nobody else is here. It shows the counts it drew that from, so you can quote them.

**Before you sign, Nib shows you everyone already on the document** — not just whoever handed it to
you, who under a relayed ceremony may be a convener who signs nothing at all. A signature that does
not verify is listed and marked rather than quietly dropped.

**While a ceremony is live, its document is frozen.** Editing, redacting, sanitising or saving over
it are refused, and the refusal says so and names the proceeding — because every other party was
invited to sign *those bytes*, and changing them would break their copies rather than yours. The
ceremony's record is listed in the attachments panel for what it is, so it is not a mysterious
embedded file. And your copy of a proceeding that is still travelling is **named in progress**,
never as the finished document.

**Six ways a ceremony ends**, and Nib distinguishes them because they call for different
actions:

- **Completed** — everyone obliged has signed and the finished document has been delivered.
- **Declined** — someone refused. The convener attests to that with their own signature and
  delivers *the attestation* to everyone who had already signed, so nobody is left believing
  the document is still travelling. **Their signatures still stand**; a decline does not
  unmake a signature already given.
- **Expired** — the deadline passed. Any party's own Nib refuses a contribution after it,
  with no need to ask the convener.
- **Abandoned** — nothing was ever heard. This one is a conclusion your own machine draws,
  not something anybody attests, because the party who would attest is the one that stopped
  answering.
- **Stopped** — the convener ended it before everyone had signed (*Stop this ceremony*).
  The convener attests to that, as for a decline, and signatures already given still stand.
- **Left** — you stopped taking part on this machine (*Leave this ceremony*). It is not a
  decline, and it is your own machine's record rather than anything anybody signed.

**What it will not do, and these are limits rather than missing features:**

- **The deadline cannot be extended.** It is inside what every party signed. Moving it would
  mean a different proceeding, so a ceremony that runs out of time is convened again.
- **A party cannot be replaced.** The roster is signed too. Swapping a signer after the fact
  is exactly the substitution the identity pinning exists to refuse.
- **Ending early means starting again from the unsigned document.** There is no way to
  reopen a proceeding that has ended, and the signatures on a partly-signed document cannot
  be carried into a new one — a signature is over *that* document in *that* proceeding.
- **Nib executes one instrument in sequence, not in counterparts.** Practitioners often
  circulate an identical document to everyone at once and staple the signature pages
  together; Nib does not do that. One document goes round one time.

**What is kept, and where.** Each party's Nib keeps a folder under `~/nib/ceremonies/` while
a ceremony is live: the record, and the document as they last held it. That folder is what
lets Nib pick up where it left off after a restart. **The invitation's secret is never
written there** — it lives in your vault, sealed to your SSH key.

When a ceremony ends and its delivery round has finished — or three days after its deadline,
whichever comes first — the folder is **moved, not deleted**, to `~/nib/ended/`. That matters more than it sounds: on every machine but the
convener's, that folder holds the only copy of *your own* signature on a proceeding that was
declined or abandoned, and nothing has carried it anywhere else. Nib also leaves a small
note beside it saying how the ceremony ended and when your machine decided so. Nothing ever
removes what was moved — that is your file, and deleting it is your decision.

**What the document itself proves — and what it does not.** A finished Nib PDF proves that
its signatures are intact, that they all commit to the same proceeding, and which of the
roster's obliged parties have signed. It does **not** record how the ceremony ended, and it
cannot: nothing may be written into a PDF after its last signature without breaking that
signature, and three of the six end states are conclusions nobody can sign. So when
`nib verify` tells you a ceremony was declined, it is reading your own machine's records and
it says so, under a heading of its own. **Run the same file on a machine that was not part
of the proceeding and that line is simply absent** — which is the honest answer, and the
reason it is separated from everything the document says about itself.

### Open a document
**Open…** (File tab, or Ctrl+O) takes a typed path or URL, or browses your
filesystem. Browsing opens the file *by path*, so it can be saved back in place
and is listed in **Open Recent…** (the button beside Open…) — unlike dragging a file onto the window, which
uploads a copy with nowhere to save to.

**Closing Nib closes its documents.** Starting it again opens only what you asked it to open.
What was open when you closed — the files, in tab order — is offered back as **Resume last
session** in the empty window. It reopens them from disk: a document that never had a file
behind it (a dragged-in copy, a combine, a conversion) and an edit you did not save are not
part of it. Reloading the window is not closing it, and keeps everything.

A file that isn't a PDF is refused with a message rather than opening an empty
viewer — the header is looked for in the first 1024 bytes, the same window
pdf.js allows, so a document with a little junk before its header still opens.

### Several documents at once
Opening a document **adds** it — the one you had stays open. A strip of tabs
appears above the page as soon as a document is open, and clicking one switches to it.
Each document keeps its own scroll position, page, zoom, form fills and typed
overlay values while you are on another, because Nib hides the document you leave
rather than tearing it down and rebuilding it later.

A **reload of the browser window brings them all back**, on the document you were
looking at. Nib's documents live in the local Nib process rather than in the page,
so refreshing — or recovering from a browser crash — asks it what is still open
instead of starting empty.

Nib holds **eight documents, or 512 MB of them, whichever comes first**, and says
so rather than degrading. Two limits because one does not bound the other: eight
documents is anywhere from a few hundred KB to well over a gigabyte, since Nib
accepts documents up to 200 MB each.

The **×** on a tab closes that document and moves to the next tab; with two or
more open, **Close all** at the end of the tabs closes every one — every document, that
is: a Settings or Signing page open in the page tab is not a document and stays. Either way, if
a document has changes you have not saved, Nib asks first. Saving it answers the
question; a copy *downloaded* from a document with no file behind it does not, because
Nib cannot tell that the download landed. Closing the last document returns the viewer to "Open a PDF to begin."
with Nib still running, ready for the next file.

Opening a PDF from your file manager reaches the Nib you already have running and
adds it to that session, rather than starting a second one.

The same folder browser backs every destination picker (Save As, and both
splits). On Windows it lists your **drives** once you reach the top of one:
there's no single root there the way `/` is on Linux and macOS, so without that
a document on `D:`, a USB stick or a mapped share couldn't be reached by
clicking. And a folder Nib can't read says so — "you don't have permission to
read that folder", "that's a file, not a folder" — instead of looking empty.

### Pages & export
**Combine PDFs…** (Page Functions tab) assembles several documents into one: add the files,
arrange them with ↑ ↓, and they merge top-to-bottom into a new document — then
reorder individual pages across them by dragging thumbnails. It works even with
nothing open, and the result is a new, unsigned document (Save As to keep it).

Changed your mind? **↶ Undo** / **↷ Redo** (the arrows in the toolbar, or Ctrl+Z /
Ctrl+Shift+Z) step back and forth through document operations — rotate, delete, reorder,
crop, split, page numbers, outline and metadata edits — for the open document.
The same Ctrl+Z also undoes overlays you place — a stamp, border, shape, note,
cover-edit, or sign/date/initial flag — and dragging or resizing one, so one
keystroke walks back your most recent change whichever kind it was. Each open
document keeps its own history, up to 30 steps; it goes when you close the document or
run a content-destroying step (redaction or flatten can't be undone), and Nib may drop
the history of a document you are not looking at to save memory. While you're drawing pdf.js annotations (text boxes,
highlights, ink) those keep pdf.js's own Ctrl+Z; typing in a field uses your
browser's normal undo.

Rotate, delete, **append**, and reorder pages — **drag a page's thumbnail** in the
sidebar's **Pages** tab to move it where you want. Rotate every page at
once with **Rotate all ↺ / ↻** in **Rotate All Pages** on the **Page Functions** tab, or
hover a thumbnail to rotate (either direction) or delete a single page.
**Shift- or Ctrl/Cmd-click thumbnails** to select several at once, then
rotate, delete, or **move the whole selection to the front or back** (⤒ / ⤓) from the
bar above the thumbnails — or **drag a selected thumbnail** to slide the whole group to
any spot, keeping its order. **Extract pages…** saves a range (type `1-3, 5`) as a new PDF without
touching the open document, and **Blank page before** / **Blank page after** drop a fresh page —
matching its neighbour's size — on that side of the current one. **Duplicate page** drops a copy of the
current page right after it, and **Insert PDF before…** / **Insert PDF after…** splice another PDF in on
that side of the current page (before page 1 to prepend a cover, after the last page to add to the end).
**Page numbers…** stamps a running
number onto every page at the corner you choose — add a prefix and a zero-pad
width for Bates numbering (e.g. `ABC` + width 6 → `ABC000001`), or tick "of N" for
classic "Page 3 of 10". **Page labels…** sets the document's *logical* page
numbers — the ones a viewer shows in its page box and thumbnails, as distinct from
ink stamped on the page — so front matter can read i, ii, iii while the body reads
1, 2, 3. Add a range per section (from which page, what style — decimal, upper/lower
Roman, upper/lower letters, or a prefix-only label — and where its count starts);
pages before the first range carry no label. Going the other way, **Pages per sheet…** combines several pages onto
each sheet (2-up, 4-up, up to 16) for printing or handouts, in reading order, with
an optional border — each sheet keeps the document's page size. Got a document
whose pages are all different sizes? **Normalize sizes** (Page Functions tab) resizes every
page to the document's most common size, scaling each page's content to fit and
centring it — a one-click way to make a mixed-size scan or merge uniform. It keeps
each page's orientation (a landscape page stays landscape) and the text stays live
and selectable. **Crop…** trims
the margins away — draw a box around the part to keep and every page (or just the
current one) is cut down to it. The box is taken as a proportion of the page, so a
document with mixed page sizes keeps the same relative region on each. It's a
re-crop, not a re-render, so quality is
untouched; the trimmed-off content is hidden behind the smaller page, not
destroyed — links, comments and form fields stay too — so use **Flatten** or **Redact** to remove
it for good. Got a scanned 2-up
or 4-up sheet? **Split page…**
(on the **Page Functions** tab) cuts the current page into a grid of separate pages — pick the
columns and rows, preview where the cuts land, and
optionally resize each piece to a full page. Not a clean grid? **Split by box**
lets you split a page by hand — drag a rectangle around each region you want, then
**Apply box split** and the page is replaced by those regions, each as its own
page. The new pages all come out the **same size** (the largest region's, smaller
ones centred and padded), so the output is uniform. It's a re-crop, not a
re-render, so every piece keeps its original quality.

**Edit the outline** — the **Jump to Section** card (sidebar, File tab) lists a PDF's
bookmarks; **Edit outline…** opens an editor to author them: add a bookmark for any
page, rename, delete, and indent to nest (chapters → sections). Bookmarks stay in
page order and jump to the top of their page; saving replaces the document's outline.
**Bookmarks survive a page operation.** Reorder, delete, extract or duplicate pages and
the outline comes with the pages it still points at — a bookmark whose page is gone is
dropped, and a heading is kept as long as something under it survives. Nothing is
silently re-aimed: a bookmark that appears still goes where it always went.

**And a hidden layer stays hidden.** If a document has optional-content layers switched
off — a draft watermark, a markup layer, an alternate language — reordering, deleting,
extracting or redacting pages leaves them switched off. (**Strip active content** still
reveals them, on purpose: that's what it's for.)

**Split by bookmarks** (File → Export & Print) turns one bookmarked PDF into a folder of
separate files — one per top-level bookmark, named from the bookmark with an
optional prefix. Point it at a scored orchestration and get one PDF per
instrument/part in seconds; pick the destination folder, and the open document is
left untouched. No bookmarks? **Split by page range** (File → Export & Print)
divides the page sequence instead — **every N pages**, or **custom ranges** like
`1-3, 4-8, 9-10` where each range becomes its own file — into a folder, the open
document untouched.

Either split will replace files already in the destination folder — it says so
before it starts — with one exception it refuses outright: a part whose name is
**the document you're splitting**. That would swap the whole document for one
piece of itself, so Nib stops before writing anything and asks for a different
prefix or folder. `nib split` and `nib fill --out-dir` refuse the same way.

**Flatten** to a guaranteed-flat PDF, or
export pages as PNGs (single or ZIP) and form data as JSON / CSV. Save back over the
original, or as a flattened or editable copy. **Print** the current document —
fills, signatures, and all — straight from the **File** tab through your
browser's print dialog.

**Reduce file size** (File → Save a Copy) shrinks a PDF two ways and shows the
before→after size before you save. **Optimize** is lossless — it strips redundant
data and keeps selectable text, but mostly helps bloated files (little effect on
scans). **Compress** re-renders pages as JPEG images at a chosen quality: big
savings on scans, but it flattens the document, so selectable text and search are
lost — best for scanned/image-heavy PDFs.

**Pull the contents out** (File → Export & Print): **Document text (.txt)** dumps the
document's text layer to a plain-text file, and **Pictures in the document (ZIP)** bundles
the pictures inside the PDF into a zip — JPEGs come out as-is, other images are
re-encoded as PNG/TIFF. Text extraction reads the text layer; scanned
(image-only) pages are read (OCR) first, which adds a text layer to them, and
complex multi-column layouts may not preserve reading order.

**This page's table → spreadsheet** (File → Export & Print → *This page's table…*)
clusters the current page's text into rows and columns and saves it as an Excel
**.xlsx**, an OpenDocument **.ods**, or a **.csv** — one button, and the Save dialog's
**Format** line chooses which. It's a **best-effort** extraction of
grid-style tables — merged cells, multi-line cells, and irregular layouts may come
out wrong, so **review the result**. Like the text export it reads the text layer
(a scanned page is read first, that one page), and like everything else the spreadsheet is built on
your machine (no office-suite dependency — Nib writes the minimal file format
itself).

### Find in the document
Click the magnifier in the toolbar (or press **Ctrl/Cmd+F**) and type to highlight every match. Step
through them with the **‹ ›** buttons or **Enter** / **Shift+Enter**, and the
readout next to the box shows which match you're on out of the total (`3/12`).

### Move around a zoomed page
When the page is larger than the window, the pointer over it is a hand: press and drag to
move the page. A drag that starts on text selects the text instead, and a field, a link or
anything you have placed keeps its own drag. On a page that is mostly text, drag with the
**middle mouse button** — it moves the page from anywhere. With a drawing tool armed the
press belongs to the tool.

### Keyboard shortcuts
- **PageUp / PageDown** — previous / next page; **Home / End** — first / last page.
- **Ctrl/Cmd + + / − / 0** — zoom in / out / fit to width.
- **Ctrl + scroll wheel** (or trackpad pinch) — zoom the document at the cursor.
  Nib intercepts this so the document zooms crisply instead of the browser
  scaling the whole UI.
- **Ctrl/Cmd + S** save, **+O** open, **+F** find, **+B** toggle the sidebar.
- **Tab** into a page thumbnail's rotate / rotate / delete buttons. They are shown
  on hover, but they stay in the tab order — per-page rotation is not available
  anywhere else, so it must not need a mouse.

Navigation keys stand down while you're typing in a field or a dialog is open, so
they never get in the way of editing.

### How the commands are arranged
The commands live on seven tabs across the top — **File**, **Mark Up**, **Page
Functions**, **Accessibility**, **Secure**, **Signing** and **Settings** — with a sidebar
carrying the panels for the tab you are on. There is no layout choice: an earlier build offered
Menus / Toolbar / Both, and the picker was never implemented on the client side.

**Accessibility is the odd one out, deliberately.** Every other tab is named for something you
do to the document; that one is named for a property of it, because the three controls it holds
— proposing tags, checking the result, correcting an element — otherwise sit in three different
tabs (ADR-035).

### Dark or light
Tap the **sun/moon** button in the top-right to switch between the dark
(Catppuccin Mocha) and light (Catppuccin Latte) themes. The choice is saved in
your vault. (Defaults to dark.)

### Private by design
- The web interface binds **`127.0.0.1` only** — never reachable from the
  network; every request needs a per-process token that the window Nib opens
  obtains with a single-use key in its address, and writes also need a
  loopback-origin check. The only other ways to the token are the log line a
  windowless (`NIB_NO_BROWSER`) run prints for whoever started it, and a second
  launch of Nib by the same user — so another program running as you can still
  reach Nib, but another user on the same machine cannot. Two deliberate exceptions, both of which you start and neither of which
  runs in the background. First, a **live co-signing session you arm yourself**:
  while armed, Nib opens a single routable listener that accepts only the one peer
  whose key you pinned, and tears it down after one exchange (see *Co-sign with a
  peer*). Second, **finding a peer for a remote ceremony**, which speaks to the
  public BitTorrent DHT — that one is worth reading in full under
  [What leaves your computer](#what-leaves-your-computer).
- Your image library, signing identity, autofill profile, and recent files live
  in one **AES-256-GCM vault**, encrypted at rest.
- The vault is **sealed to your SSH key**: it unlocks at startup with *no
  password*. Authorize more than one key to use it across machines, and back it
  up / restore it fully encrypted. (Lose every authorized key and the vault is
  unrecoverable — by design.)
- **Passphrase-protected SSH keys are supported.** If your unlock key is
  encrypted with a passphrase, Nib prompts for it at startup and decrypts the key
  *in memory* — the key file stays encrypted on disk, so a stolen disk plus key
  file still can't open the vault without the passphrase. (An unencrypted key
  keeps the no-prompt startup.) `ssh-agent` is not used: the vault is unlocked by
  decrypting to the key, an operation agents don't perform.

### What leaves your computer

Nib edits, redacts, OCRs, signs and exports **on your machine**. It is not a program that
never touches the network, though, and the list below is meant to be complete rather than
reassuring — every one of these is something *you* start, none of them runs in the
background, and Nib has no telemetry, analytics or crash reporting of any kind.

| When | What goes out | Who receives it |
|---|---|---|
| At startup, unless turned off | A version query | GitHub |
| You click the version pill to update | The download of the new build | GitHub |
| **You timestamp a document** (`nib timestamp`, or Finalize with timestamping) — in the app, only once Timestamping is turned on under *Settings → Toggle Features* | A **SHA-256 of the document** — never the document | four public OpenTimestamps calendar servers |
| **You verify a timestamp** — in the app, only once Timestamping is turned on under *Settings → Toggle Features* | The transaction/block lookup for the proof | up to three public block explorers |
| **You Finalize with an RFC-3161 timestamp authority** | A **digest of the signature** | the TSA URL *you* typed |
| **You open a document by URL** | The request for that document | the host you named |
| **You run a co-signing session** | The document itself, to your counterpart, over a channel pinned to their key | the peer you pinned — and anyone who scans the port can see it is open |
| **You arm a ceremony with an invitation** — only once *Reach peers over the internet* is turned on under *Settings → Toggle Features* | Queries that reveal this machine's public IP, and — **only if the local network does not answer first** — one small encrypted record naming the address you can be reached at | strangers on the BitTorrent DHT |
| **You run `nib rendezvous`** | Queries that reveal this machine's public IP — and with `--self-test`, one small encrypted record too | strangers on the BitTorrent DHT |
| Never, under any circumstances | Telemetry, analytics, crash reports, usage data, your document contents to *us* | — |

The row that surprises people is the co-signing one: **remote co-signing sends the document
to the person you are signing with.** That is the feature. The pin means only they can
receive it, and nothing else leaves — but "your documents never leave your computer" is not
true of a flow whose purpose is to hand a document to someone else, and it would be
dishonest to print it here.

#### The BitTorrent DHT

To sign with someone who is not on your network, two copies of Nib have to find each other
without a server in the middle — Nib runs none and does not want to. The design uses the
**public BitTorrent DHT**, a large open index anyone may read and write, as a meeting point:
each side publishes a small encrypted record saying where it can be reached, and reads its
counterpart's.

Stated plainly, because you cannot consent to what you have not been told:

- **The record's contents are encrypted** under a key derived from the invitation you and
  your counterpart exchanged privately. Someone holding neither sees opaque bytes.
- **The fact that you are there is not hidden.** Publishing means speaking to strangers'
  computers, and they learn your public IP address and roughly when — the same way they
  would for anyone using the DHT. Encryption protects the message, not the envelope.
- **Nib does not store other people's data there.** It publishes its own record and asks
  questions; requests to store someone else's bytes are refused. It does answer ordinary
  DHT queries while it is running, and it serves its own record for about two hours, so it
  is a participant rather than a pure spectator.
- **"Records expire" is not a delete button.** Your Nib stops republishing and the record
  carries a signed expiry that other Nibs refuse to act on once it passes — but the copies
  already handed to strangers age out on their own schedule. There is no recall, and nobody
  could honestly offer you one.
- **Two things in Nib join the DHT: an armed ceremony that was given an invitation, and the
  `nib rendezvous` diagnostic.** Neither does so at any other time.
  - **An armed ceremony** joins only while it is armed, and only if you pasted an invitation
    — arming for a peer you typed an address for, or one on your own network, touches the
    internet not at all. It **waits for the local network first**: if your counterparty
    reaches you within a couple of seconds, nothing is published, which is the ordinary case
    for two people in one office. Otherwise it publishes one small record saying where you
    can be reached, encrypted under a key derived from that invitation, at a location nobody
    without the invitation can compute.
  - **`nib rendezvous`** only asks questions by default and publishes nothing. With
    `--self-test` it also publishes one throwaway record and fetches it back, so the counters
    it reports have a live path — it prints a notice before it opens a socket, and that
    record is tied to no ceremony and to no identity of yours.

  **The honest part about the record, which the encryption does not cover.** Its *location* is
  a value only your ceremony can compute, but the nodes holding it can see that one other
  specific address came looking for it. So a handful of strangers can tell that two particular
  IP addresses are in a ceremony together, and roughly when — not who you are, not what the
  document is, and not what the record says. For most documents that is nothing. If who you
  are signing with is itself the sensitive fact, sign on the same network or over an address
  you exchange yourselves.

If none of that is acceptable for a particular document: sign it locally, or on the same
network, or over an address you type yourselves — and don't run `nib rendezvous`. Those paths
use no internet at all. What pulls in the DHT is pasting an **invitation**, and nothing else.

---

## Get started

### Run from source
```sh
go build -ldflags "-X main.version=$(cat VERSION)" -o nib ./cmd/nib
./nib [file.pdf]
```
The whole UI is embedded in the binary — nothing to fetch at runtime.

The `-ldflags` is what stamps the build with its version. Without it `main.version`
keeps its compile-time default of `dev`, and the version pill then reports `dev`
rather than the release it was built from — which this README used to promise it
"always shows". `build.sh` and `install.sh` pass the same flag; this line is for
building by hand. Nib opens
its window in your installed Chrome / Edge / Brave / Chromium (app mode), or
falls back to a normal browser tab.

The first run opens a short intro explaining what the SSH key protects, then a
one-time setup where you either **use an SSH key you already have** or have Nib
**create one for you** (at a path you can change — works the same on Linux,
macOS, and Windows, no key needed up front). That key is what unlocks your
vault, so keep it safe and back it up. You can authorize or create more keys
later from **Settings → Identity & Keys → Manage authorized keys…**.

### Install on Debian / Ubuntu
```sh
./install.sh
```
Builds Nib for your machine, packages a `.deb`, installs it, and adds **Nib** to
your applications menu (under Office). Run it again any time to upgrade in place.

The package **recommends** a Chromium-family browser (or `xdg-utils` and any
browser registered as the handler), because that is how Nib shows its UI — it has
no window of its own. `apt` installs recommends by default, so this only matters
if you install with `--no-install-recommends` on a machine with no browser at all:
Nib will start, bind its loopback port, and have nothing to display itself in.

### Other platforms
```sh
./build.sh
```
Cross-compiles a static, cgo-free binary for **Linux, macOS, and Windows**
(amd64 + arm64) into `dist/`, plus Linux `.deb` packages. On macOS and Windows
you run the binary directly — it's fully self-contained.

On Windows, run `nib register` once to have Nib offered for PDFs in Explorer's
**Open with** menu (`nib unregister` undoes it). Everything it writes lives under
`HKEY_CURRENT_USER`, so it needs no administrator rights and touches no other
account. It stops short of making Nib the *default* PDF handler, and so does
every other program: since Windows 8 the default for a file type is sealed
behind a per-user key that only Windows itself may write. Right-click a PDF →
**Open with** → **Choose another app** → **Nib**, and tick *Always use this app*
if you want it to stick.

Opening several PDFs *through Nib* — from Explorer, from a Linux file manager's
**Open with**, or from the command line — gives you **one Nib holding them all**,
a tab each. (On Linux a plain double-click reaches whatever your desktop has set
as the PDF handler, which is usually not Nib; `xdg-mime default nib.desktop
application/pdf` changes that, and it is deliberately your call rather than
something the package does behind your back.) The second
launch finds the running one, hands it the path, and exits; if Nib is locked at
the time, the document opens as soon as you unlock. The document arrives as a new
tab in the window you already have open — no second window — and opening several
at once still gives one Nib. Nib cannot bring a window you have covered or
minimised to the front on every desktop: on Linux under X11 it does when `wmctrl`
or `xdotool` is installed, and elsewhere the window marks its title (●) until you
return to it. Launching Nib again with *no* document always opens a window. The
hand-off itself works the same on every platform. It used to be Linux-only and it worked by killing the running process
and taking its place, which meant a second double-click could take an unsaved
document down with it.

Windows behaviour can be checked without a Windows machine: `./build/winrepro.sh`
builds `nib.exe`, runs it headless under [wine](https://www.winehq.org/) in a
throwaway prefix, and drives the same HTTP calls the UI makes — drive
enumeration, `~\` expansion, unreadable-folder reporting, save containment, and
a genuine second launch handing its document to the first. The
places `path/filepath` answers differently on Windows are exactly the ones a
Linux test suite can't reach. It skips cleanly when wine isn't installed.

A `Makefile` wraps these: `make dist` regenerates the third-party notices and
runs the cross-compile/package; `make install` does the same for a local
install; `make notices` regenerates [THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md)
on its own.

### Staying up to date
A version pill at the top always shows the installed version, colored by update
status: **yellow** — status unknown (no check has run yet, or the startup check
is turned off); **green** — you're on the latest release; **red** — a newer
release exists. At startup Nib asks GitHub for its latest release version and
colors the pill accordingly. Clicking the pill checks right now — even with the
startup check off — and, when a newer release exists, downloads the
build matching your OS and architecture (a `.deb` for a package install,
otherwise the raw binary).

This is the only call Nib makes **on its own** — the only one you never asked
for — and it's a **version query — no document data, no telemetry**: your
documents never leave your computer. It is not the only call Nib can make —
timestamping, opening by URL and co-signing all use the network when you ask
them to — and every one of them is listed under
[What leaves your computer](#what-leaves-your-computer). Turn the
startup check off from **Settings → Toggle Features → Check for updates on startup** (saved in
your vault), or set `NIB_NO_UPDATE_CHECK=1` to force it off regardless (clicking
the version pill still checks either way). Nib only notifies and downloads — it never installs or replaces
itself; you apply the update the way you installed (`apt` / `install.sh`, or by
swapping the binary).

When a newer release exists, clicking the pill downloads it. Nib does not ask where: a window says
**Downloading to** and the folder, reports progress, and when it finishes gives you **Show in
folder**. The folder is **your browser's own download folder** — the one set in the browser Nib's
window is running in (Chrome, Chromium, Edge, Brave or Firefox), or your system's Downloads folder
when that browser has none set or Nib does not know the browser. To use a different folder, type it
in **Settings → Toggle Features → Download folder**; the line under the box always shows the folder that
will be used and where it came from. If a file of that name is already in the folder, Nib says so
and downloads nothing.

To learn the browser's folder, Nib reads **only these settings and nothing else** from the browser's
own files on your computer: which profile was last used and that profile's download folder (Chrome
and its relatives: `profile.last_used`, `download.default_directory`); the default profile and its
download-folder choice (Firefox: `browser.download.folderList`, `browser.download.dir`). It does not
read history, bookmarks, passwords or anything else, keeps nothing it read except the folder, and
sends it nowhere. Nib still fetches the build itself rather than handing the job to your browser,
which is what lets it show progress and tell you where the file went.

**It stops at the file, deliberately.** Nib does not run or install what it downloaded, and writes it
without the execute bit. Nib's releases are published without checksums or signatures, so there is
nothing for Nib to check the bytes against — and running an unverified program it fetched over the
network is not something it will do on your behalf. Cancelling, or a failure part-way, leaves no
half-finished file behind.

### Options

| Variable | Effect |
| --- | --- |
| `NIB_ADDR` | Pin a fixed loopback address (e.g. `127.0.0.1:8791`) instead of a random port. Must be loopback (`127.0.0.1`, `localhost`, or `::1`) — a non-loopback address is refused at startup. |
| `NIB_NO_BROWSER` | Don't open a window — just serve and log the URL to open (headless / remote). The logged URL carries a single-use key after `#k=` that expires after ten minutes; open it once, as printed. A window without it cannot reach Nib — run `nib` again to get a fresh one. |
| `NIB_NO_UPDATE_CHECK` | Disable the automatic startup update check (clicking the version pill still checks). |

---

## Command line

Beyond the desktop app, `nib` runs a handful of operations headlessly — no
window, no server — so you can script them or batch a folder. Anything that
isn't a known command (a PDF path, or nothing) still opens the app as usual.

| Command | What it does |
| --- | --- |
| `nib timestamp FILE…` | Write an OpenTimestamps proof (`FILE.ots`) for each file, skipping any file that already has one. |
| `nib timestamp --force FILE…` | Re-stamp even where a proof exists, discarding it. |
| `nib timestamp --verify FILE…` | Check each file against its `FILE.ots` proof. |
| `nib verify [--json] FILE…` | Report each file's signature integrity, and the **ceremony** it belongs to if it has one — the roster, who was obliged to sign, who has, and whether every signature commits to the same proceeding. Exit `2` if any file is unsigned, modified, has content added after its last signature, carries a signature Nib refused, **or belongs to a ceremony an obliged party has not signed, whose signatures do not all commit to it, or that carries a signature from someone off its roster** — a signature written by a newer Nib excuses only its own unread attestation, never another signature and never the roster check. |
| `nib optimize IN -o OUT` | Losslessly shrink a PDF (or `-w FILE…` to rewrite in place). |
| `nib merge IN… -o OUT` | Concatenate PDFs, in order, into one. |
| `nib sanitize IN -o OUT` | Strip identifying metadata and active content — JavaScript, auto-actions, embedded files (or `-w FILE…`). |
| `nib sign IN -o OUT --cert ID.p12` | Certify a PDF with an imported `.p12` identity. |
| `nib rotate IN -o OUT --deg N` | Rotate pages by 90/180/270° (`--pages 1-3,5` to limit; or `-w FILE…`). |
| `nib pages IN -o OUT --keep SEL` | Keep/reorder pages (`--keep 1-3,5`) or delete them (`--remove 2,4`). |
| `nib split IN --out-dir DIR …` | Burst into one file per chunk (`--every N`), range (`--ranges 1-3,4-8`), or `--bookmarks`. |
| `nib encrypt IN -o OUT` | Add AES-256 password protection (`--password-file FILE` or `$NIB_PDF_PASSWORD`, required; an already-encrypted PDF is reported, not re-encrypted). |
| `nib decrypt IN -o OUT` | Remove password protection / owner restrictions (`--password-file FILE` or `$NIB_PDF_PASSWORD`; already-plain PDFs pass through). |
| `nib booklet IN -o OUT` | Impose for **saddle-stitch** printing: pad to a whole sheet of four, reorder into sheet order, two pages a side. Print double-sided **flipping on the short edge**, then fold and staple through the fold (`--border` for outlines). |
| `nib nup IN -o OUT --n N` | Place N pages per sheet — 2/4/6/9/16… (`--border` for outlines). |
| `nib normalize IN -o OUT` | Resize every page to the document's most common page size — make a mixed-size PDF uniform (content scaled to fit, centred; orientation kept). |
| `nib office IN -o OUT` | Convert a document (`.md`, `.docx`, `.xlsx`, `.odt`, …) to PDF. Markdown is converted by Nib itself; office formats need LibreOffice. `--lang en` declares the document's language. |
| `nib pdfa IN -o OUT` | Convert to a **PDF/A-2b** archival candidate (add `--gs` to convert through Ghostscript instead) (embed sRGB OutputIntent + PDF/A XMP, strip active content). Refuses documents with non-embedded fonts or encryption. Verify the result with [veraPDF](https://verapdf.org/) — Nib can't certify conformance itself. |
| `nib ua IN` | Check a document against the **PDF/UA-1** accessibility rules Nib can verify itself — **105 of the 106** veraPDF evaluates — each marked passes / fails / does not apply / **Nib could not check** (never shown as a pass). Exits 1 with every reason when any checked clause fails or could not be checked. **Exit 0 is not a PDF/UA certificate**: a document can pass every clause Nib checks and still fail one it does not. |
| `nib tag tree IN [--json]` | Print the document's existing structure tree in reading order: each element's id, type, page, missing alt text or header scope, a table cell's spans and header cells, and text; then the role map — each of the document's own type names, what it is mapped to, and how many tags carry it. `--json` is the shape the app reads. |
| `nib tag propose IN [--json]` | Print the headings, paragraphs, list items, ruled tables and figures Nib would propose, a table's rows and cells indented under it and a figure with the box its picture fills. Writes nothing. |
| `nib tag commit IN -o OUT --review REVIEW.json` | Write a reviewed proposal (`nib tag propose --json` is a review that keeps every role; give each proposed `Figure` an `"alt"` saying what its picture shows, or `"ignore": true`, or the commit is refused). A signed document is refused. |
| `nib tag untagged IN [--page N] [--json]` | Print what the pages draw that no tag owns — text in paragraphs, pictures, drawings, then rules and boxes — each with its page, whether the page marks it as decoration, and where it is (left, top, right, bottom as fractions of the page as shown). Writes nothing. `--json` is the shape the app reads, and each piece's `"rect"` is what a `region` edit takes. |
| `nib tag edit IN -o OUT --edits EDITS.json` | Correct the existing tree as one batch — retype, move, alt, scope, colspan, rowspan, headers, artifact, create, delete, region, promote, rolemap — by the ids `nib tag tree` prints. `create` adds an empty tag (`"value"` its type, `"parent"` the id it goes under or `-1` for the top, `"index"` its place); `delete` takes a tag away and keeps what it held; `move` takes `"parent"` and `"index"` the same way. `region` tags untagged content of a `"page"` as one new tag, placed as `create` places one: what is centred in `"rect"` (`[left, top, right, bottom]`, fractions of the page as shown) or lies inside any of `"pieces"` (the rects `nib tag untagged --json` lists), with an `"alt"` that a `Figure` must have. `promote` takes nothing else and gives every tag written inline (id 0) a number, so it can be named. `rolemap` sets what one of the document's own type names means — `"role"` the name, `"value"` the standard type, or `""` to remove a mapping no tag uses. A signed document is refused. |
| `nib tag remove IN -o OUT` | Take the structure tree and every marked-content id away, leaving the document untagged so it can be tagged again. What is marked as an artifact stays marked. A signed document is refused. |
| `nib pagenum IN -o OUT` | Stamp running page numbers or Bates numbering (`--prefix ABC --pad 6 --position br --total`). `--continuous (-w \| --out-dir DIR) FILE…` threads one counter across a whole file set (multi-file Bates production). |
| `nib pagelabels IN -o OUT` | Set logical page labels — one `--range PAGE:STYLE[:START[:PREFIX]]` per section (STYLE = `decimal`/`roman-lower`/`roman-upper`/`alpha-lower`/`alpha-upper`/`none`), e.g. `--range 1:roman-lower --range 5:decimal`. |
| `nib fill IN --data D` | Fill a form: a JSON or **XFDF** record (`--data x.json\|.xfdf -o OUT`, the inverse of *Export form data*) or a **CSV mail-merge** (`--data rows.csv --out-dir DIR` — header row = field names, one filled PDF per row; `--name-col COL` names each output). Filling removes any existing signature. |
| `nib export-xfdf IN -o OUT` | Export a form's field data as **XFDF** — the XML interchange format Acrobat and Foxit read and write (the inverse of `nib fill --data x.xfdf`). |
| `nib attachments IN [--json]` | List embedded files; `--extract ID -o OUT` pulls one out (the id `--json` lists, or a name only one file carries), `--add FILE -o OUT` embeds one. |
| `nib outline IN [--json]` | List the document's bookmark outline (indented by level, or JSON). |
| `nib register` / `nib unregister` | **Windows only.** Add or remove Nib from Explorer's "Open with" menu for PDFs (per-user, no admin). Windows reserves the *default* handler for the user to pick. |
| `nib watch DIR --do OP` | Run `timestamp`/`optimize`/`sanitize`/`ua` on each PDF added to `DIR`, until interrupted. |
| `nib discover` | Report what link-local peer discovery can see from this machine: which interfaces were joined and why, whether announcements left, and what came back. Local network only. |
| `nib rendezvous` | Report whether the BitTorrent DHT that remote co-signing uses is reachable, and what this machine's public address looks like from outside. **Contacts the public internet** — it prints a notice before it opens a socket. Publishes nothing unless you add `--self-test`, which also publishes one throwaway record and fetches it back. |
| `nib version` | Print the version. |

Commands that produce a PDF write it to `-o`/`--out`; `timestamp` writes a
sidecar `.ots` beside each input and `verify` prints a report. Flags may go
before or after the file arguments.

**`timestamp` is the one command here that is not about PDFs.** It hashes the file you give
it — any file: a spreadsheet, a photograph, a zip, a source tarball — and anchors that hash, so
`nib timestamp notes.txt` is as valid as `nib timestamp contract.pdf`. Nothing is read as a
document and nothing is modified: the proof is a separate `.ots` beside the input, and
`--verify` reports on the bytes rather than on the format. (`nib watch DIR --do timestamp` is
the exception and is deliberately `.pdf`-only — it is a watch folder for documents.)

The exit status is non-zero on failure —
`verify` returns `2` when a signature is invalid, absent, **valid over only
part of the document**, or **valid over only part of the roster** — the third being a
counterparty who returned your signed contract with pages appended, and the fourth a
multi-party document some obliged party never signed. Both are cases where every
signature present is genuine and the document is still not what it looks like, so a
check drops straight into a script:

On a document that carries a ceremony, `verify` also prints the roster — so a stranger
handed a nine-party deed can see at a glance who was supposed to sign and who has:

```
lease.pdf: valid (5 signer(s))
  ceremony 19db9b44b8d3…: 5 of 9 obliged signer(s) have signed
  recital: "We agree to be bound by the lease of 14 Elm Row"
  INCOMPLETE — 4 obliged party(ies) have not signed
  every signature commits to this document's ceremony
  ✓ Alice Tenant                 signed
  ✗ Frank Director               HAS NOT SIGNED
```

`--json` carries the same facts as fields (`obliged`, `signed`, `complete`,
`oneProceeding`, `missing`), so a script never has to parse the sentence. A document
with no ceremony gets no ceremony section and no `ceremony` key — most signed PDFs
belong to no proceeding, and describing them as a ceremony of nobody would be a
verdict on something that does not exist.

```sh
nib verify contract.pdf && echo "signature intact"
for f in *.pdf; do nib timestamp "$f"; done   # re-runnable: already-stamped files are skipped
nib sanitize -w *.pdf          # scrub a whole folder in place
nib optimize in.pdf -o - | nib sanitize - -o out.pdf   # compose in a pipeline
```

For `optimize`, `merge`, `sanitize`, `sign`, `rotate`, `pages`, `encrypt`,
`decrypt`, `booklet`, `nup`, `normalize`, `pagenum`, `pagelabels`, `fill` (JSON
or XFDF), and `export-xfdf`, a filename of `-` reads a PDF from stdin, and `-o -`
writes the result to stdout (refused when stdout is a terminal), so the commands
chain together. Anything a command has to say to a person — `booklet`'s folding
instruction, `ua`'s verdict — goes to stderr, so stdout carries only the
document.

`booklet`, `decrypt`, `encrypt`, `normalize`, `nup`, `optimize`, `pagelabels`,
`pagenum`, `pages`, `rotate`, and `sanitize` take `-w`/`--in-place` to rewrite each file
given instead of writing a single `-o` output — the batch form for a folder.
`tag commit` and `tag edit` take `-w` too, over exactly one file, because a review or a batch of
edits describes one document.
Each rewrite is atomic (written through a temp file and renamed over the
original, so a failure never corrupts it) and preserves the file's permissions.
**A signed PDF is refused in place**, by every one of those commands and by
`nib watch`: a structural rewrite invalidates every signature on a document, and
in place there is no undo and no second copy — the result would be a valid PDF
that no longer proves anything. Write to a new file with `-o` if that is what you
want; the original then survives either way.

`nib sign` reads the certificate passphrase from `--password-file FILE`, the
`NIB_P12_PASSWORD` environment variable, or — when run in a terminal with neither
set — a no-echo prompt, so it's never on the command line where other processes
could see it. Add `--tsa URL` to fix the signing time with an RFC3161 timestamp
authority.

`nib watch DIR --do timestamp|optimize|sanitize|ua` runs that operation on each PDF
dropped into `DIR` and keeps running until you stop it (Ctrl-C) — the "process
my inbox" / scheduled-job workflow. It polls (no background file-watching
dependency), waits for each file to finish copying before acting, and handles
each file once. `timestamp` writes a `.ots` sidecar; `optimize`/`sanitize`
rewrite in place — and so skip any signed PDF that lands in the directory,
reporting it, rather than silently invalidating its signatures. `ua` writes the report `nib ua`
prints — the table and its verdict — to `FILE.ua.txt` beside each PDF and touches nothing else.
`--do tag` is refused: a proposed structure is reviewed before it is written, and a watch has no
one to review it; use `nib tag propose` and `nib tag commit`. Run `nib <command> -h` for a command's own flags.

---

## How it works

Nib is a small Go server that embeds the entire single-page UI and the
[pdf.js](https://mozilla.github.io/pdf.js/) engine. Your browser renders and
fills the PDF; [pdfcpu](https://pdfcpu.io) stamps, flattens, and redacts;
[pdfsign](https://github.com/digitorus/pdfsign) signs and verifies. Pure Go, no
cgo, permissive dependencies only — so it builds to one portable binary per
platform.

```
cmd/nib          entry point — run a headless command, or bind loopback and open the window
internal/cli     headless subcommands (everything under Command line)
internal/server  HTTP API + embedded UI, loopback-only guard
internal/vault   encrypted store (AES-256-GCM, sealed to your SSH key)
internal/pdfops  pdfcpu stamping / flattening / redaction
internal/sign    signing + signature verification
web/             the single-page UI and the vendored pdf.js engine (embedded)
```

---

## License

Nib is free software under the **GNU Affero General Public License v3.0** — see
[LICENSE](LICENSE). Copyright © 2026 Daniel Alexander.

Distributed **as-is, with no warranty** of any kind, to the extent permitted by
law (AGPLv3 §§15–16). You may use, study, share, and modify it under the AGPL;
derivative works must also be released under the AGPL. Because Nib is AGPL,
if you run a modified version to provide a network service, you must offer that
version's complete source to its users (AGPLv3 §13).

Nib also incorporates third-party software (Go modules and the vendored pdf.js
engine), all under AGPLv3-compatible permissive licenses (BSD, MIT, Apache-2.0).
Their required copyright and license notices are collected in
[THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md), regenerated with
`build/gen-notices.sh`.

**Settings → About** shows these in-app — a plain-English account of what a Nib
signature does and doesn't prove, plus the licence and third-party notices read
straight from the shipped files.
