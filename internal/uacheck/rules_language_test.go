package uacheck

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
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
		// The document's ONLY /Lang is on a used BDC property list, so the marked-content half of 7.2 t29's
		// population is the only thing that can give it a subject (the slice's blind mutation pass).
		{"only a valid /Lang, on a BDC property list", langClauseDoc("", "", langSpanText("<< /Lang (de) >>"), "", false), NotApplicable, Pass},
		{"only a bad /Lang, on a BDC property list", langClauseDoc("", "", langSpanText("<< /Lang (de_DE) >>"), "", false), NotApplicable, Fail},
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

// P04.S02 — 7.2 t21, t22 and t23. Every row below is veraPDF 1.30.2's own verdict over eighty fixtures run
// before the rules were written (the slice's grill note in the plan has the table), mapped by the oracle's
// three-state rule: veraPDF failed → Fail, passed with checks → Pass, zero checks → NotApplicable.

// altElem is langClauseDoc's one /P element carrying extra entries, with nothing else changed.
func altElem(extra string) []byte { return langClauseDoc("", extra, langText, "", false) }

// altKid is a /Span element under the /P element: parent entries on the /P, kid entries on the Span.
func altKid(cat, parent, kid string) []byte {
	return langClauseDocWith(map[int]string{
		8:  "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [11 0 R] " + parent + " >>",
		11: "<< /Type /StructElem /S /Span /P 8 0 R /Pg 3 0 R /K 0 " + kid + " >>",
	}, cat, "", langText, "", false)
}

// altSideways is one shallow element whose /P names a chain of n dictionaries that are in no /K at all — the
// shape that makes the climb unbounded in a document the tree walk's own bound never sees. `tail` is what the
// far end does: extra entries (a /Lang), or "" to end the chain, or a /P back to its head with `altLoopTail`.
const altLoopTail = "/P 100 0 R"

func altSideways(n int, tail string) []byte {
	extra := map[int]string{
		8:  "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [11 0 R] >>",
		11: "<< /Type /StructElem /S /Span /P 100 0 R /Pg 3 0 R /K 0 /Alt (a) >>",
	}
	for i := 0; i < n; i++ {
		obj, entries := 100+i, fmt.Sprintf("/P %d 0 R", 100+i+1)
		if i == n-1 {
			entries = tail
		}
		extra[obj] = fmt.Sprintf("<< /Type /StructElem /S /Div %s >>", entries)
	}
	return langClauseDocWith(extra, "", "", langText, "", false)
}

// altDeepTree is a tree n elements deep whose only /Lang is on the StructTreeRoot and whose DEEPEST element
// carries the /Alt. An element at walk depth d has d element ancestors and the root above them, so this is the
// fixture that says whether the climb's bound can refuse an element the tree walk admitted.
func altDeepTree(n int) []byte {
	extra := map[int]string{
		7: "<< /Type /StructTreeRoot /K [100 0 R] /ParentTree 9 0 R /Lang (en-US) >>",
		9: fmt.Sprintf("<< /Nums [0 [%d 0 R]] >>", 100+n-1),
	}
	for i := 0; i < n; i++ {
		obj, parent := 100+i, fmt.Sprintf("%d 0 R", 100+i-1)
		if i == 0 {
			parent = "7 0 R"
		}
		kid := fmt.Sprintf("/K [%d 0 R]", obj+1)
		if i == n-1 {
			kid = "/K 0 /Pg 3 0 R /Alt (a)"
		}
		extra[obj] = fmt.Sprintf("<< /Type /StructElem /S /Span /P %s %s >>", parent, kid)
	}
	return langClauseDocWith(extra, "", "", langText, "", false)
}

