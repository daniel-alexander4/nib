package pdfops

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

// Moving a picture (PLAN-image-edit P02.S01, D8): the placement reader, the one conversion and the door.

func moveObject(t *testing.T, pdf []byte, page, ordinal int, box MapRect) ([]byte, ObjectEditResult) {
	t.Helper()
	objs := readObjects(t, pdf, page)
	if !objs.Objects[ordinal].Movable {
		t.Fatalf("picture %d of page %d is not movable: %s", ordinal, page, objs.Objects[ordinal].MoveWhy)
	}
	out, res, err := EditPageObjects(pdf, []ObjectEdit{{Kind: ObjectEditMove, Page: page, Ordinal: ordinal, Digest: objs.Objects[ordinal].Digest, Box: &box}})
	if err != nil {
		t.Fatalf("moving picture %d of page %d: %v", ordinal, page, err)
	}
	return out, res
}

func nearBox(a, b MapRect) bool {
	for i := range a {
		if math.Abs(a[i]-b[i]) > 1e-6 {
			return false
		}
	}
	return true
}

func TestAPlainPictureMovesAndResizesAndNothingElseChanges(t *testing.T) {
	pdf := removeFixture(twoPicturesAndText, nil)
	before := readObjects(t, pdf, 1)
	want := MapRect{0.5, 0.5, 0.75, 0.6}
	out, res := moveObject(t, pdf, 1, 0, want)
	after := readObjects(t, out, 1)
	if len(after.Objects) != 2 {
		t.Fatalf("%d pictures after a move, want 2", len(after.Objects))
	}
	if !nearBox(after.Objects[0].Box, want) {
		t.Fatalf("the moved picture reads at %v, asked %v", after.Objects[0].Box, want)
	}
	if after.Objects[1].operator != before.Objects[1].operator || after.Objects[1].Box != before.Objects[1].Box {
		t.Fatal("the other picture changed")
	}
	if len(res.Moved) != 1 || !nearBox(res.Moved[0].Box, want) || len(res.Removed) != 0 {
		t.Fatalf("answer %+v", res)
	}
	if !strings.Contains(string(pageContentOf(t, out, 1)), "(Hello) Tj") {
		t.Fatal("the text went")
	}
	// The moved picture's address is a new one: its `cm` is in the digest, so the old address names nothing.
	_, _, err := EditPageObjects(out, []ObjectEdit{{Kind: ObjectEditMove, Page: 1, Ordinal: 0, Digest: before.Objects[0].Digest, Box: &want}})
	wantRefusal(t, err, CauseObjectChanged)
}

// TestTheOuterMatrixIsKept: a `cm` before the picture's `q` is the page's; the new `cm` undoes it, so the picture
// lands where asked and the outer `cm` is left as written.
func TestTheOuterMatrixIsKept(t *testing.T) {
	pdf := removeFixture("1 0 0 1 20 30 cm 2 0 0 2 0 0 cm q 50 0 0 40 10 10 cm /Im0 Do Q", nil)
	want := MapRect{0.1, 0.2, 0.3, 0.25}
	out, _ := moveObject(t, pdf, 1, 0, want)
	if got := readObjects(t, out, 1).Objects[0].Box; !nearBox(got, want) {
		t.Fatalf("reads at %v, asked %v", got, want)
	}
	if !strings.HasPrefix(string(pageContentOf(t, out, 1)), "1 0 0 1 20 30 cm 2 0 0 2 0 0 cm q ") {
		t.Fatalf("the outer matrices were touched: %q", pageContentOf(t, out, 1))
	}
}

// A turned outer matrix: the picture's own `cm` turns it back, so it is drawn straight, and the new `cm` must undo
// the turn as well as the shift.
func TestATurnedOuterMatrixIsUndone(t *testing.T) {
	pdf := removeFixture("0 1 -1 0 600 0 cm q 0 -100 80 0 100 500 cm /Im0 Do Q", nil)
	want := MapRect{0.3, 0.4, 0.5, 0.55}
	out, _ := moveObject(t, pdf, 1, 0, want)
	if got := readObjects(t, out, 1).Objects[0].Box; !nearBox(got, want) {
		t.Fatalf("reads at %v, asked %v", got, want)
	}
}

