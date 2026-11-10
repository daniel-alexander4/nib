package pdfops

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
)

// The page objects' edit door (PLAN-image-edit P01.S02): what a removal takes, what it keeps, and every refusal by
// its cause.

// plantedImage is a 4×4 grey image whose sixteen bytes are pattern, so the bytes can be looked for after a removal.
func plantedImage(pattern string, extra string) string {
	if len(pattern) != 16 {
		panic("a planted image is sixteen bytes")
	}
	return fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width 4 /Height 4 /ColorSpace /DeviceGray /BitsPerComponent 8%s /Length 16 >>\nstream\n%s\nendstream", extra, pattern)
}

const (
	secretA = "NIBPLANTEDIMAGEA"
	secretB = "NIBPLANTEDIMAGEB"
	secretM = "NIBPLANTEDSMASK!"
)

// removeFixture is a one-page document: `/Im0` (secretA) and `/Im1` (secretB) drawn each in its own q, and text.
func removeFixture(content string, over map[int]string) []byte {
	objs := map[int]string{
		4: streamObject("", content),
		6: plantedImage(secretA, ""),
		8: plantedImage(secretB, ""),
	}
	for k, v := range over {
		objs[k] = v
	}
	return objectsFixtureWith(objs)
}

const twoPicturesAndText = "q 100 0 0 80 10 10 cm /Im0 Do Q\nq 50 0 0 40 200 200 cm /Im1 Do Q\nBT /F1 12 Tf 10 700 Td (Hello) Tj ET"

func removeOne(t *testing.T, pdf []byte, page, ordinal int) ([]byte, ObjectEditResult) {
	t.Helper()
	objs := readObjects(t, pdf, page)
	out, res, err := EditPageObjects(pdf, []ObjectEdit{{Kind: ObjectEditDelete, Page: page, Ordinal: ordinal, Digest: objs.Objects[ordinal].Digest}})
	if err != nil {
		t.Fatalf("removing picture %d of page %d: %v", ordinal, page, err)
	}
	return out, res
}

func wantRefusal(t *testing.T, err error, cause string) {
	t.Helper()
	var ref *ObjectRefusal
	if !errors.As(err, &ref) || ref.Cause != cause {
		t.Fatalf("want a %s refusal, got %v", cause, err)
	}
	if ObjectCauseWords(cause) == "" || !strings.Contains(err.Error(), ObjectCauseWords(cause)) {
		t.Fatalf("the %s refusal does not say its sentence: %q", cause, err)
	}
}

func TestRemovingAPictureTakesItsOperatorAndItsBytesAndNothingElse(t *testing.T) {
	pdf := removeFixture(twoPicturesAndText, nil)
	before := readObjects(t, pdf, 1)
	out, res := removeOne(t, pdf, 1, 0)
	after := readObjects(t, out, 1)
	if len(after.Objects) != 1 || after.Objects[0].Digest != before.Objects[1].Digest {
		t.Fatalf("after removing picture 1 the page lists %+v, want only the second picture", after.Objects)
	}
	if got := strings.Join(pageTexts(t, out, 1), "|"); got != "Hello" {
		t.Fatalf("the page's text is %q after the removal", got)
	}
	decoded := everyDecodedByte(t, out)
	if bytes.Contains(decoded, []byte(secretA)) {
		t.Fatal("the removed picture's bytes are still in the file: the prune did not take its resource entry")
	}
	if !bytes.Contains(decoded, []byte(secretB)) {
		t.Fatal("the picture that was kept is gone from the file")
	}
	if len(res.Removed) != 1 || res.Removed[0].Kind != "image" || len(res.Removed[0].SharedWith) != 0 || res.Removed[0].ElementRemoved {
		t.Fatalf("answer = %+v", res)
	}
	// q, cm and Q stay (D3): only the painting operator went.
	src := string(pageContentOf(t, out, 1))
	if strings.Contains(src, "/Im0 Do") || !strings.Contains(src, "q 100 0 0 80 10 10 cm") {
		t.Fatalf("content after the removal: %q", src)
	}
}

func pageTexts(t *testing.T, pdf []byte, page int) []string {
	t.Helper()
	ctx, err := inspectionRead(pdf)
	if err != nil {
		t.Fatal(err)
	}
	pr, err := readPageDrawings(ctx, pageAt(ctx, pdfread.Pages(ctx), page))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range pr.runs {
		out = append(out, r.text)
	}
	return out
}

