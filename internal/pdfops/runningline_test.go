package pdfops

import (
	"fmt"
	"testing"
)

// Running headers and footers are not columns — /pending 787.

// runningBody is n lines of a body column 190 points wide at x 72, 14 points apart from y 700.
func runningBody(n int) []textRun {
	var out []textRun
	for k := 0; k < n; k++ {
		out = append(out, run(fmt.Sprintf("Body line %02d words run on and on", k), 72, 700-14*float64(k), 190, 12))
	}
	return out
}

// TestARunningLineIsNotAColumn — a page number or running header set apart from one body column, and narrower than it,
// is not counted as a column; it keeps a cluster of its own, reads where it sits (a header first, a footer last), and
// is marked running. Each condition of the rule is held separately by a case it alone decides.
func TestARunningLineIsNotAColumn(t *testing.T) {
	footer := run("Page 1 of 2", 290, 30, 50, 9)
	header := run("Confidential", 420, 760, 60, 9)
	cases := []struct {
		name    string
		runs    []textRun
		columns int
		first   string // the first paragraph's text, when it matters
		last    string // the last paragraph's text, when it matters
		running string // the text of the one running paragraph, or ""
	}{
		{"a centred page number below the body", append(runningBody(6), footer), 1, "", "Page 1 of 2", "Page 1 of 2"},
		{"a page number at y 60", append(runningBody(6), run("1", 300, 60, 6, 9)), 1, "", "1", "1"},
		{"a running header above the body reads first", append(runningBody(6), header), 1, "Confidential", "", "Confidential"},
		{"a header and a footer", append(append(runningBody(6), header), footer), 1, "Confidential", "Page 1 of 2", ""},
		// Each condition alone keeps the second column.
		{"two columns side by side", append(runningBody(6), run("A right column line", 300, 686, 190, 12),
			run("Another right column line", 300, 672, 190, 12)), 2, "", "", ""},
		{"three lines are a column, not a footer", append(runningBody(6), run("a", 290, 60, 50, 9), run("b", 290, 50, 50, 9),
			run("c", 290, 40, 50, 9)), 2, "", "", ""},
		{"as wide as the body is a column", append(runningBody(6), run("A footer as wide as the body", 290, 30, 190, 9)), 2, "", "", ""},
		{"within an em of the body's last baseline", append(runningBody(6), run("Beside", 290, 700-14*5-8, 50, 9)), 2, "", "", ""},
		{"within an em of the body's first baseline", append(runningBody(6), run("Beside", 420, 700+8, 50, 9)), 2, "", "", ""},
		{"two bodies of equal length", append(runningBody(1), run("Other", 290, 30, 50, 9)), 2, "", "", ""},
	}
	for _, c := range cases {
		l := groupRuns(c.runs)
		if l.columns != c.columns {
			t.Errorf("%s: columns=%d, want %d (%q)", c.name, l.columns, c.columns, paragraphTexts(l))
			continue
		}
		if c.first != "" && l.paragraphs[0].text() != c.first {
			t.Errorf("%s: reads first %q, want %q", c.name, l.paragraphs[0].text(), c.first)
		}
		if c.last != "" && l.paragraphs[len(l.paragraphs)-1].text() != c.last {
			t.Errorf("%s: reads last %q, want %q", c.name, l.paragraphs[len(l.paragraphs)-1].text(), c.last)
		}
		runningN, bodyCols := 0, map[int]bool{}
		for _, p := range l.paragraphs {
			if p.running {
				runningN++
				if c.running != "" && p.text() != c.running {
					t.Errorf("%s: %q is running, want only %q", c.name, p.text(), c.running)
				}
			} else {
				bodyCols[p.column] = true
			}
		}
		if c.columns == 1 && len(bodyCols) != 1 {
			t.Errorf("%s: body paragraphs span clusters %v", c.name, bodyCols)
		}
		if c.columns != 1 && runningN != 0 {
			t.Errorf("%s: a page that keeps its columns marked %d paragraph(s) running", c.name, runningN)
		}
		// The body's paragraphs are read from the body alone: the cascade's lines are one paragraph each pair or the
		// body's six lines one paragraph — never a paragraph per line because a page number reached past the right edge.
		if c.columns == 1 {
			body := 0
			for _, p := range l.paragraphs {
				if !p.running {
					body++
				}
			}
			if body != 1 {
				t.Errorf("%s: the body read as %d paragraphs, want 1 (%q)", c.name, body, paragraphTexts(l))
			}
		}
	}
}

