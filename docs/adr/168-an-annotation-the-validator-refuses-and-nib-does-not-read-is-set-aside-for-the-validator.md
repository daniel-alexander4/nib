# ADR-168 — an annotation entry the validator refuses and nib does not read is set aside for the validator

**Status:** accepted
**Date:** 2026-10-13
**Extends:** ADR-115 and ADR-116 (the checker's set-asides), ADR-129 and ADR-164 (what the read door settles
before pdfcpu's validator), ADR-166 (a field can be on no page).
**Context:** `/pending 888`. `build/formrt.sh` over 203 real documents left six that "pdfcpu could not export".
Traced on the documents: for five of them it was not the form at all. pdfcpu's validator refuses the WHOLE
document, so every operation that reads through `pdfread` — rotate, optimize, export, fill, sanitize — answered
with the validator's sentence, on documents that open and draw in the window and in every viewer:

- **four** (one real-estate forms engine wrote all of them) hold, in a page's `/Annots`, stamp-like annotations
  of exactly `/AP /F /NM /P /Rect` — no `/Subtype`: "dict=annotDict required entry=Subtype missing"
  (`validate/annotation.go:1516`, pdfcpu v0.13.0). 13, 32, 2 and 2 of them.
- **one** has a widget whose `/P` names a 19-byte stream: "dereferenceDict: wrong type types.StreamDict"
  (`validateEntryP`, `validate/annotation.go:1369`).

The sixth is an encrypted file pdfcpu cannot decrypt, and is not this.
**Applies:** `internal/pdfread/annotaside.go`, `internal/pdfread/pdfread.go` (`validated`).

## Decision

**1. `setAsideUnreadableAnnotationParts` runs in `validated`, between the read and the validation, for every
reader** — beside `escapeInfoKeys` and `setAsideOversizedPacket`, and by their rule: taken out of the
validator's sight, put back as parsed, so every reader afterwards — nib's, pdfcpu's writer, the content digest
— sees the document as it was written.

**2. An entry of a page's `/Annots` that is a dictionary with no `/Subtype`, no `/FT` and no `/Parent` is left
out of the array the validator walks.** It goes back in front of the first entry that followed it in the
document's own array and is still there, or at the end: the validator replaces the array with a cleaned one of
its own (`validateAnnotationsArray`), so "where it stood" is read against what survived.

**3. A field's widget is not set aside.** The validator gives one its `/Subtype /Widget` itself when it reaches
it through the form (`validate/form.go:427`), and it records the widget among its page's annotations, which
pdfcpu's own operations read afterwards.

**4. A `/P` that is not a reference to a dictionary whose `/Type` is `/Page` is taken off its annotation or
field and put back.** Both walks do it: the pages' `/Annots`, and the form's tree — a field can be on no page.
`/P` is optional and nothing in nib reads it.

## Refused

- **Dropping the entries**, as the validator already drops an `/Annots` entry it cannot resolve. What a viewer
  shows of these documents is IN the entries (the stamps' appearance streams); a rotation that handed back a
  document without them would have removed content nobody asked to remove.
- **Writing a `/Subtype` into them.** No subtype is true of an annotation that is only an appearance, and the
  validator then holds the entry to that subtype's own required keys.
- **A refusal with better words.** The documents are ordinary to every other reader; "this document cannot be
  changed" is not an answer to "rotate page 1".

## Consequences

- A document that read before reads the same: neither defect can be in a document the validator accepted, so
  only what was refused outright is touched, and `ContentDigestVersion` does not move.
- Measured on the five documents with the built binary: rotate, optimize and the form export each succeed; the
  annotations with no `/Subtype` are in every output, the same number as in the source.
- The walks follow the page tree and the field tree with `ctx.Dereference` alone (ADR-116) to a depth of 64; a
  tree nested deeper is left to the validator.
- **Declared:** pdf.js does not draw an annotation with no `/Subtype`, so the window does not show those stamps.
  What they are in the source documents (initials, a logo) was not looked at; whether other viewers draw them
  was not measured.
- **Declared:** an annotation dictionary reached only through some other road (a structure element's `/Obj`, a
  pop-up's `/Parent`) is not walked. No such document was found.
- Other operations may still meet such an entry and answer their own error for it; none was seen to in the three
  that were run.