// twoPagesDrawingIm0 is a two-page document whose pages share one `/Resources` dictionary, both drawing `/Im0`.
func twoPagesDrawingIm0(second string) []byte {
	return assembleFixture(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R 13 0 R] /Count 2 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Resources 9 0 R /Contents 4 0 R /Thumb 15 0 R >>",
		4:  streamObject("", "q 100 0 0 80 10 10 cm /Im0 Do Q"),
		6:  plantedImage(secretA, ""),
		9:  "<< /XObject << /Im0 6 0 R >> >>",
		13: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Resources 9 0 R /Contents 14 0 R >>",
		14: streamObject("", second),
		15: plantedImage(secretB, ""),
	})
}

func TestAPictureAnotherPageDrawsStaysInTheFileAndIsSaid(t *testing.T) {
	out, res := removeOne(t, twoPagesDrawingIm0("q 30 0 0 30 0 0 cm /Im0 Do Q"), 1, 0)
	if !bytes.Contains(everyDecodedByte(t, out), []byte(secretA)) {
		t.Fatal("page 2 still draws the picture, and its bytes left the file")
	}
	if got := res.Removed[0].SharedWith; len(got) != 1 || got[0] != 2 {
		t.Fatalf("SharedWith = %v, want [2]", got)
	}
	if n := len(readObjects(t, out, 2).Objects); n != 1 {
		t.Fatalf("page 2 draws %d pictures after page 1's was removed", n)
	}
}

func TestTheEditedPagesThumbnailGoes(t *testing.T) {
	pdf := twoPagesDrawingIm0("BT ET")
	out, res := removeOne(t, pdf, 1, 0)
	if bytes.Contains(everyDecodedByte(t, out), []byte(secretB)) {
		t.Fatal("the edited page's /Thumb (secretB) is still in the file, still showing the picture")
	}
	if bytes.Contains(everyDecodedByte(t, out), []byte(secretA)) || len(res.Removed[0].SharedWith) != 0 {
		t.Fatalf("no other page draws the picture, and it stayed: %+v", res)
	}
}

func TestASoftMaskGoesWithItsImage(t *testing.T) {
	pdf := removeFixture("q 100 0 0 80 10 10 cm /Sm0 Do Q", map[int]string{
		10: plantedImage("NIBPLANTEDSMIMG!", " /SMask 11 0 R"),
		11: plantedImage(secretM, ""),
	})
	out, _ := removeOne(t, pdf, 1, 0)
	if bytes.Contains(everyDecodedByte(t, out), []byte(secretM)) {
		t.Fatal("the soft mask of the removed picture is still in the file")
	}
}

func TestALinkOverThePictureIsKeptAndCounted(t *testing.T) {
	pdf := removeFixture(twoPicturesAndText, map[int]string{
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Resources 9 0 R /Contents 4 0 R /Annots [20 0 R 21 0 R] >>",
		20: "<< /Type /Annot /Subtype /Link /Rect [20 20 60 60] /A << /S /URI /URI (https://example.org/) >> >>",
		21: "<< /Type /Annot /Subtype /Link /Rect [400 400 450 450] /A << /S /URI /URI (https://example.org/) >> >>",
	})
	out, res := removeOne(t, pdf, 1, 0)
	if res.Removed[0].LinksOver != 1 {
		t.Fatalf("LinksOver = %d, want 1 (the link inside the box, not the one elsewhere)", res.Removed[0].LinksOver)
	}
	ctx, err := inspectionRead(out)
	if err != nil {
		t.Fatal(err)
	}
	annots, _ := ctx.DereferenceArray(pageAt(ctx, pdfread.Pages(ctx), 1).Dict["Annots"])
	if len(annots) != 2 {
		t.Fatalf("the page has %d annotations after the removal, want both links kept", len(annots))
	}
}

