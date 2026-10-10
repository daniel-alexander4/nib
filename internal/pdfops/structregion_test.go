package pdfops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// Tagging a region of a page, and the reader of what is untagged — ADR-125.

// regionFixture is a tagged two-page document. Page 1 draws one owned paragraph ("Owned", MCID 0, element 8)
// and then content; page 2 draws second and has NO `/StructParents`. `/Im0` is an image, `/Fm0` a form that
// draws formBody.
func regionFixture(content, second string, pageExtra ...string) []byte {
	owned := "/P <</MCID 0>> BDC\nBT /F1 12 Tf 72 740 Td (Owned) Tj ET\nEMC\n"
	formBody := "BT /F1 12 Tf 300 300 Td (Inside) Tj ET"
	extra := strings.Join(pageExtra, " ")
	res := "/Resources << /Font << /F1 5 0 R >> /XObject << /Im0 6 0 R /Fm0 10 0 R >> >>"
	return assembleFixture(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
		2:  "<< /Type /Pages /Kids [3 0 R 11 0 R] /Count 2 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] " + res + " /Contents 4 0 R /StructParents 0 " + extra + " >>",
		4:  streamObject("", owned+content),
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6:  "<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 4 >>\nstream\nabcd\nendstream",
		7:  "<< /Type /StructTreeRoot /K [13 0 R] /ParentTree 9 0 R /ParentTreeNextKey 1 >>",
		8:  "<< /Type /StructElem /S /P /P 13 0 R /Pg 3 0 R /K [0] >>",
		9:  "<< /Nums [0 [8 0 R]] >>",
		10: streamObject("/Type /XObject /Subtype /Form /BBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >>", formBody),
		11: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] " + res + " /Contents 12 0 R >>",
		12: streamObject("", second),
		13: "<< /Type /StructElem /S /Document /P 7 0 R /K [8 0 R 14 0 R] >>",
		14: "<< /Type /StructElem /S /Sect /P 13 0 R /K [] >>",
	})
}

// The pieces the fixtures draw, each with the box it fills in user space.
const (
	bareText     = "BT /F1 12 Tf 72 700 Td (Bare words) Tj ET\n"
	decoration   = "/Artifact BMC BT /F1 12 Tf 72 660 Td (Decoration) Tj ET EMC\n"
	straightPath = "100 400 m 200 400 l 200 450 l S\n"
	curvedPath   = "300 400 m 320 460 380 460 400 400 c f\n"
)

var (
	bareTextBox   = [4]float64{70, 690, 200, 715}
	decorationBox = [4]float64{70, 650, 200, 675}
	imageBox      = [4]float64{72, 500, 172, 580}
	straightBox   = [4]float64{95, 395, 205, 455}
	curvedBox     = [4]float64{295, 395, 405, 465}
)

// onPage is a user-space box of a 612 by 792 page as a region: fractions of the page as displayed.
func onPage(b [4]float64) [4]float64 {
	return [4]float64{b[0] / 612, 1 - b[3]/792, b[2] / 612, 1 - b[1]/792}
}

// newElements is the elements after has and before had not, in tree order.
func newElements(before, after structureView) (out []viewElement) {
	had := byID(before)
	for _, e := range after.elements {
		if _, ok := had[e.id]; !ok {
			out = append(out, e)
		}
	}
	return out
}

// regioned applies one region edit and answers the written document and its one new element.
func regioned(t *testing.T, pdf []byte, ed structEdit) ([]byte, viewElement) {
	t.Helper()
	ed.kind = editRegion
	if ed.page == 0 {
		ed.page = 1
	}
	out := mustApply(t, pdf, ed)
	requireConsistent(t, "a region", out)
	if err := Validate(out); err != nil {
		t.Errorf("the document a region wrote does not validate: %v", err)
	}
	added := newElements(viewOf(t, pdf), viewOf(t, out))
	if len(added) != 1 {
		t.Fatalf("a region made %d new elements, want one", len(added))
	}
	return out, added[0]
}

var (
	bracketOpen  = regexp.MustCompile(`/Note <</MCID \d+>> BDC\n`)
	rewrittenArt = regexp.MustCompile(`/Note <</MCID \d+>> BDC `)
)

// unbracketed is content with every sequence a region wrote as a `Note` taken out again: a bracket's opener
// and the `EMC` it added, and a rewritten artifact's opener put back.
func unbracketed(content string) string {
	for {
		at := bracketOpen.FindStringIndex(content)
		if at == nil {
			break
		}
		rest := content[at[1]:]
		end := strings.Index(rest, "\nEMC")
		if end < 0 {
			return "a bracket with no EMC: " + content
		}
		content = content[:at[0]] + rest[:end] + rest[end+len("\nEMC"):]
	}
	return rewrittenArt.ReplaceAllString(content, "/Artifact BMC ")
}

// regionPage is page nr's content as the document holds it.
func regionPage(t *testing.T, pdf []byte, nr int) string {
	t.Helper()
	return string(pageContentOf(t, pdf, nr))
}

// TestARegionTagsBareTextAsOneElement — text the page leaves bare: bracketed at its own show operator under
// the new element's MCID, read back as the element's text, and nothing else on the page changed.
func TestARegionTagsBareTextAsOneElement(t *testing.T) {
	src := regionFixture(bareText+decoration, "")
	out, e := regioned(t, src, structEdit{value: "Note", rect: onPage(bareTextBox)})
	if e.text != "Bare words" || e.kind != "Note" || e.page != 1 || e.hasAlt {
		t.Errorf("the new element reads %q as %s on page %d (alt: %v)", e.text, e.kind, e.page, e.hasAlt)
	}
	got := regionPage(t, out, 1)
	if !strings.Contains(got, "BT /F1 12 Tf 72 700 Td /Note <</MCID 1>> BDC\n(Bare words) Tj\nEMC ET") {
		t.Errorf("the run is not bracketed at its own show operator:\n%s", got)
	}
	if unbracketed(got) != regionPage(t, src, 1) {
		t.Errorf("with the brackets taken out the page is not what it was:\n%q\nwas\n%q", unbracketed(got), regionPage(t, src, 1))
	}
	if arrays, _ := slotOwners(t, out); !equalInts(arrays[0], []int{8, e.id}) {
		t.Errorf("the page's ParentTree row reads %v, want the owned paragraph and then the new element", arrays[0])
	}
	if !strings.Contains(got, decoration) {
		t.Error("decoration outside the region changed")
	}
}

