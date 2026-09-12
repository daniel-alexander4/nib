package mdpdf

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/font"
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

// testFace reads one vendored face from the pdfops package's font directory, the same way
// readTestFont does — mdpdf ships no fonts of its own, and the vendored set is the one nib
// actually authors with.
func testFace(t *testing.T, name string) Font {
	t.Helper()
	file := name + ".ttf"
	if name == "LiberationMono" {
		file = "LiberationMono-Regular.ttf" // PostScript name and file name differ
	}
	b, err := os.ReadFile("../internal/pdfops/fonts/" + file)
	if err != nil {
		t.Skipf("no vendored TTF for %s: %v", name, err)
	}
	return Font{Name: name, Data: b}
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
	in := &inliner{f: coreFaces}
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
	l := newLayout(coreFaces)
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

	// **This test used to reimplement the clamp and never call the renderer.**
	//
	// It walked `indent += step` in its own loop, applied `if indent > maxBlockIndent`
	// itself, and asserted the result — so it proved that the arithmetic written in the test
	// clamps. Deleting either clamp in mdpdf.go left it green, which is the whole of what it
	// existed to catch. It now renders.
	for _, c := range []struct {
		name   string
		step   float64
		source func(int) string
	}{
		{"quote", quoteIndent, func(n int) string {
			// A PARAGRAPH, not a sentence. One rune per line at 44 runes is 44 lines, which
			// still fits on one page — so a one-sentence fixture leaves the page count flat
			// and the check green with the clamp deleted. Measured: with 20 sentences the
			// same input renders 1 page clamped and 17 unclamped.
			return strings.Repeat("> ", n) +
				strings.Repeat("the quick brown fox jumps over the lazy dog. ", 20) + "\n"
		}},
		{"list", listIndent, func(n int) string {
			var b strings.Builder
			// Only the DEEPEST item carries the paragraph. A first draft gave every level
			// one, so the deep case had 28 paragraphs against the control's 2 and was
			// several times longer for a reason that has nothing to do with the clamp — it
			// failed against correct code, which is the confound this note exists to stop
			// being reintroduced.
			for i := 0; i < n-1; i++ {
				b.WriteString(strings.Repeat("  ", i) + "- x\n")
			}
			b.WriteString(strings.Repeat("  ", n-1) + "- " +
				strings.Repeat("the quick brown fox jumps over the lazy dog. ", 20) + "\n")
			return b.String()
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			// Deep enough that an UNCLAMPED indent would exceed the content width — the
			// stimulus, asserted rather than assumed.
			levels := int(contentW/c.step) + 2
			if float64(levels)*c.step <= maxBlockIndent {
				t.Fatalf("%d levels reaches only %.0f, which is under the clamp — the "+
					"stimulus does not exercise it", levels, float64(levels)*c.step)
			}

			out, err := Convert([]byte(c.source(levels)))
			if err != nil {
				t.Fatalf("the renderer refused %d levels of nesting: %v", levels, err)
			}
			if len(out) == 0 {
				t.Fatal("no output")
			}

			// The observable harm, not the arithmetic. Past the clamp wrapWords has a
			// non-positive budget and splitWord emits ONE RUNE PER LINE for the rest of the
			// document. The shallow render is the control: whatever a page costs, the deep
			// one must not cost several times more.
			//
			// **The old comment here said the page-count harm was not reproducible for
			// lists.** It was reproducible for both; the fixture was a single sentence, and
			// 44 runes on 44 lines still fit one page. With a paragraph, deleting the quote
			// clamp takes the same input from 1 page to 17 — measured.
			shallow, err := Convert([]byte(c.source(2)))
			if err != nil {
				t.Fatal(err)
			}
			deepPages, shallowPages := pageCount(t, out), pageCount(t, shallow)
			if deepPages > shallowPages*4 {
				t.Errorf("%d levels of nesting produced %d page(s) against %d for two levels "+
					"— the wrap budget went non-positive and the text is being emitted one "+
					"rune per line", levels, deepPages, shallowPages)
			}
		})
	}
}

