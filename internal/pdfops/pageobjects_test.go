package pdfops

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"nib/internal/pdfread"
)

// The page objects' reader (PLAN-image-edit P01.S01): every picture with its address, and the hostile streams D3's
// pin names.

// objectsFixture is a one-page document drawing content. `/Im0` and `/Im1` are two different 2×2 images, `/Sm0` an
// image with a soft mask, `/Fm0` a form drawing `/Im0`, `/Fself` a form that draws itself and `/Fbad` a form whose
// content does not decode.
func objectsFixture(content string) []byte {
	return objectsFixtureWith(map[int]string{4: streamObject("", content)})
}

func objectsFixtureWith(over map[int]string) []byte {
	form := "q 100 0 0 80 0 0 cm /Im0 Do Q"
	self := "q /Fself Do Q"
	objs := map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Resources 9 0 R /Contents 4 0 R >>",
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6:  "<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 4 >>\nstream\nabcd\nendstream",
		7:  streamObject("/Type /XObject /Subtype /Form /BBox [0 0 100 80] /Resources << /XObject << /Im0 6 0 R >> >>", form),
		8:  "<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 4 >>\nstream\nwxyz\nendstream",
		9:  "<< /Font << /F1 5 0 R >> /XObject << /Im0 6 0 R /Im1 8 0 R /Sm0 10 0 R /Fm0 7 0 R /Fself 12 0 R /Fbad 13 0 R >> >>",
		10: "<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8 /SMask 11 0 R /Length 4 >>\nstream\nmnop\nendstream",
		11: "<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 4 >>\nstream\n\xff\x80\x80\xff\nendstream",
		12: streamObject("/Type /XObject /Subtype /Form /BBox [0 0 10 10] /Resources << /XObject << /Fself 12 0 R /Im0 6 0 R >> >>", "q 5 0 0 5 0 0 cm /Im0 Do Q "+self),
		13: "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Filter /FlateDecode /Length 8 >>\nstream\nnotflate\nendstream",
	}
	for k, v := range over {
		objs[k] = v
	}
	return assembleFixture(objs)
}

func readObjects(t *testing.T, pdf []byte, page int) PageObjects {
	t.Helper()
	objs, err := ReadPageObjects(pdf, page)
	if err != nil {
		t.Fatal(err)
	}
	return objs
}

// whys is each object's verdict, "ok" for an editable one.
func whys(objs PageObjects) []string {
	var out []string
	for _, o := range objs.Objects {
		if o.Editable {
			out = append(out, "ok")
		} else {
			out = append(out, o.Why)
		}
	}
	return out
}

func wantWhys(t *testing.T, objs PageObjects, want ...string) {
	t.Helper()
	if got := whys(objs); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("verdicts = %v, want %v", got, want)
	}
	for i, o := range objs.Objects {
		if o.Ordinal != i || len(o.Digest) != 64 || strings.ToLower(o.Digest) != o.Digest {
			t.Fatalf("object %d: ordinal %d digest %q — want ordinal %d and 64 lowercase hex", i, o.Ordinal, o.Digest, i)
		}
	}
}

func TestPageObjectsListEveryPictureWithItsAddressAndBox(t *testing.T) {
	pdf := objectsFixture("q 120 0 0 80 60 640 cm /Im0 Do Q\nq 60 0 0 60 300 100 cm " + inlineImage + " Q\nq 30 0 0 30 0 0 cm /Sm0 Do Q")
	objs := readObjects(t, pdf, 1)
	wantWhys(t, objs, "ok", "ok", "ok")
	o := objs.Objects[0]
	if o.Kind != "image" || objs.Objects[1].Kind != "inline" || o.QDepth != 1 || o.PlaceSpan == nil || o.InForm || o.Tagged || o.MCID != -1 || o.Stream != 0 {
		t.Fatalf("first object = %+v", o)
	}
	// 120×80 at (60, 640) on a 600×800 page: x 0.1-0.3, y from the top 1-(720/800)=0.1 to 0.2.
	want := MapRect{0.1, 0.1, 0.3, 0.2}
	for i := range want {
		if d := o.Box[i] - want[i]; d > 1e-9 || d < -1e-9 {
			t.Fatalf("box = %v, want %v", o.Box, want)
		}
	}
	if objs.Objects[0].Digest == objs.Objects[2].Digest {
		t.Fatal("two different pictures share a digest")
	}
}

