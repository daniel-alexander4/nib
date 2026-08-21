package mdpdf

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"unicode"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// readTestFont supplies a real TTF for the fallback tests. It uses one of the faces Nib
// already vendors for OCR — a font this repo ships anyway, rather than a fixture invented
// for the test. mdpdf itself does not depend on that directory (the whole point of Font is
// that the caller brings the bytes); only this test reaches for it, and it skips rather
// than fails where it is absent, because mdpdf is meant to be importable on its own.
func readTestFont(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../internal/pdfops/fonts/NotoSansThai-Regular.ttf")
	if err != nil {
		t.Skipf("no vendored TTF to test embedding with: %v", err)
	}
	return b
}

// embeddedFaces returns the BaseFont names in a PDF, read through pdfcpu rather than by
// searching the bytes.
//
// The byte search is the obvious version and it is vacuous: pdfcpu writes object streams,
// so a font name lives COMPRESSED inside one and `bytes.Contains(pdf, "NotoSansThai")` is
// false for a document that does embed it. (The same trap cost a scan_test.go assertion in
// this repo the same week; it is written out here so the next person meets it as a comment
// rather than as an hour.)
func embeddedFaces(t *testing.T, pdf []byte) []string {
	t.Helper()
	// ReadValidateAndOptimize, not ReadContext: the plain reader leaves font dictionaries
	// unresolved, so the walk below finds ZERO fonts in a document that certainly has one
	// — measured, on a core-font render. Optimizing is what populates them.
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-reading the PDF: %v", err)
	}
	var out []string
	for _, e := range ctx.XRefTable.Table {
		if e == nil {
			continue
		}
		d, ok := e.Object.(types.Dict)
		if !ok {
			continue
		}
		if n := d.NameEntry("BaseFont"); n != nil {
			out = append(out, *n)
		}
	}
	return out
}

func hasFace(faces []string, want string) bool {
	for _, f := range faces {
		if strings.Contains(f, want) {
			return true
		}
	}
	return false
}

func render(t *testing.T, md string) []byte {
	t.Helper()
	pdf, err := Convert([]byte(md))
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatal("output is not a PDF")
	}
	if err := api.Validate(bytes.NewReader(pdf), model.NewDefaultConfiguration()); err != nil {
		t.Fatalf("invalid PDF: %v", err)
	}
	return pdf
}

func pageCount(t *testing.T, pdf []byte) int {
	t.Helper()
	n, err := api.PageCount(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("page count: %v", err)
	}
	return n
}

func TestConvertBasic(t *testing.T) {
	md := `# Title

A paragraph with *italic*, **bold**, ` + "`code`" + `, a [link](https://example.org), and 45% growth.

## Section

1. first item
2. second item
   - nested bullet

> a blockquote

---

` + "```\nfunc main() {}\n```\n"
	pdf := render(t, md)
	if n := pageCount(t, pdf); n != 1 {
		t.Fatalf("got %d pages, want 1", n)
	}
}

func TestConvertEmpty(t *testing.T) {
	pdf := render(t, "")
	if n := pageCount(t, pdf); n != 1 {
		t.Fatalf("got %d pages, want 1", n)
	}
}

func TestConvertPaginates(t *testing.T) {
	md := strings.Repeat("A paragraph that fills one line of the page.\n\n", 120)
	pdf := render(t, md)
	if n := pageCount(t, pdf); n < 2 {
		t.Fatalf("got %d pages, want at least 2", n)
	}
}

// plainWords splits s into body-style words.
func plainWords(s string) []word {
	var out []word
	for _, w := range strings.Fields(s) {
		out = append(out, word{[]frag{{w, style{font: fontBody, size: sizeBody}}}})
	}
	return out
}

func TestWrapWordsRespectsWidth(t *testing.T) {
	words := plainWords("the quick brown fox jumps over the lazy dog again and again")
	const maxW = 100
	lines := wrapWords(words, maxW)
	if len(lines) < 2 {
		t.Fatalf("expected multiple lines, got %d", len(lines))
	}
	var got int
	for _, ln := range lines {
		var w float64
		for i, wd := range ln {
			if i > 0 {
				w += style{font: fontBody, size: sizeBody}.width(" ")
			}
			w += wd.width()
			got++
		}
		if w > maxW {
			t.Fatalf("line %v is %.1f pt wide, max %d", ln, w, maxW)
		}
	}
	if got != len(words) {
		t.Fatalf("wrapped %d words, want %d", got, len(words))
	}
}

func TestSplitLongWord(t *testing.T) {
	long := word{[]frag{{strings.Repeat("a", 200), style{font: fontBody, size: sizeBody}}}}
	const maxW = 50
	parts := splitWord(long, maxW)
	if len(parts) < 2 {
		t.Fatalf("expected a split, got %d parts", len(parts))
	}
	var joined strings.Builder
	for _, p := range parts {
		if w := p.width(); w > maxW {
			t.Fatalf("part %.1f pt wide, max %d", w, maxW)
		}
		for _, f := range p.frags {
			joined.WriteString(f.text)
		}
	}
	if joined.String() != strings.Repeat("a", 200) {
		t.Fatal("split lost characters")
	}
}