// TestARegionMakesAWholeArtifactSequenceContent — decoration: when every piece of the sequence is in the
// region its OPENER is rewritten and nothing is nested; when only part is, the edit is refused.
func TestARegionMakesAWholeArtifactSequenceContent(t *testing.T) {
	src := regionFixture(bareText+decoration, "")
	out, e := regioned(t, src, structEdit{value: "Note", rect: onPage(decorationBox)})
	if e.text != "Decoration" {
		t.Errorf("the new element reads %q", e.text)
	}
	got := regionPage(t, out, 1)
	if !strings.Contains(got, "/Note <</MCID 1>> BDC BT /F1 12 Tf 72 660 Td (Decoration) Tj ET EMC") || strings.Contains(got, "/Artifact") {
		t.Errorf("the artifact's opener was not rewritten in place:\n%s", got)
	}
	if unbracketed(got) != regionPage(t, src, 1) {
		t.Errorf("with the opener put back the page is not what it was:\n%q", unbracketed(got))
	}
	if n, err := UnmarkedTextRuns(out); err != nil || n != 1 {
		t.Errorf("%d run(s) are neither tagged nor decoration (%v) — want the one bare run the region left", n, err)
	}

	// Two runs in ONE artifact sequence, and a property list on its opener.
	two := "/Artifact <</Type /Pagination>> BDC BT /F1 12 Tf 72 660 Td (Left) Tj 300 0 Td (Right) Tj ET EMC\n"
	src = regionFixture(two, "")
	_, err := applyStructEdits(src, []structEdit{{kind: editRegion, value: "Note", page: 1, rect: onPage([4]float64{70, 650, 150, 675})}})
	if !errors.Is(err, ErrTagsReview) || !strings.Contains(err.Error(), "cuts through content page 1 marks as one piece of decoration (1 of its 2 pieces") || !strings.Contains(err.Error(), "widen") {
		t.Errorf("a region over half an artifact sequence was answered %v", err)
	}
	out, e = regioned(t, src, structEdit{value: "Note", rect: onPage([4]float64{70, 650, 450, 675})})
	if e.text != "LeftRight" && e.text != "Left Right" {
		t.Errorf("the whole sequence reads %q", e.text)
	}
	if got := regionPage(t, out, 1); !strings.Contains(got, "/Note <</MCID 1>> BDC BT /F1 12 Tf 72 660 Td (Left) Tj") || strings.Contains(got, "Pagination") {
		t.Errorf("the opener and its property list were not replaced:\n%s", got)
	}
	if arrays, _ := slotOwners(t, out); len(arrays[0]) != 2 {
		t.Errorf("a sequence made content took %d slots, want one MCID for the whole sequence: %v", len(arrays[0])-1, arrays[0])
	}
}

// TestARegionTagsAnImage — an image XObject at its `Do`, and an inline image with the `q … cm` and `Q` it is
// drawn between (`drawingBrackets.around`, ADR-122).
func TestARegionTagsAnImage(t *testing.T) {
	src := regionFixture(drawImage(72, 500, 100, 80)+drawInline(300, 500, 100, 80), "")
	out, e := regioned(t, src, structEdit{value: "Note", rect: onPage(imageBox)})
	got := regionPage(t, out, 1)
	if !strings.Contains(got, "cm /Note <</MCID 1>> BDC\n/Im0 Do\nEMC Q") {
		t.Errorf("the image is not bracketed at its own operator:\n%s", got)
	}
	if e.text != "" || !e.marked {
		t.Errorf("the image's element reads %q (owning content: %v)", e.text, e.marked)
	}
	if unbracketed(got) != regionPage(t, src, 1) {
		t.Error("with the brackets taken out the page is not what it was")
	}
	out, _ = regioned(t, src, structEdit{value: "Note", rect: onPage([4]float64{300, 500, 400, 580})})
	got = regionPage(t, out, 1)
	if !strings.Contains(got, "/Note <</MCID 1>> BDC\nq 100 0 0 80 300 500 cm "+inlineImage+" Q\nEMC") {
		t.Errorf("the inline image is not bracketed with its q … cm and Q:\n%s", got)
	}
	if unbracketed(got) != regionPage(t, src, 1) {
		t.Error("with the inline image's brackets taken out the page is not what it was")
	}
}

// TestARegionTagsAStraightPathAndACurvedOne — a painted path is taken whole, from its first construction
// operator through the operator that paints it; a curve is boxed by its control points; a path that only
// clips is never taken.
func TestARegionTagsAStraightPathAndACurvedOne(t *testing.T) {
	clip := "95 395 110 60 re W n\n"
	src := regionFixture(clip+straightPath+curvedPath, "")
	out, _ := regioned(t, src, structEdit{value: "Note", rect: onPage(straightBox)})
	got := regionPage(t, out, 1)
	if !strings.Contains(got, clip+"/Note <</MCID 1>> BDC\n100 400 m 200 400 l 200 450 l S\nEMC\n"+curvedPath) {
		t.Errorf("the straight path is not bracketed whole, and alone:\n%s", got)
	}
	if unbracketed(got) != regionPage(t, src, 1) {
		t.Error("with the brackets taken out the page is not what it was")
	}
	out, _ = regioned(t, src, structEdit{value: "Note", rect: onPage(curvedBox)})
	got = regionPage(t, out, 1)
	if !strings.Contains(got, straightPath+"/Note <</MCID 1>> BDC\n300 400 m 320 460 380 460 400 400 c f\nEMC") {
		t.Errorf("the curved path is not bracketed whole:\n%s", got)
	}
	// The curve's own ink reaches y 445; its control points reach 460, and they are the box: a region whose
	// top edge is at 425 holds the centre of 400..460 (430) and not of 400..445 (422.5)... so it is read by
	// the control points' box, as declared.
	if _, err := applyStructEdits(src, []structEdit{{kind: editRegion, value: "Note", page: 1, rect: onPage([4]float64{295, 428, 405, 470})}}); err != nil {
		t.Errorf("a region holding the centre of the curve's control box was answered %v", err)
	}
	// Only the clip is in this region.
	_, err := applyStructEdits(regionFixture(clip, ""), []structEdit{{kind: editRegion, value: "Note", page: 1, rect: onPage([4]float64{90, 390, 210, 460})}})
	if !errors.Is(err, ErrTagsReview) || !strings.Contains(err.Error(), "nothing untagged is in that region") {
		t.Errorf("a region holding only a clipping path was answered %v", err)
	}
}