func TestAnInlinePictureIsRemovedWhole(t *testing.T) {
	pdf := removeFixture("q 10 0 0 10 5 5 cm BI /W 2 /H 2 /CS /G /BPC 8 ID abcd EI Q\nq 50 0 0 40 200 200 cm /Im1 Do Q", nil)
	out, res := removeOne(t, pdf, 1, 0)
	if res.Removed[0].Kind != "inline" || strings.Contains(string(pageContentOf(t, out, 1)), "BI") {
		t.Fatalf("answer %+v, content %q", res, string(pageContentOf(t, out, 1)))
	}
}

func TestTwoPicturesOnOnePageInOneRequestAreOneWrite(t *testing.T) {
	pdf := removeFixture(twoPicturesAndText, nil)
	objs := readObjects(t, pdf, 1)
	out, res, err := EditPageObjects(pdf, []ObjectEdit{
		{Kind: ObjectEditDelete, Page: 1, Ordinal: 1, Digest: objs.Objects[1].Digest},
		{Kind: ObjectEditDelete, Page: 1, Ordinal: 0, Digest: objs.Objects[0].Digest},
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(readObjects(t, out, 1).Objects); n != 0 || len(res.Removed) != 2 || res.Removed[0].Ordinal != 1 {
		t.Fatalf("after removing both: %d pictures, answer %+v", n, res)
	}
}

// ---- refusals, one per cause ----

func TestAStaleAddressIsRefusedAndNothingIsWritten(t *testing.T) {
	pdf := removeFixture("q 10 0 0 10 5 5 cm /Im0 Do Q\nq 10 0 0 10 5 5 cm /Im0 Do Q", nil)
	objs := readObjects(t, pdf, 1)
	if objs.Objects[0].Digest != objs.Objects[1].Digest {
		t.Fatal("fixture: the two identical draws should share a digest")
	}
	for name, e := range map[string]ObjectEdit{
		"a wrong digest":          {Kind: ObjectEditDelete, Page: 1, Ordinal: 0, Digest: strings.Repeat("0", 64)},
		"an ordinal past the end": {Kind: ObjectEditDelete, Page: 1, Ordinal: 2, Digest: objs.Objects[0].Digest},
	} {
		out, _, err := EditPageObjects(pdf, []ObjectEdit{e})
		if out != nil {
			t.Fatalf("%s: bytes were returned", name)
		}
		wantRefusal(t, err, CauseObjectChanged)
	}
}

func TestAFormDrawnPictureIsRefused(t *testing.T) {
	pdf := removeFixture("q /Fm0 Do Q", nil)
	objs := readObjects(t, pdf, 1)
	_, _, err := EditPageObjects(pdf, []ObjectEdit{{Kind: ObjectEditDelete, Page: 1, Ordinal: 0, Digest: objs.Objects[0].Digest}})
	wantRefusal(t, err, WhyFormDrawn)
}

func TestAPictureDrawnOutsideAnyQIsRefused(t *testing.T) {
	pdf := removeFixture("10 0 0 10 5 5 cm /Im0 Do", nil)
	objs := readObjects(t, pdf, 1)
	_, _, err := EditPageObjects(pdf, []ObjectEdit{{Kind: ObjectEditDelete, Page: 1, Ordinal: 0, Digest: objs.Objects[0].Digest}})
	wantRefusal(t, err, WhyNotIsolated)
}

func TestARequestThatNamesNothingTooMuchOrTwiceIsRefused(t *testing.T) {
	pdf := removeFixture(twoPicturesAndText, nil)
	d := readObjects(t, pdf, 1).Objects[0].Digest
	many := make([]ObjectEdit, maxObjectEdits+1)
	for i := range many {
		many[i] = ObjectEdit{Kind: ObjectEditDelete, Page: 1, Ordinal: i, Digest: d}
	}
	for name, edits := range map[string][]ObjectEdit{
		"none":         nil,
		"too many":     many,
		"unknown kind": {{Kind: "recolour", Page: 1, Ordinal: 0, Digest: d}},
		"twice":        {{Kind: ObjectEditDelete, Page: 1, Ordinal: 0, Digest: d}, {Kind: ObjectEditDelete, Page: 1, Ordinal: 0, Digest: d}},
		"page 0":       {{Kind: ObjectEditDelete, Page: 0, Ordinal: 0, Digest: d}},
		"page 2 of 1":  {{Kind: ObjectEditDelete, Page: 2, Ordinal: 0, Digest: d}},
	} {
		_, _, err := EditPageObjects(pdf, edits)
		if err == nil {
			t.Fatalf("%s: accepted", name)
		}
		wantRefusal(t, err, CauseBadRequest)
	}
}

func TestADocumentThatDoesNotSurviveARewriteIsNotRewritable(t *testing.T) {
	// A page /AF reaching a file nothing else reaches: `pdfread.Write` refuses it (ADR-136), after the edit applied.
	pdf := removeFixture(twoPicturesAndText, map[int]string{
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Resources 9 0 R /Contents 4 0 R /AF [20 0 R] >>",
		20: "<< /Type /Filespec /F (a.csv) /UF (a.csv) /AFRelationship /Source /EF << /F 22 0 R >> >>",
		22: "<< /Type /EmbeddedFile /Length 3 >>\nstream\na,b\nendstream",
	})
	d := readObjects(t, pdf, 1).Objects[0].Digest
	_, _, err := EditPageObjects(pdf, []ObjectEdit{{Kind: ObjectEditDelete, Page: 1, Ordinal: 0, Digest: d}})
	wantRefusal(t, err, CauseNotRewritable)
	if !errors.Is(err, pdfread.ErrAssociatedFileLost) {
		t.Fatalf("the refusal lost what caused it: %v", err)
	}
}

// TestTheWrittenPageIsHeldToWhatTheRemovalLeaves is the after-check (D2's pin) on its own: each way a rewrite can
// differ — a picture too many, the wrong picture, a run of text gone, a run's text changed — is refused.
func TestTheWrittenPageIsHeldToWhatTheRemovalLeaves(t *testing.T) {
	plan := pageRemoval{page: 1, want: []string{"b", "c"}, text: []string{"Hello", "World"}}
	if err := rewriteAgrees(plan, []string{"b", "c"}, []string{"Hello", "World"}); err != nil {
		t.Fatalf("the page as planned is refused: %v", err)
	}
	for name, got := range map[string][2][]string{
		"a picture too many":   {{"a", "b", "c"}, {"Hello", "World"}},
		"the wrong picture":    {{"a", "c"}, {"Hello", "World"}},
		"a run of text gone":   {{"b", "c"}, {"Hello"}},
		"a run's text changed": {{"b", "c"}, {"Hello", "Wor"}},
	} {
		var ref *ObjectRefusal
		if err := rewriteAgrees(plan, got[0], got[1]); !errors.As(err, &ref) || ref.Cause != CauseRewriteDiffers {
			t.Errorf("%s: want a rewrite-differs refusal, got %v", name, err)
		}
	}
}

// ---- a tagged picture (D4) ----

// taggedFixture is a one-page tagged document: a Document element over figK (the Figure's /K) and a P owning MCID 1.
// content draws the page; root is the root's /K (default the Document).
func objectFigureFixture(content, figK, root string) []byte {
	if root == "" {
		root = "[31 0 R]"
	}
	return removeFixture(content, map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 30 0 R >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Resources 9 0 R /Contents 4 0 R /StructParents 0 >>",
		30: "<< /Type /StructTreeRoot /K " + root + " /ParentTree 34 0 R /ParentTreeNextKey 1 >>",
		31: "<< /Type /StructElem /S /Document /P 30 0 R /K [32 0 R 33 0 R] >>",
		32: "<< /Type /StructElem /S /Figure /P 31 0 R /Pg 3 0 R /Alt (A cat on a mat) /K " + figK + " >>",
		33: "<< /Type /StructElem /S /P /P 31 0 R /Pg 3 0 R /K 1 >>",
		34: "<< /Nums [0 [32 0 R 33 0 R]] >>",
	})
}

const figureThenText = "/Figure <</MCID 0>> BDC q 100 0 0 80 10 10 cm /Im0 Do Q EMC\n/P <</MCID 1>> BDC BT /F1 12 Tf 10 700 Td (Hello) Tj ET EMC"

func TestAFiguresOnlyPictureGoesWithItsElement(t *testing.T) {
	pdf := objectFigureFixture(figureThenText, "0", "")
	objs := readObjects(t, pdf, 1)
	if !objs.Objects[0].Editable {
		t.Fatalf("a Figure that holds only this picture is listed %q", objs.Objects[0].Why)
	}
	out, res := removeOne(t, pdf, 1, 0)
	if r := res.Removed[0]; !r.ElementRemoved || r.Alt != "A cat on a mat" {
		t.Fatalf("answer = %+v", r)
	}
	tree, err := ReadStructure(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range tree.Elements {
		if e.Kind == "Figure" {
			t.Fatalf("the Figure is still in the tree: %+v", tree.Elements)
		}
	}
	if len(tree.Elements) != 2 {
		t.Fatalf("tree after = %+v, want the Document and the P", tree.Elements)
	}
	src := string(pageContentOf(t, out, 1))
	if strings.Contains(src, "/Figure") || !strings.Contains(src, "/P <</MCID 1>> BDC") {
		t.Fatalf("the Figure's bracket was not taken, or the P's was: %q", src)
	}
	if !carryIsComplete(out) {
		t.Fatal("the completeness gate does not agree the tree describes what the page draws")
	}
	ctx, err := inspectionRead(out)
	if err != nil {
		t.Fatal(err)
	}
	live := livePageObjects(ctx)
	st, err := readStructTree(ctx, live)
	if err != nil {
		t.Fatal(err)
	}
	if owner := parentTreeSlotOwner(ctx, st, 0, 0); owner != 0 {
		t.Fatalf("the parent tree's slot 0 still names object %d", owner)
	}
	if d := checkStructConsistency(ctx, st); len(d) != 0 {
		t.Fatalf("the tree contradicts itself after the removal: %+v", d)
	}
}

func TestAFigureThatHoldsMoreIsRefusedAndSaysSo(t *testing.T) {
	cases := map[string][]byte{
		"its element holds a second content item": objectFigureFixture(figureThenText+"\n/Figure <</MCID 2>> BDC BT /F1 9 Tf 10 50 Td (Caption) Tj ET EMC", "[0 2]", ""),
		"its sequence draws text too":             objectFigureFixture("/Figure <</MCID 0>> BDC q 100 0 0 80 10 10 cm /Im0 Do Q BT /F1 9 Tf 10 50 Td (Caption) Tj ET EMC\n/P <</MCID 1>> BDC BT /F1 12 Tf 10 700 Td (Hello) Tj ET EMC", "0", ""),
	}
	for name, pdf := range cases {
		objs := readObjects(t, pdf, 1)
		if objs.Objects[0].Why != WhyTaggedShared {
			t.Fatalf("%s: listed %q", name, objs.Objects[0].Why)
		}
		_, _, err := EditPageObjects(pdf, []ObjectEdit{{Kind: ObjectEditDelete, Page: 1, Ordinal: 0, Digest: objs.Objects[0].Digest}})
		wantRefusal(t, err, WhyTaggedShared)
	}
}

func TestTheLastElementOfTheTreeIsRefused(t *testing.T) {
	pdf := removeFixture("/Figure <</MCID 0>> BDC q 100 0 0 80 10 10 cm /Im0 Do Q EMC", map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 30 0 R >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Resources 9 0 R /Contents 4 0 R /StructParents 0 >>",
		30: "<< /Type /StructTreeRoot /K [32 0 R] /ParentTree 34 0 R /ParentTreeNextKey 1 >>",
		32: "<< /Type /StructElem /S /Figure /P 30 0 R /Pg 3 0 R /Alt (A cat) /K 0 >>",
		34: "<< /Nums [0 [32 0 R]] >>",
	})
	objs := readObjects(t, pdf, 1)
	_, _, err := EditPageObjects(pdf, []ObjectEdit{{Kind: ObjectEditDelete, Page: 1, Ordinal: 0, Digest: objs.Objects[0].Digest}})
	wantRefusal(t, err, WhyTaggedLast)
}

func TestAMarkedPictureNoTreeOwnsIsRemovedWithItsBracket(t *testing.T) {
	pdf := removeFixture("/Figure <</MCID 3>> BDC q 10 0 0 10 5 5 cm /Im0 Do Q EMC", nil)
	out, res := removeOne(t, pdf, 1, 0)
	if res.Removed[0].ElementRemoved || strings.Contains(string(pageContentOf(t, out, 1)), "BDC") {
		t.Fatalf("answer %+v, content %q", res, string(pageContentOf(t, out, 1)))
	}
}

// TestEveryObjectCauseHasItsWords pins the causes: each has one sentence, and the count is fixed so dropping or
// adding one is a decision made here.
func TestEveryObjectCauseHasItsWords(t *testing.T) {
	want := []string{CauseBadRequest, CauseCeremony, WhyFormDrawn, CauseNotRewritable, WhyNotIsolated, CauseObjectChanged,
		WhyOffPage, CauseRewriteDiffers, CauseSigned, WhyTagged, WhyTaggedLast, WhyTaggedShared, WhyUnreadableBox}
	got := objectCauses()
	if len(got) != 13 || len(want) != 13 {
		t.Fatalf("%d causes, want 13: %v", len(got), got)
	}
	for _, c := range want {
		if ObjectCauseWords(c) == "" {
			t.Fatalf("cause %q has no words", c)
		}
	}
}

// removeFirstPicture removes page 1's first removable picture by the address a read hands out: the census tables'
// drive. A fixture with none is an error, which every census records as "not exercised".
func removeFirstPicture(pdf []byte) ([]byte, error) {
	objs, err := ReadPageObjects(pdf, 1)
	if err != nil {
		return nil, err
	}
	for _, o := range objs.Objects {
		if o.Editable {
			out, _, err := EditPageObjects(pdf, []ObjectEdit{{Kind: ObjectEditDelete, Page: 1, Ordinal: o.Ordinal, Digest: o.Digest}})
			return out, err
		}
	}
	return nil, errors.New("the fixture draws no picture an edit may remove")
}

// A shading inside the Figure's sequence is content the reader's lists keep no span for (`sh` is only counted): the
// picture is not the sequence's alone, and the element is not deleted with it (code-review 2026-11-09).
func TestAShadingInsideTheFiguresSequenceKeepsItsElement(t *testing.T) {
	pdf := objectFigureFixture("/Figure <</MCID 0>> BDC /Sh0 sh q 100 0 0 80 10 10 cm /Im0 Do Q EMC\n/P <</MCID 1>> BDC BT /F1 12 Tf 10 700 Td (Hello) Tj ET EMC", "0", "")
	objs := readObjects(t, pdf, 1)
	if objs.Objects[0].Why != WhyTaggedShared {
		t.Fatalf("listed %q, want tagged-shared", objs.Objects[0].Why)
	}
	_, _, err := EditPageObjects(pdf, []ObjectEdit{{Kind: ObjectEditDelete, Page: 1, Ordinal: 0, Digest: objs.Objects[0].Digest}})
	wantRefusal(t, err, WhyTaggedShared)
}

// Two top-level Figures, the tree's only elements, removed in one request: each verdict alone sees the other, so the
// page's own count is what refuses — by its cause, not as a document that will not write.
func TestTwoTopLevelFiguresThatAreTheWholeTreeAreRefusedAsTheLast(t *testing.T) {
	pdf := removeFixture("/Figure <</MCID 0>> BDC q 100 0 0 80 10 10 cm /Im0 Do Q EMC\n/Figure <</MCID 1>> BDC q 50 0 0 40 200 200 cm /Im1 Do Q EMC", map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 30 0 R >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Resources 9 0 R /Contents 4 0 R /StructParents 0 >>",
		30: "<< /Type /StructTreeRoot /K [32 0 R 33 0 R] /ParentTree 34 0 R /ParentTreeNextKey 1 >>",
		32: "<< /Type /StructElem /S /Figure /P 30 0 R /Pg 3 0 R /Alt (A) /K 0 >>",
		33: "<< /Type /StructElem /S /Figure /P 30 0 R /Pg 3 0 R /Alt (B) /K 1 >>",
		34: "<< /Nums [0 [32 0 R 33 0 R]] >>",
	})
	objs := readObjects(t, pdf, 1)
	if !objs.Objects[0].Editable || !objs.Objects[1].Editable {
		t.Fatalf("each Figure alone is removable, listed %+v", whys(objs))
	}
	_, _, err := EditPageObjects(pdf, []ObjectEdit{
		{Kind: ObjectEditDelete, Page: 1, Ordinal: 0, Digest: objs.Objects[0].Digest},
		{Kind: ObjectEditDelete, Page: 1, Ordinal: 1, Digest: objs.Objects[1].Digest},
	})
	wantRefusal(t, err, WhyTaggedLast)
	if _, _, err := EditPageObjects(pdf, []ObjectEdit{{Kind: ObjectEditDelete, Page: 1, Ordinal: 0, Digest: objs.Objects[0].Digest}}); err != nil {
		t.Fatalf("one of the two alone: %v", err)
	}
}

