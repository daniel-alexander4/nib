package pdfops

import (
	"bytes"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/testpdf"
	"nib/mdpdf"
)

// The width reader — `PLAN-accessibility.md` P08.S01 (`PLAN-text-reflow.md` P02).

func widthXRef(t *testing.T) *model.XRefTable {
	t.Helper()
	pdf, err := testpdf.Text("x")
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	return ctx.XRefTable
}

func nums(vs ...float64) types.Array {
	a := types.Array{}
	for _, v := range vs {
		if v == float64(int(v)) {
			a = append(a, types.Integer(int(v)))
		} else {
			a = append(a, types.Float(v))
		}
	}
	return a
}

// TestEveryWidthLookupNamesItsSource — S01's first clause: a width AND where it came from, with `none`
// where the dictionary says nothing. Each source is driven by a dictionary carrying exactly it.
func TestEveryWidthLookupNamesItsSource(t *testing.T) {
	xt := widthXRef(t)
	simple := types.Dict{"Type": types.Name("Font"), "Subtype": types.Name("TrueType"),
		"BaseFont": types.Name("Arial"), "FirstChar": types.Integer(32), "Widths": nums(278, 0, 512.5)}
	withMissing := simple.Clone().(types.Dict)
	withMissing["FontDescriptor"] = types.Dict{"MissingWidth": types.Integer(333)}
	helvetica := types.Dict{"Type": types.Name("Font"), "Subtype": types.Name("Type1"),
		"BaseFont": types.Name("Helvetica"), "Encoding": types.Name("WinAnsiEncoding")}
	type0 := func(desc types.Dict) types.Dict {
		return types.Dict{"Type": types.Name("Font"), "Subtype": types.Name("Type0"),
			"BaseFont": types.Name("X"), "DescendantFonts": types.Array{desc}}
	}
	cidW := types.Dict{"Subtype": types.Name("CIDFontType2"),
		"W": types.Array{types.Integer(1), nums(500), types.Integer(10), nums(100, 200, 300),
			types.Integer(20), types.Integer(25), types.Integer(700)},
		"DW": types.Integer(800)}
	cidNoDW := types.Dict{"Subtype": types.Name("CIDFontType2"), "W": types.Array{types.Integer(1), nums(500)}}

	for _, c := range []struct {
		name string
		font types.Dict
		code int
		want float64
		src  widthSource
	}{
		{"simple font, in /Widths", simple, 32, 278, widthFromWidths},
		{"simple font, a DECLARED zero is not silent", simple, 33, 0, widthFromWidths},
		{"simple font, a real in /Widths", simple, 34, 512.5, widthFromWidths},
		{"simple font, below /FirstChar", simple, 31, 0, widthNone},
		{"simple font, past /Widths", simple, 35, 0, widthNone},
		{"MissingWidth covers a code outside /Widths", withMissing, 40, 333, widthFromMissing},
		{"a WinAnsi core font through CoreWidth", helvetica, 'A', mdpdf.CoreWidth("A", "Helvetica", 1000), widthFromStd14},
		{"/W form c [w]", type0(cidW), 1, 500, widthFromW},
		{"/W form c [w1 w2 w3], third entry", type0(cidW), 12, 300, widthFromW},
		{"/W form cfirst clast w, last CID", type0(cidW), 25, 700, widthFromW},
		{"a CID /W does not list falls to /DW", type0(cidW), 26, 800, widthFromDW},
		{"no /DW is the declared default, named apart", type0(cidNoDW), 2, 1000, widthFromDefault},
		{"a Type0 with no descendant describes nothing", types.Dict{"Subtype": types.Name("Type0")}, 1, 0, widthNone},
		{"a non-core name with no widths", types.Dict{"Subtype": types.Name("TrueType"), "BaseFont": types.Name("Arial")}, 'A', 0, widthNone},
	} {
		got, src := readFontWidths(xt, c.font).advance(c.code)
		if src != c.src || got != c.want {
			t.Errorf("%s: code %d = %v from %q, want %v from %q", c.name, c.code, got, src, c.want, c.src)
		}
	}
}