func TestTheAlternateTextLanguageRulesAgreeWithWhatVeraPDFMeasured(t *testing.T) {
	// A structure tree root holding no elements at all — the only shape that makes the three clauses NotApplicable.
	noElements := langClauseDocWith(map[int]string{
		7: "<< /Type /StructTreeRoot /ParentTree 9 0 R >>",
		9: "<< /Nums [] >>",
	}, "", "", "BT /F1 12 Tf 72 700 Td (x) Tj ET", "", false)

	for _, tc := range []struct {
		name          string
		pdf           []byte
		t21, t22, t23 Verdict
	}{
		// The subject is every ELEMENT, not every element carrying the key: a tagged document with none of the
		// three keys passes all three clauses with checks, and only an empty tree has no subject.
		{"one element, none of the three keys", altElem(""), Pass, Pass, Pass},
		{"a structure tree root with no elements", noElements, NotApplicable, NotApplicable, NotApplicable},
		// One key at a time, with no language anywhere: only that clause fails.
		{"/Alt and no language", altElem("/Alt (a)"), Pass, Fail, Pass},
		{"/ActualText and no language", altElem("/ActualText (a)"), Fail, Pass, Pass},
		{"/E and no language", altElem("/E (a)"), Pass, Pass, Fail},
		{"all three and no language", altElem("/Alt (a) /ActualText (b) /E (c)"), Fail, Fail, Fail},
		// An EMPTY string is a subject — unlike 7.3 t1, which wants a non-empty /Alt — and so is a hex string
		// and an indirect one.
		{"an empty /Alt", altElem("/Alt ()"), Pass, Fail, Pass},
		{"an empty /ActualText", altElem("/ActualText ()"), Fail, Pass, Pass},
		{"a hex-string /Alt", altElem("/Alt <FEFF0041>"), Pass, Fail, Pass},
		{"an indirect /Alt", altElem("/Alt 50 0 R"), Pass, Fail, Pass},
		{"an indirect /ActualText", altElem("/ActualText 50 0 R"), Fail, Pass, Pass},
		{"an indirect /E", altElem("/E 50 0 R"), Pass, Pass, Fail},
		// The three ways a language is determined, and the empty and indirect forms of each.
		{"/Alt and the element's own /Lang", altElem("/Alt (a) /Lang (en-US)"), Pass, Pass, Pass},
		{"/Alt and an empty own /Lang", altElem("/Alt (a) /Lang ()"), Pass, Pass, Pass},
		{"/Alt and an indirect own /Lang", altElem("/Alt (a) /Lang 50 0 R"), Pass, Pass, Pass},
		{"/Alt and the catalog's /Lang", langClauseDoc("/Lang (en-US)", "/Alt (a)", langText, "", false), Pass, Pass, Pass},
		{"/Alt and the parent's /Lang", altKid("", "/Lang (en-US)", "/Alt (a)"), Pass, Pass, Pass},
		{"/Alt and the parent's empty /Lang", altKid("", "/Lang ()", "/Alt (a)"), Pass, Pass, Pass},
		{"/Alt and the parent's indirect /Lang", altKid("", "/Lang 50 0 R", "/Alt (a)"), Pass, Pass, Pass},
		{"/Alt and no /Lang above it either", altKid("", "", "/Alt (a)"), Pass, Fail, Pass},
		// The climb does not stop at the structure tree, and it skips nothing: a /Lang on the StructTreeRoot
		// itself satisfies the rule, and so does one on a pass-through ancestor `significantParent` climbs past.
		{"/Alt and a /Lang on the StructTreeRoot, one level up", langClauseDocWith(map[int]string{
			7:  "<< /Type /StructTreeRoot /K 8 0 R /ParentTree 9 0 R /Lang (en-US) >>",
			8:  "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [11 0 R] >>",
			11: "<< /Type /StructElem /S /Span /P 8 0 R /Pg 3 0 R /K 0 /Alt (a) >>",
		}, "", "", langText, "", false), Pass, Pass, Pass},
		{"/Alt and a /Lang on the StructTreeRoot the element names directly", langClauseDocWith(map[int]string{
			7: "<< /Type /StructTreeRoot /K 8 0 R /ParentTree 9 0 R /Lang (en-US) >>",
		}, "", "/Alt (a)", langText, "", false), Pass, Pass, Pass},
		{"/Alt under a Div carrying the /Lang", langClauseDocWith(map[int]string{
			8:  "<< /Type /StructElem /S /Div /P 7 0 R /Pg 3 0 R /K [11 0 R] /Lang (en-US) >>",
			11: "<< /Type /StructElem /S /Span /P 8 0 R /Pg 3 0 R /K 0 /Alt (a) >>",
		}, "", "", langText, "", false), Pass, Pass, Pass},
		// The climb follows /P wherever it points — including out of the tree, and onto a dictionary that is
		// not a structure element at all.
		{"/P names a non-ancestor element carrying a /Lang", langClauseDocWith(map[int]string{
			8:  "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [11 0 R] >>",
			11: "<< /Type /StructElem /S /Span /P 60 0 R /Pg 3 0 R /K 0 /Alt (a) >>",
		}, "", "", langText, "", false), Pass, Pass, Pass},
		{"/P names a plain dictionary carrying a /Lang", langClauseDocWith(map[int]string{
			8:  "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [11 0 R] >>",
			11: "<< /Type /StructElem /S /Span /P 40 0 R /Pg 3 0 R /K 0 /Alt (a) >>",
		}, "", "", langText, "", false), Pass, Pass, Pass},
		// No road up at all, and the two shapes that close one. A cycle is a COMPLETE answer — every ancestor
		// was seen and none carried a language — which is why it fails rather than reporting a refusal.
		{"an element with no /P at all", langClauseDocWith(map[int]string{
			8: "<< /Type /StructElem /S /P /Pg 3 0 R /K 0 /Alt (a) >>",
		}, "", "", langText, "", false), Pass, Fail, Pass},
		{"/P names the element itself", langClauseDocWith(map[int]string{
			8: "<< /Type /StructElem /S /P /P 8 0 R /Pg 3 0 R /K 0 /Alt (a) >>",
		}, "", "", langText, "", false), Pass, Fail, Pass},
		{"/P names a number", langClauseDocWith(map[int]string{
			8: "<< /Type /StructElem /S /P /P 5 /Pg 3 0 R /K 0 /Alt (a) >>",
		}, "", "", langText, "", false), Pass, Fail, Pass},
		{"a /P cycle of two with no /Lang anywhere", langClauseDocWith(map[int]string{
			8:  "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [11 0 R] >>",
			11: "<< /Type /StructElem /S /Span /P 12 0 R /Pg 3 0 R /K 0 /Alt (a) >>",
			12: "<< /Type /StructElem /S /Div /P 11 0 R >>",
		}, "", "", langText, "", false), Pass, Fail, Pass},
		// The cycle need not contain the asking element: veraPDF seeds its guard with that element's key AND adds
		// each ancestor, so a chain that runs into a cycle above itself ends with no answer just the same.
		{"a /P cycle beyond the element itself", langClauseDocWith(map[int]string{
			8:  "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [11 0 R] >>",
			11: "<< /Type /StructElem /S /Span /P 12 0 R /Pg 3 0 R /K 0 /Alt (a) >>",
			12: "<< /Type /StructElem /S /Div /P 13 0 R >>",
			13: "<< /Type /StructElem /S /Div /P 12 0 R >>",
		}, "", "", langText, "", false), Pass, Fail, Pass},
		{"a /P cycle reached only after the language", langClauseDocWith(map[int]string{
			8:  "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [11 0 R] >>",
			11: "<< /Type /StructElem /S /Span /P 12 0 R /Pg 3 0 R /K 0 /Alt (a) >>",
			12: "<< /Type /StructElem /S /Div /P 13 0 R >>",
			13: "<< /Type /StructElem /S /Div /P 12 0 R /Lang (en-US) >>",
		}, "", "", langText, "", false), Pass, Pass, Pass},
		// An element reachable ONLY through the parent tree is not a subject: veraPDF's elements come from the
		// tree walk, and the /Alt on this one is never checked (the tree's own element still passes).
		{"/Alt on an element only the parent tree names", langClauseDocWith(map[int]string{
			9:  "<< /Nums [0 [8 0 R] 1 60 0 R] >>",
			60: "<< /Type /StructElem /S /Annot /P 7 0 R /Alt (a) >>",
		}, "", "", langText, langNote, false), Pass, Pass, Pass},
	} {
		for _, c := range []struct {
			clause string
			want   Verdict
		}{{"7.2 t21", tc.t21}, {"7.2 t22", tc.t22}, {"7.2 t23", tc.t23}} {
			if got := verdictOf(t, tc.pdf, c.clause); got.Verdict != c.want {
				t.Errorf("%s: %s = %v (%s), want %v", tc.name, c.clause, got.Verdict, got.Why, c.want)
			}
		}
	}
}

