package uacheck

import (
	"strings"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// The structure rules — `PLAN-accessibility.md` P07.S03.

// TestTheStructureRulesAgreeWithWhatP06Measured — the slice's third acceptance clause.
//
// Every document here is one P06 produced through a PRODUCT door, and every expected verdict is what
// veraPDF reported about that document shape during P05, P06 and this slice's step zero.
func TestTheStructureRulesAgreeWithWhatP06Measured(t *testing.T) {
	plain, err := testpdf.Text("host")
	if err != nil {
		t.Fatal(err)
	}
	wrapped := committedProposal(t, plain)
	fields := []pdfops.FormField{{Page: 1, Rect: [4]float64{100, 700, 300, 720}, Kind: "text", Name: "n", Label: "Your name"}}
	undescribed, err := pdfops.AuthorForm(plain, fields)
	if err != nil {
		t.Fatal(err)
	}
	// Described into the committed proposal: on the untagged page the door claims nothing (`/pending 495`).
	described, tagged, err := pdfops.AuthorTaggedForm(wrapped, fields)
	if err != nil || !tagged {
		t.Fatalf("setup: AuthorTaggedForm tagged=%v: %v", tagged, err)
	}
	markdown, err := pdfops.ConvertDocToPDF([]byte("# Heading\n\nA paragraph.\n\n- one\n- two\n"), ".md")
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		doc    string
		pdf    []byte
		clause string
		want   Verdict
	}{
		{"an untagged page", plain, "6.2 t1", Fail},
		{"an untagged page", plain, "7.1 t3", Fail},
		{"an untagged page", plain, "7.18.4 t1", NotApplicable},

		{"a committed proposal", wrapped, "6.2 t1", Pass},
		{"a committed proposal", wrapped, "7.1 t3", Pass},

		// P06.S07's measurement: the undescribed form fails 7.18.4 t1 and the described one passes.
		// The described form's host is the committed proposal, so its text is tagged and 7.1 t3 passes;
		// on the untagged page it used to fail 7.1 t3 under a claim of tagging (`/pending 495`).
		{"an undescribed form", undescribed, "7.18.4 t1", Fail},
		{"a described form", described, "7.18.4 t1", Pass},
		{"a described form", described, "6.2 t1", Pass},
		{"a described form", described, "7.1 t3", Pass},

		// P06.S02, through the door /pending 481 wired.
		{"converted Markdown", markdown, "6.2 t1", Pass},
		{"converted Markdown", markdown, "7.1 t3", Pass},
		{"converted Markdown", markdown, "7.1 t11", Pass},
	} {
		got := verdictOf(t, c.pdf, c.clause)
		if got.Verdict != c.want {
			t.Errorf("%s: %s reports %v (%s at %s), want %v", c.doc, c.clause, got.Verdict, got.Why, got.Where, c.want)
		}
	}
}

// TestContentFailureNamesTheOperatorItFound — the slice's first acceptance clause.
func TestContentFailureNamesTheOperatorItFound(t *testing.T) {
	plain, err := testpdf.Text("host")
	if err != nil {
		t.Fatal(err)
	}
	got := verdictOf(t, plain, "7.1 t3")
	if got.Verdict != Fail {
		t.Fatalf("control: an untagged page reports %v for 7.1 t3", got.Verdict)
	}
	for _, want := range []string{"page 1", "operator #", "`"} {
		if !strings.Contains(got.Where, want) {
			t.Errorf("the failure's location %q does not contain %q. P01 spent a slice discovering "+
				"veraPDF points at `contentStream[0]/content[2]`, and a checker that says only "+
				"`7.1 t3 fails` reproduces the problem it exists to solve", got.Where, want)
		}
	}
}

// TestAnArtifactIsCoveredAndAnMCIDIsNotUntilItReachesTheRoot — the two states 7.1 t3 accepts.
//
// P06.S06 found nib's OCR layer wrapped in `/Artifact` — covered by this clause, and precisely why it
// was a disclaimer rather than a description. And an optional-content sequence (`/OC … BDC`) is NOT
// coverage: it controls visibility and says nothing about whether content is real or decoration.
//
// **An MCID alone is NOT the second state, and this test used to say it was** (P04.S04). veraPDF's
// `isTaggedContent` resolves the sequence's `/MCID` through the parent tree and climbs `/P` asking whether
// the chain reaches the structure tree root; these documents have no structure tree at all, so it does not.
// Measured on 1.30.2: `/P <</MCID 0>> BDC` around text on a page with no `/StructTreeRoot` **FAILS** 7.1 t3,
// where nib reported Pass — a false pass in the sense `Verdict.conformant` means. The artifact row beside it
// is what keeps this from being a rule that fails whatever it is shown: an `/Artifact` needs no tree.
func TestAnArtifactIsCoveredAndAnMCIDIsNotUntilItReachesTheRoot(t *testing.T) {
	for _, c := range []struct {
		name    string
		content string
		want    Verdict
	}{
		{"bare text", "BT /F1 12 Tf 72 700 Td (x) Tj ET", Fail},
		{"text in an /Artifact", "/Artifact BMC BT /F1 12 Tf 72 700 Td (x) Tj ET EMC", Pass},
		{"text in an MCID sequence with no structure tree to resolve it", "/P <</MCID 0>> BDC BT /F1 12 Tf 72 700 Td (x) Tj ET EMC", Fail},
		{"text in an /OC sequence only", "/OC /oc1 BDC BT /F1 12 Tf 72 700 Td (x) Tj ET EMC", Fail},
		{"a path painted outside any sequence", "0 0 m 100 100 l S", Fail},
		{"path construction with no painting", "0 0 m 100 100 l n", NotApplicable},
		{"text spelling EMC inside a string does not close the sequence",
			"/Artifact BMC BT /F1 12 Tf 72 700 Td (EMC) Tj (y) Tj ET EMC", Pass},
		{"a close after the sequence leaves later text uncovered",
			"/Artifact BMC BT /F1 12 Tf (a) Tj ET EMC BT /F1 12 Tf (b) Tj ET", Fail},
	} {
		got := verdictOf(t, pageWithContent(c.content), "7.1 t3")
		if got.Verdict != c.want {
			t.Errorf("%s: 7.1 t3 reports %v (%s at %s), want %v", c.name, got.Verdict, got.Why, got.Where, c.want)
		}
	}
}

