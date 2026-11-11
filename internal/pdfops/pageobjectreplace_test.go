package pdfops

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
)

// The replace door (PLAN-image-edit P03.S01): the new picture is Import's one-page PDF, made here as the route and
// the CLI make it.

func replacement(t *testing.T, b []byte, name string) []byte {
	t.Helper()
	res, err := importBytes(t, b, name, ReplacementImport("test"))
	if err != nil {
		t.Fatalf("importing the replacement: %v", err)
	}
	return res.PDF
}

func replaceObject(t *testing.T, pdf []byte, ordinal int, with []byte, e ObjectEdit) ([]byte, ObjectEditResult, error) {
	t.Helper()
	objs := readObjects(t, pdf, 1)
	e.Kind, e.Page, e.Ordinal, e.Digest, e.With = ObjectEditReplace, 1, ordinal, objs.Objects[ordinal].Digest, with
	return EditPageObjects(pdf, []ObjectEdit{e})
}

func alphaPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{R: 200, G: 30, B: 30, A: uint8(x * 255 / w)})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// A movable picture is replaced by one fitted inside its box keeping the new proportion, centred; the other picture,
// the text and the count stay; the old image goes from the file.
func TestAReplacedPictureIsFittedInsideItsBox(t *testing.T) {
	pdf := removeFixture(twoPicturesAndText, nil)
	before := readObjects(t, pdf, 1)
	if !before.Objects[0].Movable {
		t.Fatalf("fixture: picture 1 is not movable (%s)", before.Objects[0].MoveWhy)
	}
	out, res, err := replaceObject(t, pdf, 0, replacement(t, solidPNG(t, 40, 20, 0), "r.png"), ObjectEdit{})
	if err != nil {
		t.Fatal(err)
	}
	after := readObjects(t, out, 1)
	if len(after.Objects) != 2 || after.Objects[1].operator != before.Objects[1].operator {
		t.Fatalf("the other picture changed: %+v", after.Objects)
	}
	// The old box is 100 × 80 at (10, 10); a 2:1 picture fills its width: 100 × 50, centred at y 25.
	if got, want := after.Objects[0].userBox, [4]float64{10, 25, 110, 75}; !boxesAgree(got, want) {
		t.Fatalf("the new picture is at %v, want %v", got, want)
	}
	if len(res.Replaced) != 1 || !res.Replaced[0].Fitted || res.Replaced[0].Stretched {
		t.Fatalf("answer %+v", res.Replaced)
	}
	if after.Objects[0].objNr == 6 {
		t.Fatal("the page still draws the old image")
	}
	streams, err := imageStreams(out)
	if err != nil {
		t.Fatal(err)
	}
	if _, still := streams[6]; still {
		t.Fatal("the old image is still in the file, and no page draws it")
	}
}

// A picture that cannot be moved keeps its `cm`, so the new picture fills the old box and the answer says it was
// stretched; one of the same proportion is not.
func TestAReplacedPictureThatCannotMoveFillsItsBox(t *testing.T) {
	pdf := removeFixture("q 1 0 0 1 0 0 cm 100 0 0 80 10 10 cm /Im0 Do Q\nBT /F1 12 Tf 10 700 Td (Hello) Tj ET", nil)
	if readObjects(t, pdf, 1).Objects[0].Movable {
		t.Fatal("fixture: the picture is movable")
	}
	for _, c := range []struct {
		w, h      int
		stretched bool
	}{{40, 20, true}, {50, 40, false}} {
		out, res, err := replaceObject(t, pdf, 0, replacement(t, solidPNG(t, c.w, c.h, 0), "r.png"), ObjectEdit{})
		if err != nil {
			t.Fatal(err)
		}
		if got := readObjects(t, out, 1).Objects[0].userBox; !boxesAgree(got, [4]float64{10, 10, 110, 90}) {
			t.Fatalf("%d×%d: at %v, want the old box", c.w, c.h, got)
		}
		if res.Replaced[0].Stretched != c.stretched || res.Replaced[0].Fitted {
			t.Fatalf("%d×%d: answer %+v", c.w, c.h, res.Replaced[0])
		}
	}
}