// TestTheCoreFontPathRefusesTheNumbersPdfcpuInvents — the two ways the std14 source would guess.
func TestTheCoreFontPathRefusesTheNumbersPdfcpuInvents(t *testing.T) {
	xt := widthXRef(t)
	// Measured: the codes pdfcpu's WinAnsi map lacks, found by Courier reading 1000 where every real
	// glyph is 600. Stimulus first — the discriminator must find some and must not find printable ASCII.
	var unmapped []int
	for c := 0; c <= 0xFF; c++ {
		if !coreCodeMapped(c) {
			unmapped = append(unmapped, c)
		}
	}
	if len(unmapped) == 0 {
		t.Fatal("no code reads 1000 in Courier, so the discriminator separates nothing")
	}
	for c := 32; c < 127; c++ {
		if !coreCodeMapped(c) {
			t.Errorf("printable ASCII code %d reads as unmapped — the discriminator is refusing real glyphs", c)
		}
	}
	t.Logf("%d of 256 codes unmapped in pdfcpu's WinAnsi core metrics", len(unmapped))

	helv := types.Dict{"Subtype": types.Name("Type1"), "BaseFont": types.Name("Helvetica"), "Encoding": types.Name("WinAnsiEncoding")}
	if w, src := readFontWidths(xt, helv).advance(unmapped[0]); src != widthNone {
		t.Errorf("an unmapped code in Helvetica returned %v from %q — pdfcpu's substituted 1000, reported as the document's width", w, src)
	}
	// The same font with no /Encoding is StandardEncoding, whose codes WinAnsi's metrics do not describe.
	noEnc := types.Dict{"Subtype": types.Name("Type1"), "BaseFont": types.Name("Helvetica")}
	if _, src := readFontWidths(xt, noEnc).advance('A'); src != widthNone {
		t.Errorf("a core font with no /Encoding was measured with WinAnsi metrics (source %q)", src)
	}
	for _, name := range []string{"Symbol", "ZapfDingbats"} {
		sym := types.Dict{"Subtype": types.Name("Type1"), "BaseFont": types.Name(name), "Encoding": types.Name("WinAnsiEncoding")}
		if _, src := readFontWidths(xt, sym).advance('a'); src != widthNone {
			t.Errorf("%s was measured through the WinAnsi discriminator (source %q), which does not cover its map", name, src)
		}
	}
}

// TestAFractionalSizeIsNotTruncated — the core-font door takes an int size; a reader's does not.
func TestAFractionalSizeIsNotTruncated(t *testing.T) {
	xt := widthXRef(t)
	helv := readFontWidths(xt, types.Dict{"Subtype": types.Name("Type1"), "BaseFont": types.Name("Helvetica"), "Encoding": types.Name("WinAnsiEncoding")})
	var glyph float64
	for _, c := range []byte("Hello") {
		w, src := helv.advance(int(c))
		if src != widthFromStd14 {
			t.Fatalf("%q read from %q", c, src)
		}
		glyph += w
	}
	if got, want := advanceAt(glyph, 12), mdpdf.CoreWidth("Hello", "Helvetica", 12); got != want {
		t.Errorf("at a whole size the reader says %v and the door says %v", got, want)
	}
	lo, hi, got := mdpdf.CoreWidth("Hello", "Helvetica", 9), mdpdf.CoreWidth("Hello", "Helvetica", 10), advanceAt(glyph, 9.5)
	if !(got > lo && got < hi) {
		t.Errorf("at 9.5pt the width is %v, not strictly between the 9pt %v and 10pt %v — the size was truncated", got, lo, hi)
	}
}

