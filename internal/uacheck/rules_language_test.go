package uacheck

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// P04.S01 — every row below was run through veraPDF 1.30.2 before the rule it grades was written (the slice's
// grill note in the plan has the table), and each verdict here is veraPDF's.

// langClauseDoc is a one-page tagged document with one P element: cat is extra catalog entries, elem extra entries
// on the element, content the page content, annot an optional annotation (object 30) whose /StructParent is 1, and
// outline whether /Outlines holds an item. Object 40 is a /Properties entry /PL with /Lang (en_US), defined on
// every page and used only where the content names it; object 60 is an element with /Lang (en_US) that is not in
// the tree, reachable only when the parent tree's key 1 is pointed at it.
func langClauseDoc(cat, elem, content, annot string, outline bool) []byte {
	return langClauseDocWith(nil, cat, elem, content, annot, outline)
}

// langClauseDocWith is langClauseDoc with extra objects added (or replacing the defaults).
func langClauseDocWith(extra map[int]string, cat, elem, content, annot string, outline bool) []byte {
	objs := map[int]string{
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		4:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7:  "<< /Type /StructTreeRoot /K 8 0 R /ParentTree 9 0 R >>",
		8:  "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 0 " + elem + " >>",
		9:  "<< /Nums [0 [8 0 R] 1 8 0 R] >>",
		40: "<< /Lang (en_US) >>",
		50: "(en-US)",
		60: "<< /Type /StructElem /S /Annot /P 7 0 R /Lang (en_US) >>",
	}
	annots := ""
	if annot != "" {
		objs[30] = annot
		annots = "/Annots [30 0 R]"
	}
	objs[3] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 /Tabs /S " + annots +
		" /Resources << /Font << /F1 5 0 R >> /Properties << /PL 40 0 R >> >> /Contents 4 0 R >>"
	out := ""
	if outline {
		objs[20] = "<< /Type /Outlines /First 21 0 R /Last 21 0 R /Count 1 >>"
		objs[21] = "<< /Title (One) /Parent 20 0 R /Dest [3 0 R /Fit] >>"
		out = " /Outlines 20 0 R"
	}
	objs[1] = "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R" + out + " " + cat + " >>"
	for k, v := range extra {
		objs[k] = v
	}
	return buildPDF(objs)
}

const langText = "/P << /MCID 0 >> BDC BT /F1 12 Tf 72 700 Td (x) Tj ET EMC"

// langSpanText wraps langText in a Span whose property list is props.
func langSpanText(props string) string { return "/Span " + props + " BDC " + langText + " EMC" }

const langNote = "<< /Type /Annot /Subtype /Text /Rect [0 0 10 10] /Contents (n) /StructParent 1 >>"

