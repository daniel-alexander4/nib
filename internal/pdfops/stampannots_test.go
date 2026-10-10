package pdfops

import (
	"fmt"
	"math"
	"sort"
	"testing"

	"nib/internal/pdfread"
	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// shownAnnot is where one annotation is on the DISPLAYED page (ADR-088's fractions, through the page map's own
// displaySpace), and how what it draws is turned there.
type shownAnnot struct {
	rect    MapRect   // its rectangle
	points  []float64 // every point of /QuadPoints, /L, /Vertices, /CL and /InkList, in that order
	inner   MapRect   // the rectangle /RD insets it to
	hasAP   bool
	turn    [4]float64 // the appearance's own matrix, then the page's rotation: how its drawing is shown
	upright bool       // NoRotate
	w, h    float64    // an upright annotation's size, in points
	mkTurn  int        // a widget's /MK /R less the page's rotation: how a viewer would redraw it
}

// shownAnnots reads page 1's annotations by name (/NM, else a field's /T).
func shownAnnots(t *testing.T, pdf []byte) map[string]shownAnnot {
	t.Helper()
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	pg := pageAt(ctx, nil, 1)
	sp := newDisplaySpace(pg)
	// /Rotate turns the page clockwise: user-space right is shown pointing down at 90.
	shownTurn := map[int][4]float64{0: {1, 0, 0, 1}, 90: {0, -1, 1, 0}, 180: {-1, 0, 0, -1}, 270: {0, 1, -1, 0}}[sp.rot]
	annots, err := ctx.DereferenceArray(pg.Dict["Annots"])
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]shownAnnot{}
	for _, a := range annots {
		d := derefDict(ctx.XRefTable, a)
		name := ""
		for _, k := range []string{"NM", "T"} {
			if s, ok := d[k].(types.StringLiteral); ok && name == "" {
				name = s.Value()
			}
		}
		if name == "" {
			continue // the watermark's own, or a field's nameless kid
		}
		llx, lly, urx, ury, ok := rectOf(ctx, d["Rect"])
		if !ok {
			t.Fatalf("%s has no /Rect", name)
		}
		s := shownAnnot{rect: sp.rect([4]float64{llx, lly, urx, ury})}
		flags, _ := intVal(ctx.XRefTable, d["F"])
		if s.upright = flags&annotNoRotate != 0; s.upright {
			x, y := sp.point(llx, ury)
			s.rect, s.w, s.h = MapRect{x, y, x, y}, urx-llx, ury-lly
		}
		flat := func(o types.Object) {
			arr, _ := ctx.DereferenceArray(o)
			for i := 0; i+1 < len(arr); i += 2 {
				x, _ := pdfNumber(ctx.XRefTable, arr[i])
				y, _ := pdfNumber(ctx.XRefTable, arr[i+1])
				fx, fy := sp.point(x, y)
				s.points = append(s.points, fx, fy)
			}
		}
		for _, k := range []string{"QuadPoints", "L", "Vertices", "CL"} {
			flat(d[k])
		}
		paths, _ := ctx.DereferenceArray(d["InkList"])
		for _, p := range paths {
			flat(p)
		}
		if rd, _ := ctx.DereferenceArray(d["RD"]); len(rd) == 4 {
			var v [4]float64
			for i, e := range rd {
				v[i], _ = pdfNumber(ctx.XRefTable, e)
			}
			s.inner = sp.rect([4]float64{llx + v[0], lly + v[1], urx - v[2], ury - v[3]})
		}
		if ap := derefDict(ctx.XRefTable, d["AP"]); ap != nil {
			n, _ := ctx.Dereference(ap["N"])
			if states, ok := n.(types.Dict); ok {
				keys := make([]string, 0, len(states))
				for k := range states {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				n, _ = ctx.Dereference(states[keys[0]])
			}
			if sd, ok := n.(types.StreamDict); ok {
				m := [4]float64{1, 0, 0, 1}
				if arr, _ := ctx.DereferenceArray(sd.Dict["Matrix"]); len(arr) == 6 {
					for i := range m {
						m[i], _ = pdfNumber(ctx.XRefTable, arr[i])
					}
				}
				r := shownTurn
				if s.upright {
					r = [4]float64{1, 0, 0, 1} // drawn upright whatever the page is turned
				}
				s.hasAP = true
				s.turn = [4]float64{m[0]*r[0] + m[1]*r[2], m[0]*r[1] + m[1]*r[3], m[2]*r[0] + m[3]*r[2], m[2]*r[1] + m[3]*r[3]}
			}
		}
		if nameVal(d, "Subtype") == "Widget" {
			r, _ := intVal(ctx.XRefTable, derefDict(ctx.XRefTable, d["MK"])["R"])
			s.mkTurn = ((r-sp.rot)%360 + 360) % 360
		}
		out[name] = s
	}
	return out
}

func (a shownAnnot) differsFrom(b shownAnnot) string {
	near := func(x, y float64) bool { return math.Abs(x-y) < 1e-3 }
	for i := range a.rect {
		if !near(a.rect[i], b.rect[i]) {
			return fmt.Sprintf("its rectangle was shown at %.3f and is now at %.3f", a.rect, b.rect)
		}
		if !near(a.inner[i], b.inner[i]) {
			return fmt.Sprintf("its /RD inner rectangle was shown at %.3f and is now at %.3f", a.inner, b.inner)
		}
		if !near(a.turn[i], b.turn[i]) {
			return fmt.Sprintf("its appearance was shown turned %v and is now turned %v", a.turn, b.turn)
		}
	}
	if len(a.points) != len(b.points) {
		return fmt.Sprintf("it had %d point coordinates and now has %d", len(a.points), len(b.points))
	}
	for i := range a.points {
		if !near(a.points[i], b.points[i]) {
			return fmt.Sprintf("its points were shown at %.3f and are now at %.3f", a.points, b.points)
		}
	}
	switch {
	case a.hasAP != b.hasAP:
		return "its appearance stream was lost"
	case !near(a.w, b.w) || !near(a.h, b.h):
		return fmt.Sprintf("an upright annotation %.1f×%.1f is now %.1f×%.1f", a.w, a.h, b.w, b.h)
	case a.mkTurn != b.mkTurn:
		return fmt.Sprintf("a viewer redrawing the field turned it %d° and now turns it %d°", a.mkTurn, b.mkTurn)
	}
	return ""
}

// annotatedForm is testpdf.Form() — two widgets — with one annotation of every shape that says where it is in
// user space, each named, on page 1.
func annotatedForm(t *testing.T) []byte {
	t.Helper()
	form, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	out, err := writeMutated(form, func(ctx *model.Context) error {
		pg := pdfread.Pages(ctx)[0]
		annots, err := ctx.DereferenceArray(pg.Dict["Annots"])
		if err != nil {
			return err
		}
		num := func(vs ...float64) types.Array { return numberArray(vs...) }
		// The form's own fields: one drawn a quarter turn against the page, as a field on a turned page often is.
		for _, a := range annots {
			if d := derefDict(ctx.XRefTable, a); nameVal(d, "Subtype") == "Widget" {
				d["NM"], d["MK"] = types.StringLiteral("field"), types.Dict{"R": types.Integer(90)}
				break
			}
		}
		ap := types.StreamDict{Dict: types.Dict{"Type": types.Name("XObject"), "Subtype": types.Name("Form"),
			"BBox": num(0, 0, 80, 20), "Matrix": num(2, 0, 0, 1, 5, 0)}, Content: []byte("0 0 80 20 re f")}
		if err := ap.Encode(); err != nil {
			return err
		}
		apRef, err := ctx.IndRefForNewObject(ap)
		if err != nil {
			return err
		}
		add := func(name, subtype string, rect types.Array, more types.Dict) error {
			d := types.Dict{"Type": types.Name("Annot"), "Subtype": types.Name(subtype), "Rect": rect,
				"NM": types.StringLiteral(name), "F": types.Integer(4)}
			for k, v := range more {
				d[k] = v
			}
			ref, err := ctx.IndRefForNewObject(d)
			if err != nil {
				return err
			}
			annots = append(annots, *ref)
			return nil
		}
		for _, a := range []struct {
			name, subtype string
			rect          types.Array
			more          types.Dict
		}{
			{"link", "Link", num(100, 500, 220, 520), types.Dict{"QuadPoints": num(100, 520, 220, 520, 100, 500, 220, 500),
				"A": types.Dict{"S": types.Name("URI"), "URI": types.StringLiteral("https://example.com")}}},
			{"highlight", "Highlight", num(100, 468, 220, 480), types.Dict{"QuadPoints": num(100, 480, 220, 480, 100, 468, 220, 468)}},
			{"line", "Line", num(50, 400, 250, 430), types.Dict{"L": num(50, 400, 250, 430)}},
			{"ink", "Ink", num(60, 300, 100, 320), types.Dict{"InkList": types.Array{num(60, 300, 80, 320, 100, 300), num(70, 305, 90, 305)}}},
			{"polygon", "Polygon", num(300, 300, 360, 380), types.Dict{"Vertices": num(300, 300, 360, 300, 330, 380)}},
			{"square", "Square", num(300, 500, 400, 540), types.Dict{"RD": num(1, 2, 3, 4)}},
			{"callout", "FreeText", num(300, 600, 420, 640), types.Dict{"CL": num(280, 590, 300, 620), "Contents": types.StringLiteral("x"),
				"DA": types.StringLiteral("/Helv 10 Tf 0 g")}},
			{"note", "Text", num(40, 700, 60, 720), types.Dict{"F": types.Integer(4 + 8 + annotNoRotate), "Contents": types.StringLiteral("x"),
				"AP": types.Dict{"N": *apRef}}},
			{"stamp", "Stamp", num(100, 200, 180, 220), types.Dict{"AP": types.Dict{"N": *apRef}}},
			{"stamp, the same appearance", "Stamp", num(100, 160, 180, 180), types.Dict{"AP": types.Dict{"N": *apRef}}},
			{"stamp, by state", "Stamp", num(100, 120, 180, 140), types.Dict{"AS": types.Name("On"),
				"AP": types.Dict{"N": types.Dict{"On": *apRef, "Off": *apRef}}}},
		} {
			if err := add(a.name, a.subtype, a.rect, a.more); err != nil {
				return err
			}
		}
		pg.Dict["Annots"] = annots
		return nil
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return out
}

// TestAStampLeavesATurnedPagesAnnotationsWhereTheyWere is /pending 829. pdfcpu folds a page's `/Rotate` into its
// content to stamp it and moves no annotation, so every widget, link, note and markup on a turned page was left
// in the old user space by every stamp — the bake that every save runs, the watermark, the page numbers, the OCR
// layer. Measured before the fix on `testpdf.Form()` turned 90°: the name field was shown at the top of the page
// and, watermarked, was off it.
//
// Each annotation is read where the DISPLAYED page shows it, before and after, at every quarter turn, with the
// box at the origin and away from it (stampInPlace turns the content about the box's corner; the annotations go
// with it).
func TestAStampLeavesATurnedPagesAnnotationsWhereTheyWere(t *testing.T) {
	doc := annotatedForm(t)
	stamps := map[string]func(pdf []byte) ([]byte, error){
		"watermark":    func(pdf []byte) ([]byte, error) { return StampWatermark(pdf, "DRAFT", WatermarkStyle{}) },
		"page numbers": func(pdf []byte) ([]byte, error) { return StampPageNumbers(pdf, PageNumberStyle{}) },
		"fields": func(pdf []byte) ([]byte, error) {
			out, _, err := StampFields(pdf, []Field{{Page: 1, Rect: [4]float64{100, 100, 200, 120}, Text: "Jane"}})
			return out, err
		},
		// Twice: the second stamp meets a page with no /Rotate left, and must move nothing again.
		"watermark, then page numbers": func(pdf []byte) ([]byte, error) {
			out, err := StampWatermark(pdf, "DRAFT", WatermarkStyle{})
			if err != nil {
				return nil, err
			}
			return StampPageNumbers(out, PageNumberStyle{})
		},
	}
	for _, deg := range []int{0, 90, 180, 270} {
		for _, cropped := range []bool{false, true} {
			turned := doc
			var err error
			if deg != 0 {
				if turned, err = Rotate(turned, []string{"1"}, deg); err != nil {
					t.Fatal(err)
				}
			}
			if cropped {
				if turned, err = Crop(turned, [4]float64{0.02, 0.03, 0.9, 0.92}, []string{"1"}); err != nil {
					t.Fatal(err)
				}
			}
			before := shownAnnots(t, turned)
			if len(before) != 13 {
				t.Fatalf("STIMULUS: %d named annotations on the page, want the form's 2 widgets and the 11 added", len(before))
			}
			if !before["stamp, by state"].hasAP || !before["stamp"].hasAP || before["field"].mkTurn != (450-deg)%360 || before["square"].inner == (MapRect{}) ||
				len(before["ink"].points) != 10 || !before["note"].upright || !before["note"].hasAP {
				t.Fatalf("STIMULUS: the fixture does not carry what the test reads — stateful appearance %v, stamp appearance %v, "+
					"/RD %v, %d ink coordinates, upright note %v, a field redrawn at %d°", before["stamp, by state"].hasAP, before["stamp"].hasAP,
					before["square"].inner, len(before["ink"].points), before["note"].upright, before["field"].mkTurn)
			}
			widgetsBefore, err := MapPage(turned, 1)
			if err != nil {
				t.Fatal(err)
			}
			for name, stamp := range stamps {
				out, err := stamp(turned)
				if err != nil {
					t.Errorf("%s on a page turned %d (cropped %v): %v", name, deg, cropped, err)
					continue
				}
				after := shownAnnots(t, out)
				for an, b := range before {
					a, ok := after[an]
					if !ok {
						t.Errorf("%s on a page turned %d (cropped %v): %s is gone", name, deg, cropped, an)
						continue
					}
					if diff := b.differsFrom(a); diff != "" {
						t.Errorf("%s on a page turned %d (cropped %v) moved %s: %s", name, deg, cropped, an, diff)
					}
				}
				// And the page map — what detection, the field overlay and the search read — agrees.
				widgetsAfter, err := MapPage(out, 1)
				if err != nil {
					t.Fatal(err)
				}
				if len(widgetsAfter.Widgets) != len(widgetsBefore.Widgets) {
					t.Errorf("%s on a page turned %d (cropped %v): the page map shows %d fields on the page, and showed %d",
						name, deg, cropped, len(widgetsAfter.Widgets), len(widgetsBefore.Widgets))
					continue
				}
				for i, w := range widgetsBefore.Widgets {
					for k := range w.Rect {
						if math.Abs(w.Rect[k]-widgetsAfter.Widgets[i].Rect[k]) > 1e-3 {
							t.Errorf("%s on a page turned %d (cropped %v): the page map shows field %s at %.3f, and showed it at %.3f",
								name, deg, cropped, w.Name, widgetsAfter.Widgets[i].Rect, w.Rect)
							break
						}
					}
				}
			}
		}
	}
}

// TestAnAppearanceSharedWithAnUnturnedPageIsNotTurnedThere — the appearance stream is copied for the turned
// page's annotation, never edited: the same stream may draw an annotation on a page the stamp did not turn.
func TestAnAppearanceSharedWithAnUnturnedPageIsNotTurnedThere(t *testing.T) {
	text, err := testpdf.Text("one", "two")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := writeMutated(text, func(ctx *model.Context) error {
		ap := types.StreamDict{Dict: types.Dict{"Type": types.Name("XObject"), "Subtype": types.Name("Form"),
			"BBox": numberArray(0, 0, 80, 20)}, Content: []byte("0 0 80 20 re f")}
		if err := ap.Encode(); err != nil {
			return err
		}
		apRef, err := ctx.IndRefForNewObject(ap)
		if err != nil {
			return err
		}
		for _, pg := range pdfread.Pages(ctx) {
			ref, err := ctx.IndRefForNewObject(types.Dict{"Type": types.Name("Annot"), "Subtype": types.Name("Stamp"),
				"Rect": numberArray(100, 200, 180, 220), "NM": types.StringLiteral("stamp"), "F": types.Integer(4),
				"AP": types.Dict{"N": *apRef}})
			if err != nil {
				return err
			}
			pg.Dict["Annots"] = types.Array{*ref}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if doc, err = Rotate(doc, []string{"1"}, 90); err != nil {
		t.Fatal(err)
	}
	out, err := StampWatermark(doc, "DRAFT", WatermarkStyle{})
	if err != nil {
		t.Fatal(err)
	}
	matrixOn := func(pdf []byte, page int) string {
		ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
		if err != nil {
			t.Fatal(err)
		}
		annots, _ := ctx.DereferenceArray(pageAt(ctx, nil, page).Dict["Annots"])
		for _, a := range annots {
			d := derefDict(ctx.XRefTable, a)
			if _, named := d["NM"]; !named {
				continue
			}
			sd, _, err := ctx.DereferenceStreamDict(derefDict(ctx.XRefTable, d["AP"])["N"])
			if err != nil || sd == nil {
				t.Fatalf("page %d: the stamp's appearance is gone (%v)", page, err)
			}
			if m, ok := sd.Dict["Matrix"]; ok {
				return m.PDFString()
			}
			return "none"
		}
		t.Fatalf("page %d: the stamp is gone", page)
		return ""
	}
	if got := matrixOn(out, 1); got == "none" {
		t.Error("STIMULUS: the turned page's appearance carries no matrix, so nothing was turned")
	}
	if got := matrixOn(out, 2); got != "none" {
		t.Errorf("the unturned page's stamp is now drawn through %s — its appearance was edited in place for the page that turned", got)
	}
}