// The conversion against hand arithmetic, not only against point (whose inverse it is, so a mistake made in both
// would pass every round trip): a box from the top-left of the display at (0.2, 0.1)-(0.45, 0.3) on a 600 × 800
// page, at every turn. /Rotate turns the page clockwise for display, so the display's top-left corner is user
// (0, 800) unturned, (0, 0) at 90, (600, 0) at 180 and (600, 800) at 270; from there the display's x and y run along
// user x or y, one way or the other.
func TestTheConversionLandsWhereArithmeticSays(t *testing.T) {
	for _, c := range []struct {
		rotate string
		want   [4]float64
	}{
		{"", [4]float64{120, 560, 270, 720}},            // x 600·[0.2, 0.45]; y 800 − 800·[0.3, 0.1]
		{"/Rotate 90", [4]float64{60, 160, 180, 360}},   // x 600·[0.1, 0.3]; y 800·[0.2, 0.45]
		{"/Rotate 180", [4]float64{330, 80, 480, 240}},  // x 600 − 600·[0.45, 0.2]; y 800·[0.1, 0.3]
		{"/Rotate 270", [4]float64{420, 440, 540, 640}}, // x 600 − 600·[0.3, 0.1]; y 800 − 800·[0.45, 0.2]
	} {
		pdf := removeFixture("q 100 0 0 80 100 100 cm /Im0 Do Q", map[int]string{
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] " + c.rotate + " /Resources 9 0 R /Contents 4 0 R >>",
		})
		out, _ := moveObject(t, pdf, 1, 0, MapRect{0.2, 0.1, 0.45, 0.3})
		if got := readObjects(t, out, 1).Objects[0].userBox; !boxesAgree(got, c.want) {
			t.Errorf("%q: user box %v, want %v", c.rotate, got, c.want)
		}
	}
}

// An inline image moves the way an XObject does.
func TestAnInlineImageMoves(t *testing.T) {
	pdf := removeFixture("q 100 0 0 80 10 10 cm BI /W 2 /H 2 /CS /G /BPC 8 ID abcd EI Q", nil)
	want := MapRect{0.4, 0.4, 0.6, 0.5}
	out, _ := moveObject(t, pdf, 1, 0, want)
	if o := readObjects(t, out, 1).Objects[0]; o.Kind != "inline" || !nearBox(o.Box, want) {
		t.Fatalf("after: %+v", o)
	}
}

// TestTheConversionHoldsAtEveryTurn is the one conversion (`unpoint`) through the door, read back through the
// listing (`point`): every `/Rotate`, a crop box away from the origin, and `/UserUnit` — which no conversion reads,
// because a fraction of the page is the same fraction at any unit size; that case pins that none starts to.
func TestTheConversionHoldsAtEveryTurn(t *testing.T) {
	want := MapRect{0.2, 0.1, 0.45, 0.3}
	for _, page := range []string{
		"/MediaBox [0 0 600 800]",
		"/MediaBox [0 0 600 800] /Rotate 90",
		"/MediaBox [0 0 600 800] /Rotate 180",
		"/MediaBox [0 0 600 800] /Rotate 270",
		"/MediaBox [0 0 600 800] /CropBox [50 60 550 760] /Rotate 90",
		"/MediaBox [0 0 600 800] /UserUnit 2",
	} {
		pdf := removeFixture("q 100 0 0 80 100 100 cm /Im0 Do Q", map[int]string{
			3: "<< /Type /Page /Parent 2 0 R " + page + " /Resources 9 0 R /Contents 4 0 R >>",
		})
		out, _ := moveObject(t, pdf, 1, 0, want)
		if got := readObjects(t, out, 1).Objects[0].Box; !nearBox(got, want) {
			t.Errorf("%s: reads at %v, asked %v", page, got, want)
		}
	}
}

func TestUnpointInvertsPointAtEveryTurn(t *testing.T) {
	for _, rot := range []int{0, 90, 180, 270} {
		sp := displaySpace{box: [4]float64{50, 60, 550, 760}, rot: rot}
		for _, f := range [][2]float64{{0.1, 0.2}, {0.9, 0.05}, {0.5, 0.5}} {
			x, y := sp.unpoint(f[0], f[1])
			if gx, gy := sp.point(x, y); math.Abs(gx-f[0]) > 1e-12 || math.Abs(gy-f[1]) > 1e-12 {
				t.Errorf("rot %d: %v → (%g, %g) → (%g, %g)", rot, f, x, y, gx, gy)
			}
		}
	}
}

