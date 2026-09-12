package pdfops

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The optional-content configuration a stamp leaves behind — `/pending 473`.

// ocConfig returns the catalog's default optional-content configuration dictionary, and the OCGs
// the document declares.
func ocConfig(t *testing.T, pdf []byte) (types.Dict, []types.Object) {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	root, rerr := ctx.XRefTable.Catalog()
	if rerr != nil {
		t.Fatal(rerr)
	}
	ocp, oerr := ctx.DereferenceDict(root["OCProperties"])
	if oerr != nil || ocp == nil {
		t.Fatal("the stamped document declares no /OCProperties — the stamp stopped using an " +
			"optional-content group and every assertion below would be about a dictionary that " +
			"is not there")
	}
	d, derr := ctx.DereferenceDict(ocp["D"])
	if derr != nil || d == nil {
		t.Fatal("/OCProperties has no default configuration dictionary")
	}
	on, _ := ctx.DereferenceArray(d["ON"])
	return d, on
}

// stampers is every door that puts its output in an optional-content group. Both 7.10 clauses say
// "each … configuration dictionary", so the population is the doors, not the one that was noticed.
var stampers = map[string]func(*testing.T, []byte) ([]byte, error){
	"StampWatermark": func(t *testing.T, b []byte) ([]byte, error) {
		return StampWatermark(b, "DRAFT", WatermarkStyle{})
	},
	"StampPageNumbers": func(t *testing.T, b []byte) ([]byte, error) {
		return StampPageNumbers(b, PageNumberStyle{})
	},
	"StampImages": func(t *testing.T, b []byte) ([]byte, error) {
		return StampImages(b, []Stamp{{Page: 1, Rect: [4]float64{72, 700, 140, 740}, PNG: pngBytes(t, 40, 20)}})
	},
	"StampFields": func(t *testing.T, b []byte) ([]byte, error) {
		out, _, err := StampFields(b, []Field{{Page: 1, Rect: [4]float64{72, 700, 200, 716}, Text: "filled"}})
		return out, err
	},
	"StampTextLayer": func(t *testing.T, b []byte) ([]byte, error) {
		return StampTextLayer(b, []Word{{Page: 1, Rect: [4]float64{72, 700, 140, 712}, Text: "Invoice"}}, "eng")
	},
}

// TestNoStampLeavesAConfigurationPDFUAForbids — `/pending 473`.
//
// veraPDF's own words for the two clauses: 7.10 t1 *"Each optional content configuration dictionary
// … shall contain the Name key"*, 7.10 t2 *"The AS key shall not appear in any optional content
// configuration dictionary"*. The ua1 oracle measures the outcome on the whole census; this asserts
// the SHAPE, so a failure says which key on which door rather than which clause on which file.
func TestNoStampLeavesAConfigurationPDFUAForbids(t *testing.T) {
	base, err := testpdf.Text("a page")
	if err != nil {
		t.Fatal(err)
	}
	ran := 0
	for name, stamp := range stampers {
		out, err := stamp(t, base)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		ran++
		d, _ := ocConfig(t, out)
		if _, has := d["AS"]; has {
			t.Errorf("%s leaves /AS in the optional-content configuration — ua1 7.10 t2 forbids the "+
				"key by name", name)
		}
		n, ok := d["Name"].(types.StringLiteral)
		if !ok || len(n) == 0 {
			t.Errorf("%s leaves the optional-content configuration with no /Name — ua1 7.10 t1 "+
				"requires one, and an empty string fails it the same way", name)
		}
	}
	// Stimulus floor: a map that lost its entries would pass every assertion above.
	if ran < 5 {
		t.Fatalf("only %d stamping door(s) ran, want all 5 — the population shrank and this "+
			"guard is checking almost nothing", ran)
	}
}

// TestRemovingTheAutoStateLeavesTheStampVISIBLE — the half of `/pending 473` the item said had to be
// measured before anything was removed.
//
// `/AS` applies usage settings automatically. Removing a key that controls visibility could hide
// every watermark nib has ever stamped, which is why the item refused to guess. Measured: the
// written configuration is `[ON Order RBGroups AS]` — `/ON` names the group explicitly, there is no
// `/OFF` and no `/BaseState` (which defaults to ON), and all three `/AS` entries set that same group
// ON for View, Print and Export. So `/AS` restates `/ON` and removing it changes nothing.
//
// This asserts the part that argument rests on, because the argument is only as good as the
// document still being shaped that way.
func TestRemovingTheAutoStateLeavesTheStampVISIBLE(t *testing.T) {
	base, err := testpdf.Text("a page")
	if err != nil {
		t.Fatal(err)
	}
	out, err := StampWatermark(base, "DRAFT", WatermarkStyle{})
	if err != nil {
		t.Fatal(err)
	}
	d, on := ocConfig(t, out)
	if len(on) == 0 {
		t.Error("the configuration's /ON array is empty, so nothing states the group is visible — " +
			"with /AS gone as well, the stamp's visibility now rests on the BaseState default alone")
	}
	if _, off := d["OFF"]; off {
		t.Error("the configuration now carries an /OFF array. /AS was removed on the measured " +
			"ground that /ON said the same thing; an /OFF entry means it did not")
	}
	if bs, has := d["BaseState"]; has {
		if n, _ := bs.(types.Name); n != "ON" {
			t.Errorf("the configuration declares /BaseState %v. The safety of removing /AS rested "+
				"on the default, which is ON", bs)
		}
	}
}

