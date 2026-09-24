package uacheck

import (
	"strings"
	"testing"
)

// type0Doc builds a one-page document showing text in a Type 0 font (P07.S01): enc is the font's /Encoding value,
// cid the CIDFont's extra entries, and extra merges in further objects (an embedded CMap at 20).
func type0Doc(enc, cid string, extra map[int]string) map[int]string {
	o := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 5 0 R /Lang (en) >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R /StructParents 0 " +
			"/Resources << /Font << /F0 10 0 R >> >> >>",
		// **Tagged, one P element over the one text run** — so the documents exercise the CMap clauses and not
		// two unrelated divergences an untagged page reaches (/pending 674: an empty structure root, and
		// marked-content clauses answering Pass with no marked content).
		4:  spStream("", "/P <</MCID 0>> BDC BT /F0 12 Tf 10 10 Td <2121> Tj ET EMC"),
		5:  "<< /Type /StructTreeRoot /K [6 0 R] /ParentTree 7 0 R >>",
		6:  "<< /Type /StructElem /S /P /P 5 0 R /Pg 3 0 R /K [0] >>",
		7:  "<< /Nums [0 [6 0 R]] >>",
		10: "<< /Type /Font /Subtype /Type0 /BaseFont /STSong-Light /Encoding " + enc + " /DescendantFonts [11 0 R] >>",
		11: "<< /Type /Font /Subtype /CIDFontType0 /BaseFont /STSong-Light " + cid + " /FontDescriptor 12 0 R >>",
		12: "<< /Type /FontDescriptor /FontName /STSong-Light /Flags 4 /FontBBox [0 0 1000 1000] /ItalicAngle 0 " +
			"/Ascent 880 /Descent -120 /CapHeight 880 /StemV 80 >>",
	}
	for k, v := range extra {
		o[k] = v
	}
	return o
}

// cmapStream is an embedded CMap named `name`, with `dict` added to its stream dictionary and `body` to its program.
func cmapStream(dict, name, body string) string {
	prog := "/CIDInit /ProcSet findresource begin 12 dict begin begincmap /CIDSystemInfo << /Registry (Adobe) " +
		"/Ordering (GB1) /Supplement 2 >> def /CMapName /" + name + " def " + body + " 1 begincodespacerange <0000> " +
		"<FFFF> endcodespacerange 1 begincidrange <0000> <FFFF> 0 endcidrange endcmap CMapName currentdict /CMap " +
		"defineresource pop end end"
	return spStream("/Type /CMap /CMapName /"+name+" "+dict, prog)
}

