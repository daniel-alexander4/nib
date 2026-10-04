package uacheck

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"strings"
	"testing"
)

// The P07 phase-close re-review's cost findings in the font clauses (RR1-3, RR1-4, RR1-6). Every test asserts a WORK
// COUNTER, never a wall clock, and asserts the stimulus happened before it grades the response.

// flatedStream is a /FlateDecode stream whose decoded bytes are `b`.
func flatedStream(b []byte) string {
	var z bytes.Buffer
	w := zlib.NewWriter(&z)
	w.Write(b)
	w.Close()
	return fmt.Sprintf("<< /Filter /FlateDecode /Length %d >>\nstream\n%s\nendstream", z.Len(), z.String())
}

// TestACIDSetIsReadOncePerDescendant — RR1-3: 7.21.4.2 t2's answer is the descendant's, and it was re-read for every
// Type 0 font naming it. Eight fonts over one descendant read its /CIDSet once, and each still answers.
func TestACIDSetIsReadOncePerDescendant(t *testing.T) {
	const n = 8
	sub := "ABCDEF+Probe"
	objs := map[int]string{
		11: "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /" + sub + " /CIDSystemInfo << /Registry (Adobe) /Ordering " +
			"(Identity) /Supplement 0 >> /FontDescriptor 12 0 R /CIDToGIDMap /Identity >>",
	}
	for k, v := range ttObjects("/Flags 4", "FontFile2", ttCounts([]int{100}, []int{100}, ttCmap(ttSub{3, 1, cmapFmt4(0x20, 95)})), "") {
		objs[k] = v
	}
	objs[12] = strings.TrimSuffix(objs[12], " >>") + " /CIDSet 30 0 R >>"
	objs[30] = cidSetBytes(1, 2) // short: every font fails, so every font is judged
	var fonts, content strings.Builder
	for i := 1; i < n; i++ {
		fmt.Fprintf(&fonts, "/F%d %d 0 R ", i, 100+i)
		objs[100+i] = fmt.Sprintf("<< /Type /Font /Subtype /Type0 /BaseFont /%s%d /Encoding /Identity-H /DescendantFonts [11 0 R] >>", sub, i)
	}
	for i := 0; i < n; i++ {
		fmt.Fprintf(&content, "/F%d 12 Tf <00010002> Tj ", i)
	}
	pdf := glyphPage("BT 10 10 Td "+content.String()+"ET", fonts.String(), "",
		"<< /Type /Font /Subtype /Type0 /BaseFont /"+sub+" /Encoding /Identity-H /DescendantFonts [11 0 R] >>", objs)
	d, err := open(pdf)
	if err != nil {
		t.Fatal(err)
	}
	used, why := d.usedFonts()
	type0 := 0
	for _, f := range used {
		if !f.unresolve && d.name(f.dict["Subtype"]) == "Type0" {
			type0++
			fd := d.descriptorOf(f.dict)
			if r := d.cidSetResult(f, fd); r == nil || r.Verdict != Fail || !strings.Contains(r.Where, f.name) {
				t.Errorf("font %s: %v, want its own Fail — the cached answer must still be each font's", f.name, r)
			}
		}
	}
	if why != "" || type0 != n {
		t.Fatalf("stimulus: %d Type 0 fonts used (why %q), want %d", type0, why, n)
	}
	if d.cidSetJudged != 1 {
		t.Errorf("the /CIDSet was judged %d times over %d fonts sharing one descendant, want once", d.cidSetJudged, n)
	}
}

