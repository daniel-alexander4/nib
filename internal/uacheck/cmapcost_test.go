package uacheck

import (
	"fmt"
	"strings"
	"testing"
)

// TestAnEmbeddedCMapIsDecodedAndReadOncePerStream — `/pending 730`: `DereferenceStreamDict` hands each font a fresh copy
// of the CMap's stream dictionary, so the decode one font wrote into its copy was not in the next font's, and the glyph
// door's codespace and 7.21.3.3 t2's program /WMode were read per FONT (670 ms + 234 ms a font, measured on an 8 MB CMap).
// One font and eight naming the same stream must cost the same decodes and the same parses.
func TestAnEmbeddedCMapIsDecodedAndReadOncePerStream(t *testing.T) {
	sys := "/CIDSystemInfo << /Registry (Adobe) /Ordering (GB1) /Supplement 2 >>"
	read := func(n int) (decodes, parses, decoded int) {
		extra := map[int]string{20: cmapStream("/WMode 1 "+sys, "Cust", "/WMode 1 def")}
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
		for _, f := range fs {
			if gf := d.glyphFontFor(f.dict, 0, "F"); gf.cs == nil {
				t.Fatalf("stimulus: a font's codespace was not read (%s)", gf.unread)
			}
		}
		if got := checkCMapWMode(d); got.Verdict != Pass {
			t.Fatalf("stimulus: 7.21.3.3 t2 = %v (%s), want Pass — the program and dictionary agree", got.Verdict, got.Why)
		}
		return d.fontDecodes, d.cmapCodespaceParses, d.fontDecoded
	}
	d1, p1, b1 := read(1)
	d8, p8, b8 := read(8)
	if d1 == 0 || p1 == 0 {
		t.Fatalf("stimulus: one font decoded %d streams and parsed %d codespaces", d1, p1)
	}
	if d8 != d1 || b8 != b1 {
		t.Errorf("eight fonts naming one CMap decoded %d streams (%d bytes) where one font decoded %d (%d bytes)", d8, b8, d1, b1)
	}
	if p8 != p1 {
		t.Errorf("eight fonts naming one CMap parsed its codespace %d times where one font parsed it %d", p8, p1)
	}
}