// TestTheCompositeFontCMapClausesAgreeWithVeraPDF — P07.S01's five clauses on every shape measured on veraPDF
// 1.30.2 before the rules were written; the verdicts are veraPDF's, clause by clause, and a "-" is a clause with
// no subject in that document. The corpus holds the ordinary halves; these are the shapes it does not.
func TestTheCompositeFontCMapClausesAgreeWithVeraPDF(t *testing.T) {
	gb := func(sup string) string {
		return "/CIDSystemInfo << /Registry (Adobe) /Ordering (GB1) /Supplement " + sup + " >>"
	}
	sysInfo := "/CIDSystemInfo << /Registry (Adobe) /Ordering (GB1) /Supplement 2 >>"
	clauses := []string{"7.21.3.1 t1", "7.21.3.2 t1", "7.21.3.3 t1", "7.21.3.3 t2", "7.21.3.3 t3"}
	v := map[string]Verdict{"pass": Pass, "fail": Fail, "-": NotApplicable}
	for _, c := range []struct {
		name string
		objs map[int]string
		want [5]string
		why  string
	}{
		{"Identity-H over a mismatched collection", type0Doc("/Identity-H", "/CIDSystemInfo << /Registry (X) /Ordering (Y) /Supplement 9 >>", nil),
			[5]string{"pass", "pass", "pass", "-", "-"}, "Identity is exempt from 7.21.3.1 whatever the CIDFont says"},
		// **The ISO 32000-1 column of veraPDF's table, measured**: GB-EUC-H is Adobe-GB1-0 there and -5 in the
		// ISO 32000-2 column, so a CIDFont at supplement 2 separates them.
		{"GB-EUC-H over supplement 2", type0Doc("/GB-EUC-H", gb("2"), nil), [5]string{"fail", "pass", "pass", "-", "-"},
			"the CMap is Adobe-GB1-0 in PDF/UA-1's column, and 2 > 0"},
		{"GB-EUC-H over supplement 0", type0Doc("/GB-EUC-H", gb("0"), nil), [5]string{"pass", "pass", "pass", "-", "-"}, "0 <= 0"},
		{"UniGB-UCS2-H over supplement 4", type0Doc("/UniGB-UCS2-H", gb("4"), nil), [5]string{"pass", "pass", "pass", "-", "-"}, "4 <= 4"},
		{"UniGB-UCS2-H over supplement 3", type0Doc("/UniGB-UCS2-H", gb("3"), nil), [5]string{"pass", "pass", "pass", "-", "-"}, "3 <= 4"},
		{"an embedded CMap at the CIDFont's supplement", type0Doc("20 0 R", gb("2"), map[int]string{20: cmapStream(sysInfo, "Cust", "")}),
			[5]string{"pass", "pass", "pass", "pass", "-"}, "a stream CMap's collection is its /CIDSystemInfo"},
		{"an embedded CMap below the CIDFont's supplement", type0Doc("20 0 R", gb("3"), map[int]string{20: cmapStream(sysInfo, "Cust", "")}),
			[5]string{"fail", "pass", "pass", "pass", "-"}, "3 > 2"},
		{"a program declaring /WMode 1 under a dictionary that says nothing", type0Doc("20 0 R", gb("2"), map[int]string{20: cmapStream(sysInfo, "Cust", "/WMode 1 def")}),
			[5]string{"pass", "pass", "pass", "fail", "-"}, "the dictionary's absent /WMode is 0"},
		{"/WMode 1 in both", type0Doc("20 0 R", gb("2"), map[int]string{20: cmapStream("/WMode 1 "+sysInfo, "Cust", "/WMode 1 def")}),
			[5]string{"pass", "pass", "pass", "pass", "-"}, "they agree"},
		{"/WMode 1 only inside a string", type0Doc("20 0 R", gb("2"), map[int]string{20: cmapStream(sysInfo, "Cust", "(/WMode 1 def) pop")}),
			[5]string{"pass", "pass", "pass", "pass", "-"}, "a string's bytes are not a definition"},
		{"/WMode 1 before a procedure never closed", type0Doc("20 0 R", gb("2"), map[int]string{20: cmapStream(sysInfo, "Cust", "/WMode 1 def {")}),
			[5]string{"pass", "pass", "pass", "pass", "-"}, "veraPDF's parser throws and keeps its default 0 (the review, measured)"},
		{"/WMode 1 only inside a procedure", type0Doc("20 0 R", gb("2"), map[int]string{20: cmapStream(sysInfo, "Cust", "{ /WMode 1 def } pop")}),
			[5]string{"pass", "pass", "pass", "pass", "-"}, "a procedure body never runs (the slice review, measured)"},
		{"a CMap referencing a predefined one", type0Doc("20 0 R", gb("2"), map[int]string{20: cmapStream("/UseCMap /GB-EUC-H "+sysInfo, "Cust", "")}),
			[5]string{"pass", "pass", "pass", "pass", "pass"}, "GB-EUC-H is in Table 118"},
		{"an unknown named CMap", type0Doc("/Foo-H", gb("2"), nil), [5]string{"fail", "pass", "fail", "-", "-"},
			"neither predefined nor embedded, and a name outside the table has an empty collection"},
	} {
		t.Run(c.name, func(t *testing.T) {
			pdf := buildPDF(c.objs)
			for i, clause := range clauses {
				if got := verdictOf(t, pdf, clause); got.Verdict != v[c.want[i]] {
					t.Errorf("%s reports %v (%s), want %s — %s (measured on veraPDF)", clause, got.Verdict, got.Why, c.want[i], c.why)
				}
			}
		})
	}
}

