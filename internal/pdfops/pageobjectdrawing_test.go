package pdfops

import (
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// Drawings as page objects (PLAN-image-edit P03.S03): listed beside the pictures through the one grouping
// (`groupDrawnPaths`), numbered on their own, removed whole and moved by their own operands through the one door.

// drawingsContent draws, in order: a picture, a ruled grid of four touching rules, text, a lone curve and a lone rule.
const drawingsContent = "q 100 0 0 80 10 10 cm /Im0 Do Q\n" +
	"100 600 m 300 600 l S\n100 650 m 300 650 l S\n100 600 m 100 650 l S\n300 600 m 300 650 l S\n" +
	"BT /F1 12 Tf 10 700 Td (Hello) Tj ET\n" +
	"400 300 m 420 360 480 360 500 300 c f\n" +
	"50 100 m 250 100 l S\n"

func drawingKinds(objs PageObjects) []string {
	var out []string
	for _, d := range objs.Drawings {
		state := "ok"
		if !d.Editable {
			state = d.Why
		}
		move := "ok"
		if !d.Movable {
			move = d.MoveWhy
		}
		out = append(out, d.Kind+" "+state+" "+move)
	}
	return out
}

func TestDrawingsAreListedBesideThePicturesAndNumberedApart(t *testing.T) {
	objs := readObjects(t, removeFixture(drawingsContent, nil), 1)
	if len(objs.Objects) != 1 || objs.Objects[0].Object != ObjectPicture || objs.Objects[0].Ordinal != 0 {
		t.Fatalf("the pictures read %+v, want the one picture as picture 0", objs.Objects)
	}
	want := []string{"drawing ok ok", "drawing ok ok", "rule ok ok"}
	if got := drawingKinds(objs); !reflect.DeepEqual(got, want) || objs.DrawingsUngrouped {
		t.Fatalf("the drawings read %v (ungrouped %v), want %v: the grid, the curve and the rule, in the order drawn", got, objs.DrawingsUngrouped, want)
	}
	seen := map[string]bool{objs.Objects[0].Digest: true}
	for i, d := range objs.Drawings {
		if d.Object != ObjectDrawing || d.Ordinal != i || seen[d.Digest] || len(d.Digest) != 64 {
			t.Errorf("drawing %d reads object %q ordinal %d digest %q; want its own ordinal and a digest no other object has", i, d.Object, d.Ordinal, d.Digest)
		}
		seen[d.Digest] = true
	}
	if n := len(objs.Drawings[0].members); n != 4 {
		t.Errorf("the grid holds %d paths, want its four rules", n)
	}
	// The grid's box is its four rules', on the displayed page (600 by 800), a stroke's half width past them.
	if b := objs.Drawings[0].Box; math.Abs(b[0]-99.5/600) > 1e-9 || math.Abs(b[1]-(1-650.5/800)) > 1e-9 {
		t.Errorf("the grid is listed at %v", b)
	}
}

// moveBy is box shifted by (dx, dy) in displayed-page fractions.
func moveBy(b MapRect, dx, dy float64) *MapRect {
	return &MapRect{b[0] + dx, b[1] + dy, b[2] + dx, b[3] + dy}
}

func TestADrawingIsRemovedWholeAndNothingElseGoes(t *testing.T) {
	src := removeFixture(drawingsContent, nil)
	before := readObjects(t, src, 1)
	grid := before.Drawings[0]
	out, res, err := EditPageObjects(src, []ObjectEdit{{Kind: ObjectEditDelete, Object: ObjectDrawing, Page: 1, Ordinal: 0, Digest: grid.Digest}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Removed) != 1 || res.Removed[0].Object != ObjectDrawing || res.Removed[0].Kind != "drawing" {
		t.Errorf("the answer reads %+v", res.Removed)
	}
	after := readObjects(t, out, 1)
	if got := drawingKinds(after); !reflect.DeepEqual(got, []string{"drawing ok ok", "rule ok ok"}) {
		t.Errorf("after removing the grid the drawings read %v, want the curve and the rule", got)
	}
	if len(after.Objects) != 1 || after.Objects[0].operator != before.Objects[0].operator {
		t.Error("the picture changed when a drawing was removed")
	}
	if after.Drawings[0].operator != before.Drawings[1].operator || after.Drawings[1].operator != before.Drawings[2].operator {
		t.Error("a drawing that was not removed changed")
	}
	if page := string(pageContentOf(t, out, 1)); strings.Contains(page, "100 600 m") || strings.Contains(page, "300 650 l") || !strings.Contains(page, "(Hello) Tj") {
		t.Errorf("the page still draws the grid, or lost its text:\n%s", page)
	}
	// The address is now stale: its ordinal names the curve, whose digest is not the grid's.
	if _, _, err := EditPageObjects(out, []ObjectEdit{{Kind: ObjectEditDelete, Object: ObjectDrawing, Page: 1, Ordinal: 0, Digest: grid.Digest}}); !isCause(err, CauseObjectChanged) {
		t.Errorf("a replayed removal was answered %v", err)
	}
}

func TestADrawingIsMovedByItsOwnOperands(t *testing.T) {
	for _, c := range []struct {
		name, content string
	}{
		{"drawn in page space", drawingsContent},
		// Under a matrix that scales and turns it, the operands move by the translation undone through that matrix.
		{"under a scaling cm", "q 2 0 0 2 0 0 cm 200 150 m 210 180 240 180 250 150 c f Q\n"},
		{"under a turning cm", "q 0 1 -1 0 600 0 cm 200 150 m 210 180 240 180 250 150 c f Q\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := removeFixture(c.content, nil)
			before := readObjects(t, src, 1)
			k := len(before.Drawings) - 1
			if c.content == drawingsContent {
				k = 1 // the curve
			}
			d := before.Drawings[k]
			if !d.Movable {
				t.Fatalf("setup: the drawing is not movable: %s", d.MoveWhy)
			}
			target := moveBy(d.Box, -0.05, 0.1)
			out, res, err := EditPageObjects(src, []ObjectEdit{{Kind: ObjectEditMove, Object: ObjectDrawing, Page: 1, Ordinal: k, Digest: d.Digest, Box: target}})
			if err != nil {
				t.Fatal(err)
			}
			after := readObjects(t, out, 1)
			if len(after.Drawings) != len(before.Drawings) {
				t.Fatalf("%d drawings after the move, %d before", len(after.Drawings), len(before.Drawings))
			}
			got := after.Drawings[k].Box
			for i := range got {
				if math.Abs(got[i]-target[i]) > 1e-6 {
					t.Fatalf("the drawing reads at %v, asked %v", got, *target)
				}
			}
			if len(res.Moved) != 1 || res.Moved[0].Object != ObjectDrawing || math.Abs(res.Moved[0].Box[0]-target[0]) > 1e-6 {
				t.Errorf("the answer reads %+v", res.Moved)
			}
			for i, o := range before.Drawings {
				if i != k && after.Drawings[i].operator != o.operator {
					t.Errorf("drawing %d changed when drawing %d moved", i, k)
				}
			}
			for i, o := range before.Objects {
				if after.Objects[i].operator != o.operator {
					t.Errorf("picture %d changed when a drawing moved", i)
				}
			}
		})
	}
}

func TestADrawingIsRefusedByItsCause(t *testing.T) {
	pathForm := streamObject("/Type /XObject /Subtype /Form /BBox [0 0 600 800]", "100 100 m 200 100 l 200 200 l S")
	for _, c := range []struct {
		name, content  string
		over           map[int]string
		why, moveWhy   string
		tagged, inForm bool
	}{
		{"drawn by a form", "/Fm0 Do\n", map[int]string{7: pathForm}, WhyFormDrawn, WhyFormDrawn, false, true},
		{"tagged", "/P <</MCID 0>> BDC 100 100 m 200 100 l 200 200 l S EMC\n", nil, WhyTagged, "", true, false},
		{"filled with a pattern", "/Pattern cs /P0 scn 100 100 m 200 100 l 200 200 l f\n", nil, "", WhyPatterned, false, false},
		{"stroked with a pattern", "/Pattern CS /P0 SCN 100 100 m 200 100 l 200 200 l S\n", nil, "", WhyPatterned, false, false},
		{"a pattern no longer in force", "/Pattern cs /P0 scn 0 g 100 100 m 200 100 l 200 200 l f\n", nil, "", "", false, false},
		{"a fill pattern under a stroke", "/Pattern cs /P0 scn 100 100 m 200 100 l 200 200 l S\n", nil, "", "", false, false},
		{"a clip as well as painted", "100 100 100 100 re W f\n", nil, WhyClips, WhyClips, false, false},
		{"under a clip", "q 0 0 300 300 re W n 100 100 m 200 100 l 200 200 l S Q\n", nil, "", WhyClipped, false, false},
		{"partly off the page", "500 100 m 700 100 l 700 200 l S\n", nil, "", WhyPartlyOffPage, false, false},
		// A colour set between a path's points is drawn by viewers; taking its bytes would recolour what follows.
		{"holding a colour change", "100 100 m 200 100 l 1 0 0 RG 200 200 l S\n", nil, WhyNotPathOnly, WhyNotPathOnly, false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			objs := readObjects(t, removeFixture(c.content, c.over), 1)
			if len(objs.Drawings) != 1 {
				t.Fatalf("%d drawings listed, want one", len(objs.Drawings))
			}
			d := objs.Drawings[0]
			if (d.Why != c.why) || (d.Editable != (c.why == "")) || d.MoveWhy != c.moveWhy || d.Movable != (c.moveWhy == "") ||
				d.Tagged != c.tagged || d.InForm != c.inForm || d.Replaceable || d.ReplaceWhy != WhyNotAPicture {
				t.Errorf("listed removable %v (%q), movable %v (%q), tagged %v, in a form %v, replaceable %v (%q); want removal %q, move %q",
					d.Editable, d.Why, d.Movable, d.MoveWhy, d.Tagged, d.InForm, d.Replaceable, d.ReplaceWhy, c.why, c.moveWhy)
			}
			src := removeFixture(c.content, c.over)
			_, _, err := EditPageObjects(src, []ObjectEdit{{Kind: ObjectEditDelete, Object: ObjectDrawing, Page: 1, Ordinal: 0, Digest: d.Digest}})
			if c.why != "" && !isCause(err, c.why) || c.why == "" && err != nil {
				t.Errorf("a removal was answered %v, want %q", err, c.why)
			}
			_, _, err = EditPageObjects(src, []ObjectEdit{{Kind: ObjectEditMove, Object: ObjectDrawing, Page: 1, Ordinal: 0, Digest: d.Digest, Box: moveBy(d.Box, -0.01, 0.01)}})
			if c.moveWhy != "" && !isCause(err, c.moveWhy) || c.moveWhy == "" && err != nil {
				t.Errorf("a move was answered %v, want %q", err, c.moveWhy)
			}
		})
	}
}

