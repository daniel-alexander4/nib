# Nib

[![License: GPLv3](https://img.shields.io/badge/license-GPLv3-blue.svg)](LICENSE)
![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)
![Platforms](https://img.shields.io/badge/platforms-Linux%20%7C%20macOS%20%7C%20Windows-555)
![100% local](https://img.shields.io/badge/data-100%25%20local-success)
![cgo-free](https://img.shields.io/badge/build-pure%20Go%2C%20single%20binary-00ADD8)

**Fill, sign, flatten, and redact PDFs on your own machine.** Nib opens like a
desktop app, but it's really just a tiny local web server and your browser — so
nothing you open, type, or sign ever leaves your computer. No accounts, no cloud,
no telemetry. Your documents, signatures, and signing identity live in an
encrypted vault that only your SSH key can open.

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
A border's thickness is yours to set, in points.

### Edit existing text
**Edit → Edit text**, then drag a box over baked-in text. Nib covers it with a
fill sampled from the background and drops an editable box prefilled in the
original's size, colour, and closest font (serif / sans / mono, bold, italic) —
so a fix reads like an edit, not a patch. The page stays sharp and vector. The
original text stays underneath (it's a visual edit) until you press **Remove
originals**, which flattens just the edited pages so the old text is gone for
good — or until you flatten / finalize the whole document.

### Real redaction
Draw redaction boxes and press **Apply**. Nib re-renders those pages flat so the
content underneath is **actually gone** — not just hidden behind a black
rectangle. (Verified: a redacted page exposes no hidden text or form field.)

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
  and the session tears down after a single exchange.

**Reachability for live sessions.** Nib runs no relay, rendezvous, or NAT-traversal
infrastructure of its own (that would mean a server, and Nib has none). The dialing
peer reaches the receiver's armed listener directly, so the receiver makes their
chosen `host:port` reachable one of two ways:

- **Port-forward** the chosen port on their router to their machine, or
- **Share a private network** you both already trust — a VPN such as **Tailscale**
  or **WireGuard** — and bind / dial the address it hands you.

If the receiver is behind **CGNAT** (common on mobile and some ISPs), port-forwarding
won't work — use the VPN path. The security model doesn't depend on *how* you reach
each other: the pinned-key handshake holds over any transport.

### Pages & export
**Combine PDFs…** (File tab) assembles several documents into one: add the files,
arrange them with ↑ ↓, and they merge top-to-bottom into a new document — then
reorder individual pages across them by dragging thumbnails. It works even with
nothing open, and the result is a new, unsigned document (Save As to keep it).

Rotate, delete, **append**, and reorder pages — **drag a page's thumbnail** in the
sidebar to move it where you want. Rotate every page at once with **Rotate all ↺ / ↻**
on the **Edit** tab, or hover a thumbnail to rotate (either direction) or delete a
single page. **Shift- or Ctrl/Cmd-click thumbnails** to select several at once, then
rotate or delete the whole selection from the bar above the thumbnails. **Extract pages…** saves a range (type `1-3, 5`) as a new PDF without
touching the open document, and **Insert blank page** drops a fresh page — matching
its neighbour's size — after the current one. **Duplicate page** drops a copy of the
current page right after it, and **Insert PDF…** splices another PDF in before the
current page (before page 1 to prepend a cover; use **+ Append PDF** for the end).
**Page numbers…** stamps a running
number onto every page at the corner you choose — add a prefix and a zero-pad
width for Bates numbering (e.g. `ABC` + width 6 → `ABC000001`), or tick "of N" for
classic "Page 3 of 10". Going the other way, **N-up…** combines several pages onto
each sheet (2-up, 4-up, up to 16) for printing or handouts, in reading order, with
an optional border — each sheet keeps the document's page size. **Crop…** trims
the margins away — draw a box around the part to keep and every page (or just the
current one) is cut down to it. It's a re-crop, not a re-render, so quality is
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

**Pull the contents out** (File → Export): **Document text (.txt)** dumps the
document's text layer to a plain-text file, and **Embedded images (ZIP)** bundles
the pictures inside the PDF into a zip — JPEGs come out as-is, other images are
re-encoded as PNG/TIFF. Text extraction reads the existing text layer, so a
scanned (image-only) page has nothing to give and contributes nothing (there's no
OCR), and complex multi-column layouts may not preserve reading order.

### Find in the document
Type in the **Find** box (or press **Ctrl/Cmd+F**) to highlight every match. Step
through them with the **‹ ›** buttons or **Enter** / **Shift+Enter**, and the
readout next to the box shows which match you're on out of the total (`3/12`).

### Keyboard shortcuts
- **PageUp / PageDown** — previous / next page; **Home / End** — first / last page.
- **Ctrl/Cmd + + / − / 0** — zoom in / out / fit to width.
- **Ctrl/Cmd + S** save, **+O** open, **+F** find, **+B** toggle the sidebar.

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
- Binds **`127.0.0.1` only** — never reachable from the network; writes are
  guarded by a per-process CSRF token and a loopback-origin check. The one
  deliberate exception is a **live co-signing session you arm yourself**: while
  armed, Nib opens a single routable listener that accepts only the one peer whose
  key you pinned, and tears it down after one exchange (see *Co-sign with a peer*).
- Your image library, signing identity, autofill profile, and recent files live
  in one **AES-256-GCM vault**, encrypted at rest.
- The vault is **sealed to your SSH key**: it unlocks at startup with *no
  password*. Authorize more than one key to use it across machines, and back it
  up / restore it fully encrypted. (Lose every authorized key and the vault is
  unrecoverable — by design.)

---

## Get started

### Run from source
```sh
go build -o nib ./cmd/nib
./nib [file.pdf]
```
The whole UI is embedded in the binary — nothing to fetch at runtime. Nib opens
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

### Other platforms
```sh
./build.sh
```
Cross-compiles a static, cgo-free binary for **Linux, macOS, and Windows**
(amd64 + arm64) into `dist/`, plus Linux `.deb` packages. On macOS and Windows
you run the binary directly — it's fully self-contained.

A `Makefile` wraps these: `make dist` regenerates the third-party notices and
runs the cross-compile/package; `make install` does the same for a local
install; `make notices` regenerates [THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md)
on its own.

### Staying up to date
At startup Nib asks GitHub for its latest release version; if a newer one exists,
a pill appears at the top that downloads the build matching your OS and
architecture (a `.deb` for a package install, otherwise the raw binary). You can
also trigger it any time from the **⚙ Settings & Help** menu → **Check for updates…**.

This is the only call Nib makes on its own, and it's a **version query — no
document data, no telemetry**: your documents never leave your computer. Turn the
startup check off from **⚙ Settings → Check for updates on startup** (saved in
your vault), or set `NIB_NO_UPDATE_CHECK=1` to force it off regardless (the manual
**Check for updates…** still works either way). Nib only notifies and downloads — it never installs or replaces
itself; you apply the update the way you installed (`apt` / `install.sh`, or by
swapping the binary).

### Options

| Variable | Effect |
| --- | --- |
| `NIB_ADDR` | Pin a fixed loopback address (e.g. `127.0.0.1:8791`) instead of a random port. Must be loopback (`127.0.0.1`, `localhost`, or `::1`) — a non-loopback address is refused at startup. |
| `NIB_NO_BROWSER` | Don't open a window — just serve and log the URL (headless / remote). |
| `NIB_NO_UPDATE_CHECK` | Disable the automatic startup update check (the manual **Check for updates…** still works). |

---

## How it works

Nib is a small Go server that embeds the entire single-page UI and the
[pdf.js](https://mozilla.github.io/pdf.js/) engine. Your browser renders and
fills the PDF; [pdfcpu](https://pdfcpu.io) stamps, flattens, and redacts;
[pdfsign](https://github.com/digitorus/pdfsign) signs and verifies. Pure Go, no
cgo, permissive dependencies only — so it builds to one portable binary per
platform.

```
cmd/nib          entry point — bind loopback, serve, open the window
internal/server  HTTP API + embedded UI, loopback-only guard
internal/vault   encrypted store (AES-256-GCM, sealed to your SSH key)
internal/pdfops  pdfcpu stamping / flattening / redaction
internal/sign    signing + signature verification
web/             the single-page UI and the vendored pdf.js engine (embedded)
```

---

## License

Nib is free software under the **GNU General Public License v3.0** — see
[LICENSE](LICENSE). Copyright © 2026 Daniel Alexander.

Distributed **as-is, with no warranty** of any kind, to the extent permitted by
law (GPLv3 §§15–16). You may use, study, share, and modify it under the GPL;
derivative works must also be released under the GPL.

Nib also incorporates third-party software (Go modules and the vendored pdf.js
engine), all under GPLv3-compatible permissive licenses (BSD, MIT, Apache-2.0).
Their required copyright and license notices are collected in
[THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md), regenerated with
`build/gen-notices.sh`.

**⚙ Settings & Help → About Nib…** shows these in-app — a plain-English account of what a Nib
signature does and doesn't prove, plus the licence and third-party notices read
straight from the shipped files.