// TestTheCIDSetAndCIDToGIDMapDecodesHaveACeiling — RR1-3: both were a plain `Decode()`, so a compressed stream expanded
// as far as the document chose. The control sits exactly at each ceiling and answers; one byte past it refuses.
func TestTheCIDSetAndCIDToGIDMapDecodesHaveACeiling(t *testing.T) {
	sub := "ABCDEF+Probe"
	p100 := ttCounts([]int{100}, []int{100}, ttCmap(ttSub{3, 1, cmapFmt4(0x20, 95)}))
	// A complete set for CIDs 0-99, zero-padded to n bytes: veraPDF reads the first 16 KiB and finds nothing more.
	set := func(n int) string {
		b := make([]byte, n)
		for i := 0; i < 12; i++ {
			b[i] = 0xFF
		}
		b[12] = 0xF0
		return flatedStream(b)
	}
	for _, c := range []struct {
		name    string
		pdf     []byte
		refused bool
	}{
		{"a /CIDSet at the ceiling", cid2Doc(sub, "/CIDToGIDMap /Identity", p100, "FontFile2", "", set(maxCIDSetDecoded), nil), false},
		{"a /CIDSet a byte past it", cid2Doc(sub, "/CIDToGIDMap /Identity", p100, "FontFile2", "", set(maxCIDSetDecoded+1), nil), true},
	} {
		d, err := open(c.pdf)
		if err != nil {
			t.Fatal(err)
		}
		got := checkCIDSetsComplete(d)
		if d.cidSetJudged != 1 {
			t.Fatalf("%s: stimulus: the /CIDSet was judged %d times, want once", c.name, d.cidSetJudged)
		}
		switch {
		case c.refused && (got.Verdict != CannotCheck || !strings.Contains(got.Why, "exceeds")):
			t.Errorf("%s: %v (%s), want CannotCheck naming the ceiling", c.name, got.Verdict, got.Why)
		case !c.refused && got.Verdict != Pass:
			t.Errorf("%s: %v (%s), want Pass — the set is complete", c.name, got.Verdict, got.Why)
		}
	}
	// The map: CIDs 1-3 to glyphs 1-3, and every later CID to glyph 0, zero-padded to n bytes.
	gidMap := func(n int) map[int]string {
		b := make([]byte, n)
		copy(b, []byte{0, 0, 0, 1, 0, 2, 0, 3})
		return map[int]string{31: flatedStream(b)}
	}
	for _, c := range []struct {
		name    string
		n       int
		refused bool
	}{
		{"a /CIDToGIDMap at the ceiling", maxCIDToGIDMapDecoded, false},
		{"a /CIDToGIDMap two bytes past it", maxCIDToGIDMapDecoded + 2, true},
	} {
		d, err := open(cid2Doc(sub, "/CIDToGIDMap 31 0 R", p100, "FontFile2", "", cidSetBytes(1, 2, 3), gidMap(c.n)))
		if err != nil {
			t.Fatal(err)
		}
		got := checkCIDSetsComplete(d)
		used, _ := d.usedFonts()
		if len(used) != 1 || d.descendantOf(used[0].dict) == nil {
			t.Fatalf("%s: stimulus: %d fonts used, want the one Type 0 font", c.name, len(used))
		}
		m, why := d.cidGIDMapOf(d.descendantOf(used[0].dict))
		switch {
		case c.refused && (got.Verdict != CannotCheck || !strings.Contains(got.Why, "exceeds")):
			t.Errorf("%s: %v (%s), want CannotCheck naming the ceiling", c.name, got.Verdict, got.Why)
		case !c.refused && (got.Verdict == CannotCheck || m == nil || m.size() != c.n/2):
			t.Errorf("%s: %v (%s), map %v (%q) — want a definite answer over the whole map", c.name, got.Verdict, got.Why, m != nil, why)
		}
	}
}

