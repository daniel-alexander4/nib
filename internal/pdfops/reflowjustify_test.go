package pdfops

import (
	"bytes"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
	"nib/mdpdf"
)

// justifiedPara sets lines of Helvetica 12 from x 72 to a flush edge at 400 — the way InDesign does it (`Tw` per line,
// the words in one show) when byTJ is false, the way Antenna House does it (a `TJ` adjustment after each space) when true —
// and the last line at its natural spaces.
func justifiedPara(byTJ bool, lines ...string) string {
	const left, edge, size = 72.0, 400.0, 12
	var b strings.Builder
	fmt.Fprintf(&b, "BT /F1 %d Tf 14 TL %v 700 Td", size, left)
	for i, ln := range lines {
		if i > 0 {
			b.WriteString(" T*")
		}
		words := strings.Fields(ln)
		extra := 0.0
		if i < len(lines)-1 {
			extra = (edge - left - mdpdf.CoreWidth(ln, "Helvetica", size)) / float64(len(words)-1)
		}
		switch {
		case !byTJ:
			fmt.Fprintf(&b, " %s Tw (%s) Tj", num(extra), ln)
		default:
			b.WriteString(" [")
			for wi, w := range words {
				if wi > 0 {
					fmt.Fprintf(&b, "( ) %s ", num(-extra/size*1000))
				}
				fmt.Fprintf(&b, "(%s) ", w)
			}
			b.WriteString("] TJ")
		}
	}
	b.WriteString(" 0 Tw ET")
	return b.String()
}

var justifiedText = []string{
	"The quick brown fox jumps over the lazy dog and then",
	"runs far away from here across the open fields to a",
	"river where it stops to drink and rests in the shade",
	"of an old tree.",
}

// lineEnds reads paragraph 0 of pdf and returns each line's right end and its gaps.
func lineEnds(t *testing.T, pdf []byte) (ends []float64, gaps [][]float64, lines [][]reflowWord) {
	t.Helper()
	l, _ := layoutOf(t, pdf)
	lines, _, cause := paragraphWords(l.paragraphs[0])
	if cause != "" {
		t.Fatalf("cannot be read (%s)", cause)
	}
	for _, ln := range lines {
		end := ln[len(ln)-1]
		ends = append(ends, end.startX+end.width)
		var g []float64
		for i := 1; i < len(ln); i++ {
			g = append(g, ln[i].startX-ln[i-1].startX-ln[i-1].width)
		}
		gaps = append(gaps, g)
	}
	return ends, gaps, lines
}

// TestAJustifiedParagraphReWrapsJustified — `PLAN-text-reflow.md` P08.S03: a justified paragraph, set either way a producer
// sets one, re-wraps with every line but the last ending at its flush edge, and its last line at its natural spaces — it
// used to come out ragged, every space the paragraph's median. The edit moves words between lines, so no line keeps the
// spacing it was drawn with.
func TestAJustifiedParagraphReWrapsJustified(t *testing.T) {
	for _, byTJ := range []bool{false, true} {
		t.Run(map[bool]string{false: "by Tw", true: "by TJ"}[byTJ], func(t *testing.T) {
			pdf := helveticaPage(justifiedPara(byTJ, justifiedText...))
			ends, gaps, lines := lineEnds(t, pdf)
			l, _ := layoutOf(t, pdf)
			a := paragraphAlignment(l, 0, lines)
			if !a.justified || math.Abs(a.edge-400) > 1e-6 {
				t.Fatalf("setup: read as %+v, ends %v — want justified at 400", a, ends)
			}
			natural := gaps[len(gaps)-1][0]
			// Two edits: one drops three words from the first line, one drops most of it — so words set on stretched lines
			// land on the new last line, where their old stretch must not follow them (the review of S03, C3).
			for _, edit := range []string{
				strings.Replace(strings.Join(justifiedText, " "), "quick brown fox", "fox", 1),
				strings.Replace(strings.Join(justifiedText, " "), "The quick brown fox jumps over the lazy dog and then", "Then", 1),
			} {
				out, cause := reflowed(t, pdf, 0, edit)
				if cause != "" {
					t.Fatalf("refused (%s)", cause)
				}
				ends, gaps, lines := lineEnds(t, out)
				if len(lines) < 2 {
					t.Fatalf("re-wrapped into %d lines", len(lines))
				}
				for i := 0; i < len(lines)-1; i++ {
					if len(lines[i]) > 1 && math.Abs(ends[i]-400) > 1e-6 {
						t.Errorf("%q: line %d ends at %v, not the flush edge 400", edit[:20], i, ends[i])
					}
					for _, g := range gaps[i] {
						if math.Abs(g-gaps[i][0]) > 1e-6 {
							t.Errorf("%q: line %d's spaces are uneven %v — a word kept its old line's stretch", edit[:20], i, gaps[i])
							break
						}
					}
				}
				for _, g := range gaps[len(gaps)-1] {
					if math.Abs(g-natural) > 1e-6 {
						t.Errorf("%q: the last line's space is %v, want its natural %v", edit[:20], g, natural)
					}
				}
			}
		})
	}
}