func TestADrawingRequestIsReadForWhatADrawingTakes(t *testing.T) {
	src := removeFixture(drawingsContent, nil)
	objs := readObjects(t, src, 1)
	curve := objs.Drawings[1]
	grown := curve.Box
	grown[2] += 0.05
	for _, c := range []struct {
		name string
		e    ObjectEdit
		want string
	}{
		{"resized", ObjectEdit{Kind: ObjectEditMove, Object: ObjectDrawing, Page: 1, Ordinal: 1, Digest: curve.Digest, Box: &grown}, CauseBadRequest},
		{"replaced", ObjectEdit{Kind: ObjectEditReplace, Object: ObjectDrawing, Page: 1, Ordinal: 1, Digest: curve.Digest, With: []byte("x")}, WhyNotAPicture},
		{"neither a picture nor a drawing", ObjectEdit{Kind: ObjectEditDelete, Object: "text", Page: 1, Ordinal: 1, Digest: curve.Digest}, CauseBadRequest},
		{"a drawing's address as a picture's", ObjectEdit{Kind: ObjectEditDelete, Page: 1, Ordinal: 0, Digest: objs.Drawings[0].Digest}, CauseObjectChanged},
	} {
		if _, _, err := EditPageObjects(src, []ObjectEdit{c.e}); !isCause(err, c.want) {
			t.Errorf("%s: answered %v, want %q", c.name, err, c.want)
		}
	}
	// Picture 0 and drawing 0 are two addresses: one request removes both.
	out, res, err := EditPageObjects(src, []ObjectEdit{
		{Kind: ObjectEditDelete, Page: 1, Ordinal: 0, Digest: objs.Objects[0].Digest},
		{Kind: ObjectEditDelete, Object: ObjectDrawing, Page: 1, Ordinal: 0, Digest: objs.Drawings[0].Digest},
	})
	if err != nil || len(res.Removed) != 2 || res.Removed[0].Object != ObjectPicture || res.Removed[1].Object != ObjectDrawing {
		t.Fatalf("picture 0 and drawing 0 in one request: %+v, %v", res.Removed, err)
	}
	if after := readObjects(t, out, 1); len(after.Objects) != 0 || len(after.Drawings) != 2 {
		t.Errorf("after both went the page lists %d pictures and %d drawings", len(after.Objects), len(after.Drawings))
	}
}