func TestTheLanguageRulesAgreeWithWhatVeraPDFMeasured(t *testing.T) {
	onlyThroughAnnot := bytes.Replace(langClauseDoc("/Lang (en-US)", "", langText, langNote, false),
		[]byte("1 8 0 R"), []byte("1 60 0 R"), 1)
	// A text field whose OWN /StructParent (its widget carries none) names element 60 — reachable no other way.
	onlyThroughField := langClauseDocWith(map[int]string{
		9:  "<< /Nums [0 [8 0 R] 1 60 0 R] >>",
		30: "<< /Type /Annot /Subtype /Widget /Rect [0 0 10 10] /Parent 31 0 R >>",
		31: "<< /FT /Tx /T (f) /DA (/Helv 0 Tf 0 g) /StructParent 1 /Kids [30 0 R] >>",
	}, "/Lang (en-US) /AcroForm << /Fields [31 0 R] >>", "", langText, "x", false)
	// An /Outlines whose /First names an object that does not exist, and no catalog /Lang: veraPDF runs no check.
	danglingFirst := bytes.Replace(langClauseDoc("", "", langText, "", true), []byte("/First 21 0 R"), []byte("/First 99 0 R"), 1)
	for _, tc := range []struct {
		name    string
		pdf     []byte
		t2, t29 Verdict
	}{
		{"an outline and no catalog /Lang", langClauseDoc("", "", langText, "", true), Fail, NotApplicable},
		{"an outline and an empty catalog /Lang", langClauseDoc("/Lang ()", "", langText, "", true), Pass, Fail},
		{"an outline and an indirect catalog /Lang", langClauseDoc("/Lang 50 0 R", "", langText, "", true), Pass, Pass},
		{"an outline and en-US", langClauseDoc("/Lang (en-US)", "", langText, "", true), Pass, Pass},
		{"no outline and no /Lang anywhere", langClauseDoc("", "", langText, "", false), NotApplicable, NotApplicable},
		{"en_US on the catalog", langClauseDoc("/Lang (en_US)", "", langText, "", false), NotApplicable, Fail},
		{"en- on the catalog", langClauseDoc("/Lang (en-)", "", langText, "", false), NotApplicable, Fail},
		{"a nine-letter subtag", langClauseDoc("/Lang (en-abcdefghi)", "", langText, "", false), NotApplicable, Fail},
		{"x-klingon", langClauseDoc("/Lang (x-klingon)", "", langText, "", false), NotApplicable, Pass},
		{"UTF-16 en-US", langClauseDoc("/Lang <FEFF0065006E002D00550053>", "", langText, "", false), NotApplicable, Pass},
		{"a digit first", langClauseDoc("/Lang (1en)", "", langText, "", false), NotApplicable, Fail},
		{"a space", langClauseDoc("/Lang (en US)", "", langText, "", false), NotApplicable, Fail},
		{"en_US on an element", langClauseDoc("/Lang (en-US)", "/Lang (en_US)", langText, "", false), NotApplicable, Fail},
		{"an empty /Lang on an element", langClauseDoc("/Lang (en-US)", "/Lang ()", langText, "", false), NotApplicable, Fail},
		{"en_US on an inline BDC property list", langClauseDoc("/Lang (en-US)", "", langSpanText("<< /Lang (en_US) >>"), "", false), NotApplicable, Fail},
		{"en_US on a named BDC property list", langClauseDoc("/Lang (en-US)", "", langSpanText("/PL"), "", false), NotApplicable, Fail},
		{"de on an inline BDC property list", langClauseDoc("/Lang (en-US)", "", langSpanText("<< /Lang (de) >>"), "", false), NotApplicable, Pass},
		// veraPDF 1.30.2 does not read a DP's property list for 7.2-29, whatever its source says.
		{"en_US on a DP property list", langClauseDoc("/Lang (en-US)", "", langText+" /Span << /Lang (en_US) >> DP", "", false), NotApplicable, Pass},
		{"en_US on an annotation's struct-parent element, also in the tree", langClauseDoc("/Lang (en-US)", "/Lang (en_US)", langText, langNote, false), NotApplicable, Fail},
		{"en_US on an element reachable only through an annotation", onlyThroughAnnot, NotApplicable, Fail},
		{"en_US on an element reachable only through a form field", onlyThroughField, NotApplicable, Fail},
		{"an outline whose first entry does not resolve", danglingFirst, NotApplicable, NotApplicable},
	} {
		if got := verdictOf(t, tc.pdf, "7.2 t2"); got.Verdict != tc.t2 {
			t.Errorf("%s: 7.2 t2 = %v (%s), want %v", tc.name, got.Verdict, got.Why, tc.t2)
		}
		if got := verdictOf(t, tc.pdf, "7.2 t29"); got.Verdict != tc.t29 {
			t.Errorf("%s: 7.2 t29 = %v (%s), want %v", tc.name, got.Verdict, got.Why, tc.t29)
		}
	}
}

// patternDoc builds a page whose unwalked marked content carries a Span property list with /Lang (bad), by mode:
// "pattern" a drawn tiling pattern; "unused" the same pattern, never drawn; "type3" a drawn Type 3 glyph whose font
// is its own object; "type3-direct" the same font written directly in /Resources; "form-in-pattern" a drawn pattern
// whose only content is a form carrying the /Lang. Measured on veraPDF 1.30.2: every drawn one FAILS 7.2-29 and the
// undrawn pattern passes; nib walks none of these streams, so every one is CannotCheck — never Pass, never Fail.
func patternDoc(mode, bad string) []byte {
	st := func(body string) string { return fmt.Sprintf("/Length %d >>\nstream\n%s\nendstream", len(body), body) }
	span := "/Span << /Lang " + bad + " >> BDC 0 0 5 5 re f EMC"
	tiling := "/Type /Pattern /PatternType 1 /PaintType 1 /TilingType 1 /BBox [0 0 10 10] /XStep 10 /YStep 10 "
	drawPattern := "/P << /MCID 0 >> BDC /Pattern cs /P0 scn 0 0 100 100 re f EMC"
	drawGlyph := "/P << /MCID 0 >> BDC BT /F3 12 Tf 72 700 Td (a) Tj ET EMC"
	type3 := "<< /Type /Font /Subtype /Type3 /FontBBox [0 0 10 10] /FontMatrix [0.001 0 0 0.001 0 0] /CharProcs << /a 22 0 R >> " +
		"/Encoding << /Differences [97 /a] >> /FirstChar 97 /LastChar 97 /Widths [1000] /Resources << >> >>"
	res, content, extra := "/Pattern << /P0 20 0 R >>", drawPattern, map[int]string{20: "<< " + tiling + "/Resources << >> " + st(span)}
	switch mode {
	case "unused":
		content = "/P << /MCID 0 >> BDC 0 0 100 100 re f EMC"
	case "type3", "type3-direct":
		res, content, extra = "/Font << /F3 21 0 R >>", drawGlyph, map[int]string{21: type3, 22: "<< " + st("0 0 d0 "+span)}
		if mode == "type3-direct" {
			res, extra = "/Font << /F3 "+type3+" >>", map[int]string{22: "<< " + st("0 0 d0 "+span)}
		}
	case "form-in-pattern":
		extra = map[int]string{
			20: "<< " + tiling + "/Resources << /XObject << /Fx 23 0 R >> >> " + st("/Fx Do"),
			23: "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] " + st(span),
		}
	}
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /Lang (en-US) /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 /Resources << " + res + " >> /Contents 4 0 R >>",
		4: "<< " + st(content),
		7: "<< /Type /StructTreeRoot /K 8 0 R /ParentTree 9 0 R >>",
		8: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 0 >>",
		9: "<< /Nums [0 [8 0 R]] >>",
	}
	for k, v := range extra {
		objs[k] = v
	}
	return buildPDF(objs)
}

