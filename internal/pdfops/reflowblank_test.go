package pdfops

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// spacedShows draws words as one-glyph-run producers do: each word its own show and each space its own `( ) Tj`, n words
// to a line over three lines.
func spacedShows(n int) string {
	words := strings.Fields("The quick brown fox jumps over the lazy dog and runs away from here today and on")
	var b strings.Builder
	b.WriteString("BT /F1 12 Tf 14 TL 72 700 Td")
	for i, w := range words[:3*n] {
		if i > 0 && i%n == 0 {
			b.WriteString(" T*")
		} else if i > 0 {
			b.WriteString(" ( ) Tj")
		}
		fmt.Fprintf(&b, " (%s) Tj", w)
	}
	b.WriteString(" ET")
	return b.String()
}

// TestASpaceDrawnAsItsOwnShowGoesWithTheParagraph — `PLAN-text-reflow.md` P08.S02, /pending 732: a producer that shows
// every space as its own `( ) Tj` was refused as `mixed-content` at every N, because grouping keeps no blank run in a line
// and its show read as foreign. Its spaces are the paragraph's: the edit is accepted, reads back, and leaves no blank show
// behind — at several words to a line.
func TestASpaceDrawnAsItsOwnShowGoesWithTheParagraph(t *testing.T) {
	for _, n := range []int{2, 3, 5} {
		t.Run(fmt.Sprintf("%d words a line", n), func(t *testing.T) {
			content := spacedShows(n)
			if !strings.Contains(content, "( ) Tj") {
				t.Fatal("setup: no space is drawn as its own show")
			}
			pdf := helveticaPage(content)
			l, _ := layoutOf(t, pdf)
			old := l.paragraphs[0].text()
			edit := strings.Replace(old, "quick", "slow", 1)
			out, cause := reflowed(t, pdf, 0, edit)
			if cause != "" {
				t.Fatalf("refused (%s): %q", cause, old)
			}
			l, _ = layoutOf(t, out)
			if got := normalizedText(l.paragraphs[0].text()); got != normalizedText(edit) {
				t.Errorf("reads back %q, want %q", got, edit)
			}
			if bytes.Contains(pageContentOf(t, out, 1), []byte("( ) Tj")) {
				t.Error("a blank show was left behind: the old spaces are still drawn")
			}
		})
	}
}

// TestATrailingBlankShowEndsTheParagraph — Acrobat ends a paragraph on a space shown on its own, straight after the last
// word: left in place it is a show drawn where the deleted text ended (`inline-follower`, 89 corpus paragraphs). It is the
// paragraph's, also across a text-state operator; a blank after a REPOSITIONING belongs to whatever is drawn there.
func TestATrailingBlankShowEndsTheParagraph(t *testing.T) {
	edit := "The quick brown fox leaps over the lazy dog"
	for _, c := range []struct {
		name, tail string
		deleted    bool
	}{
		{"straight after", " ( ) Tj ET", true},
		{"across a Tc", " 0.5 Tc ( ) Tj ET", true},
		{"after a repositioning, on its own baseline", " 10 0 Td ( ) Tj ET", false},
		// `"` sets the word and character spacing the text after it is drawn in (the review's W1: deleted, the line below
		// lost its spacing).
		{"a trailing \" sets spacing", " 0 TL 5 1 ( ) \" ET", false}, // TL 0: the " stays on the last baseline
	} {
		t.Run(c.name, func(t *testing.T) {
			content := strings.TrimSuffix(twoLines, " ET") + c.tail
			out, cause := reflowed(t, helveticaPage(content), 0, edit)
			if cause != "" {
				t.Fatalf("refused (%s)", cause)
			}
			if got := bytes.Contains(pageContentOf(t, out, 1), []byte("( )")); got == c.deleted {
				t.Errorf("the trailing blank show is still drawn: %v, want %v", got, !c.deleted)
			}
		})
	}
}