// TestAMixedRegionIsOneElementOwningEveryPieceInContentOrder — text, decoration, an image and two paths in
// one region: one element, one MCID a piece, in the order the page draws them.
func TestAMixedRegionIsOneElementOwningEveryPieceInContentOrder(t *testing.T) {
	content := straightPath + bareText + drawImage(72, 500, 100, 80) + decoration + curvedPath
	src := regionFixture(content, "")
	out, e := regioned(t, src, structEdit{value: "Note", rect: [4]float64{0, 0.07, 1, 1}})
	if e.text != "Bare wordsDecoration" && e.text != "Bare words Decoration" {
		t.Errorf("the element reads %q", e.text)
	}
	got := regionPage(t, out, 1)
	want := []string{
		"/Note <</MCID 1>> BDC\n100 400 m", "/Note <</MCID 2>> BDC\n(Bare words) Tj", "/Note <</MCID 3>> BDC\n/Im0 Do",
		"/Note <</MCID 4>> BDC BT /F1 12 Tf 72 660", "/Note <</MCID 5>> BDC\n300 400 m",
	}
	at := 0
	for _, w := range want {
		i := strings.Index(got[at:], w)
		if i < 0 {
			t.Fatalf("%q is not in the page after offset %d — the pieces are not numbered in content order:\n%s", w, at, got)
		}
		at += i
	}
	if unbracketed(got) != regionPage(t, src, 1) {
		t.Errorf("with every bracket taken out the page is not what it was:\n%q", unbracketed(got))
	}
	if arrays, _ := slotOwners(t, out); !equalInts(arrays[0], []int{8, e.id, e.id, e.id, e.id, e.id}) {
		t.Errorf("the page's ParentTree row reads %v, want five slots naming the one new element", arrays[0])
	}
	_, kids := rawKids(t, out, e.id)
	if len(kids) != 5 {
		t.Errorf("the element's /K holds %d entries, want its five MCIDs", len(kids))
	}
	if strings.Contains(got, "/P <</MCID 0>> BDC\nBT /F1 12 Tf 72 740 Td /Note") {
		t.Error("the owned paragraph above the region was bracketed again")
	}
}

// TestPiecesNamesExactlyWhatIsTaken — a region given as pieces takes what lies inside ANY of them, nothing
// between them, and nothing that merely has its centre in one: a ground painted under the page is centred in
// the piece at the page's middle and is not part of it.
func TestPiecesNamesExactlyWhatIsTaken(t *testing.T) {
	src := regionFixture(bareText+decoration+straightPath+curvedPath, "")
	out, e := regioned(t, src, structEdit{value: "Note", pieces: [][4]float64{onPage(bareTextBox), onPage(curvedBox)},
		rect: [4]float64{0, 0, 1, 1}}) // the rect is not read when pieces are given
	if e.text != "Bare words" {
		t.Errorf("the element reads %q", e.text)
	}
	got := regionPage(t, out, 1)
	if !strings.Contains(got, decoration+straightPath+"/Note <</MCID 2>> BDC\n300 400 m") {
		t.Errorf("what lies between the two pieces was taken, or the second piece was not:\n%s", got)
	}

	ground := "0 0 612 792 re f\n"
	middle := "256 376 m 276 436 336 436 356 376 c f\n" // boxed 256..356 by 376..436, around the page's centre
	src = regionFixture(ground+middle, "")
	box := onPage([4]float64{255, 375, 357, 437})
	out, _ = regioned(t, src, structEdit{value: "Note", pieces: [][4]float64{box}})
	if got := regionPage(t, out, 1); strings.Count(got, "/Note <</MCID") != 1 || !strings.Contains(got, ground+"/Note <</MCID 1>> BDC\n256 376 m") {
		t.Errorf("a piece at the page's middle took the ground under the page with it:\n%s", got)
	}
	// The same rectangle DRAWN takes what is centred in it — the ground too.
	out, _ = regioned(t, src, structEdit{value: "Note", rect: box})
	if got := regionPage(t, out, 1); strings.Count(got, "/Note <</MCID") != 2 {
		t.Errorf("a drawn rectangle took %d piece(s), want the two centred in it:\n%s", strings.Count(got, "/Note <</MCID"), got)
	}
}

// TestARegionLeavesOwnedContentAlone — content an element owns is not taken and is not an error; a region
// holding nothing else is refused, and writes nothing.
func TestARegionLeavesOwnedContentAlone(t *testing.T) {
	src := regionFixture(bareText, "")
	whole := [4]float64{0, 0, 1, 1}
	out, e := regioned(t, src, structEdit{value: "Note", rect: whole})
	if e.text != "Bare words" {
		t.Errorf("the element reads %q, want the bare run and not the owned one", e.text)
	}
	if got := byID(viewOf(t, out))[8].text; got != "Owned" {
		t.Errorf("the owned paragraph reads %q now", got)
	}
	// An image and a path an element owns are left alone too: only the bare run beside them is taken.
	ownedDrawings := "/Figure <</MCID 1>> BDC\n" + drawImage(72, 500, 100, 80) + "EMC\n/Figure <</MCID 2>> BDC\n" + straightPath + "EMC\n"
	out2, e2 := regioned(t, regionFixture(ownedDrawings+bareText, ""), structEdit{value: "Note", rect: whole})
	if page := regionPage(t, out2, 1); strings.Count(page, "/Note <</MCID") != 1 || e2.text != "Bare words" {
		t.Errorf("a region over an owned image and an owned path took %d sequence(s) reading %q, want the one bare run:\n%s", strings.Count(page, "/Note <</MCID"), e2.text, page)
	}
	// A piece is judged by the part of it the page shows: a path running off the right edge, most of it out of
	// sight, is taken by a region over the part that is shown.
	offEdge := "500 300 m 1400 300 l 1400 340 l S\n" // shown from 500 to 612; its whole box is centred at 950, off the page
	if _, e3 := regioned(t, regionFixture(offEdge, ""), structEdit{value: "Note", rect: onPage([4]float64{490, 290, 612, 350})}); !e3.marked {
		t.Error("the path running off the page was not taken by a region over the part of it that is shown")
	}
	_, err := applyStructEdits(out, []structEdit{{kind: editRegion, value: "Note", page: 1, rect: whole}})
	if !errors.Is(err, ErrTagsReview) || !strings.Contains(err.Error(), "nothing untagged is in that region of page 1") {
		t.Errorf("a region over a fully tagged page was answered %v", err)
	}
}