// altJoin is two elements whose /P chains MEET: the near one climbs `chain` links to a /Lang, and the far one
// climbs `pre` links before joining the same chain at its head. It is the memo's trap — the near climb answers
// first and records the chain, and the far climb must still be refused where a fresh walk would have been.
func altJoin(pre, chain int, near, far bool) []byte {
	extra := map[int]string{}
	kids := []string{}
	if near {
		kids = append(kids, "11 0 R")
		extra[11] = "<< /Type /StructElem /S /Span /P 100 0 R /Pg 3 0 R /K 0 /Alt (a) >>"
	}
	if far {
		kids = append(kids, "12 0 R")
		extra[12] = "<< /Type /StructElem /S /Span /P 200 0 R /Pg 3 0 R /K 1 /Alt (a) >>"
	}
	extra[8] = "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [" + strings.Join(kids, " ") + "] >>"
	for i := 0; i < chain; i++ {
		entries := fmt.Sprintf("/P %d 0 R", 100+i+1)
		if i == chain-1 {
			entries = "/Lang (en-US)"
		}
		extra[100+i] = fmt.Sprintf("<< /Type /StructElem /S /Div %s >>", entries)
	}
	for i := 0; i < pre; i++ {
		next := 200 + i + 1
		if i == pre-1 {
			next = 100
		}
		extra[200+i] = fmt.Sprintf("<< /Type /StructElem /S /Div /P %d 0 R >>", next)
	}
	return langClauseDocWith(extra, "", "", langText+" /P << /MCID 1 >> BDC BT /F1 12 Tf 72 680 Td (y) Tj ET EMC", "", false)
}