// A page pruned because it named the removed picture without drawing it is held by the after-check too: its pictures
// and text are what they were.
func TestAPagePrunedForNamingThePictureIsHeldUnchanged(t *testing.T) {
	pdf := twoPagesDrawingIm0("BT ET")
	_, _, err := EditPageObjects(pdf, []ObjectEdit{{Kind: ObjectEditDelete, Page: 1, Ordinal: 0, Digest: readObjects(t, pdf, 1).Objects[0].Digest}})
	if err != nil {
		t.Fatal(err)
	}
	// The plan the door verifies against carries page 2, unchanged, beside page 1.
	var plans []pageRemoval
	if _, err := writeMutated(pdf, func(ctx *model.Context) error {
		pages, err := groupObjectEdits([]ObjectEdit{{Kind: ObjectEditDelete, Page: 1, Ordinal: 0, Digest: readObjects(t, pdf, 1).Objects[0].Digest}})
		if err != nil {
			return err
		}
		plans, err = removeObjects(ctx, pages)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var held bool
	for _, p := range plans {
		if p.page == 2 && len(p.removed) == 0 {
			held = true
		}
	}
	if !held {
		t.Fatalf("the pruned page 2 is not in the after-check's plan: %+v", plans)
	}
}

// A picture a page draws through resources it INHERITS from the page tree: the page is given its own, pruned, and the
// parent node's entry goes too, or the writer keeps the picture through the node (the corpus's 27, 2026-11-09).
func TestAPictureNamedOnlyByAParentPagesNodeLeavesTheFile(t *testing.T) {
	pdf := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 /Resources << /XObject << /Im0 6 0 R /Im1 8 0 R >> >> >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Contents 4 0 R >>",
		4: streamObject("", "q 100 0 0 80 10 10 cm /Im0 Do Q\nq 50 0 0 40 200 200 cm /Im1 Do Q"),
		6: plantedImage(secretA, ""),
		8: plantedImage(secretB, ""),
	})
	out, res := removeOne(t, pdf, 1, 0)
	if len(res.Removed[0].SharedWith) != 0 {
		t.Fatalf("SharedWith = %v", res.Removed[0].SharedWith)
	}
	decoded := everyDecodedByte(t, out)
	if bytes.Contains(decoded, []byte(secretA)) {
		t.Fatal("the removed picture is still in the file, held by the parent /Pages node's resources")
	}
	if !bytes.Contains(decoded, []byte(secretB)) || len(readObjects(t, out, 1).Objects) != 1 {
		t.Fatal("the kept picture did not survive")
	}
}

