# ADR-083 — a PDF/A identification survives only what nib verified, through ADR-032's door

**Status:** accepted. `/pending 641` (2026-10-03). Extends ADR-032; supersedes ADR-082 in part (`Encrypt` and
`RemovePassword` are no longer restated in `pdfread`).
**Applies:** `dropUAIdentification` (`internal/pdfops/uaid.go`) and everything that reaches it — every operation's
rewrite, the save / Save As / signing door `DropUAIdentificationUnlessSigned`, `Encrypt` and `RemovePassword`.

## Context

ADR-032 made every change drop a PDF/UA identification, because nib cannot verify an edit kept a document
conformant and pdfcpu carries the catalog's XMP packet through every write. The drop matched one namespace,
`http://www.aiim.org/pdfua/ns/id/`. A PDF/A identification — `pdfaid:part` / `pdfaid:conformance` in
`http://www.aiim.org/pdfa/ns/id/` — is the same assertion about a different standard, and nothing removed it.

**Measured** (2026-10-03): `Rotate` on `PreparePDFA`'s own output kept `pdfaid:part 2`, and so did `Rotate` on
Ghostscript's PDF/A output. The same review found `Encrypt` → `RemovePassword` returned a labelled document still
claiming PDF/UA: both called `pdfread`'s restated read-optimize-write (ADR-082), which has no place for the drop,
and the census skipped both because their rows had no drive.

## Decision

1. **The PDF/A identification is dropped by the same door, for the same reason.** `isIdentificationNS` names both
   namespaces; `withoutUAIdentification` and `dropUAIdentification` remove either. nib checks none of PDF/A's rules,
   so the case is stronger than PDF/UA's 19-of-106. The PDF/A extension-schema namespaces (`.../pdfa/ns/extension/`,
   `/schema#`, `/property#`, `/type#`, `/field#`) are other URIs, describe a schema, claim nothing, and stay.
2. **The writing doors are `PreparePDFA` and `ConvertPDFAGhostscript`**, exactly as `LabelUA` is PDF/UA's (ADR-033):
   `PreparePDFA` writes its marker in `writeMutated`'s `fn`, which runs after the drop, and Ghostscript writes its own
   packet. Both hand the user a *candidate* to verify with veraPDF; nothing after them drops the claim (the server
   and CLI send the bytes as they came back). That the pure-Go candidate is labelled without veraPDF's measurement is
   their existing, declared contract, not something this ADR adds.
3. **`Encrypt` and `RemovePassword` go through `rewriteWithConf`** under `model.ENCRYPT` / `model.DECRYPT`
   (`protectRewrite`) — the same `ReadOptimized` and write `pdfread` restated, plus the drop, on the plaintext
   context. `pdfread.Encrypt`, `pdfread.Decrypt` and their `rewrite` had no other caller and are gone.
4. **The census asks every row that changes a document.** A row whose tag-fate `drive` cannot serve gets a `uaDrive`
   (Encrypt's output read back through pdfcpu's own decrypt; RemovePassword handed pdfcpu's own encrypt; RedactPages
   given a raster; both PDF/A doors) or a `uaWhy`; a row with neither fails, because a `continue` on a nil drive is
   what hid this. The census fixture carries both identifications and each writing door is exempt for its own claim only.

## Consequences

- **Signing a PDF/A document drops its claim**, as signing a PDF/UA one already did: the signature is a change nib
  cannot verify kept the file conformant. A document already signed keeps both claims — ADR-032's declared gap, now
  for two standards.
- A user who wants a signed PDF/A file re-runs the PDF/A conversion and has it verified; nib does not assert it.