// altThreeClimbs is the memo's SECOND-ORDER trap: three elements whose chains meet in sequence, so the third
// climb hits a memo entry that the second climb wrote THROUGH a memo hit of its own.
//
// X climbs `chain` links to a /Lang and memoises the chain. Y climbs three links, hits X's memo and completes, so
// its own three links are memoised with a distance carried across that hit. Z climbs three links and hits Y's —
// and at `chain` = maxLangClimb - 5 the true total is one past the bound while a distance that lost the linking
// hop is exactly at it. Without Z, two climbs cannot tell the two arithmetics apart (the slice's blind pass).
func altThreeClimbs(chain int, withZ bool) []byte {
	extra := map[int]string{}
	kids := []string{"11 0 R", "12 0 R"}
	extra[11] = "<< /Type /StructElem /S /Span /P 100 0 R /Pg 3 0 R /K 0 /Alt (a) >>"
	extra[12] = "<< /Type /StructElem /S /Span /P 200 0 R /Pg 3 0 R /K 1 /Alt (a) >>"
	if withZ {
		kids = append(kids, "13 0 R")
		extra[13] = "<< /Type /StructElem /S /Span /P 300 0 R /Pg 3 0 R /K 2 /Alt (a) >>"
	}
	extra[8] = "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [" + strings.Join(kids, " ") + "] >>"
	for i := 0; i < chain; i++ {
		entries := fmt.Sprintf("/P %d 0 R", 100+i+1)
		if i == chain-1 {
			entries = "/Lang (en-US)"
		}
		extra[100+i] = fmt.Sprintf("<< /Type /StructElem /S /Div %s >>", entries)
	}
	for base, join := range map[int]int{200: 100, 300: 200} {
		for i := 0; i < 3; i++ {
			next := base + i + 1
			if i == 2 {
				next = join
			}
			extra[base+i] = fmt.Sprintf("<< /Type /StructElem /S /Div /P %d 0 R >>", next)
		}
	}
	content := langText
	for i := 1; i < 3; i++ {
		content += fmt.Sprintf(" /P << /MCID %d >> BDC BT /F1 12 Tf 72 %d Td (y) Tj ET EMC", i, 680-i*20)
	}
	return langClauseDocWith(extra, "", "", content, "", false)
}

