package uacheck

import (
	"fmt"
	"strings"
	"testing"
)

// The package's convention — a definite failure beats a refusal — held at every site the P07 phase-close review
// found returning at its first refusal (R4-7). Each fixture holds one subject nib cannot settle, FIRST, and one
// that definitely fails, after it. Every row's Fail is veraPDF 1.30.2's verdict on exactly that document.

// heldContentDoc is a page whose content names two elements: MCID 0 an element whose /P chain runs seventy
// links without reaching the root — past `maxLangClimb`, so nib cannot settle whether it is tagged or what
// language it has — and MCID 1 an element directly under the root. No /Lang anywhere.
func heldContentDoc(content string) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 " +
			"/Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7: "<< /Type /StructTreeRoot /K [8 0 R] /ParentTree 9 0 R >>",
		8: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 1 >>",
		9: "<< /Nums [0 [20 0 R 8 0 R]] >>",
	}
	const links = 70
	for i := 0; i < links; i++ {
		p, k := "", ""
		if i < links-1 {
			p = fmt.Sprintf(" /P %d 0 R", 21+i)
		}
		if i == 0 {
			k = " /Pg 3 0 R /K 0"
		}
		objs[20+i] = fmt.Sprintf("<< /Type /StructElem /S /Div%s%s >>", p, k)
	}
	return buildPDF(objs)
}

// twoAnnots is annotFixture with a second annotation (object 31), and the first's element typed by a role map.
func twoAnnots(first, second, roleMap string) []byte {
	return annotFixture{annot: first, elem: "/S /Alpha", annots: "[30 0 R 31 0 R]", extra: map[int]string{
		31: second,
		7:  "<< /Type /StructTreeRoot /K [8 0 R 10 0 R] /ParentTree 9 0 R /RoleMap << " + roleMap + " >> >>",
	}}.build()
}

// shortTreeAnnots is annotFixture with a second annotation, and a parent tree whose key 9 lies past the lookup's
// depth bound, so an annotation naming 9 is one nib cannot resolve while key 1 resolves. The root is inline, because
// pdfcpu refuses to open an object-rooted number tree deeper than 100. Past the bound the DOCUMENT is refused too
// (RR3-2), so these rows run the rule itself (`ruleVerdict`): what they measure is each clause's own held refusal.
func shortTreeAnnots(first, second string) []byte {
	extra := map[int]string{}
	for i := 0; i < maxParentTreeDepth; i++ {
		extra[300+i] = fmt.Sprintf("<< /Kids [%d 0 R] /Limits [9 9] >>", 301+i)
	}
	extra[300+maxParentTreeDepth] = "<< /Nums [9 11 0 R] /Limits [9 9] >>"
	extra[11] = "<< /Type /StructElem /S /Annot /P 7 0 R /Alt (described) >>"
	extra[290] = "<< /Nums [0 [8 0 R] 1 10 0 R] /Limits [0 1] >>"
	extra[7] = "<< /Type /StructTreeRoot /K [8 0 R 10 0 R 11 0 R] /ParentTree << /Kids [290 0 R 300 0 R] >> >>"
	extra[31] = second
	extra[40] = "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length 0 >>\nstream\n\nendstream"
	return annotFixture{annot: first, elem: "/S /Annot", annots: "[30 0 R 31 0 R]", extra: extra}.build()
}

// heldCIDSetDoc draws first in /F1, a Type 0 font over a subset-named CIDFontType0 whose /FontFile3 /OpenType holds a
// "CFF " table (nib does not read inside the wrapper) and whose /CIDSet marks CID 1 only — so 7.21.4.2 t2 cannot be
// settled on it — and then in /F0, a subset-named CIDFontType2 whose program holds 100 glyphs, under the /CIDSet `set`.
func heldCIDSetDoc(set string) []byte {
	sub := "ABCDEF+Probe"
	cidStd := cidSpec{cids: []int{1, 2, 3}, charstrings: [][]byte{cs(0, "endchar"), cs(500, "endchar"), cs(500, "endchar"),
		cs(500, "endchar")}}.build()
	objs := map[int]string{
		11: "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /" + sub + " /CIDSystemInfo << /Registry (Adobe) /Ordering " +
			"(Identity) /Supplement 0 >> /FontDescriptor 12 0 R /CIDToGIDMap /Identity >>",
	}
	for k, v := range ttObjects("/Flags 4", "FontFile2", ttCounts([]int{100}, []int{100}, ttCmap(ttSub{3, 1, cmapFmt4(0x20, 95)})), "") {
		objs[k] = v
	}
	objs[12] = strings.TrimSuffix(objs[12], " >>") + " /CIDSet 30 0 R >>"
	objs[30] = set
	objs[40] = "<< /Type /Font /Subtype /Type0 /BaseFont /" + sub + " /Encoding /Identity-H /DescendantFonts [41 0 R] >>"
	objs[41] = "<< /Type /Font /Subtype /CIDFontType0 /BaseFont /" + sub + " /CIDSystemInfo << /Registry (Adobe) /Ordering " +
		"(Identity) /Supplement 0 >> /FontDescriptor 42 0 R /W [1 [500 500 500]] >>"
	objs[42] = "<< /Type /FontDescriptor /FontName /" + sub + " /Flags 4 /FontBBox [0 0 1000 1000] /ItalicAngle 0 /Ascent 800 " +
		"/Descent -200 /CapHeight 700 /StemV 80 /FontFile3 43 0 R /CIDSet 44 0 R >>"
	objs[43] = spStream("/Subtype /OpenType", string(sfnt(ttTable{"CFF ", cidStd})))
	objs[44] = cidSetBytes(1)
	return glyphPage("BT /F1 12 Tf 10 10 Td <00010002> Tj /F0 12 Tf <00010002> Tj ET", "/F1 40 0 R", "",
		"<< /Type /Font /Subtype /Type0 /BaseFont /"+sub+" /Encoding /Identity-H /DescendantFonts [11 0 R] >>", objs)
}