// TestARaggedParagraphIsNotJustified — the door reads a ragged paragraph as not justified, and its re-set lines keep the
// spaces it drew: nothing is stretched to an edge it never had.
func TestARaggedParagraphIsNotJustified(t *testing.T) {
	pdf := helveticaPage(reflowPara + reflowTail)
	_, gaps, lines := lineEnds(t, pdf)
	l, _ := layoutOf(t, pdf)
	if a := paragraphAlignment(l, 0, lines); a.justified {
		t.Fatalf("a ragged paragraph read as justified at %v", a.edge)
	}
	drawn := gaps[0][0]
	out, cause := reflowed(t, pdf, 0, "The quick brown fox leaps over the lazy dog and runs away from here.")
	if cause != "" {
		t.Fatalf("refused (%s)", cause)
	}
	_, gaps, _ = lineEnds(t, out)
	for i, ln := range gaps {
		for _, g := range ln {
			if math.Abs(g-drawn) > 1e-6 {
				t.Errorf("line %d: a space of %v, want the drawn %v", i, g, drawn)
			}
		}
	}
}

// TestATableIsNotJustified — rows that end together by ONE wide gap each (a tab to a page number) are a table, not
// justified lines: justification stretches a line's spaces alike (the review of S03, C2).
func TestATableIsNotJustified(t *testing.T) {
	word := func(x, w float64) reflowWord {
		return reflowWord{startX: x, width: w, glyphs: []runGlyph{{text: "x"}}, spacing: []textSpacing{{th: 1}}}
	}
	l := pageLayout{paragraphs: []textParagraph{{lines: []textLine{{size: 12}}}}}
	row := func(a, b, num float64) []reflowWord {
		return []reflowWord{word(72, a), word(72+a+3.3, b), word(400-num, num)}
	}
	lines := [][]reflowWord{row(60, 40, 12), row(80, 30, 18), row(50, 70, 12), {word(72, 30)}}
	if a := paragraphAlignment(l, 0, lines); a.justified {
		t.Errorf("a table of rows ending at a page number read as justified at %v", a.edge)
	}
	// The control (the review's R2-C1): a justified line whose space after a sentence is set wider is still justified —
	// a gap after punctuation is the producer's sentence space, not a jump. 70 of the corpus's justified paragraphs.
	sentence := func(w []reflowWord, after int) []reflowWord {
		w[after].glyphs = []runGlyph{{text: "x"}, {text: "."}}
		return w
	}
	even := func(y float64) []reflowWord { // four words, gaps 5, then 9 after a full stop, then 5, ending at 400
		return []reflowWord{word(72, 80), word(157, 80), word(246, 70), word(321, 79)}
	}
	lines = [][]reflowWord{sentence(even(0), 1), sentence(even(1), 1), {word(72, 30), word(105, 30)}}
	if a := paragraphAlignment(l, 0, lines); !a.justified {
		t.Error("justified lines with a wider space after a full stop did not read as justified")
	}
}