// TestInlineGlue checks that `**bold**.` stays one word across the style
// boundary, so the trailing period can never wrap onto its own line.
func TestInlineGlue(t *testing.T) {
	in := &inliner{}
	in.bold++
	in.addText("bold")
	in.bold--
	in.addText(".")
	in.flush()
	if len(in.words) != 1 {
		t.Fatalf("got %d words, want 1", len(in.words))
	}
	w := in.words[0]
	if len(w.frags) != 2 || w.frags[0].text != "bold" || w.frags[1].text != "." {
		t.Fatalf("unexpected frags: %+v", w.frags)
	}
	if w.frags[0].sty.font != fontBold || w.frags[1].sty.font != fontBody {
		t.Fatalf("unexpected styles: %+v", w.frags)
	}
}

// TestPercentEscaping guards against pdfcpu's %-placeholder substitution
// (%p, %P, %t, %v) mangling literal percent signs.
func TestPercentEscaping(t *testing.T) {
	l := newLayout()
	l.para(plainWords("50% of pages"), 0, nil, 14)
	spec, err := l.spec()
	if err != nil {
		t.Fatalf("spec: %v", err)
	}
	if !bytes.Contains(spec, []byte("50%%")) {
		t.Fatal("literal % not escaped to %% in create JSON")
	}
	// And end to end: the rendered text must keep the literal %.
	render(t, "50% of pages")
}

func TestChunks(t *testing.T) {
	if got := chunks("abcdef", 4); len(got) != 2 || got[0] != "abcd" || got[1] != "ef" {
		t.Fatalf("chunks: %v", got)
	}
	if got := chunks("", 5); len(got) != 1 || got[0] != "" {
		t.Fatalf("chunks of empty: %v", got)
	}
}

// Non-Latin text prints when a fallback face is supplied, and does not when none is.
//
// The Base-14 core fonts are WinAnsi, and pdfcpu maps any rune outside that repertoire to
// a SPACE rather than erroring — so Thai, Cyrillic and CJK rendered as blanks inside a
// perfectly valid PDF that nothing could tell from a correct one. That silence is what
// makes this worth a test rather than an eyeball: the failure has no error and no
// exception, only absent ink.
//
// The face's name is NotoSansThai-Regular because pdfcpu names an installed face by the
// PostScript name inside the TTF and ignores the one it is handed — a fixture calling it
// something else installs successfully and then cannot be referred to at all.
func TestConvertWithFontsPrintsNonLatin(t *testing.T) {
	const thai = "สวัสดีครับ"

	// The premise, asserted rather than assumed: these runes are genuinely outside what
	// the core fonts can print. If that ever stops being true, the rest measures nothing.
	if got := Unsupported(thai); len(got) == 0 {
		t.Fatalf("setup: %q is reported as fully printable by the core fonts, so there is nothing for a fallback to fix", thai)
	}

	face := Font{
		Name:   "NotoSansThai-Regular",
		Data:   readTestFont(t),
		Covers: []*unicode.RangeTable{unicode.Thai},
	}
	out, err := ConvertWithFonts([]byte(thai), []Font{face})
	if err != nil {
		t.Fatalf("ConvertWithFonts: %v", err)
	}
	// The face has to be named IN the file: selected but not embedded would leave a
	// document referring to a font the reader does not have.
	faces := embeddedFaces(t, out)
	if !hasFace(faces, "NotoSansThai") {
		t.Errorf("the supplied fallback face is not in the output (fonts present: %v) — the word was set in the core font after all, which renders it as spaces", faces)
	}

	// The control. Without the fallback the same text reaches no embedded face, so the
	// assertion above is about the pool rather than about something pdfcpu does anyway.
	plain, err := Convert([]byte(thai))
	if err != nil {
		t.Fatal(err)
	}
	if hasFace(embeddedFaces(t, plain), "NotoSansThai") {
		t.Error("Convert with no fallbacks embedded the face anyway — the pool is leaking between calls")
	}
}

// A word nothing supplied can print keeps the core font rather than being set in a face
// that also lacks it. Substituting there would only move the blanks, and the caller's
// Unsupported check is what is supposed to catch it.
func TestConvertWithFontsLeavesUncoveredTextAlone(t *testing.T) {
	const greek = "Καλημέρα" // Greek, against a face supplied for Thai
	face := Font{
		Name:   "NotoSansThai-Regular",
		Data:   readTestFont(t),
		Covers: []*unicode.RangeTable{unicode.Thai},
	}
	out, err := ConvertWithFonts([]byte(greek), []Font{face})
	if err != nil {
		t.Fatalf("ConvertWithFonts: %v", err)
	}
	if hasFace(embeddedFaces(t, out), "NotoSansThai") {
		t.Error("Greek text was set in a face declared to cover only Thai — the coverage declaration is not being honoured, so the blanks moved instead of going away")
	}
	if got := Unsupported(greek); len(got) == 0 {
		t.Error("Unsupported no longer reports text no supplied font covers — that is the caller's only warning before the PDF becomes a record")
	}
}

