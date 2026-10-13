# ADR-145 — an XFA form is dropped by every write, announced, and a placeholder for one is not written

**Status:** accepted
**Date:** 2026-10-12
**Extends:** ADR-129 (one door writes), ADR-136 (the write door refuses by name; a mark kept from the read),
ADR-032 (a correction goes inside the write the change already makes; a signed document is the declared gap),
ADR-072 (a refusal about a document is a 422 naming its cause), ADR-080 (which named `/XFA` as covered by no
digest rule), ADR-009 (one door, and the guard checks the door).
**Context:** /pending 866 and 843, decided by Dan 2026-10-12: nib does not keep XFA. A PDF may carry its form
twice — as fields on its pages, and as an XFA packet in `/AcroForm /XFA` that some readers draw INSTEAD of the
pages. nib shows and fills only the pages. Three things followed, none of them asked for and none announced:

- pdfcpu's validator deletes the whole `/AcroForm` of a form with no fields before nib's code has the context,
  and every write then left `/NeedsRendering true` on the catalog naming a form that was gone. Measured again
  for this ADR on veraPDF's `7.15-t01-fail-a.pdf` and on hand-built fixtures: after a validated read, no
  `/AcroForm`, `/NeedsRendering` still there (`TestTheValidatorDeletesAnXFAOnlyFormAndTheReadStillKnowsOfIt`).
- A form holding both kept its packet through every change (`irs-fw9.pdf`, `irs-f1040.pdf`), so an XFA-aware
  reader went on drawing what the packet said while nib changed the pages.
- A ceremony's fingerprint covers nothing of `/XFA`, so such a reader could be shown something other than what
  the parties signed while every check passed (ADR-080's declared gap).

**Applies:** `internal/pdfread` (`xfa.go`, `Write`, `validated`, the merge), `internal/pdfops` (the movers,
`RedactPages`, `ConvertPDFAGhostscript`, `dropUAIdentificationBytes`, `FlagsAndXFA`), `internal/server`
(`docResponse.XFA`, `httpError`, save, Save As, finalize, convene), `nib sign`, the window's `#xfaNotice`.

## Decision

1. **Read before validation.** `validated` reads what the catalog says about XFA before `api.ValidateContext`
   and keeps it against the context (`XFAOf`): `dynamic` for `/NeedsRendering true`, `static` for an
   `/AcroForm` holding `/XFA`. `ReadXFA` is the same reading over bytes, by one unvalidated parse.
2. **Every write drops the two keys together.** `pdfread.Write` calls `dropXFA` — `/XFA` off the form,
   `/NeedsRendering` off the catalog — and nothing else calls it. `Write` is already the one door to pdfcpu's
   writer, so no operation can write a document that keeps either.
3. **A placeholder is not written.** `/NeedsRendering true` says the pages in the file are not the document
   (one page reading "Please wait…"); dropping the form from that leaves only the placeholder. `Write` refuses
   it with `ErrDynamicXFA`, on the mark from the read OR the catalog as it stands — a page selection rebuilds
   the catalog from an allow-list and would otherwise make a placeholder writable. The trigger is that key, not
   "XFA-only": a static XFA form's pages are the document. The server answers 422, cause `dynamic-xfa`.
4. **A door that does not write the context it read asks first.** The mark does not follow pages into another
   context, so each function that moves them (both merges, the cut, the n-up) calls `RefuseDynamicXFA` beside
   `PutBackValidatorLosses`; so do `RedactPages` (assembled from images and runs) and `ConvertPDFAGhostscript`
   (Ghostscript reads the file itself).
5. **Bytes changed outside `pdfops`** — a posted save, Save As of edited bytes, finalize, `nib sign` — go through
   `DropUAIdentificationUnlessSigned`, which now rewrites an unsigned document carrying an XFA form and returns
   `ErrDynamicXFA` for a placeholder: the one error from that door its four callers treat as a refusal.
6. **Announced.** `docResponse.XFA` is `static`, `dynamic`, or `removed` (it arrived with a static form and a
   change took it out — sticky; an undo brings `static` back). It comes from the read the server already makes
   for the document's flags (`pdfops.FlagsAndXFA`), cached with them. The window words it in a third banner
   beside the tagging and loss notices — a page notice, never a dialog.
7. **No ceremony over a document that still carries one.** `handleCeremonyConvene` refuses, before
   `ceremony.Convene`, with 422 and cause `xfa` (409 on that route means "a ceremony is running"), static or
   dynamic. For a static form the sentence names what removes it (*Remove active content*). No digest rule 6
   and no `ContentDigestVersion` change.

## Refused

- **Keeping XFA through writes** (866's designs A and B): a put-back cannot tell "the validator took the form"
  from "the operation took it", so it would hand *Remove active content* back the scripts it removed, and
  pdfcpu's merge would graft one document's XFA onto another.
- **Covering `/XFA` in the digest**: a rule bump that reads as tampering across a point release, to protect a
  form nib cannot show.
- **Refusing every XFA-only document.** A static XFA-only form has real pages; refusing it would lock a user
  out of a document nib shows correctly.

## Consequences

- A hybrid form (Designer's IRS forms) loses its XFA packet on the first change and the user is told at the
  open and after. Readers that preferred the packet show the pages from then on.
- A dynamic XFA document can be opened and read and nothing else: no operation, save of edits, signature,
  export through a rewrite, or ceremony.
- **Declared, as ADR-032 declares a signed document's claim:** a signed document keeps its XFA form through the
  outside-`pdfops` door (a rewrite would destroy the signature), so co-signing a signed XFA document leaves the
  packet; a signed dynamic document can be co-signed. A ceremony is still not convened over either.
- Declared: `ReadXFA` is one more parse at convene and before a Ghostscript conversion — measured once each on
  four real files, 1.1 / 4.1 / 13.1 / 7.9 ms at 80 / 137 / 215 / 371 KB, against validated reads of
  1.6 / 26 / 65 / 34 ms. The read's own cost is two catalog lookups and was not measured separately.
- Declared: the packet's own `dynamicRender` setting is not read here (`uacheck` reads it for clause 7.15); a
  packet that asks for dynamic rendering in a file without `/NeedsRendering true` is treated as static.
- Handlers that do not pass the error text through `httpError` keep their own status, as under ADR-136.
- Guards: `TestEveryWriteRoutesThroughTheDoor` rules 3 and 4 (the drop has one caller, `Write` refuses then
  drops then writes, every mover asks); `TestNoOperationWritesAnXFAFormOrChangesAPlaceholder` drives every
  operation in the tag-fate census on both kinds — it found `RedactPages` and `ConvertPDFAGhostscript`.