// TestALastLineWithAWideGapKeepsItsNaturalSpace — the natural space a justified paragraph's last line is set at is that
// line's NARROWEST gap: a last line holding a wide gap (a label and its value) does not widen every space on the new last
// line (the review of S03, C2: they were written 271pt wide and the paragraph ran to thirteen lines).
func TestALastLineWithAWideGapKeepsItsNaturalSpace(t *testing.T) {
	text := append(append([]string(nil), justifiedText[:3]...), "an old tree.") // two gaps, one wide: a median would take the wide one
	pdf := helveticaPage(strings.Replace(justifiedPara(true, text...), "(old) ", "(old) -12000 ", 1))
	_, gaps, lines := lineEnds(t, pdf)
	last := gaps[len(gaps)-1]
	if len(last) < 2 || slices.Max(last) < slices.Min(last)+100 {
		t.Fatalf("setup: the last line's gaps %v hold no wide gap", last)
	}
	l, _ := layoutOf(t, pdf)
	if a := paragraphAlignment(l, 0, lines); !a.justified {
		t.Fatal("setup: not read as justified")
	}
	out, cause := reflowed(t, pdf, 0, strings.Replace(strings.Join(text, " "), "quick brown fox", "fox", 1))
	if cause != "" {
		t.Fatalf("refused (%s)", cause)
	}
	_, gaps, lines = lineEnds(t, out)
	if len(lines) > len(text)+1 {
		t.Errorf("re-wrapped into %d lines from %d", len(lines), len(text))
	}
	for _, g := range gaps[len(gaps)-1] {
		if g > slices.Min(last)+1e-6 {
			t.Errorf("a space of %v on the new last line, the natural one is %v", g, slices.Min(last))
		}
	}
}

// TestTwoLinesAreJustifiedOnlyOnEvidence — a paragraph of two lines can show only one line set: it reads as justified when
// that line ends at its column's right edge AND is set looser than the last, and not when it merely ends there.
func TestTwoLinesAreJustifiedOnlyOnEvidence(t *testing.T) {
	loose := helveticaPage(justifiedPara(false, justifiedText[0], "runs far away."))
	_, _, lines := lineEnds(t, loose)
	l, _ := layoutOf(t, loose)
	if a := paragraphAlignment(l, 0, lines); !a.justified {
		t.Error("a looser first line at the column's edge did not read as justified")
	}
	// A last line as long as the first is not a paragraph's short end, and a one-word line has no spaces to be loose.
	// Both lines end at the edge, the first looser (fewer words): only the "last line ends short" guard says no.
	twTo := func(ln string) string {
		return num((400 - 72 - mdpdf.CoreWidth(ln, "Helvetica", 12)) / float64(len(strings.Fields(ln))-1))
	}
	short := "The quick brown fox jumps"
	full := helveticaPage("BT /F1 12 Tf 14 TL 72 700 Td " + twTo(short) + " Tw (" + short + ") Tj T* " + twTo(justifiedText[0]) +
		" Tw (" + justifiedText[0] + ") Tj 0 Tw ET")
	_, _, lines = lineEnds(t, full)
	l, _ = layoutOf(t, full)
	if a := paragraphAlignment(l, 0, lines); a.justified {
		t.Error("two lines ending together read as justified on two lines' evidence")
	}
	tight := helveticaPage("BT /F1 12 Tf 14 TL 72 700 Td (The quick brown fox jumps over the lazy dog and then) Tj T* (runs far away.) Tj ET")
	_, _, lines = lineEnds(t, tight)
	l, _ = layoutOf(t, tight)
	if a := paragraphAlignment(l, 0, lines); a.justified {
		t.Error("a first line set at its natural spaces read as justified because it is the column's widest")
	}
}

