package pdfops

import (
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/testpdf"
)

// layeredFixture is a three-page document whose PAGE 1 — a page every test below keeps — carries a
// 300×200pt filled box inside an optional-content group the catalog lists in `/D /OFF`. The box is
// the thing a reader must not draw, and it is on a kept page deliberately: a hidden layer on a
// DROPPED page is carried away with the page and proves nothing.
//
// tweak, when non-nil, is handed the catalog and the group's reference after both exist, so a test
// can craft the rest of `/OCProperties` or set a `/PageMode`.
func layeredFixture(t *testing.T, tweak func(ctx *model.Context, root types.Dict, ocg types.IndirectRef) error) []byte {
	t.Helper()
	src, err := testpdf.Text("alpha", "beta", "gamma")
	if err != nil {
		t.Fatal(err)
	}
	out, err := writeMutated(src, func(ctx *model.Context) error {
		xt := ctx.XRefTable
		root, rerr := xt.Catalog()
		if rerr != nil {
			return rerr
		}
		ocgRef, ierr := xt.IndRefForNewObject(types.Dict{
			"Type": types.Name("OCG"),
			"Name": types.StringLiteral("Attorney notes"),
		})
		if ierr != nil {
			return ierr
		}
		root["OCProperties"] = types.Dict{
			"OCGs": types.Array{*ocgRef},
			"D": types.Dict{
				"Name":  types.StringLiteral("Default"),
				"Order": types.Array{*ocgRef},
				"OFF":   types.Array{*ocgRef},
			},
		}
		pref, perr := ctx.PageDictIndRef(1)
		if perr != nil {
			return perr
		}
		pd := derefDict(xt, *pref)
		res := derefDict(xt, pd["Resources"])
		if res == nil {
			res = types.Dict{}
			pd["Resources"] = res
		}
		res["Properties"] = types.Dict{"L0": *ocgRef}
		sd, serr := xt.NewStreamDictForBuf([]byte("/OC /L0 BDC\nq 0 0 0 rg 100 400 300 200 re f Q\nEMC\n"))
		if serr != nil {
			return serr
		}
		if eerr := sd.Encode(); eerr != nil {
			return eerr
		}
		sref, ierr := xt.IndRefForNewObject(*sd)
		if ierr != nil {
			return ierr
		}
		switch c := pd["Contents"].(type) {
		case types.Array:
			pd["Contents"] = append(c, *sref)
		default:
			pd["Contents"] = types.Array{c, *sref}
		}
		if tweak != nil {
			return tweak(ctx, root, *ocgRef)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// hiddenGroupOfPage1 resolves the object number the FIRST page's content names as its
// optional-content group, and the set of group object numbers the catalog switches off. Reading
// both from the same document is the point: what makes a reader hide the box is that these two are
// the same object, and an assertion on either alone cannot see that.
func hiddenGroupOfPage1(t *testing.T, pdf []byte) (group int, off map[int]bool, hasOCP bool) {
	t.Helper()
	ctx := readCtx(t, pdf)
	xt := ctx.XRefTable
	root, err := xt.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	group = -1
	pref, perr := ctx.PageDictIndRef(1)
	if perr != nil {
		t.Fatal(perr)
	}
	res := derefDict(xt, derefDict(xt, *pref)["Resources"])
	if props := derefDict(xt, res["Properties"]); props != nil {
		if ir, ok := props["L0"].(types.IndirectRef); ok {
			group = ir.ObjectNumber.Value()
		}
	}
	off = map[int]bool{}
	ocp := derefDict(xt, root["OCProperties"])
	hasOCP = ocp != nil
	if ocp != nil {
		if d := derefDict(xt, ocp["D"]); d != nil {
			for _, o := range derefArray(xt, d["OFF"]) {
				if ir, ok := o.(types.IndirectRef); ok {
					off[ir.ObjectNumber.Value()] = true
				}
			}
		}
	}
	return group, off, hasOCP
}

// pageObjectCount counts every object in the document that is a page DICTIONARY, whether or not the
// page tree reaches it. It is how a leak is seen: pdfcpu writes by reachability, so a dropped page
// whose dict something still references is written out even though no page tree names it.
func pageObjectCount(t *testing.T, pdf []byte) int {
	t.Helper()
	ctx := readCtx(t, pdf)
	n := 0
	for nr := range ctx.XRefTable.Table {
		o, err := ctx.XRefTable.Dereference(*types.NewIndirectRef(nr, 0))
		if err != nil {
			continue
		}
		if d, ok := o.(types.Dict); ok && nameVal(d, "Type") == "Page" {
			n++
		}
	}
	return n
}

// TestAHiddenLayerIsStillHiddenAfterASelection is this change's central instrument — `/pending 525`.
//
// Dropping `/OCProperties` does not lose a layer, it REVEALS one: the bracketing `/OC /L0 BDC … EMC`
// and the page's `/Resources /Properties` both travel with the page dictionary, and the only thing
// that said the group was off was the catalog key. Measured on the same fixture at 40 dpi, page 1
// went from 39 dark pixels to 18,855 under Ghostscript 10.02.1 and from 6 to 18,598 under poppler's
// pdftoppm — the box the author hid, printed on the page the user is about to hand over.
//
// **Both doors, and `collectWithoutStructure` is the one that matters.** `RedactPages` builds its
// runs of untouched pages through it, so the reveal landed inside a redaction; a carry gated on the
// structure carry would have fixed the door where this is least dangerous and left the one where it
// is worst.
func TestAHiddenLayerIsStillHiddenAfterASelection(t *testing.T) {
	src := layeredFixture(t, nil)

	srcGroup, srcOff, srcHas := hiddenGroupOfPage1(t, src)
	if !srcHas || srcGroup < 0 || !srcOff[srcGroup] {
		t.Fatalf("setup: the fixture does not hide page 1's box (OCProperties=%v group=%d off=%v) — "+
			"every assertion below would be grading a document that was never hiding anything",
			srcHas, srcGroup, srcOff)
	}

	for _, c := range []struct {
		name string
		run  func([]byte) ([]byte, error)
	}{
		{"Collect", func(b []byte) ([]byte, error) { return Collect(b, []string{"1", "3"}) }},
		{"collectWithoutStructure", func(b []byte) ([]byte, error) {
			return collectWithoutStructure(b, []string{"1", "3"})
		}},
	} {
		out, err := c.run(src)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		group, off, has := hiddenGroupOfPage1(t, out)
		if !has {
			t.Errorf("%s: the output has no /OCProperties, so a reader has no optional content to "+
				"recognise and draws the box the source hid", c.name)
			continue
		}
		if group < 0 {
			t.Errorf("%s: the kept page no longer names an optional-content group, so nothing in "+
				"the catalog can switch its content off", c.name)
			continue
		}
		if !off[group] {
			t.Errorf("%s: the kept page's group %d is not among the groups the output switches off "+
				"(%v) — the layer is carried and visible, which is worse than carried and absent",
				c.name, group, off)
		}
	}
}

// TestOptionalContentReachingADroppedPageIsRefused drives the walk that makes the carry safe.
//
// `/OCProperties` holds indirect references, pdfcpu writes by reachability, and a reference in it
// that resolves to a page this selection DROPPED puts that page's dictionary and its `/Contents`
// back into the output — `pageselect.go`'s `/StructTreeRoot` hazard one key over. No real document
// does this (measured: the six corpus files carrying the key hold no page reference anywhere), so
// the refusal is written against a crafted one, and it refuses the subtree ENTIRE: the result is the
// document this primitive wrote before the carry existed.
func TestOptionalContentReachingADroppedPageIsRefused(t *testing.T) {
	src := layeredFixture(t, func(ctx *model.Context, root types.Dict, ocg types.IndirectRef) error {
		// Page 2 is the page the selection below drops.
		second, err := ctx.PageDictIndRef(2)
		if err != nil {
			return err
		}
		ocp := root["OCProperties"].(types.Dict)
		ocp["OCGs"] = types.Array{ocg, *second}
		return nil
	})
	if n := pageObjectCount(t, src); n != 3 {
		t.Fatalf("setup: the fixture holds %d page objects, want 3", n)
	}

	out, err := Collect(src, []string{"1", "3"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := readCtx(t, out)
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if _, has := root["OCProperties"]; has {
		t.Error("an /OCProperties naming a dropped page was carried")
	}
	if n := pageObjectCount(t, out); n != 2 {
		t.Errorf("the output holds %d page objects, want the 2 it kept — the dropped page's "+
			"dictionary, and so its /Contents, is still in the file", n)
	}
}

// TestASelectionCarriesNoOutputIntent is the converse assertion for a key decided DROPPED, so a
// later change cannot silently start carrying it.
//
// `/OutputIntents` is the ICC profile a PDF/A claim rests on, and a subset has already removed the
// claim: `/Metadata` is off the allowlist, and the carrying path builds a fresh title-only packet.
// Measured, 26 of 26 entries in veraPDF's corpus are `/S /GTS_PDFA1` — a subtype whose whole meaning
// is "this is the output intent PDF/A requires", which on a document making no PDF/A claim is
// ADR-032's rule one key over.
func TestASelectionCarriesNoOutputIntent(t *testing.T) {
	src, err := testpdf.Text("alpha", "beta", "gamma")
	if err != nil {
		t.Fatal(err)
	}
	src, err = writeMutated(src, func(ctx *model.Context) error {
		root, rerr := ctx.XRefTable.Catalog()
		if rerr != nil {
			return rerr
		}
		oi, ierr := ctx.XRefTable.IndRefForNewObject(types.Dict{
			"Type":                      types.Name("OutputIntent"),
			"S":                         types.Name("GTS_PDFA1"),
			"OutputConditionIdentifier": types.StringLiteral("sRGB"),
		})
		if ierr != nil {
			return ierr
		}
		root["OutputIntents"] = types.Array{*oi}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if root, _ := readCtx(t, src).XRefTable.Catalog(); root["OutputIntents"] == nil {
		t.Fatal("setup: the fixture carries no /OutputIntents, so both assertions below would pass " +
			"on a build that carries the key")
	}

	for _, c := range []struct {
		name string
		run  func([]byte) ([]byte, error)
	}{
		{"Collect", func(b []byte) ([]byte, error) { return Collect(b, []string{"1", "3"}) }},
		{"collectWithoutStructure", func(b []byte) ([]byte, error) {
			return collectWithoutStructure(b, []string{"1", "3"})
		}},
	} {
		out, err := c.run(src)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		root, rerr := readCtx(t, out).XRefTable.Catalog()
		if rerr != nil {
			t.Fatal(rerr)
		}
		if _, has := root["OutputIntents"]; has {
			t.Errorf("%s: the subset carries an /OutputIntents onto a document whose PDF/A "+
				"identification it removed", c.name)
		}
	}
}

// TestPageLayoutSurvivesASelection — the key is a bare name that makes no claim about any particular
// page, which is what separates it from `/PageLabels`.
//
// The second half is the decision that it is READ as a name and re-written as one: a `/PageLayout`
// written as an indirect reference is dropped, because passing the source's object through would
// let one of its references ride out on a key nobody would think to check.
func TestPageLayoutSurvivesASelection(t *testing.T) {
	build := func(t *testing.T, indirect bool) []byte {
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
			if !indirect {
				root["PageLayout"] = types.Name("TwoColumnLeft")
				return nil
			}
			ref, ierr := ctx.XRefTable.IndRefForNewObject(types.Name("TwoColumnLeft"))
			if ierr != nil {
				return ierr
			}
			root["PageLayout"] = *ref
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}

	for _, c := range []struct {
		name string
		run  func([]byte) ([]byte, error)
	}{
		{"Collect", func(b []byte) ([]byte, error) { return Collect(b, []string{"1", "3"}) }},
		{"collectWithoutStructure", func(b []byte) ([]byte, error) {
			return collectWithoutStructure(b, []string{"1", "3"})
		}},
	} {
		out, err := c.run(build(t, false))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		root, rerr := readCtx(t, out).XRefTable.Catalog()
		if rerr != nil {
			t.Fatal(rerr)
		}
		if got := nameVal(root, "PageLayout"); got != "TwoColumnLeft" {
			t.Errorf("%s: /PageLayout = %q, want TwoColumnLeft", c.name, got)
		}
	}

	out, err := Collect(build(t, true), []string{"1", "3"})
	if err != nil {
		t.Fatal(err)
	}
	root, rerr := readCtx(t, out).XRefTable.Catalog()
	if rerr != nil {
		t.Fatal(rerr)
	}
	if _, has := root["PageLayout"]; has {
		t.Error("an indirect /PageLayout was passed through, so a reference of the source's rode out " +
			"on a key nothing inspects")
	}
}

// TestUseOCIsCarriedOnlyWhereItsOptionalContentSurvived — `/PageMode` states which panel the reader
// opens with, and the rule is one sentence: a value survives when the thing it names does.
//
// The two halves are the same document differing in one crafted reference, so neither can pass by
// accident: where `/OCProperties` is carried the mode is too, and where the walk refuses it the mode
// goes with it rather than opening a layers panel on a document that declares no layers.
func TestUseOCIsCarriedOnlyWhereItsOptionalContentSurvived(t *testing.T) {
	useOC := func(ctx *model.Context, root types.Dict, ocg types.IndirectRef) error {
		root["PageMode"] = types.Name("UseOC")
		return nil
	}
	kept := layeredFixture(t, useOC)
	refused := layeredFixture(t, func(ctx *model.Context, root types.Dict, ocg types.IndirectRef) error {
		if err := useOC(ctx, root, ocg); err != nil {
			return err
		}
		second, err := ctx.PageDictIndRef(2)
		if err != nil {
			return err
		}
		ocp := root["OCProperties"].(types.Dict)
		ocp["OCGs"] = types.Array{ocg, *second}
		return nil
	})
	for _, src := range [][]byte{kept, refused} {
		if root, _ := readCtx(t, src).XRefTable.Catalog(); nameVal(root, "PageMode") != "UseOC" {
			t.Fatal("setup: a fixture does not ask for /UseOC, so its half below grades nothing")
		}
	}

	out, err := Collect(kept, []string{"1", "3"})
	if err != nil {
		t.Fatal(err)
	}
	root, rerr := readCtx(t, out).XRefTable.Catalog()
	if rerr != nil {
		t.Fatal(rerr)
	}
	if got := nameVal(root, "PageMode"); got != "UseOC" {
		t.Errorf("/PageMode = %q, want UseOC — the optional content it names survived", got)
	}

	out, err = Collect(refused, []string{"1", "3"})
	if err != nil {
		t.Fatal(err)
	}
	root, rerr = readCtx(t, out).XRefTable.Catalog()
	if rerr != nil {
		t.Fatal(rerr)
	}
	if got := nameVal(root, "PageMode"); got != "" {
		t.Errorf("/PageMode = %q where the /OCProperties it names was refused — the reader is told "+
			"to open a layers panel for a document that declares no layers", got)
	}
}
