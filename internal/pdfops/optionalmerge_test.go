package pdfops

import (
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
	pref, err := ctx.PageDictIndRef(page)
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
	inserted, err := InsertPDF(plain, layered, 1, true)
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