// TestARegionThatCannotBeWrittenIsRefused — each refusal, driven by the request or the page it is for.
func TestARegionThatCannotBeWrittenIsRefused(t *testing.T) {
	plain := regionFixture(bareText+drawImage(72, 500, 100, 80), "")
	whole := [4]float64{0, 0, 1, 1}
	tooMany := make([][4]float64, maxRegionRects+1)
	for i := range tooMany {
		tooMany[i] = whole
	}
	for _, c := range []struct {
		name string
		pdf  []byte
		ed   structEdit
		want error
		says string
	}{
		{"a type that is not standard", plain, structEdit{value: "Paragraph", page: 1, rect: whole}, ErrTagsReview, "not a standard structure type"},
		{"an element named", plain, structEdit{value: "P", elem: 8, page: 1, rect: whole}, ErrTagsReview, "leave the element out"},
		{"page 0", plain, structEdit{value: "P", rect: whole}, ErrTagsReview, "page 0 is not a page of this document, which has 2"},
		{"a page past the end", plain, structEdit{value: "P", page: 3, rect: whole}, ErrTagsReview, "page 3 is not a page"},
		{"an empty rectangle", plain, structEdit{value: "P", page: 1}, ErrTagsReview, "a region is a rectangle on the page"},
		{"a rectangle with no width", plain, structEdit{value: "P", page: 1, rect: [4]float64{0.5, 0, 0.5, 1}}, ErrTagsReview, "a region is a rectangle"},
		{"a rectangle with no height", plain, structEdit{value: "P", page: 1, rect: [4]float64{0, 0.5, 1, 0.5}}, ErrTagsReview, "a region is a rectangle"},
		{"a rectangle past the page", plain, structEdit{value: "P", page: 1, rect: [4]float64{0, 0, 1.01, 1}}, ErrTagsReview, "a region is a rectangle"},
		{"a rectangle before the page", plain, structEdit{value: "P", page: 1, rect: [4]float64{-0.01, 0, 1, 1}}, ErrTagsReview, "a region is a rectangle"},
		{"one bad piece among good ones", plain, structEdit{value: "P", page: 1, pieces: [][4]float64{whole, {0.2, 0.9, 0.1, 1}}}, ErrTagsReview, "a region is a rectangle"},
		{"more pieces than a region names", plain, structEdit{value: "P", page: 1, pieces: tooMany}, ErrTagsReview, "at most 4096 pieces"},
		{"a figure with no description", plain, structEdit{value: "Figure", page: 1, rect: whole}, ErrTagsReview, "only with a description"},
		{"a figure described in spaces", plain, structEdit{value: "Figure", page: 1, rect: whole, alt: " \t"}, ErrTagsReview, "only with a description"},
		{"a parent the tree does not have", plain, structEdit{value: "P", page: 1, rect: whole, parent: 99}, ErrTagsStale, "element 99"},
		{"bare content a form draws", regionFixture("/Fm0 Do\n", ""), structEdit{value: "P", page: 1, rect: onPage([4]float64{290, 290, 420, 320})}, ErrTagsReview, "drawn inside a form XObject"},
		{"decoration a form draws", regionFixture("/Artifact BMC /Fm0 Do EMC\n", ""), structEdit{value: "P", page: 1, rect: onPage([4]float64{290, 290, 420, 320})}, ErrTagsReview, "drawn inside a form XObject"},
		{"decoration marked together with a form", regionFixture("/Artifact BMC 100 400 m 200 400 l 200 450 l S /Fm0 Do EMC\n", ""),
			structEdit{value: "P", page: 1, rect: onPage(straightBox)}, ErrTagsReview, "together with a shading or a form XObject"},
		{"decoration inside owned content", regionFixture("/P <</MCID 7>> BDC /Artifact BMC 100 400 m 200 400 l 200 450 l S EMC EMC\n", ""),
			structEdit{value: "P", page: 1, rect: onPage(straightBox)}, ErrTagsReview, "one owner's content sitting inside another's"},
		{"decoration holding another artifact", regionFixture("/Artifact BMC 100 400 m 200 400 l 200 450 l S /Artifact BMC 300 400 m 320 460 380 460 400 400 c f EMC EMC\n", ""),
			structEdit{value: "P", page: 1, rect: [4]float64{0, 0.2, 1, 1}}, ErrTagsReview, "one owner's content sitting inside another's"},
		{"decoration the page never closes", regionFixture("/Artifact BMC 100 400 m 200 400 l 200 450 l S\n", ""),
			structEdit{value: "P", page: 1, rect: onPage(straightBox)}, ErrTagsReview, "never closes"},
		{"nothing in the region", plain, structEdit{value: "P", page: 1, rect: [4]float64{0.8, 0.8, 0.9, 0.9}}, ErrTagsReview, "nothing untagged is in that region"},
	} {
		c.ed.kind = editRegion
		out, err := applyStructEdits(c.pdf, []structEdit{c.ed})
		if !errors.Is(err, c.want) || !strings.Contains(err.Error(), c.says) || out != nil {
			t.Errorf("%s: answered %v (wrote %d bytes) — want %v saying %q", c.name, err, len(out), c.want, c.says)
		}
	}

	// A document with no structure tree is the stale answer every edit gives.
	untagged := figureFixture(drawText(72, 700, "Untagged"))
	if _, err := EditStructure(untagged, []StructureEdit{{Kind: "region", Value: "P", Page: 1, Rect: whole}}); !errors.Is(err, ErrTagsStale) || !strings.Contains(err.Error(), "no structure tree") {
		t.Errorf("a region on an untagged document was answered %v", err)
	}
	// A figure WITH a description is written, and any type takes one when it is given.
	for _, typ := range []string{"Figure", "P"} {
		out, err := EditStructure(plain, []StructureEdit{{Kind: "region", Value: typ, Page: 1, Rect: onPage(imageBox), Alt: "A chart of ünits", Index: -1}})
		if err != nil {
			t.Fatalf("a described %s: %v", typ, err)
		}
		e := newElements(viewOf(t, plain), viewOf(t, out))[0]
		if !e.hasAlt || e.alt != "A chart of ünits" || e.kind != typ {
			t.Errorf("a described %s reads %q (has alt: %v) as %s", typ, e.alt, e.hasAlt, e.kind)
		}
	}
}

