# ADR-158 — the server draws the signature block, and accepts no picture of one

**Status:** accepted
**Date:** 2026-10-12
**Extends:** ADR-009 (one door) to the block's picture; ADR-147 (a document says what a step will do before the
step) — the Sign now preview shows the picture that will be signed, not only its place.
**Supersedes:** no ADR. It supersedes the arrangement the co-sign path was built with and
`PLAN-signing-ceremony.md` P06.S06 left standing (`/pending 317`, `/pending 286`, `/pending 305`): the window
rasterised the quoted lines on a canvas (`renderAttestation`) and posted the picture back.
**Bound by:** ADR-142 (the vendored signing library differs from upstream in its two `verify/` files only — its
`sign/` half is not patched, so the block stays an IMAGE the library embeds); ADR-150 (a face is renamed when a
document embeds it by name — these are not embedded, so they are not renamed).
**Context:** `/pending 367`, decided by Dan 2026-10-12 (option A, with the new dependency approved). After P06.S06
the quoted lines and the signed `/Reason` were built by one function from one input. The step between them was not:
the window drew the lines and the server embedded whatever picture it was handed, comparing nothing. A stale or
modified window therefore produced a document whose PICTURE said one thing and whose signed text another — and a
reader looks at the picture. The wrap was measured against a Helvetica stand-in for "the browser's sans-serif".
**Applies:** `internal/p2p` (`blockdraw.go`, `attestation.go`, `cosign.go`), `internal/server` (the five signing
routes and three quotes), `internal/ceremony` (`convene.go`), `web/app.js`, `build/pairrepro.sh`,
`build/ceremonyrepro.sh`.

## Decision

**1. The block is drawn by `p2p.Contribute`, from the attestation it signs.** `Contribute` has no picture
parameter; it calls `Attestation.BlockImage`, which rasterises `AppearanceLines()` into the block's frame (280 × 84
points at three pixels a point, the text at `min(0.7 × line slot, 9pt)` as before). Every contribution carries a
visible block — there is no invisible co-signature to ask for by sending nothing.

**2. No route accepts a picture.** `POST /api/cosign/sign`, `/api/session/initiate`, `/api/session/respond`,
`/api/ceremony/hop` and `/api/ceremony/sign-turn` answer **400** with one sentence (`clientPictureRefused`) to a
request carrying an `appearance` part or field, before anything is signed, parked or dialled. Refused, not ignored:
a window still sending a picture is a window still drawing one. `p2p.Confirmer.Confirm` returns no picture.

**3. One door turns a line into glyphs** — `shapeBlockLine` — and three things read it, so they cannot disagree:
the wrap (`blockLineWidth`: the shaped advance of the faces drawn), the refusal (`BlockDrawable`), the drawing
(`drawBlock`). The lines, the wrap and the signed `/Reason` still come from the one attestation.

**4. A shaper is a dependency: `github.com/go-text/typesetting` v0.3.5** (Unlicense OR BSD-3-Clause, pure Go).
`golang.org/x/image`, which `go.mod` already carried, draws one glyph per rune left to right: it has no GSUB/GPOS and
no bidi. Measured before this was built, 42 runes of a mixed-script block drew as boxes; Arabic does not join, the
Indic scripts do not conjoin, Thai marks are not placed, Hebrew and Arabic run the wrong way. The module's HarfBuzz
port, its bidi and its segmenter are used; its font scanner is not imported.

**5. Explicit faces only.** The chain is the fourteen faces in the binary, in a fixed order (`blockFaceFiles`:
Roboto, then the Noto faces for Arabic, Hebrew, Devanagari, Bengali, Gujarati, Gurmukhi, Kannada, Malayalam, Tamil,
Telugu and Thai, then DroidSansFallback for CJK and NanumGothic, which is preferred for Hangul). No system font is
looked for: the same words draw the same picture on every machine, and a request reads no font directory.