// TestTheStampIsSTILLONTHEPAGEAfterTheAutoStateIsRemoved — the same claim as the test above,
// settled by rasterising instead of by reading the dictionary.
//
// The structural argument (`/ON` names the group, no `/OFF`, no `/BaseState`) is sound and it is
// still an argument. `/AS` removal is a change to what a VIEWER shows, and the cheap direction is
// to look: stamp a solid red square, render both the corrected document and one with `/AS` put
// back, and require red pixels in both. A watermark that silently stopped printing on every
// document nib has ever stamped is exactly the failure `/pending 473` refused to risk on reasoning.
//
// Skips loudly when pdftoppm is absent, because a quiet skip here turns the only visual evidence
// for a visibility change back into nothing.
func TestTheStampIsSTILLONTHEPAGEAfterTheAutoStateIsRemoved(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("SKIP (not a pass): pdftoppm is absent, so `/pending 473`'s visibility claim is " +
			"resting on the dictionary alone in this run.")
	}
	bg := image.NewRGBA(image.Rect(0, 0, 612, 792))
	for i := range bg.Pix {
		bg.Pix[i] = 0xff
	}
	var bgBuf bytes.Buffer
	if err := png.Encode(&bgBuf, bg); err != nil {
		t.Fatal(err)
	}
	base, err := ImagesToPDF([]RasterPage{{Image: bgBuf.Bytes(), W: 612, H: 792}})
	if err != nil {
		t.Fatal(err)
	}
	sq := image.NewRGBA(image.Rect(0, 0, 100, 40))
	for x := 0; x < 100; x++ {
		for y := 0; y < 40; y++ {
			sq.Set(x, y, color.RGBA{220, 0, 0, 255})
		}
	}
	var sqBuf bytes.Buffer
	if err := png.Encode(&sqBuf, sq); err != nil {
		t.Fatal(err)
	}
	corrected, err := StampImages(base, []Stamp{{Page: 1, Rect: [4]float64{100, 100, 300, 180}, PNG: sqBuf.Bytes()}})
	if err != nil {
		t.Fatal(err)
	}
	// The same document with /AS restored — the state before this fix, and the control that says
	// the renderer would have shown the stamp either way rather than never showing it at all.
	withAS, err := writeMutated(corrected, func(ctx *model.Context) error {
		root, _ := ctx.XRefTable.Catalog()
		ocp, _ := ctx.DereferenceDict(root["OCProperties"])
		d, _ := ctx.DereferenceDict(ocp["D"])
		on, _ := ctx.DereferenceArray(d["ON"])
		d["AS"] = types.Array{types.Dict{
			"Category": types.Array{types.Name("View")},
			"Event":    types.Name("View"),
			"OCGs":     on,
		}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	red := func(label string, pdf []byte) int {
		dir := t.TempDir()
		p := filepath.Join(dir, "x.pdf")
		if err := os.WriteFile(p, pdf, 0o600); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command("pdftoppm", "-png", "-r", "36", "-singlefile", p, filepath.Join(dir, "page")).CombinedOutput(); err != nil {
			t.Fatalf("%s: pdftoppm: %v\n%s", label, err, out)
		}
		f, err := os.Open(filepath.Join(dir, "page.png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		img, derr := png.Decode(f)
		if derr != nil {
			t.Fatal(derr)
		}
		n := 0
		b := img.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r, g, bl, _ := img.At(x, y).RGBA()
				if r>>8 > 150 && g>>8 < 100 && bl>>8 < 100 {
					n++
				}
			}
		}
		t.Logf("%s: %d red pixel(s)", label, n)
		return n
	}
	was := red("/AS restored (the state before this fix)", withAS)
	is := red("/AS removed (what nib writes now)", corrected)
	// Control FIRST: with /AS present the stamp renders, so a zero below is the removal and not a
	// renderer that never showed it.
	if was == 0 {
		t.Fatalf("control: the stamp does not render even with /AS present, so the assertion below " +
			"cannot tell a hidden stamp from a test that rasterises the wrong thing")
	}
	if is == 0 {
		t.Errorf("removing /AS made the stamp INVISIBLE — %d red pixel(s) with it, none without. "+
			"Every watermark, page number and OCR layer nib writes would be gone from the page",
			was)
	}
	// And not merely non-zero: the same stamp, so the same coverage.
	if d := is - was; d < -was/20 || d > was/20 {
		t.Errorf("the stamp covers %d red pixel(s) without /AS against %d with it — more than a "+
			"5%% difference, so removing the key changed what is drawn and not just what is declared",
			is, was)
	}
}