// TestARegionIsPlacedAtItsParentAndIndex — as a created tag is: among its parent's ELEMENT kids, the root for
// a parent of 0 or -1, last for an index left out; and its `/P` names where it is listed.
func TestARegionIsPlacedAtItsParentAndIndex(t *testing.T) {
	src := regionFixture(bareText, "")
	rect := onPage(bareTextBox)
	for _, c := range []struct {
		name          string
		parent, index int
		holder        int // the object whose /K lists it: 7 is the root
		want          []int
	}{
		{"first under the Document", 13, 0, 13, []int{-1, 8, 14}},
		{"between the Document's kids", 13, 1, 13, []int{8, -1, 14}},
		{"last under the Document", 13, -1, 13, []int{8, 14, -1}},
		{"past the end", 13, 9, 13, []int{8, 14, -1}},
		{"inside the empty section", 14, 0, 14, []int{-1}},
		{"at the top, parent 0", 0, 0, 7, []int{-1, 13}},
		{"at the top, parent -1", rootParent, -1, 7, []int{13, -1}},
	} {
		out, e := regioned(t, src, structEdit{value: "Note", rect: rect, parent: c.parent, index: c.index})
		ctx, kids := rawKids(t, out, c.holder)
		var got []int
		for _, k := range kids {
			nr := k.(interface{ String() string }).String()
			switch {
			case refTo(k, e.id):
				got = append(got, -1)
			default:
				var n int
				fmt.Sscanf(nr, "(%d", &n)
				got = append(got, n)
			}
		}
		if !equalInts(got, c.want) {
			t.Errorf("%s: the holder's /K reads %v, want %v (-1 is the new element)", c.name, got, c.want)
		}
		d, err := ctx.DereferenceDict(kids[indexOf(got, -1)])
		if err != nil || !refTo(d["P"], c.holder) {
			t.Errorf("%s: the new element's /P reads %v, want object %d", c.name, d["P"], c.holder)
		}
		if e.text != "Bare words" {
			t.Errorf("%s: the element reads %q", c.name, e.text)
		}
	}
}

func indexOf(xs []int, x int) int {
	for i, v := range xs {
		if v == x {
			return i
		}
	}
	return 0
}

// TestARegionGivesAPageItsStructParents — a page no element described has no `/ParentTree` row: the region
// gives it a key no row uses and writes the row.
func TestARegionGivesAPageItsStructParents(t *testing.T) {
	src := regionFixture("", "BT /F1 12 Tf 72 700 Td (Second page) Tj ET\n")
	out, e := regioned(t, src, structEdit{value: "Note", page: 2, rect: onPage(bareTextBox)})
	if e.text != "Second page" || e.page != 2 {
		t.Errorf("the element reads %q on page %d", e.text, e.page)
	}
	ctx := parsed(t, out)
	pg := pageAt(ctx, nil, 2)
	key, ok, _ := structParentsOf(ctx.XRefTable, pg.Dict)
	if !ok || key != 1 {
		t.Fatalf("page 2's /StructParents reads %d (a key: %v), want the next key, 1", key, ok)
	}
	if arrays, _ := slotOwners(t, out); !equalInts(arrays[1], []int{e.id}) || !equalInts(arrays[0], []int{8}) {
		t.Errorf("the ParentTree reads %v, want row 1 naming the new element and row 0 as it was", arrays)
	}
	if got := regionPage(t, out, 2); !strings.Contains(got, "/Note <</MCID 0>> BDC\n(Second page) Tj\nEMC") {
		t.Errorf("page 2 reads:\n%s", got)
	}
	if regionPage(t, out, 1) != regionPage(t, src, 1) {
		t.Error("page 1 changed")
	}
}

// TestARegionIsReadOnThePageAsItIsDisplayed — the rectangle is fractions of the DISPLAYED page (ADR-088): on
// a page turned a quarter and cropped, the region that holds a piece is where the piece is shown.
func TestARegionIsReadOnThePageAsItIsDisplayed(t *testing.T) {
	src := regionFixture(bareText+straightPath, "", "/Rotate 90 /CropBox [36 36 576 756]")
	ctx := parsed(t, src)
	sp := newDisplaySpace(pageAt(ctx, nil, 1))
	shown := sp.rect(bareTextBox)
	if sp.w != 720 || sp.h != 540 {
		t.Fatalf("setup: the page displays %g by %g", sp.w, sp.h)
	}
	_, e := regioned(t, src, structEdit{value: "Note", rect: shown})
	if e.text != "Bare words" {
		t.Errorf("the region where the text is shown took %q", e.text)
	}
	// The same numbers read as an unturned page's would be somewhere else: nothing is there.
	if _, err := applyStructEdits(src, []structEdit{{kind: editRegion, value: "Note", page: 1, rect: onPage(bareTextBox)}}); err == nil {
		t.Error("a region placed as if the page were not turned or cropped still took something")
	}
}

