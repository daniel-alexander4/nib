package uacheck

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// P07.S04b's fixtures: every shape of 7.21.5 t1, 7.21.4.1 t2 and 7.21.8 t1 over a Type 3 font and over a CIDFontType2
// font under an embedded CMap, each run on veraPDF 1.30.2 before the reader was written. The corpus holds no fail file
// for either; the oracle asks veraPDF again on every run.

// measuredFixture is one measured document and veraPDF's verdicts over metricClauses ('P' pass, 'F' fail); `refused` is
// the words nib's CannotCheck carries where it deliberately does not answer.
type measuredFixture struct {
	name, vera, refused string
	pdf                 []byte
}

// t3Font is a Type 3 font drawing codes 97 and 98 by /Differences `diffs`, with `widths` and `charProcs` and `extra`.
func t3Font(diffs, widths, charProcs, extra string) string {
	return "<< /Type /Font /Subtype /Type3 /FontBBox [0 0 10 10] /FontMatrix [0.001 0 0 0.001 0 0] " + charProcs +
		" /Encoding << /Type /Encoding /Differences [" + diffs + "] >> " + widths + " /Resources << >> " + extra + " >>"
}

func t3Doc(diffs, widths, charProcs, extra, show string, objs map[int]string) []byte {
	return glyphDoc(t3Font(diffs, widths, charProcs, extra), show, objs)
}

func proc(body string) string { return spStream("", body+" 0 0 10 10 re f") }

// cmapStream is an embedded CMap over a two-byte codespace holding `body`, its dictionary carrying `dict`.
func cidCMap(dict, body string) string {
	return spStream("/Type /CMap /CMapName /Probe /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> "+dict,
		"/CIDInit /ProcSet findresource begin 12 dict begin begincmap /CMapName /Probe def "+
			"1 begincodespacerange <0000> <FFFF> endcodespacerange "+body+" endcmap CMapName currentdict /CMap defineresource pop end end")
}

// cidCMapDoc is cidFontDoc over the embedded CMap object 21.
func cidCMapDoc(cmap, cid, content string, prog []byte, extra map[int]string) []byte {
	objs := map[int]string{
		11: "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /Probe /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> " +
			"/FontDescriptor 12 0 R " + cid + " >>",
		21: cmap,
	}
	for k, v := range ttObjects("/Flags 4", "FontFile2", prog, "") {
		objs[k] = v
	}
	for k, v := range extra {
		objs[k] = v
	}
	return glyphPage(content, "", "", "<< /Type /Font /Subtype /Type0 /BaseFont /Probe /Encoding 21 0 R /DescendantFonts [11 0 R] >>", objs)
}