// TestRealJustifiedParagraphsReWrapFlush — S03 over the real-producer corpus (D12): every justified paragraph the rewrite
// accepts — the edit DELETES its second word, so every later word moves back and lines change their words — reads back
// from the written document with every line but its last, of more than one word, ending at its flush edge, and every
// re-set line's spaces even (none keeps an old line's stretch; the review of S03, W3). Lines are found by baseline, as the
// read-back test finds them. A sample: at most thirty justified paragraphs a document; refusals are counted by cause.
// **The stimulus is asserted**: justified paragraphs must have been re-set, or the test proved nothing.
func TestRealJustifiedParagraphsReWrapFlush(t *testing.T) {
	corp := externalPDFs(t, "real producers", "NIB_UA_PRODUCERS", filepath.Join(homeDir(), "nib", "producers"))
	if corp.absent != "" {
		t.Skip("SKIP (not a measurement): " + corp.absent)
	}
	reset, flush, lines := 0, 0, 0
	refused := map[string]int{}
	for _, doc := range corp.docs {
		ctx, err := pdfread.Validated(doc.pdf, model.NewDefaultConfiguration())
		if err != nil {
			continue
		}
		tried := 0
		for p := 1; p <= ctx.PageCount && tried < 30; p++ {
			l, err := readPageGlyphLayout(ctx, pageAt(ctx, nil, p))
			if err != nil {
				continue
			}
			for pi, para := range l.paragraphs {
				orig, _, cause := paragraphWords(para)
				f := strings.Fields(para.text())
				if cause != "" || len(f) < 6 || tried >= 30 {
					continue
				}
				a := paragraphAlignment(l, pi, orig)
				if !a.justified {
					continue
				}
				tried++
				edit := strings.Join(append(f[:1:1], f[2:]...), " ")
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
				size := para.lines[0].size
				pitch := paragraphPitch(l, pi)
				var got []textLine
				for i, n := 0, 0; n < editLetters(edit); i++ { // letters: a hyphenated break can read back rejoined (P08.S07)
					y := para.lines[min(i, len(para.lines)-1)].y - float64(max(0, i-len(para.lines)+1))*pitch
					ln, ok := lineAtBaseline(l3, paragraphBox(para), y, size)
					if !ok {
						break
					}
					got = append(got, ln)
					n += editLetters(ln.text)
				}
				ws, _, cause := paragraphWords(textParagraph{lines: got, column: para.column})
				if cause != "" || len(ws) < 2 {
					continue
				}
				for i, ln := range ws {
					for k := 2; k < len(ln); k++ {
						g0, g := ln[1].startX-ln[0].startX-ln[0].width, ln[k].startX-ln[k-1].startX-ln[k-1].width
						if math.Abs(g-g0) > justifyTol*size {
							t.Errorf("%s p%d ¶%d line %d: uneven spaces %v then %v", doc.name, p, pi, i, g0, g)
							break
						}
					}
					if i == len(ws)-1 || len(ln) < 2 {
						continue
					}
					lines++
					end := ln[len(ln)-1]
					if math.Abs(end.startX+end.width-a.edge) <= justifyTol*size {
						flush++
					} else {
						t.Errorf("%s p%d ¶%d line %d ends at %v, the flush edge is %v", doc.name, p, pi, i, end.startX+end.width, a.edge)
					}
				}
			}
		}
	}
	if reset == 0 {
		t.Fatal("no justified paragraph was re-set — the justification was never exercised")
	}
	// The door's own floor (the review's R2-C1: an evenness rule silently vetoed 76 of 284): of the multi-line paragraphs
	// whose lines before the last end together and whose lines after the first begin together, the door reads nearly all
	// as justified — what it declines must be the rare table, not the producer's sentence spaces.
	flushPara, detected := 0, 0
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
				if cause != "" || len(orig) < 3 {
					continue
				}
				tol := justifyTol * para.lines[0].size
				lo, hi, llo, lhi := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
				for i, ln := range orig {
					if i < len(orig)-1 {
						e := ln[len(ln)-1].startX + ln[len(ln)-1].width
						lo, hi = math.Min(lo, e), math.Max(hi, e)
					}
					if i > 0 {
						llo, lhi = math.Min(llo, ln[0].startX), math.Max(lhi, ln[0].startX)
					}
				}
				if hi-lo > tol || lhi-llo > tol {
					continue
				}
				flushPara++
				if paragraphAlignment(l, pi, orig).justified {
					detected++
				}
			}
		}
	}
	if detected*100 < flushPara*95 {
		t.Errorf("the door reads %d of %d flush paragraphs as justified — under 95%%", detected, flushPara)
	}
	t.Logf("the door reads %d of %d flush paragraphs as justified", detected, flushPara)
	t.Logf("%d justified paragraphs re-set; %d of %d lines before the last flush; refused by cause %v", reset, flush, lines, refused)
}

