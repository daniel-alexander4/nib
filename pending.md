# Pending

## SSH-key unlock (replaces the password) — accepted tradeoff + unverified UI

- The vault is now sealed to the user's SSH key (no password). Consequence,
  accepted deliberately: the vault is **exactly as safe as the on-disk SSH key** —
  anyone with both the vault file and the (unencrypted) private key decrypts it,
  and **losing the key = unrecoverable vault, including the PDF-signing identity**.
  No break-glass recovery by choice.
- The Go side is fully tested (sshkey wrap/unwrap/generate; vault v2 round-trip,
  key-missing, encrypted-at-rest, password→SSH migration; server enroll/status/
  CSRF/Origin). The **first-run wizard, key-missing recovery, and migration UI are
  unverified in-sandbox** (no live server) — confirm on a real machine.
- A moved key currently recovers only by **restoring it to its recorded path**
  (the wizard retries unlock); relocating to a new path would need a small
  re-point endpoint — deferred.

Open questions, deferred work, and residual doubts carried forward from the
Nib design and build.

## Residual doubts (from scrutinize)

1. **pdf.js ↔ pdfcpu field round-trip / XFA forms.** Interactive fill goes through
   pdf.js `saveDocument()` (M1), which sidesteps name-matching for the GUI path.
   But XFA / dynamic forms are not guaranteed to round-trip, and the *headless*
   fill path (mail-merge, M5) will use pdfcpu `FillForm` and does depend on exact
   field-name matching. Verify against real-world forms when M5 lands.

2. **Coordinate transform (canvas ↔ PDF points).** Not exercised in M1 (AcroForm
   fill is name-based, not coordinate-based). It becomes load-bearing in M4 (flat-
   PDF overlays) and M5 (stamping). Isolate it in one unit-tested function then;
   page rotation and non-zero MediaBox origin are where bugs will hide.

3. **Vector appearance-flatten feasibility (M5).** Drawing each widget's `/AP /N`
   appearance onto page content + removing widgets is the canonical flatten but
   pdfcpu doesn't expose it as one call. If its object model resists, vector
   flatten stays an upgrade and rasterize remains the default flatten.

4. **True redaction must actually remove content (M9).** Implement via full-page
   rasterization of any page carrying a redaction — a rasterized patch over a
   region leaves the text underneath (the classic fake-redaction leak).

5. **TSA timestamp (M5) adds an optional network dependency** — keep it opt-in,
   against the no-remotes default; fall back to a local date + warning when offline.

6. **Self-signed signatures (M5) = integrity, not third-party identity.** A valid
   Nib signature proves untampered-since-finalize; it does not prove signer
   identity to others unless they trust the exported cert. Stripping the signature
   is detectable (absence of a valid signature), not preventable.

7. **Wizard "create" has no destination field.** It always targets
   `DefaultNewKeyPath()` (`~/.ssh/id_ed25519`) and `sshkey.Generate` refuses to
   overwrite, so a first-run user who already has `id_ed25519` but wants a *new,
   separate* key hits a dead-end (ErrExist, no way to pick another path until the
   vault is unlocked and the keys dialog — which does take a path — is reachable).
   Auto-selection avoids it in the common case (create is only pre-checked when no
   `~/.ssh` keys exist), so this is the deliberate-pick edge only. Add a path field
   to the wizard's create option if it bites.

## Stamp placement (M3) — switched to the pdfcpu pipeline; verify drag/resize

- The pdf.js annotation-editor STAMP path was **dropped**: its `saveDocument()`
  baking placed stamps a few points too high (signature/date/"Approved" landed
  above the line they were dropped on). Placement is now a Nib **overlay widget**
  (draggable + resizable), baked server-side via `pdfops.StampImages` at
  save/flatten — the same coordinate-accurate path as auto-detected fields.
- Still needs test-time confirmation on a real machine (the sandbox can't run the
  live UI): drag/resize feel, that placement matches the baked output exactly, and
  that aspect is preserved. The vault image library and `/api/images` CRUD are
  fully Go-tested regardless.

## Blank-line auto-detect (M4) — heuristic, unverified

- `findUnderlines()` scans the rendered page canvas for long horizontal dark runs
  and drops a FreeText box above each. It's pure heuristic (no tuning against real
  documents in-sandbox) and runs through the same pdf.js editor internals as the
  M3 stamps, so it shares their test-time verification need. Expect to tune the
  thresholds (min run length, row-band dedup) once tested on real forms.

## M5 deferred / best-effort

- **Vector flatten deferred.** Rasterize flatten (client renders pages → server
  assembles an image-PDF) is implemented and is the working flatten. The vector
  appearance-flatten (residual doubt #3) remains the upgrade — not built; raster
  is the default, as planned.
- **Autofill is best-effort.** It sets `annotationStorage` values via
  `getFieldObjects()` and calls `viewer.refresh?.()`; whether the rendered inputs
  visibly update needs test-time confirmation (shares the pdf.js-internals risk).
- **Field validation not implemented.** Required/regex pre-finalize checks were
  scoped out of M5 to contain it; add when needed.
- Finalize/export front-end (render-to-PNG, multipart) is unverified in-sandbox;
  the Go signing/encrypt/assemble/export core is fully unit + HTTP tested,
  including a sign→verify→tamper round trip.

## M7–M9 front-end (verify at test time)

- Page-op thumbnail buttons (M7), annotation tool toggles (M8), and the redaction
  box-drawing + apply flow (M9) are unverified in-sandbox (no live server). The
  Go side is fully tested: page ops, and redaction's guaranteed-removal property
  (a redacted form page exposes no field) and page preservation. The redaction
  box→canvas coordinate mapping (pointer rects → page fractions → painted on the
  2× render) is the spot to eyeball on a real machine.

## Verification gap (M1–M2)

- The **live in-browser flows** were NOT verified end-to-end in the build sandbox:
  it signal-kills any process that binds a listening socket, and pdf.js's worker
  won't load over `file://`. Verified instead: the full Go backend incl. the M2
  auth/vault/CSRF/idle logic (unit tests), and the UI rendered statically via
  real chromium screenshots (viewer shell + the setup/login overlay). Not yet
  live-confirmed: the PDF render → fill → saveDocument → save round-trip, and the
  setup→login→lock→unlock click-through. Confirm by running `nib sample.pdf`
  on a real machine and exercising the flow.