// TestTheUntaggedReaderListsWhatARegionThenTakes — every kind of piece, grouped: text into paragraphs, a
// picture, paths that touch as one drawing, a rule and a plain box last; decoration and content a form draws
// said. A region given every listed piece leaves the page with nothing untagged a region could take.
func TestTheUntaggedReaderListsWhatARegionThenTakes(t *testing.T) {
	twoLines := "BT /F1 12 Tf 72 700 Td (Bare words that run) Tj 0 -14 Td (onto a second line) Tj ET\n"
	rule := "72 600 m 400 600 l S\n"
	box := "420 560 60 30 re f\n"
	drawing := straightPath + "200 450 m 220 470 260 470 280 450 c S\n" + "150 410 20 20 re f\n" // the curve touches the path; the bar is inside
	content := twoLines + decoration + drawImage(72, 500, 100, 80) + drawing + curvedPath + rule + box
	src := regionFixture(content, "")
	got, err := ReadUntagged(src, 1)
	if err != nil {
		t.Fatal(err)
	}
	var rows []string
	for _, p := range got.Pieces {
		rows = append(rows, fmt.Sprintf("%s %q decoration=%v form=%v", p.Kind, p.Text, p.Decoration, p.InForm))
		if p.Page != 1 || !validRegion(p.Rect) {
			t.Errorf("%s is listed on page %d at %v, which is not a rectangle on the page", p.Kind, p.Page, p.Rect)
		}
	}
	want := []string{
		`text "Bare words that run onto a second line" decoration=false form=false`,
		`text "Decoration" decoration=true form=false`,
		`image "" decoration=false form=false`,
		`drawing "" decoration=false form=false`,
		`drawing "" decoration=false form=false`,
		`rule "" decoration=false form=false`,
		`box "" decoration=false form=false`,
	}
	if !reflect.DeepEqual(rows, want) || got.Pages != 2 {
		t.Fatalf("page 1 of %d lists\n  %s\nwant\n  %s", got.Pages, strings.Join(rows, "\n  "), strings.Join(want, "\n  "))
	}
	// The first drawing is the two paths that touch, with the bar inside them: one box holding all three.
	if d := got.Pieces[3].Rect; !reflect.DeepEqual(roundRect(d), roundRect(onPage([4]float64{99.5, 399.5, 280.5, 470.5}))) {
		t.Errorf("the drawing is listed at %v", d)
	}
	if im := got.Pieces[2].Rect; !reflect.DeepEqual(roundRect(im), roundRect(onPage(imageBox))) {
		t.Errorf("the picture is listed at %v, want the image as drawn", im)
	}

	// One piece, ticked: exactly it is taken.
	out, e := regioned(t, src, structEdit{value: "Note", pieces: [][4]float64{got.Pieces[0].Rect}})
	if e.text != "Bare words that run onto a second line" && e.text != "Bare words that runonto a second line" {
		t.Errorf("the ticked paragraph reads %q", e.text)
	}
	after, err := ReadUntagged(out, 1)
	if err != nil || len(after.Pieces) != len(got.Pieces)-1 || after.Pieces[0].Text != "Decoration" {
		t.Errorf("with one piece tagged the page lists %d pieces (%v), want the other %d", len(after.Pieces), err, len(got.Pieces)-1)
	}
	// Every piece: nothing is left.
	var all [][4]float64
	for _, p := range got.Pieces {
		all = append(all, p.Rect)
	}
	out, _ = regioned(t, src, structEdit{value: "Note", pieces: all})
	if after, err = ReadUntagged(out, 1); err != nil || len(after.Pieces) != 0 {
		t.Errorf("with every listed piece tagged the page still lists %v (%v)", after.Pieces, err)
	}
	if n, _ := UnmarkedTextRuns(out); n != 0 {
		t.Errorf("%d text run(s) are still neither tagged nor decoration", n)
	}
	if n, _ := uncoveredDrawings(out); n != 0 {
		t.Errorf("%d drawing(s) are still neither tagged nor decoration", n)
	}

	// Every page, for page 0; a page the document does not have is the page-range refusal.
	every, err := ReadUntagged(regionFixture(bareText, drawText(72, 700, "Second")), 0)
	if err != nil || len(every.Pieces) != 2 || every.Pieces[0].Page != 1 || every.Pieces[1].Page != 2 {
		t.Errorf("every page lists %v (%v)", every.Pieces, err)
	}
	var past *PageRangeError
	if _, err := ReadUntagged(src, 3); !errors.As(err, &past) {
		t.Errorf("page 3 of 2 was answered %v", err)
	}
}

func roundRect(r [4]float64) [4]float64 {
	for i := range r {
		r[i] = float64(int(r[i]*1e6+0.5)) / 1e6
	}
	return r
}

// TestTheUntaggedReaderSaysWhatAFormDrawsAndWhatIsDecoration — content a form draws is listed and said to be
// in a form, a fully tagged page lists nothing, and paths of different standing are not one drawing.
func TestTheUntaggedReaderSaysWhatAFormDrawsAndWhatIsDecoration(t *testing.T) {
	got, err := ReadUntagged(regionFixture("/Fm0 Do\n", ""), 1)
	if err != nil || len(got.Pieces) != 1 || !got.Pieces[0].InForm || got.Pieces[0].Text != "Inside" || got.Pieces[0].Decoration {
		t.Errorf("a form's text is listed as %+v (%v)", got.Pieces, err)
	}
	if got, err = ReadUntagged(regionFixture("", ""), 1); err != nil || len(got.Pieces) != 0 {
		t.Errorf("a fully tagged page lists %+v (%v)", got.Pieces, err)
	}
	// A bare curve and a decoration curve that overlap: two drawings, never one — a region must be able to
	// take the bare one without cutting through the other's sequence.
	overlapping := curvedPath + "/Artifact BMC 310 400 m 320 460 380 460 390 400 c f EMC\n"
	got, err = ReadUntagged(regionFixture(overlapping, ""), 1)
	if err != nil || len(got.Pieces) != 2 || got.Pieces[0].Decoration == got.Pieces[1].Decoration {
		t.Errorf("a bare drawing over a decoration one lists %+v (%v)", got.Pieces, err)
	}
	// A ruled grid is ONE drawing — its rules cross — and a ground painted under the whole page joins nothing
	// to anything: it holds the grid and the curve whole, so it is a box of its own and they stay two drawings.
	ground := "0 0 612 792 re f\n"
	grid := hRule(100, 400, 700) + hRule(100, 400, 670) + vRule(100, 670, 700) + vRule(250, 670, 700) + vRule(400, 670, 700)
	got, err = ReadUntagged(regionFixture(ground+grid+curvedPath+hRule(72, 300, 200), ""), 1)
	var kinds []string
	for _, p := range got.Pieces {
		kinds = append(kinds, p.Kind)
	}
	if err != nil || !reflect.DeepEqual(kinds, []string{"drawing", "drawing", "rule", "box"}) {
		t.Fatalf("a ground, a grid, a curve and a lone rule list as %v (%v), want two drawings, the rule and the ground", kinds, err)
	}
	if g := got.Pieces[0].Rect; !reflect.DeepEqual(roundRect(g), roundRect(onPage([4]float64{99.5, 669.5, 400.5, 700.5}))) {
		t.Errorf("the grid is listed at %v, want the five rules as one box", g)
	}
	// Ticking the grid takes its five rules and not the ground it stands on.
	out, _ := regioned(t, regionFixture(ground+grid+curvedPath, ""), structEdit{value: "Note", pieces: [][4]float64{got.Pieces[0].Rect}})
	if page := regionPage(t, out, 1); strings.Count(page, "/Note <</MCID") != 5 || strings.Contains(page, "BDC\n0 0 612 792 re f") {
		t.Errorf("the ticked grid took %d sequence(s), want its five rules and not the ground:\n%s", strings.Count(page, "/Note <</MCID"), page)
	}
	// A page with no content at all.
	if got, err = ReadUntagged(regionFixture("", ""), 2); err != nil || len(got.Pieces) != 0 {
		t.Errorf("an empty page lists %+v (%v)", got.Pieces, err)
	}
}

