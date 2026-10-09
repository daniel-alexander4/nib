package uacheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// carriedFixture is one Type 0 font with no /ToUnicode entry for what it shows, and what 7.21.7 answers on it:
// veraPDF 1.30.2's verdicts for t1 and t2, or — `refused` — the words of the reason nib gives for not answering.
type carriedFixture struct {
	name    string
	pdf     []byte
	t1, t2  string
	refused string
}

// carriedFixtures are ADR-117's shapes: the UCS2 fallback under Identity for each ordering, the predefined CMaps'
// codespaces and CIDs, and the places nib still refuses.
func carriedFixtures() []carriedFixture {
	type0 := func(enc, more string) string {
		return "<< /Type /Font /Subtype /Type0 /BaseFont /SomeCID /Encoding " + enc + " /DescendantFonts [11 0 R] " + more + ">>"
	}
	over := func(ordering string, extra map[int]string) map[int]string {
		o := map[int]string{11: strings.Replace(adobeIdentity, "(Identity)", "("+ordering+")", 1), 12: cidDescriptor}
		for k, v := range extra {
			o[k] = v
		}
		return o
	}
	cmap := func(name, dict, body string) string {
		return spStream("/Type /CMap /CMapName /"+name+" /CIDSystemInfo << /Registry (Adobe) /Ordering (Japan1) /Supplement 2 >> "+dict,
			"/CIDInit /ProcSet findresource begin 12 dict begin begincmap /CIDSystemInfo << /Registry (Adobe) /Ordering (Japan1) /Supplement 2 >> def "+
				"/CMapName /"+name+" def "+body+" endcmap CMapName currentdict /CMap defineresource pop end end")
	}
	id := func(name, ordering, show, t1, t2, refused string) carriedFixture {
		return carriedFixture{name, glyphDoc(type0("/Identity-H", ""), show+" Tj", over(ordering, nil)), t1, t2, refused}
	}
	pre := func(name, enc, ordering, show, t1, t2, refused string) carriedFixture {
		return carriedFixture{name, glyphDoc(type0("/"+enc, ""), show+" Tj", over(ordering, nil)), t1, t2, refused}
	}
	return []carriedFixture{
		id("Identity-H over Japan1, a CID with text", "Japan1", "<0022>", "pass", "pass", ""),
		id("Identity-H over Japan1, CID 0", "Japan1", "<0000>", "pass", "pass", ""),
		id("Identity-H over Japan1, a CID past the collection", "Japan1", "<F000>", "fail", "pass", ""),
		id("Identity-H over Japan1, one CID with text and one past the collection", "Japan1", "<0022F000>", "fail", "pass", ""),
		id("Identity-H over Japan1, the last CID both versions map", "Japan1", "<5A0C>", "pass", "pass", ""),
		id("Identity-H over Japan1, a CID only nib's version maps", "Japan1", "<5A0D>", "fail", "pass", "differ"),
		id("Identity-H over Japan1, a CID between the two runs the versions part on", "Japan1", "<5A0F>", "pass", "pass", ""),
		id("Identity-H over Japan1, a CID only veraPDF's version maps", "Japan1", "<5A12>", "pass", "pass", "differ"),
		id("Identity-H over Japan1, a CID the versions give different text", "Japan1", "<0072>", "pass", "pass", ""),
		id("Identity-H over GB1, a CID with text", "GB1", "<0022>", "pass", "pass", ""),
		id("Identity-H over GB1, a CID past the collection", "GB1", "<F000>", "fail", "pass", ""),
		id("Identity-H over CNS1, a CID with text", "CNS1", "<0022>", "pass", "pass", ""),
		id("Identity-H over CNS1, a CID past the collection", "CNS1", "<F000>", "fail", "pass", ""),
		id("Identity-H over CNS1, a CID only veraPDF's version maps", "CNS1", "<4A90>", "pass", "pass", "differ"),
		id("Identity-H over CNS1, the CID before those", "CNS1", "<4A8F>", "pass", "pass", ""),
		id("Identity-H over Korea1, a CID with text", "Korea1", "<0022>", "pass", "pass", ""),
		id("Identity-H over Korea1, a CID past the collection", "Korea1", "<F000>", "fail", "pass", ""),
		id("Identity-H over Korea1, the CID the versions give different text", "Korea1", "<205A>", "pass", "pass", ""),
		id("Identity-H over KR", "KR", "<0022>", "pass", "pass", "Adobe-KR-UCS2"),
		id("Identity-H over Japan2, which has no UCS2 CMap", "Japan2", "<0022>", "fail", "pass", ""),
		{"Identity-V over Japan1", glyphDoc(type0("/Identity-V", ""), "<0022> Tj", over("Japan1", nil)), "pass", "pass", ""},
		{"Identity-H over Japan1, a /ToUnicode mapping one code of two", glyphDoc(type0("/Identity-H", "/ToUnicode 20 0 R "), "<00220023> Tj",
			over("Japan1", map[int]string{20: toUni("1 beginbfchar <0022> <0041> endbfchar")})), "pass", "pass", ""},
		{"Identity-H over Japan1, a /ToUnicode mapping one code and the other past the collection", glyphDoc(type0("/Identity-H", "/ToUnicode 20 0 R "), "<0022F000> Tj",
			over("Japan1", map[int]string{20: toUni("1 beginbfchar <0022> <0041> endbfchar")})), "fail", "pass", ""},

		pre("90ms-RKSJ-H, a two-byte code", "90ms-RKSJ-H", "Japan1", "<82A0>", "pass", "pass", ""),
		pre("90ms-RKSJ-H, a one-byte code", "90ms-RKSJ-H", "Japan1", "<41>", "pass", "pass", ""),
		pre("90ms-RKSJ-H, one- and two-byte codes in one string", "90ms-RKSJ-H", "Japan1", "<4182A0B1>", "pass", "pass", ""),
		pre("90ms-RKSJ-H, a code in the codespace with no CID", "90ms-RKSJ-H", "Japan1", "<FCFC>", "pass", "pass", ""),
		pre("90ms-RKSJ-H, a first byte no range admits", "90ms-RKSJ-H", "Japan1", "<FD41>", "pass", "pass", ""),
		pre("90ms-RKSJ-H, a second byte no range admits", "90ms-RKSJ-H", "Japan1", "<8130>", "pass", "pass", ""),
		pre("90ms-RKSJ-H, a string ending inside a code", "90ms-RKSJ-H", "Japan1", "<82>", "pass", "pass", ""),
		pre("90ms-RKSJ-H over a GB1 descendant", "90ms-RKSJ-H", "GB1", "<82A0>", "pass", "pass", ""),
		pre("90ms-RKSJ-H over an Identity descendant", "90ms-RKSJ-H", "Identity", "<82A0>", "pass", "pass", ""),
		pre("90ms-RKSJ-V, a code its own list maps", "90ms-RKSJ-V", "Japan1", "<8141>", "pass", "pass", ""),
		pre("90ms-RKSJ-V, a code the CMap it uses maps", "90ms-RKSJ-V", "Japan1", "<82A0>", "pass", "pass", ""),
		pre("90ms-RKSJ-V, a first byte no range admits", "90ms-RKSJ-V", "Japan1", "<FD41>", "-", "-", "reader throws"),
		pre("UniJIS-UCS2-H", "UniJIS-UCS2-H", "Japan1", "<3042>", "pass", "pass", ""),
		pre("UniJIS-UCS2-H, a code with no CID", "UniJIS-UCS2-H", "Japan1", "<FFFF>", "pass", "pass", ""),
		pre("UniJIS-UCS2-HW-V", "UniJIS-UCS2-HW-V", "Japan1", "<3042005C>", "pass", "pass", ""),
		pre("EUC-H", "EUC-H", "Japan1", "<A4A28EB1>", "pass", "pass", ""),
		pre("H", "H", "Japan1", "<2422>", "pass", "pass", ""),
		pre("V", "V", "Japan1", "<2422>", "pass", "pass", ""),
		pre("83pv-RKSJ-H", "83pv-RKSJ-H", "Japan1", "<82A0>", "pass", "pass", ""),
		pre("UniGB-UCS2-H", "UniGB-UCS2-H", "GB1", "<4E00>", "pass", "pass", ""),
		pre("GBK-EUC-H", "GBK-EUC-H", "GB1", "<B0A141>", "pass", "pass", ""),
		pre("GB-EUC-V", "GB-EUC-V", "GB1", "<B0A1>", "pass", "pass", ""),
		pre("GBpc-EUC-H", "GBpc-EUC-H", "GB1", "<B0A1>", "pass", "pass", ""),
		pre("UniCNS-UCS2-H", "UniCNS-UCS2-H", "CNS1", "<4E00>", "pass", "pass", ""),
		pre("ETen-B5-H", "ETen-B5-H", "CNS1", "<A440>", "pass", "pass", ""),
		pre("ETenms-B5-V", "ETenms-B5-V", "CNS1", "<A440>", "pass", "pass", ""),
		pre("B5pc-H", "B5pc-H", "CNS1", "<A440>", "pass", "pass", ""),
		pre("HKscs-B5-H", "HKscs-B5-H", "CNS1", "<A440>", "pass", "pass", ""),
		pre("CNS-EUC-H", "CNS-EUC-H", "CNS1", "<A4A1>", "pass", "pass", ""),
		pre("UniKS-UCS2-H", "UniKS-UCS2-H", "Korea1", "<AC00>", "pass", "pass", ""),
		pre("KSC-EUC-H", "KSC-EUC-H", "Korea1", "<B0A1>", "pass", "pass", ""),
		pre("KSCms-UHC-HW-V", "KSCms-UHC-HW-V", "Korea1", "<B0A1>", "pass", "pass", ""),
		pre("KSCpc-EUC-H", "KSCpc-EUC-H", "Korea1", "<B0A1>", "pass", "pass", ""),
		pre("UniJIS-UTF16-H, whose table nib does not carry", "UniJIS-UTF16-H", "Japan1", "<3042>", "pass", "pass", "carries no table"),
		pre("GBK2K-V, which uses a table nib does not carry", "GBK2K-V", "GB1", "<B0A1>", "pass", "pass", "carries no table"),

		{"an embedded CMap using 90ms-RKSJ-H in its program", glyphDoc(type0("21 0 R", ""), "<82A041> Tj",
			over("Japan1", map[int]string{21: cmap("Mine", "", "/90ms-RKSJ-H usecmap")})), "pass", "pass", ""},
		{"an embedded CMap using 90ms-RKSJ-H in its program, then its own range", glyphDoc(type0("21 0 R", ""), "<82A041> Tj",
			over("Japan1", map[int]string{21: cmap("Mine", "", "/90ms-RKSJ-H usecmap 1 begincodespacerange <00> <80> endcodespacerange 1 begincidrange <41> <41> 843 endcidrange")})), "pass", "pass", ""},
		{"an embedded CMap using 90ms-RKSJ-H in its dictionary", glyphDoc(type0("21 0 R", ""), "<82A041> Tj",
			over("Japan1", map[int]string{21: cmap("Mine", "/UseCMap /90ms-RKSJ-H", "1 begincodespacerange <00> <80> endcodespacerange 1 begincidrange <41> <41> 843 endcidrange")})), "pass", "pass", ""},
		{"an embedded CMap using UniJIS-UTF16-H in its program", glyphDoc(type0("21 0 R", ""), "<3042> Tj",
			over("Japan1", map[int]string{21: cmap("Mine", "", "/UniJIS-UTF16-H usecmap")})), "pass", "pass", "carries no table"},
		{"an embedded CMap of its own over Japan1, a CID with text", glyphDoc(type0("21 0 R", ""), "<41> Tj",
			over("Japan1", map[int]string{21: cmap("Mine", "", "1 begincodespacerange <00> <FF> endcodespacerange 2 begincidchar <41> 843 <42> 61000 endcidchar")})), "pass", "pass", ""},
		{"an embedded CMap of its own over Japan1, a CID past the collection", glyphDoc(type0("21 0 R", ""), "<42> Tj",
			over("Japan1", map[int]string{21: cmap("Mine", "", "1 begincodespacerange <00> <FF> endcodespacerange 2 begincidchar <41> 843 <42> 61000 endcidchar")})), "fail", "pass", ""},
		{"an embedded CMap of its own over Japan1, a code it does not map", glyphDoc(type0("21 0 R", ""), "<43> Tj",
			over("Japan1", map[int]string{21: cmap("Mine", "", "1 begincodespacerange <00> <FF> endcodespacerange 2 begincidchar <41> 843 <42> 61000 endcidchar")})), "pass", "pass", ""},
		{"an embedded CMap of its own over Japan1, a negative CID", glyphDoc(type0("21 0 R", ""), "<41> Tj",
			over("Japan1", map[int]string{21: cmap("Mine", "", "1 begincodespacerange <00> <FF> endcodespacerange 1 begincidchar <41> -5 endcidchar")})), "fail", "pass", "negative CID"},
	}
}

