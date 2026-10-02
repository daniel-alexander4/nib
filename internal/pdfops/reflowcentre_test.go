package pdfops

import (
	"bytes"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
	"nib/mdpdf"
)

// centredPage draws heading at size 18, centred on axis, above a left-aligned body paragraph at 72 that fills the column
// to about 540 — the column a heading is read against.
func centredPage(heading string, axis float64) string {
	x := axis - mdpdf.CoreWidth(heading, "Helvetica", 18)/2
	return fmt.Sprintf("BT /F1 18 Tf 1 0 0 1 %s 720 Tm (%s) Tj ET ", num(x), heading) + centredBody
}

const centredBody = "BT /F1 12 Tf 14 TL 72 680 Td (The body of the page runs from the left margin across the whole of the column, and) Tj" +
	" T* (wraps here, as a body paragraph does, until it ends on a shorter last line of its own.) Tj ET"

// midsOf reads the paragraph whose text starts with prefix and returns each line's middle.
func midsOf(t *testing.T, pdf []byte, prefix string) []float64 {
	t.Helper()
	l, _ := layoutOf(t, pdf)
	for _, p := range l.paragraphs {
		if len(p.text()) >= len(prefix) && p.text()[:len(prefix)] == prefix {
			lines, _, cause := paragraphWords(p)
			if cause != "" {
				t.Fatalf("%q cannot be read (%s)", prefix, cause)
			}
			var out []float64
			for _, ln := range lines {
				e := ln[len(ln)-1]
				out = append(out, (ln[0].startX+e.startX+e.width)/2)
			}
			return out
		}
	}
	t.Fatalf("no paragraph begins %q", prefix)
	return nil
}

// TestACentredHeadingStaysCentred — `PLAN-text-reflow.md` P08.S04: a heading centred on its page is re-set centred on the
// same axis when edited shorter and longer. It used to be re-set from its old left edge, so any change of length left it
// off-centre, silently. (Centred lines with different starts are grouped as a paragraph EACH, so a centred paragraph is a
// line; one that would need a second line refuses `centred-grows` — it would read back as two paragraphs, the first not
// centred: the P08 phase-close review, C1.)
func TestACentredHeadingStaysCentred(t *testing.T) {
	pdf := helveticaPage(centredPage("Annual Report", 306))
	l, _ := layoutOf(t, pdf)
	lines, _, _ := paragraphWords(l.paragraphs[0])
	if a := paragraphAlignment(l, 0, lines); !a.centred || math.Abs(a.axis-306) > 1e-6 {
		t.Fatalf("setup: the heading reads as %+v, want centred on 306", a)
	}
	// Each is re-set centred, READS BACK centred on the same axis, and a second edit of the result keeps it there (the
	// P08 phase-close review, C1: a line written wider than the door reads as centred lost its axis on the next edit).
	for _, edit := range []string{"Short Report", "The Annual Report for the Year"} {
		out, cause := reflowed(t, pdf, 0, edit)
		if cause != "" {
			t.Fatalf("%q refused (%s)", edit, cause)
		}
		mids := midsOf(t, out, edit[:6])
		for i, m := range mids {
			if math.Abs(m-306) > 1e-6 {
				t.Errorf("%q: line %d centred on %v, not 306", edit, i, m)
			}
		}
		l2, _ := layoutOf(t, out)
		lines2, _, _ := paragraphWords(l2.paragraphs[0])
		if a := paragraphAlignment(l2, 0, lines2); !a.centred || math.Abs(a.axis-306) > 1e-6 {
			t.Errorf("%q re-set reads as %+v, want centred on 306", edit, a)
		}
		again, cause := reflowed(t, out, 0, "Annual Report")
		if cause != "" {
			t.Fatalf("%q then \"Annual Report\" refused (%s)", edit, cause)
		}
		if m := midsOf(t, again, "Annual"); math.Abs(m[0]-306) > 1e-6 {
			t.Errorf("%q then \"Annual Report\": centred on %v, not 306", edit, m[0])
		}
	}
	// Wider than the door reads as centred (70% of the column) it would not read back centred, and as two lines it would
	// read back as two paragraphs: refused, named.
	if _, cause := reflowed(t, pdf, 0, "The Annual Report of the Board for the Year 2025"); cause != causeCentredGrows {
		t.Errorf("a centred line wider than the door reads: cause %q, want %q", cause, causeCentredGrows)
	}
}

