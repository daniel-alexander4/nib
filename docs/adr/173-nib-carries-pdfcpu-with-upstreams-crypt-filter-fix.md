# ADR-173 — nib carries pdfcpu v0.13.0 with upstream's crypt-filter fix, and nothing else

**Status:** accepted
**Date:** 2026-10-14
**Extends:** ADR-066 and ADR-142 (a patched module is a `replace` to a copy under `third_party/`, its `NOTICE.nib`
says what differs, and a guard holds the pin, the replace and "nothing else differs"). Reverses, for this one line,
Dan's 2026-09-30 decision of no pdfcpu fork (`completed.md` 763), on his decision of 2026-10-14 (/pending 892).
**Context:** /pending 892. A real AES-128 disclosure (V 4, R 4, empty user password, `/CF /StdCF << /CFM /AESV2 >>`
with no `/Length`) opens in the window and every pdfcpu-backed step refuses it: `pdfcpu: crypt filter missing entry
"Length"`, from `api.ReadContext`, before nib holds a context. ISO 32000-1 Table 25 makes the key optional. v0.13.0's
`validateCryptFilter` (pkg/pdfcpu/crypto.go:1121) reads `if length == nil && !pdf20` — it demands the key from
every document EXCEPT PDF 2.0. Upstream inverted it; v0.16.2's crypto.go:1295 reads `&& pdf20`. The bump to
v0.16.2 was measured the same day (`memory/deepdives/2026-10-14-pdfcpu-v0130-to-v0162.md`) and is not one: it moves
`ContentDigest` on 24 of 2,326 real files, refuses ~25 kinds of document nib reads today, reclassifies radio
groups, and needs Go 1.26.
**Applies:** `third_party/pdfcpu/` (`pkg/pdfcpu/crypto.go`, `NOTICE.nib`), `go.mod`, `go.sum`,
`librarypatch_test.go`, `internal/pdfread/cryptfilterlength_test.go`, `THIRD-PARTY-NOTICES.md`.

## Decision

**1. `go.mod` replaces `github.com/pdfcpu/pdfcpu` with `third_party/pdfcpu`,** a copy of v0.13.0 that differs in
ONE line of `pkg/pdfcpu/crypto.go` — upstream's own fix, marked "Modified by nib (ADR-173)" as Apache-2.0 §4(b)
asks of a changed file — and adds `NOTICE.nib`. The require line keeps v0.13.0, so every pin that names it
(`contentcostpin_test.go`) still holds.

**2. The copy is not whole, and the guard names what is left out.** The module is 329 MB, 318 of them upstream's
tests, `testdata/`, `pkg/samples/`, the top-level `resources/` and `_scripts/`, none compiled into a package nib
imports. The copy is 11 MB (`pkg/pdfcpu/model` embeds 6.9 MB of certificates and a font). Unlike ADR-066's and
ADR-142's guards, `TestThePdfcpuPatchIsTheCryptFilterLineOnly` allows a missing upstream file only under that
list, and holds crypto.go to upstream's bytes with exactly the one line replaced.

**3. The behaviour is upstream's, both ways.** A 1.x crypt filter with no `/Length` reads; a PDF 2.0 one is now
refused, as v0.16.2 refuses it. `TestACryptFilterWithNoLengthIsReadOutsidePDF20` holds both arms, each red on the
stock line. Measured on the 28 encrypted local PDFs: one more reads (the disclosure — outline, optimize
3,359,954 B, decrypt 3,063,344 B), none reads less, none is PDF 2.0.

## Consequences

- No `ContentDigest` moves: the line runs only for an encrypted document pdfcpu refused before.
- `go.sum` loses pdfcpu's two lines; a replaced module is not checksummed.
- **Drop the copy at the pdfcpu bump** (/pending 898): delete the directory, the replace and the guard. The guard's
  first check fails on any change to the require line, so a bump cannot leave this patch applied to a version it
  was not written against.
- The v0.13.0 optimize hang on a nested-form document (/pending 897) is untouched by this; v0.16.2 does not have it.
- Reporting upstream is moot: v0.16.2 carries the fix.