// TestAType0FontIsReadWithTheCMapsNibCarries — ADR-117, `/pending 676`. A Type 0 glyph with no /ToUnicode entry has
// the text Adobe's UCS2 CMap gives its CID, and a font whose /Encoding is a predefined CMap is cut and mapped by that
// CMap; nib refused both for want of the tables. Every row was measured on veraPDF 1.30.2 before it was pinned, and
// with veraPDF present every document is compared with it again on EVERY clause nib checks: a row that answers must
// agree strictly, and a row that refuses may refuse — it may never answer otherwise than veraPDF does.
//
// One row is a document veraPDF reports nothing on at all (its own reader throws cutting the string), which is a
// refusal here too.
func TestAType0FontIsReadWithTheCMapsNibCarries(t *testing.T) {
	fx := carriedFixtures()
	pinned := map[string]Verdict{"pass": Pass, "fail": Fail}
	var docs [][]byte
	for _, f := range fx {
		docs = append(docs, f.pdf)
		for _, c := range []struct{ clause, want string }{{"7.21.7 t1", f.t1}, {"7.21.7 t2", f.t2}} {
			got := verdictOf(t, f.pdf, c.clause)
			switch want, answers := pinned[c.want]; {
			case f.refused != "":
				if got.Verdict != CannotCheck || !strings.Contains(got.Why, f.refused) {
					t.Errorf("%s: %s reports %v (%s), want CannotCheck naming %q (veraPDF %s)", f.name, c.clause, got.Verdict, got.Why, f.refused, c.want)
				}
			case !answers:
				t.Fatalf("%s: %s's measured verdict %q is neither pass nor fail", f.name, c.clause, c.want)
			case got.Verdict != want:
				t.Errorf("%s: %s reports %v (%s), veraPDF %s", f.name, c.clause, got.Verdict, got.Why, c.want)
			}
		}
	}
	vera := veraAsk(t, docs)
	if vera == nil {
		return
	}
	words := map[Verdict]string{Pass: "passed", Fail: "failed", NotApplicable: "none"}
	table := map[string]string{"pass": "passed", "fail": "failed"}
	for i, f := range fx {
		if vera[i] == nil {
			if f.t1 != "-" {
				t.Errorf("%s: veraPDF reports nothing on the document, and the table says %s/%s", f.name, f.t1, f.t2)
			}
			continue
		}
		if vera[i]["7.21.7 t1"] != table[f.t1] || vera[i]["7.21.7 t2"] != table[f.t2] {
			t.Errorf("%s: veraPDF answers 7.21.7 %s/%s, and the table says %s/%s", f.name, vera[i]["7.21.7 t1"], vera[i]["7.21.7 t2"], f.t1, f.t2)
		}
		for _, clause := range Clauses() {
			got := verdictOf(t, f.pdf, clause)
			if got.Verdict == CannotCheck && f.refused != "" {
				continue
			}
			if words[got.Verdict] != vera[i][clause] {
				t.Errorf("%s: %s — nib reports %v (%s), veraPDF %q", f.name, clause, got.Verdict, got.Why, vera[i][clause])
			}
		}
	}
}

// TestTheCorpusFontOverAdobeJapan1IsAnswered: veraPDF's own 7.21.7 pass file draws KozMinPro over Identity-H and
// Adobe-Japan1 with no /ToUnicode — the document `/pending 676` was filed on. Both tests answer, and pass.
func TestTheCorpusFontOverAdobeJapan1IsAnswered(t *testing.T) {
	home, _ := os.UserHomeDir()
	pdf, err := os.ReadFile(filepath.Join(home, "nib", "verapdfs", "PDF_UA-1", "7.21 Fonts", "7.21.7 Unicode character maps", "7.21.7-t01-pass-a.pdf"))
	if err != nil {
		t.Skip("veraPDF's corpus is not at ~/nib/verapdfs")
	}
	for _, clause := range []string{"7.21.7 t1", "7.21.7 t2"} {
		if got := verdictOf(t, pdf, clause); got.Verdict != Pass {
			t.Errorf("%s reports %v (%s), want Pass", clause, got.Verdict, got.Why)
		}
	}
}