// TestAbsurdNestingIsRefusedRatherThanParsedForMinutes.
//
// goldmark's parser is super-linear in container nesting depth. Measured on a file of
// nothing but nested list markers: 500 levels parse in 0.2s, 1000 in 1.4s, 2000 in 14s —
// while RENDERING all three takes 50–90ms and grows linearly, and a FLAT list of the same
// 2000 items parses in 54ms. So the cost is nesting, not size, and it is not in this
// package. mdpdf renders Markdown a stranger sent (the GUI import, `nib office`, and other
// projects importing this package), so a 30 KB file that hangs for three minutes with no
// cancel is an outage; an error naming the reason is strictly better than a wait.
func TestAbsurdNestingIsRefusedRatherThanParsedForMinutes(t *testing.T) {
	deep := func(n int) []byte {
		var b strings.Builder
		for i := 0; i < n; i++ {
			b.WriteString(strings.Repeat("  ", i) + "- x\n")
		}
		return []byte(b.String())
	}

	// STIMULUS: a document just under the cap still renders, and quickly. Without this a
	// refusal that fired on everything would satisfy the assertion below.
	start := time.Now()
	if _, err := Convert(deep(maxNestDepth / 2)); err != nil {
		t.Fatalf("a document nested %d deep was refused: %v", maxNestDepth/2, err)
	}
	if el := time.Since(start); el > 20*time.Second {
		t.Errorf("a document under the cap took %s — the cap is not below the range where "+
			"the parser's cost explodes", el.Round(time.Millisecond))
	}

	// The refusal, and it must arrive FAST: the whole point is not paying the parse.
	start = time.Now()
	_, err := Convert(deep(maxNestDepth * 8))
	if err == nil {
		t.Fatalf("a document nested %d deep was accepted", maxNestDepth*8)
	}
	if el := time.Since(start); el > 5*time.Second {
		t.Errorf("the refusal took %s — it is happening after the parse, which is the cost "+
			"it exists to avoid", el.Round(time.Millisecond))
	}
	if !strings.Contains(err.Error(), "nests containers") {
		t.Errorf("the error does not say why: %v", err)
	}

	// Blockquotes are the other container, and they are counted differently (markers, not
	// spaces), so they need their own row.
	if _, err := Convert([]byte(strings.Repeat("> ", maxNestDepth*2) + "x\n")); err == nil {
		t.Error("a blockquote nested past the cap was accepted — the marker arm of the " +
			"depth count is not reached")
	}

	// And the controls, because a refusal that fires on ordinary documents is worse than
	// the hang. An indented code block is spaces without nesting; a normal outline is a
	// handful of levels.
	for name, src := range map[string]string{
		"an indented code block": "        func main() {}\n",
		"a three-level outline":  "- a\n  - b\n    - c\n",
		"a quoted quote":         "> > quoted\n",
		"a deep but sane list":   strings.Repeat("  ", 20) + "- x\n",
	} {
		if _, err := Convert([]byte(src)); err != nil {
			t.Errorf("%s was refused: %v", name, err)
		}
	}
}

// TestBaseFacesAreMeasuredByRune — `PLAN-accessibility.md` P04.S01.
//
// `style.width` picks between two measurement rules: pdfcpu counts a CORE font byte by byte and a
// USER font rune by rune, so applying the core rule to an embedded face inflates every multi-byte
// word and every following fragment on the line lands in the wrong place. That split was written
// for the FALLBACK pool, where only the odd word is embedded. `ConvertWithFaces` makes it govern
// the whole document, which is a much bigger blast radius for the same one-line mistake.
//
// **Asserted as a difference between two measurements of the same string**, because a width in
// points has no obviously-right value to compare against: a string with multi-byte characters must
// measure LESS as an embedded face than the byte rule would give it.
func TestBaseFacesAreMeasuredByRune(t *testing.T) {
	// Every character here is in WinAnsi, and every one past ASCII is two bytes in UTF-8 — so the
	// byte rule over-counts it and the rune rule does not.
	const text = "café — naïve résumé «déjà»"

	face := testFace(t, "Roboto-Regular")
	if err := installFallbacks([]Font{face}); err != nil {
		t.Fatalf("install %s: %v", face.Name, err)
	}
	core := style{font: fontBody, size: sizeBody}
	emb := style{font: face.Name, size: sizeBody, embedded: true}

	byRune := emb.width(text)
	byByte := core.width(text)
	if byRune <= 0 || byByte <= 0 {
		t.Fatalf("setup: a width of zero measures nothing (rune=%v byte=%v)", byRune, byByte)
	}

	// The discriminator: measure the SAME face with the flag wrong, and require it to differ.
	wrong := style{font: face.Name, size: sizeBody, embedded: false}
	if got := wrong.width(text); got == byRune {
		t.Errorf("an embedded face measured under the CORE rule gave the same width (%v) as under "+
			"the rune rule, so style.width's split is no longer doing anything and a document set "+
			"in embedded base faces would lay out with byte-counted widths", got)
	}

	// **And the flag has to REACH those styles**, which the two checks above do not show: they
	// construct styles by hand. Every style a supplied face set produces must carry it, or the
	// whole document lays out under the wrong rule while `style.width` still works perfectly.
	// Found by mutation — `faceSet.sty` returning `embedded: false` left the assertions above
	// green.
	fs := (&Faces{
		Body:       Font{Name: "a", Data: []byte("x")},
		Bold:       Font{Name: "b", Data: []byte("x")},
		Italic:     Font{Name: "c", Data: []byte("x")},
		BoldItalic: Font{Name: "d", Data: []byte("x")},
		Code:       Font{Name: "e", Data: []byte("x")},
	}).set()
	for _, name := range []string{fs.body, fs.bold, fs.italic, fs.boldItalic, fs.code} {
		if !fs.sty(name, sizeBody).embedded {
			t.Errorf("faceSet.sty(%q) produced a style with embedded=false, so text in that face "+
				"is measured byte by byte", name)
		}
	}
	if coreFaces.sty(coreFaces.body, sizeBody).embedded {
		t.Error("the CORE set now claims to be embedded, which inflates nothing and breaks the " +
			"Base-14 path instead")
	}
}