func type3AndCMapFixtures() []measuredFixture {
	std := ttProgram(sub31, sub10)
	ab := "97 /a /b"
	w := "/FirstChar 97 /LastChar 98 /Widths [10 10]"
	cp := "/CharProcs << /a 26 0 R /b 27 0 R >>"
	good := map[int]string{26: proc("10 0 d0"), 27: proc("10 0 d0")}
	one := func(body string) map[int]string { return map[int]string{26: proc(body), 27: proc("10 0 d0")} }
	show := func(s string) string { return "BT /F0 12 Tf 10 10 Td " + s + " ET" }
	c21 := show("<0021> Tj")
	// /W: CID 5 is 500 (the program's width), CID 33 (0x21) 900 — so reading 0x21 as its own CID fails 7.21.5.
	wBy := "/DW 900 /W [5 [500]] /CIDToGIDMap /Identity"
	desc := func(extra string) string {
		return "<< /Type /FontDescriptor /FontName /T3 /Flags 32 /FontBBox [0 0 10 10] /ItalicAngle 0 /Ascent 10 /Descent 0 /CapHeight 10 /StemV 1 " + extra + " >>"
	}
	return []measuredFixture{
		// ── Type 3.
		{name: "T3: d0 agrees", vera: "PPP", pdf: t3Doc(ab, w, cp, "", "(ab) Tj", good)},
		{name: "T3: d0 5 off", vera: "FPP", pdf: t3Doc(ab, w, cp, "", "(a) Tj", one("15 0 d0"))},
		{name: "T3: d0 1 off", vera: "PPP", pdf: t3Doc(ab, w, cp, "", "(a) Tj", one("11 0 d0"))},
		{name: "T3: d0 1.5 off", vera: "FPP", pdf: t3Doc(ab, w, cp, "", "(a) Tj", one("11.5 0 d0"))},
		{name: "T3: d1 agrees", vera: "PPP", pdf: t3Doc(ab, w, cp, "", "(a) Tj", one("10 0 0 0 10 10 d1"))},
		{name: "T3: d1 off", vera: "FPP", pdf: t3Doc(ab, w, cp, "", "(a) Tj", one("15 0 0 0 10 10 d1"))},
		{name: "T3: d1 short a number", vera: "FPP", pdf: t3Doc(ab, w, cp, "", "(a) Tj", one("10 0 0 0 10 d1"))},
		{name: "T3: d1 short, /MissingWidth 10", vera: "PPP", pdf: t3Doc(ab, w, cp, "/FontDescriptor 12 0 R", "(a) Tj", mergeObjs(one("10 0 0 0 10 d1"), map[int]string{12: desc("/MissingWidth 10")}))},
		{name: "T3: wx a name", vera: "FPP", pdf: t3Doc(ab, w, cp, "", "(a) Tj", one("/x 0 d0"))},
		{name: "T3: wx -5 against /Widths -5", vera: "PPP", pdf: t3Doc(ab, "/FirstChar 97 /LastChar 98 /Widths [-5 10]", cp, "", "(a) Tj", one("-5 0 d0"))},
		{name: "T3: wx -1 against /Widths -1", vera: "PPP", pdf: t3Doc(ab, "/FirstChar 97 /LastChar 98 /Widths [-1 10]", cp, "", "(a) Tj", one("-1 0 d0"))},
		{name: "T3: wx -1 against /Widths -1, /MissingWidth 7", vera: "FPP", pdf: t3Doc(ab, "/FirstChar 97 /LastChar 98 /Widths [-1 10]", cp, "/FontDescriptor 12 0 R", "(a) Tj", mergeObjs(one("-1 0 d0"), map[int]string{12: desc("/MissingWidth 7")}))},
		{name: "T3: a code /Differences does not name", vera: "FFP", pdf: t3Doc("97 /a", w, cp, "", "(b) Tj", good)},
		{name: "T3: a name /CharProcs lacks", vera: "FFP", pdf: t3Doc(ab, w, "/CharProcs << /a 26 0 R >>", "", "(b) Tj", good)},
		{name: "T3: a /CharProcs entry that is null", vera: "FFP", pdf: t3Doc(ab, w, "/CharProcs << /a 26 0 R /b null >>", "", "(b) Tj", good)},
		{name: "T3: no /CharProcs", vera: "FFP", pdf: t3Doc(ab, w, "", "", "(a) Tj", good)},
		{name: "T3: code 0 named, its procedure present", vera: "PPP", pdf: t3Doc("0 /a", "/FirstChar 0 /LastChar 0 /Widths [10]", cp, "", "<00> Tj", good)},
		{name: "T3: code 0 unnamed", vera: "PFP", pdf: t3Doc(ab, "/FirstChar 0 /LastChar 0 /Widths [0]", cp, "", "<00> Tj", good)},
		{name: "T3: a width off, drawn invisibly", vera: "PPP", pdf: glyphPage("BT /F0 12 Tf 10 10 Td 3 Tr (a) Tj ET", "", "", t3Font(ab, w, cp, ""), one("15 0 d0"))},
		{name: "T3: .notdef with a procedure", vera: "PPF", pdf: t3Doc("97 /.notdef", w, "/CharProcs << /.notdef 26 0 R >>", "", "(a) Tj", good)},
		{name: "T3: a FontMatrix of 0.01, widths 50", vera: "PPP", pdf: glyphDoc(`<< /Type /Font /Subtype /Type3 /FontBBox [0 0 10 10] /FontMatrix [0.01 0 0 0.01 0 0] `+cp+
			` /Encoding << /Type /Encoding /Differences [97 /a /b] >> /FirstChar 97 /LastChar 98 /Widths [50 50] /Resources << >> >>`, "(a) Tj", one("50 0 d0"))},
		{name: "T3: a comment before the width", vera: "PPP", pdf: t3Doc(ab, w, cp, "", "(a) Tj", one("% c\n10 0 d0"))},
		{name: "T3: wx 10.5", vera: "PPP", pdf: t3Doc(ab, w, cp, "", "(a) Tj", one("10.5 0 d0"))},
		{name: "T3: wx a hex string", vera: "FPP", pdf: t3Doc(ab, w, cp, "", "(a) Tj", one("<01> 0 d0"))},
		{name: "T3: d0 in d1's place", vera: "FPP", pdf: t3Doc(ab, w, cp, "", "(a) Tj", one("10 0 0 0 10 10 d0"))},
		{name: "T3: a string as wy", vera: "PPP", pdf: t3Doc(ab, w, cp, "", "(a) Tj", one("10 (a b) d0"))},
		{name: "T3: 0d0 run together", vera: "PPP", pdf: t3Doc(ab, w, cp, "", "(a) Tj", one("10 0d0"))},
		{name: "T3: +10", vera: "PPP", pdf: t3Doc(ab, w, cp, "", "(a) Tj", one("+10 0 d0"))},
		{name: "T3: 1e1", vera: "FPP", pdf: t3Doc(ab, w, cp, "", "(a) Tj", one("1e1 0 d0"))},
		{name: "T3: a lone dot", vera: "FPP", pdf: t3Doc(ab, w, cp, "", "(a) Tj", one(". 0 d0"))},
		{name: "T3: an empty procedure", vera: "PPP", pdf: t3Doc(ab, "/FirstChar 97 /LastChar 98 /Widths [0 10]", cp, "", "(a) Tj", map[int]string{26: spStream("", ""), 27: proc("10 0 d0")})},
		{name: "T3: outside /FirstChar-/LastChar, /MissingWidth 10", vera: "PPP", pdf: t3Doc("97 /a 99 /c", w, "/CharProcs << /a 26 0 R /c 27 0 R >>", "/FontDescriptor 12 0 R", "(c) Tj", mergeObjs(good, map[int]string{12: desc("/MissingWidth 10")}))},
		{name: "T3: 10. with a trailing dot", vera: "PPP", pdf: t3Doc(ab, w, cp, "", "(a) Tj", one("10. 0 d0"))},
		{name: "T3: -.5 against /Widths 0", vera: "PPP", pdf: t3Doc(ab, "/FirstChar 97 /LastChar 98 /Widths [0 10]", cp, "", "(a) Tj", one("-.5 0 d0"))},
		{name: "T3: a name as wy", vera: "PPP", pdf: t3Doc(ab, w, cp, "", "(a) Tj", one("10 /y d0"))},
		{name: "T3: an array as wy", vera: "FPP", pdf: t3Doc(ab, w, cp, "", "(a) Tj", one("10 [1 2] d0"))},
		{name: "T3: a dictionary as wy", vera: "FPP", pdf: t3Doc(ab, w, cp, "", "(a) Tj", one("10 <<>> d0"))},
		{name: "T3: /StandardEncoding as the base", vera: "FFP", pdf: glyphDoc("<< /Type /Font /Subtype /Type3 /FontBBox [0 0 10 10] /FontMatrix [0.001 0 0 0.001 0 0] "+cp+" /Encoding << /Type /Encoding /BaseEncoding /StandardEncoding >> "+w+" /Resources << >> >>", "(a) Tj", good)},
		{name: "T3: a lone > as wy", vera: "FPP", pdf: t3Doc(ab, w, cp, "", "(a) Tj", one("10 > d0"))},
		{name: "T3: an integer past a long", vera: "FPP", pdf: t3Doc(ab, w, cp, "", "(a) Tj", one("99999999999999999999 0 d0"))},
		{name: "T3: a real past a double", vera: "FPP", pdf: t3Doc(ab, w, cp, "", "(a) Tj", one("1"+strings.Repeat("0", 400)+".0 0 d0"))},
		// The review's shapes, measured before the fix (P07.S04b's review round).
		{name: "T3: /CharProcs a stream", vera: "PPP", pdf: t3Doc(ab, w, "/CharProcs 41 0 R", "", "(a) Tj", mergeObjs(good, map[int]string{41: spStream("/a 26 0 R /b 27 0 R", "")}))},
		{name: "T3: an ASCII85 string first", vera: "FPP", pdf: t3Doc(ab, w, cp, "", "(a) Tj", map[int]string{26: proc("<~87cURD]i,\"Ebo80~> 10 0 d0"), 27: proc("10 0 d0")})},
		{name: "T3: an ASCII85 string as wy", vera: "PPP", pdf: t3Doc(ab, w, cp, "", "(a) Tj", map[int]string{26: proc("10 <~87cURD]i~> d0"), 27: proc("10 0 d0")})},
		{name: "T3: /CharProcs a missing object reads as absent", vera: "FFP", pdf: t3Doc(ab, w, "/CharProcs 45 0 R", "", "(a) Tj", good)},
		// The red-proof's survivors, each a shape no fixture held (measured).
		{name: "T3: a keyword other than d0 as the third token", vera: "FPP", pdf: t3Doc(ab, w, cp, "", "(a) Tj", map[int]string{26: spStream("", "10 0 q 0 0 10 10 re f Q"), 27: proc("10 0 d0")})},
		{name: "T3: a lone dot against /Widths 0", vera: "FPP", pdf: t3Doc(ab, "/FirstChar 97 /LastChar 98 /Widths [0 10]", cp, "", "(a) Tj", one(". 0 d0"))},
		{name: "T3: 1.2.3 against /Widths 0", vera: "FPP", pdf: t3Doc(ab, "/FirstChar 97 /LastChar 98 /Widths [0 10]", cp, "", "(a) Tj", one("1.2.3 0 d0"))},
		{name: "T3: a#2Eb in both places", vera: "PPP", pdf: t3Doc("97 /a#2Eb", w, "/CharProcs << /a#2Eb 26 0 R >>", "", "(a) Tj", good)},
		{name: "T3: a.b in /Differences, a#2Eb in /CharProcs", vera: "PPP", pdf: t3Doc("97 /a.b", w, "/CharProcs << /a#2Eb 26 0 R >>", "", "(a) Tj", good)},
		// ── CIDFontType2 over an embedded CMap.
		{name: "CMap: a cidrange as identity", vera: "PPP", pdf: cidCMapDoc(cidCMap("", "1 begincidrange <0000> <00FF> 0 endcidrange"), "/DW 500 /CIDToGIDMap /Identity", c21, std, nil)},
		{name: "CMap: 0x21 to CID 5", vera: "PPP", pdf: cidCMapDoc(cidCMap("", "1 begincidchar <0021> 5 endcidchar"), wBy, c21, std, nil)},
		{name: "CMap: 0x21 by range to CID 5", vera: "PPP", pdf: cidCMapDoc(cidCMap("", "1 begincidrange <0020> <0030> 4 endcidrange"), wBy, c21, std, nil)},
		{name: "CMap: a code no mapping holds", vera: "PFF", pdf: cidCMapDoc(cidCMap("", "1 begincidchar <0022> 5 endcidchar"), "/DW 500 /CIDToGIDMap /Identity", c21, std, nil)},
		{name: "CMap: a notdefrange holds the code", vera: "PPP", pdf: cidCMapDoc(cidCMap("", "1 begincidchar <0022> 5 endcidchar 1 beginnotdefrange <0020> <0030> 5 endnotdefrange"), "/DW 500 /CIDToGIDMap /Identity", c21, std, nil)},
		{name: "CMap: a notdefchar to CID 5", vera: "PPP", pdf: cidCMapDoc(cidCMap("", "1 beginnotdefchar <0021> 5 endnotdefchar"), wBy, c21, std, nil)},
		{name: "CMap: a cidchar after a cidrange", vera: "PPP", pdf: cidCMapDoc(cidCMap("", "1 begincidrange <0000> <00FF> 0 endcidrange 1 begincidchar <0021> 5 endcidchar"), wBy, c21, std, nil)},
		{name: "CMap: a cidrange after a cidchar", vera: "FPP", pdf: cidCMapDoc(cidCMap("", "1 begincidchar <0021> 5 endcidchar 1 begincidrange <0000> <00FF> 0 endcidrange"), wBy, c21, std, nil)},
		{name: "CMap: a cidrange after a cidchar, same list", vera: "FPP", pdf: cidCMapDoc(cidCMap("", "2 begincidrange <0021> <0021> 5 <0000> <00FF> 0 endcidrange"), wBy, c21, std, nil)},
		{name: "CMap: a notdefchar before a cidchar", vera: "PPP", pdf: cidCMapDoc(cidCMap("", "1 beginnotdefchar <0021> 33 endnotdefchar 1 begincidchar <0021> 5 endcidchar"), wBy, c21, std, nil)},
		{name: "CMap: two notdefchars", vera: "PPP", pdf: cidCMapDoc(cidCMap("", "2 beginnotdefchar <0021> 5 <0021> 33 endnotdefchar"), wBy, c21, std, nil)},
		{name: "CMap: usecmap Identity-H, then its own cidchar", vera: "PPP", pdf: cidCMapDoc(cidCMap("", "/Identity-H usecmap 1 begincidchar <0021> 5 endcidchar"), wBy, c21, std, nil)},
		{name: "CMap: usecmap Identity-H, the code not its own", vera: "FPP", pdf: cidCMapDoc(cidCMap("", "/Identity-H usecmap 1 begincidchar <0022> 5 endcidchar"), wBy, c21, std, nil)},
		{name: "CMap: its own cidchar, then usecmap Identity-H", vera: "PPP", pdf: cidCMapDoc(cidCMap("", "1 begincidchar <0021> 5 endcidchar /Identity-H usecmap"), wBy, c21, std, nil)},
		{name: "CMap: /UseCMap a stream holding the code", vera: "PPP", pdf: cidCMapDoc(cidCMap("/UseCMap 22 0 R", "1 begincidchar <0022> 7 endcidchar"), wBy, c21, std, map[int]string{22: cidCMap("", "1 begincidchar <0021> 5 endcidchar")})},
		{name: "CMap: /UseCMap a stream beside its own", vera: "PPP", pdf: cidCMapDoc(cidCMap("/UseCMap 22 0 R", "1 begincidchar <0021> 5 endcidchar"), wBy, c21, std, map[int]string{22: cidCMap("", "1 begincidchar <0021> 33 endcidchar")})},
		{name: "CMap: /UseCMap /Identity-H", vera: "FPP", pdf: cidCMapDoc(cidCMap("/UseCMap /Identity-H", "1 begincidchar <0022> 7 endcidchar"), wBy, c21, std, nil)},
		{name: "CMap: a CID past the CIDToGIDMap", vera: "PFF", pdf: cidCMapDoc(cidCMap("", "1 begincidchar <0021> 9 endcidchar"), "/DW 500 /CIDToGIDMap 30 0 R", c21, std, map[int]string{30: spStream("", string([]byte{0, 0, 0, 5, 0, 7}))})},
		{name: "CMap: a CID one past the CIDToGIDMap", vera: "PFF", pdf: cidCMapDoc(cidCMap("", "1 begincidchar <0021> 3 endcidchar"), "/DW 500 /CIDToGIDMap 30 0 R", c21, std, map[int]string{30: spStream("", string([]byte{0, 0, 0, 5, 0, 7}))})},
		{name: "CMap: a CID within the CIDToGIDMap", vera: "PPP", pdf: cidCMapDoc(cidCMap("", "1 begincidchar <0021> 2 endcidchar"), "/DW 500 /CIDToGIDMap 30 0 R", c21, std, map[int]string{30: spStream("", string([]byte{0, 0, 0, 5, 0, 7}))})},
		{name: "CMap: a CID past the glyph count", vera: "PFF", pdf: cidCMapDoc(cidCMap("", "1 begincidchar <0021> 150 endcidchar"), "/DW 500 /CIDToGIDMap /Identity", c21, std, nil)},
		{name: "CMap: a code mapped to CID 0", vera: "PFF", pdf: cidCMapDoc(cidCMap("", "1 begincidchar <0021> 0 endcidchar"), "/DW 500 /CIDToGIDMap /Identity", c21, std, nil)},
		{name: "CMap: a real CID", vera: "FPF", refused: "could not cut this string", pdf: cidCMapDoc(cidCMap("", "1 begincidchar <0021> 5.0 endcidchar"), wBy, c21, std, nil)},
		{name: "CMap: a cidchar to CID -1 over a cidrange", vera: "PPP", pdf: cidCMapDoc(cidCMap("", "1 begincidrange <0020> <0030> 4 endcidrange 1 begincidchar <0021> -1 endcidchar"), wBy, c21, std, nil)},
		{name: "CMap: no program", vera: "PPP", pdf: cidCMapDoc(cidCMap("", "1 begincidchar <0021> 5 endcidchar"), wBy, c21, nil, nil)},
		{name: "CMap: code 0 mapped to CID 5", vera: "PPP", pdf: cidCMapDoc(cidCMap("", "1 begincidchar <0000> 5 endcidchar"), wBy, show("<0000> Tj"), std, nil)},
		{name: "CMap: /Identity#2DH usecmap loads nothing", vera: "FFF", pdf: cidCMapDoc(cidCMap("", "/Identity#2DH usecmap 1 begincidchar <0022> 5 endcidchar"), wBy, c21, std, nil)},
		{name: "CMap: /Identity-H [ /X ] usecmap loads Identity-H", vera: "FPP", pdf: cidCMapDoc(cidCMap("", "/Identity-H [ /X ] usecmap 1 begincidchar <0022> 5 endcidchar"), wBy, c21, std, nil)},
		{name: "CMap: /Identity-H << >> usecmap loads nothing", vera: "FFF", pdf: cidCMapDoc(cidCMap("", "/Identity-H << >> usecmap 1 begincidchar <0022> 5 endcidchar"), wBy, c21, std, nil)},
		{name: "CMap: /Identity-V usecmap", vera: "FPP", pdf: cidCMapDoc(cidCMap("", "/Identity-V usecmap 1 begincidchar <0022> 5 endcidchar"), wBy, c21, std, nil)},
		{name: "CMap: /Other usecmap loads nothing", vera: "PFF", pdf: cidCMapDoc(cidCMap("", "/Other usecmap 1 begincidchar <0022> 5 endcidchar"), "/DW 500 /CIDToGIDMap /Identity", c21, std, nil)},
		{name: "CMap: a CID +5", vera: "PPP", pdf: cidCMapDoc(cidCMap("", "1 begincidchar <0021> +5 endcidchar"), wBy, c21, std, nil)},
		{name: "CMap: a CID past a long is the sentinel", vera: "PPP", pdf: cidCMapDoc(cidCMap("", "1 begincidrange <0020> <0030> 4 endcidrange 1 begincidchar <0021> 99999999999999999999 endcidchar"), wBy, c21, std, nil)},
		{name: "CMap: /Identity-H dup usecmap loads nothing", vera: "FFF", pdf: cidCMapDoc(cidCMap("", "/Identity-H dup usecmap 1 begincidchar <0022> 5 endcidchar"), wBy, c21, std, nil)},
		{name: "CMap: /Identity-H { /X } usecmap loads Identity-H", vera: "FPP", pdf: cidCMapDoc(cidCMap("", "/Identity-H { /X } usecmap 1 begincidchar <0022> 5 endcidchar"), wBy, c21, std, nil)},
		{name: "CMap: a bare minus is CID 1", vera: "PPP", pdf: cidCMapDoc(cidCMap("", "1 begincidchar <0021> - endcidchar"), "/DW 900 /W [1 [500]] /CIDToGIDMap /Identity", c21, std, nil)},
		{name: "CMap: a list count past a long opens nothing", vera: "FFF", pdf: cidCMapDoc(cidCMap("", "99999999999999999999 begincidchar 1 begincidchar <0021> 5 endcidchar"), wBy, c21, std, nil)},
		{name: "CMap: a one-byte codespace", vera: "PPP", pdf: cidCMapDoc(spStream("/Type /CMap /CMapName /P1 /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >>",
			"begincmap 1 begincodespacerange <00> <FF> endcodespacerange 1 begincidrange <20> <7E> 1 endcidrange endcmap"), "/DW 900 /W [34 [500]] /CIDToGIDMap /Identity", show("(A) Tj"), std, nil)},
		// The P07 phase-close review, R3-7: bytes no range admits under a CMap whose shortest range is ONE byte are each
		// skipped as code 0. nib merged the chain into an empty codespace, lost that length, and refused the string.
		{name: "CMap: bytes no range admits read as code 0, CID 0", vera: "PPF", pdf: cidCMapDoc(spStream("/Type /CMap /CMapName /P2 /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >>",
			"begincmap 2 begincodespacerange <00> <3F> <8000> <FFFF> endcodespacerange 1 begincidrange <00> <3F> 0 endcidrange endcmap"), "/DW 500 /CIDToGIDMap /Identity", show("<5041> Tj"), std, nil)},
		{name: "CMap: bytes no range admits read as code 0, CID 5", vera: "PPP", pdf: cidCMapDoc(spStream("/Type /CMap /CMapName /P2 /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >>",
			"begincmap 2 begincodespacerange <00> <3F> <8000> <FFFF> endcodespacerange 1 begincidrange <00> <3F> 5 endcidrange endcmap"), "/DW 900 /W [5 [500]] /CIDToGIDMap /Identity", show("<5041> Tj"), std, nil)},
	}
}