// TestAnAncestorLanguageNibStoppedClimbingForIsCannotCheck — law 1 over the one walk P04.S02 adds.
//
// The structure tree cannot be deep (pdfcpu refuses one past 100 levels), but a SIDEWAYS /P chain hanging off
// one shallow element is readable at any length: measured, veraPDF passes a 5,000-link chain. So the climb is
// bounded, and past the bound the clause says it could not look rather than passing or failing.
//
// **The controls sit ON the boundary, not near it.** `maxLangClimb` links must answer and `maxLangClimb + 1`
// must refuse; a 60-against-70 pair leaves the bound's VALUE untested, and the slice's review measured three
// mutations of it (±3 either way) that such a pair keeps green.
func TestAnAncestorLanguageNibStoppedClimbingForIsCannotCheck(t *testing.T) {
	// Exactly at the bound the climb reaches both answers.
	if got := verdictOf(t, altSideways(maxLangClimb, "/Lang (en-US)"), "7.2 t22"); got.Verdict != Pass {
		t.Fatalf("control: a %d-link /P chain with a /Lang at its end reports %v (%s), want Pass", maxLangClimb, got.Verdict, got.Why)
	}
	if got := verdictOf(t, altSideways(maxLangClimb, ""), "7.2 t22"); got.Verdict != Fail {
		t.Fatalf("control: a %d-link /P chain with no /Lang reports %v (%s), want Fail", maxLangClimb, got.Verdict, got.Why)
	}
	// One link further, neither answer is claimed — including the Fail, which is the tempting one.
	for _, tc := range []struct {
		name string
		pdf  []byte
	}{
		{"with a /Lang past the bound", altSideways(maxLangClimb+1, "/Lang (en-US)")},
		{"with no /Lang at all", altSideways(maxLangClimb+1, "")},
		{"that closes into a loop past the bound", altSideways(maxLangClimb+1, altLoopTail)},
	} {
		got := verdictOf(t, tc.pdf, "7.2 t22")
		if got.Verdict != CannotCheck || !strings.Contains(got.Why, "climbs through more than") {
			t.Errorf("a %d-link /P chain %s reports %v (%s), want CannotCheck naming the bound", maxLangClimb+1, tc.name, got.Verdict, got.Why)
		}
	}
	// **The bound cannot refuse an element the TREE WALK admitted** — the reason it is `maxWalkDepth + 1` rather
	// than `maxWalkDepth`. The deepest admitted element has `maxWalkDepth` element ancestors AND the
	// StructTreeRoot above them, and measured on veraPDF 1.30.2 all three of these pass: at a bare
	// `maxWalkDepth` the middle one answered CannotCheck.
	for _, n := range []int{maxWalkDepth - 1, maxWalkDepth, maxWalkDepth + 1} {
		if got := verdictOf(t, altDeepTree(n), "7.2 t22"); got.Verdict != Pass {
			t.Errorf("a tree %d elements deep whose only /Lang is on the StructTreeRoot reports %v (%s), want Pass", n, got.Verdict, got.Why)
		}
	}
	// One deeper is past the TREE's bound, so the refusal is the walk's and names the tree, not the climb.
	if got := verdictOf(t, altDeepTree(maxWalkDepth+2), "7.2 t22"); got.Verdict != CannotCheck || !strings.Contains(got.Why, "nests deeper than") {
		t.Errorf("a tree past the walk's own bound reports %v (%s), want CannotCheck naming the tree walk", got.Verdict, got.Why)
	}
	// **A distance carried ACROSS a memo hit must not lose the hop that reached it.** Two climbs cannot see this:
	// the second one's refusal does not depend on what it would have memoised. The third can.
	if got := verdictOf(t, altThreeClimbs(maxLangClimb-5, false), "7.2 t22"); got.Verdict != Pass {
		t.Fatalf("control: the two climbs that stay inside the bound report %v (%s), want Pass", got.Verdict, got.Why)
	}
	if got := verdictOf(t, altThreeClimbs(maxLangClimb-5, true), "7.2 t22"); got.Verdict != CannotCheck ||
		!strings.Contains(got.Why, "climbs through more than") {
		t.Errorf("the third climb, one link past the bound through two memo hits, reports %v (%s), want CannotCheck", got.Verdict, got.Why)
	}
	// **A memo hit must not let a deeper climb skip the bound** (P03's kid-depth lesson, in the other direction).
	// The near element's climb answers at exactly the bound and records the chain; the far element joins that
	// chain one link up, so its own climb is one too long and must be refused — with the memo warm and cold alike.
	if got := verdictOf(t, altJoin(1, maxLangClimb, true, false), "7.2 t22"); got.Verdict != Pass {
		t.Fatalf("control: the near element alone reports %v (%s), want Pass", got.Verdict, got.Why)
	}
	for _, tc := range []struct {
		name      string
		near, far bool
	}{
		{"cold", false, true},
		{"warmed by the near element's answer", true, true},
	} {
		got := verdictOf(t, altJoin(1, maxLangClimb, tc.near, tc.far), "7.2 t22")
		if got.Verdict != CannotCheck || !strings.Contains(got.Why, "climbs through more than") {
			t.Errorf("the far element, %s, reports %v (%s), want CannotCheck naming the bound", tc.name, got.Verdict, got.Why)
		}
	}
}