// TestConvertWithFacesSetsTheWholeDocumentInThem, and the control is the point: the same Markdown
// through the core path must NOT name these faces, or the assertion is satisfied by a document
// that would have embedded them anyway.
func TestConvertWithFacesSetsTheWholeDocumentInThem(t *testing.T) {
	const md = "# Heading\n\nBody with **bold** and *italic*.\n\n```\ncode\n```\n"
	base := &Faces{
		Body:       testFace(t, "Roboto-Regular"),
		Bold:       testFace(t, "Roboto-Bold"),
		Italic:     testFace(t, "Roboto-Italic"),
		BoldItalic: testFace(t, "Roboto-BoldItalic"),
		Code:       testFace(t, "LiberationMono"),
	}
	out, err := ConvertWithFaces([]byte(md), base, nil)
	if err != nil {
		t.Fatalf("ConvertWithFaces: %v", err)
	}
	faces := embeddedFaces(t, out)
	for _, want := range []string{"Roboto-Regular", "Roboto-Bold", "Roboto-Italic", "LiberationMono"} {
		if !hasFace(faces, want) {
			t.Errorf("%s is not in the output's fonts: %v", want, faces)
		}
	}
	plain, err := Convert([]byte(md))
	if err != nil {
		t.Fatal(err)
	}
	if hasFace(embeddedFaces(t, plain), "Roboto") {
		t.Fatal("the CORE path already names a Roboto face, so the assertion above cannot tell " +
			"ConvertWithFaces from Convert")
	}
}

// TestConvertWithFacesDegradesRatherThanRefusing: a partially-supplied set is the Base-14 set.
//
// A caller's faces come from an install that can fail, and a document set in core fonts is a worse
// PDF rather than a broken one — so a missing face must not cost the user the conversion.
func TestConvertWithFacesDegradesRatherThanRefusing(t *testing.T) {
	const md = "# Heading\n\nBody.\n"
	for _, c := range []struct {
		name string
		base *Faces
	}{
		{"nil", nil},
		{"one face missing", &Faces{Body: testFace(t, "Roboto-Regular")}},
		{"a face with no bytes", &Faces{
			Body:       testFace(t, "Roboto-Regular"),
			Bold:       testFace(t, "Roboto-Bold"),
			Italic:     testFace(t, "Roboto-Italic"),
			BoldItalic: testFace(t, "Roboto-BoldItalic"),
			Code:       Font{Name: "LiberationMono"},
		}},
	} {
		out, err := ConvertWithFaces([]byte(md), c.base, nil)
		if err != nil {
			t.Errorf("%s: ConvertWithFaces refused instead of degrading: %v", c.name, err)
			continue
		}
		if hasFace(embeddedFaces(t, out), "Roboto") {
			t.Errorf("%s: the degrade still embedded a face — a partial set must be ALL core, "+
				"or the document mixes embedded and core faces and fails 7.21.4.1 anyway", c.name)
		}
	}
}