func TestALanguageInAStreamNibDoesNotWalkIsCannotCheck(t *testing.T) {
	for _, mode := range []string{"pattern", "unused", "type3", "type3-direct", "form-in-pattern"} {
		// A direct Type 3 font never reaches nib at all (pdfcpu's validator drops it), so its reason is that one.
		reason := "does not walk"
		if mode == "type3-direct" {
			reason = "validator drops"
		}
		got := verdictOf(t, patternDoc(mode, "(en_US)"), "7.2 t29")
		if got.Verdict != CannotCheck || !strings.Contains(got.Why, reason) {
			t.Errorf("%s: 7.2 t29 = %v (%s), want CannotCheck naming %q", mode, got.Verdict, got.Why, reason)
		}
		if mode == "type3-direct" {
			continue // a valid /Lang there is just as unreadable, so it has no Pass control
		}
		// The control, per mode: a valid /Lang in the same stream changes nothing, and the catalog's passes.
		if got := verdictOf(t, patternDoc(mode, "(en-GB)"), "7.2 t29"); got.Verdict != Pass {
			t.Errorf("%s control: a valid /Lang reports %v (%s), want Pass", mode, got.Verdict, got.Why)
		}
	}
}

// The two CannotCheck paths the review found unguarded: a parent tree nib stopped reading, and a content walk that
// spent its budget. Each has a control that reaches a verdict.
func TestALanguageNibDidNotFinishReadingIsCannotCheck(t *testing.T) {
	if got := verdictOf(t, deepParentTree(3), "7.2 t29"); got.Verdict != Pass {
		t.Fatalf("control: a parent tree three levels deep reports %v (%s) for 7.2 t29, want Pass", got.Verdict, got.Why)
	}
	if got := verdictOf(t, deepParentTree(70), "7.2 t29"); got.Verdict != CannotCheck {
		t.Errorf("a parent tree past its bound reports %v (%s) for 7.2 t29, want CannotCheck", got.Verdict, got.Why)
	}
	if got := verdictOf(t, formFanOut(2, false), "7.2 t29"); got.Verdict != NotApplicable {
		t.Fatalf("control: two levels of forms report %v (%s) for 7.2 t29, want NotApplicable", got.Verdict, got.Why)
	}
	if got := verdictOf(t, formFanOut(7, false), "7.2 t29"); got.Verdict != CannotCheck || !strings.Contains(got.Why, "enters form XObjects") {
		t.Errorf("a content walk past its budget reports %v (%s) for 7.2 t29, want CannotCheck naming the budget", got.Verdict, got.Why)
	}
}

// catalogLangRead matches an index of the catalog by the "Lang" key, whatever the spacing.
var catalogLangRead = regexp.MustCompile(`Catalog\s*\[\s*"Lang"\s*\]`)

// TestTheCatalogLanguageIsReadThroughOneDoor — ADR-009: "does the catalog determine a language" is answered by
// `catalogLang` alone, which is veraPDF's `gContainsCatalogLang`, and every rule asks it there.
func TestTheCatalogLanguageIsReadThroughOneDoor(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	reads := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if n := len(catalogLangRead.FindAllString(string(b), -1)); n > 0 {
			reads += n
			if f != "rules_language.go" {
				t.Errorf("%s reads the catalog's /Lang itself; ask catalogLang or catalogDeclaresLang", f)
			}
		}
	}
	if reads != 1 {
		t.Errorf("the catalog's /Lang is read %d times in the package; want exactly once, in catalogLang", reads)
	}
}
