package pdfops

import (
	"bytes"
	"fmt"
	"image"
	"math"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// `PLAN-ua-coverage.md` P02.S05 — a crop moves each page's boxes and nothing else, so the
// structure tree it carries is the tree it was given (ADR-047).

// pageAttrs reads every page's effective box attributes, inheritance resolved.
func pageAttrs(t *testing.T, pdf []byte) []*model.InheritedPageAttrs {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("the output could not be read back: %v", err)
	}
	out := make([]*model.InheritedPageAttrs, 0, ctx.PageCount)
	for p := 1; p <= ctx.PageCount; p++ {
		_, _, attrs, err := ctx.PageDict(p, false)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, attrs)
	}
	return out
}

// pageAnnotRects is every annotation's `/Rect` per page, in page order.
func pageAnnotRects(t *testing.T, pdf []byte) [][]string {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	xt := ctx.XRefTable
	out := make([][]string, ctx.PageCount)
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, err := ctx.PageDict(p, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range derefArray(xt, d["Annots"]) {
			if ad := derefDict(xt, a); ad != nil {
				out[p-1] = append(out[p-1], derefArray(xt, ad["Rect"]).String())
			}
		}
	}
	return out
}

// pageKeys is each page dictionary's key set, sorted, with the four boxes a crop writes left out.
func pageKeys(t *testing.T, pdf []byte) []string {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, ctx.PageCount)
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, err := ctx.PageDict(p, false)
		if err != nil {
			t.Fatal(err)
		}
		keys := []string{}
		for k := range d {
			switch k {
			case "MediaBox", "CropBox", "BleedBox", "TrimBox", "ArtBox":
			default:
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		out[p-1] = strings.Join(keys, " ")
	}
	return out
}

func boxString(r *types.Rectangle) string {
	return fmt.Sprintf("[%.1f %.1f %.1f %.1f]", r.LL.X, r.LL.Y, r.UR.X, r.UR.Y)
}