// TestTheType3AndCMapClausesAgreeWithVeraPDF — every shape above against veraPDF's measured verdicts.
func TestTheType3AndCMapClausesAgreeWithVeraPDF(t *testing.T) {
	want := map[byte]Verdict{'P': Pass, 'F': Fail}
	for _, f := range type3AndCMapFixtures() {
		for j, clause := range metricClauses {
			got := verdictOf(t, f.pdf, clause)
			if f.refused != "" {
				if got.Verdict != CannotCheck || !strings.Contains(got.Why, f.refused) {
					t.Errorf("%s: %s reports %v (%s), want CannotCheck naming %q", f.name, clause, got.Verdict, got.Why, f.refused)
				}
				continue
			}
			if w := want[f.vera[j]]; got.Verdict != w {
				t.Errorf("%s: %s reports %v (%s), veraPDF %c", f.name, clause, got.Verdict, got.Why, f.vera[j])
			}
		}
	}
}

// TestCIDLookupsPastTheBudgetAreCannotCheck — a CMap's lookups are charged to the document's CMap budget, once per
// distinct code: past it the answer is a refusal naming the ceiling, and a code already asked costs nothing more.
func TestCIDLookupsPastTheBudgetAreCannotCheck(t *testing.T) {
	d, err := open(cidCMapDoc(cidCMap("", "2 begincidchar <0022> 5 <0023> 6 endcidchar"), "/DW 500 /CIDToGIDMap /Identity",
		"BT /F0 12 Tf 10 10 Td <0021> Tj ET", ttProgram(sub31, sub10), nil))
	if err != nil {
		t.Fatal(err)
	}
	fonts, why := d.usedFonts()
	if why != "" || len(fonts) == 0 {
		t.Fatalf("no used font (%s)", why)
	}
	c, known, why := d.cidFontOf(fonts[0].dict)
	if !known || c == nil {
		t.Fatalf("no CIDFontType2 reading: %s", why)
	}
	d.cidAsks = maxCIDAsks - 2
	if _, why := d.toCID(c, 0x22); why != "" {
		t.Fatalf("a lookup asking two mappings refused with two reads left: %s", why)
	}
	if _, why := d.toCID(c, 0x22); why != "" || d.cidAsks != maxCIDAsks {
		t.Fatalf("asking the same code again cost %d reads (%s), want none", d.cidAsks-maxCIDAsks, why)
	}
	if _, why := d.toCID(c, 0x21); !strings.Contains(why, "cost more than") {
		t.Fatalf("a lookup past the budget answered %q, want a refusal naming the ceiling", why)
	}
}