func TestEveryPlacementAMoveCannotRewriteIsRefusedByCause(t *testing.T) {
	for name, tc := range map[string]struct{ content, why string }{
		"two cm in the q":             {"q 1 0 0 1 10 10 cm 100 0 0 80 0 0 cm /Im0 Do Q", WhyNotAlone},
		"something else in the q":     {"q 100 0 0 80 10 10 cm /Im0 Do /Im1 Do Q", WhyNotAlone},
		"a clip inside the q":         {"q 0 0 300 300 re W n 100 0 0 80 10 10 cm /Im0 Do Q", WhyNotAlone},
		"no cm in the q":              {"100 0 0 80 10 10 cm q /Im0 Do Q", WhyNotAlone},
		"a comment in the bracket":    {"q 100 0 0 80 10 10 cm %x\n/Im0 Do Q", WhyNotAlone},
		"no q at all":                 {"100 0 0 80 10 10 cm /Im0 Do", WhyNotIsolated},
		"turned":                      {"q 0 80 -100 0 110 10 cm /Im0 Do Q", WhyNotUpright},
		"mirrored":                    {"q -100 0 0 80 110 10 cm /Im0 Do Q", WhyNotUpright},
		"slanted":                     {"q 100 0 20 80 10 10 cm /Im0 Do Q", WhyNotUpright},
		"drawn by a form":             {"q /Fm0 Do Q", WhyFormDrawn},
		"partly off the page":         {"q 100 0 0 80 550 10 cm /Im0 Do Q", WhyPartlyOffPage},
		"off the left edge":           {"q 100 0 0 80 -50 10 cm /Im0 Do Q", WhyPartlyOffPage},
		"off the top edge":            {"q 100 0 0 80 10 750 cm /Im0 Do Q", WhyPartlyOffPage},
		"off the bottom edge":         {"q 100 0 0 80 10 -40 cm /Im0 Do Q", WhyPartlyOffPage},
		"a path drawn after it":       {"q 100 0 0 80 10 10 cm /Im0 Do 0 0 m 10 10 l S Q", WhyNotAlone},
		"cut by the clip it is under": {"0 0 300 300 re W n q 100 0 0 80 250 10 cm /Im0 Do Q", WhyClipped},
	} {
		t.Run(name, func(t *testing.T) {
			pdf := removeFixture(tc.content, nil)
			o := readObjects(t, pdf, 1).Objects[0]
			if o.Movable || o.MoveWhy != tc.why {
				t.Fatalf("listed movable %v, %q; want %q", o.Movable, o.MoveWhy, tc.why)
			}
			box := MapRect{0.5, 0.5, 0.6, 0.6}
			out, _, err := EditPageObjects(pdf, []ObjectEdit{{Kind: ObjectEditMove, Page: 1, Ordinal: 0, Digest: o.Digest, Box: &box}})
			if out != nil {
				t.Fatal("bytes were returned")
			}
			wantRefusal(t, err, tc.why)
		})
	}
}

// A picture under a clip that hides part of the page is not movable even where it sits wholly inside the clip, and
// one under a clip that leaves the page open is.
func TestOnlyAClipThatLeavesThePageOpenLetsAPictureMove(t *testing.T) {
	inside := readObjects(t, removeFixture("0 0 300 300 re W n q 100 0 0 80 10 10 cm /Im0 Do Q", nil), 1).Objects[0]
	if inside.Movable || inside.MoveWhy != WhyClipped {
		t.Fatalf("under a 300 x 300 clip: movable %v, %q", inside.Movable, inside.MoveWhy)
	}
	open := readObjects(t, removeFixture("0 0 600 800 re W n q 100 0 0 80 10 10 cm /Im0 Do Q", nil), 1).Objects[0]
	if !open.Movable {
		t.Fatalf("under a clip the size of the page: %s", open.MoveWhy)
	}
}

