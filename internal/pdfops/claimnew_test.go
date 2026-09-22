package pdfops

import (
	"bytes"
	"errors"
	"fmt"
	"nib/mdpdf"
	"os"
	"path/filepath"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/contentstream"
	"nib/internal/testpdf"
)

// `/pending 495` — no door CREATES a claim of tagging over text nothing describes, no door raises the
// tier of a tree it adds to, and an annotation's page is not content an element describes.

// formHost is a document nib tagged from its own Markdown AST: a tree, recorded `Exact`, whose body text
// is all described. The form tests describe widgets INTO it, because authoring a form never builds the
// body's structure.
func formHost(t *testing.T) []byte {
	t.Helper()
	pdf, err := ConvertDocToPDF([]byte("# A form\n\nPlease fill in the fields below.\n"), ".md")
	if err != nil {
		t.Fatal(err)
	}
	if s := inspectTags(pdf); !s.claimsHonestly() || s.source != sourceExact {
		t.Fatalf("setup: the form host is not a tagged, Exact-recorded document (%+v)", s)
	}
	return pdf
}

// untaggedBody is testpdf.Text with the stimulus asserted: body text no element describes, and no claim.
func untaggedBody(t *testing.T) []byte {
	t.Helper()
	pdf, err := testpdf.Text("A whole paragraph of body text that a screen reader must reach.")
	if err != nil {
		t.Fatal(err)
	}
	if n, rerr := UnmarkedTextRuns(pdf); rerr != nil || n == 0 {
		t.Fatalf("setup: the host has %d unmarked text run(s) (err %v), so a claim over it would not be over undescribed text", n, rerr)
	}
	if inspectTags(pdf).claims() {
		t.Fatal("setup: the untagged host already claims tagging")
	}
	return pdf
}

// TestAFormOnAnUntaggedTextPDFMakesNoClaim — the finding as reproduced: `tagged=true`, `/Marked true`
// over a body only `/Form` elements sat beside, and veraPDF failing 7.1 t3.
func TestAFormOnAnUntaggedTextPDFMakesNoClaim(t *testing.T) {
	host := untaggedBody(t)
	out, tagged, err := AuthorTaggedForm(host, s07Fields())
	if err != nil {
		t.Fatal(err)
	}
	if tagged {
		t.Error("AuthorTaggedForm reported the form tagged on a document whose body text nothing describes")
	}
	if s := inspectTags(out); s.claims() {
		t.Errorf("the output claims tagging (%+v) over body text no element describes — ADR-031 law 1", s)
	}
	js, jerr := ExportFormJSON(out)
	if jerr != nil || !bytes.Contains(js, []byte("full_name")) {
		t.Errorf("the refusal cost the user the form (err %v): %s", jerr, js)
	}
}

// TestAnOCRLayerOverUntaggedTextMakesNoClaim — the same claim through the OCR door, which the finding did
// not name: a text layer stamped over a document that already has text tagged the words and claimed the
// whole document.
func TestAnOCRLayerOverUntaggedTextMakesNoClaim(t *testing.T) {
	// Control: over a scan, where the words are all the text there is, the door still tags.
	if _, tagged, err := TagOCRLayer(scannedPage(t), ocrWords(), "eng"); err != nil || !tagged {
		t.Fatalf("control: TagOCRLayer did not tag a scan (tagged=%v, err %v), so the refusal below is not about the host", tagged, err)
	}
	out, tagged, err := TagOCRLayer(untaggedBody(t), ocrWords(), "eng")
	if err != nil {
		t.Fatal(err)
	}
	if tagged {
		t.Error("TagOCRLayer reported the layer tagged over a document whose own text nothing describes")
	}
	if s := inspectTags(out); s.claims() {
		t.Errorf("the output claims tagging (%+v) over text no element describes", s)
	}
}

// TestADoorAddingToAnHonestClaimIsNotRefusedForTextItDidNotLeaveUntagged — the exemption. A tagged
// document with an appended untagged page already claims; describing a widget into it adds no claim, and
// refusing would cost the widget its description to protect nothing.
func TestADoorAddingToAnHonestClaimIsNotRefusedForTextItDidNotLeaveUntagged(t *testing.T) {
	host, err := Append(taggedFixture(), untaggedFixture())
	if err != nil {
		t.Fatal(err)
	}
	s := inspectTags(host)
	if !s.claimsHonestly() {
		t.Fatalf("setup: the host does not already claim honestly (%+v)", s)
	}
	if n, _ := UnmarkedTextRuns(host); n == 0 {
		t.Fatal("setup: the host has no unmarked text, so the exemption is not exercised")
	}
	_, tagged, err := AuthorTaggedForm(host, []FormField{{Page: 2, Rect: [4]float64{100, 100, 300, 120}, Kind: "text", Name: "f", Label: "Name"}})
	if err != nil {
		t.Fatal(err)
	}
	if !tagged {
		t.Error("a form described into an already-tagged document was refused because of a page the document already claimed")
	}
}