// A phone photo whose EXIF orientation Import put on its page is drawn upright where the picture can be moved, fitted
// by its turned proportion; where it cannot, the replace is refused.
func TestATurnedPhotoIsTurnedBackOrRefused(t *testing.T) {
	photo := replacement(t, jpegWithOrientation(t, 40, 20, 6), "photo.jpg")
	out, res, err := replaceObject(t, removeFixture(twoPicturesAndText, nil), 0, photo, ObjectEdit{})
	if err != nil {
		t.Fatal(err)
	}
	// Turned a quarter, the 40 × 20 photo stands 1:2: in the 100 × 80 box it is 40 × 80, centred at x 40.
	turned := readObjects(t, out, 1).Objects[0]
	if !boxesAgree(turned.userBox, [4]float64{40, 10, 80, 90}) {
		t.Fatalf("the turned photo is at %v", turned.userBox)
	}
	// A box does not show a turn: the picture is drawn turned (not straight), where an upright one is drawn straight.
	if turned.plain {
		t.Fatal("the photo is drawn straight, so it reads sideways")
	}
	upright, _, err := replaceObject(t, removeFixture(twoPicturesAndText, nil), 0, replacement(t, jpegWithOrientation(t, 40, 20, 1), "up.jpg"), ObjectEdit{})
	if err != nil {
		t.Fatal(err)
	}
	if !readObjects(t, upright, 1).Objects[0].plain {
		t.Fatal("an upright photo is drawn turned")
	}
	if res.Replaced[0].Turned != 90 {
		t.Fatalf("answer %+v", res.Replaced[0])
	}
	stuck := removeFixture("q 1 0 0 1 0 0 cm 100 0 0 80 10 10 cm /Im0 Do Q", nil)
	if _, _, err := replaceObject(t, stuck, 0, photo, ObjectEdit{}); err == nil {
		t.Fatal("a turned photo replaced a picture that cannot be moved")
	} else {
		wantRefusal(t, err, WhyOrientation)
	}
}

