# ADR-045 — a composition carries the comments of the pages it composes

**Status:** accepted

## Context

`api.NUp` composes its sheets as NEW page objects holding each source page's content inside a Form
XObject, and it never looks at `/Annots`. The source page dictionaries then leave the page tree, so
pdfcpu — which writes by reachability — drops every annotation with them.

**Measured on a three-page document carrying one `AddNotes` sticky note per page: 3 annotations in,
0 out.** A sticky note is the user's own comment, so an n-up was silent loss of authored content on
an operation that warned about nothing. It is `/pending 524`'s class — a page operation destroying
authored content with no sentence anywhere — and it got the same answer that item did: **carry it,
because a warning is the acceptable floor and not the fix.**

**What found it is the shape of the finding.** `/pending 488`'s digest golden recorded
`ContentDigest` of `notes-then-nup` as byte-identical to that of `nup2`, because there was nothing
left of the notes to hash. The equality sat in a committed golden file for a day. Nothing read it
as a data loss, because nothing in the repo was looking at an n-up's annotations at all.

**And it cost more than the notes.** `describeNotes` nests a note of a TAGGED document in an
`/Annot` structure element whose `/ParentTree` key is claimed by the annotation's `/StructParent`.
With the annotation gone the key is claimed by nobody — `structureCarriedCompletely`'s
`unowned-key` — so `completeOrHonest` abandoned the whole carry and `honest` deleted
`/StructTreeRoot`. Measured on `collidingMCIDFixture`: the same document n-ups `carried` without
notes and `dropped` with them. **Adding a comment to a tagged document cost it its tag tree.**

## Decision

**A page composition carries each page's sticky notes onto the sheet its content landed on, with
the rect through the same matrix the content went through, and it carries `/Text` and nothing
else.**

Four parts, each load-bearing.

**1. The matrix is READ, never re-derived.** pdfcpu draws each source page as
`q a b c d e f cm /FmN Do Q` on its sheet, so the placement is in the document. Two further terms
compose with it and neither is optional: the form's `/Matrix`, which pdfcpu writes as a translation
by `−cropBox.LL` (`createNUpFormForPDF`), and a page-rotation prefix, which
`ContentBytesForPageRotation` prepends INSIDE the form for a page carrying `/Rotate`. A point of
page space reaches sheet space through `R × FormMatrix × CM`, in that order. **Both extra terms are
reachable through nib's own operations**: `SplitPage`'s second tile carries `/CropBox [297.5 0 595
842]`, and `Rotate` sets `/Rotate`. Re-deriving pdfcpu's best-fit arithmetic instead would be a
second implementation of a calculation that is already written down in the file.

**2. `/Text` only, and the boundary is the appearance stream.** A sticky note is drawn by the
viewer as a fixed-size icon at its `/Rect`; nib's own notes carry no appearance stream at all. An
annotation WITH one would not survive the placement: §12.5.5 maps `/AP`'s bounding box onto `/Rect`
axis-aligned, and an n-up's best fit turns the page 90°, so a highlight, an ink drawing or a widget
would be drawn upright inside a rotated box — **wrong rather than absent**, which is the trade
`/pending 524`'s own decision refused. Carrying those needs the appearance transformed too, and
that is a different job.

**3. `/Widget` is excluded on a second ground.** A signature widget reaching a `/V` blob would
return to a document whose byte ranges the composition destroyed, and four gates key on an edited
document having no signature at all — `pageselect.go`'s `dropSignature` enumerates them. Measured:
`api.NUp` leaves `sign.Verify` reading `unsigned`, and nothing here changes that.

**4. An `OBJR` follows the annotation it names.** An OBJR is reachable from `/StructTreeRoot` and
pdfcpu writes by reachability, so one left naming the SOURCE annotation keeps that annotation in
the file — on no page, claiming the same `/ParentTree` key as the copy. `parentTreeOwners` walks the
annotations of PAGES, so it cannot see the second claimant and the completeness gate scores both
shapes identically. Measured: `OBJR -> obj 15 onPage=0` beside a fresh copy on sheet 1 the tree
described nothing about.

**It is attempted and abandoned, never half-done.** A placement count that is not the source page
count, a form whose decoded content is not its source page's, or a prefix that is not exactly one
`cm` returns the composed document untouched. A document that gains some of its notes and loses
others silently is worse than one that loses them all, because only the second is a fact a user can
state. Measured: reversing the placement order abandons the carry rather than putting a note on the
wrong page.

## What ADR-013 says about this, and it is not a bar

`ContentDigest` covers each page's `/Annots`, so carrying the notes moves the digest of an n-up. It
is not a `ContentDigestVersion` decision, and the distinction is the whole of ADR-013's concern:
that record refuses a change to what the digest COVERS, because such a change moves every stored
hash and reads as tampering across a point release. This changes a document, not the function.

**And an n-up's digest was never a stored commitment.** `Record.DocHash` is taken at convene time
over the bytes the convener supplies; an n-up is an editing operation upstream of that, and it
already moves the digest completely — `nup2` and `text-3page` were never equal. So no record
written by an earlier build can fail: a record commits to the bytes it was written over, and those
bytes still hash the same.

The golden row moves the other way, and the move is the acceptance: `notes-then-nup` goes from a
pinned hash identical to `nup2`'s to `#unstable`, because the carried notes bring `AddNotes`'s
`time.Now()` `/M` stamp inside the digest with them.

## Cost

Measured on a 40-page document, `NUp(2)`, 12 runs per configuration, minimum of three runs each:
**with no annotations 35.8 ms against a 37.0 ms baseline — the carry's read is inside the noise;
with 40 notes 52.6 ms against 42.4 ms, +10 ms.** The read is `api.ReadAndValidate`, which is the
read `api.NUp` itself performs, so the content bytes this compares come from the same path;
`ReadValidateAndOptimize` costs 15.7 ms against its 9.6 ms on this document and `ReadContext`, at
1.6 ms, cannot be used at all — pdfcpu fills `ctx.PageCount` during validation, so a plain read
reports zero pages.

## The declared gap

**An annotation that is not a `/Text` is still dropped, and still without a sentence.** The page-op
route returns the document and no notice channel, so there is nowhere for one to go that a user
would see; `carryNoteAnnots` counts what it left behind so the residue is a figure rather than a
silence, and the count has no reader yet. The loudest instance is a filled form: `api.NUp` drops
its widgets and keeps `/AcroForm /Fields`, so the values survive in the field dictionaries with no
widget to draw them. That is `/pending`, not an oversight here.
