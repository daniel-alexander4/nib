package uacheck

import (
	"fmt"
)

// The annotation rules — `PLAN-ua-coverage.md` P05.S01. Every verdict below was measured on veraPDF
// 1.30.2 before the rule was written; `annots.go` holds the population and the shared exemption.

func init() {
	register(Rule{
		Clause:  "7.18.1 t1",
		Summary: "an annotation that is not a Widget, PrinterMark or Link shall be nested within an Annot tag",
		Check:   checkAnnotationsAreNestedInAnnotTags,
	})
	register(Rule{
		Clause:  "7.18.1 t2",
		Summary: "an annotation shall have a Contents entry or an Alt on the structure element enclosing it",
		Check:   checkAnnotationsCarryADescription,
	})
}

// annotTagExcluded is 7.18.1 t1's own exclusion list, and it is a list of SUBTYPES rather than of
// model classes.
//
// Widget, PrinterMark and Link are excluded because each has a rule of its own — 7.18.4 t1, 7.18.8 t1
// and 7.18.5 t1. Nothing else is: measured on 1.30.2, a `/Popup` is graded by this clause (it fails
// without an Annot tag, `t1-popup-not-excluded`) and so is an annotation with NO `/Subtype` at all
// (`p-no-subtype`), because a null subtype equals none of the three names.
func annotTagExcluded(subtype string) bool {
	switch subtype {
	case "Widget", "PrinterMark", "Link":
		return true
	}
	return false
}

// checkAnnotationsAreNestedInAnnotTags evaluates ua1 7.18.1 t1.
//
// The tag is the STANDARD type of the element the annotation's `/StructParent` names, so a private
// type role-mapped to `/Annot` satisfies it — measured (`t1-rolemapped-to-annot` passes), which is
// why this reads `standardType` and not the element's raw `/S`.
func checkAnnotationsAreNestedInAnnotTags(d *Document) Result {
	subjects, missed := d.annots()
	for _, a := range subjects {
		// An excluded subtype and an exempt annotation are PASSING checks, not absent subjects: the
		// profile puts both in the test expression, and veraPDF's subject is every annotation. Measured
		// — a document whose only annotation is a hidden widget reports 7.18.1 t1 PASSED there, so a
		// `NotApplicable` here would be the "veraPDF passed, nib not applicable" gap P04.S04 named, and
		// would silently drop the clause's corpus reach.
		if annotTagExcluded(a.subtype(d)) || d.annotExempt(a) {
			continue
		}
		elem, sp, has, unread := d.annotElement(a)
		if unread != "" {
			return Result{Verdict: CannotCheck, Why: unread, Where: a.where}
		}
		if !has {
			return Result{
				Verdict: Fail,
				Why:     "the annotation carries no /StructParent, so no structure element encloses it",
				Where:   a.where,
			}
		}
		if elem == nil {
			return Result{
				Verdict: Fail,
				Why:     fmt.Sprintf("the annotation's /StructParent %d names no element in the parent tree", sp),
				Where:   a.where,
			}
		}
		ty, unresolved := d.standardType(elem)
		if unresolved != "" {
			// The element may well be an Annot; nib could not follow the role map to find out.
			return Result{Verdict: CannotCheck, Why: unresolved, Where: a.where}
		}
		if ty != "Annot" {
			return Result{
				Verdict: Fail,
				Why:     fmt.Sprintf("the annotation is nested in a %q element, not Annot", ty),
				Where:   a.where,
			}
		}
	}
	if missed != "" {
		return Result{Verdict: CannotCheck, Why: missed}
	}
	if len(subjects) == 0 {
		return Result{Verdict: NotApplicable, Why: "the document has no annotations"}
	}
	return Result{Verdict: Pass, Why: fmt.Sprintf("all %d annotations are nested in Annot elements, excluded by subtype, hidden or off the crop box", len(subjects))}
}

