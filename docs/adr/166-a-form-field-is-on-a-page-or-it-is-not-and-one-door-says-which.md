# ADR-166 — a form field is on a page or it is not, and one door says which

**Status:** accepted
**Date:** 2026-10-13
**Extends:** ADR-009 (a rule gets one door), ADR-072 (a refusal about a document is a 422 naming its cause).
**Context:** `/pending 271` and `/pending 888`. A form is two things in a PDF — a tree in the catalog
(`/AcroForm /Fields`, `/Kids` beneath) and widgets on pages — and they can disagree. pdfcpu, which does every
export and fill outside the window, finds fields from the pages' widgets; pdf.js, which fills in the window, lists
the tree. `build/formrt.sh`, run over 193 real documents, read one as the other and reported 5,177 values "lost"
that nobody could have typed: the documents were single pages cut from a larger form by other software, whose
trees still listed the other pages' fields. Counted properly, on the machine it ran on: 8,973 fields on no page in
26 documents, 1,838 of them still holding an entry. The same run found three smaller things that turn on the same
question — a document whose only field is a signature answered 500 "could not export form"; a push button with
more than one widget failed the whole export; and a text field shown in several places read back, in the window,
with the value it had before `nib fill`.
**Applies:** `internal/pdfops/formfields.go` (the door and what stands on it), `internal/pdfops/fill.go`,
`internal/pdfops/scan.go`, `internal/server/scan.go` and `export.go`, `web/app.js` (`#scanFieldsBtn`),
`test/formrt/` (the instrument's own reading).

## Decision

**1. `formFields` is the one answer to "which fields does this form have, and which are on a page".** A field is
a node of the tree that carries a name and none of whose kids does; it is on a page when some page's `/Annots`
names one of its widgets. Built on `eachFormField`, the one walk of the tree. Its kind, flags and value are its
own or the nearest ancestor's. Nothing else in `pdfops` decides this for itself.

**2. A document with no fillable field on a page says so.** `ErrNoFormFields` from every export and from the
fills that read the form first; the server answers 422 `no-form-fields` (`httpError` recognises the sentence) and
the CLI prints it. A fill whose DATA named no field keeps pdfcpu's own answer: that is a fact about the data.

**3. Fields on no page are reported, and removed only when asked.** The hidden-content scan adds one finding with
counts only — never a name or a value — medium where any holds an entry, low where none does. It says "on no
page", never "from removed pages": a form may keep a field with no box on purpose, for a calculation another
reader runs, and nothing in the file says which this is. `RemoveOffPageFields` is the fourth sanitize method; the
window asks first and says what may be lost; it takes the fields, any container left empty and their entries in
the calculation order, and checks its own result by a second scan. **Never on a signed document** (422): it is
housekeeping, and nobody should lose a signature to it.

**4. A push button is set aside for pdfcpu's walk.** pdfcpu has no such kind and reads one with several widgets as
a radio group, asking each widget's appearance for a dictionary of states. `pushButtonsAside` takes its widgets
out of the pages' `/Annots` for the length of the export or fill and puts them back before a fill is written. A
push button is therefore not in an export.

**5. A fill leaves a repeated field's widgets holding no value of their own.** pdfcpu sets the field's value and
redraws each widget; a copy of the old value some producers write on each widget is what a nearest-`/V` reader
(pdf.js) then shows. Where a fill changed such a field, the widgets' copies are removed; a repeated field the fill
did not change is left exactly as written.

## Consequences

The instrument fills only fields on a page and counts the rest. On the 193 documents, installed v1.214.39 against
this build: see `instruments/form-roundtrip.md` in the project memory for the numbers of each run.

**Declared.** Whether a filled value is DRAWN correctly in a reader other than pdf.js is observed by nothing here.
Radio groups, date fields, list and combo boxes are not judged by the instrument. One real document fails export
with pdfcpu's "wrong type StreamDict" and has no push button — not traced. Four are refused by pdfcpu's validator
for an annotation with no `/Subtype`, which refuses every operation on them and is not a form matter. A deep
form tree the walk cannot follow past 50 levels is read as far as that. A tagged document's structure
tree is carried through the removal untouched; an element of it that points at a removed field's widget keeps
that widget in the file, out of the form. The command line reports the finding
(`nib scan`) and has no door for the removal: `nib sanitize` is one fixed pair of removals with no method to name.