// TestAMalformedCMapIsParsedOncePerStream — RR1-4: 7.21.3.3 t2 re-parsed a malformed CMap for every font naming it.
func TestAMalformedCMapIsParsedOncePerStream(t *testing.T) {
	const n = 8
	sys := "/CIDSystemInfo << /Registry (Adobe) /Ordering (GB1) /Supplement 2 >>"
	extra := map[int]string{20: cmapStream("/WMode 1 "+sys, "Cust", "/WMode 1 def 1 begincidchar <0021> (x) endcidchar")}
	var fonts, content strings.Builder
	fonts.WriteString("/F0 10 0 R ")
	content.WriteString("/F0 12 Tf <2121> Tj ")
	for i := 1; i < n; i++ {
		fmt.Fprintf(&fonts, "/F%d %d 0 R ", i, 100+i)
		fmt.Fprintf(&content, "/F%d 12 Tf <2121> Tj ", i)
		extra[100+i] = fmt.Sprintf("<< /Type /Font /Subtype /Type0 /BaseFont /STSong-Light-%d /Encoding 20 0 R /DescendantFonts [11 0 R] >>", i)
	}
	extra[3] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R /StructParents 0 /Resources << /Font << " +
		fonts.String() + ">> >> >>"
	extra[4] = spStream("", "/P <</MCID 0>> BDC BT 10 10 Td "+content.String()+"ET EMC")
	d, err := open(buildPDF(type0Doc("20 0 R", sys, extra)))
	if err != nil {
		t.Fatal(err)
	}
	fs, why := d.type0Fonts()
	if why != "" || len(fs) != n {
		t.Fatalf("stimulus: %d Type 0 fonts (why %q), want %d", len(fs), why, n)
	}
	if got := checkCMapWMode(d); got.Verdict != CannotCheck || !strings.Contains(got.Why, "wrong kind") {
		t.Fatalf("7.21.3.3 t2 = %v (%s), want the malformed CMap's refusal", got.Verdict, got.Why)
	}
	if d.cmapCodespaceParses != 1 {
		t.Errorf("one malformed CMap named by %d fonts was parsed %d times, want once", n, d.cmapCodespaceParses)
	}
}

// TestASpentFontBudgetIsReportsNothingsRefusal — RR1-6: past a document budget, the programs read after it are refused
// UNREAD, so a throw veraPDF meets in one is invisible to `reportsNothing` — font A's cost hid font B's throw. A spent
// counter is itself the refusal; at its bound exactly, it is not. Each counter is set directly, because a document that
// spends one costs seconds to read (`TestTheCFFBudgetsAreTheDocuments` and `TestTheType1BudgetIsTheDocuments` drive
// the readers to them).
func TestASpentFontBudgetIsReportsNothingsRefusal(t *testing.T) {
	pdf := ttDoc("/Flags 32", "", "(A) Tj", ttProgram(sub31))
	for _, c := range []struct {
		name  string
		spend func(d *Document, over int)
	}{
		{"CFF charstrings", func(d *Document, o int) { d.cffSpent.charstring = cffMaxCharstringTotal + o }},
		{"CFF font dicts", func(d *Document, o int) { d.cffSpent.dict = cffMaxDictBytes + o }},
		{"CFF FDSelect", func(d *Document, o int) { d.cffSpent.fill = cffMaxFDSelectFill + o }},
		{"CFF names", func(d *Document, o int) { d.cffSpent.names = cffMaxNameBytes + o }},
		{"Type 1 objects", func(d *Document, o int) { d.type1Spent.ops = t1MaxOps + o }},
		{"Type 1 array slots", func(d *Document, o int) { d.type1Spent.alloc = t1MaxAlloc + o }},
		{"Type 1 decryption", func(d *Document, o int) { d.type1Spent.decrypted = t1MaxDecrypt + o }},
		{"Type 1 private tokens", func(d *Document, o int) { d.type1Spent.steps = t1MaxSteps + o }},
	} {
		for _, over := range []int{0, 1} {
			d, err := open(pdf)
			if err != nil {
				t.Fatal(err)
			}
			c.spend(d, over)
			why := d.reportsNothing()
			switch {
			case over == 0 && why != "":
				t.Errorf("%s at its bound: reportsNothing refuses (%s) — a budget not yet passed hid nothing", c.name, why)
			case over == 1 && !strings.Contains(why, "budget"):
				t.Errorf("%s one past its bound: reportsNothing = %q, want a refusal naming the budget", c.name, why)
			}
		}
	}
}