// TestAGlyphThatReadsAsNothingIsNotASpace — a run grouping drops because it decodes to NO text (a ZapfDingbats check mark
// with no /ToUnicode, an undefined code) is ink, not a space: never deleted with the paragraph, and the stretch it sits in
// still refuses (the review's C1: it was silently cut from the page).
func TestAGlyphThatReadsAsNothingIsNotASpace(t *testing.T) {
	content := "BT /F1 12 Tf 14 TL 72 700 Td (The quick brown) Tj /F2 12 Tf (4) Tj /F1 12 Tf ( fox jumps) Tj T* (over the lazy dog) Tj ET"
	pdf := pageWith(content, "", "", "", map[string]string{"F1": helvetica["F1"],
		"F2": "<< /Type /Font /Subtype /Type1 /BaseFont /ZapfDingbats >>"})
	l, _ := layoutOf(t, pdf)
	mark := false
	for _, r := range paragraphRunsWithBlanks(l, 0) {
		mark = mark || (r.font == "F2" && !joinsALine(r))
	}
	if !mark {
		t.Fatal("setup: the check mark is not a run grouping drops")
	}
	out, cause := reflowed(t, pdf, 0, "The quick brown fox leaps over the lazy dog")
	if cause == "" && !bytes.Contains(pageContentOf(t, out, 1), []byte("(4) Tj")) {
		t.Fatal("the check mark was deleted with the paragraph")
	}
	if cause == "" {
		t.Errorf("the edit was accepted around a mark it cannot re-set")
	}
}

// TestABlankOutsideTheParagraphIsNotItsToDelete — a blank show drawn BEFORE the paragraph on its first baseline (in its
// own text object) is not between the paragraph's shows, so it is neither deleted nor a reason to refuse; and a blank show
// inside a FORM drawn on the paragraph's baseline — whose offsets index the form's stream, here inside the paragraph's own
// range of the page's — is never taken for one of the page's.
func TestABlankOutsideTheParagraphIsNotItsToDelete(t *testing.T) {
	t.Run("before it", func(t *testing.T) {
		pdf := helveticaPage("BT /F1 12 Tf 60 700 Td ( ) Tj ET " + twoLines)
		out, cause := reflowed(t, pdf, 0, "The quick brown fox leaps over the lazy dog")
		if cause != "" {
			t.Fatalf("refused (%s)", cause)
		}
		if !bytes.Contains(pageContentOf(t, out, 1), []byte("60 700 Td ( ) Tj")) {
			t.Error("the blank show before the paragraph was deleted with it")
		}
	})
	t.Run("in a form", func(t *testing.T) {
		form := "BT /F1 12 Tf 1 0 0 1 100 700 Tm ( ) Tj ET"
		content := twoLines + " q /Fm1 Do Q"
		if i := strings.Index(form, "( ) Tj"); i < strings.Index(content, "(The") || i > strings.Index(content, "dog) Tj") {
			t.Fatalf("setup: the form's blank show at %d is not inside the paragraph's range of the page", i)
		}
		pdf := pageWith(content, "/XObject << /Fm1 6 0 R >>", "", "", helvetica,
			fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Length %d >>\nstream\n%s\nendstream", len(form), form))
		l, _ := layoutOf(t, pdf)
		blank := false
		for _, r := range paragraphRunsWithBlanks(l, 0) {
			blank = blank || r.inForm
		}
		if !blank {
			t.Fatal("setup: the form's blank show is not read as near the paragraph")
		}
		out, cause := reflowed(t, pdf, 0, "The quick brown fox leaps over the lazy dog")
		if cause != "" {
			t.Fatalf("refused (%s)", cause)
		}
		l, _ = layoutOf(t, out)
		if got := normalizedText(l.paragraphs[0].text()); got != "The quick brown fox leaps over the lazy dog" {
			t.Errorf("reads back %q — page bytes were cut at a form's offsets", got)
		}
	})
}

// TestARefusalBetweenTheLinesNamesWhatIsThere — what still stands between a paragraph's lines is named for what it is,
// over the whole stretch rather than by the first operator met: lines each in their own text object are `text-objects`
// (a producer's way of drawing, not "a colour change or a drawing"); tagged line by line — which closes a text object per
// line too — is `tagged`; a colour change is `mixed-content` even when the lines also change text objects (/pending 732).
func TestARefusalBetweenTheLinesNamesWhatIsThere(t *testing.T) {
	edit := "The quick brown fox leaps over the lazy dog"
	for _, c := range []struct{ name, content, want string }{
		{"a text object per line", "BT /F1 12 Tf 72 700 Td (The quick brown fox jumps) Tj ET BT /F1 12 Tf 72 686 Td (over the lazy dog) Tj ET", causeTextObjects},
		{"tagged line by line", "BT /F1 12 Tf 72 700 Td (The quick brown fox jumps) Tj ET /P <</MCID 0>> BDC BT /F1 12 Tf 72 686 Td (over the lazy dog) Tj ET EMC", causeTagged},
		// Nothing after the last show — the stream ends on it — so the verdict on the stretch is given at the end.
		{"a text object per line, ending on its last show", "BT /F1 12 Tf 72 700 Td (The quick brown fox jumps) Tj ET BT /F1 12 Tf 72 686 Td (over the lazy dog) Tj", causeTextObjects},
		{"tagged and a colour change", "BT /F1 12 Tf 14 TL 72 700 Td /P <</MCID 0>> BDC (The quick brown fox jumps) Tj EMC 1 0 0 rg T* (over the lazy dog) Tj ET", causeTagged},
		{"a colour change and a text object", "BT /F1 12 Tf 72 700 Td (The quick brown fox jumps) Tj ET 1 0 0 rg BT /F1 12 Tf 72 686 Td (over the lazy dog) Tj ET", causeMixedContent},
	} {
		t.Run(c.name, func(t *testing.T) {
			pdf := helveticaPage(c.content)
			l, _ := layoutOf(t, pdf)
			if len(l.paragraphs) == 0 || normalizedText(l.paragraphs[0].text()) != "The quick brown fox jumps over the lazy dog" {
				t.Fatalf("setup: the two lines do not read as one paragraph")
			}
			if _, cause := reflowed(t, pdf, 0, edit); cause != c.want {
				t.Errorf("fell back on %q, want %q", cause, c.want)
			}
		})
	}
}

// toUnicodeOne is a ToUnicode CMap stream mapping one single-byte code to one UTF-16 value.
func toUnicodeOne(code, uni string) string {
	b := "/CIDInit /ProcSet findresource begin 12 dict begin begincmap /CMapName /P def 1 begincodespacerange <00> <FF> endcodespacerange 1 beginbfchar <" + code + "> <" + uni + "> endbfchar endcmap CMapName currentdict /CMap defineresource pop end end"
	return fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(b), b)
}

