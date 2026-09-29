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
)

// A three-line Helvetica paragraph in one text object, then — still in that object, two lines down — a signature line
// that must not move when the paragraph is rewritten.
const reflowPara = "BT /F1 12 Tf 14 TL 72 700 Td (The quick brown fox jumps) Tj T* (over the lazy dog and runs) Tj T* (away from here.) Tj"
const reflowTail = " T* T* T* (Signature line) Tj ET"

func helveticaPage(content string) []byte {
	return onePageFixture(content, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
}

// reflowed applies reflowParagraph to paragraph pi of page 1 and returns the written document, or the cause.
func reflowed(t *testing.T, pdf []byte, pi int, text string) ([]byte, string) {
	t.Helper()
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	out, err := reflowParagraph(ctx, 1, pi, text)
	if err != nil {
		t.Fatal(err)
	}
	if out.content == nil {
		return nil, out.cause
	}
	d, _, _, _ := ctx.PageDict(1, false)
	if err := setPageContent(ctx, d, out.content); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := api.WriteContext(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), ""
}

func layoutOf(t *testing.T, pdf []byte) (pageLayout, []textRun) {
	t.Helper()
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	l, err := readPageGlyphLayout(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	pr, err := readPageGlyphRuns(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	return l, pr.runs
}

// TestAWordIsReplacedAndTheParagraphRewraps — `PLAN-text-reflow.md` P06.S04: an edited paragraph reads back as the edit,
// in its own font, starting on its own first baseline and left edge; the replaced word is GONE from the page's content
// (not covered); and text the same text object draws after the paragraph lands exactly where it did.
func TestAWordIsReplacedAndTheParagraphRewraps(t *testing.T) {
	pdf := helveticaPage(reflowPara + reflowTail)
	before, beforeRuns := layoutOf(t, pdf)
	if len(before.paragraphs) != 2 || before.paragraphs[0].text() != "The quick brown fox jumps over the lazy dog and runs away from here." {
		t.Fatalf("the fixture reads as %d paragraphs: %q", len(before.paragraphs), paragraphTexts(before))
	}
	edit := "The quick brown fox jumps over the sleepy old dog and runs away from here."
	out, cause := reflowed(t, pdf, 0, edit)
	if cause != "" {
		t.Fatalf("the edit fell back: %s", cause)
	}
	after, afterRuns := layoutOf(t, out)
	if len(after.paragraphs) != 2 || after.paragraphs[0].text() != edit {
		t.Fatalf("the rewritten page reads as %q", paragraphTexts(after))
	}
	var page strings.Builder
	for _, r := range afterRuns {
		page.WriteString(r.text + "\n")
	}
	if strings.Contains(page.String(), "lazy") {
		t.Errorf("\"lazy\" is still drawn on the page — covered, not removed:\n%s", page.String())
	}
	p0, q0 := before.paragraphs[0], after.paragraphs[0]
	if len(q0.lines) > len(p0.lines) {
		t.Errorf("%d lines became %d", len(p0.lines), len(q0.lines))
	}
	for i := range q0.lines {
		if math.Abs(q0.lines[i].y-p0.lines[i].y) > 1e-6 || math.Abs(q0.lines[i].x0-p0.lines[0].x0) > 1e-6 {
			t.Errorf("line %d sits at (%v, %v); its original baseline and the paragraph's left edge are (%v, %v)",
				i, q0.lines[i].x0, q0.lines[i].y, p0.lines[0].x0, p0.lines[i].y)
		}
		if q0.lines[i].runs[0].baseFont != "Helvetica" || q0.lines[i].runs[0].size != 12 {
			t.Errorf("line %d is set in %s %v, not the paragraph's Helvetica 12", i, q0.lines[i].runs[0].baseFont, q0.lines[i].runs[0].size)
		}
	}
	sig := func(runs []textRun) textRun {
		for _, r := range runs {
			if r.text == "Signature line" {
				return r
			}
		}
		t.Fatal("no signature line")
		return textRun{}
	}
	if b, a := sig(beforeRuns), sig(afterRuns); math.Abs(a.x-b.x) > 1e-6 || math.Abs(a.y-b.y) > 1e-6 || a.size != b.size {
		t.Errorf("the signature line drawn after the paragraph moved from (%v, %v) to (%v, %v)", b.x, b.y, a.x, a.y)
	}
	if right := rightEdgeOf(p0); right > 0 {
		for i, l := range q0.lines {
			if l.x1 > right+1e-6 {
				t.Errorf("line %d reaches %v, past the paragraph's right edge %v", i, l.x1, right)
			}
		}
	}
}

// rightEdgeOf is the furthest a paragraph's lines reach, as its words and space measure them.
func rightEdgeOf(p textParagraph) float64 {
	lines, space, _ := paragraphWords(p)
	if len(lines) == 0 {
		return 0
	}
	return lines[0][0].startX + lineMeasures(lines, space)[0]
}

// TestAnUneditedReflowWritesNothing — law 1 at the door: the paragraph's own text is no edit, and the page is not
// rewritten at all.
func TestAnUneditedReflowWritesNothing(t *testing.T) {
	ctx, err := pdfread.Validated(helveticaPage(reflowPara+reflowTail), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	out, err := reflowParagraph(ctx, 1, 0, "  The quick brown fox jumps over\nthe lazy dog and runs away from here. ")
	if err != nil || out.content != nil || out.cause != "" {
		t.Errorf("the paragraph's own text wrote %d bytes, cause %q, err %v", len(out.content), out.cause, err)
	}
}

// TestAReflowItCannotDoExactlyNamesWhy — every refusal the rewrite owns, each from a fixture built to reach it (law 3).
func TestAReflowItCannotDoExactlyNamesWhy(t *testing.T) {
	grow := strings.Repeat("and then some more words ", 12)
	for _, c := range []struct {
		name, content, text, want string
	}{
		{"a character the font does not carry", reflowPara + reflowTail, "The quick brown Ω fox", causeMissingGlyph},
		{"an edit needing more lines than the paragraph has", reflowPara + reflowTail, "The quick " + grow, causeGrows},
		{"a word wider than the measure", reflowPara + reflowTail, "The " + strings.Repeat("x", 80), causeWordTooWide},
		{"marked content between its lines",
			"BT /F1 12 Tf 14 TL 72 700 Td /P <</MCID 0>> BDC (The quick brown fox jumps) Tj EMC T* /P <</MCID 1>> BDC (over the lazy dog) Tj EMC ET",
			"The quick brown fox leaps over the lazy dog", causeTagged},
		{"a colour change between its lines",
			"BT /F1 12 Tf 14 TL 72 700 Td (The quick brown fox jumps) Tj 1 0 0 rg T* (over the lazy dog) Tj ET",
			"The quick brown fox leaps over the lazy dog", causeMixedContent},
		{"an artifact drawn straight after it",
			"BT /F1 12 Tf 14 TL 72 700 Td (The quick brown fox jumps) Tj T* (over the lazy dog) Tj /Artifact BMC (DRAFT) Tj EMC ET",
			"The quick brown fox leaps over the lazy dog", causeInlineFollower},
		{"runs set under different character spacing",
			// `"` sets word and character spacing AS it shows, so the second line is set under different spacing with
			// nothing between the two shows for the content check to see.
			"BT /F1 12 Tf 14 TL 72 700 Td (The quick brown fox jumps) Tj 0 1 (over the lazy dog) \" ET",
			"The quick brown fox leaps over the lazy dog", causeMixedState},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, cause := reflowed(t, helveticaPage(c.content), 0, c.text)
			if cause != c.want {
				t.Errorf("fell back on %q, want %q", cause, c.want)
			}
		})
	}
}

// TestEveryRewrittenParagraphReadsBackAsItsEdit — S04 over real documents (D12's law: the corpus is the oracle). For
// every multi-word paragraph on every page of the generated and real-producer corpora, the edit swaps its first two
// words — always drawable, since the paragraph already draws both — and every paragraph the rewrite ACCEPTS must read
// back, from the written document, as exactly that text at the same paragraph index, re-broken as the one line breaker
// sets it at the paragraph's own measure (P06's first exit criterion). Refusals are counted by cause; a paragraph
// accepted and read back wrong is the failure this exists to find.
//
// **A deterministic SAMPLE, and it says so**: the first two pages of each document and at most four paragraphs from
// them — each edit re-reads and rewrites the whole document, and doing every paragraph of a 1,000-paragraph census file
// was minutes. Every producer is still reached.
//
// **The stimulus is asserted**: some paragraph must have been rewritten, or the test proved nothing about the rewrite.
func TestEveryRewrittenParagraphReadsBackAsItsEdit(t *testing.T) {
	corpora := []lawOneCorpus{
		{name: "generated", docs: runCorpus(t)},
		externalPDFs(t, "real producers", "NIB_UA_PRODUCERS", filepath.Join(homeDir(), "nib", "producers")),
	}
	rewritten, respelled := 0, 0
	causes, respellRefused := map[string]int{}, map[string]int{}
	for _, corp := range corpora {
		if corp.absent != "" {
			t.Logf("NOTE (a narrower population, not a pass over it): %s: %s", corp.name, corp.absent)
			continue
		}
		for _, doc := range corp.docs {
			ctx, err := pdfread.Validated(doc.pdf, model.NewDefaultConfiguration())
			if err != nil {
				continue
			}
			tried := 0
			for p := 1; p <= ctx.PageCount && p <= 2; p++ {
				l, err := readPageGlyphLayout(ctx, p)
				if err != nil {
					continue
				}
				for pi, para := range l.paragraphs {
					if tried >= 4 {
						break
					}
					f := strings.Fields(para.text())
					if len(f) < 2 || f[0] == f[1] {
						continue
					}
					tried++
					// try rewrites paragraph pi as edit and checks what the written document reads: the paragraph as the
					// edit, re-broken as the breaker sets it, and EVERY OTHER paragraph on the page exactly as it was — a
					// rewrite that deleted a later paragraph's shows must not pass because the edited one reads right.
					try := func(edit string) (accepted bool, cause string) {
						c2, _ := pdfread.Validated(doc.pdf, model.NewDefaultConfiguration())
						out, err := reflowParagraph(c2, p, pi, edit)
						if err != nil {
							t.Fatalf("%s / %s p%d ¶%d: %v", corp.name, doc.name, p, pi, err)
						}
						if out.content == nil {
							return false, out.cause
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
							t.Fatalf("%s / %s p%d ¶%d: the rewritten document does not read: %v", corp.name, doc.name, p, pi, err)
						}
						l3, err := readPageGlyphLayout(c3, p)
						if err != nil || pi >= len(l3.paragraphs) {
							t.Errorf("%s / %s p%d ¶%d: the rewritten page lost the paragraph (%v)", corp.name, doc.name, p, pi, err)
							return true, ""
						}
						got := l3.paragraphs[pi]
						if got.text() != edit {
							t.Errorf("%s / %s p%d ¶%d: wrote %q, reads back %q", corp.name, doc.name, p, pi, edit, got.text())
							return true, ""
						}
						others := func(l pageLayout) []string {
							var o []string
							for j, q := range l.paragraphs {
								if j != pi {
									o = append(o, q.text())
								}
							}
							return o
						}
						// Compared as TEXT, not as paragraphs: grouping reads the whole page, so new line widths in one
						// paragraph can move a boundary between two others (measured: fda-83122 p1, where "These are also"
						// and "explained in this paper." join) with every glyph still drawn where it was.
						if a, b := normalizedText(strings.Join(others(l), " ")), normalizedText(strings.Join(others(l3), " ")); a != b {
							t.Errorf("%s / %s p%d ¶%d: the page's other paragraphs changed:\n was %q\n now %q", corp.name, doc.name, p, pi, a, b)
						}
						origLines, space, _ := paragraphWords(para)
						newLines, _, cause := paragraphWords(got)
						if cause == "" {
							var words []reflowWord
							for _, ln := range newLines {
								words = append(words, ln...)
							}
							if a, b := lineTexts(rebreak(words, lineMeasures(origLines, space), space)), lineTexts(newLines); strings.Join(a, "\n") != strings.Join(b, "\n") {
								t.Errorf("%s / %s p%d ¶%d: written as %q, the breaker sets %q", corp.name, doc.name, p, pi, b, a)
							}
						}
						return true, ""
					}
					swapped := append([]string(nil), f...)
					swapped[0], swapped[1] = swapped[1], swapped[0]
					if ok, cause := try(strings.Join(swapped, " ")); !ok {
						causes[cause]++
						continue
					}
					rewritten++
					// A NEW word — the first word spelled backwards, which the paragraph does not already draw — reaches
					// the path the swap never can: each character's code chosen by `codesFor`, its width by the font.
					rs := []rune(f[0])
					for a, b := 0, len(rs)-1; a < b; a, b = a+1, b-1 {
						rs[a], rs[b] = rs[b], rs[a]
					}
					if rev := string(rs); len(rs) > 2 && rev != f[0] && !strings.Contains(" "+para.text()+" ", " "+rev+" ") {
						if ok, cause := try(rev + " " + strings.Join(f[1:], " ")); ok {
							respelled++
						} else {
							respellRefused[cause]++
						}
					}
				}
			}
		}
	}
	if rewritten == 0 {
		t.Errorf("no paragraph was rewritten (refusals %v) — the rewrite was never exercised", causes)
	}
	if respelled == 0 {
		t.Errorf("no new word was spelled and read back (refusals %v) — codesFor's path was never exercised", respellRefused)
	}
	t.Logf("%d paragraphs rewritten and read back; refused by cause %v", rewritten, causes)
	t.Logf("%d new words spelled from the font and read back; refused by cause %v", respelled, respellRefused)
}

// TestANewWordReusesTheCodeTheParagraphAlreadyDraws — when a font draws a character with two codes, a new word is spelled
// with the one the paragraph already uses: the two may be different glyphs of one character (a subset's duplicate), and
// the one on the page is the one the reader has seen. Here <0001> and <0002> both decode to "a" and the paragraph draws
// <0002>.
func TestANewWordReusesTheCodeTheParagraphAlreadyDraws(t *testing.T) {
	cmap := "/CIDInit /ProcSet findresource begin 12 dict begin begincmap\n1 begincodespacerange <0000> <FFFF> endcodespacerange\n" +
		"4 beginbfchar <0001> <0061> <0002> <0061> <0003> <0062> <0004> <0020> endbfchar\nendcmap CMapName currentdict /CMap defineresource pop end end"
	show := "BT /F1 10 Tf 12 TL 72 700 Td <000200040003> Tj T* <0003> Tj ET"
	pdf := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(show), show),
		5: "<< /Type /Font /Subtype /Type0 /BaseFont /ABCDEF+Sub /Encoding /Identity-H /DescendantFonts [6 0 R] /ToUnicode 7 0 R >>",
		6: "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /ABCDEF+Sub /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /FontDescriptor 8 0 R /CIDToGIDMap /Identity /DW 500 >>",
		7: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(cmap), cmap),
		8: "<< /Type /FontDescriptor /FontName /ABCDEF+Sub /Flags 4 /FontBBox [0 -200 1000 800] /ItalicAngle 0 /Ascent 800 /Descent -200 /CapHeight 700 /StemV 80 >>",
	})
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	l, err := readPageGlyphLayout(ctx, 1)
	if err != nil || len(l.paragraphs) != 1 || l.paragraphs[0].text() != "a b b" {
		t.Fatalf("the fixture reads as %q (%v)", paragraphTexts(l), err)
	}
	out, err := reflowParagraph(ctx, 1, 0, "ba b")
	if err != nil || out.content == nil {
		t.Fatalf("the edit fell back on %q (%v)", out.cause, err)
	}
	if !bytes.Contains(out.content, []byte("<0003><0002>")) || bytes.Contains(out.content, []byte("<0001>")) {
		t.Errorf("\"ba\" must be spelled <0003><0002>, the a the paragraph draws; the rewrite wrote\n%s", out.content)
	}
}