// An alpha PNG is lifted with its mask and counts as one picture.
func TestAnAlphaPictureIsLiftedWithItsMask(t *testing.T) {
	out, _, err := replaceObject(t, removeFixture(twoPicturesAndText, nil), 0, replacement(t, alphaPNG(t, 30, 30), "a.png"), ObjectEdit{})
	if err != nil {
		t.Fatal(err)
	}
	objs := readObjects(t, out, 1).Objects
	if len(objs) != 2 {
		t.Fatalf("%d pictures after the replace, want 2", len(objs))
	}
	ctx, err := pdfread.ReadOptimized(out, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	sd, _, err := ctx.DereferenceStreamDict(*types.NewIndirectRef(objs[0].objNr, 0))
	if err != nil || sd == nil {
		t.Fatal(err)
	}
	if _, ok := sd.Dict["SMask"].(types.IndirectRef); !ok {
		t.Fatalf("the new picture has no mask: %v", sd.Dict)
	}
}

// A described Figure is replaced only with an answer about its description; the element stays either way.
func TestADescribedPictureIsReplacedOnlyWithAnAnswerAboutItsDescription(t *testing.T) {
	pdf := objectFigureFixture(figureThenText, "0", "")
	with := replacement(t, solidPNG(t, 30, 30, 0), "r.png")
	if _, _, err := replaceObject(t, pdf, 0, with, ObjectEdit{}); err == nil {
		t.Fatal("a described picture was replaced with no word about its description")
	} else {
		wantRefusal(t, err, CauseAltUnanswered)
	}
	for _, c := range []struct {
		e    ObjectEdit
		want string
	}{{ObjectEdit{KeepAlt: true}, "A cat on a mat"}, {ObjectEdit{Alt: "A dog on a log"}, "A dog on a log"}} {
		out, res, err := replaceObject(t, pdf, 0, with, c.e)
		if err != nil {
			t.Fatal(err)
		}
		objs := readObjects(t, out, 1)
		if !objs.Objects[0].Tagged || objs.Objects[0].Alt != c.want || res.Replaced[0].Alt != c.want {
			t.Fatalf("%+v: the picture reads tagged %v, %q; answered %q", c.e, objs.Objects[0].Tagged, objs.Objects[0].Alt, res.Replaced[0].Alt)
		}
	}
}

// Two pages sharing one indirect `/Resources`: the new name lands on the edited page alone, and the other page draws
// what it drew.
func TestANewNameDoesNotReachAPageSharingTheResources(t *testing.T) {
	pdf := removeFixture(twoPicturesAndText, map[int]string{
		2:  "<< /Type /Pages /Kids [3 0 R 14 0 R] /Count 2 >>",
		14: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Resources 9 0 R /Contents 15 0 R >>",
		15: streamObject("", "q 50 0 0 40 200 200 cm /Im1 Do Q"),
	})
	before := readObjects(t, pdf, 2)
	out, _, err := replaceObject(t, pdf, 0, replacement(t, solidPNG(t, 30, 30, 0), "r.png"), ObjectEdit{})
	if err != nil {
		t.Fatal(err)
	}
	after := readObjects(t, out, 2)
	if len(after.Objects) != 1 || after.Objects[0].operator != before.Objects[0].operator {
		t.Fatalf("page 2 changed: %+v", after.Objects)
	}
	ctx, err := pdfread.ReadOptimized(out, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	pg := pdfread.Pages(ctx)[1]
	res := pg.Dict["Resources"]
	if res == nil && pg.Attrs != nil {
		res = pg.Attrs.Resources
	}
	rd, _ := ctx.DereferenceDict(res)
	xo, _ := ctx.DereferenceDict(rd["XObject"])
	if _, leaked := xo["NibR1"]; leaked {
		t.Fatal("page 2's resources gained the new name")
	}
}

// Refused by cause: a stencil, an image the structure refers to, a replacement of two pictures.
func TestAReplaceIsRefusedByCause(t *testing.T) {
	with := replacement(t, solidPNG(t, 20, 20, 0), "r.png")
	for _, c := range []struct {
		name  string
		pdf   []byte
		with  []byte
		cause string
	}{
		{"a stencil", removeFixture(twoPicturesAndText, map[int]string{6: "<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ImageMask true /BitsPerComponent 1 /Length 1 >>\nstream\n\x00\nendstream"}), with, WhyImageMask},
		{"an inline stencil", removeFixture("q 100 0 0 80 10 10 cm BI /W 2 /H 2 /IM true ID \x00 EI Q", nil), with, WhyImageMask},
		{"an image the tags refer to", removeFixture(twoPicturesAndText, map[int]string{6: "<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8 /StructParent 4 /Length 4 >>\nstream\nabcd\nendstream"}), with, WhyObjectReferenced},
		{"two pictures for one", removeFixture(twoPicturesAndText, nil), removeFixture(twoPicturesAndText, nil), CauseNotOneImage},
		{"a form-drawn picture", removeFixture("q /Fm0 Do Q", nil), with, WhyFormDrawn},
	} {
		_, _, err := replaceObject(t, c.pdf, 0, c.with, ObjectEdit{})
		if err == nil {
			t.Errorf("%s: replaced", c.name)
			continue
		}
		wantRefusal(t, err, c.cause)
	}
	pdf := removeFixture(twoPicturesAndText, nil)
	d := readObjects(t, pdf, 1).Objects[0].Digest
	for name, e := range map[string]ObjectEdit{
		"no picture to put": {Kind: ObjectEditReplace, Page: 1, Ordinal: 0, Digest: d},
		"a box":             {Kind: ObjectEditReplace, Page: 1, Ordinal: 0, Digest: d, With: with, Box: &MapRect{0.1, 0.1, 0.2, 0.2}},
	} {
		if _, _, err := EditPageObjects(pdf, []ObjectEdit{e}); err == nil {
			t.Errorf("%s: accepted", name)
		} else {
			wantRefusal(t, err, CauseBadRequest)
		}
	}
}

// shownCorner is where the stored image's unit point (ux, uy) shows on the displayed page, and the box's shown
// top-left: a picture reads upright exactly when its UPRIGHT top-left corner shows there.
func shownCorner(t *testing.T, pdf []byte, ordinal int, ux, uy float64) (corner, topLeft [2]float64) {
	t.Helper()
	ctx, err := pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	pg := pdfread.Pages(ctx)[0]
	pr, err := readPageDrawings(ctx, pg)
	if err != nil {
		t.Fatal(err)
	}
	sp := newDisplaySpace(pg)
	im := pr.images[ordinal]
	x, y := im.ctm.apply(ux, uy)
	fx, fy := sp.point(x, y)
	r := sp.rect(im.box)
	return [2]float64{fx, fy}, [2]float64{r[0], r[1]}
}

// A replaced picture reads upright as displayed at every EXIF turn, and on a page /Rotate turns for display: its
// UPRIGHT top-left corner shows at its box's top-left. A stored image's top row is unit y 1; a photo its EXIF says to
// turn clockwise by r reads upright with its stored bottom-left (r 90), bottom-right (180) or top-right (270) as its
// top-left.
func TestAReplacedPictureReadsUprightAsDisplayed(t *testing.T) {
	for _, c := range []struct {
		name   string
		page   string
		orient int
	}{
		{"upright", "", 1}, {"EXIF 3 (half turn)", "", 3}, {"EXIF 6", "", 6}, {"EXIF 8", "", 8},
		{"a page turned 90", "/Rotate 90", 1}, {"a page turned 270, EXIF 6", "/Rotate 270", 6},
	} {
		pdf := removeFixture(twoPicturesAndText, map[int]string{
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] " + c.page + " /Resources 9 0 R /Contents 4 0 R >>",
		})
		out, _, err := replaceObject(t, pdf, 0, replacement(t, jpegWithOrientation(t, 40, 20, c.orient), "p.jpg"), ObjectEdit{})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		upright := map[int][2]float64{1: {0, 1}, 6: {0, 0}, 3: {1, 0}, 8: {1, 1}}[c.orient]
		corner, tl := shownCorner(t, out, 0, upright[0], upright[1])
		if math.Abs(corner[0]-tl[0]) > 1e-6 || math.Abs(corner[1]-tl[1]) > 1e-6 {
			t.Errorf("%s: the picture's top-left shows at %v, its box's at %v — it reads turned", c.name, corner, tl)
		}
	}
}

// A picture drawn under a quarter-turned matrix keeps it, and whether it is stretched is read from the matrix's sides,
// not from the bounding box, which a quarter turn inverts.
func TestATurnedSlotThatCannotMoveIsRefused(t *testing.T) {
	// A picture that cannot be moved keeps its matrix, and that matrix shows the new picture's pixels as it showed the
	// old ones' (P03 phase-close review): under a quarter turn, or on a turned page, an upright photo would show turned.
	page := func(rotate string) map[int]string {
		return map[int]string{3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Resources 9 0 R /Contents 4 0 R" + rotate + " >>"}
	}
	for _, c := range []struct {
		name, content, rotate, want string
	}{
		{"a quarter-turned matrix", "q 0 100 -80 0 90 10 cm /Im0 Do Q", "", WhyTurnedSlot},
		{"a mirrored matrix", "q -100 0 0 80 110 10 cm /Im0 Do Q", "", WhyTurnedSlot},
		{"a turned page", "q 1 0 0 1 0 0 cm 100 0 0 80 10 10 cm /Im0 Do Q", " /Rotate 90", WhyTurnedSlot},
		{"straight on an unturned page", "q 1 0 0 1 0 0 cm 100 0 0 80 10 10 cm /Im0 Do Q", "", ""},
	} {
		pdf := removeFixture(c.content, page(c.rotate))
		if readObjects(t, pdf, 1).Objects[0].Movable {
			t.Fatalf("%s: fixture is movable", c.name)
		}
		_, _, err := replaceObject(t, pdf, 0, replacement(t, solidPNG(t, 50, 40, 0), "r.png"), ObjectEdit{})
		if c.want == "" && err != nil || c.want != "" && !isCause(err, c.want) {
			t.Errorf("%s: answered %v, want %q", c.name, err, c.want)
		}
	}
}

// A picture in a layer is replaced inside the layer.
func TestAReplacedPictureStaysInItsLayer(t *testing.T) {
	pdf := removeFixture(twoPicturesAndText, map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /OCProperties << /OCGs [20 0 R] /D << /OFF [20 0 R] >> >> >>",
		6:  "<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8 /OC 20 0 R /Length 4 >>\nstream\nabcd\nendstream",
		20: "<< /Type /OCG /Name (Watermark) >>",
	})
	out, _, err := replaceObject(t, pdf, 0, replacement(t, solidPNG(t, 20, 20, 0), "r.png"), ObjectEdit{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := pdfread.ReadOptimized(out, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	objs := readObjects(t, out, 1).Objects
	sd, _, err := ctx.DereferenceStreamDict(*types.NewIndirectRef(objs[0].objNr, 0))
	if err != nil || sd == nil {
		t.Fatal(err)
	}
	if _, ok := sd.Dict["OC"]; !ok {
		t.Fatal("the new picture left the layer the old one was in")
	}
}

// A reference anywhere inside a lifted value is refused, a direct value is kept.
func TestALiftedValueHoldingAReferenceIsRefused(t *testing.T) {
	for _, c := range []struct {
		name string
		o    types.Object
		want bool
	}{
		{"a name", types.Name("DeviceRGB"), false},
		{"a direct array", types.Array{types.Name("Indexed"), types.Name("DeviceRGB"), types.Integer(1)}, false},
		{"a reference", *types.NewIndirectRef(7, 0), true},
		{"a reference in an array", types.Array{types.Name("ICCBased"), *types.NewIndirectRef(7, 0)}, true},
		{"a reference in a dictionary", types.Dict{"JBIG2Globals": *types.NewIndirectRef(5, 0)}, true},
	} {
		if got := holdsReference(c.o, 0); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// The new name is written only into copies the page owns: a resources dictionary another page shares is left as it was.
func TestANewNameIsWrittenIntoCopiesOnly(t *testing.T) {
	ctx, err := pdfread.ReadOptimized(removeFixture(twoPicturesAndText, nil), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	shared := types.Dict{"XObject": types.Dict{"Im0": *types.NewIndirectRef(6, 0)}}
	page, other := types.Dict{"Resources": shared}, types.Dict{"Resources": shared}
	if err := addXObjectName(ctx, page, "NibR1", *types.NewIndirectRef(6, 0)); err != nil {
		t.Fatal(err)
	}
	if _, leaked := other["Resources"].(types.Dict)["XObject"].(types.Dict)["NibR1"]; leaked {
		t.Fatal("the shared dictionary gained the new name")
	}
	if _, ok := page["Resources"].(types.Dict)["XObject"].(types.Dict)["NibR1"]; !ok {
		t.Fatal("the page did not gain the new name")
	}
}

func TestAReplaceGivingBothDescriptionAnswersIsRefused(t *testing.T) {
	_, _, err := replaceObject(t, objectFigureFixture(figureThenText, "0", ""), 0, replacement(t, solidPNG(t, 20, 20, 0), "r.png"), ObjectEdit{Alt: "A dog", KeepAlt: true})
	if err == nil {
		t.Fatal("a replace both keeping and changing the description went through")
	}
	wantRefusal(t, err, CauseBadRequest)
}

// A replacement whose picture holds a reference inside a direct value (an ICC colour space) is refused at the lift:
// copied as it is, the reference would name an unrelated object of the edited document.
func TestALiftOfAPictureHoldingANestedReferenceIsRefused(t *testing.T) {
	with := objectsFixtureWith(map[int]string{
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources 9 0 R /Contents 4 0 R >>",
		4:  streamObject("", "q 10 0 0 10 0 0 cm /Im0 Do Q"),
		6:  "<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace [/ICCBased 21 0 R] /BitsPerComponent 8 /Length 4 >>\nstream\nabcd\nendstream",
		9:  "<< /XObject << /Im0 6 0 R >> >>",
		21: streamObject("/N 1", "icc"),
	})
	_, _, err := replaceObject(t, removeFixture(twoPicturesAndText, nil), 0, with, ObjectEdit{})
	if err == nil {
		t.Fatal("a picture with a nested reference was lifted")
	}
	wantRefusal(t, err, CauseNotOneImage)
}

// A described picture whose tag cannot be taken apart (it also holds text) still names its description, so a replace
// asks about it: the element stays, and its description must not silently describe the new picture (P03 phase-close
// review; ADR-223 §5).
func TestASharedTagsDescriptionIsAskedAboutOnReplace(t *testing.T) {
	shared := "/Figure <</MCID 0>> BDC q 100 0 0 80 10 10 cm /Im0 Do Q BT /F1 12 Tf 10 700 Td (Caption) Tj ET EMC\n" +
		"/P <</MCID 1>> BDC BT /F1 12 Tf 10 600 Td (Hello) Tj ET EMC"
	pdf := objectFigureFixture(shared, "0", "")
	o := readObjects(t, pdf, 1).Objects[0]
	if o.Why != WhyTaggedShared || !o.Replaceable || o.Alt != "A cat on a mat" {
		t.Fatalf("fixture: listed why %q, replaceable %v, alt %q", o.Why, o.Replaceable, o.Alt)
	}
	png := replacement(t, solidPNG(t, 50, 40, 0), "r.png")
	if _, _, err := replaceObject(t, pdf, 0, png, ObjectEdit{}); !isCause(err, CauseAltUnanswered) {
		t.Errorf("a replace with no answer about the description: %v", err)
	}
	if _, _, err := replaceObject(t, pdf, 0, png, ObjectEdit{KeepAlt: true}); err != nil {
		t.Errorf("a replace keeping the description: %v", err)
	}
}

// A replace says where the old picture still is, as a removal does (`pictureResidue`).
func TestAReplaceSaysTheOldPictureStaysWhereAnotherPageDrawsIt(t *testing.T) {
	two := streamObject("", "q 100 0 0 80 10 10 cm /Im0 Do Q")
	pdf := removeFixture("q 100 0 0 80 10 10 cm /Im0 Do Q", map[int]string{
		2:  "<< /Type /Pages /Kids [3 0 R 20 0 R] /Count 2 >>",
		20: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Resources 9 0 R /Contents 21 0 R >>",
		21: two,
	})
	_, res, err := replaceObject(t, pdf, 0, replacement(t, solidPNG(t, 50, 40, 0), "r.png"), ObjectEdit{})
	if err != nil {
		t.Fatal(err)
	}
	if r := res.Replaced[0]; !r.StaysInFile || len(r.SharedWith) != 1 || r.SharedWith[0] != 2 {
		t.Errorf("the answer reads %+v, want the old picture said to stay because page 2 draws it", r)
	}
	_, res, err = replaceObject(t, removeFixture("q 100 0 0 80 10 10 cm /Im0 Do Q", nil), 0, replacement(t, solidPNG(t, 50, 40, 0), "r.png"), ObjectEdit{})
	if err != nil || res.Replaced[0].StaysInFile || res.Replaced[0].SharedWith == nil {
		t.Errorf("alone on its page, the answer reads %+v (%v), want it gone and an empty list", res.Replaced, err)
	}
}

// A path whose box is not a number sorts last and joins nothing, so the drawings beside it are grouped whole.
func TestAPathWithNoBoxDoesNotPartItsNeighbours(t *testing.T) {
	nan := math.NaN()
	a := drawnPath{box: [4]float64{0, 0, 10, 10}, curved: true, ops: 2, mcid: -1}
	b := a
	b.box = [4]float64{5, 5, 15, 15}
	bad := a
	bad.box = [4]float64{nan, 0, nan, 10}
	groups, _ := groupDrawnPaths([]drawnPath{a, bad, b})
	if len(groups) != 2 || len(groups[0].members) != 2 {
		t.Errorf("two touching paths beside one with no box group as %+v, want them together and it alone", groups)
	}
}
