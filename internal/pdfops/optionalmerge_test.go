package pdfops

import (
	"context"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/testpdf"
)

// layerState reads, for the optional-content group page `page` names as `/L0`, where the catalog's
// `/OCProperties` lists it: among `/OCGs`, and in the `/ON`, `/OFF`, `/Order` and `/Locked` of the
// configuration at cfg — 0 is `/D`, n is the n-th of `/Configs`.
func layerState(t *testing.T, pdf []byte, page, cfg int) (in map[string]bool, d types.Dict) {
	t.Helper()
	ctx := readCtx(t, pdf)
	xt := ctx.XRefTable
	root, err := xt.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	pref, err := ctx.PageDictIndRef(context.Background(), page)
	if err != nil {
		t.Fatal(err)
	}
	res := derefDict(xt, derefDict(xt, *pref)["Resources"])
	group, ok := derefDict(xt, res["Properties"])["L0"].(types.IndirectRef)
	if !ok {
		t.Fatalf("page %d names no optional-content group", page)
	}
	ocp := derefDict(xt, root["OCProperties"])
	d = derefDict(xt, ocp["D"])
	if cfg > 0 {
		d = derefDict(xt, derefArray(xt, ocp["Configs"])[cfg-1])
	}
	in = map[string]bool{}
	has := func(o types.Object) bool {
		for _, e := range derefArray(xt, o) {
			if r, isRef := e.(types.IndirectRef); isRef && r.ObjectNumber == group.ObjectNumber {
				return true
			}
		}
		return false
	}
	in["OCGs"] = has(ocp["OCGs"])
	for _, k := range []string{"ON", "OFF", "Order", "Locked"} {
		in[k] = has(d[k])
	}
	return in, d
}