// TestACropCarriesTheStructureOfEveryPage is the slice's central reader, driven on the fixture with
// an MCR, two OBJRs, a RoleMap and annotations — the census's one-page fixture has none of those.
func TestACropCarriesTheStructureOfEveryPage(t *testing.T) {
	src := subsetFixture()
	_, srcDefects, _, srcElems := carryOf(t, src)
	if srcElems == 0 || len(srcDefects) != 0 {
		t.Fatalf("setup: the fixture has %d elements and %d defects; a crop cannot be graded against it",
			srcElems, len(srcDefects))
	}
	srcRects := pageAnnotRects(t, src)
	srcKeys := pageKeys(t, src)

	out, err := Crop(src, [4]float64{0.1, 0.1, 0.5, 0.5}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// The STIMULUS first: a Crop that did nothing would carry everything below perfectly.
	attrs := pageAttrs(t, out)
	if len(attrs) != 4 {
		t.Fatalf("the crop returned %d pages, want 4", len(attrs))
	}
	const want = "[61.2 316.8 367.2 712.8]" // 612×792, the display window from 10% to 60% each way
	for i, a := range attrs {
		if got := boxString(a.MediaBox); got != want {
			t.Fatalf("setup: page %d's MediaBox is %s, want %s — the crop did not happen, so nothing "+
				"below can say whether a crop carries the tree", i+1, got, want)
		}
	}

	verdict, defects, orphans, elems := carryOf(t, out)
	if verdict != "carried" {
		t.Errorf("a crop's fate is %q, want carried: the tree describes the file, not the view", verdict)
	}
	if len(defects) != 0 {
		t.Errorf("the cropped tree has %d completeness defects: %v", len(defects), defects)
	}
	if len(orphans) != 0 {
		t.Errorf("the crop left page objects outside the page tree: %v", orphans)
	}
	if elems != srcElems {
		t.Errorf("the crop kept %d of %d elements; moving a box removes no content", elems, srcElems)
	}
	for p := 1; p <= 4; p++ {
		if want := []string{"ZZPAGEONE", "ZZPAGETWO", "ZZPAGETHREE", "ZZPAGEFOUR"}[p-1]; !strings.Contains(pageText(t, out, p), want) {
			t.Errorf("page %d no longer holds %s — a crop hides content, it does not remove it", p, want)
		}
	}
	// The annotations stay, where they were. The previous shape rebuilt each page through
	// `CutPage`, which deletes `/Annots`, so a crop silently destroyed every link, comment and form
	// field on the pages it touched — and the two `/Link` elements' OBJRs pointed at nothing.
	// "Its boxes and nothing else", asserted where it can fail: every other key of every page
	// dictionary is still there, and none was added.
	for p, keys := range pageKeys(t, out) {
		if keys != srcKeys[p] {
			t.Errorf("page %d's dictionary keys (boxes aside) were %s and are %s after a crop", p+1, srcKeys[p], keys)
		}
	}
	gotRects := pageAnnotRects(t, out)
	for p := range srcRects {
		if strings.Join(gotRects[p], " ") != strings.Join(srcRects[p], " ") {
			t.Errorf("page %d's annotations were %v and are %v after a crop", p+1, srcRects[p], gotRects[p])
		}
	}
}

// TestCropMapsTheDisplayWindowAtEveryRotation — the fractions are read in DISPLAY space, and the box
// is written in the page's own space, so each of the four rotations is a different mapping. The
// oracle is poppler's own rendering of the uncropped rotated page: the colour at the centre of the
// display quadrant the user drew around must be the colour of the whole cropped page.
func TestCropMapsTheDisplayWindowAtEveryRotation(t *testing.T) {
	skipNoPoppler(t)
	base := fourQuadrantPDF(t, 300, 400)
	quadrants := [4][4]float64{{0, 0, 0.5, 0.5}, {0.5, 0, 0.5, 0.5}, {0, 0.5, 0.5, 0.5}, {0.5, 0.5, 0.5, 0.5}}
	for _, rot := range []int{0, 90, 180, 270, -90} {
		t.Run(fmt.Sprintf("rotate%d", rot), func(t *testing.T) {
			src := base
			if rot != 0 {
				var err error
				if src, err = Rotate(base, nil, rot); err != nil {
					t.Fatal(err)
				}
			}
			truth := onePage(t, renderPDF(t, src, fmt.Sprintf("truth%d", rot)))
			tb := truth.Bounds()
			seen := map[int]bool{}
			for qi, q := range quadrants {
				// The truth colour at this display quadrant's centre, read off the rotated render.
				cx := tb.Min.X + int((q[0]+q[2]/2)*float64(tb.Dx()))
				cy := tb.Min.Y + int((q[1]+q[3]/2)*float64(tb.Dy()))
				want := nearestQuad(truth.At(cx, cy))
				seen[want] = true

				out, err := Crop(src, q, nil)
				if err != nil {
					t.Fatal(err)
				}
				img := onePage(t, renderPDF(t, out, fmt.Sprintf("crop%d-%d", rot, qi)))
				for _, p := range cornersAndCentre(img.Bounds()) {
					if got := nearestQuad(img.At(p[0], p[1])); got != want {
						t.Errorf("display quadrant %d: the cropped page shows colour %d at (%d,%d), want %d",
							qi, got, p[0], p[1], want)
					}
				}
				// Half the display in each direction, in display orientation.
				if dx, dy := img.Bounds().Dx(), img.Bounds().Dy(); absInt(2*dx-tb.Dx()) > 4 || absInt(2*dy-tb.Dy()) > 4 {
					t.Errorf("display quadrant %d: cropped page is %dx%d px, want half of %dx%d",
						qi, dx, dy, tb.Dx(), tb.Dy())
				}
			}
			// The stimulus: four display quadrants, four different colours, or the fixture cannot
			// tell one mapping from another.
			if len(seen) != 4 {
				t.Fatalf("setup: the rotated page shows %d distinct quadrant colours, want 4", len(seen))
			}
		})
	}
}

func onePage(t *testing.T, imgs []image.Image) image.Image {
	t.Helper()
	if len(imgs) != 1 {
		t.Fatalf("rendered %d pages, want 1", len(imgs))
	}
	return imgs[0]
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// TestCropReadsTheBoxAndRotationAPageINHERITS — a page whose `/CropBox` and `/Rotate` come from an
// intermediate `/Pages` node says nothing about either on its own dictionary. The display box is the
// inherited CropBox turned by the inherited rotation, and the inherited CropBox is then overridden
// explicitly: deleting a key the page never had would leave the ancestor's larger box in force.
func TestCropReadsTheBoxAndRotationAPageINHERITS(t *testing.T) {
	src := nestedInheritingFixture(t)
	before := pageAttrs(t, src)
	if before[1].CropBox == nil || before[1].Rotate != 90 {
		t.Fatalf("setup: page 2 inherits CropBox %v and Rotate %d; the fixture no longer has the shape",
			before[1].CropBox, before[1].Rotate)
	}
	// The LEFT half of what the user sees. At /Rotate 90 the display's horizontal axis is the page's
	// VERTICAL one, running up from the CropBox's bottom edge — so the window is the bottom half of
	// the CropBox [20 20 400 500], full width.
	out, err := Crop(src, [4]float64{0, 0, 0.5, 1}, []string{"2"})
	if err != nil {
		t.Fatal(err)
	}
	after := pageAttrs(t, out)
	if got, want := boxString(after[1].MediaBox), "[20.0 20.0 400.0 260.0]"; got != want {
		t.Errorf("page 2's MediaBox is %s, want %s", got, want)
	}
	if after[1].CropBox == nil || boxString(after[1].CropBox) != boxString(after[1].MediaBox) {
		t.Errorf("page 2's effective CropBox is %v; the inherited one was not overridden", after[1].CropBox)
	}
	if after[1].Rotate != 90 {
		t.Errorf("page 2's rotation is %d, want the inherited 90 kept", after[1].Rotate)
	}
	// Page 3 shares the ancestor and was not selected: overriding page 2 must not reach it.
	if boxString(after[2].MediaBox) != boxString(before[2].MediaBox) || boxString(after[2].CropBox) != boxString(before[2].CropBox) {
		t.Errorf("page 3 moved from %s/%s to %s/%s; it was not in the selection",
			boxString(before[2].MediaBox), boxString(before[2].CropBox),
			boxString(after[2].MediaBox), boxString(after[2].CropBox))
	}
}

// TestACropKeepsEveryAnnotationSubtype — the crop never touches `/Annots`, so a note and a form field
// keep their place exactly as a link does. The README and ADR-047 say all three; the carry reader above
// drives links only.
func TestACropKeepsEveryAnnotationSubtype(t *testing.T) {
	objs := subsetFixtureObjects()
	objs[3] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 11 0 R >> >> " +
		"/Contents 4 0 R /StructParents 0 /Annots [20 0 R 24 0 R 25 0 R] >>"
	objs[24] = "<< /Type /Annot /Subtype /Text /Rect [400 700 420 720] /Contents (ZZNOTE) /P 3 0 R >>"
	objs[25] = "<< /Type /Annot /Subtype /Widget /FT /Tx /T (ZZFIELD) /DA (/Helv 0 Tf 0 g) /Rect [72 100 300 124] /P 3 0 R >>"
	objs[1] = strings.Replace(objs[1], "/Metadata 43 0 R", "/Metadata 43 0 R /AcroForm << /Fields [25 0 R] >>", 1)
	src := assembleFixture(objs)
	before := pageAnnotRects(t, src)
	if len(before[0]) != 3 {
		t.Fatalf("setup: page 1 carries %d annotations, want a link, a note and a field", len(before[0]))
	}
	out, err := Crop(src, [4]float64{0.1, 0.1, 0.5, 0.5}, []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	if got := boxString(pageAttrs(t, out)[0].MediaBox); got != "[61.2 316.8 367.2 712.8]" {
		t.Fatalf("setup: page 1's MediaBox is %s — the crop did not happen", got)
	}
	if after := pageAnnotRects(t, out); strings.Join(after[0], " ") != strings.Join(before[0], " ") {
		t.Errorf("page 1's annotations were %v and are %v after a crop", before[0], after[0])
	}
}

// TestACropWindowMustLieOnThePage — a crop REPLACES the page's boxes with the window, so a window
// reaching past what the page displays would widen a CropBox an earlier crop narrowed and bring back the
// content it hid. Each edge is driven separately: a check on one side says nothing about the other three.
func TestACropWindowMustLieOnThePage(t *testing.T) {
	src := subsetFixture()
	if _, err := Crop(src, [4]float64{0.1, 0.1, 0.8, 0.8}, []string{"1"}); err != nil {
		t.Fatalf("setup: a window inside the page was refused (%v), so a refusal below proves nothing", err)
	}
	// Float noise at the edge is what a clamped drag produces, and it is accepted.
	if _, err := Crop(src, [4]float64{0.2, 0.2, 0.8 + 1e-12, 0.8 + 1e-12}, []string{"1"}); err != nil {
		t.Errorf("a window ending at the page edge within float noise was refused: %v", err)
	}
	for name, frac := range map[string][4]float64{
		"left":   {-0.1, 0, 0.5, 0.5},
		"top":    {0, -0.1, 0.5, 0.5},
		"right":  {0.6, 0, 0.5, 0.5},
		"bottom": {0, 0.6, 0.5, 0.5},
		// NaN compares false with everything, so a refusal spelled as "x < 0" lets it straight through.
		"NaN x":     {math.NaN(), 0, 0.5, 0.5},
		"NaN width": {0, 0, math.NaN(), 0.5},
	} {
		if _, err := Crop(src, frac, []string{"1"}); err == nil {
			t.Errorf("a window past the page's %s edge (%v) was accepted", name, frac)
		}
	}
}