// TestTheWidgetIsReadFromTheAnnotationUpAsVeraPDFReadsIt — the slice's second acceptance clause,
// as amended by law 5.
//
// This test used to be `…IsCheckedInBothDirections` and required the element's OBJR to name the
// widget back. The S05 guard found nib failing a document veraPDF passes, and the measurement was
// one link at a time: OBJR removed → veraPDF PASSES; `/StructParent` removed → FAILS; element retyped
// `Div` → FAILS. So the OBJR case is now a PASS row, asserted, rather than silently dropped.
func TestTheWidgetIsReadFromTheAnnotationUpAsVeraPDFReadsIt(t *testing.T) {
	plain, err := testpdf.Text("host")
	if err != nil {
		t.Fatal(err)
	}
	described, tagged, err := pdfops.AuthorTaggedForm(committedProposal(t, plain), []pdfops.FormField{
		{Page: 1, Rect: [4]float64{100, 700, 300, 720}, Kind: "text", Name: "n", Label: "Your name"},
	})
	if err != nil || !tagged {
		t.Fatalf("setup: tagged=%v: %v", tagged, err)
	}
	if got := verdictOf(t, described, "7.18.4 t1"); got.Verdict != Pass {
		t.Fatalf("control: the described form reports %v (%s)", got.Verdict, got.Why)
	}
	for _, c := range []struct {
		name string
		pdf  []byte
		want Verdict
		why  string
	}{
		{"the annotation's /StructParent removed", widgetMutation(t, described, "drop-structparent"), Fail, "no /StructParent"},
		{"the element retyped Div", widgetMutation(t, described, "retype-div"), Fail, `"Div"`},
		{"the OBJR removed from the element — veraPDF passes it", widgetMutation(t, described, "drop-objr"), Pass, ""},
		{"a custom type role-mapped to Form", widgetMutation(t, described, "rolemap-form"), Pass, ""},
	} {
		got := verdictOf(t, c.pdf, "7.18.4 t1")
		if got.Verdict != c.want {
			t.Errorf("%s: 7.18.4 t1 reports %v (%s), want %v", c.name, got.Verdict, got.Why, c.want)
			continue
		}
		if c.why != "" && !strings.Contains(got.Why, c.why) {
			t.Errorf("%s: the reason %q does not name %s", c.name, got.Why, c.why)
		}
	}
}

// pageWithContent is a one-page document drawing exactly content, with a Helvetica resource and one
// optional-content property list so `/OC /oc1 BDC` resolves.
func pageWithContent(content string) []byte {
	return buildPDF(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> /Properties << /oc1 6 0 R >> >> /Contents 4 0 R >>",
		4: "<< /Length " + itoa(len(content)) + " >>\nstream\n" + content + "\nendstream",
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6: "<< /Type /OCG /Name (layer) >>",
	})
}

func itoa(n int) string {
	return strings.TrimSpace(strings.Repeat(" ", 0) + fmtInt(n))
}

func fmtInt(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// TestMarkInfoReadsTheVALUEAndNotJustTheDictionary.
//
// **Found by probing**: no fixture carried a `/MarkInfo` dictionary saying `/Marked false`, so a rule
// accepting it left every test green. A document declaring `Marked false` has told readers NOT to
// use its structure, which is the outcome this clause exists to prevent — and the dictionary being
// present is not the fact the clause is about.
func TestMarkInfoReadsTheVALUEAndNotJustTheDictionary(t *testing.T) {
	plain, err := testpdf.Text("host")
	if err != nil {
		t.Fatal(err)
	}
	wrapped := committedProposal(t, plain)
	if got := verdictOf(t, wrapped, "6.2 t1"); got.Verdict != Pass {
		t.Fatalf("control: a committed proposal reports %v for 6.2 t1", got.Verdict)
	}
	got := verdictOf(t, markedFalse(t, wrapped), "6.2 t1")
	if got.Verdict != Fail {
		t.Errorf("/MarkInfo << /Marked false >> reports %v (%s), want Fail", got.Verdict, got.Why)
	} else if !strings.Contains(got.Why, "/Marked true") {
		t.Errorf("the reason does not say what the clause wants: %q", got.Why)
	}
}