// TestTheWarmedClimbReallyHitsTheMemo — the stimulus behind the row above.
//
// Cold and warm expect the same verdict, so if the memo silently stopped being hit (a pdfcpu change that made
// `DereferenceDict` return copies would do it, since `dictID` is a pointer) both rows would stay green while the
// guard they exist to probe was never reached. This asserts the mechanism ran before the verdict is graded.
func TestTheWarmedClimbReallyHitsTheMemo(t *testing.T) {
	d, err := open(altJoin(1, maxLangClimb, true, true))
	if err != nil {
		t.Fatal(err)
	}
	nodes, unread := d.structNodes()
	if unread != "" {
		t.Fatalf("the fixture's tree was not fully read: %s", unread)
	}
	var climbed []types.Dict
	for _, n := range nodes {
		if _, has := d.text(n.dict["Alt"]); has {
			climbed = append(climbed, n.dict)
		}
	}
	if len(climbed) != 2 {
		t.Fatalf("the fixture has %d element(s) carrying /Alt, want 2 — the near one and the far one", len(climbed))
	}
	if found, why := d.parentLang(climbed[0]); !found || why != "" {
		t.Fatalf("the near element reports found=%v (%s), want the /Lang at the end of its chain", found, why)
	}
	warm := len(d.langs)
	if warm == 0 {
		t.Fatal("the near element's climb memoised nothing, so the far element's climb cannot hit the memo and " +
			"the bound-past-a-memo guard is never reached")
	}
	found, why := d.parentLang(climbed[1])
	if found || why == "" {
		t.Errorf("the far element reports found=%v (%s), want the bound's refusal", found, why)
	}
	if len(d.langs) != warm {
		t.Errorf("the far element's climb added %d memo entr(ies); it should have HIT the memo at the chain's head "+
			"and been refused there, and a refusal is never memoised", len(d.langs)-warm)
	}
	// **"Only a dictionary with no /Lang of its own is ever memoised"** — the claim the memo-hit branch rests on,
	// since it returns the memo without re-reading the dictionary it hit. Executed over every dictionary in the
	// fixture's chain, including the one that HOLDS the language.
	memoised, holders := 0, 0
	for obj := 100; obj < 100+maxLangClimb; obj++ {
		dict := d.dict(types.IndirectRef{ObjectNumber: types.Integer(obj)})
		if dict == nil {
			continue
		}
		_, hasLang := d.text(dict["Lang"])
		if hasLang {
			holders++
		}
		if _, inMemo := d.langs[dictID(dict)]; !inMemo {
			continue
		}
		memoised++
		if hasLang {
			t.Errorf("object %d carries its own /Lang and is in the memo; the memo-hit branch answers without "+
				"re-reading it, so its own language would be lost for every asker below it", obj)
		}
	}
	if memoised == 0 || holders == 0 {
		t.Fatalf("the check read %d memoised dictionar(ies) and %d /Lang holder(s) in the chain; it needs both to "+
			"mean anything", memoised, holders)
	}
}