// Two drawings drawn by the same bytes are twins (ADR-220, restated): removing one makes the other's address stale.
func TestTwinDrawingsCountEachOther(t *testing.T) {
	// The same rule, bare and as decoration: two standings, so two drawings, drawn by the same bytes.
	rule := "50 100 m 250 100 l S\n"
	src := removeFixture(rule+"/Artifact BMC "+rule+"EMC\n", nil)
	objs := readObjects(t, src, 1)
	if len(objs.Drawings) != 2 || objs.Drawings[0].operator != objs.Drawings[1].operator || objs.Drawings[0].Digest != objs.Drawings[1].Digest {
		t.Fatalf("two identical rules list as %+v", objs.Drawings)
	}
	out, _, err := EditPageObjects(src, []ObjectEdit{{Kind: ObjectEditDelete, Object: ObjectDrawing, Page: 1, Ordinal: 1, Digest: objs.Drawings[1].Digest}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := EditPageObjects(out, []ObjectEdit{{Kind: ObjectEditDelete, Object: ObjectDrawing, Page: 1, Ordinal: 0, Digest: objs.Drawings[0].Digest}}); !isCause(err, CauseObjectChanged) {
		t.Errorf("the survivor's old address was answered %v, want object-changed", err)
	}
}

// The after-check holds every path the edit did not touch: a plan whose record of the page's paths does not match
// what was written is refused, and so is a moved path found anywhere but where its translation put it.
func TestTheAfterCheckHoldsThePathsTheEditDidNotTouch(t *testing.T) {
	a := drawnPath{bytes: []byte("1 1 m 2 2 l S"), ops: 2, box: [4]float64{1, 1, 2, 2}}
	b := drawnPath{bytes: []byte("5 5 m 6 6 l S"), ops: 2, box: [4]float64{5, 5, 6, 6}}
	plan := pageRemoval{page: 1, paths: pathsAsTheyStand([]drawnPath{a, b})}
	if err := pathsAgree(plan, []drawnPath{a, b}); err != nil {
		t.Errorf("an untouched page: %v", err)
	}
	for name, got := range map[string][]drawnPath{"a path gone": {a}, "a path changed": {a, a}, "an extra path": {a, b, b}} {
		if err := pathsAgree(plan, got); !isCause(err, CauseRewriteDiffers) {
			t.Errorf("%s: %v", name, err)
		}
	}
	moved := b
	moved.box = [4]float64{15, 5, 16, 6}
	plan.paths[1] = pathWant{bytes: plan.paths[1].bytes, moved: true, ops: 2, box: moved.box}
	if err := pathsAgree(plan, []drawnPath{a, moved}); err != nil {
		t.Errorf("a path where its move put it: %v", err)
	}
	if err := pathsAgree(plan, []drawnPath{a, b}); !isCause(err, CauseRewriteDiffers) {
		t.Errorf("a moved path still where it was: %v", err)
	}
	// A page the edit left with no path is held to none: an empty record is a record.
	if err := pathsAgree(pageRemoval{page: 1, paths: []pathWant{}}, []drawnPath{a}); !isCause(err, CauseRewriteDiffers) {
		t.Errorf("a path on a page that should draw none: %v", err)
	}
}

// The moved picture's answer counts pictures only: a drawing removed in the same request shifts no picture
// (code-review of P03.S03: drawing ordinals were counted as pictures').
func TestAMovedPicturesAnswerIgnoresARemovedDrawing(t *testing.T) {
	src := removeFixture(twoPicturesAndText+"\n400 300 m 420 360 480 360 500 300 c f\n", nil)
	objs := readObjects(t, src, 1)
	pic := objs.Objects[1]
	target := moveBy(pic.Box, 0.05, 0.05)
	out, res, err := EditPageObjects(src, []ObjectEdit{
		{Kind: ObjectEditDelete, Object: ObjectDrawing, Page: 1, Ordinal: 0, Digest: objs.Drawings[0].Digest},
		{Kind: ObjectEditMove, Page: 1, Ordinal: 1, Digest: pic.Digest, Box: target},
	})
	if err != nil {
		t.Fatal(err)
	}
	after := readObjects(t, out, 1)
	if len(res.Moved) != 1 || res.Moved[0].Box != after.Objects[1].Box || math.Abs(res.Moved[0].Box[0]-target[0]) > 1e-6 {
		t.Errorf("the move answers %+v; picture 2 reads at %v", res.Moved, after.Objects[1].Box)
	}
}

// A form drawn twice draws the same bytes at the same offsets twice: the listing names each path by where the walk
// met it, so the two are two members, never one path counted twice (code-review of P03.S03).
func TestAFormDrawnTwiceListsEachOfItsPaths(t *testing.T) {
	pathForm := streamObject("/Type /XObject /Subtype /Form /BBox [0 0 600 800]", "100 100 m 200 100 l 200 200 l S")
	objs := readObjects(t, removeFixture("/Fm0 Do\nq 1 0 0 1 50 0 cm /Fm0 Do Q\n", map[int]string{7: pathForm}), 1)
	if len(objs.Drawings) != 1 {
		t.Fatalf("%d drawings, want the two touching copies as one", len(objs.Drawings))
	}
	d := objs.Drawings[0]
	if len(d.memberAt) != 2 || d.memberAt[0] == d.memberAt[1] || d.members[0].ctm == d.members[1].ctm {
		t.Errorf("the drawing's members are %v under %v and %v: one path counted twice", d.memberAt, d.members[0].ctm, d.members[len(d.members)-1].ctm)
	}
}

func TestTranslatedPathRewritesOnlyItsPoints(t *testing.T) {
	p := drawnPath{bytes: []byte("10 20 m 30 40 l 1 2 3 4 5 6 c 7 8 9 10 v 11 12 13 14 y 0 0 5 5 re h W f"), ctm: runIdentity}
	got, ok := translatedPath(p, 1, -1)
	if want := "11 19 m 31 39 l 2 1 4 3 6 5 c 8 7 10 9 v 12 11 14 13 y 1 -1 5 5 re h W f"; !ok || string(got) != want {
		t.Errorf("translated reads %q (%v), want %q", got, ok, want)
	}
	p.ctm = runMatrix{2, 0, 0, 4, 100, 100} // a page step of (2, 4) is (1, 1) in the path's own space
	if got, ok := translatedPath(drawnPath{bytes: []byte("10 10 m 20 20 l S"), ctm: p.ctm}, 2, 4); !ok || string(got) != "11 11 m 21 21 l S" {
		t.Errorf("under a scale it reads %q (%v)", got, ok)
	}
	for _, bad := range []string{"10 10 m q 20 20 l S", "10 m S", "/X 10 m S", "10 10 m 20 20 l (s) S", "1e5 10 m 20 20 l S", "10 10 m 1 0 0 RG 20 20 l S"} {
		if _, ok := translatedPath(drawnPath{bytes: []byte(bad), ctm: runIdentity}, 1, 1); ok {
			t.Errorf("%q was rewritten", bad)
		}
	}
	if _, ok := translatedPath(drawnPath{bytes: []byte("1 1 m S"), ctm: runMatrix{1, 1, 1, 1, 0, 0}}, 1, 1); ok {
		t.Error("a path under a singular matrix was rewritten")
	}
}

// TestThePathGroupingHasTwoCallers is the census for P03.S03 T01's one door: `groupDrawnPaths` is called by the
// untagged listing and the objects listing, and by nothing else in the package, so a drawing an edit names is the
// drawing the region reader lists.
func TestThePathGroupingHasTwoCallers(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var callers []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "groupDrawnPaths" {
						callers = append(callers, f+":"+fn.Name.Name)
					}
				}
				return true
			})
		}
	}
	sort.Strings(callers)
	if want := []string{"pageobjects.go:drawingsOf", "structregion.go:untaggedPieces"}; !reflect.DeepEqual(callers, want) {
		t.Errorf("groupDrawnPaths is called from %v, want exactly %v", callers, want)
	}
}

// isCause says err is an object refusal with cause.
func isCause(err error, cause string) bool {
	ref, ok := err.(*ObjectRefusal)
	return ok && ref.Cause == cause
}
