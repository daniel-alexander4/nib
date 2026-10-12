package pdfops

import (
	"math"
	"strings"
	"testing"

	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// TestAnElementIsPlacedOnThePageAsDisplayed — /pending 653 part 4. The Tags panel outlined an element from its
// user-space box and the page's MediaBox, over a page the viewer draws from the CROP box turned by `/Rotate`:
// on a cropped or turned page the outline pointed somewhere else. `Place` is the box through the page map's
// one conversion (ADR-088), for a proposal and for an existing tree alike.
//
// Two oracles. The arithmetic, written out here for each page shape. And the page map itself: the text the
// map reads on the displayed page must sit inside the outline of the element that draws it.
func TestAnElementIsPlacedOnThePageAsDisplayed(t *testing.T) {
	src, err := testpdf.Text("section 1 opening paragraph")
	if err != nil {
		t.Fatal(err)
	}
	pr, err := ProposeTags(src)
	if err != nil || len(pr.Elements) != 1 {
		t.Fatalf("setup: want one proposed element, got %+v (err %v)", pr.Elements, err)
	}
	tagged, err := CommitTags(src, reviewAll(pr))
	if err != nil {
		t.Fatal(err)
	}
	r := pr.Elements[0].Rect
	var mb [4]float64
	if mb[0], mb[1], mb[2], mb[3], err = PageBox(src, 1); err != nil {
		t.Fatal(err)
	}
	// A crop box that holds the text, with no corner at the media box's: every edge moves.
	crop := [4]float64{r[0] - 20, r[1] - 40, r[2] + 60, r[3] + 30}
	if !(crop[0] > mb[0] && crop[1] > mb[1] && crop[2] < mb[2] && crop[3] < mb[3]) {
		t.Fatalf("setup: the crop box %v is not inside the media box %v", crop, mb)
	}
	cases := []struct {
		name    string
		cropped bool
		rot     int
	}{
		{"as written", false, 0},
		{"a smaller CropBox", true, 0},
		{"Rotate 90", false, 90},
		{"a smaller CropBox and Rotate 90", true, 90},
		{"a smaller CropBox and Rotate 270", true, 270},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			shape := func(pdf []byte) []byte {
				return mustMutate(t, pdf, func(ctx *model.Context) error {
					d, _, _, err := ctx.PageDict(1, false)
					if err != nil {
						return err
					}
					if c.cropped {
						d["CropBox"] = types.NewNumberArray(crop[0], crop[1], crop[2], crop[3])
					}
					if c.rot != 0 {
						d["Rotate"] = types.Integer(c.rot)
					}
					return nil
				})
			}
			vis := mb
			if c.cropped {
				vis = crop
			}
			w, h := vis[2]-vis[0], vis[3]-vis[1]
			want := func(b [4]float64) [4]float64 {
				u0, u1, v0, v1 := (b[0]-vis[0])/w, (b[2]-vis[0])/w, (b[1]-vis[1])/h, (b[3]-vis[1])/h
				switch c.rot {
				case 90:
					return [4]float64{v0, u0, v1, u1}
				case 270:
					return [4]float64{1 - v1, 1 - u1, 1 - v0, 1 - u0}
				}
				return [4]float64{u0, 1 - v1, u1, 1 - v0}
			}
			check := func(what string, pdf []byte, rect, place [4]float64) {
				t.Helper()
				exp := want(rect)
				for i := range exp {
					if math.Abs(place[i]-exp[i]) > 1e-9 {
						t.Fatalf("%s: the element at %v is placed at %v on the displayed page, want %v", what, rect, place, exp)
					}
				}
				m, err := MapPage(pdf, 1)
				if err != nil {
					t.Fatal(err)
				}
				seen := false
				for _, run := range m.Text {
					if !strings.Contains(run.Text, "section") {
						continue
					}
					seen = true
					x, y := (run.Rect[0]+run.Rect[2])/2, (run.Rect[1]+run.Rect[3])/2
					if !(x > place[0] && x < place[2] && y > place[1] && y < place[3]) {
						t.Errorf("%s: the page map reads the element's text at %v, outside its outline %v", what, run.Rect, place)
					}
				}
				if !seen {
					t.Fatalf("%s: setup — the page map reads no run of the element's text", what)
				}
			}
			untagged := shape(src)
			got, err := ProposeTags(untagged)
			if err != nil || len(got.Elements) != 1 {
				t.Fatalf("proposing: %+v (err %v)", got.Elements, err)
			}
			check("the proposal", untagged, got.Elements[0].Rect, got.Elements[0].Place)

			withTree := shape(tagged)
			tree, err := ReadStructure(withTree)
			if err != nil {
				t.Fatal(err)
			}
			placed := 0
			for _, e := range tree.Elements {
				if e.Rect == ([4]float64{}) {
					if e.Place != ([4]float64{}) {
						t.Errorf("the tree: a %s with no box is placed at %v", e.Standard, e.Place)
					}
					continue
				}
				placed++
				check("the tree's "+e.Standard, withTree, e.Rect, e.Place)
			}
			if placed == 0 {
				t.Fatal("setup: no element of the tree carries a box")
			}
		})
	}
}