// A clip whose box covers the page but which is not that box hides part of the page all the same: a triangle, or
// text set to clip. Neither is read as a box, so neither lets a picture move (code-review v2.56.1 P02.S01 #2).
func TestAClipThatIsNotOneStraightRectangleStopsAMove(t *testing.T) {
	for name, content := range map[string]string{
		"a triangle":             "0 0 m 600 0 l 0 800 l h W n q 100 0 0 80 10 10 cm /Im0 Do Q",
		"two rectangles":         "0 0 300 800 re 300 0 300 800 re W n q 100 0 0 80 10 10 cm /Im0 Do Q",
		"a rectangle and a line": "0 0 600 800 re 0 0 m 600 800 l h W* n q 100 0 0 80 10 10 cm /Im0 Do Q",
		"text that clips":        "BT 7 Tr /F1 12 Tf 10 700 Td (Hi) Tj ET q 100 0 0 80 10 10 cm /Im0 Do Q",
		"a turned rectangle":     "q 0.7071 0.7071 -0.7071 0.7071 0 0 cm 0 -600 1200 1200 re W n 0.7071 -0.7071 0.7071 0.7071 0 0 cm q 100 0 0 80 10 10 cm /Im0 Do Q Q",
	} {
		o := readObjects(t, removeFixture(content, nil), 1).Objects[0]
		if o.Movable || o.MoveWhy != WhyClipped {
			t.Errorf("%s: movable %v, %q", name, o.Movable, o.MoveWhy)
		}
	}
}

// A soft mask hides part of the page as a clip does, so a picture drawn under one is not movable; a graphics state
// with no mask, a mask of /None, and a mask the `Q` has taken away again leave it movable (P02 phase close, W11).
// A page-sized clip that rounding left a hair inside the page still leaves it open; one a point inside does not.
func TestAClipARoundingInsideThePageLeavesItOpen(t *testing.T) {
	for _, c := range []struct {
		name, clip string
		movable    bool
	}{
		{"a hair inside", "0 0 599.9999999 800 re W n", true},
		{"through a scaled cm", "q 0.1 0 0 0.1 0 0 cm 0 0 5999.999999 8000 re W n", true},
		{"a point inside", "0 0 599 800 re W n", false},
	} {
		o := readObjects(t, removeFixture(c.clip+" q 100 0 0 80 10 10 cm /Im0 Do Q", nil), 1).Objects[0]
		if o.Movable != c.movable {
			t.Errorf("%s: movable %v (%q), want %v", c.name, o.Movable, o.MoveWhy, c.movable)
		}
	}
}

func TestASoftMaskStopsAMoveAsAClipDoes(t *testing.T) {
	res := "<< /Font << /F1 5 0 R >> /XObject << /Im0 6 0 R /Im1 8 0 R /Fm0 7 0 R >> /ExtGState << " +
		"/GSm << /SMask << /Type /Mask /S /Luminosity /G 7 0 R >> >> /GSn << /SMask /None >> /GSa << /ca 0.5 >> >> >>"
	for _, c := range []struct {
		name, content string
		movable       bool
	}{
		{"a soft mask", "/GSm gs q 100 0 0 80 10 10 cm /Im0 Do Q", false},
		{"a soft mask of /None", "/GSn gs q 100 0 0 80 10 10 cm /Im0 Do Q", true},
		{"an alpha and no mask", "/GSa gs q 100 0 0 80 10 10 cm /Im0 Do Q", true},
		{"a soft mask the Q took back", "q /GSm gs Q q 100 0 0 80 10 10 cm /Im0 Do Q", true},
	} {
		o := readObjects(t, removeFixture(c.content, map[int]string{9: res}), 1).Objects[0]
		if o.Movable != c.movable || (!c.movable && o.MoveWhy != WhyClipped) {
			t.Errorf("%s: movable %v (%q), want %v", c.name, o.Movable, o.MoveWhy, c.movable)
		}
	}
}

func TestAMoveRequestIsReadStrictly(t *testing.T) {
	pdf := removeFixture(twoPicturesAndText, nil)
	d := readObjects(t, pdf, 1).Objects[0].Digest
	ok := MapRect{0.1, 0.1, 0.2, 0.2}
	for name, e := range map[string]ObjectEdit{
		"a move with no box":       {Kind: ObjectEditMove, Page: 1, Ordinal: 0, Digest: d},
		"a removal with a box":     {Kind: ObjectEditDelete, Page: 1, Ordinal: 0, Digest: d, Box: &ok},
		"a box off the page":       {Kind: ObjectEditMove, Page: 1, Ordinal: 0, Digest: d, Box: &MapRect{-0.1, 0.1, 0.2, 0.2}},
		"a box with no width":      {Kind: ObjectEditMove, Page: 1, Ordinal: 0, Digest: d, Box: &MapRect{0.2, 0.1, 0.2, 0.2}},
		"a box that is not number": {Kind: ObjectEditMove, Page: 1, Ordinal: 0, Digest: d, Box: &MapRect{math.NaN(), 0.1, 0.2, 0.2}},
	} {
		if _, _, err := EditPageObjects(pdf, []ObjectEdit{e}); err == nil {
			t.Errorf("%s: accepted", name)
		} else {
			wantRefusal(t, err, CauseBadRequest)
		}
	}
}