// TestAWidgetsPageIsNotContentAnElementDescribes — `describedPages` counted an OBJR's `/Pg`, so a page
// whose only structure is a widget's `/Form` element read as described.
func TestAWidgetsPageIsNotContentAnElementDescribes(t *testing.T) {
	host, err := Append(taggedFixture(), untaggedFixture())
	if err != nil {
		t.Fatal(err)
	}
	if s := inspectTags(host); s.undescribed != 1 {
		t.Fatalf("setup: the appended page is not the one undescribed page (%+v)", s)
	}
	out, tagged, err := AuthorTaggedForm(host, []FormField{{Page: 2, Rect: [4]float64{100, 100, 300, 120}, Kind: "text", Name: "f", Label: "Name"}})
	if err != nil || !tagged {
		t.Fatalf("setup: the widget was not described onto page 2 (tagged=%v, err %v)", tagged, err)
	}
	if s := inspectTags(out); s.undescribed != 1 {
		t.Errorf("after a widget was described on the untagged page, %d page(s) read undescribed, want 1: an annotation's page is not its content (%+v)", s.undescribed, s)
	}
}

// TestADoorAddingToATreeNeverRaisesItsTier — D4's tier is a fact about the whole tree, so a tree grown by a
// second door is only as trustworthy as its weakest part, and a tree nib did not record stays unrecorded.
func TestADoorAddingToATreeNeverRaisesItsTier(t *testing.T) {
	field := []FormField{{Page: 1, Rect: [4]float64{100, 100, 200, 120}, Kind: "text", Name: "f1", Label: "Name"}}
	approx, err := writeMutated(formHost(t), func(ctx *model.Context) error { return setTagSource(ctx, sourceApproximate) })
	if err != nil {
		t.Fatal(err)
	}
	if s := inspectTags(approx); s.source != sourceApproximate {
		t.Fatalf("setup: the host records %q, want Approximate", s.source)
	}
	out, tagged, err := AuthorTaggedForm(approx, field)
	if err != nil || !tagged {
		t.Fatalf("setup: the form was not described (tagged=%v, err %v)", tagged, err)
	}
	if got, _ := StructureSource(out); got != sourceApproximate {
		t.Errorf("a form described into an OCR-tier tree records %q, want Approximate — the tier rose", got)
	}

	// The other direction: an OCR layer added to an Exact tree lowers it.
	ocr, tagged, err := TagOCRLayer(formHost(t), ocrWords(), "eng")
	if err != nil || !tagged {
		t.Fatalf("setup: the OCR layer was not tagged into the Exact host (tagged=%v, err %v)", tagged, err)
	}
	if got, _ := StructureSource(ocr); got != sourceApproximate {
		t.Errorf("an OCR layer added to an Exact tree records %q, want Approximate", got)
	}

	// A tree nib has no record of stays unrecorded.
	if s := inspectTags(taggedFixture()); !s.claimsHonestly() || s.source != "" {
		t.Fatalf("setup: the fixture is not a tagged, unrecorded document (%+v)", s)
	}
	un, tagged, err := AuthorTaggedForm(taggedFixture(), field)
	if err != nil || !tagged {
		t.Fatalf("setup: the form was not described into the unrecorded tree (tagged=%v, err %v)", tagged, err)
	}
	if got, ok := StructureSource(un); ok {
		t.Errorf("a form described into a tree nib did not record now records %q — an unrecorded tree is not Exact", got)
	}
}