// heldCMapDoc draws first in /F0, over an embedded CMap declaring /WMode 1 in both its dictionary and its program and
// then a `cidchar` of the wrong kind (a CMap veraPDF discards, R2-5 — so nib refuses it), and then in /F1 over an
// embedded CMap with no dictionary /WMode whose program is `second`.
func heldCMapDoc(second string) []byte {
	sys := "/CIDSystemInfo << /Registry (Adobe) /Ordering (GB1) /Supplement 2 >>"
	return buildPDF(type0Doc("20 0 R", sys, map[int]string{
		20: cmapStream("/WMode 1 "+sys, "Cust", "/WMode 1 def 1 begincidchar <0021> (x) endcidchar"),
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R /StructParents 0 " +
			"/Resources << /Font << /F0 10 0 R /F1 40 0 R >> >> >>",
		4:  spStream("", "/P <</MCID 0>> BDC BT /F0 12 Tf 10 10 Td <2121> Tj /F1 12 Tf <2121> Tj ET EMC"),
		40: "<< /Type /Font /Subtype /Type0 /BaseFont /STSong-Light-Two /Encoding 21 0 R /DescendantFonts [11 0 R] >>",
		21: cmapStream(sys, "Cust2", second),
	}))
}

func TestADefiniteFailureBeatsARefusalAtEverySite(t *testing.T) {
	const text = "BT /F1 12 Tf 72 700 Td (x) Tj ET"
	const loop = "/Alpha /Beta /Beta /Alpha"
	widget := func(sp string) string {
		return note("Subtype", "/Widget", "FT", "/Btn", "T", "(b)", "StructParent", sp)
	}
	mark := func(sp string) string {
		return note("Subtype", "/PrinterMark", "StructParent", sp, "AP", "<< /N 40 0 R >>")
	}
	for _, tc := range []struct {
		name, clause string
		// held is the document with its failing subject; control is the same document with that subject made
		// to pass or removed, which must be CannotCheck — the proof the first subject IS a refusal.
		held, control []byte
	}{
		{"7.1 t3: coverage past the climb, then untagged text", "7.1 t3",
			heldContentDoc("/P << /MCID 0 >> BDC " + text + " EMC " + text),
			heldContentDoc("/P << /MCID 0 >> BDC " + text + " EMC")},
		{"7.1 t1: an artifact in unsettled content, then one in tagged content", "7.1 t1",
			heldContentDoc("/P << /MCID 0 >> BDC /Artifact BMC " + text + " EMC EMC /P << /MCID 1 >> BDC /Artifact BMC " + text + " EMC EMC"),
			heldContentDoc("/P << /MCID 0 >> BDC /Artifact BMC " + text + " EMC EMC")},
		{"7.1 t2: unsettled content in an artifact, then tagged content in one", "7.1 t2",
			heldContentDoc("/Artifact BMC /P << /MCID 0 >> BDC " + text + " EMC EMC /Artifact BMC /P << /MCID 1 >> BDC " + text + " EMC EMC"),
			heldContentDoc("/Artifact BMC /P << /MCID 0 >> BDC " + text + " EMC EMC")},
		{"7.2 t31: a Span whose language is unsettled, then one with none", "7.2 t31",
			heldContentDoc("/Span << /MCID 0 /Alt (a) >> BDC " + text + " EMC /Span << /MCID 1 /Alt (b) >> BDC " + text + " EMC"),
			heldContentDoc("/Span << /MCID 0 /Alt (a) >> BDC " + text + " EMC")},
		{"7.2 t34: text whose language is unsettled, then text with none", "7.2 t34",
			heldContentDoc("/P << /MCID 0 >> BDC " + text + " EMC /P << /MCID 1 >> BDC " + text + " EMC"),
			heldContentDoc("/P << /MCID 0 >> BDC " + text + " EMC")},
		{"7.18.1 t1: an annotation on a role-map loop, then one with no /StructParent", "7.18.1 t1",
			twoAnnots(note(), note("StructParent", ""), loop),
			twoAnnots(note(), note("StructParent", "1"), loop)},
		{"7.18.5 t1: a link on a role-map loop, then one with no /StructParent", "7.18.5 t1",
			twoAnnots(note("Subtype", "/Link"), note("Subtype", "/Link", "StructParent", ""), loop),
			twoAnnots(note("Subtype", "/Link"), note("Subtype", "/Link"), loop)},
		{"7.18.4 t1: a widget on a role-map loop, then one with no /StructParent", "7.18.4 t1",
			twoAnnots(widget("1"), widget(""), loop),
			twoAnnots(widget("1"), widget("1"), loop)},
		// The same three clauses, where the first subject's /StructParent row is past the parent tree's bound.
		{"7.18.1 t1: an annotation past the parent tree's bound, then one with no /StructParent", "7.18.1 t1",
			shortTreeAnnots(note("StructParent", "9"), note("StructParent", "")),
			shortTreeAnnots(note("StructParent", "9"), note())},
		{"7.18.5 t1: a link past the parent tree's bound, then one with no /StructParent", "7.18.5 t1",
			shortTreeAnnots(note("Subtype", "/Link", "StructParent", "9"), note("Subtype", "/Link", "StructParent", "")),
			shortTreeAnnots(note("Subtype", "/Link", "StructParent", "9"), note("Subtype", "/Link", "StructParent", "9"))},
		{"7.18.4 t1: a widget past the parent tree's bound, then one with no /StructParent", "7.18.4 t1",
			shortTreeAnnots(widget("9"), widget("")),
			shortTreeAnnots(widget("9"), widget("9"))},
		{"7.18.1 t2: an annotation past the parent tree's bound, then an undescribed one", "7.18.1 t2",
			shortTreeAnnots(note("StructParent", "9", "Contents", ""), note("Contents", "")),
			shortTreeAnnots(note("StructParent", "9", "Contents", ""), note())},
		{"7.18.1 t3: a widget past the parent tree's bound, then an undescribed one", "7.18.1 t3",
			shortTreeAnnots(widget("9"), widget("1")),
			shortTreeAnnots(widget("9"), note("Subtype", "/Widget", "FT", "/Btn", "T", "(b)", "TU", "(a button)"))},
		{"7.18.8 t1: a printer's mark past the parent tree's bound, then one in the tree", "7.18.8 t1",
			shortTreeAnnots(mark("9"), mark("1")),
			shortTreeAnnots(mark("9"), mark(""))},
		// The P07 phase-close re-review (RR1-1, RR1-4): two more sites returned at their first refusal.
		{"7.21.4.2 t2: a CIDFont whose program nib does not read, then a short /CIDSet", "7.21.4.2 t2",
			heldCIDSetDoc(cidSetBytes(1)), heldCIDSetDoc(cidSetBytes(cidRange(0, 100)...))},
		{"7.21.3.3 t2: a malformed CMap, then a program /WMode its dictionary does not declare", "7.21.3.3 t2",
			heldCMapDoc("/WMode 1 def 1 begincidchar <2121> 5 endcidchar"),
			heldCMapDoc("1 begincidchar <2121> 5 endcidchar")},
	} {
		// The stimulus first: without the failing subject, the document is a refusal.
		if got := ruleVerdict(t, tc.control, tc.clause); got.Verdict != CannotCheck {
			t.Errorf("%s: control = %v (%s), want CannotCheck — the first subject must be one nib cannot settle, "+
				"or the row below proves nothing", tc.name, got.Verdict, got.Why)
			continue
		}
		if got := ruleVerdict(t, tc.held, tc.clause); got.Verdict != Fail {
			t.Errorf("%s: %s = %v (%s), want Fail — veraPDF's verdict; a refusal about one subject threw away "+
				"the failure of another", tc.name, tc.clause, got.Verdict, got.Why)
		}
	}
}