// The form's default resources name the picture: they are the form's, so the picture stays in the file, and the answer
// says so (`StaysInFile`) where no page does.
func TestAPictureTheFormsDefaultsNameStaysInTheFileAndIsSaid(t *testing.T) {
	pdf := removeFixture(twoPicturesAndText, map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [21 0 R] /DR 20 0 R >> >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Resources 9 0 R /Contents 4 0 R /Annots [21 0 R] >>",
		20: "<< /XObject << /Im0 6 0 R >> >>",
		21: "<< /Type /Annot /Subtype /Widget /FT /Tx /T (name) /DA (/Helv 0 Tf 0 g) /Rect [300 300 400 320] /P 3 0 R >>",
	})
	out, res := removeOne(t, pdf, 1, 0)
	if r := res.Removed[0]; len(r.SharedWith) != 0 || !r.StaysInFile {
		t.Fatalf("answer = %+v, want no page and the picture staying in the file", r)
	}
	if !bytes.Contains(everyDecodedByte(t, out), []byte(secretA)) {
		t.Fatal("the form's default resources were pruned")
	}
	_, plain := removeOne(t, removeFixture(twoPicturesAndText, nil), 1, 0)
	if plain.Removed[0].StaysInFile {
		t.Fatal("a picture nothing else names is said to stay in the file")
	}
}