// The ordinal is the picture's index among pictures, identical whether or not the map's pieces are kept (D2's pin).
func TestTheOrdinalIsTheSameWithAndWithoutShapes(t *testing.T) {
	pdf := objectsFixture("0 0 10 10 re f\nq 10 0 0 10 0 0 cm /Im0 Do Q\n1 1 5 5 re S\nq 10 0 0 10 50 50 cm /Im1 Do Q BT /F1 9 Tf (x) Tj ET")
	ctx, err := inspectionRead(pdf)
	if err != nil {
		t.Fatal(err)
	}
	pg := pageAt(ctx, pdfread.Pages(ctx), 1)
	a, err := readPageShapes(ctx, pg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := readPageDrawings(ctx, pg)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.images) != 2 || len(b.images) != 2 {
		t.Fatalf("images: %d with shapes, %d with drawings, want 2 each", len(a.images), len(b.images))
	}
	for i := range a.images {
		if a.images[i].span != b.images[i].span {
			t.Fatalf("image %d: span %v with shapes, %v with drawings", i, a.images[i].span, b.images[i].span)
		}
	}
	if a.images[1].seq == b.images[1].seq {
		t.Fatalf("seq is %d in both readers: the fixture no longer shows seq is not the ordinal", a.images[1].seq)
	}
}

func TestAClipBeforeTheDrawIsReadInsideAndOutsideQ(t *testing.T) {
	objs := readObjects(t, objectsFixture("0 0 300 300 re W n q 10 0 0 10 5 5 cm /Im0 Do Q\nq 0 0 300 300 re W n 10 0 0 10 5 5 cm /Im0 Do Q"), 1)
	wantWhys(t, objs, "ok", "ok")
	// The clip is no part of the address; the same cm at the same depth names the same operator.
	if objs.Objects[0].Digest != objs.Objects[1].Digest {
		t.Fatal("a clip changed the digest")
	}
}

func TestAnUnbalancedQIsIgnoredAndADrawOutsideAnyQIsNotIsolated(t *testing.T) {
	objs := readObjects(t, objectsFixture("Q Q q 10 0 0 10 5 5 cm /Im0 Do Q Q 10 0 0 10 5 5 cm /Im0 Do"), 1)
	wantWhys(t, objs, "ok", WhyNotIsolated)
	if objs.Objects[0].QDepth != 1 || objs.Objects[1].QDepth != 0 {
		t.Fatalf("depths %d, %d, want 1, 0", objs.Objects[0].QDepth, objs.Objects[1].QDepth)
	}
	if objs.Objects[0].Digest == objs.Objects[1].Digest {
		t.Fatal("the same cm and Do at two depths share a digest")
	}
}

func TestACmWithNoQIsNotIsolated(t *testing.T) {
	objs := readObjects(t, objectsFixture("10 0 0 10 5 5 cm /Im0 Do"), 1)
	wantWhys(t, objs, WhyNotIsolated)
	if objs.Objects[0].PlaceSpan == nil {
		t.Fatal("the cm that places it is not recorded")
	}
	// A cm outside the picture's q is not its placement: a q starts a level with none.
	if o := readObjects(t, objectsFixture("10 0 0 10 5 5 cm q /Im0 Do Q"), 1).Objects[0]; o.PlaceSpan != nil || !o.Editable {
		t.Fatalf("a picture whose cm is outside its q: %+v", o)
	}
}