// TestOnlyTheRegionReaderKeepsDrawings — the painted-path record is the region reader's alone: the run
// reader, reflow's and the page map's keep none, so the proposer's walk carries nothing new (ADR-088,
// ADR-125).
func TestOnlyTheRegionReaderKeepsDrawings(t *testing.T) {
	ctx := parsed(t, regionFixture(decoration+straightPath+drawImage(72, 500, 100, 80), ""))
	pg := pageAt(ctx, nil, 1)
	for name, read := range map[string]func() (pageRuns, error){
		"readPageRuns":      func() (pageRuns, error) { return readPageRuns(ctx, pg) },
		"readPageGlyphRuns": func() (pageRuns, error) { return readPageGlyphRuns(ctx, pg) },
		"readPageShapes":    func() (pageRuns, error) { return readPageShapes(ctx, pg) },
	} {
		pr, err := read()
		if err != nil {
			t.Fatal(err)
		}
		if len(pr.paths) != 0 || len(pr.artifacts) != 0 {
			t.Errorf("%s kept %d path(s) and %d artifact sequence(s)", name, len(pr.paths), len(pr.artifacts))
		}
		for _, r := range pr.runs {
			if r.art != 0 {
				t.Errorf("%s numbered a run's artifact sequence", name)
			}
		}
	}
	pr, err := readPageDrawings(ctx, pg)
	if err != nil {
		t.Fatal(err)
	}
	if len(pr.paths) != 1 || len(pr.images) != 1 || len(pr.artifacts) != 1 || len(pr.shapes) != 0 || len(pr.marks) != 0 {
		t.Fatalf("the region reader kept %d path(s), %d image(s), %d artifact(s), %d shape(s), %d mark(s)", len(pr.paths), len(pr.images), len(pr.artifacts), len(pr.shapes), len(pr.marks))
	}
	if a := pr.artifacts[0]; a.pieces != 1 || a.unboxed != 0 || a.nested || a.inForm || a.close == (opSpan{}) {
		t.Errorf("the artifact sequence reads %+v", a)
	}
	if p := pr.paths[0]; p.box != [4]float64{99.5, 399.5, 200.5, 450.5} || p.curved || !p.stroked || p.ops != 3 || p.art != 0 || p.artifact || p.marked {
		t.Errorf("the path reads %+v", p)
	}
}

// TestADocumentTaggedByNibTakesBackAnIgnoredParagraph — the round trip the feature is for, on nib's own
// commit: a paragraph the reviewer ignored was written as decoration; the reader lists it, a region tags it,
// and it reads the words it drew.
func TestADocumentTaggedByNibTakesBackAnIgnoredParagraph(t *testing.T) {
	src, err := untaggedMarkdown([]byte(commitFixtureMarkdown))
	if err != nil {
		t.Fatal(err)
	}
	pr := mustPropose(t, src)
	reviews := reviewAll(pr)
	ignored := -1
	for i, e := range pr.Elements {
		if e.Role == "P" && ignored < 0 {
			ignored = i
		}
	}
	if ignored < 0 {
		t.Fatal("setup: the proposal holds no paragraph")
	}
	reviews[ignored].Ignore = true
	committed, err := CommitTags(src, reviews)
	if err != nil {
		t.Fatal(err)
	}
	squash := func(s string) string { return strings.Join(strings.Fields(s), "") }
	words := squash(pr.Elements[ignored].Text)
	listed, err := ReadUntagged(committed, pr.Elements[ignored].Page)
	if err != nil {
		t.Fatal(err)
	}
	var piece *UntaggedPiece
	for i, p := range listed.Pieces {
		if p.Kind == "text" && squash(p.Text) == words {
			piece = &listed.Pieces[i]
		}
	}
	if piece == nil || !piece.Decoration {
		t.Fatalf("the ignored paragraph %q is not listed as decoration: %+v", words, listed.Pieces)
	}
	out, e := regioned(t, committed, structEdit{value: "P", page: piece.Page, pieces: [][4]float64{piece.Rect}})
	if got := squash(e.text); got != words {
		t.Errorf("the paragraph tagged by region reads %q, and it drew %q", got, words)
	}
	if n, _ := UnmarkedTextRuns(out); n != 0 {
		t.Errorf("%d run(s) are neither tagged nor decoration after the region", n)
	}
}