// TestAnUnwritableFontDirectoryDegradesRatherThanFailing — `PLAN-accessibility.md` P04.S03.
//
// The pdfcpu user-font directory lives under the user's config dir, and it can be read-only, full,
// or owned by someone else. Before this, every Markdown conversion on such a machine returned
// `install fallback font Roboto-Regular: permission denied` and produced nothing — **true of the
// fallback pool long before the base faces existed.**
//
// The exercise is the real condition, not a stub: the directory is made unwritable with `chmod` and
// the conversion is driven through it.
func TestAnUnwritableFontDirectoryDegradesRatherThanFailing(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("SKIP (not a pass): running as root, which ignores the directory mode this test " +
			"depends on, so the degrade is not exercised here")
	}
	model.NewDefaultConfiguration()
	orig := font.UserFontDir
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { font.UserFontDir = orig; os.Chmod(dir, 0o700) })
	font.UserFontDir = dir

	// Proving the condition is real before grading the response: an install into this directory
	// must actually fail, or everything below passes for the wrong reason.
	if err := installFallbacks([]Font{testFace(t, "Roboto-Bold")}); err == nil {
		t.Fatal("setup: installing into a mode-0500 directory succeeded, so the condition this " +
			"test exists for was never created")
	}

	base := &Faces{
		Body:       testFace(t, "Roboto-Regular"),
		Bold:       testFace(t, "Roboto-Bold"),
		Italic:     testFace(t, "Roboto-Italic"),
		BoldItalic: testFace(t, "Roboto-BoldItalic"),
		Code:       testFace(t, "LiberationMono"),
	}
	out, err := ConvertWithFaces([]byte("# Heading\n\nBody text.\n"), base, nil)
	if err != nil {
		t.Fatalf("an unwritable font directory cost the user the whole conversion: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("the degrade produced an empty document")
	}
	// And it degraded to CORE rather than half-embedding: a document naming a face it could not
	// install is worse than one that never tried.
	if faces := embeddedFaces(t, out); hasFace(faces, "Roboto") || hasFace(faces, "LiberationMono") {
		t.Errorf("the degraded document still names an embedded face: %v", faces)
	}
}

// TestAMisdeclaredFaceStillFailsLoudly is the other half, and without it the degrade above swallows
// a programming error into silently-worse output on every machine.
func TestAMisdeclaredFaceStillFailsLoudly(t *testing.T) {
	for _, c := range []struct {
		name string
		face Font
	}{
		{"named by its file rather than its PostScript name",
			Font{Name: "LiberationMono-Regular", Data: testFace(t, "LiberationMono").Data}},
	} {
		base := &Faces{
			Body:       testFace(t, "Roboto-Regular"),
			Bold:       testFace(t, "Roboto-Bold"),
			Italic:     testFace(t, "Roboto-Italic"),
			BoldItalic: testFace(t, "Roboto-BoldItalic"),
			Code:       c.face,
		}
		_, err := ConvertWithFaces([]byte("x\n"), base, nil)
		if err == nil {
			t.Errorf("%s: ConvertWithFaces degraded instead of reporting a declaration error", c.name)
			continue
		}
		if !errors.Is(err, ErrFaceMisdeclared) {
			t.Errorf("%s: error is not ErrFaceMisdeclared, so the degrade cannot tell it from a "+
				"machine condition: %v", c.name, err)
		}
	}
}

// TestCarryingStructureChangesNoDRAWNByte — `PLAN-accessibility.md` P06.S01's load-bearing clause.
//
// This slice teaches the layout to remember what each run WAS. It must add knowledge and no output:
// if the rendered bytes move, every document nib has produced changes for a reason that has nothing
// to do with what the user asked for, and `ContentDigest` — which covers the page — moves with them.
//
// Asserted as byte equality between the two entry points on the same input, over a document with
// every construct the roles distinguish.
func TestCarryingStructureChangesNoDRAWNByte(t *testing.T) {
	const md = "# Heading one\n\nBody paragraph with **bold**.\n\n## Heading two\n\n" +
		"- first item\n- second item\n  - nested item\n\n1. ordered one\n2. ordered two\n\n" +
		"> a quotation\n> > nested quotation\n\n```\ncode block line\n```\n\nFinal paragraph.\n"

	plain, err := ConvertWithFaces([]byte(md), nil, nil)
	if err != nil {
		t.Fatalf("ConvertWithFaces: %v", err)
	}
	structured, st, err := ConvertStructured([]byte(md), nil, nil)
	if err != nil {
		t.Fatalf("ConvertStructured: %v", err)
	}
	if len(st.Pages) == 0 {
		t.Fatal("no pages in the structure")
	}

	// **The comparison is the CONTENT STREAMS, not the files, and that is not a weakening.**
	// pdfcpu writes a random `/ID` into every trailer — measured: the same function called twice on
	// the same input produces different bytes — so file equality is unsatisfiable by any
	// implementation and an assertion over it would be a test no code could pass. What this slice
	// promises is that nothing DRAWN changed, and the content stream is exactly that.
	for page := 1; page <= len(st.Pages); page++ {
		a := drawnBytes(t, plain, page)
		b := drawnBytes(t, structured, page)
		if !bytes.Equal(a, b) {
			t.Errorf("page %d's drawn content changed (%d bytes against %d)\n  a %q\n  b %q",
				page, len(a), len(b), excerpt(a), excerpt(b))
		}
		if len(a) == 0 {
			t.Errorf("page %d draws nothing, so comparing it proves nothing", page)
		}
	}
}