// checkAnnotationsCarryADescription evaluates ua1 7.18.1 t2.
//
// **The `Alt` is the ENCLOSING ELEMENT's, never the annotation's own key** (`GFPDAnnot.getAlt`), and
// both it and `/Contents` must be non-empty strings. Measured, each its own fixture: an `/Alt` on the
// annotation dictionary fails, an empty `/Contents` fails, an empty element `/Alt` fails, and an
// indirect string on either passes.
func checkAnnotationsCarryADescription(d *Document) Result {
	subjects, missed := d.annots()
	for _, a := range subjects {
		// Widgets alone are excluded here — a Link and a PrinterMark are graded, unlike in t1 — and an
		// exclusion is a passing check, for the reason `checkAnnotationsAreNestedInAnnotTags` states.
		if a.subtype(d) == "Widget" || d.annotExempt(a) {
			continue
		}
		if s, ok := d.text(a.dict["Contents"]); ok && s != "" {
			continue
		}
		elem, _, _, unread := d.annotElement(a)
		if unread != "" {
			return Result{Verdict: CannotCheck, Why: unread, Where: a.where}
		}
		if elem != nil {
			if alt, ok := d.text(elem["Alt"]); ok && alt != "" {
				continue
			}
		}
		return Result{
			Verdict: Fail,
			Why:     "the annotation has no /Contents and the element enclosing it has no /Alt, so nothing describes it",
			Where:   a.where,
		}
	}
	if missed != "" {
		return Result{Verdict: CannotCheck, Why: missed}
	}
	if len(subjects) == 0 {
		return Result{Verdict: NotApplicable, Why: "the document has no annotations"}
	}
	return Result{Verdict: Pass, Why: fmt.Sprintf("all %d annotations carry a description, or are a widget, hidden or off the crop box", len(subjects))}
}

// The typed annotation rules — P05.S02. Same door, same exemption; what differs is the population
// (one subtype each) and, for 7.18.8 t1, that it reads the element's RAW `/S` rather than its standard
// type. Every verdict below was measured on veraPDF 1.30.2 over seventeen fixtures first.

func init() {
	register(Rule{
		Clause:  "7.18.5 t1",
		Summary: "a Link annotation shall be nested within a Link tag",
		Check:   checkLinksAreNestedInLinkTags,
	})
	register(Rule{
		Clause:  "7.18.5 t2",
		Summary: "a Link annotation shall carry an alternate description in its Contents entry",
		Check:   checkLinksCarryTheirOwnContents,
	})
	register(Rule{
		Clause:  "7.18.2 t1",
		Summary: "an annotation of subtype TrapNet shall not be present",
		Check:   checkNoTrapNetAnnotations,
	})
	register(Rule{
		Clause:  "7.18.8 t1",
		Summary: "a PrinterMark annotation shall not be in the structure tree, being an incidental artifact",
		Check:   checkPrinterMarksAreNotInTheTree,
	})
}

// annotsOfSubtype is every annotation of one `/Subtype`, with the door's reason for a short population.
//
// It is the population of a typed clause — veraPDF picks a model subclass per subtype
// (`GFPDAnnot.createAnnot`) and the profile names that subclass as the rule's object, so the subject set
// is "the annotations of this subtype" and nothing else.
func (d *Document) annotsOfSubtype(subtype string) ([]annotSubject, string) {
	subjects, missed := d.annots()
	var out []annotSubject
	for _, a := range subjects {
		if a.subtype(d) == subtype {
			out = append(out, a)
		}
	}
	return out, missed
}

// checkLinksAreNestedInLinkTags evaluates ua1 7.18.5 t1.
//
// The tag is the enclosing element's STANDARD type, as in 7.18.1 t1 — a private type role-mapped to
// `/Link` satisfies it (measured).
func checkLinksAreNestedInLinkTags(d *Document) Result {
	links, missed := d.annotsOfSubtype("Link")
	for _, a := range links {
		if d.annotExempt(a) {
			continue
		}
		elem, sp, has, unread := d.annotElement(a)
		if unread != "" {
			return Result{Verdict: CannotCheck, Why: unread, Where: a.where}
		}
		if !has {
			return Result{
				Verdict: Fail,
				Why:     "the link carries no /StructParent, so no structure element encloses it",
				Where:   a.where,
			}
		}
		if elem == nil {
			return Result{
				Verdict: Fail,
				Why:     fmt.Sprintf("the link's /StructParent %d names no element in the parent tree", sp),
				Where:   a.where,
			}
		}
		ty, unresolved := d.standardType(elem)
		if unresolved != "" {
			// The element may well be a Link; nib could not follow the role map to find out.
			return Result{Verdict: CannotCheck, Why: unresolved, Where: a.where}
		}
		if ty != "Link" {
			return Result{
				Verdict: Fail,
				Why:     fmt.Sprintf("the link is nested in a %q element, not Link", ty),
				Where:   a.where,
			}
		}
	}
	if missed != "" {
		return Result{Verdict: CannotCheck, Why: missed}
	}
	if len(links) == 0 {
		return Result{Verdict: NotApplicable, Why: "the document has no link annotations"}
	}
	return Result{Verdict: Pass, Why: fmt.Sprintf("all %d link annotations are nested in Link elements, hidden or off the crop box", len(links))}
}