// budgetDoc is heldContentDoc's tree under a page that draws `prefix` and then a form of twenty rectangles
// 60,000 times — 1.2 million drawing operators, past the content walk's event budget, so the walk stops after
// the prefix has been read (`TestEachContentBudgetHoldsOnItsOwn`'s shape).
func budgetDoc(prefix string) []byte { return budgetDocWith(prefix, "") }

// budgetDocWith is budgetDoc with extra catalog entries.
func budgetDocWith(prefix, cat string) []byte {
	body := strings.Repeat("0 0 1 1 re f ", 20)
	content := prefix + " " + strings.Repeat("/X0 Do ", 60000)
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R " + cat + " >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 " +
			"/Resources << /Font << /F1 5 0 R >> /XObject << /X0 100 0 R >> >> /Contents 4 0 R >>",
		4:   fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5:   "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7:   "<< /Type /StructTreeRoot /K [8 0 R] /ParentTree 9 0 R >>",
		8:   "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 1 >>",
		9:   "<< /Nums [0 [8 0 R 8 0 R]] >>",
		100: fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length %d >>\nstream\n%s\nendstream", len(body), body),
	}
	return buildPDF(objs)
}

// TestAStoppedContentWalkIsAnsweredAfterTheSubjectsItRead — the walk-stop half of R4-7. The marked-content rules
// and 7.2 t34 read a population the content walk builds, and a walk that stopped at a budget left every subject
// it HAD recorded unjudged. Each subject here is settled where it was drawn, before the stop.
func TestAStoppedContentWalkIsAnsweredAfterTheSubjectsItRead(t *testing.T) {
	const text = "BT /F1 12 Tf 72 700 Td (x) Tj ET"
	// Two rows, not five: the marked-content rules answer through ONE helper (`markedVerdict`), so 7.1 t1 stands
	// for all three, and each budget document costs seconds to walk. The failing subjects are the shapes
	// `TestADefiniteFailureBeatsARefusalAtEverySite` measured on veraPDF.
	for _, tc := range []struct {
		clause, failing, passing string
	}{
		// An /Artifact inside tagged content, drawn before the stop.
		{"7.1 t1", "/P << /MCID 1 >> BDC /Artifact BMC " + text + " EMC EMC", "/P << /MCID 1 >> BDC " + text + " EMC"},
		// Text with no language anywhere.
		{"7.2 t34", "/P << /MCID 1 >> BDC " + text + " EMC", "/P << /MCID 1 >> BDC EMC"},
	} {
		// The stimulus first: the same walk, stopped at the same budget, over a prefix that does not fail.
		control := verdictOf(t, budgetDoc(tc.passing), tc.clause)
		if control.Verdict != CannotCheck || !strings.Contains(control.Why, "drawing operators") {
			t.Errorf("%s control = %v (%s), want CannotCheck naming the event budget", tc.clause, control.Verdict, control.Why)
			continue
		}
		if got := verdictOf(t, budgetDoc(tc.failing), tc.clause); got.Verdict != Fail {
			t.Errorf("%s = %v (%s), want Fail — the subject drawn before the stop fails, and the stop is a "+
				"refusal about what came after it", tc.clause, got.Verdict, got.Why)
		}
	}
}