// TestAJustifiedLineNeverRunsPastItsEdge — the review of S03's own build, by construction: a paragraph justified by `TJ`
// whose stretched lines outnumber its natural ones has a median space carrying the stretch, so its own measure lies past
// the flush edge and a re-broken line could not be set flush — it was written out to 368 against an edge of 271. Such a
// paragraph is broken again at the edge: every line ends within it, the lines before the last flush; and a word wider
// than the edge is `word-too-wide`, never set past it.
func TestAJustifiedLineNeverRunsPastItsEdge(t *testing.T) {
	const left = 72.0
	l3 := "abcdefgh ijklmnop qrstuvwx yzabcdef"
	edge := left + mdpdf.CoreWidth(l3, "Helvetica", 12)
	line := func(ln string) string {
		ws := strings.Fields(ln)
		extra := (edge - left - mdpdf.CoreWidth(ln, "Helvetica", 12)) / float64(len(ws)-1)
		out := " ["
		for i, w := range ws {
			if i > 0 {
				out += fmt.Sprintf("( ) %s ", num(-extra/12*1000))
			}
			out += "(" + w + ") "
		}
		return out + "] TJ"
	}
	pdf := helveticaPage("BT /F1 12 Tf 14 TL 72 700 Td" + line("a b c d e") + " T*" + line("a b c d e") + " T*" + line(l3) +
		" T* (end of it) Tj ET")
	_, _, lines := lineEnds(t, pdf)
	l, _ := layoutOf(t, pdf)
	if a := paragraphAlignment(l, 0, lines); !a.justified || math.Abs(a.edge-edge) > 1e-6 {
		t.Fatalf("setup: read as %+v, want justified at %v", a, edge)
	}
	out, cause := reflowed(t, pdf, 0, "a b c d e a b c d e abcdefghijklmnopqrstuvwxyz abcdefghijklmnopqrstuvwxyzabcd end of it")
	if cause != "" {
		t.Fatalf("refused (%s), want it broken again at the edge", cause)
	}
	l, _ = layoutOf(t, out)
	var all []textLine
	for _, q := range l.paragraphs {
		all = append(all, q.lines...)
	}
	for _, ln := range all {
		if ln.x1 > edge+1e-6 {
			t.Errorf("%q runs to %v, past the flush edge %v", ln.text, ln.x1, edge)
		}
	}
	if _, cause := reflowed(t, pdf, 0, "a b c d e a b c d e abcdefghijklmnopqrstuvwxyzabcdefghijklmnop end of it"); cause != causeWordTooWide {
		t.Errorf("a word wider than the edge: %q, want %q", cause, causeWordTooWide)
	}
}

// TestARightAlignedParagraphIsNotJustified — lines ending together are not enough: lines that END flush but BEGIN ragged
// are right-aligned (S04's), and stretching them to an edge from a left margin they never had would move every word. Built
// on the door directly: grouping reads such lines as one paragraph only when their starts lie close (the census found 7).
func TestARightAlignedParagraphIsNotJustified(t *testing.T) {
	word := func(x, w float64) reflowWord {
		return reflowWord{startX: x, width: w, glyphs: []runGlyph{{text: "x"}}, spacing: []textSpacing{{th: 1}}}
	}
	l := pageLayout{paragraphs: []textParagraph{{lines: []textLine{{size: 12}}}}}
	// Three lines ending at 400; the first two begin at 100 and 160, the last at 300.
	lines := [][]reflowWord{
		{word(100, 140), word(244, 156)},
		{word(160, 100), word(264, 136)},
		{word(300, 40), word(344, 56)},
	}
	if a := paragraphAlignment(l, 0, lines); a.justified {
		t.Errorf("a right-aligned paragraph read as justified at %v", a.edge)
	}
	// The control: the same ends, with the lines after the first beginning together, is justified.
	lines[1][0].startX, lines[2][0].startX = 100, 100
	if a := paragraphAlignment(l, 0, lines); !a.justified || a.edge != 400 {
		t.Errorf("flush on both sides read as %+v, want justified at 400", a)
	}
}