// A marked picture in a document with NO structure tree is owned by nothing, so it is removable with its bracket
// (P01.S02 refined S01's blanket `tagged`: pageobjectedit_test.go holds the tree's cases).
func TestAPictureThatIsAMarkedSequencesOnlyContentIsRemovableWithNoTree(t *testing.T) {
	objs := readObjects(t, objectsFixture("/Figure <</MCID 3>> BDC q 10 0 0 10 5 5 cm /Im0 Do Q EMC\n/Artifact BMC q 10 0 0 10 50 5 cm /Im1 Do Q EMC"), 1)
	wantWhys(t, objs, "ok", "ok")
	if objs.Objects[0].MCID != 3 || !objs.Objects[0].Tagged || !objs.Objects[1].Decoration || objs.Objects[1].MCID != -1 {
		t.Fatalf("objects = %+v", objs.Objects)
	}
}

func TestAnInlineImageWhoseNeighboursAreOutsideQIsNotIsolated(t *testing.T) {
	objs := readObjects(t, objectsFixture("q 1 0 0 1 0 0 cm Q 10 0 0 10 5 5 cm "+inlineImage+" q Q"), 1)
	wantWhys(t, objs, WhyNotIsolated)
	if objs.Objects[0].Kind != "inline" {
		t.Fatalf("kind %q", objs.Objects[0].Kind)
	}
}

// A `/Contents` array split between the name and `Do`, and inside `BI … EI`: the spans index the joined content
// (`pdfread.PageContent`), so the reader sees one picture each and the delete probe removes it.
func TestAContentsArraySplitInsideAnOperatorReadsAsOneStream(t *testing.T) {
	for name, parts := range map[string][2]string{
		"name|Do":   {"q 10 0 0 10 5 5 cm /Im0", " Do Q\nq 10 0 0 10 50 50 cm /Im1 Do Q"},
		"BI|EI":     {"q 10 0 0 10 5 5 cm BI /W 2 /H 2 /CS /G /BPC 8 ID ", "abcd\nEI Q\nq 10 0 0 10 50 50 cm /Im1 Do Q"},
		"inside ID": {"q 10 0 0 10 5 5 cm BI /W 2 /H 2 /CS /G /BPC 8 ID ab", "cd\nEI Q\nq 10 0 0 10 50 50 cm /Im1 Do Q"},
	} {
		t.Run(name, func(t *testing.T) {
			pdf := objectsFixtureWith(map[int]string{
				3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Resources 9 0 R /Contents [4 0 R 14 0 R] >>",
				4:  streamObject("", parts[0]),
				14: streamObject("", parts[1]),
			})
			objs := readObjects(t, pdf, 1)
			if len(objs.Objects) != 2 {
				t.Fatalf("%d objects, want 2: %v", len(objs.Objects), whys(objs))
			}
			for ord := range objs.Objects {
				if v := probeDeletion(pdf, 1, objs, ord); v.outcome != deletedClean {
					t.Fatalf("deleting %d: %s (%s)", ord, v.outcome, v.why)
				}
			}
		})
	}
}

// Two pages share one `/Resources` dictionary and each draws `/Im0`: removing page 1's leaves page 2's address as it was.
func TestASharedResourcesDictionaryLeavesTheOtherPageAlone(t *testing.T) {
	pdf := objectsFixtureWith(map[int]string{
		2:  "<< /Type /Pages /Kids [3 0 R 15 0 R] /Count 2 >>",
		4:  streamObject("", "q 10 0 0 10 5 5 cm /Im0 Do Q"),
		15: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Resources 9 0 R /Contents 16 0 R >>",
		16: streamObject("", "q 10 0 0 10 5 5 cm /Im0 Do Q"),
	})
	before := readObjects(t, pdf, 2)
	v := probeDeletion(pdf, 1, readObjects(t, pdf, 1), 0)
	if v.outcome != deletedClean {
		t.Fatalf("delete: %s (%s)", v.outcome, v.why)
	}
	after := readObjects(t, v.out, 2)
	if len(after.Objects) != 1 || after.Objects[0].Digest != before.Objects[0].Digest || after.Objects[0].Box != before.Objects[0].Box {
		t.Fatalf("page 2 before %+v, after %+v", before.Objects, after.Objects)
	}
}

