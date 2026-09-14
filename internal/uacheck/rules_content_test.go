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
	wrapped, n, err := pdfops.TagAuthored(plain)
	if err != nil || n == 0 {
		t.Fatalf("setup: TagAuthored wrapped %d page(s): %v", n, err)
	}
	fields := []pdfops.FormField{{Page: 1, Rect: [4]float64{100, 700, 300, 720}, Kind: "text", Name: "n", Label: "Your name"}}
	undescribed, err := pdfops.AuthorForm(plain, fields)
	if err != nil {
		t.Fatal(err)
	}
	described, tagged, err := pdfops.AuthorTaggedForm(plain, fields)
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

		{"a wrapped page", wrapped, "6.2 t1", Pass},
		{"a wrapped page", wrapped, "7.1 t3", Pass},

		// P06.S07's measurement: the undescribed form fails 7.18.4 t1 and the described one passes.
		// Both fail 7.1 t3 — the HOST page's text is untagged, which is what its content owes.
		{"an undescribed form", undescribed, "7.18.4 t1", Fail},
		{"a described form", described, "7.18.4 t1", Pass},
		{"a described form", described, "6.2 t1", Pass},
		{"a described form", described, "7.1 t3", Fail},

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

// TestAnArtifactIsCoveredAndAnMCIDIsCoveredAndNothingElseIs — the two states 7.1 t3 accepts.
//
// P06.S06 found nib's OCR layer wrapped in `/Artifact` — covered by this clause, and precisely why it
// was a disclaimer rather than a description. And an optional-content sequence (`/OC … BDC`) is NOT
// coverage: it controls visibility and says nothing about whether content is real or decoration.
func TestAnArtifactIsCoveredAndAnMCIDIsCoveredAndNothingElseIs(t *testing.T) {
	for _, c := range []struct {
		name    string
		content string
		want    Verdict
	}{
		{"bare text", "BT /F1 12 Tf 72 700 Td (x) Tj ET", Fail},
		{"text in an /Artifact", "/Artifact BMC BT /F1 12 Tf 72 700 Td (x) Tj ET EMC", Pass},
		{"text in an MCID sequence", "/P <</MCID 0>> BDC BT /F1 12 Tf 72 700 Td (x) Tj ET EMC", Pass},
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

// TestTheWidgetLinkageIsCheckedInBothDirections — the slice's second acceptance clause.
//
// Each case breaks exactly one of the three things that must agree, starting from a document P06.S07
// produced and this rule passes.
func TestTheWidgetLinkageIsCheckedInBothDirections(t *testing.T) {
	plain, err := testpdf.Text("host")
	if err != nil {
		t.Fatal(err)
	}
	described, tagged, err := pdfops.AuthorTaggedForm(plain, []pdfops.FormField{
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
		want string
	}{
		{"the annotation's /StructParent removed", widgetMutation(t, described, "drop-structparent"), "no /StructParent"},
		{"the element retyped Div", widgetMutation(t, described, "retype-div"), `"Div"`},
		{"the OBJR removed from the element", widgetMutation(t, described, "drop-objr"), "no OBJR"},
	} {
		got := verdictOf(t, c.pdf, "7.18.4 t1")
		if got.Verdict != Fail {
			t.Errorf("%s: 7.18.4 t1 reports %v, want Fail", c.name, got.Verdict)
			continue
		}
		if !strings.Contains(got.Why, c.want) {
			t.Errorf("%s: the reason %q does not name %s", c.name, got.Why, c.want)
		}
	}
	// And the role map is resolved: a custom type mapped to Form passes.
	if got := verdictOf(t, widgetMutation(t, described, "rolemap-form"), "7.18.4 t1"); got.Verdict != Pass {
		t.Errorf("a widget in a /MyField element role-mapped to /Form reports %v (%s), want Pass — "+
			"veraPDF resolves the role map, so another producer's correct form would fail here",
			got.Verdict, got.Why)
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
	wrapped, n, err := pdfops.TagAuthored(plain)
	if err != nil || n == 0 {
		t.Fatalf("setup: wrapped %d: %v", n, err)
	}
	if got := verdictOf(t, wrapped, "6.2 t1"); got.Verdict != Pass {
		t.Fatalf("control: a wrapped page reports %v for 6.2 t1", got.Verdict)
	}
	got := verdictOf(t, markedFalse(t, wrapped), "6.2 t1")
	if got.Verdict != Fail {
		t.Errorf("/MarkInfo << /Marked false >> reports %v (%s), want Fail", got.Verdict, got.Why)
	} else if !strings.Contains(got.Why, "/Marked true") {
		t.Errorf("the reason does not say what the clause wants: %q", got.Why)
	}
}