// checkLinksCarryTheirOwnContents evaluates ua1 7.18.5 t2.
//
// **This clause reads the ANNOTATION's `/Contents` and nothing else** — where 7.18.1 t2 falls back to the
// enclosing element's `/Alt`, this one does not. Measured: a link with no `/Contents` whose Link element
// carries `/Alt` PASSES 7.18.1 t2 and FAILS this clause, on one document. Two descriptions of the same
// annotation, and only one of them satisfies both rules.
func checkLinksCarryTheirOwnContents(d *Document) Result {
	links, missed := d.annotsOfSubtype("Link")
	for _, a := range links {
		if d.annotExempt(a) {
			continue
		}
		if s, ok := d.text(a.dict["Contents"]); ok && s != "" {
			continue
		}
		return Result{
			Verdict: Fail,
			Why:     "the link has no /Contents, so nothing describes where it goes",
			Where:   a.where,
		}
	}
	if missed != "" {
		return Result{Verdict: CannotCheck, Why: missed}
	}
	if len(links) == 0 {
		return Result{Verdict: NotApplicable, Why: "the document has no link annotations"}
	}
	return Result{Verdict: Pass, Why: fmt.Sprintf("all %d link annotations carry a /Contents description, or are hidden or off the crop box", len(links))}
}

// checkNoTrapNetAnnotations evaluates ua1 7.18.2 t1.
//
// The clause refuses the subtype outright, and its whole test is the shared exemption — so a TrapNet is
// permitted only where it is hidden or off the crop box. Measured: visible fails, hidden passes, off the
// crop box passes.
func checkNoTrapNetAnnotations(d *Document) Result {
	traps, missed := d.annotsOfSubtype("TrapNet")
	for _, a := range traps {
		if d.annotExempt(a) {
			continue
		}
		return Result{
			Verdict: Fail,
			Why:     "the page carries a TrapNet annotation, which PDF/UA does not permit",
			Where:   a.where,
		}
	}
	if missed != "" {
		return Result{Verdict: CannotCheck, Why: missed}
	}
	if len(traps) == 0 {
		return Result{Verdict: NotApplicable, Why: "the document has no TrapNet annotations"}
	}
	return Result{Verdict: Pass, Why: fmt.Sprintf("the document's %d TrapNet annotation(s) are all hidden or off the crop box", len(traps))}
}

// checkPrinterMarksAreNotInTheTree evaluates ua1 7.18.8 t1.
//
// **The clause is "not in the structure tree at all", and its own description reads the other way.** That
// description says a PrinterMark "shall be considered Incidental Artifacts", which invites tagging one as
// `/Artifact`; the profile's test is `structParentType == null`, and measured on 1.30.2 a PrinterMark whose
// `/StructParent` names an `/Artifact` element **FAILS**. An incidental artifact is content no structure
// element describes, not content described as an artifact.
//
// **And it reads the RAW `/S`, not the standard type** (`GFPDAnnot.getstructParentType` takes
// `getNameKeyStringValue(ASAtom.S)`), so a private type nothing maps is still "in the tree", and a role-map
// loop — which makes every clause that resolves a type answer CannotCheck — leaves this one a definite Fail.
// It is the only clause in this family that reads an element's type RAW; three of its siblings read no
// element type at all.
func checkPrinterMarksAreNotInTheTree(d *Document) Result {
	marks, missed := d.annotsOfSubtype("PrinterMark")
	for _, a := range marks {
		if d.annotExempt(a) {
			continue
		}
		elem, _, has, unread := d.annotElement(a)
		if unread != "" {
			return Result{Verdict: CannotCheck, Why: unread, Where: a.where}
		}
		if !has || elem == nil {
			// No `/StructParent`, or a row that is not an element: veraPDF reads null and passes. Measured
			// both, the second with a row holding an array.
			continue
		}
		// **A present but EMPTY `/S` is a type, not an absence** — `d.name` cannot tell the two apart and
		// veraPDF fails the empty one (measured: pdfcpu accepts `/S /`, veraPDF fails 7.18.8 t1, and nib
		// passed it until this read). `nameOf` answers the key's presence separately from its value.
		if raw, isName := d.nameOf(elem["S"]); isName {
			tag := fmt.Sprintf("a %q element", raw)
			if raw == "" {
				tag = "an element whose /S is an empty name"
			}
			return Result{
				Verdict: Fail,
				Why: fmt.Sprintf("the printer's mark is in the structure tree as %s; an incidental "+
					"artifact is described by no element at all, not by one tagged /Artifact", tag),
				Where: a.where,
			}
		}
	}
	if missed != "" {
		return Result{Verdict: CannotCheck, Why: missed}
	}
	if len(marks) == 0 {
		return Result{Verdict: NotApplicable, Why: "the document has no PrinterMark annotations"}
	}
	return Result{Verdict: Pass, Why: fmt.Sprintf("none of the document's %d printer's mark(s) is in the structure tree", len(marks))}
}
