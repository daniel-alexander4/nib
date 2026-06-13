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
- **"Circle one" choices** → `Y / N`, or option sets near a "(circle one)" note
  (e.g. `Checking | Savings`, `Male / Female`).

It's a smart proposal, not magic — move, resize, retype, or ignore anything it
suggests.

### Circles & pills for multiple choice
Choosing an option marks it the way a person would: a **circle** around a single
letter (or `Y`/`N`), or a **pill** around a whole word — baked cleanly into the
PDF on save.

### One-click stamps
Quick-stamps for the things you reach for most — **today's date**, **"Approved"**,
and a **checkmark**. Drop one on, drag to place, resize to fit.

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
**Highlight** text, **draw** freehand, and add free **text** boxes anywhere on a
page — works on flat PDFs too.

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
**Edit → Scan for hidden content** lists what's lurking in a PDF that you can't
see on the page: auto-run hooks (OpenAction, additional actions), JavaScript,
risky link/widget actions (launch a program, submit a form, open a URL),
embedded files, optional-content layers, and metadata. Then remove it three
ways, strongest fidelity-preserving first:
- **Strip active content** — neutralises every auto-run hook, script and risky
  action while keeping the page text and layout intact.
- **Remove files & media** — deletes only embedded files and media annotations,
  leaving all other interactivity untouched.
- **Flatten to images** — the guaranteed-inert floor: turns every page into an
  image so nothing active can remain (selectable text is lost).

If a strip can't produce a sound document it's reported and your open document
is left untouched, so you can step down to the next method safely. Any removal
produces a new, **unsigned** copy — save it to keep the cleaned version.

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
the document or its hash — and the block is confirmed against two public block
explorers that must agree (point it at your own Esplora endpoint to verify
trustlessly). You can also verify with any other OpenTimestamps tool
(e.g. [opentimestamps.org](https://opentimestamps.org)) — the proof is standard.

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
  One person **arms to receive** (File → *Receive a live co-signature…*), the other
  **dials in** (File → *Co-sign live with a peer…*). The connection is mutually
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
Rotate, delete, **append**, and reorder pages — **drag a page's thumbnail** in the
sidebar to move it where you want. **Flatten** to a guaranteed-flat PDF, or export
pages as PNGs (single or ZIP) and form data as JSON / CSV. Save back over the
original, or as a flattened or editable copy.

### Choose your layout
Pick how the commands are presented from **⚙ Settings → Layout**: the classic
**Menus** (File / Edit / View), a compact **Toolbar** of dropdowns and icons, or
**Both**. The choice is saved in your vault. (Defaults to Menus.)

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
later from **File → Manage authorized keys** (or **More** in the toolbar layout).

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
also trigger it any time from **File → Check for updates…** (or **More** in the
toolbar layout).

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

**File → About Nib…** shows these in-app — a plain-English account of what a Nib
signature does and doesn't prove, plus the licence and third-party notices read
straight from the shipped files.