// **Two shapes veraPDF cannot be asked about, pinned to the predicate instead** (P07.S01).
//
//   - A `/UseCMap` naming an UNKNOWN CMap: veraPDF 1.30.2 throws ("Cannot read field "cidMappings" because
//     "another" is null") and reports nothing, measured. The profile's predicates fail both 7.21.3.3 t1 and t3.
//   - A CIDFont with no `/CIDSystemInfo`: veraPDF FAILS 7.21.3.1 (measured), but pdfcpu's validator drops the whole
//     Type 0 font, so nib has nothing to read and refuses — never a Pass over a font it did not see.
func TestTheCMapShapesVeraPDFCannotAnswerFollowThePredicate(t *testing.T) {
	sysInfo := "/CIDSystemInfo << /Registry (Adobe) /Ordering (GB1) /Supplement 2 >>"
	unknown := buildPDF(type0Doc("20 0 R", sysInfo, map[int]string{20: cmapStream("/UseCMap /Foo-H "+sysInfo, "Cust", "")}))
	for _, clause := range []string{"7.21.3.3 t1", "7.21.3.3 t3"} {
		if got := verdictOf(t, unknown, clause); got.Verdict != Fail {
			t.Errorf("a /UseCMap naming an unknown CMap reports %v (%s) for %s, want Fail", got.Verdict, got.Why, clause)
		}
	}
	dropped := buildPDF(type0Doc("/GB-EUC-H", "", nil))
	if got := verdictOf(t, dropped, "7.21.3.1 t1"); got.Verdict != CannotCheck || !strings.Contains(got.Why, "does not resolve") {
		t.Errorf("a CIDFont with no CIDSystemInfo reports %v (%s), want CannotCheck because the font does not resolve — pdfcpu drops it", got.Verdict, got.Why)
	}
}

// **A CIDFontType2's program in either place veraPDF reads it** (P07.S01 and its review): `/FontFile2`, or a
// `/FontFile3` whose `/Subtype` is `/OpenType`. Both map-less shapes are failed by veraPDF, both mapped ones passed.
func TestACIDFontType2IsJudgedWhereverItsProgramIs(t *testing.T) {
	for _, c := range []struct {
		name    string
		withMap bool
		ff3     string
		want    Verdict
	}{
		{"FontFile2, no map", false, "", Fail},
		{"FontFile2, Identity map", true, "", Pass},
		{"FontFile3 /OpenType, no map", false, "OpenType", Fail},
		// TrueType bytes under a CFF subtype: veraPDF reads them AS CFF, fails, and PASSES the clause (measured). nib
		// parses no CFF, so it refuses — a real CFF program there is failed by veraPDF, a bad one passed.
		{"FontFile3 /CIDFontType0C, no map", false, "CIDFontType0C", CannotCheck},
	} {
		pdf := withCIDFontType2(t, c.withMap)
		if c.ff3 != "" {
			pdf = asFontFile3(t, pdf, c.ff3)
		}
		if got := verdictOf(t, pdf, "7.21.3.2 t1"); got.Verdict != c.want {
			t.Errorf("%s reports %v (%s), want %v (measured on veraPDF)", c.name, got.Verdict, got.Why, c.want)
		}
	} // A /FontFile2 no parser opens: veraPDF has no PARSED program, so containsFontFile is false and it PASSES
	// (measured); nib cannot tell unparseable-to-veraPDF from unparseable-to-nib, so it refuses.
	garbage := withCIDFontType2Program(t, []byte("this is not a TrueType program at all, only text"), false)
	if got := verdictOf(t, garbage, "7.21.3.2 t1"); got.Verdict != CannotCheck {
		t.Errorf("a map-less CIDFontType2 over an unparseable program reports %v (%s), want CannotCheck", got.Verdict, got.Why)
	}
}