// TestALineThatIsNotCentredIsNotReadAsCentred — the door's evidence: a heading at the left margin, a line wider than 70%
// of its column whose middle happens to fall on the axis, and — on a page of two columns — a line on the page's centre,
// which is a gutter there, are none of them centred.
func TestALineThatIsNotCentredIsNotReadAsCentred(t *testing.T) {
	read := func(content string) alignment {
		l, _ := layoutOf(t, helveticaPage(content))
		lines, _, _ := paragraphWords(l.paragraphs[0])
		return paragraphAlignment(l, 0, lines)
	}
	if a := read("BT /F1 18 Tf 1 0 0 1 72 720 Tm (Annual Report) Tj ET " + centredBody); a.centred {
		t.Error("a heading at the left margin read as centred")
	}
	wide := "A heading that runs across most of a column" // ~370 of 468: centred, but too wide to be read as such
	if a := read(centredPage(wide, 306)); a.centred {
		t.Errorf("a line %v wide in a 468 column read as centred", mdpdf.CoreWidth(wide, "Helvetica", 18))
	}
	l := pageLayout{columns: 2, box: [4]float64{0, 0, 612, 792}, paragraphs: []textParagraph{{lines: []textLine{{size: 12, x0: 72, x1: 300}}}}} // the left column
	if a := centredAlignment(l, 0, []float64{276}, []float64{336}); a.centred {
		t.Error("on a page of two columns, a line on the page's centre read as centred")
	}
}

// TestACentredLabelNeverRunsUnderAFieldBesideIt — a centred line's room is both sides of its axis, and short of anything
// drawn beside it on either side: a field to its right or to its left (the review of P08.S04: a longer label was drawn
// under the field — P07's phase-close C3 shape, re-opened by a symmetric measure). And the line is MOVED, so the page's
// geometry reads it where its ink is, which is what lets a second edit see the field.
func TestACentredLabelNeverRunsUnderAFieldBesideIt(t *testing.T) {
	label := "Your name"
	x := 306 - mdpdf.CoreWidth(label, "Helvetica", 12)/2
	content := fmt.Sprintf("BT /F1 12 Tf 1 0 0 1 %s 720 Tm (%s) Tj ET ", num(x), label) + centredBody
	for _, field := range []struct {
		name string
		rect [4]float64
	}{{"right", [4]float64{360, 712, 520, 732}}, {"left", [4]float64{100, 712, 250, 732}}} {
		t.Run(field.name, func(t *testing.T) {
			widget := fmt.Sprintf("<< /Type /Annot /Subtype /Widget /FT /Tx /T (f) /Rect [%v %v %v %v] /P 3 0 R >>",
				field.rect[0], field.rect[1], field.rect[2], field.rect[3])
			pdf := pageWith(content, "", "", "/Annots [6 0 R]", helvetica, widget)
			l, _ := layoutOf(t, pdf)
			lines, _, _ := paragraphWords(l.paragraphs[0])
			a := paragraphAlignment(l, 0, lines)
			if !a.centred {
				t.Fatalf("setup: the label reads as %+v, want centred", a)
			}
			if a.axis-a.width/2 < 72-1e-9 || a.axis+a.width/2 > 520 {
				t.Fatalf("setup: a centred width of %v about %v", a.width, a.axis)
			}
			for _, edit := range []string{"Your full name", "Your full legal name as it appears on your passport"} {
				out, cause := reflowed(t, pdf, 0, edit)
				if cause != "" {
					continue // refused: nothing was drawn under the field
				}
				l, _ := layoutOf(t, out)
				for _, p := range l.paragraphs {
					for _, ln := range p.lines {
						if ln.y > 700 && ln.x1 > field.rect[0] && ln.x0 < field.rect[2] {
							t.Errorf("%q is drawn [%v..%v], under the field [%v..%v]", edit, ln.x0, ln.x1, field.rect[0], field.rect[2])
						}
					}
				}
			}
		})
	}
	// The line's origin moves with its ink: re-set longer, the page reads its start where the ink starts.
	pdf := helveticaPage(content)
	out, cause := reflowed(t, pdf, 0, "Your full name")
	if cause != "" {
		t.Fatalf("refused (%s)", cause)
	}
	l, _ := layoutOf(t, out)
	lines, _, _ := paragraphWords(l.paragraphs[0])
	if ink, origin := lines[0][0].startX, l.paragraphs[0].lines[0].x0; math.Abs(ink-origin) > 1e-6 {
		t.Errorf("the re-set label's ink begins at %v and the page reads its line from %v", ink, origin)
	}
}