// A form drawn twice gives two records whose span is the same bytes of the form: both form-drawn, two addresses.
func TestAFormDrawnTwiceIsTwoFormDrawnObjects(t *testing.T) {
	objs := readObjects(t, objectsFixture("/Fm0 Do q 1 0 0 1 200 0 cm /Fm0 Do Q"), 1)
	wantWhys(t, objs, WhyFormDrawn, WhyFormDrawn)
	a, b := objs.Objects[0], objs.Objects[1]
	if a.span != b.span || a.Stream != 7 || b.Stream != 7 || !a.InForm {
		t.Fatalf("objects = %+v", objs.Objects)
	}
	if a.Digest == b.Digest {
		t.Fatal("two draws of a form at different depths share a digest")
	}
}

// A form that draws itself and a form whose content does not decode are counted, never passed over in silence; a
// form the BUDGET refuses (here, nested past the depth bound) refuses the page instead (`formWalkBudget`).
func TestAFormTheWalkCannotReadIsCountedOrRefused(t *testing.T) {
	objs := readObjects(t, objectsFixture("q 10 0 0 10 5 5 cm /Im0 Do Q /Fself Do /Fbad Do"), 1)
	if objs.FormsNotRead != 2 {
		t.Fatalf("FormsNotRead = %d, want 2 (the self-drawing form's second entry and the undecodable one)", objs.FormsNotRead)
	}
	wantWhys(t, objs, "ok", WhyFormDrawn)

	over := map[int]string{4: streamObject("", "/D0 Do")}
	res := "<< /XObject << "
	for i := 0; i <= maxFormDepth+1; i++ {
		res += fmt.Sprintf("/D%d %d 0 R ", i, 20+i)
		over[20+i] = streamObject(fmt.Sprintf("/Type /XObject /Subtype /Form /BBox [0 0 1 1] /Resources 19 0 R"), fmt.Sprintf("/D%d Do", i+1))
	}
	over[19] = res + ">> >>"
	over[9] = over[19]
	if _, err := ReadPageObjects(objectsFixtureWith(over), 1); err == nil || !strings.Contains(err.Error(), "deep") {
		t.Fatalf("a page nested past the depth bound read as %v, want the budget's refusal", err)
	}
}

// Two identical `Do`s share a digest and are told apart by ordinal; a right digest at the wrong ordinal names neither.
func TestTwoIdenticalDosAreTwoAddresses(t *testing.T) {
	objs := readObjects(t, objectsFixture("q 10 0 0 10 5 5 cm /Im0 Do /Im0 Do Q q 10 0 0 10 5 5 cm /Im1 Do Q"), 1)
	wantWhys(t, objs, "ok", "ok", "ok")
	d := objs.Objects[0].Digest
	if objs.Objects[1].Digest != d {
		t.Fatal("two identical Dos under one cm differ")
	}
	a, okA := ObjectAt(objs, 0, d)
	b, okB := ObjectAt(objs, 1, d)
	if !okA || !okB || a.span == b.span {
		t.Fatalf("ObjectAt by ordinal: %v %v, spans %v %v", okA, okB, a.span, b.span)
	}
	if _, ok := ObjectAt(objs, 2, d); ok {
		t.Fatal("the right digest at the wrong ordinal named the other picture")
	}
	if _, ok := ObjectAt(objs, 0, objs.Objects[2].Digest); ok {
		t.Fatal("the third picture's digest at ordinal 0 named the first")
	}
	// The same Do at the same depth under another cm is another operator.
	if o := readObjects(t, objectsFixture("q 10 0 0 10 5 5 cm /Im0 Do Q q 20 0 0 20 5 5 cm /Im0 Do Q"), 1).Objects; o[0].Digest == o[1].Digest {
		t.Fatal("two different cms give one digest")
	}
	// Removing the second leaves the first and the third, as they were.
	if v := probeDeletion(objectsFixture("q 10 0 0 10 5 5 cm /Im0 Do /Im0 Do Q q 10 0 0 10 5 5 cm /Im1 Do Q"), 1, objs, 1); v.outcome != deletedClean {
		t.Fatalf("delete: %s (%s)", v.outcome, v.why)
	}
}

