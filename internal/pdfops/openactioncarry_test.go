package pdfops

import (
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"nib/internal/testpdf"
)

// openActionFixture is a three-page document whose `/OpenAction` is whatever `set` returns, built
// against the page object references of the SOURCE — which is the only thing a real producer has to
// name, and the reason a carry has to be pruned rather than copied.
func openActionFixture(t *testing.T, set func(ctx *model.Context) (types.Object, error)) []byte {
	t.Helper()
	src, err := testpdf.Text("alpha", "beta", "gamma")
	if err != nil {
		t.Fatal(err)
	}
	out, err := writeMutated(src, func(ctx *model.Context) error {
		root, rerr := ctx.XRefTable.Catalog()
		if rerr != nil {
			return rerr
		}
		v, serr := set(ctx)
		if serr != nil {
			return serr
		}
		root["OpenAction"] = v
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// openActionOf returns the output's `/OpenAction`, dereferenced, or nil.
func openActionOf(t *testing.T, pdf []byte) (types.Object, *model.Context) {
	t.Helper()
	ctx := readCtx(t, pdf)
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	raw, ok := root.Find("OpenAction")
	if !ok {
		return nil, ctx
	}
	r, derr := ctx.XRefTable.Dereference(raw)
	if derr != nil {
		t.Fatalf("the carried /OpenAction does not dereference: %v", derr)
	}
	return r, ctx
}

// openActionDestPage is the 1-based page index a destination array points at, or 0. It is the
// object-reference form, where `destPage` (outlinecarry_test.go) resolves a NAME.
func openActionDestPage(t *testing.T, ctx *model.Context, o types.Object) int {
	t.Helper()
	r, err := ctx.XRefTable.Dereference(o)
	if err != nil {
		return 0
	}
	arr, ok := r.(types.Array)
	if !ok || len(arr) == 0 {
		return 0
	}
	ir, ok := arr[0].(types.IndirectRef)
	if !ok {
		return 0
	}
	for i := 1; ; i++ {
		p, perr := ctx.PageDictIndRef(i)
		if perr != nil || p == nil {
			return 0
		}
		if p.ObjectNumber.Value() == ir.ObjectNumber.Value() {
			return i
		}
	}
}

// TestASubsetCarriesAnOpenActionThatStillReachesAPage — /pending 555.
//
// Measured in /pending 525's corpus scan: 220 of 295 readable catalogs carry an `/OpenAction`, and
// every subset dropped it, so an extract opened at page one whatever its source said.
//
// **The cases are the two FORMS, not two page selections**, because the form is the whole decision:
// a destination positions the view and an action dictionary runs. Reading them as one key is what
// /pending 524 refused to do for free, and it is what `Scan` was doing.
func TestASubsetCarriesAnOpenActionThatStillReachesAPage(t *testing.T) {
	destTo := func(page int) func(*model.Context) (types.Object, error) {
		return func(ctx *model.Context) (types.Object, error) {
			ir, err := ctx.PageDictIndRef(page)
			if err != nil {
				return nil, err
			}
			return types.Array{*ir, types.Name("Fit")}, nil
		}
	}

	t.Run("a destination naming a kept page survives and still names it", func(t *testing.T) {
		// Page 3 of the source, kept, and it comes out as the SECOND page of the output. The
		// assertion is the output index, so a carry that copied the array without the page tree
		// having been rewritten in place would land on the wrong page rather than on none.
		src := openActionFixture(t, destTo(3))
		out, err := Collect(src, []string{"2", "3"})
		if err != nil {
			t.Fatal(err)
		}
		got, ctx := openActionOf(t, out)
		if got == nil {
			t.Fatal("the subset dropped an /OpenAction whose destination it kept — an extract " +
				"that opens at page one is the defect /pending 555 is about")
		}
		if p := openActionDestPage(t, ctx, got); p != 2 {
			t.Errorf("the carried /OpenAction opens at output page %d, want 2 — the source's "+
				"page 3 is the output's page 2, and a destination that resolves to anything "+
				"else sends the reader to the wrong place, which is worse than not having one", p)
		}
	})

	t.Run("a destination naming a dropped page goes", func(t *testing.T) {
		// The converse, and the half that keeps the carry from being a copy. A kept /OpenAction
		// naming a removed page is an indirect reference to that page's dictionary — pdfcpu writes
		// by reachability, so it would put the dropped page and its /Contents back into the output.
		src := openActionFixture(t, destTo(1))
		out, err := Collect(src, []string{"2", "3"})
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := openActionOf(t, out); got != nil {
			t.Errorf("the subset kept an /OpenAction naming a page it dropped: %v", got)
		}
	})

	t.Run("a GoTo action becomes a plain destination", func(t *testing.T) {
		src := openActionFixture(t, func(ctx *model.Context) (types.Object, error) {
			ir, err := ctx.PageDictIndRef(3)
			if err != nil {
				return nil, err
			}
			// A /Next chain onto JavaScript, which is the trick this rewrite exists to cut. The
			// GoTo is genuine navigation and the chain is what makes carrying the DICTIONARY
			// different from carrying the destination inside it.
			return types.Dict{
				"Type": types.Name("Action"),
				"S":    types.Name("GoTo"),
				"D":    types.Array{*ir, types.Name("Fit")},
				"Next": types.Dict{
					"Type": types.Name("Action"),
					"S":    types.Name("JavaScript"),
					"JS":   types.StringLiteral("app.alert(1)"),
				},
			}, nil
		})
		out, err := Collect(src, []string{"2", "3"})
		if err != nil {
			t.Fatal(err)
		}
		got, ctx := openActionOf(t, out)
		if got == nil {
			t.Fatal("a /S /GoTo open action lost its destination entirely — the outline carry " +
				"reads the same shape for its /D and this door must agree with it")
		}
		if d, isDict := got.(types.Dict); isDict {
			t.Fatalf("the action DICTIONARY survived (%v) — carrying it re-admits an auto-run "+
				"hook the subset dropped for free, and this one chains to JavaScript", d)
		}
		if p := openActionDestPage(t, ctx, got); p != 2 {
			t.Errorf("the rewritten destination opens at output page %d, want 2", p)
		}
		// The chain is gone with the dictionary, asserted through the scanner rather than by
		// walking objects: `Scan` is what tells a user whether a document runs anything.
		rep, serr := Scan(out)
		if serr != nil {
			t.Fatal(serr)
		}
		for _, f := range rep.Findings {
			if f.Kind == "javascript" || f.Kind == "openAction" {
				t.Errorf("the subset reports %q (%s) — the executable surface after this carry "+
					"must be the one that existed before it", f.Kind, f.Detail)
			}
		}
	})

	t.Run("a JavaScript action is dropped", func(t *testing.T) {
		src := openActionFixture(t, func(ctx *model.Context) (types.Object, error) {
			// No /Type, deliberately: it is OPTIONAL on an action dictionary, so a classifier
			// keying on /Type rather than /S reads this as a destination and hands it on.
			return types.Dict{
				"S":  types.Name("JavaScript"),
				"JS": types.StringLiteral("app.alert(1)"),
			}, nil
		})
		out, err := Collect(src, []string{"2", "3"})
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := openActionOf(t, out); got != nil {
			t.Errorf("a JavaScript /OpenAction survived a subset: %v", got)
		}
	})
}

// TestScanCallsAnOpenActionAnActionOnlyWhenItIsOne — the second reader of the same key.
//
// `Scan` reported the PRESENCE of `/OpenAction` as *"Runs an action automatically when the document
// opens"*, high severity. A destination runs nothing, so that was a false statement about every
// document carrying the commonest of the two forms — and it stayed invisible while subsets dropped
// the key, because the only documents it was ever made about were sources.
func TestScanCallsAnOpenActionAnActionOnlyWhenItIsOne(t *testing.T) {
	has := func(t *testing.T, pdf []byte) bool {
		t.Helper()
		rep, err := Scan(pdf)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range rep.Findings {
			if f.Kind == "openAction" {
				return true
			}
		}
		return false
	}

	dest := openActionFixture(t, func(ctx *model.Context) (types.Object, error) {
		ir, err := ctx.PageDictIndRef(2)
		if err != nil {
			return nil, err
		}
		return types.Array{*ir, types.Name("Fit")}, nil
	})
	if has(t, dest) {
		t.Error("a document whose /OpenAction is a DESTINATION is reported as running an action " +
			"automatically — it runs nothing, and a high-severity finding a user cannot act on " +
			"is what makes the rest of the report worth ignoring")
	}

	// The control, and it is what stops the fix from being "report nothing". Same fixture, same
	// key, an action in it.
	act := openActionFixture(t, func(ctx *model.Context) (types.Object, error) {
		return types.Dict{"S": types.Name("JavaScript"), "JS": types.StringLiteral("app.alert(1)")}, nil
	})
	if !has(t, act) {
		t.Error("a document whose /OpenAction IS an action dictionary is no longer reported")
	}
}
