package pdfops

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// The P08 phase-close review's findings, each held by the shape that found it.

func courierPage(content string) []byte {
	return onePageFixture(content, "<< /Type /Font /Subtype /Type1 /BaseFont /Courier /Encoding /WinAnsiEncoding >>")
}

// TestASentenceEndingInAQuoteStillReadsJustified — W2: the sentence-gap exemption tested the last BYTE, so a sentence
// ending ".”" (a curly quote's last byte is a continuation byte) counted its wide gap as an ordinary one and the
// paragraph read ragged — then every word kept its old line's `Tw` (S03's C3). Read by rune, past closing quotes.
//
// **The stimulus is asserted**: the same paragraph ending "abc." reads justified.
func TestASentenceEndingInAQuoteStillReadsJustified(t *testing.T) {
	read := func(w string) bool {
		var b strings.Builder
		b.WriteString("BT /F1 10 Tf\n")
		for _, ln := range []struct {
			y  float64
			ws []string
			xs []float64
		}{
			{700, []string{"aaaa", "bbbb", "cccc", "dddd"}, []float64{72, 104, 136, 168}},
			{688, []string{w, "cccc", "dddd", "eeee"}, []float64{72, 108, 138, 168}},
			{676, []string{"hhhh", "iiii", "jjjj", "kkkk"}, []float64{72, 104, 136, 168}},
			{664, []string{"ffff", "gggg"}, []float64{72, 102}},
		} {
			for i := range ln.ws {
				fmt.Fprintf(&b, "1 0 0 1 %g %g Tm (%s) Tj\n", ln.xs[i], ln.y, ln.ws[i])
			}
		}
		b.WriteString("ET")
		l, _ := layoutOf(t, courierPage(b.String()))
		lines, _, cause := paragraphWords(l.paragraphs[0])
		if cause != "" {
			t.Fatalf("%q: refused (%s)", w, cause)
		}
		return paragraphAlignment(l, 0, lines).justified
	}
	if !read("abc.") {
		t.Fatalf("setup: the paragraph ending \"abc.\" does not read justified")
	}
	if !read(`ab.\224`) {
		t.Errorf("the paragraph ending \"ab.”\" does not read justified")
	}
}

// TestAWordThatReadsAsNothingDoesNotPanic — W3: a glyph a /ToUnicode maps to nothing starts a word whose text is "",
// and the justification check sliced its last byte: `slice bounds [-1:]`, the request dropped instead of answered.
func TestAWordThatReadsAsNothingDoesNotPanic(t *testing.T) {
	cmap := "/CIDInit /ProcSet findresource begin 12 dict begin begincmap /CMapName /X def 1 begincodespacerange <00> <FF> endcodespacerange 1 beginbfchar <41> <> endbfchar endcmap CMapName currentdict /CMap defineresource pop end end"
	content := "BT /F1 12 Tf 72 700 Td (xx A yy zz) Tj 0 -14 Td (ww vv) Tj ET"
	pdf := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding /ToUnicode 6 0 R >>",
		6: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(cmap), cmap),
	})
	l, _ := layoutOf(t, pdf)
	lines, _, cause := paragraphWords(l.paragraphs[0])
	empty := false
	for _, ln := range lines {
		for _, w := range ln {
			empty = empty || w.text() == ""
		}
	}
	if cause != "" || !empty {
		t.Fatalf("setup: the paragraph reads with cause %q and an empty word %v", cause, empty)
	}
	paragraphAlignment(l, 0, lines) // panicked
	reflowed(t, pdf, 0, "xx yy zz ww vv qq")
}

// TestTheSpaceAfterALeadingFigureIsTheBodys — W1: S06's fix took the space after a word in its FIRST glyph's look, so
// a word that begins small ("¹⁴C") had the space after it drawn and measured in the figure's — 3.336 pt became 1.946.
// The space is the word's body look, its largest.
func TestTheSpaceAfterALeadingFigureIsTheBodys(t *testing.T) {
	pdf := helveticaPage("BT /F1 12 Tf 72 700 Td (The ) Tj /F1 7 Tf (14) Tj /F1 12 Tf (C dating is old and more) Tj ET")
	l, _ := layoutOf(t, pdf)
	if w := findWordIn(t, l, "14C"); w == nil || w.looks[0].tfSize == w.looks[2].tfSize {
		t.Fatalf("setup: no word 14C beginning small")
	}
	out, cause := reflowed(t, pdf, 0, strings.Replace(l.paragraphs[0].text(), "old", "il", 1))
	if cause != "" {
		t.Fatalf("refused (%s)", cause)
	}
	l2, _ := layoutOf(t, out)
	lines, _, _ := paragraphWords(l2.paragraphs[0])
	var after float64 = -1
	var others []float64
	for _, ln := range lines {
		for i := 1; i < len(ln); i++ {
			g := ln[i].startX - ln[i-1].startX - ln[i-1].width
			if ln[i-1].text() == "14C" {
				after = g
			} else {
				others = append(others, g)
			}
		}
	}
	if after < 0 || len(others) == 0 || math.Abs(after-others[0]) > 1e-6 {
		t.Errorf("the space after 14C is %v, the body's %v", after, others)
	}
}

func findWordIn(t *testing.T, l pageLayout, text string) *reflowWord {
	t.Helper()
	lines, _, _ := paragraphWords(l.paragraphs[0])
	return findWord(lines, text)
}