// One page drawing many copies of one picture: every ordinal distinct, every digest equal, each named by its ordinal.
func TestASeededDuplicateDigestPageIsAddressedByOrdinal(t *testing.T) {
	const n = 1000
	objs := readObjects(t, objectsFixture(strings.Repeat("q 2 0 0 2 10 10 cm /Im0 Do Q\n", n)), 1)
	if len(objs.Objects) != n {
		t.Fatalf("%d objects, want %d", len(objs.Objects), n)
	}
	seen := map[opSpan]bool{}
	for i, o := range objs.Objects {
		got, ok := ObjectAt(objs, i, objs.Objects[0].Digest)
		if !ok || got.span != o.span || seen[o.span] {
			t.Fatalf("ordinal %d: %v %+v", i, ok, got)
		}
		seen[o.span] = true
	}
}

// tenThousandImages is one page drawing 10,000 DISTINCT image XObjects, each its own bytes and name.
func tenThousandImages() []byte {
	const n = 10_000
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Resources 5 0 R /Contents 4 0 R >>",
	}
	var content, res strings.Builder
	res.WriteString("<< /XObject << ")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&res, "/I%d %d 0 R ", i, 10+i)
		fmt.Fprintf(&content, "q 5 0 0 5 %d %d cm /I%d Do Q\n", i%100*6, i/100*8, i)
		objs[10+i] = fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 4 >>\nstream\n%04d\nendstream", i%10000)
	}
	res.WriteString(">> >>")
	objs[4], objs[5] = streamObject("", content.String()), res.String()
	return assembleFixture(objs)
}

// The bound's measurement (T02): `maxPageImages`' comment states what this prints.
func TestTenThousandDistinctImagesAreReadWithinTheCeiling(t *testing.T) {
	pdf := tenThousandImages()
	var m0, m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m0)
	start := time.Now()
	objs, err := ReadPageObjects(pdf, 1)
	took := time.Since(start)
	runtime.ReadMemStats(&m1)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs.Objects) != 10_000 {
		t.Fatalf("%d objects, want 10000", len(objs.Objects))
	}
	digests := map[string]bool{}
	for _, o := range objs.Objects {
		digests[o.Digest] = true
	}
	if len(digests) != 10_000 {
		t.Fatalf("%d distinct digests, want 10000", len(digests))
	}
	// The walk alone, over a context already read: what the cap bounds (pdfcpu's parse of 10,000 objects it does not).
	ctx, err := inspectionRead(pdf)
	if err != nil {
		t.Fatal(err)
	}
	pg := pageAt(ctx, pdfread.Pages(ctx), 1)
	var w0, w1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&w0)
	wstart := time.Now()
	if _, err := readPageDrawings(ctx, pg); err != nil {
		t.Fatal(err)
	}
	wtook := time.Since(wstart)
	runtime.ReadMemStats(&w1)
	t.Logf("10,000 distinct images: ReadPageObjects %v, %.1f MB allocated; the walk alone %v, %.1f MB allocated",
		took, float64(m1.TotalAlloc-m0.TotalAlloc)/1e6, wtook, float64(w1.TotalAlloc-w0.TotalAlloc)/1e6)
	if took > 20*time.Second {
		t.Fatalf("10,000 images took %v", took)
	}
}

