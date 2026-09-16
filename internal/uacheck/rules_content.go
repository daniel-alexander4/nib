package uacheck

import (
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The structure rules — `PLAN-accessibility.md` P07.S03.

func init() {
	register(Rule{
		Clause:  "6.2 t1",
		Summary: "the document catalog shall contain a MarkInfo dictionary with a Marked entry set to true",
		Check:   checkMarkInfo,
	})
	register(Rule{
		Clause:  "7.1 t3",
		Summary: "content shall be marked as Artifact or tagged as real content",
		Check:   checkContentTaggedOrArtifact,
	})
	register(Rule{
		Clause:  "7.18.4 t1",
		Summary: "a Widget annotation shall be nested within a Form tag",
		Check:   checkWidgetsInFormElements,
	})
}

// checkMarkInfo evaluates ua1 6.2 t1.
//
// The rule reads the claim, and the claim alone. Whether the claim is TRUE — whether a document
// saying `/Marked true` has structure behind it — is 7.1 t3's and 7.1 t11's question, and a rule
// that folded them in would report one defect under three clauses.
func checkMarkInfo(d *Document) Result {
	mi := d.dict(d.Catalog["MarkInfo"])
	if mi == nil {
		return Result{
			Verdict: Fail,
			Why:     "the catalog has no /MarkInfo dictionary, so the document never says it is tagged",
			Where:   "catalog",
		}
	}
	if marked, ok := d.boolValue(mi["Marked"]); !ok || !marked {
		return Result{
			Verdict: Fail,
			Why:     "/MarkInfo does not say /Marked true, so a reader is never told to use the structure tree",
			Where:   "catalog /MarkInfo",
		}
	}
	return Result{Verdict: Pass}
}

// checkContentTaggedOrArtifact evaluates ua1 7.1 t3, and reports the OFFENDING content.
//
// P01 spent a slice discovering that veraPDF points at
// `pages[0]/contentStream[0]/content[2]{mcid:0}` — the location is what made that finding possible,
// and a checker saying only "7.1 t3 fails" reproduces the problem it exists to solve. So the result
// names the first uncovered operator by page, stream and position, and says how many more there are.
func checkContentTaggedOrArtifact(d *Document) Result {
	events, errWhy := d.contentEvents()
	if errWhy != "" {
		return Result{Verdict: CannotCheck, Why: errWhy}
	}
	var first *contentEvent
	uncovered, pageEvents := 0, 0
	for i := range events {
		// Appearance streams are not content for this clause — veraPDF located no 7.1 t3 failure
		// inside a widget's appearance on any form measured, while it did locate fonts there.
		if events[i].appearance {
			continue
		}
		pageEvents++
		if !events[i].covered {
			uncovered++
			if first == nil {
				first = &events[i]
			}
		}
	}
	if pageEvents == 0 {
		return Result{
			Verdict: NotApplicable,
			Why:     "no page draws anything, so there is no content to be tagged or artifacted",
		}
	}
	if first == nil {
		return Result{Verdict: Pass}
	}
	why := "content is neither inside an /Artifact sequence nor inside a marked-content sequence with an MCID"
	if uncovered > 1 {
		why = fmt.Sprintf("%s — %d drawing operator(s) of %d, the first located here", why, uncovered, pageEvents)
	}
	return Result{Verdict: Fail, Why: why, Where: first.where}
}

// checkWidgetsInFormElements evaluates ua1 7.18.4 t1 — in the direction veraPDF reads it.
//
// A widget is nested in a Form tag when its `/StructParent` resolves through the parent tree to an
// element whose standard type is `Form` (through the role map).
//
// # Why this rule does NOT require the element's OBJR back-link
//
// P06.S07 and P07.S03 built "both directions" into this rule — the annotation pointing at the
// element AND the element's `OBJR` naming the annotation back — and law 5's guard found nib
// stricter than the clause. Measured, one link at a time: removing the `OBJR` PASSES 7.18.4 t1;
// removing `/StructParent` FAILS it; retyping the element `Div` FAILS it. The clause is read from the
// annotation up. The `OBJR` is still what lets a tree-walking reader reach the widget, and
// `pdfops.AuthorTaggedForm` still writes it — but no ua1 clause nib checks fails without it, and a
// checker reporting a failure the oracle does not is wrong in exactly the way law 5 exists to catch.
func checkWidgetsInFormElements(d *Document) Result {
	widgets := 0
	for p := 1; p <= d.Ctx.PageCount; p++ {
		page, _, _, err := d.Ctx.PageDict(p, false)
		if err != nil || page == nil {
			return Result{Verdict: CannotCheck, Why: fmt.Sprintf("page %d does not resolve", p)}
		}
		annots, _ := d.Ctx.DereferenceArray(page["Annots"])
		for i, a := range annots {
			ad := d.dict(a)
			if ad == nil {
				continue
			}
			if d.name(ad["Subtype"]) != "Widget" {
				continue
			}
			widgets++
			where := fmt.Sprintf("page %d, annotation %d", p, i)
			ir, isRef := a.(types.IndirectRef)
			if !isRef {
				return Result{
					Verdict: Fail,
					Why:     "a widget annotation is written inline in /Annots, so it has no object number an OBJR could name",
					Where:   where,
				}
			}
			where = fmt.Sprintf("page %d, widget annotation (object %d)", p, ir.ObjectNumber.Value())
			sp, hasSP := d.intValue(ad["StructParent"])
			if !hasSP {
				return Result{
					Verdict: Fail,
					Why:     "the widget carries no /StructParent, so nothing in the structure tree describes it",
					Where:   where,
				}
			}
			pt, unread := d.parentTree()
			entry, found := pt[sp]
			if !found && unread != "" {
				return Result{Verdict: CannotCheck, Why: unread, Where: where}
			}
			elem := d.dict(entry)
			if elem == nil {
				return Result{
					Verdict: Fail,
					Why:     fmt.Sprintf("the widget's /StructParent %d names no element in the parent tree", sp),
					Where:   where,
				}
			}
			if ty := d.standardType(elem); ty != "Form" {
				return Result{
					Verdict: Fail,
					Why:     fmt.Sprintf("the widget is nested in a %q element, not Form", ty),
					Where:   where,
				}
			}
		}
	}
	if widgets == 0 {
		return Result{Verdict: NotApplicable, Why: "the document has no widget annotations"}
	}
	return Result{Verdict: Pass}
}