// TestAMalformedWEntryStopsTheParseAndIsNotExpanded — a `cfirst clast w` claiming more than 16 bits
// of CIDs, and a truncated entry, must neither allocate without bound nor invent widths.
func TestAMalformedWEntryStopsTheParseAndIsNotExpanded(t *testing.T) {
	xt := widthXRef(t)
	huge := readW(xt, types.Array{types.Integer(0), types.Integer(2147483647), types.Integer(500)})
	if len(huge) != maxCID+1 {
		t.Errorf("a range to 2^31 expanded to %d entries, want the 16-bit ceiling %d", len(huge), maxCID+1)
	}
	trunc := readW(xt, types.Array{types.Integer(1), nums(500), types.Integer(5), types.Integer(9)})
	if len(trunc) != 1 || trunc[1] != 500 {
		t.Errorf("a truncated trailing range yielded %v, want only CID 1", trunc)
	}
}

// TestTheWidthCensusOverTheCorpus — S01's census clause, and a guard: every font the generated corpus
// carries is measured over the codes it can draw, per source, and `none` over those codes is red.
//
// The probe set is the font's own range, not a fixed ASCII span: LibreOffice re-encodes a subset from
// code 0 (measured: `LiberationMono`, `/FirstChar 0`, 25 widths), so a probe of 32–126 would read a
// perfectly described font as `none`. What this cannot see is a code the CONTENT draws outside that
// range — that needs S02's run reader, which is where it is asserted.
func TestTheWidthCensusOverTheCorpus(t *testing.T) {
	core, err := testpdf.Text("census")
	if err != nil {
		t.Fatal(err)
	}
	md, err := ConvertDocToPDF([]byte("# Heading\n\nA paragraph with **bold** and *italic*.\n\n- one\n"), ".md")
	if err != nil {
		t.Fatal(err)
	}
	corpus := []struct {
		name string
		pdf  []byte
	}{{"core-font page", core}, {"converted Markdown", md}}
	if LibreOfficeAvailable() {
		lo, lerr := ConvertOfficeToPDF([]byte("A heading\n\nA paragraph of ordinary prose.\n"), "txt")
		if lerr != nil {
			t.Fatalf("LibreOffice is present and could not convert: %v", lerr)
		}
		corpus = append(corpus, struct {
			name string
			pdf  []byte
		}{"LibreOffice text", lo})
	} else {
		t.Log("NOTE (a narrower census, not a pass): LibreOffice is absent, so /Widths is not measured on a third-party producer")
	}

	counts := map[widthSource]int{}
	for _, doc := range corpus {
		ctx, rerr := api.ReadValidateAndOptimize(bytes.NewReader(doc.pdf), model.NewDefaultConfiguration())
		if rerr != nil {
			t.Fatalf("%s: %v", doc.name, rerr)
		}
		fonts := 0
		for _, e := range ctx.Table {
			if e == nil {
				continue
			}
			d, ok := e.Object.(types.Dict)
			if !ok || d.Type() == nil || *d.Type() != "Font" {
				continue
			}
			st := d.NameEntry("Subtype")
			if st != nil && strings.HasPrefix(*st, "CIDFontType") {
				continue // a descendant; measured through its Type0 parent
			}
			fonts++
			fw := readFontWidths(ctx.XRefTable, d)
			var codes []int
			switch {
			case fw.cid:
				for cid := range fw.w {
					codes = append(codes, cid)
				}
			case fw.widths != nil:
				for i := range fw.widths {
					codes = append(codes, fw.firstChar+i)
				}
			default:
				for c := 32; c < 127; c++ {
					codes = append(codes, c)
				}
			}
			sort.Ints(codes)
			for _, c := range codes {
				_, src := fw.advance(c)
				counts[src]++
				if src == widthNone {
					t.Errorf("%s: font %v code %d reads as none — the corpus's own font is not described", doc.name, d["BaseFont"], c)
				}
			}
		}
		if fonts == 0 {
			t.Errorf("%s carries no font, so it measured nothing", doc.name)
		}
	}
	t.Logf("census by source: %v", counts)
	floors := []widthSource{widthFromStd14, widthFromW}
	if LibreOfficeAvailable() {
		floors = append(floors, widthFromWidths)
	}
	for _, src := range floors {
		if counts[src] == 0 {
			t.Errorf("no lookup in the corpus came from %q — that source is no longer measured by it", src)
		}
	}
}
