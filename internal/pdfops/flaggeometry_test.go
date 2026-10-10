package pdfops

import (
	"bytes"
	"fmt"
	"image"
	pngenc "image/png"
	"strings"
	"testing"

	"nib/internal/contentstream"
	"nib/internal/pdfread"
	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// A flag is a page coordinate with no content anchor (flags.go, /pending 457), so a rewrite of a flagged
// document keeps the flag beside what it was placed beside ONLY if the rewrite moves nothing the page draws.
// These tests hold the rewrites that operate on the document the server holds — the ones a flagged document
// reaches with its NibFlags intact — to exactly that, structurally: the routes are classified in
// internal/server's flagroutes_test.go, and every route classified `keepsGeometry` names a function driven here.

// drawnOps is a content stream read as operators, each with its operands, with the marked-content operators
// left out — a `BDC`/`EMC` pair tags what is drawn and moves none of it, and tagging is exactly what the
// OCR layer and the tag commit add around existing content.
func drawnOps(src []byte) []string {
	var out, operands []string
	for _, tk := range contentstream.Tokenize(src) {
		switch tk.Kind {
		case contentstream.Whitespace:
			continue
		case contentstream.Operator:
			op := string(tk.Bytes(src))
			switch op {
			case "BDC", "BMC", "EMC", "MP", "DP":
			default:
				out = append(out, strings.Join(append(operands, op), " "))
			}
			operands = operands[:0]
		case contentstream.InlineImage:
			out = append(out, string(tk.Bytes(src)))
			operands = operands[:0]
		default:
			operands = append(operands, string(tk.Bytes(src)))
		}
	}
	return out
}

// opName is the operator a drawnOps entry ends with.
func opName(entry string) string {
	if i := strings.LastIndexByte(entry, ' '); i >= 0 {
		return entry[i+1:]
	}
	return entry
}

var paintOps = map[string]bool{
	"f": true, "F": true, "f*": true, "B": true, "B*": true, "b": true, "b*": true, "S": true, "s": true, "sh": true,
}

var showOps = map[string]bool{"Tj": true, "TJ": true, "'": true, `"`: true}

// paintsNothingVisible reports the first operator in ops that puts ink on the page: a path paint, a shading,
// an inline image, a text show in any render mode but 3 (invisible), or a Do of an image — a Do of a form is
// read through. tr is the render mode in force where ops begin.
func paintsNothingVisible(ctx *model.Context, res types.Dict, ops []string, tr string, depth int) error {
	if depth > 8 {
		return fmt.Errorf("forms nest deeper than the check reads")
	}
	for _, e := range ops {
		op := opName(e)
		switch {
		case op == "Tr":
			tr = strings.TrimSpace(strings.TrimSuffix(e, "Tr"))
		case paintsOps(op):
			return fmt.Errorf("adds a visible paint %q", e)
		case strings.HasPrefix(e, "BI"):
			return fmt.Errorf("adds an inline image")
		case showOps[op] && tr != "3":
			return fmt.Errorf("adds text drawn in render mode %q: %q", tr, e)
		case op == "Do":
			name := strings.TrimPrefix(strings.Fields(e)[0], "/")
			xobjs := derefDict(ctx.XRefTable, res["XObject"])
			sd, _, err := ctx.DereferenceStreamDict(xobjs[name])
			if err != nil || sd == nil {
				return fmt.Errorf("adds a Do of %q, which does not resolve", name)
			}
			if nameVal(sd.Dict, "Subtype") != "Form" {
				return fmt.Errorf("adds a Do of the %s %q", nameVal(sd.Dict, "Subtype"), name)
			}
			if err := sd.Decode(); err != nil {
				return err
			}
			fres := derefDict(ctx.XRefTable, sd.Dict["Resources"])
			if fres == nil {
				fres = res
			}
			if err := paintsNothingVisible(ctx, fres, drawnOps(sd.Content), tr, depth+1); err != nil {
				return fmt.Errorf("form %q: %w", name, err)
			}
		}
	}
	return nil
}

func paintsOps(op string) bool { return paintOps[op] }

// shown maps a point of a page's user space to where the viewer shows it, as a flag's fraction reads it: relative
// to the visible box's lower-left corner, turned clockwise by the page's rotation. It returns the shown width
// and height too, which is what the fraction is a fraction OF.
type shown struct {
	llx, lly, w, h float64
	rot            int
}

func shownOf(p pdfread.Page) (shown, error) {
	box := p.Attrs.MediaBox
	if p.Attrs.CropBox != nil {
		box = p.Attrs.CropBox
	}
	if box == nil {
		return shown{}, fmt.Errorf("no box")
	}
	if u, ok := p.Dict["UserUnit"]; ok {
		return shown{}, fmt.Errorf("a /UserUnit of %v, which this check does not model", u)
	}
	return shown{box.LL.X, box.LL.Y, box.Width(), box.Height(), ((p.Attrs.Rotate % 360) + 360) % 360}, nil
}

func (s shown) size() (float64, float64) {
	if s.rot == 90 || s.rot == 270 {
		return s.h, s.w
	}
	return s.w, s.h
}

func (s shown) at(x, y float64) (float64, float64) {
	u, v := x-s.llx, y-s.lly
	switch s.rot {
	case 90:
		return v, s.w - u
	case 180:
		return s.w - u, s.h - v
	case 270:
		return s.h - v, u
	}
	return u, v
}

// cmMatrix reads a `cm` entry of drawnOps as the affine map it applies to a point.
func cmMatrix(e string) (func(x, y float64) (float64, float64), error) {
	var a, b, c, d, ex, f float64
	if _, err := fmt.Sscanf(e, "%g %g %g %g %g %g cm", &a, &b, &c, &d, &ex, &f); err != nil {
		return nil, err
	}
	return func(x, y float64) (float64, float64) { return a*x + c*y + ex, b*x + d*y + f }, nil
}

func shownNear(a, b float64) bool { return a-b < 0.01 && b-a < 0.01 }

// keepsDrawnGeometry is the property: every page is shown at the same size and draws everything it drew before,
// in the same order with the same operands, after nothing but `q` and `cm` — and the matrices those `cm`s add,
// read through the page's new box and rotation, show every point of the old content exactly where the old box
// and rotation showed it. That is what lets a stamp fold a page's `/Rotate` into its content (pdfcpu does, see
// stampInPlace) and still pass, while a rotation turned about the wrong point fails. Anything added after the
// old content must put no ink on the page.
// The marked-content operators are read past (drawnOps). Forms the old content draws are not re-read: the
// three routes this guards change no form, and the pixel comparison recorded in flags.go measured it.
func keepsDrawnGeometry(before, after []byte) error { return keepsDrawnContent(before, after, false) }

// keepsDrawnContent is keepsDrawnGeometry, with the ink check off when addsInk: a visible stamp adds ink by
// design, and what it must not do is move what was there.
func keepsDrawnContent(before, after []byte, addsInk bool) error {
	bctx, err := pdfread.ReadOptimized(before, model.NewDefaultConfiguration())
	if err != nil {
		return err
	}
	actx, err := pdfread.ReadOptimized(after, model.NewDefaultConfiguration())
	if err != nil {
		return err
	}
	bp, ap := pdfread.Pages(bctx), pdfread.Pages(actx)
	if len(bp) != len(ap) {
		return fmt.Errorf("the page count moved from %d to %d", len(bp), len(ap))
	}
	for i := range bp {
		if bp[i].Err != nil || ap[i].Err != nil {
			return fmt.Errorf("page %d does not read: %v / %v", i+1, bp[i].Err, ap[i].Err)
		}
		was, err := shownOf(bp[i])
		if err != nil {
			return fmt.Errorf("page %d: %v", i+1, err)
		}
		is, err := shownOf(ap[i])
		if err != nil {
			return fmt.Errorf("page %d: %v", i+1, err)
		}
		ww, wh := was.size()
		iw, ih := is.size()
		if !shownNear(ww, iw) || !shownNear(wh, ih) {
			return fmt.Errorf("page %d is shown at %.2fx%.2f, was %.2fx%.2f", i+1, iw, ih, ww, wh)
		}
		bc, _ := pdfread.PageContent(bctx, bp[i].Dict, i+1)
		ac, _ := pdfread.PageContent(actx, ap[i].Dict, i+1)
		o, n := drawnOps(bc), drawnOps(ac)
		at := -1
		for s := 0; s+len(o) <= len(n) && at < 0; s++ {
			match := true
			for k := range o {
				if n[s+k] != o[k] {
					match = false
					break
				}
			}
			if match {
				at = s
			}
		}
		if at < 0 {
			return fmt.Errorf("page %d no longer draws its own content unchanged (%d operators before, %d after)", i+1, len(o), len(n))
		}
		// The matrices in front of the old content, applied to a point in reverse order of listing.
		var ms []func(x, y float64) (float64, float64)
		for _, e := range n[:at] {
			switch {
			case e == "q":
			case opName(e) == "cm":
				m, err := cmMatrix(e)
				if err != nil {
					return fmt.Errorf("page %d: %v", i+1, err)
				}
				ms = append(ms, m)
			default:
				return fmt.Errorf("page %d sets %q before its own content", i+1, e)
			}
		}
		for _, pt := range [][2]float64{{was.llx, was.lly}, {was.llx + was.w, was.lly}, {was.llx, was.lly + was.h}} {
			x, y := pt[0], pt[1]
			for k := len(ms) - 1; k >= 0; k-- {
				x, y = ms[k](x, y)
			}
			gx, gy := is.at(x, y)
			wx, wy := was.at(pt[0], pt[1])
			if !shownNear(gx, wx) || !shownNear(gy, wy) {
				return fmt.Errorf("page %d shows its own content's point (%.2f, %.2f) at (%.2f, %.2f), was (%.2f, %.2f)",
					i+1, pt[0], pt[1], gx, gy, wx, wy)
			}
		}
		if addsInk {
			continue
		}
		if err := paintsNothingVisible(actx, ap[i].Attrs.Resources, n[at+len(o):], "0", 0); err != nil {
			return fmt.Errorf("page %d %w", i+1, err)
		}
	}
	return nil
}

// flaggedFixtures are the documents the census drives each rewrite over, each carrying a flag. They span the
// shapes a fraction can be read against: plain body text, a scan (OCR's own input), a tagged document, and a
// page whose crop box is offset and whose rotation is not zero — the two things that make a fraction mean
// something other than "of the media box, upright".
func flaggedFixtures(t *testing.T) map[string][]byte {
	t.Helper()
	text, err := testpdf.Text("Sign here: ____________", "Second page")
	if err != nil {
		t.Fatal(err)
	}
	turned, err := Rotate(text, []string{"1"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	turned, err = Crop(turned, [4]float64{0.1, 0.05, 0.8, 0.85}, []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	docs := map[string][]byte{
		"body text":         text,
		"scan":              scannedPage(t),
		"tagged":            formHost(t),
		"turned and offset": turned,
	}
	flags := []byte(`[{"page":1,"frac":[0.2,0.7,0.2,0.05],"type":"sig"}]`)
	for name, doc := range docs {
		flagged, err := SetFlags(doc, flags)
		if err != nil {
			t.Fatalf("setup: flagging %s: %v", name, err)
		}
		docs[name] = flagged
	}
	return docs
}

// geometryKeepers are the rewrites behind every route internal/server classifies `keepsGeometry`, by the
// name that classification cites. Each takes the flagged document and returns what the route commits.
var geometryKeepers = map[string]func(t *testing.T, pdf []byte) ([]byte, error){
	"TagOCRLayer": func(t *testing.T, pdf []byte) ([]byte, error) {
		out, _, err := TagOCRLayer(pdf, ocrWords(), "eng")
		if err != nil {
			return nil, err
		}
		return SetLang(out, "en") // handleOCR's second step
	},
	"StripActive":         func(_ *testing.T, pdf []byte) ([]byte, error) { return StripActive(pdf) },
	"RemoveFilesAndMedia": func(_ *testing.T, pdf []byte) ([]byte, error) { return RemoveFilesAndMedia(pdf) },
	"StripMetadata":       func(_ *testing.T, pdf []byte) ([]byte, error) { return StripMetadata(pdf) },
	"AddAttachment": func(_ *testing.T, pdf []byte) ([]byte, error) {
		return AddAttachment(pdf, "notes.txt", []byte("hello"))
	},
	"RemovePassword": func(_ *testing.T, pdf []byte) ([]byte, error) {
		locked, err := Encrypt(pdf, "pw")
		if err != nil {
			return nil, err
		}
		return RemovePassword(locked, "pw")
	},
	"CommitTags": func(t *testing.T, pdf []byte) ([]byte, error) {
		p, err := ProposeTags(pdf)
		if err != nil {
			return nil, err
		}
		reviews := make([]TagReview, len(p.Elements))
		for i, e := range p.Elements {
			reviews[i] = TagReview{ID: e.ID, Role: e.Role, Text: e.Text}
		}
		return CommitTags(pdf, reviews)
	},
	"RemoveStructure": func(_ *testing.T, pdf []byte) ([]byte, error) { return RemoveStructure(pdf) },
	"EditStructure": func(_ *testing.T, pdf []byte) ([]byte, error) {
		v, err := readStructureView(pdf)
		if err != nil {
			return nil, err
		}
		for _, e := range v.elements {
			if e.id != 0 {
				return EditStructure(pdf, []StructureEdit{{Kind: "artifact", Element: e.id}})
			}
		}
		return nil, fmt.Errorf("no element to edit")
	},
}

// TestEveryRewriteOfAHeldDocumentKeepsWhatAFlagWasPlacedBeside — /pending 457's guard. A flag carries no
// content anchor, and none is needed while every rewrite that can reach a flagged document either moves no
// drawn content (this test) or moves its flags with it (the reflow door, anchormove.go). A rewrite added to
// the OCR, sanitize or attachment routes that deskews, reflows or re-imposes a page turns this red, which is
// when an anchor — and the format change it is — becomes owed.
//
// Two rewrites refuse some fixtures by their own rules, and those pairs are named rather than skipped on any
// error: a tag proposal is not written over an existing tree, nor committed with nothing in it (the scan has no
// text), and a structure edit, like the removal of a tree, needs a tree. Any other failure is a failure.
var notApplicable = map[string]bool{
	"CommitTags/tagged": true, "CommitTags/scan": true,
	"EditStructure/body text": true, "EditStructure/scan": true, "EditStructure/turned and offset": true,
	"RemoveStructure/body text": true, "RemoveStructure/scan": true, "RemoveStructure/turned and offset": true,
}

func TestEveryRewriteOfAHeldDocumentKeepsWhatAFlagWasPlacedBeside(t *testing.T) {
	docs := flaggedFixtures(t)
	for name, rewrite := range geometryKeepers {
		for fixture, doc := range docs {
			if notApplicable[name+"/"+fixture] {
				continue
			}
			out, err := rewrite(t, doc)
			if err != nil {
				t.Errorf("%s on %s: %v", name, fixture, err)
				continue
			}
			if err := keepsDrawnGeometry(doc, out); err != nil {
				t.Errorf("%s on %s moves what a flag was placed beside: %v", name, fixture, err)
			}
		}
	}
}

// TestTheGeometryCheckSeesAMove proves keepsDrawnGeometry is not inert: each rewrite here moves what a flag
// sits beside — a turn, a crop, a page dropped, a stamp drawn over the page, content shifted by a matrix — and
// each must be refused.
func TestTheGeometryCheckSeesAMove(t *testing.T) {
	doc := flaggedFixtures(t)["body text"]
	movers := map[string]func() ([]byte, error){
		"rotate":    func() ([]byte, error) { return Rotate(doc, []string{"1"}, 90) },
		"crop":      func() ([]byte, error) { return Crop(doc, [4]float64{0.1, 0.1, 0.8, 0.8}, []string{"1"}) },
		"delete":    func() ([]byte, error) { return RemovePages(doc, []string{"1"}) },
		"stamp":     func() ([]byte, error) { return StampWatermark(doc, "DRAFT", WatermarkStyle{}) },
		"shift":     func() ([]byte, error) { return prependToPage(doc, "1 0 0 1 0 -40 cm\n") },
		"invisible": func() ([]byte, error) { return prependToPage(doc, "") }, // the control: must PASS
	}
	for name, move := range movers {
		out, err := move()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		err = keepsDrawnGeometry(doc, out)
		if name == "invisible" {
			if err != nil {
				t.Errorf("an unchanged page reads as moved: %v", err)
			}
			continue
		}
		if err == nil {
			t.Errorf("%s moves the page's content and the check passes it", name)
		}
	}
}

// TestAStampLeavesATurnedPageWhereItWas — the defect /pending 457's census found. pdfcpu folds a page's
// `/Rotate` into its content to stamp it, turning it about the origin, so a turned page whose box does not
// start at the origin had its whole drawing carried across the page by every stamp: the OCR layer, and the
// bake's fields and images, the watermark and the page numbers, which reach every save. stampInPlace turns it
// about the box's corner; each stamp, at each quarter turn, must show the page's own content where it was.
func TestAStampLeavesATurnedPageWhereItWas(t *testing.T) {
	text, err := testpdf.Text("Sign here: ____________")
	if err != nil {
		t.Fatal(err)
	}
	png := func() []byte {
		var buf bytes.Buffer
		img := image.NewRGBA(image.Rect(0, 0, 4, 2))
		if err := pngenc.Encode(&buf, img); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}()
	stamps := map[string]struct {
		run     func(pdf []byte) ([]byte, error)
		addsInk bool
	}{
		"OCR layer": {func(pdf []byte) ([]byte, error) { return StampTextLayer(pdf, ocrWords(), "eng") }, false},
		"fields": {func(pdf []byte) ([]byte, error) {
			out, _, err := StampFields(pdf, []Field{{Page: 1, Rect: [4]float64{100, 100, 200, 120}, Text: "Jane"}})
			return out, err
		}, true},
		"images": {func(pdf []byte) ([]byte, error) {
			return StampImages(pdf, []Stamp{{Page: 1, Rect: [4]float64{100, 100, 140, 120}, PNG: png}})
		}, true},
		"watermark":    {func(pdf []byte) ([]byte, error) { return StampWatermark(pdf, "DRAFT", WatermarkStyle{}) }, true},
		"page numbers": {func(pdf []byte) ([]byte, error) { return StampPageNumbers(pdf, PageNumberStyle{}) }, true},
	}
	for _, deg := range []int{90, 180, 270} {
		turned, err := Rotate(text, []string{"1"}, deg)
		if err != nil {
			t.Fatal(err)
		}
		turned, err = Crop(turned, [4]float64{0.1, 0.05, 0.8, 0.85}, []string{"1"})
		if err != nil {
			t.Fatal(err)
		}
		for name, st := range stamps {
			out, err := st.run(turned)
			if err != nil {
				t.Errorf("%s on a page turned %d: %v", name, deg, err)
				continue
			}
			if err := keepsDrawnContent(turned, out, st.addsInk); err != nil {
				t.Errorf("%s on a page turned %d and cropped: %v", name, deg, err)
			}
		}
	}
}

// prependToPage rewrites page 1's content as pre + its own content — the shape of a rewrite that moves a page's
// drawing with a matrix.
func prependToPage(pdf []byte, pre string) ([]byte, error) {
	return writeMutated(pdf, func(ctx *model.Context) error {
		p := pdfread.Pages(ctx)[0]
		c, err := pdfread.PageContent(ctx, p.Dict, 1)
		if err != nil {
			return err
		}
		sd, err := ctx.NewStreamDictForBuf(append([]byte(pre), c...))
		if err != nil {
			return err
		}
		if err := sd.Encode(); err != nil {
			return err
		}
		ref, err := ctx.IndRefForNewObject(*sd)
		if err != nil {
			return err
		}
		p.Dict["Contents"] = *ref
		return nil
	})
}