**6. A character no shipped face has is refused, by name, where the text is typed.** Never a box. `BlockDrawable`
is asked wherever `BlockFits` is — at convene (`checkBlocksFit`, per signing party: `ErrUndrawable`), at every
quote and every signing route (`cosignAttestation`, `ownTurnAttestation`), at an acceptance
(`refusedAnswerBlock`: the intent an answer carries need not be the one a quote measured) and where a peer's label
is pinned — so a ceremony nobody could sign is not convened. A line break, a bidirectional override and an emoji are
such characters today.

**7. A block that does not fit is refused, never shrunk and never clipped.** The wrap measures the face drawn; the
line count is still `maxBlockLines` (8, Dan's legibility floor). `drawBlock` is the backstop: more lines than the
limit, a line wider than the frame, or ink outside the picture is an error, not a smaller or a cut-off block.

**8. A quote carries the picture.** `cosignQuote.Image` (and `ceremonyHopQuote.Image`) is the PNG, base64, built
by `quoteFor` — inline, not a second route, so the lines and their picture arrive in one answer and no new route
needs the session door. The picture is a function of the lines alone (no clock, no machine state), so the quote's
picture and the signature's are the same samples whenever the lines are the same, and the lines agree by the
existing quote/sign rule. The window shows that picture in the Sign now preview and draws nothing.

## Measured

- **Fit, the doubt the decision carried.** Twenty-four realistic blocks — a long name, a capacity and a one-sentence
  recital in Latin, Arabic, Devanagari, Thai, CJK, Hangul, Hebrew and a mixed-direction line; two-party, ceremony,
  and ceremony without a capacity — wrapped by the faces drawn: **0 of 24 exceed eight lines** (5 to 8 lines), and
  every line's ink stays inside its own slot (at worst 12.1pt of a 12.7pt slot). The Helvetica estimate had been
  0.97-1.02 of a Latin line's true width, 1.2-1.5 of an Arabic, Devanagari, Thai or Hebrew line's and 2.0-3.6 of a
  CJK or Hangul line's: those lines ran past the frame.
- **Cost** (`TestWhatDrawingABlockCosts`): 3.6 ms for five lines, 4.5 ms for seven, 5.7 ms for eight, 5.3 ms for
  eight lines of mixed scripts. Unshaped it was 1.4-1.9 ms; most of the rest is the PNG encoder (shaping five lines
  is 0.35 ms). Wrapping a 4,096-character recital is 44 ms and the refusal's bisection 113 ms more; a field longer
  than that (`blockMeasureCap`) is refused without being shaped — a million characters are answered in 0.11 s.
- A block's PNG is 19-25 KB; a quote grows by that, base64.

## Consequences

- **A build before this one cannot sign through a server at this one** and is told why (decision 2); the window and
  the server ship together, so this is a stale tab, not a peer. Nothing changes on the wire: the picture never
  crossed it.
- **A record convened by an older build is measured by the new wrap.** `Record` validation runs `checkBlocksFit`,
  so a ceremony whose CJK recital "fit" by the old estimate and does not fit the frame is refused on arrival by
  name. Its blocks would have run past the frame.
- **`nib sign` and Finalize draw no block, and that is not a gap.** They make a CERTIFICATION signature with a
  typed reason (`sign.SignExternal` / `sign.Sign`): there is no attestation, no counterparty and no page allocated
  for a block, and Finalize's own rule is that a certification is invisible. `/pending 367` named the CLI by
  analogy; the trace refused it.
- The receiving consent and the two dial dialogs show no picture before the press — they never did. Showing the
  quote's picture there is a second press in each flow, which is a flow decision, not this one.
- Tier 3's `blockink` supplied its own magenta picture to test the appearance stream; it now reads the block the
  server draws (`test/ui/blockink.test.mjs`). The four red-proof rows that mutated `renderAttestation` mutate
  `drawBlock`.
- **Declared, and outside this decision:** the vendored signing library writes a `/Reason` in UTF-16 without
  escaping, so a recital or pinned label holding a character whose UTF-16 form has a `(`, `)` or `\` byte — most
  CJK, some Arabic — fails nib's read-back check and is not signed. It failed the same way before this, with any
  picture or none. The block is drawn correctly for those scripts; signing them waits on that fault.
