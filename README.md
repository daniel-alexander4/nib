# Nib

[![License: AGPLv3](https://img.shields.io/badge/license-AGPLv3-blue.svg)](LICENSE)
![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)
![Platforms](https://img.shields.io/badge/platforms-Linux%20%7C%20macOS%20%7C%20Windows-555)
![100% local](https://img.shields.io/badge/data-100%25%20local-success)
![cgo-free](https://img.shields.io/badge/build-pure%20Go%2C%20single%20binary-00ADD8)

**A complete PDF toolkit that runs entirely on your own machine.** Fill any form,
make scans searchable with on-device OCR in 41 languages, truly redact, compare
revisions visually, sign and timestamp, convert to and from Office and PDF/A,
pull tables out to spreadsheets, and reshape pages — all from a desktop-style app
*or* a scriptable command line.

And it's **yours**: free and open source under the AGPL, a single self-contained
binary with no installer, no account, no subscription, and no cloud. Nothing you
open, type, or sign ever leaves your computer, and your documents and signing
identity live in an encrypted vault only your SSH key can open.

---

## How Nib compares

How Nib's feature set lines up against the three best-known PDF editors.
**Legend:** ✅ built in · 🟡 partial or limited · ❌ not available.

| Capability | **Nib** | Adobe Acrobat Pro | Foxit PDF Editor | PDF-XChange Editor |
| --- | :--: | :--: | :--: | :--: |
| Fill forms — including flat & scanned | ✅ | ✅ | ✅ | ✅ |
| Detect fields, turn a scan into a fillable form | ✅ | ✅ | ✅ | ✅ |
| On-device OCR *(41 languages)* | ✅ | ✅ | ✅ | ✅ |
| True redaction + pattern / PII search-and-redact | ✅ | ✅ | ✅ | ✅ |
| Visual **and** text document compare | ✅ | ✅ | ✅ | 🟡 |
| Edit existing text with paragraph reflow | 🟡 *(cover & replace)* | ✅ | ✅ | ✅ |
| Digital signature with your own certificate | ✅ | ✅ | ✅ | ✅ |
| RFC-3161 trusted timestamp | ✅ | ✅ | ✅ | ✅ |
| **OpenTimestamps** (Bitcoin) proof of *when* | ✅ | ❌ | ❌ | ❌ |
| **Peer-to-peer co-signing**, no server | ✅ | ❌ | ❌ | ❌ |
| PDF/A archival export | ✅ | ✅ | ✅ | ✅ |
| Office ↔ PDF conversion † | ✅ | ✅ | ✅ | 🟡 |
| Table → spreadsheet (XLSX / ODS / CSV) | ✅ | ✅ | ✅ | 🟡 |
| Merge, split, rotate, crop, N-up, Bates, page labels | ✅ | ✅ | ✅ | ✅ |
| AES-256 encryption | ✅ | ✅ | ✅ | ✅ |
| **Scriptable command line** / batch a folder | ✅ | ❌ | ❌ | 🟡 |
| Runs offline §, no account, no telemetry | ✅ | ❌ | 🟡 | ✅ |
| **Free & open source** (AGPLv3) | ✅ | ❌ | ❌ | ❌ |
| Single self-contained binary, cross-platform ‡ | ✅ | ❌ | ❌ | ❌ |
| **Price** | 🟢 **Free** | 🔴 Subscription | 🔴 Paid | 🟡 Free tier |

<sub>† Office conversion uses LibreOffice if it's installed — optional, detected at runtime, never bundled. ‡ One portable binary for Linux / macOS / Windows (PDF-XChange Editor is Windows-only). § No account, no telemetry, no analytics, and every editing feature works with no network at all. A few features do reach the network — timestamping, timestamp verification, opening a document by URL, remote co-signing, and the update check — each one started by you and never in the background. All of them are listed in [What leaves your computer](#what-leaves-your-computer).</sub>

Acrobat, Foxit and PDF-XChange are mature commercial editors that do plenty Nib
doesn't aim to — full WYSIWYG content editing, prepress, cloud collaboration. The
table is about the jobs Nib *does* cover, and where it works differently.

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
Press **Detect** and Nib scans the page, then drops fillable widgets where they
belong:

- **Blank lines** → a text box above every fill-in rule, including the faint
  light-gray lines on modern forms.
- **Boxes** → a field inside each empty box (boxes that already contain text are
  skipped).
- **Tables** → one input per blank cell.
- **Checkboxes** → click to check.
- **Circle-the-answer choices** → `Y / N`, option sets near a "(circle one)"
  note, pipe-separated lists on their own (`$5 | $10 | $25`), a labelled run of
  options (`Type of Membership: Youth Teen Adult …`), and repeats of a choice the
  "(circle one)" governs (every `Male / Female` on the page, not just the first).

It's a smart proposal, not magic — move, resize, retype, or ignore anything it
suggests.

### Turn a flat scan into a fillable form
Run **Detect** on a flat or scanned form, then **File → Save as → Save as fillable
form…** to emit a real interactive **AcroForm PDF**: every detected text box and
checkbox becomes a live, fillable field (with proper appearance streams, so it
works in Adobe and any browser), dropped right onto the original page — the scan
itself is untouched. Need a **dropdown** or a **radio-button group**? The Edit-tab
**Dropdown** and **Radio** tools let you draw one and type its choices; they're
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
Got a fillable form and a spreadsheet of records? **File → Fill from spreadsheet…**
picks a CSV and fills the form once per row, handing you a **ZIP with one PDF per
row**. The CSV's first row is the form's field names (export them with *File →
Export → Form data (CSV)* to see the exact names); checkbox fields take
true/false/yes/1. It runs **entirely on your machine** — the same engine as the
`nib fill` command line, just point-and-click. (The form needs real fillable
fields; on a flat scan, run *Save as fillable form…* first.)

### Exchange form data as XFDF
Trading form data with Acrobat or Foxit? Nib reads and writes **XFDF**, the XML
form-data interchange format both speak. **File → Export → Form data (XFDF)**
saves the open form's values; **File → Import form data (XFDF)…** fills the form
from an `.xfdf` file and saves the result. On the command line it's
`nib export-xfdf IN -o OUT.xfdf` and `nib fill IN --data DATA.xfdf -o OUT`.
Hierarchical (dotted) field names are preserved as nested fields; like every Nib
operation it runs entirely on your machine.

### Convert to PDF/A for archiving
Need a document that archives will still open decades from now? **File → Export →
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
general conversion pure Go can't do. It's strictly optional: when Ghostscript is
present the dialog offers a *"Convert with Ghostscript"* button (and the CLI a
`--gs` flag); when it isn't, the built-in converter is used and the feature simply
isn't offered. Ghostscript is detected at runtime and never bundled, so Nib stays a
single pure-Go binary. (Note: that path runs Ghostscript over your PDF; it executes
under Ghostscript's sandbox, but it is processing the document, so only enable it
for files you'd open anyway.)

Either way, because no pure-Go PDF/A validator exists, Nib can't certify the result
itself — it produces a *candidate* you should **verify with
[veraPDF](https://verapdf.org/)** before relying on it for archival.

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
Nib stays a single cgo-free binary). When LibreOffice isn't installed the file
picker narrows to Markdown, and the CLI verb reports it's missing:

```
nib office report.docx -o report.pdf
nib office notes.md -o notes.pdf
```

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
Filling a form with the same fields on every page? Run **Detect** to find the
blanks, then in the **Flags** tab pick **Sign**, **Date**, **Initial**, **Name**,
**Title**, or **Company** and click a blank to flag it — the flag snaps to that
line (or click anywhere to place one freehand).
Then click each flag to fill it and Nib **jumps to the next** one: a date flag
stamps today's date; a sign/initial flag drops your signature or initials, picked
from the Library once and reused; and a name/title/company flag fills from your
**autofill profile** (add the value once under *Edit → Edit autofill profile* and
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
**Collaborate → Originate → Send a document to a peer…** hands it straight to a pinned peer over
the same encrypted, no-cloud channel co-signing uses (both of you online; they
pick **Receive a document…** first). Received files save into `~/nib` — a flagged
document waiting for you under `to-sign/`, a finished signature under `signed/` —
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
**Edit → Edit text**, then drag a box over baked-in text. Nib covers it with a
fill sampled from the background and drops an editable box prefilled in the
original's size, colour, and closest font (serif / sans / mono, bold, italic) —
so a fix reads like an edit, not a patch. The page stays sharp and vector. The
original text stays underneath (it's a visual edit) until you press **Remove
originals**, which flattens just the edited pages so the old text is gone for
good — or until you flatten / finalize the whole document.

### OCR — make a scan searchable
Got a scanned PDF that's just images? **Edit → OCR** reads the text on every page
and adds an **invisible text layer** underneath the scan, so the page still looks
exactly the same but the text is now **selectable, copyable, and findable** (and
shows up in *Find*). The OCR runs **entirely on your machine** — the recognition
engine is built into Nib (no install, no cloud, nothing leaves your computer) —
so it works offline like everything else. Pick the scan's language from the
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
Draw redaction boxes and press **Apply**. Nib re-renders those pages flat so the
content underneath is **actually gone** — not just hidden behind a black
rectangle. (Verified: a redacted page exposes no hidden text or form field.)

Don't want to hunt for every occurrence by hand? **Redact text…** finds them for
you: type a word or phrase, and/or tick a built-in pattern — **SSN, email, phone,
card number** — and Nib marks every match in the document as a redaction box.
Review the boxes (remove any you don't want), then press **Apply** to flatten
them for real. It reads the text layer, so it works on any text-based PDF — and
on a scan once you've run **OCR**. Matches split across the page's text runs are
still caught; matches that wrap across a line break are not (rare for the
patterns).

### Compare two versions
Wondering what changed between draft v3 and v4 of a contract? **File → Compare…**
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
embedded files, optional-content layers, XMP metadata, and the document's
identifying properties (author, title, creator…). Then remove it four
ways, strongest fidelity-preserving first:
- **Strip active content** — neutralises every auto-run hook, script and risky
  action while keeping the page text and layout intact.
- **Strip identifying metadata** — clears the document properties (author, title,
  creator, subject, keywords), deletes the XMP metadata, and regenerates the
  document's tracking identifier, leaving the visible content untouched. (pdfcpu
  re-stamps a generic producer and the current date on write, so the file names
  Nib, not you.)
- **Remove files & media** — deletes only embedded files and media annotations,
  leaving all other interactivity untouched.
- **Flatten to images** — the guaranteed-inert floor: turns every page into an
  image so nothing active can remain (selectable text is lost).

If a strip can't produce a sound document it's reported and your open document
is left untouched, so you can step down to the next method safely. Any removal
produces a new, **unsigned** copy — save it to keep the cleaned version.

### Add password protection
**Secure → Add password protection…** saves a separate **AES-256 encrypted copy**
that needs the password to open (the same password opens and owns the file). You
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
— that's the point. Export your public certificate so others can verify it's you.

**Sign with your own certificate.** By default Finalize uses Nib's self-signed
identity (integrity, not third-party trust). If you have a CA-issued credential,
import it under **⚙ → Identity & peers → Signing certificate** (a PKCS#12
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
authority or merely stated by the signer.

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

### Co-sign with a peer
Two people can sign the *same* document, each attesting — in a visible block and a
cryptographically-signed reason — that they accept the other's identity. Nib pins
that identity by its **key fingerprint**, which you compare once over a channel you
both trust (read it aloud on a call, or paste it across a secure chat) under
**Identity & peers**; every fingerprint has a **Copy** button for that comparison.

There are two ways to exchange the document:

- **Pass the file** — you co-sign, then send the PDF to the other person (email, USB,
  Signal); they co-sign and send it back. Nothing but the file moves, and the result
  verifies on its own, with no server in between.
- **Live, over an encrypted channel** — co-sign in real time without passing a file.
  One person **arms to receive** (Collaborate → Receive → *Receive a live co-signature…*), the other
  **dials in** (Collaborate → Originate → *Co-sign live with a peer…*). The connection is mutually
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
in it is run by us.) Today the dialing peer reaches the receiver's armed listener directly,
so the receiver makes their chosen `host:port` reachable one of two ways:

- **Port-forward** the chosen port on their router to their machine, or
- **Share a private network** you both already trust — a VPN such as **Tailscale**
  or **WireGuard** — and bind / dial the address it hands you.

If the receiver is behind **CGNAT** (common on mobile and some ISPs), port-forwarding
won't work — use the VPN path. The security model doesn't depend on *how* you reach
each other: the pinned-key handshake holds over any transport.

### Open a document
**Open…** (File tab, or Ctrl+O) takes a typed path or URL, or browses your
filesystem. Browsing opens the file *by path*, so it can be saved back in place
and shows up under **Recent** — unlike dragging a file onto the window, which
uploads a copy with nowhere to save to.

A file that isn't a PDF is refused with a message rather than opening an empty
viewer — the header is looked for in the first 1024 bytes, the same window
pdf.js allows, so a document with a little junk before its header still opens.

### Several documents at once
Opening a document **adds** it — the one you had stays open. A strip of tabs
appears above the page as soon as there are two, and clicking one switches to it.
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

**Close view** puts down the document you are looking at and moves to the next
tab; **Close all** puts down every one. With a single document open there is just
**Close**, and the app looks exactly as it did before tabs existed. Either way, if
anything has been edited since a document was opened, Nib asks first — and it asks
about edits *since the last save*, because that is what it can actually tell:
saving deliberately leaves the undo history intact, so a save does not silence the
question. Closing the last document returns the viewer to "Open a PDF to begin."
with Nib still running, ready for the next file.

One caveat, until single-instance handling lands: opening a PDF from your file
manager doesn't reach the Nib you already have running — it starts a *second* Nib,
with its own set of documents. Use **Open…** from inside the app to add a document
to the session you are already in.

The same folder browser backs every destination picker (Save As, and both
splits). On Windows it lists your **drives** once you reach the top of one:
there's no single root there the way `/` is on Linux and macOS, so without that
a document on `D:`, a USB stick or a mapped share couldn't be reached by
clicking. And a folder Nib can't read says so — "you don't have permission to
read that folder", "that's a file, not a folder" — instead of looking empty.

### Pages & export
**Combine PDFs…** (File tab) assembles several documents into one: add the files,
arrange them with ↑ ↓, and they merge top-to-bottom into a new document — then
reorder individual pages across them by dragging thumbnails. It works even with
nothing open, and the result is a new, unsigned document (Save As to keep it).

Changed your mind? **↶ Undo** / **↷ Redo** (Ctrl+Z / Ctrl+Shift+Z, on the **Edit**
tab) step back and forth through document operations — rotate, delete, reorder,
crop, split, page numbers, outline and metadata edits — for the open document.
The same Ctrl+Z also undoes overlays you place — a stamp, border, shape, note,
cover-edit, or sign/date/initial flag — and dragging or resizing one, so one
keystroke walks back your most recent change whichever kind it was. History
clears when you open another file or run a content-destroying step (redaction or
flatten can't be undone). While you're drawing pdf.js annotations (text boxes,
highlights, ink) those keep pdf.js's own Ctrl+Z; typing in a field uses your
browser's normal undo.

Rotate, delete, **append**, and reorder pages — **drag a page's thumbnail** in the
sidebar to move it where you want. Rotate every page at once with **Rotate all ↺ / ↻**
on the **Edit** tab, or hover a thumbnail to rotate (either direction) or delete a
single page. **Shift- or Ctrl/Cmd-click thumbnails** to select several at once, then
rotate, delete, or **move the whole selection to the front or back** (⤒ / ⤓) from the
bar above the thumbnails — or **drag a selected thumbnail** to slide the whole group to
any spot, keeping its order. **Extract pages…** saves a range (type `1-3, 5`) as a new PDF without
touching the open document, and **Insert blank page** drops a fresh page — matching
its neighbour's size — after the current one. **Duplicate page** drops a copy of the
current page right after it, and **Insert PDF…** splices another PDF in before the
current page (before page 1 to prepend a cover; use **+ Append PDF** for the end).
**Page numbers…** stamps a running
number onto every page at the corner you choose — add a prefix and a zero-pad
width for Bates numbering (e.g. `ABC` + width 6 → `ABC000001`), or tick "of N" for
classic "Page 3 of 10". **Page labels…** sets the document's *logical* page
numbers — the ones a viewer shows in its page box and thumbnails, as distinct from
ink stamped on the page — so front matter can read i, ii, iii while the body reads
1, 2, 3. Add a range per section (from which page, what style — decimal, upper/lower
Roman, upper/lower letters, or a prefix-only label — and where its count starts);
pages before the first range carry no label. Going the other way, **N-up…** combines several pages onto
each sheet (2-up, 4-up, up to 16) for printing or handouts, in reading order, with
an optional border — each sheet keeps the document's page size. Got a document
whose pages are all different sizes? **Normalize sizes** (Edit tab) resizes every
page to the document's most common size, scaling each page's content to fit and
centring it — a one-click way to make a mixed-size scan or merge uniform. It keeps
each page's orientation (a landscape page stays landscape) and the text stays live
and selectable. **Crop…** trims
the margins away — draw a box around the part to keep and every page (or just the
current one) is cut down to it. The box is taken as a proportion of the page, so a
document with mixed page sizes keeps the same relative region on each. It's a
re-crop, not a re-render, so quality is
untouched; the trimmed-off content is hidden behind the smaller page, not
destroyed — use **Flatten** or **Redact** to remove it for good. Got a scanned 2-up
or 4-up sheet? **Split page…**
(on the **Edit** tab) cuts the current page into a grid of separate pages — pick the
columns and rows, preview where the cuts land, and
optionally resize each piece to a full page. Not a clean grid? **Split by box**
lets you split a page by hand — drag a rectangle around each region you want, then
**Apply box split** and the page is replaced by those regions, each as its own
page. The new pages all come out the **same size** (the largest region's, smaller
ones centred and padded), so the output is uniform. It's a re-crop, not a
re-render, so every piece keeps its original quality.

**Edit the outline** — the **Outline** panel (sidebar, File tab) lists a PDF's
bookmarks; **Edit outline…** opens an editor to author them: add a bookmark for any
page, rename, delete, and indent to nest (chapters → sections). Bookmarks stay in
page order and jump to the top of their page; saving replaces the document's outline.

**Split by bookmarks** (File → Export) turns one bookmarked PDF into a folder of
separate files — one per top-level bookmark, named from the bookmark with an
optional prefix. Point it at a scored orchestration and get one PDF per
instrument/part in seconds; pick the destination folder, and the open document is
left untouched. No bookmarks? **Split into files by page range** (File → Export)
divides the page sequence instead — **every N pages**, or **custom ranges** like
`1-3, 4-8, 9-10` where each range becomes its own file — into a folder, the open
document untouched.

**Flatten** to a guaranteed-flat PDF, or
export pages as PNGs (single or ZIP) and form data as JSON / CSV. Save back over the
original, or as a flattened or editable copy. **Print** the current document —
fills, signatures, and all — straight from the **File** tab through your
browser's print dialog.

**Reduce file size** (File → Save as) shrinks a PDF two ways and shows the
before→after size before you save. **Optimize** is lossless — it strips redundant
data and keeps selectable text, but mostly helps bloated files (little effect on
scans). **Compress** re-renders pages as JPEG images at a chosen quality: big
savings on scans, but it flattens the document, so selectable text and search are
lost — best for scanned/image-heavy PDFs.

**Pull the contents out** (File → Export): **Document text (.txt)** dumps the
document's text layer to a plain-text file, and **Embedded images (ZIP)** bundles
the pictures inside the PDF into a zip — JPEGs come out as-is, other images are
re-encoded as PNG/TIFF. Text extraction reads the existing text layer, so a
scanned (image-only) page has nothing to give and contributes nothing (there's no
OCR), and complex multi-column layouts may not preserve reading order.

**This page's table → spreadsheet** (File → Export → *This page's table (XLSX)*,
*(CSV)*, or *(ODS)*) clusters the current page's text into rows and columns and
saves it as an Excel **.xlsx**, an OpenDocument **.ods**, or a **.csv**. It's a **best-effort** extraction of
grid-style tables — merged cells, multi-line cells, and irregular layouts may come
out wrong, so **review the result**. Like the text export it reads the text layer
(OCR a scanned page first), and like everything else the spreadsheet is built on
your machine (no office-suite dependency — Nib writes the minimal file format
itself).

### Find in the document
Type in the **Find** box (or press **Ctrl/Cmd+F**) to highlight every match. Step
through them with the **‹ ›** buttons or **Enter** / **Shift+Enter**, and the
readout next to the box shows which match you're on out of the total (`3/12`).

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

### Choose your layout
Pick how the commands are presented from **⚙ Settings → Layout**: the classic
**Menus** (File / Edit / View), a compact **Toolbar** of dropdowns and icons, or
**Both**. The choice is saved in your vault. (Defaults to Menus.)

### Dark or light
Tap the **sun/moon** button in the top-right to switch between the dark
(Catppuccin Mocha) and light (Catppuccin Latte) themes. The choice is saved in
your vault. (Defaults to dark.)

### Private by design
- The web interface binds **`127.0.0.1` only** — never reachable from the
  network; writes are guarded by a per-process CSRF token and a loopback-origin
  check. Two deliberate exceptions, both of which you start and neither of which
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
| **You timestamp a document** (`nib timestamp`, or Finalize with timestamping) | A **SHA-256 of the document** — never the document | four public OpenTimestamps calendar servers |
| **You verify a timestamp** | The transaction/block lookup for the proof | up to three public block explorers |
| **You Finalize with an RFC-3161 timestamp authority** | A **digest of the signature** | the TSA URL *you* typed |
| **You open a document by URL** | The request for that document | the host you named |
| **You run a co-signing session** | The document itself, to your counterpart, over a channel pinned to their key | the peer you pinned — and anyone who scans the port can see it is open |
| **You run `nib rendezvous`** | Queries that reveal this machine's public IP | strangers on the BitTorrent DHT |
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
- **Today, the only thing in Nib that joins the DHT is the `nib rendezvous` diagnostic**,
  which prints a warning before it opens a socket and publishes nothing. Remote co-signing
  currently needs a `host:port` you type. As the peer-finding work lands, an armed ceremony
  will use the DHT too, and it will do so only while the ceremony is running.

If none of that is acceptable for a particular document, sign it locally or on the same
network, and don't run `nib rendezvous` — those paths use no internet at all.

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
later from the **⚙ menu → Manage authorized keys…**.

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
the time, the document opens as soon as you unlock. This works the same on every
platform. It used to be Linux-only and it worked by killing the running process
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
startup check off — and, when a newer release exists, offers to download the
build matching your OS and architecture (a `.deb` for a package install,
otherwise the raw binary).

This is the only call Nib makes **on its own** — the only one you never asked
for — and it's a **version query — no document data, no telemetry**: your
documents never leave your computer. It is not the only call Nib can make —
timestamping, opening by URL and co-signing all use the network when you ask
them to — and every one of them is listed under
[What leaves your computer](#what-leaves-your-computer). Turn the
startup check off from **⚙ Settings → Check for updates on startup** (saved in
your vault), or set `NIB_NO_UPDATE_CHECK=1` to force it off regardless (clicking
the version pill still checks either way). Nib only notifies and downloads — it never installs or replaces
itself; you apply the update the way you installed (`apt` / `install.sh`, or by
swapping the binary).

### Options

| Variable | Effect |
| --- | --- |
| `NIB_ADDR` | Pin a fixed loopback address (e.g. `127.0.0.1:8791`) instead of a random port. Must be loopback (`127.0.0.1`, `localhost`, or `::1`) — a non-loopback address is refused at startup. |
| `NIB_NO_BROWSER` | Don't open a window — just serve and log the URL (headless / remote). |
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
| `nib verify [--json] FILE…` | Report each file's signature integrity. |
| `nib optimize IN -o OUT` | Losslessly shrink a PDF (or `-w FILE…` to rewrite in place). |
| `nib merge IN… -o OUT` | Concatenate PDFs, in order, into one. |
| `nib sanitize IN -o OUT` | Strip identifying metadata and active content — JavaScript, auto-actions, embedded files (or `-w FILE…`). |
| `nib sign IN -o OUT --cert ID.p12` | Certify a PDF with an imported `.p12` identity. |
| `nib rotate IN -o OUT --deg N` | Rotate pages by 90/180/270° (`--pages 1-3,5` to limit; or `-w FILE…`). |
| `nib pages IN -o OUT --keep SEL` | Keep/reorder pages (`--keep 1-3,5`) or delete them (`--remove 2,4`). |
| `nib split IN --out-dir DIR …` | Burst into one file per chunk (`--every N`), range (`--ranges 1-3,4-8`), or `--bookmarks`. |
| `nib encrypt IN -o OUT` | Add AES-256 password protection (`--password-file FILE` or `$NIB_PDF_PASSWORD`, required; an already-encrypted PDF is reported, not re-encrypted). |
| `nib decrypt IN -o OUT` | Remove password protection / owner restrictions (`--password-file FILE` or `$NIB_PDF_PASSWORD`; already-plain PDFs pass through). |
| `nib nup IN -o OUT --n N` | Place N pages per sheet — 2/4/6/9/16… (`--border` for outlines). |
| `nib normalize IN -o OUT` | Resize every page to the document's most common page size — make a mixed-size PDF uniform (content scaled to fit, centred; orientation kept). |
| `nib pdfa IN -o OUT` | Convert to a **PDF/A-2b** archival candidate (embed sRGB OutputIntent + PDF/A XMP, strip active content). Refuses documents with non-embedded fonts or encryption. Verify the result with [veraPDF](https://verapdf.org/) — Nib can't certify conformance itself. |
| `nib pagenum IN -o OUT` | Stamp running page numbers or Bates numbering (`--prefix ABC --pad 6 --position br --total`). `--continuous (-w \| --out-dir DIR) FILE…` threads one counter across a whole file set (multi-file Bates production). |
| `nib pagelabels IN -o OUT` | Set logical page labels — one `--range PAGE:STYLE[:START[:PREFIX]]` per section (STYLE = `decimal`/`roman-lower`/`roman-upper`/`alpha-lower`/`alpha-upper`/`none`), e.g. `--range 1:roman-lower --range 5:decimal`. |
| `nib fill IN --data D` | Fill a form: a JSON or **XFDF** record (`--data x.json\|.xfdf -o OUT`, the inverse of *Export form data*) or a **CSV mail-merge** (`--data rows.csv --out-dir DIR` — header row = field names, one filled PDF per row; `--name-col COL` names each output). Filling removes any existing signature. |
| `nib export-xfdf IN -o OUT` | Export a form's field data as **XFDF** — the XML interchange format Acrobat and Foxit read and write (the inverse of `nib fill --data x.xfdf`). |
| `nib attachments IN [--json]` | List embedded files; `--extract NAME -o OUT` pulls one out, `--add FILE -o OUT` embeds one. |
| `nib outline IN [--json]` | List the document's bookmark outline (indented by level, or JSON). |
| `nib register` / `nib unregister` | **Windows only.** Add or remove Nib from Explorer's "Open with" menu for PDFs (per-user, no admin). Windows reserves the *default* handler for the user to pick. |
| `nib watch DIR --do OP` | Run `timestamp`/`optimize`/`sanitize` on each PDF added to `DIR`, until interrupted. |
| `nib discover` | Report what link-local peer discovery can see from this machine: which interfaces were joined and why, whether announcements left, and what came back. Local network only. |
| `nib rendezvous` | Report whether the BitTorrent DHT that remote co-signing uses is reachable, and what this machine's public address looks like from outside. **Contacts the public internet** — it prints a notice before it opens a socket. Publishes nothing. |
| `nib version` | Print the version. |

Commands that produce a PDF write it to `-o`/`--out`; `timestamp` writes a
sidecar `.ots` beside each input and `verify` prints a report. Flags may go
before or after the file arguments. The exit status is non-zero on failure —
`verify` returns `2` when a signature is invalid or absent — so a check drops
straight into a script:

```sh
nib verify contract.pdf && echo "signature intact"
for f in *.pdf; do nib timestamp "$f"; done   # re-runnable: already-stamped files are skipped
nib sanitize -w *.pdf          # scrub a whole folder in place
nib optimize in.pdf -o - | nib sanitize - -o out.pdf   # compose in a pipeline
```

For `optimize`, `merge`, `sanitize`, `sign`, `rotate`, `pages`, `encrypt`,
`decrypt`, `nup`, `normalize`, `pagenum`, `pagelabels`, `fill` (JSON or XFDF),
and `export-xfdf`, a filename of `-` reads a PDF from stdin, and `-o -` writes
the result to stdout (refused when stdout is a terminal), so the commands chain
together.

`decrypt`, `encrypt`, `normalize`, `nup`, `optimize`, `pagelabels`, `pagenum`,
`pages`, `rotate`, and `sanitize` take `-w`/`--in-place` to rewrite each file
given instead of writing a single `-o` output — the batch form for a folder.
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

`nib watch DIR --do timestamp|optimize|sanitize` runs that operation on each PDF
dropped into `DIR` and keeps running until you stop it (Ctrl-C) — the "process
my inbox" / scheduled-job workflow. It polls (no background file-watching
dependency), waits for each file to finish copying before acting, and handles
each file once. `timestamp` writes a `.ots` sidecar; `optimize`/`sanitize`
rewrite in place — and so skip any signed PDF that lands in the directory,
reporting it, rather than silently invalidating its signatures. Run `nib <command> -h` for a command's own flags.

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
internal/cli     headless subcommands (timestamp, verify, optimize, merge, sanitize, sign)
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

**⚙ Settings & Help → About Nib…** shows these in-app — a plain-English account of what a Nib
signature does and doesn't prove, plus the licence and third-party notices read
straight from the shipped files.