// TestCentredLinesOverTheCorpus — S04 over the real-producer corpus: every paragraph the door reads as centred, edited
// (its last word dropped) and accepted, reads back as the edit on its baseline, centred on its axis within 0.05 em, and
// as its own text alone — not merged into a neighbouring cell. Refusals are counted by cause. **The stimulus is
// asserted**: centred lines must have been re-set. Skips without the corpus.
func TestCentredLinesOverTheCorpus(t *testing.T) {
	corp := externalPDFs(t, "real producers", "NIB_UA_PRODUCERS", filepath.Join(homeDir(), "nib", "producers"))
	if corp.absent != "" {
		t.Skip("SKIP (not a measurement): " + corp.absent)
	}
	read, reset := 0, 0
	refused := map[string]int{}
	for _, doc := range corp.docs {
		ctx, err := pdfread.Validated(doc.pdf, model.NewDefaultConfiguration())
		if err != nil {
			continue
		}
		for p := 1; p <= ctx.PageCount; p++ {
			l, err := readPageGlyphLayout(ctx, pageAt(ctx, nil, p))
			if err != nil {
				continue
			}
			for pi, para := range l.paragraphs {
				orig, _, cause := paragraphWords(para)
				if cause != "" {
					continue
				}
				a := paragraphAlignment(l, pi, orig)
				if !a.centred {
					continue
				}
				read++
				ws := editWords(para.text())
				if len(ws) < 3 {
					continue
				}
				edit := strings.Join(ws[:len(ws)-1], " ")
				c2, _ := pdfread.Validated(doc.pdf, model.NewDefaultConfiguration())
				out, err := reflowParagraph(c2, p, pi, edit)
				if err != nil {
					t.Fatalf("%s p%d ¶%d: %v", doc.name, p, pi, err)
				}
				if out.content == nil {
					refused[out.cause]++
					continue
				}
				d, _, _, _ := c2.PageDict(p, false)
				if err := setPageContent(c2, d, out.content); err != nil {
					t.Fatal(err)
				}
				var buf bytes.Buffer
				if err := api.WriteContext(c2, &buf); err != nil {
					t.Fatal(err)
				}
				c3, err := pdfread.Validated(buf.Bytes(), model.NewDefaultConfiguration())
				if err != nil {
					t.Fatalf("%s p%d ¶%d: the rewritten document does not read: %v", doc.name, p, pi, err)
				}
				l3, _ := readPageGlyphLayout(c3, pageAt(c3, nil, p))
				reset++
				ln, ok := lineAtBaseline(l3, paragraphBox(para), para.lines[0].y, para.lines[0].size)
				if !ok {
					t.Errorf("%s p%d ¶%d %q: nothing on its baseline", doc.name, p, pi, para.text())
					continue
				}
				if !readsAsEdit(ln.text, edit) {
					t.Errorf("%s p%d ¶%d: reads back %q, want %q", doc.name, p, pi, ln.text, edit)
					continue
				}
				if mid := (ln.x0 + ln.x1) / 2; math.Abs(mid-a.axis) > justifyTol*para.lines[0].size {
					t.Errorf("%s p%d ¶%d %q: centred on %v, the axis is %v", doc.name, p, pi, edit, mid, a.axis)
				}
				// And it READS BACK centred, so the next edit keeps the axis (the P08 phase-close review, C1).
				for qi, q := range l3.paragraphs {
					if len(q.lines) == 1 && q.lines[0].y == ln.y && q.lines[0].x0 == ln.x0 {
						w3, _, cause := paragraphWords(q)
						if a3 := paragraphAlignment(l3, qi, w3); cause == "" && !a3.centred {
							t.Errorf("%s p%d ¶%d %q: re-set centred, reads back not centred", doc.name, p, pi, edit)
						}
					}
				}
			}
		}
	}
	if reset == 0 {
		t.Fatal("no centred line was re-set — the centring was never exercised")
	}
	t.Logf("%d paragraphs read as centred, %d re-set and read back centred; refused by cause %v", read, reset, refused)
}

// TestALineOnASharedLeftEdgeIsNotCentred — a line that begins where two other lines of its column begin is set from that
// edge, whatever its middle: on a three-column page grouped as one column, a body line's middle fell on the column's
// centre and an edit moved it off the left edge eight lines share (the review of P08.S04). On the door directly.
func TestALineOnASharedLeftEdgeIsNotCentred(t *testing.T) {
	line := func(x0, x1 float64) textLine { return textLine{size: 12, x0: x0, x1: x1, y: 700} }
	l := pageLayout{columns: 1, box: [4]float64{0, 0, 612, 792}, paragraphs: []textParagraph{
		{lines: []textLine{line(227.8, 384.2)}},
		{lines: []textLine{line(72, 540)}},
		{lines: []textLine{line(227.8, 500), line(227.8, 498)}},
	}}
	if a := centredAlignment(l, 0, []float64{227.8}, []float64{384.2}); a.centred {
		t.Error("a line on a left edge two others share read as centred")
	}
	l.paragraphs[2].lines[1].x0 = 240 // one other: no edge
	if a := centredAlignment(l, 0, []float64{227.8}, []float64{384.2}); !a.centred {
		t.Error("the control — one other line on that edge — did not read as centred")
	}
}