// TestARegionBringsBackWhatAnArtifactEditTookOut — the round trip on a producer's document: LibreOffice's
// paragraph is declared decoration (the artifact edit), the reader lists it, a region placed where it was
// brings it back, and it reads the same text; the page's tokens are what they were but for the openers.
func TestARegionBringsBackWhatAnArtifactEditTookOut(t *testing.T) {
	if !LibreOfficeAvailable() {
		t.Skip("NOTE: LibreOffice is not installed, so there is no producer's document to round-trip; nib's own commit and the hand-built cases still run")
	}
	src := truthCorpus(t)[0].pdf // Document › H1 P H2 P L P
	v := viewOf(t, src)
	top := kidIDs(v, v.elements[0])
	para := byID(v)[top[1]]
	if para.standard != "P" || para.text == "" {
		t.Fatalf("setup: the Document's second kid reads %s %q", para.standard, para.text)
	}
	gone := mustApply(t, src, structEdit{kind: editArtifact, elem: para.id})
	listed, err := ReadUntagged(gone, para.page)
	if err != nil {
		t.Fatal(err)
	}
	var pieces [][4]float64
	var words []string
	for _, p := range listed.Pieces {
		if p.Kind == "text" && p.Decoration {
			pieces = append(pieces, p.Rect)
			words = append(words, p.Text)
		}
	}
	squash := func(s string) string { return strings.Join(strings.Fields(s), "") }
	if len(pieces) == 0 || !strings.Contains(squash(strings.Join(words, " ")), squash(para.text)) {
		t.Fatalf("the paragraph made decoration (%q) is not listed: %q", para.text, words)
	}
	before, _ := ReadUntagged(src, para.page)
	for _, p := range before.Pieces {
		// What was decoration before the artifact edit is not this paragraph's, and is left where it is.
		for i := 0; i < len(pieces); i++ {
			if pieces[i] == [4]float64(p.Rect) {
				pieces = append(pieces[:i], pieces[i+1:]...)
				i--
			}
		}
	}
	out, e := regioned(t, gone, structEdit{value: "P", page: para.page, pieces: pieces, parent: v.elements[0].id, index: 1})
	if squash(e.text) != squash(para.text) {
		t.Errorf("brought back, the paragraph reads %q and read %q", e.text, para.text)
	}
	bv := viewOf(t, out)
	if got := kidIDs(bv, bv.elements[0]); len(got) != len(top) || got[1] != e.id || got[0] != top[0] || got[2] != top[2] {
		t.Errorf("the Document's kids read %v, want the new paragraph second among %v", got, top)
	}
	for id, text := range textByElement(v) {
		if id != para.id && id != v.elements[0].id && textByElement(bv)[id] != text {
			t.Errorf("element %d read %q and now reads %q", id, text, textByElement(bv)[id])
		}
	}
	if !reflect.DeepEqual(markerFreeTokens(pageContentOf(t, src, para.page)), markerFreeTokens(pageContentOf(t, out, para.page))) {
		t.Error("the page's tokens changed beyond its marked-content openers")
	}
}

// vectorFigure is a hand-built drawing: a stroked frame, a filled curve and a diagonal, touching.
const vectorFigure = "100 400 200 100 re S\n100 400 m 150 520 250 520 300 400 c f\n100 400 m 300 500 l S\n"

// TestAVectorDrawingTaggedAsAFigureFailsNoFigureClause — veraPDF's verdict (ADR-125, `/pending 860`): on a
// page nib committed, a drawn graphic tagged by region as a Figure with a description fails neither 7.3 t1
// nor 7.1 t3, and nothing the page did not fail with the drawing left as decoration; and a paragraph the
// reviewer ignored fails nothing new once a region tags it.
func TestAVectorDrawingTaggedAsAFigureFailsNoFigureClause(t *testing.T) {
	vp := verapdfPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so ADR-125's ua1 differential is UNCHECKED in this run.")
	}
	src := figureFixture("BT /F1 24 Tf 72 700 Td (Above the drawing) Tj ET\n" + drawText(72, 640, "A paragraph under the heading") + vectorFigure)
	pr := mustPropose(t, src)
	reviews := reviewAll(pr)
	if len(reviews) != 2 {
		t.Fatalf("setup: %d element(s) proposed, want the heading and the paragraph", len(reviews))
	}
	all, err := CommitTags(src, reviews)
	if err != nil {
		t.Fatal(err)
	}
	reviews[1].Ignore = true
	committed, err := CommitTags(src, reviews)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := ReadUntagged(committed, 1)
	if err != nil {
		t.Fatal(err)
	}
	var drawing, para [][4]float64
	for _, p := range listed.Pieces {
		switch p.Kind {
		case "drawing":
			drawing = append(drawing, p.Rect)
		case "text":
			para = append(para, p.Rect)
		}
	}
	if len(drawing) != 1 || len(para) != 1 {
		t.Fatalf("the committed page lists %+v, want one drawing and the ignored paragraph", listed.Pieces)
	}
	if _, err := EditStructure(committed, []StructureEdit{{Kind: "region", Value: "Figure", Page: 1, Pieces: drawing, Index: -1}}); !errors.Is(err, ErrTagsReview) {
		t.Errorf("a drawing tagged as a figure with no description was answered %v", err)
	}
	figure, err := EditStructure(committed, []StructureEdit{{Kind: "region", Value: "Figure", Page: 1, Pieces: drawing, Alt: "A chart rising to the right", Index: -1}})
	if err != nil {
		t.Fatal(err)
	}
	both, err := EditStructure(figure, []StructureEdit{{Kind: "region", Value: "P", Page: 1, Pieces: para, Index: 1}})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var files []string
	for n, b := range map[string][]byte{"all.pdf": all, "committed.pdf": committed, "figure.pdf": figure, "both.pdf": both} {
		f := filepath.Join(dir, n)
		if werr := os.WriteFile(f, b, 0o600); werr != nil {
			t.Fatal(werr)
		}
		files = append(files, f)
	}
	cl := ua1FailedClauses(t, vp, files)
	before := cl["committed.pdf"]
	if before == nil || cl["figure.pdf"] == nil || cl["both.pdf"] == nil || cl["all.pdf"] == nil {
		t.Fatal("veraPDF could not validate one of the documents, so there is no differential")
	}
	t.Logf("committed fails %v; with the figure %v; with the paragraph too %v; every paragraph kept at the commit %v",
		sortedClauses(before), sortedClauses(cl["figure.pdf"]), sortedClauses(cl["both.pdf"]), sortedClauses(cl["all.pdf"]))
	for _, name := range []string{"figure.pdf", "both.pdf"} {
		for _, c := range []string{"7.3 t1", "7.1 t3"} {
			if cl[name][c] {
				t.Errorf("%s fails %s", name, c)
			}
		}
		for c := range cl[name] {
			if !before[c] {
				t.Errorf("%s fails %s, which the committed page does not", name, c)
			}
		}
	}
	for c := range cl["both.pdf"] {
		if !cl["all.pdf"][c] {
			t.Errorf("with the ignored paragraph tagged by region the page fails %s, which it does not when the commit keeps the paragraph", c)
		}
	}
}
