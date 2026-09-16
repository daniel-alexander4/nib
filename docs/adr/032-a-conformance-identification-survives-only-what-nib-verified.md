# ADR-032 — A PDF/UA identification survives only what nib verified; any change drops it

**Status:** accepted
**Date:** 2026-09-15
**Context:** `/pending 492`, found while grilling `/pending 486` (whether nib may write the identification).
**Applies:** every path that changes a document's bytes — each operation in `internal/pdfops`, the server's
save and Save As routes, and both signing paths (`handleFinalize`, `nib sign`).

## Decision

**A document's PDF/UA identification — any element or attribute in the `http://www.aiim.org/pdfua/ns/id/`
namespace in the catalog XMP packet — is removed by every change nib makes to the document**, unless the
change is itself the step that verified conformance (none exists yet; `/pending 486` is where one would).

The drop is made **inside the rewrite the change already performs**, never as a second write:

- **Operations** read, change and write through one door: `writeMutated`, or `rewriteWithConf` for the
  operations that used to call a pdfcpu `api` wrapper (it reproduces the wrapper's configuration and panic
  handling). The drop runs between the read and the change. `Append`, `Combine` and `NUp` keep pdfcpu's own
  read path — it skips the optimize pass, and the tag-fate census measures their output — and drop after
  their write with `withoutUAClaim`.
- **Bytes that change a document outside `pdfops`** — pdf.js form fills and annotations posted to
  `handleSave`, bytes posted to Save As, a document about to be signed — go through
  `DropUAIdentificationUnlessSigned`. Save drops when the posted bytes differ from the held document; Save As
  drops unless they equal some open document's bytes; both signing paths drop before the signature.

## Why

`pdfuaid:part` asserts the whole document conforms to PDF/UA. pdfcpu carries the catalog `/Metadata` stream
through an ordinary write, so the assertion survived whatever an edit did. **Measured on veraPDF's own PDF/UA-1
corpus**, four labelled files in both encodings: `AddNotes` kept the identification and failed 7.18.1 t1 on
all four, `StampWatermark` kept it and failed 7.21.4.1 t1 on all four. That is ADR-031 law 1's shape in a
different field — a conformance assertion over content it no longer describes — and it applied to documents
from any producer, not only nib's.

nib cannot tell whether an edit preserved conformance: its checker covers 19 of the 106 rules veraPDF
evaluates. So it does not try. `Rotate` and `Optimize` measurably keep a document conformant and lose the
identification anyway. **A visible loss instead of a false claim** is the trade ADR-031 already makes for
tagging, for the same reason: a reader told a document conforms stops checking.

## Why at the operation and not at the commit doors

The first plan put the drop in `commitMutation` and `commitBarrier`. A trace of every byte-changing path
refuted it: `handleSave` writes client bytes into the document with no door; `commitMutation` is also the
reload door, which is not an edit; both doors accept a still-signed result, so a second write there can land
after a signature; convene's result is byte-bound to its mirror and every hop prefix; the CLI has four output
paths. At the operation the drop rides a rewrite that was happening anyway, so none of those constraints can
be crossed by it.

## The declared gap: a signed document keeps its claim

Dropping the identification is a full rewrite, and a full rewrite destroys a signature. So
`DropUAIdentificationUnlessSigned` returns a document that already carries a signature blob unchanged, and a
labelled, signed document saved after a pdf.js edit, or signed again, keeps its identification. nib does not
rewrite evidence to tidy metadata. Asserted by `TestASignedDocumentIsNeverRewrittenToDropAClaim`.

A document pdfcpu cannot read is passed through too, logged: a metadata step never costs the user a save or a
signature.

## Cost, measured

The tail `Append`, `Combine` and `NUp` pay is one parse: 43 ms against `Rotate`'s 55 ms on a 190 KB document,
plus a rewrite only when there is a claim. The converted operations pay nothing extra. `handleSave` and Save As
pay one parse per save of changed bytes; Save As first compares lengths against at most eight open documents.
Nothing above 190 KB was measured for the tail.

## Enforcement

- `TestNoOperationCarriesAnIdentificationItDidNotVerify` drives the tag-fate census's whole population on a
  labelled fixture; an operation added with a census row is asked at once.
- `TestDropUAIdentificationOnVeraPDFsOwnLabelledFiles` removes the claim from third-party packets in both
  encodings. The claim is read by namespace, never by string: a PDF/A extension-schema block names the
  namespace as text and claims nothing.
- `TestSavingKeepsAClaimOnlyOverBytesItDidNotChange`, `TestSaveAsKeepsAClaimOnlyForAnOpenDocumentsOwnBytes`,
  `TestFinalizingDropsTheClaimBeforeTheSignature`, `TestSignDropsTheClaimBeforeTheSignature` — the doors.

## What this ADR does NOT say

It does not let nib write the identification. That is `/pending 486`, which this was built to unblock.