// withUnmarkedPath prepends a stroked line, in no marked content, to page 1.
func withUnmarkedPath(t *testing.T, pdf []byte) []byte {
	t.Helper()
	out, err := writeMutated(pdf, func(ctx *model.Context) error {
		d, _, _, err := ctx.PageDict(1, false)
		if err != nil {
			return err
		}
		c, err := ctx.PageContent(d, 1)
		if err != nil {
			return err
		}
		return setPageContent(ctx, d, append([]byte("q 1 w 50 50 m 300 50 l S Q\n"), c...))
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestACommitDeclaresAPathNoElementCoversAnArtifact — the commit writer's sibling: a painted path stayed
// neither tagged nor an artifact under the claim it makes, 7.1 t3 on a committed page with one rule.
func TestACommitDeclaresAPathNoElementCoversAnArtifact(t *testing.T) {
	src, err := untaggedMarkdown([]byte(commitFixtureMarkdown))
	if err != nil {
		t.Fatal(err)
	}
	pathed := withUnmarkedPath(t, src)
	out, err := commitProposal(pathed, proposeFor(t, pathed).elements)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pageStream(t, out, 1), []byte("/Artifact BMC\n50 50 m 300 50 l S\nEMC")) {
		t.Errorf("the committed page does not bracket the stroked path as an artifact:\n%.400s", pageStream(t, out, 1))
	}

	vp := verapdfPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so the path's 7.1 t3 is UNCHECKED in this run.")
	}
	plain, err := commitProposal(src, proposeFor(t, src).elements)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var files []string
	for n, b := range map[string][]byte{"plain.pdf": plain, "pathed.pdf": out} {
		p := filepath.Join(dir, n)
		if werr := os.WriteFile(p, b, 0o600); werr != nil {
			t.Fatal(werr)
		}
		files = append(files, p)
	}
	cl := ua1FailedClauses(t, vp, files)
	if cl["plain.pdf"] == nil || cl["plain.pdf"]["7.1 t3"] {
		t.Fatalf("control: the committed document without a path fails 7.1 t3 or did not validate (%v), so the path is not what is measured", sortedClauses(cl["plain.pdf"]))
	}
	if cl["pathed.pdf"] == nil || cl["pathed.pdf"]["7.1 t3"] {
		t.Errorf("a committed page with one stroked path fails 7.1 t3 (%v)", sortedClauses(cl["pathed.pdf"]))
	}
}

// TestUncoveredPathSpansSkipsWhatIsAlreadyMarkedOrUnpainted — each exclusion of the path reader, separately.
func TestUncoveredPathSpansSkipsWhatIsAlreadyMarkedOrUnpainted(t *testing.T) {
	for _, c := range []struct {
		name, src string
		want      int
	}{
		{"a bare stroke", "0 0 m 10 10 l S", 1},
		{"a filled rectangle", "0 0 10 10 re f", 1},
		{"inside an artifact", "/Artifact BMC 0 0 m 10 10 l S EMC", 0},
		{"under an MCID", "/P <</MCID 0>> BDC 0 0 m 10 10 l S EMC", 0},
		{"under a plain tag", "/Span BMC 0 0 m 10 10 l S EMC", 1},
		{"a clip that paints nothing", "0 0 10 10 re W n", 0},
		{"after the artifact closed", "/Artifact BMC EMC 0 0 m 10 10 l S", 1},
		// Malformed, and never split: a bracket around a path that crosses a sequence boundary would nest wrongly.
		{"begun inside an artifact, painted outside", "/Artifact BMC 0 0 m EMC 10 10 l S", 0},
		{"begun outside, painted inside an artifact", "0 0 m /Artifact BMC 10 10 l S EMC", 0},
	} {
		sp, _ := uncoveredDrawingSpans([]byte(c.src), nil)
		if got := len(sp); got != c.want {
			t.Errorf("%s: %d span(s), want %d", c.name, got, c.want)
		}
	}
	for src, want := range map[string]string{
		"q 1 w 50 50 m 300 50 l S Q": "50 50 m 300 50 l S",
		// `n` ends the clip's path object, so the next path's bracket does not swallow the clip.
		"0 0 10 10 re W n 5 5 m 6 6 l S": "5 5 m 6 6 l S",
	} {
		if sp, _ := uncoveredDrawingSpans([]byte(src), nil); len(sp) != 1 || src[sp[0].start:sp[0].end] != want {
			t.Errorf("%q: the span does not enclose exactly the path object %q: %v", src, want, sp)
		}
	}
}

// TestACommitRefusesTextInsideAFormEvenWhenNotProposed — text drawn inside a form XObject cannot be
// bracketed; unproposed, it was left unmarked under the claim. Now it is the commit's own refusal.
func TestACommitRefusesTextInsideAFormEvenWhenNotProposed(t *testing.T) {
	untagged, err := untaggedMarkdown([]byte(commitFixtureMarkdown))
	if err != nil {
		t.Fatal(err)
	}
	stamped, err := StampWatermark(untagged, "DRAFT", WatermarkStyle{})
	if err != nil {
		t.Fatal(err)
	}
	bare, err := writeMutated(stamped, func(ctx *model.Context) error {
		for pg := 1; pg <= ctx.PageCount; pg++ {
			d, _, _, derr := ctx.PageDict(pg, false)
			if derr != nil {
				return derr
			}
			src, cerr := ctx.PageContent(d, pg)
			if cerr != nil {
				return cerr
			}
			edit := contentstream.NewEdit(src)
			for _, m := range watermarkArtifactSpans(src) {
				edit.Replace(m.start, m.end, []byte("/Span BMC"))
			}
			out, aerr := edit.Apply()
			if aerr != nil {
				return aerr
			}
			if serr := setPageContent(ctx, d, out); serr != nil {
				return serr
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var kept []proposedElement
	droppedInForm := false
	for _, el := range proposeFor(t, bare).elements {
		inForm := false
		for _, r := range elementRuns(el) {
			inForm = inForm || r.inForm
		}
		if inForm {
			droppedInForm = true
			continue
		}
		kept = append(kept, el)
	}
	if !droppedInForm || len(kept) == 0 {
		t.Fatalf("setup: no in-form element to leave out (%v) or nothing left to commit (%d)", droppedInForm, len(kept))
	}
	if _, cerr := commitProposal(bare, kept); !errors.Is(cerr, errCommitInForm) {
		t.Errorf("unproposed text inside a form XObject: err = %v, want errCommitInForm", cerr)
	}
}

// TestTagAuthoredPagesRefusesATreeThatDescribesNothing — the door's product IS the tagged page, so a
// tagging that anchors nothing (a page that draws no text, given no roles) is an error, never an
// untagged page handed back as if it were tagged.
func TestTagAuthoredPagesRefusesATreeThatDescribesNothing(t *testing.T) {
	drawing := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>",
		4: streamObj("0 0 m 100 100 l S\n"),
	})
	if _, err := TagAuthoredPages(drawing, [][]mdpdf.Role{{}}); err == nil {
		t.Error("tagging a page with no text returned a document; it describes nothing and must be refused")
	}
}

// TestTagAuthoredPagesRefusesADocumentThatAlreadyHasATree — the door brackets every text run it is given
// without asking whether a run is already marked, so on a tagged document it would describe the same
// words twice. Its census row declares `untouched` on the strength of this refusal.
func TestTagAuthoredPagesRefusesADocumentThatAlreadyHasATree(t *testing.T) {
	_, err := TagAuthoredPages(taggedFixture(), [][]mdpdf.Role{{{Kind: mdpdf.RoleBody}}})
	if !errors.Is(err, errAuthoredAlreadyTagged) {
		t.Errorf("tagging an already-tagged document returned err %v, want errAuthoredAlreadyTagged", err)
	}
}

// TestTagAuthoredPagesRefusesARoleCountThatIsNotThePageCount — one role list per page, exactly. The
// door used to tag the pages the two counts shared and ignore the rest, so a second page nobody gave
// roles for shipped undescribed under a tree claiming the document, and a surplus list — a line the
// caller composed that the document does not hold — vanished without a word.
func TestTagAuthoredPagesRefusesARoleCountThatIsNotThePageCount(t *testing.T) {
	page := func(n int) string {
		return fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 9 0 R >> >> /Contents %d 0 R >>", n)
	}
	text := streamObj("q BT /F1 12 Tf 72 700 Td (Authored line) Tj ET Q\n")
	font := "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"
	one := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>", 2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: page(4), 4: text, 9: font,
	})
	two := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>", 2: "<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>",
		3: page(4), 4: text, 5: page(6), 6: text, 9: font,
	})
	body := []mdpdf.Role{{Kind: mdpdf.RoleBody}}
	if _, err := TagAuthoredPages(one, [][]mdpdf.Role{body}); err != nil {
		t.Fatalf("setup: one page with one role list was refused (%v), so a refusal below proves nothing", err)
	}
	if _, err := TagAuthoredPages(one, [][]mdpdf.Role{body, body}); err == nil {
		t.Error("two role lists for a one-page document were accepted; the surplus list tagged nothing")
	}
	if _, err := TagAuthoredPages(two, [][]mdpdf.Role{body}); err == nil {
		t.Error("one role list for a two-page document was accepted; page 2 ships undescribed under the tree")
	}
}