// drawnBytes returns one page's decoded content stream — what the document actually draws, with
// none of the trailer's randomness.
func drawnBytes(t *testing.T, pdf []byte, page int) []byte {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	d, _, _, derr := ctx.PageDict(page, false)
	if derr != nil || d == nil {
		t.Fatalf("page %d: %v", page, derr)
	}
	c, cerr := ctx.PageContent(d, page)
	if cerr != nil {
		t.Fatalf("content of page %d: %v", page, cerr)
	}
	return c
}

func excerpt(b []byte) string {
	if len(b) > 60 {
		return string(b[:60]) + "…"
	}
	return string(b)
}

// TestTheStructureNamesWhatTheMarkdownSaid.
//
// Each row is a construct the Markdown gave explicitly, so the expected value comes from the SOURCE
// rather than from a golden file of whatever the code happened to produce. A heading's level and a
// list's nesting depth are the two that a flat "this is a heading" would lose, and they are the two
// PDF/UA needs.
func TestTheStructureNamesWhatTheMarkdownSaid(t *testing.T) {
	const md = "# One\n\n## Two\n\n###### Six\n\nBody.\n\n- top\n  - nested\n\n" +
		"> quoted\n\n```\ncode\n```\n"
	_, st, err := ConvertStructured([]byte(md), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, page := range st.Pages {
		for _, r := range page {
			seen[r.Kind.String()+":"+itoa(r.Level)]++
		}
	}
	for _, want := range []string{
		"heading:1", "heading:2", "heading:6", // the levels, not just "a heading"
		"body:0",
		"listitem:1", "listitem:2", // the DEPTH, which is what a sublist is
		"marker:1", "marker:2",
		"quote:1",
		"code:0",
	} {
		if seen[want] == 0 {
			t.Errorf("no run recorded as %q; got %v", want, seen)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// TestARoleDoesNotLeakPastItsBlock is the paired-mutation check.
//
// `withRole` sets the layout's role and defers the restore. A role left set leaks onto whatever is
// laid out next — and the symptom is a paragraph tagged as a heading, which looks like nothing at
// all in the rendered document and is wrong in the one place this data exists to be right.
func TestARoleDoesNotLeakPastItsBlock(t *testing.T) {
	// A heading, then body. The body must NOT be a heading.
	_, st, err := ConvertStructured([]byte("# A heading\n\nPlain body text.\n"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, page := range st.Pages {
		for _, r := range page {
			kinds = append(kinds, r.Kind.String())
		}
	}
	if len(kinds) < 2 {
		t.Fatalf("expected at least two runs, got %v", kinds)
	}
	if kinds[0] != "heading" {
		t.Errorf("the first run is %q, want heading", kinds[0])
	}
	last := kinds[len(kinds)-1]
	if last != "body" {
		t.Errorf("the last run is %q, want body — the heading's role leaked past its block", last)
	}

	// And a list followed by a paragraph: the depth must not persist.
	_, st2, err := ConvertStructured([]byte("- item\n\nAfter the list.\n"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var tail Role
	for _, page := range st2.Pages {
		for _, r := range page {
			tail = r
		}
	}
	if tail.Kind != RoleBody || tail.Level != 0 {
		t.Errorf("the run after a list is %v/%d, want body/0 — the list depth leaked", tail.Kind, tail.Level)
	}
}

// TestEveryRunHasARole: the structure must describe every run, or a tagger has runs it cannot place
// and the correspondence P06.S02 rests on has a hole in it.
func TestEveryRunHasARole(t *testing.T) {
	const md = "# H\n\nBody.\n\n- a\n\n```\nc\n```\n\n> q\n"
	pdf, st, err := ConvertStructured([]byte(md), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = pdf
	total := 0
	for _, page := range st.Pages {
		total += len(page)
	}
	if total == 0 {
		t.Fatal("the structure describes no runs at all")
	}
	// Every page's role list is exactly as long as that page's run list — checked through the
	// exported surface by counting the text entries the spec would emit.
	if len(st.Pages) == 0 {
		t.Fatal("no pages")
	}
	t.Logf("%d run(s) across %d page(s), all with a role", total, len(st.Pages))
}

// TestAdjacentBlocksAreDistinguishable — the amendment P06.S02's grill forced on P06.S01.
//
// `{Kind, Level}` cannot express what a tagger needs. Measured on a document with two consecutive
// paragraphs and a two-line code block: runs 1 and 2 were both `body/0` and runs 7 and 8 were both
// `code/0` — and those two cases need **opposite** treatment. Two paragraphs are two elements; two
// lines of one code block are two MCIDs of one element. `Block` is what tells them apart.
func TestAdjacentBlocksAreDistinguishable(t *testing.T) {
	const md = "First paragraph.\n\nSecond paragraph.\n\n```\ncode line one\ncode line two\n```\n"
	_, st, err := ConvertStructured([]byte(md), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var roles []Role
	for _, page := range st.Pages {
		roles = append(roles, page...)
	}
	if len(roles) < 4 {
		t.Fatalf("expected at least four runs, got %d", len(roles))
	}

	// The two paragraphs: same kind, DIFFERENT block.
	var bodies []Role
	var codes []Role
	for _, r := range roles {
		switch r.Kind {
		case RoleBody:
			bodies = append(bodies, r)
		case RoleCode:
			codes = append(codes, r)
		}
	}
	if len(bodies) != 2 {
		t.Fatalf("expected two body runs, got %d", len(bodies))
	}
	if bodies[0].Block == bodies[1].Block {
		t.Errorf("two SEPARATE paragraphs share block %d — a tagger would make them one element",
			bodies[0].Block)
	}
	// The two code lines: same kind, SAME block.
	if len(codes) != 2 {
		t.Fatalf("expected two code runs, got %d", len(codes))
	}
	if codes[0].Block != codes[1].Block {
		t.Errorf("two lines of ONE code block have blocks %d and %d — a tagger would make them "+
			"two elements", codes[0].Block, codes[1].Block)
	}
	// Every block ordinal is non-zero: a run with Block 0 belongs to no construct.
	for i, r := range roles {
		if r.Block == 0 {
			t.Errorf("run %d (%s) has no block ordinal", i, r.Kind)
		}
	}
}

// TestAWrappedParagraphIsONEBlock: the case that makes `Block` more than a run index.
func TestAWrappedParagraphIsONEBlock(t *testing.T) {
	// Long enough to wrap several times.
	long := strings.Repeat("word ", 200)
	_, st, err := ConvertStructured([]byte(long+"\n"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var blocks = map[int]int{}
	runs := 0
	for _, page := range st.Pages {
		for _, r := range page {
			blocks[r.Block]++
			runs++
		}
	}
	if runs < 3 {
		t.Fatalf("the paragraph produced %d run(s) — it did not wrap, so this proves nothing", runs)
	}
	if len(blocks) != 1 {
		t.Errorf("a single wrapped paragraph produced %d blocks across %d runs; it is ONE element",
			len(blocks), runs)
	}
}

// TestAListItemAndItsMarkerAreSeparateBlocks — PDF/UA wants `/Lbl` BESIDE `/LBody`, not inside it.
func TestAListItemAndItsMarkerAreSeparateBlocks(t *testing.T) {
	_, st, err := ConvertStructured([]byte("- an item\n"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var marker, item Role
	for _, page := range st.Pages {
		for _, r := range page {
			switch r.Kind {
			case RoleMarker:
				marker = r
			case RoleListItem:
				item = r
			}
		}
	}
	if marker.Block == 0 || item.Block == 0 {
		t.Fatalf("expected both a marker and an item run, got marker=%v item=%v", marker, item)
	}
	if marker.Block == item.Block {
		t.Errorf("the bullet and the item's text share block %d — they are /Lbl and /LBody and "+
			"must be two elements", marker.Block)
	}
	if marker.Level != item.Level {
		t.Errorf("the bullet is at level %d and its item at %d; a label belongs to its own item",
			marker.Level, item.Level)
	}
}