// A move writes no structure: a tagged Figure keeps its element and its description, and the tree reads as before.
func TestATaggedFigureMovesAndKeepsItsElement(t *testing.T) {
	pdf := objectFigureFixture(figureThenText, "0", "")
	before, err := ReadStructure(pdf)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := moveObject(t, pdf, 1, 0, MapRect{0.4, 0.4, 0.6, 0.5})
	after, err := ReadStructure(out)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.Elements, after.Elements) {
		t.Fatalf("the tree moved with the picture:\nbefore %+v\nafter  %+v", before.Elements, after.Elements)
	}
	if o := readObjects(t, out, 1).Objects[0]; o.Alt != "A cat on a mat" {
		t.Fatalf("the description reads %q after the move", o.Alt)
	}
}

// A move and a removal on one page land in one rewrite, and the after-check holds both.
func TestAMoveAndARemovalTogether(t *testing.T) {
	pdf := removeFixture(twoPicturesAndText, nil)
	objs := readObjects(t, pdf, 1)
	box := MapRect{0.6, 0.6, 0.7, 0.7}
	out, res, err := EditPageObjects(pdf, []ObjectEdit{
		{Kind: ObjectEditDelete, Page: 1, Ordinal: 0, Digest: objs.Objects[0].Digest},
		{Kind: ObjectEditMove, Page: 1, Ordinal: 1, Digest: objs.Objects[1].Digest, Box: &box},
	})
	if err != nil {
		t.Fatal(err)
	}
	after := readObjects(t, out, 1)
	if len(after.Objects) != 1 || !nearBox(after.Objects[0].Box, box) || len(res.Moved) != 1 || !nearBox(res.Moved[0].Box, box) {
		t.Fatalf("after %+v, answer %+v", after.Objects, res)
	}
	if after.Objects[0].painter != objs.Objects[1].painter {
		t.Fatal("the picture left is not the one that was moved")
	}
	// The other order: the moved picture comes first, so no removal stands before it.
	out, res, err = EditPageObjects(pdf, []ObjectEdit{
		{Kind: ObjectEditMove, Page: 1, Ordinal: 0, Digest: objs.Objects[0].Digest, Box: &box},
		{Kind: ObjectEditDelete, Page: 1, Ordinal: 1, Digest: objs.Objects[1].Digest},
	})
	if err != nil {
		t.Fatal(err)
	}
	after = readObjects(t, out, 1)
	if len(after.Objects) != 1 || after.Objects[0].painter != objs.Objects[0].painter || !nearBox(res.Moved[0].Box, box) {
		t.Fatalf("after %+v, answer %+v", after.Objects, res)
	}
}

// The after-check for a moved picture on its own: where it was asked, drawn straight, by the same operator bytes —
// each refused alone, because a turned matrix or another picture can fill the same box.
func TestAMovedPictureReadingElsewhereIsRefused(t *testing.T) {
	box := [4]float64{10, 10, 110, 90}
	paint := [32]byte{1}
	plan := pageRemoval{page: 1, want: []string{"", "b"}, moved: map[int]movedWant{0: {box: box, painter: paint}}}
	good := []PageObject{{operator: "x", userBox: box, plain: true, painter: paint}, {operator: "b"}}
	if err := rewriteAgrees(plan, good, nil); err != nil {
		t.Fatalf("the page as planned is refused: %v", err)
	}
	for name, edit := range map[string]func(o *PageObject){
		"somewhere else":    func(o *PageObject) { o.userBox[3] = 91 },
		"turned or flipped": func(o *PageObject) { o.plain = false },
		"another picture":   func(o *PageObject) { o.painter = [32]byte{2} },
	} {
		t.Run(name, func(t *testing.T) {
			got := append([]PageObject(nil), good...)
			edit(&got[0])
			wantRefusal(t, rewriteAgrees(plan, got, nil), CauseRewriteDiffers)
		})
	}
}
