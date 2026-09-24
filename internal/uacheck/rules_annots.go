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
