# Nib

A local, single-binary tool for filling, signing, and finalizing PDFs. It runs a
loopback-only web server and opens its UI in a chromeless app-mode window, using
[pdf.js](https://mozilla.github.io/pdf.js/) to render pages and interactive form
fields and [pdfcpu](https://pdfcpu.io) / [pdfsign](https://github.com/digitorus/pdfsign)
for stamping, flattening, and digital signatures.

Pure Go, no cgo, permissive dependencies only (Apache-2.0 / BSD / MIT) — it
cross-compiles to Linux, macOS, and Windows as one self-contained binary.

## Status

Built in milestones. **All milestones M1–M9 complete.** M1 core fill loop ·
M2 auth + encrypted vault · M3 image library + signature + stamps · M4 flat-PDF
overlays + sources · M5 flatten + finalize-and-sign + export · M6 cross-platform
packaging · M7 page operations · M8 annotations · M9 true redaction.

M1 — the core fill loop:
- **File ▸ Open…** — type a path or URL, or browse the filesystem. Browsing
  opens by path, so the file saves in place and is remembered in Recent.
  (Drag-drop and URL opens have no path and save as a copy.)
- Render pages and fill existing AcroForm fields in place (pdf.js).
- Save the filled PDF back over the original file.
- Signature **verify on open** — every document shows an untampered / modified /
  unsigned badge (signing itself lands in M5).
- Page navigation: thumbnails, outline, in-page search.

M2 — SSH-key unlock + encrypted vault:
- A single AES-256-GCM vault file (`os.UserConfigDir()/nib/vault.nib`),
  encrypted at rest with a random content key that is **sealed to your SSH public
  key** (via `age`) in a key slot. At startup the content key is unwrapped with
  your SSH private key, so the vault opens with **no password** — a first-run
  wizard enrolls an existing `~/.ssh` key, a key you point at, or a freshly
  created `~/.ssh/id_ed25519`.
- There is no password; losing the SSH key means the vault is unrecoverable, by
  design. (An old password vault is migrated by entering its password once.)
- The vault is exactly as protected as your on-disk SSH key. Writes are guarded by
  a per-process CSRF token + loopback Origin; the server binds `127.0.0.1` only.
- **Manage authorized keys** (File ▸ Manage authorized keys…): the content key
  can be sealed to more than one SSH key, so you can authorize a second machine's
  key — paste its `authorized_keys` line or pick a local `~/.ssh` key — and revoke
  old ones later. Removing the only enrolled key, or the key in use this session,
  is refused so the vault can't be orphaned.
- Encrypted vault **backup / restore** (a backup opens only on a machine whose
  enrolled key matches a slot).

M3 — image library + signatures + stamps:
- An **encrypted image library** in the vault: add images (file or URL), draw a
  signature on a pad (saved as a transparent PNG), delete — all encrypted at rest.
- Place a library image, a drawn signature, or a quick-stamp (checkmark, date,
  "Approved") onto the page as a **draggable, resizable overlay**; it's baked
  server-side (`pdfops` image stamp) at save/flatten, the same coordinate-
  accurate pipeline as the auto-detected fields — not pdf.js's stamp editor,
  whose `saveDocument()` baking placed stamps a few points too high.

M4 — flat-PDF overlays + sources:
- A **Text tool** (pdf.js FreeText) to type anywhere on a page — works on flat
  PDFs with no form fields; checkmarks reuse the M3 stamp path.
- Open a PDF from a **URL** (server-side fetch), **drag-and-drop**, or the
  **recent-files** list (stored in the vault).
- A best-effort **field detector** (the Detect button) that proposes fillable
  widgets on a page (heuristic — proposes, doesn't perfect):
  - a text box above each blank fill-in **line** (including faint, light-gray
    rules on modern forms, found by local contrast) and inside each blank **box**;
  - a **table** grid → one input per blank cell (cells with printed text are
    skipped, as are text-filled boxes with no blank space);
  - a **"circle one"** choice set — `Y/N`, or options found near a "(circle one)"
    note (`Checking | Savings | Visa | MC`, `Male/Female`) — → click to ring your
    pick, baked as a circle over a letter or a pill around a word.

M5 — flatten + finalize-and-sign + export:
- **Finalize & sign**: a certification signature (DocMDP "no changes allowed")
  with a self-signed identity kept in the vault, a visible "Finalized {date}"
  stamp, optional password protection and RFC 3161 timestamp. Any later edit
  invalidates it — the tamper-evidence. Export the public certificate so others
  can confirm it's you.
- **Flatten**: rasterize the filled pages into a guaranteed-flat image-PDF.
- **Export**: pages as a PNG ZIP, the current page as PNG, and form data as
  JSON or CSV.
- **Save dialog**: flatten, editable save, finalize, and the ZIP/PNG exports
  prompt for a name and folder (browse from a default of `~/nib`); the file
  is written there with a name like `<original>-flattened.pdf`.
- **Autofill** matching form fields from a saved profile.

M7 — page operations:
- Rotate or delete a page from its thumbnail; **append** another PDF; reorder
  (via the API). Structural ops run server-side in pdfcpu on the saved bytes,
  so current edits are preserved.

M8 — annotations:
- **Text**, **Highlight**, and freehand **Draw** tools (pdf.js editors, mutually
  exclusive), baked into the PDF on save like everything else.

M9 — true redaction:
- Draw redaction boxes on pages; **Apply** re-renders each marked page with the
  boxes painted in and replaces it with that flat image, so the content under a
  box is genuinely removed — not just covered. Non-marked pages keep their vector
  text. (Guaranteed-removal verified: a redacted form page exposes no field.)

## Build & run

```sh
go build -o nib ./cmd/nib
./nib [file.pdf]
```

The binary embeds the entire UI, so it needs no assets at runtime.

### Install on this machine (Debian/Ubuntu)

`./install.sh [version]` builds Nib for the host architecture, packages a
`.deb`, and installs it. Afterwards **Nib** appears in the applications menu
under **Office**. Launching it (`nib --replace`) first terminates any
already-running Nib instance, then starts a fresh one — so clicking the menu
item always gives you a single, current window. Run `./install.sh` again to
upgrade in place.

### Cross-platform build & packaging

`./build.sh [version]` cross-compiles a cgo-free static binary for Linux, macOS,
and Windows (amd64 + arm64) into `dist/`, and builds Linux `.deb` packages when
[nfpm](https://nfpm.goreleaser.com) is installed
(`go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest`). The `.deb` installs
the binary to `/usr/bin/nib` plus a desktop entry and icon, so it appears in
the app launcher. Install a built package with `sudo apt install ./nib_<ver>_amd64.deb`
(distributed peer-to-peer — there is no hosted apt repository).

### Environment variables

- `NIB_ADDR` — pin a fixed loopback address (e.g. `127.0.0.1:8791`) instead of
  a random port. Useful for headless or remote runs.
- `NIB_NO_BROWSER` — don't launch a browser window; just serve and log the URL.

## Architecture

- `cmd/nib` — entry point: bind loopback, serve, launch the app-mode window.
- `internal/server` — HTTP API (`/api/open`, `/api/pdf`, `/api/save`, `/api/upload`)
  and the embedded static UI; loopback-only guard.
- `internal/sign` — signature verification (pdfsign).
- `internal/browser` — per-OS app-mode browser discovery, tab fallback.
- `internal/testpdf` — test-only AcroForm fixture generator (pdfcpu).
- `web/` — the single-page UI and the vendored pdf.js engine (embedded).

The interactive fill happens in the browser: pdf.js's `saveDocument()` writes the
user's edits back into the PDF and the bytes are persisted by the server. pdfcpu
joins the binary when stamping/flattening land in later milestones.

## License

Nib is free software, licensed under the **GNU General Public License v3.0** —
see [LICENSE](LICENSE). Copyright © 2026 Daniel Alexander.

It is distributed **as-is, with no warranty** of any kind, to the extent
permitted by law (GPLv3 §§15–16). You may use, study, share, and modify it under
the terms of the GPL; derivative works must also be released under the GPL.