// Past the ceiling the read refuses with the count and the cap — never a list cut short.
func TestAPagePastTheImageCeilingIsRefusedWithItsCount(t *testing.T) {
	n := maxPageImages + 1
	_, err := ReadPageObjects(objectsFixture(strings.Repeat("q /Im0 Do Q ", n)), 1)
	if !errors.Is(err, ErrTooManyImages) || !strings.Contains(err.Error(), fmt.Sprint(n)) || !strings.Contains(err.Error(), fmt.Sprint(maxPageImages)) {
		t.Fatalf("err = %v, want ErrTooManyImages naming %d and %d", err, n, maxPageImages)
	}
	if objs, err := ReadPageObjects(objectsFixture(strings.Repeat("q /Im0 Do Q ", maxPageImages)), 1); err != nil || len(objs.Objects) != maxPageImages {
		t.Fatalf("at the cap: %v, %d objects", err, len(objs.Objects))
	}
}

// The hostile fixtures through the instrument's own code path (T07's tier-1 floor): every editable picture deletes
// cleanly, and the oracle sees what it must.
func TestObjectDeleteOverTheHostileFixtures(t *testing.T) {
	fixtures := []string{
		"q 120 0 0 80 60 640 cm /Im0 Do Q\nq 60 0 0 60 300 100 cm " + inlineImage + " Q\nq 30 0 0 30 0 0 cm /Sm0 Do Q",
		"0 0 300 300 re W n q 10 0 0 10 5 5 cm /Im0 Do Q\nq 0 0 300 300 re W n 10 0 0 10 5 5 cm /Im1 Do Q",
		"Q Q q 10 0 0 10 5 5 cm /Im0 Do Q Q BT /F1 9 Tf 10 10 Td (text) Tj ET",
		"/Artifact BMC q 10 0 0 10 50 5 cm /Im1 Do Q EMC /Fm0 Do q 10 0 0 10 5 5 cm /Im0 Do /Im0 Do Q",
	}
	var tally deleteTally
	for _, c := range fixtures {
		tally.document(objectsFixture(c), 4, 8)
	}
	if tally.pictures < 10 || tally.attempted < 9 || tally.clean != tally.attempted {
		t.Fatalf("hostile fixtures: %s", tally.summary())
	}
	// The residue and checker halves ran, not only the oracle (code-review 2026-11-09, P01.S01 #5).
	if tally.inFile+tally.notInFile+tally.residueUnread == 0 || tally.checked+tally.uncheckable == 0 {
		t.Fatalf("the residue or checker comparison never ran: %s", tally.summary())
	}
	if tally.notEditable[WhyFormDrawn] != 1 {
		t.Fatalf("form-drawn count %d, want 1: %s", tally.notEditable[WhyFormDrawn], tally.summary())
	}
}

// A picture a form draws before a `cm` of its own has no placement: the page's `cm` that placed the form is in another
// stream, so naming its span would point the address at bytes the picture's stream does not hold (code-review
// 2026-11-09, P01.S01 #1).
func TestAPictureInAFormDoesNotInheritThePagesPlacement(t *testing.T) {
	bare := streamObject("/Type /XObject /Subtype /Form /BBox [0 0 100 80] /Resources << /XObject << /Im0 6 0 R >> >>", "/Im0 Do")
	objs := readObjects(t, objectsFixtureWith(map[int]string{
		4: streamObject("", "q 2 0 0 2 0 0 cm /Fm0 Do Q"),
		7: bare,
	}), 1)
	if len(objs.Objects) != 1 || !objs.Objects[0].InForm {
		t.Fatalf("objects = %+v", objs.Objects)
	}
	if objs.Objects[0].PlaceSpan != nil {
		t.Fatalf("the form's picture names the page's cm %v as its placement", *objs.Objects[0].PlaceSpan)
	}
}