// TestAShowThatOnlyReadsAsASpaceIsKept — reading as white space is no promise of a blank glyph: a check mark whose
// ToUnicode says U+0020, an X that says U+00A0, are ink, and a space drawn in a clipping mode cuts everything after it.
// None is deleted with the paragraph, between its words or after its last (the review's round 2, W1 and W2; HEAD refused
// each). Each stays on the page or the edit refuses.
func TestAShowThatOnlyReadsAsASpaceIsKept(t *testing.T) {
	edit := "The quick brown fox leaps over the lazy dog"
	for _, c := range []struct {
		name, content, show string
		f2, cmap            string
	}{
		{"a check mark read as U+0020", "BT /F1 12 Tf 14 TL 72 700 Td (The quick brown) Tj /F2 12 Tf (4) Tj /F1 12 Tf ( fox jumps) Tj T* (over the lazy dog) Tj ET", "(4) Tj",
			"<< /Type /Font /Subtype /Type1 /BaseFont /ZapfDingbats /ToUnicode 6 0 R >>", toUnicodeOne("34", "0020")},
		{"an X read as U+00A0", "BT /F1 12 Tf 14 TL 72 700 Td (The quick brown) Tj /F2 12 Tf (X) Tj /F1 12 Tf ( fox jumps) Tj T* (over the lazy dog) Tj ET", "(X) Tj",
			"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding /ToUnicode 6 0 R >>", toUnicodeOne("58", "00A0")},
		{"a clipping space between the words", "BT /F1 12 Tf 14 TL 72 700 Td (The quick brown) Tj 7 Tr ( ) Tj 0 Tr (fox jumps) Tj T* (over the lazy dog) Tj ET 0 0 1 rg 0 0 612 792 re f", "7 Tr ( ) Tj", "", ""},
		{"a clipping space after the last word", "BT /F1 12 Tf 14 TL 72 700 Td (The quick brown fox jumps) Tj T* (over the lazy dog) Tj 7 Tr ( ) Tj ET 0 0 1 rg 0 0 612 792 re f", "7 Tr ( ) Tj", "", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			fonts := map[string]string{"F1": helvetica["F1"]}
			var extra []string
			if c.f2 != "" {
				fonts["F2"] = c.f2
				extra = append(extra, c.cmap)
			}
			pdf := pageWith(c.content, "", "", "", fonts, extra...)
			l, _ := layoutOf(t, pdf)
			loose := false
			for _, r := range paragraphRunsWithBlanks(l, 0) {
				loose = loose || (!joinsALine(r) && r.decoded && strings.TrimSpace(r.text) == "" && r.text != "")
			}
			if !loose {
				t.Fatal("setup: no run near the paragraph reads as white space")
			}
			out, cause := reflowed(t, pdf, 0, edit)
			if cause == "" && !bytes.Contains(pageContentOf(t, out, 1), []byte(c.show)) {
				t.Errorf("%s was deleted with the paragraph", c.show)
			}
		})
	}
}