// TestAGrowthFlowsPastAPageNumber — the finding as reported: a page number drawn apart from the body made the page read
// as two columns and a growth that needed the next page refused `page-full`. It flows, and the page numbers stay where
// they are on both pages. Page 1's body still keeps an em clear of the page number: its last line stays above y 81.
func TestAGrowthFlowsPastAPageNumber(t *testing.T) {
	foot := func(p int) string { return fmt.Sprintf("BT /F1 9 Tf 290 30 Td (Page %d of 2) Tj ET\n", p) }
	for _, c := range []struct {
		name   string
		p1, p2 string
		f1, f2 string
	}{
		{"a centred footer on both pages", cascadePage(1, 16, foot(1)), cascadePage(2, 3, foot(2)), "Page 1 of 2", "Page 2 of 2"},
		{"a page number at y 60", cascadePage(1, 16, "BT /F1 9 Tf 290 60 Td (1) Tj ET\n"), cascadePage(2, 3, ""), "1", ""},
		// The same page number under the body, so in its column: an em above it reaches a point past the bottom margin's
		// edge, and that made it content below the region rather than a footer inside the margin.
		{"a page number at y 60 under the body", cascadePage(1, 16, "BT /F1 9 Tf 100 60 Td (1) Tj ET\n"), cascadePage(2, 3, ""), "1", ""},
	} {
		pdf := cascadeDoc([]string{c.p1, c.p2}, nil, "")
		orig, edit := threeLinesMore()
		out, refusal, err := ReflowParagraph(pdf, 1, 0, orig, edit)
		if err != nil || out == nil {
			t.Errorf("%s: refused %+v (%v)", c.name, refusal, err)
			continue
		}
		p1, p2 := lineYs(t, out, 1), lineYs(t, out, 2)
		for text, ys := range p1 {
			if text != c.f1 && ys[len(ys)-1] < 81 {
				t.Errorf("%s: %q drawn at %v, inside the em above the page number", c.name, text, ys)
			}
		}
		if got := p2[cascadeLine(1, 15)]; len(got) != 2 || !near6(got[0], 700) {
			t.Errorf("%s: page 1's last paragraph is at %v on page 2, want its first baseline", c.name, got)
		}
		for _, f := range []struct {
			page map[string][]float64
			text string
		}{{p1, c.f1}, {p2, c.f2}} {
			if f.text == "" {
				continue
			}
			want := 30.0
			if f.text == "1" {
				want = 60
			}
			if got := f.page[f.text]; len(got) != 1 || !near6(got[0], want) {
				t.Errorf("%s: %q at %v, want %v", c.name, f.text, got, want)
			}
		}
	}
}

// TestARunningFooterAboveTheMarginBoundsTheGrowth — a page number beside the column and above the bottom margin is not in
// the column, but the page's width is what a single-column page's band covers: it bounds the floor, as a footer in the
// column does, so a growth with room above it is made and the page number stays — not refused as drawn in the band, which
// a page read as two columns never was.
func TestARunningFooterAboveTheMarginBoundsTheGrowth(t *testing.T) {
	pdf := cascadeDoc([]string{cascadePage(1, 14, "BT /F1 9 Tf 290 100 Td (Page 1) Tj ET\n")}, nil, "")
	orig, edit := threeLinesMore()
	out, refusal, err := ReflowParagraph(pdf, 1, 0, orig, edit)
	if err != nil || out == nil {
		t.Fatalf("refused %+v (%v)", refusal, err)
	}
	p1 := lineYs(t, out, 1)
	if got := p1["Page 1"]; len(got) != 1 || !near6(got[0], 100) {
		t.Errorf("the page number is at %v, want 100", got)
	}
	if got := p1[cascadeLine(1, 13)]; len(got) != 2 || !near6(got[0], 700-38*13-42) {
		t.Errorf("the last paragraph is at %v, want pushed down by the growth", got)
	}
	// And a growth past it refuses: the footer is content below the region that stays.
	long := orig
	for i := 0; i < 12; i++ {
		long += " " + cascadeLine(1, 0)
	}
	if out, refusal, err := ReflowParagraph(pdf, 1, 0, orig, long); err != nil || out != nil || refusal.Cause != causePageFull {
		t.Errorf("a growth past the page number: out=%v refusal=%+v err=%v, want page-full", out != nil, refusal, err)
	}
}

// TestAFlowDoesNotLandOnARunningHeaderOfItsOwn — a running header set apart from the next page's body is that page's
// first paragraph in a cluster of its own, so the region it opens is bounded by the margin: landing there would put the
// block in the header's place. It refuses, as a header in the body's column does.
func TestAFlowDoesNotLandOnARunningHeaderOfItsOwn(t *testing.T) {
	head := "BT /F1 9 Tf 420 760 Td (Confidential) Tj ET\n"
	pdf := cascadeDoc([]string{cascadePage(1, 16, ""), head + cascadePage(2, 3, "")}, nil, "")
	orig, edit := threeLinesMore()
	out, refusal, err := ReflowParagraph(pdf, 1, 0, orig, edit)
	if err != nil || out != nil || refusal.Cause != causePageFull {
		t.Fatalf("landed on a running header: out=%v refusal=%+v err=%v", out != nil, refusal, err)
	}
}
