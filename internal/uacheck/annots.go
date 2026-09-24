package uacheck

import (
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The annotation door — `PLAN-ua-coverage.md` P05.S01.
//
// # One enumeration, because there were three
//
// Before this slice the package walked `/Annots` in three places — `checkWidgetsInFormElements`
// (7.18.4 t1), `scanAnnotsAndFields` (7.2 t24/t25) and `walkAppearances` — and P05 adds six more
// subjects over the same array. ADR-009 gives a rule one door: "which entries of a page's `/Annots`
// are annotations" is one rule, so it is stated once, here, and `TestEveryAnnotationReaderRoutes`
// asserts the routing rather than the agreement.
//
// # The population is veraPDF's, measured before it was written
//
// `PDPage.getAnnotations` (veraPDF-parser) keeps every entry of the page's `/Annots` array that
// resolves to a DICTIONARY and drops everything else; the array itself may be indirect. It filters
// no subtype — `GFPDAnnot.createAnnot` switches on `/Subtype` only to pick a subclass, and every
// subclass IS a `PDAnnot`, which is why 7.18.1 t1 excludes Widget, PrinterMark and Link in its own
// test expression rather than in the population. Measured on 1.30.2 (the slice's grill table):
//   - an annotation written INLINE in `/Annots` is a subject (7.18.1 t1 ran one check on it);
//   - a dangling reference is NOT (7.18.3 t1 passed a page whose only entry was one);
//   - an entry resolving to an array is NOT;
//   - a `/Popup` IS, and an annotation with no `/Subtype` at all IS.
//
// # The exemption is shared and is NOT "the annotation is invisible"
//
// Six of P05's ten rules carry `isOutsideCropBox == true || (F & 2) == 2`. Both halves are read from
// the parser's source and measured, because the profile's wording hides two traps:
//
//   - `isOutsideCropBox` is DISJOINTNESS against the inherited `/CropBox` clipped to the `/MediaBox`,
//     falling back to the `/MediaBox` entirely when there is no `/CropBox`, compared with `>=`/`<=`
//     so an annotation merely TOUCHING the box edge counts as outside (`PDAnnotation.java:285-294`,
//     `PDPage.getCropBox():112-119`). Measured: a rectangle overlapping the edge by one unit is not
//     outside, one touching it exactly is.
//   - It answers **null**, not true, when either rectangle is missing or holds fewer than four
//     numbers, and the profile tests `== true` — so an annotation with no `/Rect` is NOT exempt.
//     Measured: `x-no-rect` and `x-short-rect` both FAIL 7.18.1 t1.
//   - The rectangle is used AS WRITTEN. veraPDF does not normalise a reversed `/Rect`, so
//     `[600 600 50 50]` inside a `/CropBox [100 100 500 500]` is "outside" to it and "inside" to any
//     implementation that sorts the corners first. Measured: veraPDF passes that document.
type annotSubject struct {
	page  int
	index int
	dict  types.Dict
	// pageDict is the page the annotation sits on, for readers that need the page's own keys — the
	// appearance walk falls back to the page's `/Resources`.
	pageDict types.Dict
	// crop is the page's crop box as veraPDF computes it: inherited, clipped to the media box, and the
	// media box itself where there is no crop box. nil when the page declares neither.
	crop *types.Rectangle
	// object is the annotation's object number, or 0 when it is written inline in `/Annots`.
	object int
	where  string
}

// subtype is the annotation's `/Subtype` as a name, or "" when it has none.
func (a annotSubject) subtype(d *Document) string { return d.name(a.dict["Subtype"]) }

// annots is every annotation in the document, in page order and then `/Annots` order.
//
// The second result is why the population may be short — a page or an `/Annots` array nib could not
// read. It is a CannotCheck reason for every rule over annotations and never an empty population: a
// page that does not resolve is not a page with no annotations (`/pending 507`).
func (d *Document) annots() ([]annotSubject, string) {
	if d.annotsDone {
		return d.annotList, d.annotsErr
	}
	d.annotsDone = true
	for p := 1; p <= d.Ctx.PageCount; p++ {
		page, _, inh, err := d.Ctx.PageDict(p, false)
		if err != nil || page == nil {
			d.annotsErr = fmt.Sprintf("page %d does not resolve", p)
			return d.annotList, d.annotsErr
		}
		annots, aerr := d.Ctx.DereferenceArray(page["Annots"])
		if aerr != nil {
			// An /Annots nib cannot read is not a page with no annotations. A dangling reference is NOT
			// this: it dereferences to null with no error, and veraPDF drops that entry too.
			d.annotsErr = fmt.Sprintf("page %d's /Annots could not be read: %v", p, aerr)
			return d.annotList, d.annotsErr
		}
		crop := cropOf(inh)
		for i, a := range annots {
			ad := d.dict(a)
			if ad == nil {
				// veraPDF drops an entry that is not a dictionary, and so does this — but **nothing reaches
				// this branch through nib's own reader**, measured on pdfcpu v0.13.0: a dangling reference and
				// a `null` literal are REMOVED from the array by `ReadValidateAndOptimize` (and the `/Annots`
				// key with them when the array empties), and an entry resolving to an array makes the
				// validator refuse the file before any rule runs. It is a declared red-proof survivor rather
				// than a live rule: the fixtures named for it measure nib's answer, not the drop.
				continue
			}
			s := annotSubject{page: p, index: i, dict: ad, pageDict: page, crop: crop,
				where: fmt.Sprintf("page %d, annotation %d", p, i)}
			if ir, ok := a.(types.IndirectRef); ok {
				s.object = ir.ObjectNumber.Value()
				s.where = fmt.Sprintf("page %d, annotation %d (object %d)", p, i, s.object)
			}
			d.annotList = append(d.annotList, s)
		}
	}
	return d.annotList, d.annotsErr
}

// cropOf is veraPDF's crop box: the inherited `/CropBox` clipped to the `/MediaBox`, and the
// `/MediaBox` itself where there is no `/CropBox`. pdfcpu resolves the inheritance; the clip is
// ours, because `PDPage.getCropBox` clips and pdfcpu does not.
func cropOf(inh *model.InheritedPageAttrs) *types.Rectangle {
	if inh == nil {
		return nil
	}
	if inh.CropBox == nil {
		return inh.MediaBox
	}
	if inh.MediaBox == nil {
		return inh.CropBox
	}
	return types.NewRectangle(
		max(inh.CropBox.LL.X, inh.MediaBox.LL.X), max(inh.CropBox.LL.Y, inh.MediaBox.LL.Y),
		min(inh.CropBox.UR.X, inh.MediaBox.UR.X), min(inh.CropBox.UR.Y, inh.MediaBox.UR.Y))
}

// annotHidden reports whether the annotation's `/F` carries bit 2, the Hidden flag.
//
// An `/F` nib cannot read as an integer is not hidden — the profile's `(F & 2) == 2` is false for a
// null `F`, and a rule that treated an unreadable flag as the exemption would excuse the annotation
// it understands least.
func (d *Document) annotHidden(a annotSubject) bool {
	f, ok := d.intValue(a.dict["F"])
	return ok && f&2 == 2
}

// annotOutsideCropBox is veraPDF's `isOutsideCropBox`, tri-state: (outside, known).
//
// `known` is false where veraPDF answers null — no crop box, no `/Rect`, or a `/Rect` of fewer than
// four numbers — and null is NOT the exemption, because the profile tests `== true`.
//
// A fourth branch answers unknown for a `/Rect` entry that is not a number, and there nib and veraPDF
// **disagree** — measured, not assumed: veraPDF COERCES the entry to 0 rather than answering null
// (`/Rect [0 0 (x) 10]` inside a `/CropBox [100 100 500 500]` is "outside" to it and exempt; the same
// rectangle with no crop box is graded and fails). nib cannot reach the disagreement: pdfcpu refuses
// such a file before any rule runs ("dereferenceNumber: wrong type"), which is a reading limit of
// nib's recorded in the slice's inventory, not a rule's verdict.
func (d *Document) annotOutsideCropBox(a annotSubject) (outside, known bool) {
	if a.crop == nil {
		return false, false
	}
	rect, err := d.Ctx.DereferenceArray(a.dict["Rect"])
	if err != nil || len(rect) < 4 {
		return false, false
	}
	var r [4]float64
	for i := 0; i < 4; i++ {
		n, nerr := d.Ctx.DereferenceNumber(rect[i])
		if nerr != nil {
			return false, false
		}
		r[i] = n
	}
	// veraPDF's own comparison, corners unsorted and edges inclusive.
	return a.crop.LL.Y >= r[3] || a.crop.LL.X >= r[2] || a.crop.UR.Y <= r[1] || a.crop.UR.X <= r[0], true
}

// annotExempt is the clause six of P05's rules share: `isOutsideCropBox == true || (F & 2) == 2`.
//
// It is ONE door (ADR-009) and it reaches the rules P05 writes AND `7.18.4 t1`, which shipped in
// P03.S06 without it. Measured at this slice's grill on veraPDF 1.30.2: a hidden widget with no
// `/StructParent`, and a widget wholly outside the crop box, are both PASSES there and were both
// FAILS in nib — two live false fails in a shipped clause, invisible to the corpus because no corpus
// file holds a hidden or off-page widget outside a Form tag.
func (d *Document) annotExempt(a annotSubject) bool {
	if d.annotHidden(a) {
		return true
	}
	outside, known := d.annotOutsideCropBox(a)
	return known && outside
}

// annotElement is the structure element an annotation's `/StructParent` names, through the parent
// tree — an ASSOCIATION and never an ancestor climb, the same hop `annotAndFieldSubjects` documents.
//
// It answers three different things and a rule must tell them apart: `has` false means the
// annotation declares no `/StructParent` at all (definite), a nil `elem` with `has` true means the
// key resolves to nothing in the parent tree (definite), and a non-empty `unread` means nib could
// not finish reading the tree, so for THIS annotation "names no element" and "never read" are
// indistinguishable (`/pending 496`).
func (d *Document) annotElement(a annotSubject) (elem types.Dict, sp int, has bool, unread string) {
	sp, has = d.intValue(a.dict["StructParent"])
	if !has {
		return nil, 0, false, ""
	}
	pt, why := d.parentTree()
	// **Present-but-not-an-element is DEFINITE; absent from a tree nib did not finish is not.** The rule
	// this door absorbed drew that line with a `found` check and the first draft of the door lost it,
	// which turned a shipped clause's definite Fail into a refusal on a document with a deep tree AND a
	// malformed row. The same collapse P04 found in `elementForMCID`, in the other direction.
	entry, found := pt[sp]
	if !found && why != "" {
		return nil, sp, true, why
	}
	return d.dict(entry), sp, true, ""
}