// noReportFixtures are P07.S04b's documents veraPDF 1.30.2 answers with NO validation report, because building a glyph
// throws an exception its validator does not handle — each measured, and asked again below wherever veraPDF is present.
// veraPDF builds a glyph for invisible text too, and a verdict on an EARLIER glyph does not stop the throw (the review
// found nib failing 7.21.5 t1 on the first glyph and never reaching the second).
func noReportFixtures() []struct {
	name string
	pdf  []byte
} {
	std := ttProgram(sub31, sub10)
	t3 := func(charProcs, content string, objs map[int]string) []byte {
		return glyphPage("BT /F0 12 Tf 10 10 Td "+content+" ET", "", "", t3Font("97 /a", "/FirstChar 97 /LastChar 97 /Widths [10]", charProcs, ""),
			mergeObjs(map[int]string{26: proc("10 0 d0")}, objs))
	}
	neg := func(cidToGID, content string, extra map[int]string) []byte {
		return cidCMapDoc(cidCMap("", "1 begincidchar <0021> -5 endcidchar"), "/DW 500 /CIDToGIDMap "+cidToGID, "BT /F0 12 Tf 10 10 Td "+content+" ET", std, extra)
	}
	failFirst := func(f1 map[int]string, show string) []byte {
		return glyphPage("BT /F0 12 Tf 10 10 Td (A) Tj /F1 12 Tf "+show+" Tj ET", "/F1 50 0 R", "",
			mfDict("/FirstChar 65 /LastChar 65 /Widths [900]", "/Encoding /WinAnsiEncoding"), mergeObjs(ttObjects("/Flags 32", "FontFile2", std, ""), f1))
	}
	out := []struct {
		name string
		pdf  []byte
	}{
		{"a Type 3 font whose /CharProcs is an array", t3("/CharProcs [26 0 R]", "(a) Tj", nil)},
		{"a Type 3 font whose /CharProcs is an array, drawn invisibly", t3("/CharProcs [26 0 R]", "3 Tr (a) Tj", nil)},
		{"a Type 3 font whose /CharProcs is an indirect null", t3("/CharProcs 40 0 R", "(a) Tj", map[int]string{40: "null"})},
		{"a code mapped to a negative CID", neg("/Identity", "<0021> Tj", nil)},
		{"a code mapped to a negative CID over a CIDToGIDMap stream", neg("30 0 R", "<0021> Tj", map[int]string{30: spStream("", string([]byte{0, 0, 0, 5, 0, 7}))})},
		{"a code mapped to a negative CID, drawn invisibly", neg("/Identity", "3 Tr <0021> Tj", nil)},
		// P07.S05a: a CFF naming the ISOAdobe charset (229 names) over 230 glyphs — `initializeCharSet` indexes past it.
		{"a Type1C program naming the ISOAdobe charset past its 229 glyphs", t1cDoc("ABCDEF+Probe", "/FirstChar 65 /LastChar 67 /Widths [500 500 500] /Encoding /WinAnsiEncoding", "", "(ABC) Tj", cffSpec{predefined: 1, charstrings: make230()}.build(), "Type1C")},
		// P07.S05a: a Type1C program of 10240 bytes or more whose CharStrings offset lies past its end — the file-backed
		// stream's seek throws an IllegalArgumentException (the boundary P07.S03 measured on TrueType).
		{"a big Type1C program whose CharStrings lie past its end", func() []byte {
			p := cffSpec{names: []string{"A", "B", "C"}}.build()
			for len(p) < 12000 {
				p = append(p, 0)
			}
			for i := 0; i+5 < len(p); i++ {
				if p[i] == 29 && p[i+5] == 17 {
					copy(p[i+1:i+5], []byte{0, 0x10, 0, 0})
					break
				}
			}
			return t1cDoc("ABCDEF+Probe", "/FirstChar 65 /LastChar 67 /Widths [500 500 500] /Encoding /WinAnsiEncoding", "", "(A) Tj", p, "Type1C")
		}()},
		// The P07.S05a review: a program that throws is read when the FONT object is built — an empty string suffices —
		// and a String INDEX whose offsets run backwards throws from `Arrays.copyOfRange`.
		{"a Type1C program that throws, drawn only by an empty string", t1cDoc("ABCDEF+Probe", "/FirstChar 65 /LastChar 67 /Widths [500 500 500] /Encoding /WinAnsiEncoding", "", "() Tj", cffSpec{predefined: 1, charstrings: make230()}.build(), "Type1C")},
		{"a Type1C program whose String INDEX runs backwards", t1cDoc("ABCDEF+Probe", "/FirstChar 65 /LastChar 67 /Widths [500 500 500] /Encoding /WinAnsiEncoding", "", "(A) Tj", func() []byte {
			p := cffSpec{names: []string{"A", "xa", "C"}}.build()
			p[bytes.Index(p, []byte{0, 1, 1, 1, 3, 'x', 'a'})+3] = 4
			return p
		}(), "Type1C")},
		// P07.S05a's red-proof: a subset font computes every width at parse, so an UNDRAWN charstring running backwards throws.
		{"a subset Type1C program with an undrawn charstring running backwards", func() []byte {
			p := cffSpec{names: []string{"A", "B", "C"}, charstrings: [][]byte{cs(300, "endchar"), cs(500, "endchar"), cs(500, "endchar"), cs(500, 1, "hmoveto")}}.build()
			i := bytes.Index(p, []byte{0, 4, 1, 1})
			p[i+6] = p[i+7] + 1
			return t1cDoc("ABCDEF+Probe", "/FirstChar 65 /LastChar 67 /Widths [500 500 500] /Encoding /WinAnsiEncoding", "/CharSet (/A/B/C)", "(A) Tj", p, "Type1C")
		}()},
		{"a TrueType width off, then a Type 3 font whose /CharProcs is an array", failFirst(map[int]string{
			50: t3Font("97 /a", "/FirstChar 97 /LastChar 97 /Widths [10]", "/CharProcs [26 0 R]", ""), 26: proc("10 0 d0")}, "(a)")},
		{"a TrueType width off, then a negative CID", failFirst(map[int]string{
			50: "<< /Type /Font /Subtype /Type0 /BaseFont /Probe /Encoding 21 0 R /DescendantFonts [52 0 R] >>",
			52: "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /Probe /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /FontDescriptor 12 0 R /DW 500 /CIDToGIDMap /Identity >>",
			21: cidCMap("", "1 begincidchar <0021> -5 endcidchar")}, "<0021>")},
	}
	// P07.S05b: a CID-keyed CFF program under a simple Type 1 font, drawn or compared with a /CharSet; a CID program whose
	// charstrings run backwards. P07.S06: a Type 1 program whose PostScript throws (an empty-stack pop or dup, an idiv by
	// zero, a cleartomark with no mark, a FontMatrix of seven numbers).
	for _, f := range append(cidCFFFixtures(), type1Fixtures()...) {
		if strings.Contains(f.vera, "X") {
			out = append(out, struct {
				name string
				pdf  []byte
			}{f.name, f.pdf})
		}
	}
	return out
}