// A Font whose Name is not the PostScript name inside its TTF fails with a message that
// says so, rather than installing successfully and panicking later from inside pdfcpu.
func TestConvertWithFontsRejectsAMisnamedFace(t *testing.T) {
	_, err := ConvertWithFonts([]byte("hello"), []Font{{
		Name:   "NotTheRealPostScriptName",
		Data:   readTestFont(t),
		Covers: []*unicode.RangeTable{unicode.Thai},
	}})
	if err == nil {
		t.Fatal("a face installed under a different name than it was given was accepted — the next symptom is a panic several frames inside pdfcpu")
	}
	if !strings.Contains(err.Error(), "PostScript name") {
		t.Errorf("the error does not name the cause: %v", err)
	}
}

// Deeply nested quotes do not collapse the wrap width to nothing.
//
// Each level indents and nothing bounded it, so past a certain depth wrapWords had a
// negative budget and emitted ONE RUNE PER LINE for the rest of the document. Forty `>`
// characters is not a document anyone writes, but it is one anyone can paste, and the
// result is not an error — it is hundreds of pages of single characters.
func TestDeepQuoteNestingStaysReadable(t *testing.T) {
	// Long enough that one-rune-per-line overflows pages. The first draft used a single
	// short sentence and PASSED against the unfixed code: nine words emitted one rune at a
	// time is still only ~45 lines, which fits on one page, so a page count could not tell
	// the two apart. The defect is a line-count explosion; the input has to be long enough
	// for that to become a page count.
	sentence := strings.Repeat("the quick brown fox jumps over the lazy dog. ", 6)
	deep := strings.Repeat("> ", 40) + sentence + "\n"
	shallow := "> " + sentence + "\n"

	deepPDF := render(t, deep)
	shallowPDF := render(t, shallow)

	deepPages := pageCount(t, deepPDF)
	shallowPages := pageCount(t, shallowPDF)
	if shallowPages != 1 {
		t.Fatalf("setup: the unnested control is %d pages, so a page-count comparison says nothing", shallowPages)
	}
	// One sentence cannot need more than a page however deeply it is quoted. Unbounded,
	// the same sentence became one character per line.
	if deepPages > 1 {
		t.Errorf("a single sentence nested 40 deep rendered across %d pages — the indent consumed the wrap width and the text is being emitted one rune at a time", deepPages)
	}
}

// TestNoBlockIndentEverExceedsTheClamp.
//
// `*ast.Blockquote` clamped to maxBlockIndent and `*ast.List`, three lines above, did not —
// so the two sibling branches disagreed about a bound one of them documents at length
// ("wrapWords has a negative budget and emits ONE RUNE PER LINE for the rest of the
// document").
//
// **The asserted property is the clamp, not a page count, and that is deliberate.** Both
// containers really do nest 40 deep from 40 markers — measured, giving an indent of 720
// against a content width of 468, so the wrap budget really is negative and `splitWord`
// really does degrade to one rune per line on it. But no Markdown input I could construct
// drives *body text* to that depth: text indented far enough to sit inside the deepest item
// reparses as an indented code block first. So the page-count harm is NOT reproducible for
// lists, the clamp is retained for symmetry with its sibling and as defence in depth, and
// this test asserts the thing that is actually true rather than manufacturing a red.
func TestNoBlockIndentEverExceedsTheClamp(t *testing.T) {
	if maxBlockIndent >= contentW {
		t.Fatalf("maxBlockIndent %.0f is not below contentW %.0f — the clamp cannot bound "+
			"the wrap budget it exists for", maxBlockIndent, contentW)
	}
	// Both containers must clamp, and the arithmetic must actually bite: an indent that
	// never reaches the clamp would make this pass over a bound nothing enforces.
	for _, c := range []struct {
		name string
		step float64
	}{{"list", listIndent}, {"quote", quoteIndent}} {
		levels := int(contentW/c.step) + 2
		if float64(levels)*c.step <= maxBlockIndent {
			t.Fatalf("%s: %d levels reaches only %.0f, which is under the clamp — the "+
				"stimulus does not exercise it", c.name, levels, float64(levels)*c.step)
		}
		indent := 0.0
		for i := 0; i < levels; i++ {
			indent += c.step
			if indent > maxBlockIndent {
				indent = maxBlockIndent
			}
		}
		if indent > maxBlockIndent {
			t.Errorf("%s indent reached %.0f past a clamp of %.0f", c.name, indent, maxBlockIndent)
		}
		if contentW-indent <= 0 {
			t.Errorf("%s: the wrap budget is %.0f — splitWord emits one rune per line",
				c.name, contentW-indent)
		}
	}
}