// TestAReferenceToAnObjectThatIsNotThereIsNotAValue — the slice review's critical, both directions.
//
// pdfcpu's `DereferenceStringOrHexLiteral` answers `("", nil)` for a reference to a FREE object, which reads here
// exactly like an empty string that is present — and pdfcpu's validator does not refuse such a file. Measured on
// veraPDF 1.30.2: the dangling `/Alt` is no subject (it passes) and the dangling `/Lang` leaves the element with
// no language of its own (it fails), and before the fix nib answered the opposite of each.
func TestAReferenceToAnObjectThatIsNotThereIsNotAValue(t *testing.T) {
	for _, tc := range []struct {
		name  string
		elem  string
		want  Verdict
		whyIt string
	}{
		{"control: a resolvable indirect /Alt is a subject", "/Alt 50 0 R", Fail, "the string resolves, so the element carries alternate text and no language"},
		{"control: a resolvable indirect /Lang determines it", "/Alt (a) /Lang 50 0 R", Pass, "the string resolves, so the element declares its own language"},
		{"a dangling /Alt is no subject", "/Alt 99 0 R", Pass, "veraPDF has no subject for it"},
		{"a dangling /Lang is no language", "/Alt (a) /Lang 99 0 R", Fail, "veraPDF fails it"},
	} {
		pdf := langClauseDocWith(map[int]string{
			8: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 0 " + tc.elem + " >>",
		}, "", "", langText, "", false)
		if got := verdictOf(t, pdf, "7.2 t22"); got.Verdict != tc.want {
			t.Errorf("%s: 7.2 t22 = %v (%s), want %v — %s", tc.name, got.Verdict, got.Why, tc.want, tc.whyIt)
		}
	}
}

// alternateClauseLiteral matches the three clause ids wherever the package writes one.
var alternateClauseLiteral = regexp.MustCompile(`"7\.2 t(21|22|23)"`)

// TestTheAlternateTextLanguageClausesAreOneRelation — ADR-009. veraPDF's three predicates differ only in which
// key they name, so they are one table and one evaluator, and the guard asserts the ROUTING: each registered
// check answers exactly what the door answers for its row, over documents that separate the three keys.
func TestTheAlternateTextLanguageClausesAreOneRelation(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	sites := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if n := len(alternateClauseLiteral.FindAllString(string(b), -1)); n > 0 {
			sites += n
			if f != "rules_language.go" {
				t.Errorf("%s names one of 7.2 t21/t22/t23 itself; the three clauses are rows of alternateTextKeys", f)
			}
		}
	}
	if sites != len(alternateTextKeys) {
		t.Errorf("the three alternate-text clauses are written %d times in the package; want %d, once each in alternateTextKeys", sites, len(alternateTextKeys))
	}
	// **Routing, not agreement.** A second implementation that agrees on a handful of documents would pass an
	// agreement check; what is asserted is that the three registered checks are ONE function — the closure the
	// table's loop builds — so a clause registered with a hand-written check of its own is caught even when it
	// never writes the clause id a fourth time.
	body := func(c string) string {
		r, ok := registry[c]
		if !ok {
			t.Fatalf("%s is in alternateTextKeys and not in the registry", c)
		}
		return runtime.FuncForPC(reflect.ValueOf(r.Check).Pointer()).Name()
	}
	first := body(alternateTextKeys[0].clause)
	if first == "" {
		t.Fatal("the registered check has no resolvable function name, so this guard compares nothing")
	}
	for _, k := range alternateTextKeys[1:] {
		if got := body(k.clause); got != first {
			t.Errorf("%s is checked by %s and %s by %s — the three clauses are not one evaluator",
				k.clause, got, alternateTextKeys[0].clause, first)
		}
	}
	// And the one evaluator answers what the door answers, over documents that separate the three keys.
	for _, k := range alternateTextKeys {
		rule := registry[k.clause]
		for _, pdf := range [][]byte{
			altElem("/Alt (a)"), altElem("/ActualText (a)"), altElem("/E (a)"),
			altElem("/Alt (a) /ActualText (b) /E (c)"), altElem(""), altKid("", "/Lang (en-US)", "/Alt (a)"),
			altSideways(maxLangClimb+1, "/Lang (en-US)"),
		} {
			d, err := open(pdf)
			if err != nil {
				t.Fatal(err)
			}
			door := checkAlternateTextLanguage(d, k.key, k.what)
			e, err := open(pdf)
			if err != nil {
				t.Fatal(err)
			}
			got := rule.Check(e)
			if got.Verdict != door.Verdict || got.Why != door.Why || got.Where != door.Where {
				t.Errorf("%s's registered check answers %v (%s) where the door answers %v (%s) — it is not routed through it",
					k.clause, got.Verdict, got.Why, door.Verdict, door.Why)
			}
		}
	}
}