// TestADocumentVeraPDFReportsNothingOnRefusesEveryClause — there is no report to agree with, so EVERY clause refuses
// and says why, not only the glyph clauses; and where veraPDF is present it is asked again whether it still reports
// nothing, so a veraPDF that starts answering turns this red rather than leaving nib refusing a document it could judge.
func TestADocumentVeraPDFReportsNothingOnRefusesEveryClause(t *testing.T) {
	fixtures := noReportFixtures()
	for _, f := range fixtures {
		rep, err := Check(f.pdf)
		if err != nil {
			t.Fatalf("%s: %v", f.name, err)
		}
		if len(rep.Results) != len(Clauses()) {
			t.Fatalf("%s: %d results for %d clauses", f.name, len(rep.Results), len(Clauses()))
		}
		for _, r := range rep.Results {
			if r.Verdict != CannotCheck || !strings.Contains(r.Why, "reports nothing") {
				t.Errorf("%s: %s reports %v (%s), want CannotCheck saying veraPDF reports nothing", f.name, r.Clause, r.Verdict, r.Why)
			}
		}
	}
	vp := veraPDFPath()
	if vp == "" {
		t.Log("NOTE (not a pass): veraPDF is absent, so whether it still reports nothing on these documents is unchecked in this run")
		return
	}
	dir := t.TempDir()
	var files []string
	for i, f := range fixtures {
		files = append(files, filepath.Join(dir, fmt.Sprintf("none%02d.pdf", i)))
		if err := os.WriteFile(files[i], f.pdf, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	out, _ := exec.Command(vp, append([]string{"--flavour", "ua1", "--passed"}, files...)...).Output()
	var rep veraReport
	if err := xml.Unmarshal(out, &rep); err != nil {
		t.Fatalf("the veraPDF report did not parse: %v", err)
	}
	seen := 0
	for _, j := range rep.Jobs {
		for i, f := range files {
			if filepath.Base(j.Item.Name) == filepath.Base(f) {
				seen++
				if len(j.Report.Details.Rules) != 0 || j.Report.Status != "" {
					t.Errorf("%s: veraPDF now reports on it (%d rules) — nib refuses a document veraPDF judges", fixtures[i].name, len(j.Report.Details.Rules))
				}
			}
		}
	}
	if seen != len(files) {
		t.Errorf("veraPDF returned %d job(s) for %d document(s)", seen, len(files))
	}
}

// TestCIDLookupsPastTheBudgetRefuseTheClause — the budget refusal reaches the verdict: 25 codes held only by a notdef
// range, which is asked after all 200,000 CID entries, walk 5,000,000 mappings — past the document's font-read budget —
// and every glyph read in full passes, so the metric clauses refuse naming the ceiling rather than answering from a
// lookup that stopped short.
func TestCIDLookupsPastTheBudgetRefuseTheClause(t *testing.T) {
	var b strings.Builder
	b.WriteString("200000 begincidchar ")
	for i := 0; i < 200000; i++ {
		fmt.Fprintf(&b, "<%04X> 5 ", 0x100+i%0xF000)
	}
	b.WriteString("endcidchar 1 beginnotdefrange <0000> <00FF> 5 endnotdefrange")
	var show strings.Builder
	for c := 0x21; c < 0x21+25; c++ {
		fmt.Fprintf(&show, "<%04X> Tj ", c)
	}
	pdf := cidCMapDoc(cidCMap("", b.String()), "/DW 500 /CIDToGIDMap /Identity", "BT /F0 12 Tf 10 10 Td "+show.String()+"ET", ttProgram(sub31, sub10), nil)
	for _, clause := range metricClauses {
		if got := verdictOf(t, pdf, clause); got.Verdict != CannotCheck || !strings.Contains(got.Why, "cost more than") {
			t.Errorf("%s reports %v (%s), want CannotCheck naming the budget", clause, got.Verdict, got.Why)
		}
	}
}

// TestAnEmbeddedCMapUsingAPredefinedOneNibDoesNotCarryRefuses — an embedded CMap whose program says
// `/UniGB-UTF16-H usecmap` merges a CMap nib carries no table for (ADR-117: the version it could carry is not the one
// veraPDF reads), so its codes cannot be cut as veraPDF cuts them; the metric clauses refuse naming it. A later
// `usecmap` of a name veraPDF does not carry must not hide it (the P07.S04b review's multi-name finding; the red-proof
// found no document reaching this refusal), and neither must one nib DOES carry, which is merged and not left to refuse.
func TestAnEmbeddedCMapUsingAPredefinedOneNibDoesNotCarryRefuses(t *testing.T) {
	pdf := cidCMapDoc(cidCMap("", "/UniGB-UTF16-H usecmap /GB-EUC-H usecmap /Nope usecmap 1 begincidchar <0021> 5 endcidchar"),
		"/DW 500 /CIDToGIDMap /Identity", "BT /F0 12 Tf 10 10 Td <0021> Tj ET", ttProgram(sub31, sub10), nil)
	for _, clause := range metricClauses {
		if got := verdictOf(t, pdf, clause); got.Verdict != CannotCheck || !strings.Contains(got.Why, "UniGB-UTF16-H") {
			t.Errorf("%s reports %v (%s), want CannotCheck naming UniGB-UTF16-H", clause, got.Verdict, got.Why)
		}
	}
}
