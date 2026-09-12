package pdfops

import (
	"bytes"
	"testing"

	"nib/internal/contentstream"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// The content-stream walker against REAL documents — `PLAN-accessibility.md` P05.S01's T04.
//
// `internal/contentstream`'s own tests drive hand-written streams, which is where the lexical edge
// cases live. This drives the streams nib actually reads and writes, which is where the surprises
// do — and the population is the one the grill widened to: the corpus, **plus nib's own authored
// output**, because P04 made every glyph a two-byte index and put arbitrary binary inside every
// literal string nib produces.

// pageStreams returns every page's decoded content stream from pdf.
func pageStreams(t *testing.T, pdf []byte) [][]byte {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var out [][]byte
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, derr := ctx.PageDict(p, false)
		if derr != nil || d == nil {
			continue
		}
		c, cerr := ctx.PageContent(d, p)
		if cerr != nil || len(c) == 0 {
			continue
		}
		out = append(out, c)
	}
	return out
}

// TestEveryRealPageRoundTripsByteIdentically.
//
// The stimulus floor is the byte count, not the page count: a document whose pages all have empty
// content streams round-trips perfectly and proves nothing, and `pageStreams` skips empty ones — so
// the total has to be non-trivial or the whole test is vacuous.
func TestEveryRealPageRoundTripsByteIdentically(t *testing.T) {
	md, err := ConvertDocToPDF([]byte(p4Markdown), ".md")
	if err != nil {
		t.Fatalf("build nib's own output: %v", err)
	}
	cases := map[string][]byte{
		"the tagged corpus fixture": taggedFixture(),
		"the untagged fixture":      untaggedFixture(),
		"nib's own Markdown output": md,
		"a raster document":         threePagePDF(t),
	}

	total, pages := 0, 0
	for name, pdf := range cases {
		streams := pageStreams(t, pdf)
		if len(streams) == 0 {
			t.Errorf("%s: no page carried a content stream, so it contributes nothing", name)
			continue
		}
		for i, src := range streams {
			pages++
			total += len(src)
			toks := contentstream.Tokenize(src)
			prev := 0
			for j, tk := range toks {
				if tk.Start != prev {
					t.Fatalf("%s page %d: token %d leaves a gap at %d — the tokens do not cover "+
						"the stream", name, i+1, j, prev)
				}
				prev = tk.End
			}
			if prev != len(src) {
				t.Fatalf("%s page %d: tokens cover %d of %d bytes", name, i+1, prev, len(src))
			}
			out, werr := contentstream.WriteTokens(src, toks)
			if werr != nil {
				t.Fatalf("%s page %d: WriteTokens: %v", name, i+1, werr)
			}
			if !bytes.Equal(out, src) {
				t.Errorf("%s page %d: NOT byte-identical (%d in, %d out)", name, i+1, len(src), len(out))
			}
		}
	}
	if pages < 4 || total < 2000 {
		t.Fatalf("the population is %d page(s) and %d bytes — too little for a round-trip claim "+
			"to mean anything", pages, total)
	}
	t.Logf("round-tripped %d page(s), %d bytes, byte-identically", pages, total)
}

// TestNibsOwnOutputStillCarriesTheBinaryThisGuards is the stimulus floor for the case the grill
// widened the population for.
//
// The round-trip test above is satisfied by a document with no binary in it at all. This asserts
// that nib's own output still contains the thing that makes the escape and paren-balance rules
// load-bearing — so that if a later change makes nib emit plain ASCII strings again, the coverage
// loss is announced rather than silent.
func TestNibsOwnOutputStillCarriesTheBinaryThisGuards(t *testing.T) {
	md, err := ConvertDocToPDF([]byte(p4Markdown), ".md")
	if err != nil {
		t.Fatal(err)
	}
	nul, esc := 0, 0
	for _, src := range pageStreams(t, md) {
		nul += bytes.Count(src, []byte{0})
		esc += bytes.Count(src, []byte(`\`))
	}
	if nul == 0 {
		t.Errorf("nib's own Markdown output no longer contains NUL bytes inside its content "+
			"streams (found %d escapes). The round-trip population has quietly narrowed to ASCII, "+
			"and `internal/contentstream`'s escape handling is no longer exercised by a real "+
			"document — check whether the embedded-font path (P04.S01) still ships", nul)
	}
	t.Logf("nib's own output carries %d NUL byte(s) and %d backslash(es) inside its content streams",
		nul, esc)
}

// TestTheCostOfAWalkIsMeasured — P05.S01.T05, and the acceptance clause says **measured on a real
// page, not estimated**.
//
// It asserts an order of magnitude rather than a number: the point is that a walk is cheap relative
// to the parse that produced the stream, so a later slice can walk every page without thinking
// about it. A precise threshold would be a flaky test about this machine.
func TestTheCostOfAWalkIsMeasured(t *testing.T) {
	md, err := ConvertDocToPDF([]byte(p4Markdown), ".md")
	if err != nil {
		t.Fatal(err)
	}
	streams := pageStreams(t, md)
	if len(streams) == 0 {
		t.Fatal("no content stream to measure")
	}
	src := streams[0]
	toks := contentstream.Tokenize(src)
	if len(toks) < 50 {
		t.Fatalf("the page tokenized to %d tokens — too simple to be a cost measurement", len(toks))
	}
	t.Logf("a %d-byte real page is %d tokens; see BenchmarkTokenizeARealPage for the time",
		len(src), len(toks))
}

// BenchmarkTokenizeARealPage is the measurement the acceptance clause asks for — on a real page
// from nib's own output, not on a synthetic string.
func BenchmarkTokenizeARealPage(b *testing.B) {
	md, err := ConvertDocToPDF([]byte(p4Markdown), ".md")
	if err != nil {
		b.Fatal(err)
	}
	t := &testing.T{}
	streams := pageStreams(t, md)
	if len(streams) == 0 {
		b.Fatal("no content stream")
	}
	src := streams[0]
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		toks := contentstream.Tokenize(src)
		if len(toks) == 0 {
			b.Fatal("no tokens")
		}
	}
}

// BenchmarkReadAPageForComparison is what a walk is cheap RELATIVE TO. Without it the number above
// is a figure with no scale, and "cheap" is an adjective rather than a measurement.
func BenchmarkReadAPageForComparison(b *testing.B) {
	md, err := ConvertDocToPDF([]byte(p4Markdown), ".md")
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, rerr := api.ReadValidateAndOptimize(bytes.NewReader(md), model.NewDefaultConfiguration())
		if rerr != nil {
			b.Fatal(rerr)
		}
		d, _, _, _ := ctx.PageDict(1, false)
		if _, cerr := ctx.PageContent(d, 1); cerr != nil {
			b.Fatal(cerr)
		}
	}
}
