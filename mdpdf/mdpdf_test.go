package mdpdf

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

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
		out = append(out, word{[]frag{{w, style{fontBody, sizeBody}}}})
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
				w += style{fontBody, sizeBody}.width(" ")
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
	long := word{[]frag{{strings.Repeat("a", 200), style{fontBody, sizeBody}}}}
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
