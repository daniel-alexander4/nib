package pdfops

import (
	"testing"
)

// TestAnNUpDoesNotLeaveAFormClaimingFieldsNothingDraws — /pending 573.
//
// `api.NUp` composes sheets as new page objects and never looks at `/Annots`, so pdfcpu — which
// writes by reachability — drops every widget with the source page dictionaries. ADR-045 declares
// that and `carryNoteAnnots` COUNTS it (`left`), so the document does not lie about its annotations.
// The `/AcroForm` is the half nobody counted: the field dictionaries are reachable from the CATALOG
// rather than from a page, so they survive — and a filled form n-upped came out claiming fields with
// nothing on any page to draw or edit them.
//
// # Why the remedy is the prune and not the carry
//
// ADR-045's two grounds for excluding `/Widget` from the note carry both stand and neither is
// cosmetic: §12.5.5 maps an `/AP` bounding box onto `/Rect` axis-aligned, so a widget would be drawn
// upright inside a box the placement rotates; and a signature widget reaching a `/V` blob would come
// back onto a document whose byte ranges the composition has destroyed, past four gates that key on
// an edited document having no signature (`pageselect.go`'s `dropSignature` enumerates them).
//
// **The rule already exists at the sibling door.** `pageselect.go:792` `pruneAcroForm` keeps the
// fields whose widgets survived and DELETES `/AcroForm` when none did — a page selection has always
// answered this. `NUp` is the same question with the answer "none survived", and it was not asking.
// That is ADR-009 in the small: one rule, two doors, one of them not calling it.
func TestAnNUpDoesNotLeaveAFormClaimingFieldsNothingDraws(t *testing.T) {
	src := formOnPagesFixture(t)

	// SETUP: the fixture really is a form with widgets on pages, or "no widgets afterwards" is true
	// of a document that never had any.
	srcFields, srcWidgets := formShape(t, src)
	if srcFields != 2 || srcWidgets != 2 {
		t.Fatalf("setup: the source has %d /AcroForm field(s) and %d widget(s) on its pages, want "+
			"2 and 2", srcFields, srcWidgets)
	}

	out, err := NUp(src, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	fields, widgets := formShape(t, out)

	// The premise, asserted rather than assumed — this is `/pending 573`'s own first step, and the
	// entry called it "what the count implies but is not the same as having seen it".
	if widgets != 0 {
		t.Fatalf("the n-up kept %d widget annotation(s); this test is about the case where it "+
			"keeps none, and the shape it is asserting has changed", widgets)
	}
	if fields != 0 {
		t.Errorf("after an n-up the catalog still claims %d /AcroForm field(s) and no page carries "+
			"a widget for any of them. A reader offers a form that cannot be filled, and the values "+
			"of one already filled sit in dictionaries nothing draws — `pruneAcroForm` has answered "+
			"this at the page-selection door since /pending 524", fields)
	}
}

// formShape returns how many fields the catalog's `/AcroForm` claims and how many `/Widget`
// annotations the pages actually carry.
//
// **Both numbers, because either alone is satisfied by the wrong document.** Counting only widgets
// cannot see the orphaned claim; counting only fields cannot tell a pruned form from a carried one.
func formShape(t *testing.T, pdf []byte) (fields, widgets int) {
	t.Helper()
	ctx := readCtx(t, pdf)
	xt := ctx.XRefTable
	root, err := xt.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if form := derefDict(xt, root["AcroForm"]); form != nil {
		fields = len(derefArray(xt, form["Fields"]))
	}
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, perr := ctx.PageDict(p, false)
		if perr != nil || d == nil {
			continue
		}
		for _, a := range derefArray(xt, d["Annots"]) {
			ad := derefDict(xt, a)
			if ad == nil {
				continue
			}
			if st := ad.NameEntry("Subtype"); st != nil && *st == "Widget" {
				widgets++
			}
		}
	}
	return fields, widgets
}

// TestTheFormPruneLeavesAFormWhoseWidgetsSurvive is the control, and where it had to be written is
// itself the finding.
//
// **The first attempt at this control was vacuous and a mutation proved it.** It drove `Collect` and
// asserted the form came through intact — which it does, through `pruneAcroForm`, the OTHER door.
// Two mutations that gutted `pruneOrphanedAcroForm` (delete every `/AcroForm`; treat every field as
// orphaned) both left it GREEN, because the new predicate was never on that path. A control has to
// exercise the thing it is controlling for, and `NUp` is currently its only caller — an operation
// that destroys every widget by construction, so no document reaching it can take the
// leave-it-alone branch.
//
// So the predicate is driven directly, on a context whose widgets are all still on their pages.
// That is the shape the branch has: "every field still has its widget" is a property of a document,
// not of an operation.
func TestTheFormPruneLeavesAFormWhoseWidgetsSurvive(t *testing.T) {
	src := formOnPagesFixture(t)
	ctx := readCtx(t, src)

	// SETUP: the widgets really are reachable from the pages in THIS context, or "nothing was
	// pruned" is true of a document with nothing to prune.
	if fields, widgets := formShape(t, src); fields != 2 || widgets != 2 {
		t.Fatalf("setup: the fixture has %d field(s) and %d widget(s), want 2 and 2", fields, widgets)
	}

	changed, err := pruneOrphanedAcroForm(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("the prune reported a change on a document every one of whose fields still has its " +
			"widget on a page — a prune that fires there is a prune that deletes working forms, and " +
			"it would do it on every n-up of a document that kept its widgets")
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	form := derefDict(ctx.XRefTable, root["AcroForm"])
	if form == nil {
		t.Fatal("the prune removed the /AcroForm of a document whose widgets are all still on pages")
	}
	if n := len(derefArray(ctx.XRefTable, form["Fields"])); n != 2 {
		t.Errorf("the prune left %d field(s), want the 2 it was given", n)
	}
}

// TestASplitPageDoesNotLeaveAFormClaimingFieldsNothingDraws — the second composing door.
//
// `SplitPage` replaces one page with a grid of tiles and reassembles through `api.MergeRaw`, which
// keeps the FIRST segment's catalog. Asked rather than assumed: /pending 573 named only `NUp`, and
// whether this door has the same orphan is a fact about pdfcpu's tiling, not something to infer
// from the shape of the code.
func TestASplitPageDoesNotLeaveAFormClaimingFieldsNothingDraws(t *testing.T) {
	src := formOnPagesFixture(t)
	if _, widgets := formShape(t, src); widgets != 2 {
		t.Fatalf("setup: the fixture carries %d widget(s), want 2", widgets)
	}
	out, err := SplitPage(src, 2, 2, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	fields, widgets := formShape(t, out)
	t.Logf("after SplitPage(page 2, 2x1): %d /AcroForm field(s), %d widget(s) on pages", fields, widgets)
	if fields > widgets {
		t.Errorf("the split left %d field(s) against %d widget(s) — the same orphaned form the "+
			"n-up produced, at the other composing door", fields, widgets)
	}
}