// TestACatalogLangAnswersAStoppedWalkOnlyOnceItHasASubject — `/pending 703`. A catalog `/Lang` passes every piece of
// page text, so ONE subject read before the walk stopped settles 7.2 t34: whatever the walk did not reach would pass
// too. With none read, the catalog settles nothing — the document may show no text at all, which is veraPDF's "no
// subject" — so the stop is answered, where the rule used to return Pass without reading the page.
func TestACatalogLangAnswersAStoppedWalkOnlyOnceItHasASubject(t *testing.T) {
	const text = "BT /F1 12 Tf 72 700 Td (x) Tj ET"
	if got := verdictOf(t, budgetDocWith("/P << /MCID 1 >> BDC "+text+" EMC", "/Lang (en-US)"), "7.2 t34"); got.Verdict != Pass {
		t.Errorf("text read before the stop, under a catalog /Lang: 7.2 t34 = %v (%s), want Pass", got.Verdict, got.Why)
	}
	got := verdictOf(t, budgetDocWith("/P << /MCID 1 >> BDC EMC", "/Lang (en-US)"), "7.2 t34")
	if got.Verdict != CannotCheck || !strings.Contains(got.Why, "drawing operators") {
		t.Errorf("no text read before the stop, under a catalog /Lang: 7.2 t34 = %v (%s), want CannotCheck naming the "+
			"event budget", got.Verdict, got.Why)
	}
}

// TestAHeldRefusalNamesTheFirstSubject —the early return it replaced reported the FIRST subject nib could not
// settle, and so does the held one; an empty reason holds nothing.
func TestAHeldRefusalNamesTheFirstSubject(t *testing.T) {
	var h heldRefusal
	if _, ok := h.result(); ok {
		t.Fatal("an empty heldRefusal answers a refusal")
	}
	h.hold("", "nowhere")
	h.hold("first", "here")
	h.hold("second", "there")
	r, ok := h.result()
	if !ok || r.Verdict != CannotCheck || r.Why != "first" || r.Where != "here" {
		t.Errorf("held %v %q at %q, want CannotCheck \"first\" at \"here\"", r.Verdict, r.Why, r.Where)
	}
}