// twoFontPage is a page with /F1 Helvetica and /F2 Helvetica-Bold, under resource names r1 and r2.
func twoFontPage(r1, r2, content string) []byte {
	return assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /" + r1 + " 5 0 R /" + r2 + " 6 0 R >> >> /Contents 4 0 R >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
		6: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>",
	})
}

// rewriteReadsBack reflows paragraph 0 of pdf to edit and returns the rewritten paragraph as read back, failing on a
// refusal or a document that no longer reads as the edit.
func rewriteReadsBack(t *testing.T, pdf []byte, edit string) (before, after textParagraph, afterRuns []textRun) {
	t.Helper()
	l0, _ := layoutOf(t, pdf)
	out, cause := reflowed(t, pdf, 0, edit)
	if cause != "" {
		t.Fatalf("fell back on %q", cause)
	}
	l1, runs := layoutOf(t, out)
	if len(l1.paragraphs) == 0 || l1.paragraphs[0].text() != edit {
		t.Fatalf("the rewritten page reads as %q, want %q", paragraphTexts(l1), edit)
	}
	return l0.paragraphs[0], l1.paragraphs[0], runs
}

// TestTheReviewsFindingsHold — one fixture per finding the P06.S04 diff review made, each of which the rewrite accepted
// and rendered wrong, silently, before its fix.
func TestTheReviewsFindingsHold(t *testing.T) {
	t.Run("a bold word stays bold, and a font change never lands inside a TJ array", func(t *testing.T) {
		pdf := twoFontPage("F1", "F2", "BT /F1 12 Tf 14 TL 72 700 Td (The ) Tj /F2 12 Tf (quick) Tj /F1 12 Tf ( brown fox jumps) Tj T* (over the lazy dog) Tj ET")
		_, _, runs := rewriteReadsBack(t, pdf, "The quick brown fox leaps over the lazy dog")
		bold := false
		for _, r := range runs {
			if strings.Contains(r.text, "quick") {
				bold = r.baseFont == "Helvetica-Bold" && !strings.Contains(r.text, "brown")
			}
		}
		if !bold {
			t.Errorf("\"quick\" is no longer a Helvetica-Bold run of its own: %+v", runs)
		}
	})
	t.Run("the spacing a deleted \" set is the spacing the lines are drawn under", func(t *testing.T) {
		pdf := helveticaPage("BT /F1 12 Tf 14 TL 72 714 Td 2 1 (The quick brown fox jumps) \" 2 1 (over the lazy dog) \" ET")
		before, after, _ := rewriteReadsBack(t, pdf, "The quick brown fox leaps over the lazy dog")
		_, s0, _ := paragraphWords(before)
		_, s1, _ := paragraphWords(after)
		if math.Abs(s0-s1) > 1e-6 {
			t.Errorf("the paragraph's space was %v and is drawn %v — the lines were measured under spacing they are not drawn under", s0, s1)
		}
	})
	t.Run("a word drawn as two runs of one font keeps the gap between them", func(t *testing.T) {
		// "Word" is drawn as two runs with 0.6pt between them: one word, and the 0.6pt is part of how wide it is drawn.
		// (The Wo) is 3501/1000 × 12 = 42.012pt wide, so (rd …) placed at 72 + 42.012 + 0.6 opens a 0.6pt gap between the
		// two runs — under the 0.15em that separates words.
		pdf := helveticaPage("BT /F1 12 Tf 14 TL 1 0 0 1 72 700 Tm (The Wo) Tj 1 0 0 1 114.612 700 Tm (rd and more) Tj 1 0 0 1 72 686 Tm (words here) Tj ET")
		before, after, _ := rewriteReadsBack(t, pdf, "The Word and words here more")
		width := func(p textParagraph) float64 {
			lines, _, _ := paragraphWords(p)
			for _, l := range lines {
				for _, w := range l {
					if w.text() == "Word" {
						return w.width
					}
				}
			}
			t.Fatalf("no \"Word\" in %q", p.text())
			return 0
		}
		// Hand-derived, not measured by the rule under test: W o r d are 944 + 556 + 333 + 556 = 2389 thousandths of
		// Helvetica's em at 12pt, 28.668pt, and the 0.6pt between the runs is inside the word.
		const want = 28.668 + 0.6
		if b, a := width(before), width(after); math.Abs(b-want) > 1e-6 || math.Abs(a-want) > 1e-6 {
			t.Errorf("\"Word\" reads %v wide before and %v after; it is %v", b, a, want)
		}
	})
	t.Run("a word drawn in two fonts is refused, not re-set in the first", func(t *testing.T) {
		pdf := twoFontPage("F1", "F2", "BT /F1 12 Tf 14 TL 72 700 Td (The Wo) Tj /F2 12 Tf (rd and more) Tj /F1 12 Tf T* (words here) Tj ET")
		if _, cause := reflowed(t, pdf, 0, "The Word and more words"); cause != "styled-word" {
			t.Errorf("fell back on %q, want styled-word", cause)
		}
	})
	t.Run("a line's lead survives: a leading TJ adjustment is where its first word begins", func(t *testing.T) {
		pdf := helveticaPage("BT /F1 12 Tf 14 TL 72 700 Td [-500 (The quick brown fox jumps)] TJ T* [-500 (over the lazy dog)] TJ ET")
		before, after, _ := rewriteReadsBack(t, pdf, "The quick brown fox leaps over the lazy dog")
		bl, _, _ := paragraphWords(before)
		al, _, _ := paragraphWords(after)
		for i := range al {
			if math.Abs(al[i][0].startX-bl[0][0].startX) > 1e-6 {
				t.Errorf("line %d's first word starts at %v; the paragraph's words start at %v", i, al[i][0].startX, bl[0][0].startX)
			}
		}
	})
	t.Run("a first-line indent is kept, and the indented line has less room", func(t *testing.T) {
		// Line 0 is indented 18pt; the edit shortens it, and the breaker must fill line 0 only to the right edge minus
		// the indent — with one measure for every line, the indented line would reach 18pt past the edge.
		pdf := helveticaPage("BT /F1 12 Tf 14 TL 72 700 Td [-1500 (The quick brown fox jumps over)] TJ T* (the lazy dog and runs off.) Tj ET")
		before, after, _ := rewriteReadsBack(t, pdf, "The quick fox jumps over the lazy dog and runs off.")
		bl, _, _ := paragraphWords(before)
		al, _, _ := paragraphWords(after)
		for i := range al {
			if math.Abs(al[i][0].startX-bl[i][0].startX) > 1e-6 {
				t.Errorf("line %d's first word starts at %v; that line began at %v", i, al[i][0].startX, bl[i][0].startX)
			}
		}
		if right := rightEdgeOf(before); after.lines[0].x1 > right+1e-6 {
			t.Errorf("the indented line reaches %v, past the paragraph's right edge %v", after.lines[0].x1, right)
		}
	})
	t.Run("replacement text around the paragraph is refused", func(t *testing.T) {
		pdf := helveticaPage("/Span <</ActualText (old words)>> BDC BT /F1 12 Tf 14 TL 72 700 Td (The quick brown fox jumps) Tj T* (over the lazy dog) Tj ET EMC")
		if _, cause := reflowed(t, pdf, 0, "The quick brown fox leaps over the lazy dog"); cause != causeReplacementText {
			t.Errorf("fell back on %q, want %s", cause, causeReplacementText)
		}
	})
	t.Run("a resource name with an escaped byte is written back escaped", func(t *testing.T) {
		// The bold word's font is named `F#20a` — the name "F a" — so the rewrite must switch TO it by name: written back
		// unescaped it is the name /F and the operand a, the switch fails, and "quick" is drawn in the regular face.
		pdf := twoFontPage("F1", "F#20a", "BT /F1 12 Tf 14 TL 72 700 Td (The ) Tj /F#20a 12 Tf (quick) Tj /F1 12 Tf ( brown fox jumps) Tj T* (over the lazy dog) Tj ET")
		_, _, runs := rewriteReadsBack(t, pdf, "The quick brown fox leaps over the lazy dog")
		bold := false
		for _, r := range runs {
			bold = bold || (strings.TrimSpace(r.text) == "quick" && r.baseFont == "Helvetica-Bold") // its space is drawn in bold too
		}
		if !bold {
			t.Errorf("\"quick\" is not drawn in the face named F#20a: %+v", runs)
		}
	})
	t.Run("numbers are exact, fixed-point, and never -0", func(t *testing.T) {
		for v, want := range map[float64]string{1e-7: "0.0000001", math.Copysign(0, -1): "0", 123.5: "123.5", -2.25: "-2.25"} {
			if got := num(v); got != want {
				t.Errorf("num(%v) = %q, want %q", v, got, want)
			}
		}
	})
}