// TestAMergedInHiddenLayerStaysHidden — `/pending 559`, ADR-131 (`pdfread/optionalmerge.go`). A merge kept only the first document's
// `/OCProperties`, so a group a LATER document listed in `/D /OFF` was in nobody's list afterwards and a
// reader drew it: measured at 40 dpi, 86 → 18,902 dark pixels under Ghostscript and 6 → 18,598 under
// poppler, through `Append`, through `InsertPDF`, and on page 4 of a merge whose HOST has layers of its
// own. Read here as what makes a reader hide the box: the group the page names is one the catalog
// switches off.
func TestAMergedInHiddenLayerStaysHidden(t *testing.T) {
	plain, err := testpdf.Text("host one")
	if err != nil {
		t.Fatal(err)
	}
	layered := layeredFixture(t, nil)
	appended, err := Append(plain, layered)
	if err != nil {
		t.Fatal(err)
	}
	inserted, err := withoutCount(InsertPDF(plain, layered, 1, true))
	if err != nil {
		t.Fatal(err)
	}
	both, err := Append(layered, layeredFixture(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		pdf  []byte
		page int
	}{
		{"Append onto a document with no layers", appended, 2},
		{"InsertPDF into a document with no layers", inserted, 1},
		{"Append onto a document with its own layers: the host's page", both, 1},
		{"Append onto a document with its own layers: the merged-in page", both, 4},
	} {
		in, _ := layerState(t, c.pdf, c.page, 0)
		if !in["OCGs"] || !in["OFF"] {
			t.Errorf("%s: the page's group is in /OCGs %v and in /D /OFF %v; a group in neither is drawn", c.name, in["OCGs"], in["OFF"])
		}
		if !in["Order"] {
			t.Errorf("%s: the group is not in /D /Order, so the layers panel cannot show it", c.name)
		}
	}
}

// TestAMergedInLayerKeepsItsOwnStateUnderTheHostsConfiguration — the union gives each merged-in group the
// state its OWN default configuration gave it, in every configuration the host has, and leaves the host's
// own lists as they were.
func TestAMergedInLayerKeepsItsOwnStateUnderTheHostsConfiguration(t *testing.T) {
	// A host whose base state is OFF, with an alternate configuration beside the default.
	host := layeredFixture(t, func(ctx *model.Context, root types.Dict, ocg types.IndirectRef) error {
		ocp := root["OCProperties"].(types.Dict)
		ocp["D"].(types.Dict)["BaseState"] = types.Name("OFF")
		ocp["Configs"] = types.Array{types.Dict{"Name": types.StringLiteral("Print"), "BaseState": types.Name("OFF")}}
		return nil
	})
	// A document whose layer is VISIBLE: its group is in no /OFF list and its base state is the default.
	visible := layeredFixture(t, func(ctx *model.Context, root types.Dict, ocg types.IndirectRef) error {
		d := root["OCProperties"].(types.Dict)["D"].(types.Dict)
		delete(d, "OFF")
		d["Locked"] = types.Array{ocg}
		return nil
	})
	out, err := Append(host, visible)
	if err != nil {
		t.Fatal(err)
	}
	for cfg, name := range []string{"/D", "/Configs[0]"} {
		in, _ := layerState(t, out, 4, cfg)
		if !in["ON"] || in["OFF"] {
			t.Errorf("%s: the merged-in VISIBLE group is in /ON %v and /OFF %v under a base state of OFF; it must be listed on, or the merge hides it", name, in["ON"], in["OFF"])
		}
	}
	if in, _ := layerState(t, out, 4, 0); !in["Locked"] {
		t.Error("the merged-in group's /Locked entry was not carried into /D")
	}
	// The host's own group: off before, off after, and in no list it was not in.
	in, d := layerState(t, out, 1, 0)
	if !in["OFF"] || in["ON"] || in["Locked"] {
		t.Errorf("the host's own group is in /OFF %v, /ON %v, /Locked %v; a merge may not restate it", in["OFF"], in["ON"], in["Locked"])
	}
	if got := nameVal(d, "BaseState"); got != "OFF" {
		t.Errorf("the host's /BaseState is %q after the merge, want its own OFF", got)
	}

	// A document that hides its layer through its BASE STATE and lists nothing, onto a host whose base
	// state is the default ON: the group has to be named in /OFF, in the default and in the alternate.
	hiddenByBase := layeredFixture(t, func(ctx *model.Context, root types.Dict, ocg types.IndirectRef) error {
		d := root["OCProperties"].(types.Dict)["D"].(types.Dict)
		delete(d, "OFF")
		d["BaseState"] = types.Name("OFF")
		return nil
	})
	onHost := layeredFixture(t, func(ctx *model.Context, root types.Dict, ocg types.IndirectRef) error {
		root["OCProperties"].(types.Dict)["Configs"] = types.Array{types.Dict{"Name": types.StringLiteral("Print")}}
		return nil
	})
	if out, err = Append(onHost, hiddenByBase); err != nil {
		t.Fatal(err)
	}
	for cfg, name := range []string{"/D", "/Configs[0]"} {
		if in, _ := layerState(t, out, 4, cfg); !in["OFF"] || in["ON"] {
			t.Errorf("%s: a group its own document hid by /BaseState /OFF is in /OFF %v and /ON %v after the merge", name, in["OFF"], in["ON"])
		}
	}
}

// TestAMergedInAlternateConfigurationShowsOfTheHostWhatTheHostsDefaultShows — `/pending 870`, ADR-153. The
// source's alternate configurations (`/Configs`) were dropped by the merge. They are carried, and in each the
// HOST's groups stand as the host's own default has them: a configuration a reader can switch to may not show a
// layer the host hid, nor hide one it showed, on the say-so of a document that never held it.
func TestAMergedInAlternateConfigurationShowsOfTheHostWhatTheHostsDefaultShows(t *testing.T) {
	usage := func(ocg types.IndirectRef) types.Array {
		return types.Array{types.Dict{"Event": types.Name("View"), "Category": types.Array{types.Name("View")}, "OCGs": types.Array{ocg}}}
	}
	usages := func(pdf []byte, d types.Dict) int {
		return len(derefArray(readCtx(t, pdf).XRefTable, d["AS"]))
	}
	// The source: its layer hidden by default, with two alternates — one that shows everything, one whose base
	// state is OFF and that lists its own layer on.
	source := layeredFixture(t, func(ctx *model.Context, root types.Dict, ocg types.IndirectRef) error {
		ocp := root["OCProperties"].(types.Dict)
		ocp["D"].(types.Dict)["AS"] = usage(ocg)
		ocp["Configs"] = types.Array{
			types.Dict{"Name": types.StringLiteral("Everything")},
			types.Dict{"Name": types.StringLiteral("Only mine"), "BaseState": types.Name("OFF"), "ON": types.Array{ocg}},
		}
		return nil
	})
	// A host that HIDES its layer by default (the fixture's own `/D /OFF`), with a usage application on it and
	// an alternate of its own.
	hiding := layeredFixture(t, func(ctx *model.Context, root types.Dict, ocg types.IndirectRef) error {
		ocp := root["OCProperties"].(types.Dict)
		ocp["D"].(types.Dict)["AS"] = usage(ocg)
		ocp["Configs"] = types.Array{types.Dict{"Name": types.StringLiteral("Print")}}
		return nil
	})
	out, err := Append(hiding, source)
	if err != nil {
		t.Fatal(err)
	}
	// /Configs is now the host's Print, then the source's two.
	merged := readCtx(t, out).XRefTable
	root, err := merged.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if n := len(derefArray(merged, derefDict(merged, root["OCProperties"])["Configs"])); n != 3 {
		t.Fatalf("the merged document has %d alternate configuration(s), want the host's one and the source's two", n)
	}
	for cfg, name := range map[int]string{2: "the merged-in Everything", 3: "the merged-in Only mine"} {
		in, d := layerState(t, out, 1, cfg)
		if !in["OFF"] {
			t.Errorf("%s: the host's hidden layer is not in its /OFF, so switching to it draws what the host hid", name)
		}
		if got := usages(out, d); got != 1 {
			t.Errorf("%s: carries %d usage application(s), want the host default's one", name, got)
		}
	}
	if in, d := layerState(t, out, 4, 2); in["OFF"] || in["ON"] || d.StringLiteralEntry("Name") == nil || d.StringLiteralEntry("Name").Value() != "Everything" {
		t.Errorf("Everything: the source's own group is in /OFF %v and /ON %v in the second alternate %v; its own lists must stand",
			in["OFF"], in["ON"], d["Name"])
	}
	if in, _ := layerState(t, out, 4, 3); !in["ON"] || in["OFF"] {
		t.Errorf("Only mine: the source's own group is in /ON %v and /OFF %v; its own lists must stand", in["ON"], in["OFF"])
	}
	// The source default's usage application reaches the host's ALTERNATE too, not only its /D.
	if _, d := layerState(t, out, 4, 1); usages(out, d) != 1 {
		t.Errorf("the host's Print configuration carries %d usage application(s) for the merged-in group, want 1; "+
			"a group its own default hides by usage is shown there", usages(out, d))
	}
	if _, d := layerState(t, out, 4, 0); usages(out, d) != 2 {
		t.Errorf("/D carries %d usage application(s), want the host's and the source's", usages(out, d))
	}

	// A host that SHOWS its layer: under a merged-in alternate whose base state is OFF it has to be listed on,
	// or the merge hides it there.
	showing := layeredFixture(t, func(ctx *model.Context, root types.Dict, ocg types.IndirectRef) error {
		delete(root["OCProperties"].(types.Dict)["D"].(types.Dict), "OFF")
		return nil
	})
	if out, err = Append(showing, source); err != nil {
		t.Fatal(err)
	}
	if in, _ := layerState(t, out, 1, 2); !in["ON"] || in["OFF"] {
		t.Errorf("Only mine: the host's VISIBLE layer is in /ON %v and /OFF %v under a base state of OFF; it must be listed on", in["ON"], in["OFF"])
	}
	if in, _ := layerState(t, out, 1, 1); in["ON"] || in["OFF"] {
		t.Errorf("Everything: the host's visible layer is in /ON %v and /OFF %v under a base state of ON; it needs no list", in["ON"], in["OFF"])
	}
}

// TestARedactionKeepsALaterRunsLayerHidden — the same reveal on the door where it is worst. `RedactPages`
// assembles a raster page and the runs of untouched pages through `pdfread.MergeRaw`, the package's other
// merge loop; ADR-043 keeps each run's `/OCProperties`, and that loop then kept only the FIRST segment's.
// With page 1 redacted the first segment is the raster, which has none — so a layer hidden on page 2 was
// drawn in the redacted document.
func TestARedactionKeepsALaterRunsLayerHidden(t *testing.T) {
	plain, err := testpdf.Text("to be redacted")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Append(plain, layeredFixture(t, nil)) // the hidden layer is on page 2
	if err != nil {
		t.Fatal(err)
	}
	out, err := RedactPages(doc, map[int]RasterPage{1: rasterPage(t, 100, 140)})
	if err != nil {
		t.Fatal(err)
	}
	if in, _ := layerState(t, out, 2, 0); !in["OCGs"] || !in["OFF"] {
		t.Errorf("after redacting page 1, page 2's group is in /OCGs %v and /D /OFF %v; the redaction reveals the layer", in["OCGs"], in["OFF"])
	}
	// And a run after a redacted page in the MIDDLE, whose first segment has layers of its own.
	if out, err = RedactPages(doc, map[int]RasterPage{3: rasterPage(t, 100, 140)}); err != nil {
		t.Fatal(err)
	}
	if in, _ := layerState(t, out, 2, 0); !in["OFF"] {
		t.Errorf("after redacting page 3, page 2's group is no longer in /D /OFF")
	}
}
