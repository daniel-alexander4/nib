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

### Real redaction
Draw redaction boxes and press **Apply**. Nib re-renders those pages flat so the
content underneath is **actually gone** — not just hidden behind a black
rectangle. (Verified: a redacted page exposes no hidden text or form field.)

### Sign & finalize — tamper-evident
**Finalize & sign** seals the document with a certification signature from an
identity kept in your vault, stamps a visible "Finalized {date}", and optionally
adds a password and a trusted RFC-3161 timestamp. Any later edit breaks the
signature — that's the point. Export your public certificate so others can verify
it's you. Every PDF you open also shows a **signature badge**: untampered,
modified, or unsigned.

### Pages & export
Rotate, delete, **append**, and reorder pages. **Flatten** to a guaranteed-flat
PDF, or export pages as PNGs (single or ZIP) and form data as JSON / CSV. Save
back over the original, or as a flattened or editable copy.

### Choose your layout
Pick how the commands are presented from **⚙ Settings → Layout**: the classic
**Menus** (File / Edit / View), a compact **Toolbar** of dropdowns and icons, or
**Both**. The choice is saved in your vault. (Defaults to Menus.)

### Private by design
- Binds **`127.0.0.1` only** — never reachable from the network; writes are
  guarded by a per-process CSRF token and a loopback-origin check.
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