// TestAContinuingParagraphIsJustifiedToItsLastLine — a justified paragraph whose last line ALSO ends at the edge runs on
// past this page or column: re-set, its last line is set to the edge too, and no line's spaces keep a stretch (the review
// of S03, R2-W1: the last line was set with its OLD stretch as its "natural" space).
func TestAContinuingParagraphIsJustifiedToItsLastLine(t *testing.T) {
	for _, byTJ := range []bool{false, true} {
		t.Run(map[bool]string{false: "by Tw", true: "by TJ"}[byTJ], func(t *testing.T) {
			// The last line flush too: justifiedPara stretches every line but its last, so a fourth is added and taken off.
			content := justifiedPara(byTJ, append(append([]string(nil), justifiedText[:3]...), "end")...)
			content = content[:strings.LastIndex(content, " T*")] + " 0 Tw ET"
			pdf := helveticaPage(content)
			_, _, lines := lineEnds(t, pdf)
			l, _ := layoutOf(t, pdf)
			if a := paragraphAlignment(l, 0, lines); !a.justified || !a.continues {
				t.Fatalf("setup: read as %+v, want justified and continuing", a)
			}
			out, cause := reflowed(t, pdf, 0, strings.Replace(strings.Join(justifiedText[:3], " "), "quick brown fox", "fox", 1))
			if cause != "" {
				t.Fatalf("refused (%s)", cause)
			}
			ends, gaps, lines := lineEnds(t, out)
			for i := range lines {
				if len(lines[i]) > 1 && math.Abs(ends[i]-400) > 1e-6 {
					t.Errorf("line %d ends at %v, not the flush edge 400", i, ends[i])
				}
				for _, g := range gaps[i] {
					if math.Abs(g-gaps[i][0]) > 1e-6 {
						t.Errorf("line %d's spaces are uneven %v", i, gaps[i])
						break
					}
				}
			}
		})
	}
}

// TestAJustifiedLineShrinksToAQuarterOfItsNaturalSpace — the fit rule's floor is a quarter of the NATURAL space (the
// spacer's gap less the median stretch, `lastDelta`), not of the spacer's gap: a spacer gap of 12 whose natural is 2 may
// shrink to 0.5, not only to 3 (the review of S03, W1). On the door directly, with a stub spacer.
func TestAJustifiedLineShrinksToAQuarterOfItsNaturalSpace(t *testing.T) {
	w := func(width float64) emitWord { return emitWord{width: width, spacing: []textSpacing{{th: 1}}} }
	lines := [][]reflowWord{{{startX: 0}}}
	space := func(*runFont, float64, textSpacing) float64 { return 12 }
	a := alignment{justified: true, edge: 100, lastDelta: -10}
	// Two words of 49 with a gap of 12 run to 110: the gap must fall to 2 — under 3, over 0.5.
	out, fits := justifiedLines(a, [][]emitWord{{w(49), w(49)}, {w(10)}}, lines, space)
	if !fits || math.Abs(out[0]+10) > 1e-9 {
		t.Errorf("fits %v, adjustment %v; want it to fit with -10", fits, out[0])
	}
	// A line a hair past the edge — within the tolerance its own lines were read at — is at it (real OIDs and URLs end
	// 0.01pt and 0.33pt past their column's edge); past the tolerance it is not.
	a.tol = 0.6
	if _, fits := justifiedLines(a, [][]emitWord{{w(49), w(49)}, {w(100.3)}}, lines, space); !fits {
		t.Error("a last line 0.3 past the edge, within its tolerance 0.6, did not fit")
	}
	if _, fits := justifiedLines(a, [][]emitWord{{w(49), w(49)}, {w(100.7)}}, lines, space); fits {
		t.Error("a last line 0.7 past the edge, past its tolerance 0.6, fitted")
	}
	// And under a quarter of the natural it does not: a gap of 0.4.
	if _, fits := justifiedLines(a, [][]emitWord{{w(49.8), w(49.8)}, {w(10)}}, lines, space); fits {
		t.Error("a line whose space falls under a quarter of its natural width fitted")
	}
}
