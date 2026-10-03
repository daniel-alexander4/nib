package uacheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/filter"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// TestAFontStreamIsDecodedOnlyToItsCeiling — /pending 807 R6.
//
// Nine font-program and CMap readers called a bare `sd.Decode()`, which inflates to pdfcpu's 512 MiB default and
// keeps it for the whole check. Through the door, a stream past its ceiling is refused DURING the decode —
// allocating about the ceiling, not the stream — and a document that has spent its budget decodes nothing more.
func TestAFontStreamIsDecodedOnlyToItsCeiling(t *testing.T) {
	stream := func(size int) *types.StreamDict {
		sd := types.NewStreamDict(types.Dict{}, 0, nil, nil, []types.PDFFilter{{Name: filter.Flate}})
		sd.Raw = flatedSpacesUA(size)
		return &sd
	}

	big := stream(maxFontStreamDecoded + 32<<20)
	d := &Document{}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	why := d.decodeFontStream(big, "its program stream")
	runtime.ReadMemStats(&after)
	if why == "" || big.Content != nil {
		t.Fatalf("a %d MiB font stream decoded (%d bytes held) — past the %d MiB ceiling it must be refused",
			(maxFontStreamDecoded+32<<20)>>20, len(big.Content), maxFontStreamDecoded>>20)
	}
	// The decoder's buffer doubles toward the ceiling, so the cumulative allocation is a few times it — bounded by
	// the ceiling, never by the stream.
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 5*maxFontStreamDecoded {
		t.Errorf("refusing it allocated %d MiB, want a bound set by the %d MiB ceiling", alloc>>20, maxFontStreamDecoded>>20)
	}
	// A refusal is remembered: the stream is not inflated again for the next font that names it.
	runtime.ReadMemStats(&before)
	if again := d.decodeFontStream(big, "its program stream"); again != why {
		t.Errorf("the second ask answered %q, want the remembered %q", again, why)
	}
	runtime.ReadMemStats(&after)
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 1<<20 {
		t.Errorf("asking again for a refused stream allocated %d MiB", alloc>>20)
	}

	// The document budget: with a mebibyte left, a 16 MiB stream — well under its own ceiling — is refused.
	spent := &Document{fontDecoded: maxFontBytesDecoded - 1<<20}
	if why := spent.decodeFontStream(stream(16<<20), "an embedded CMap"); !strings.Contains(why, "have used") {
		t.Errorf("with the budget spent a 16 MiB CMap answered %q, want the document-budget refusal", why)
	}

	// The control: a stream under both decodes whole and is charged.
	ok := stream(1 << 20)
	ctl := &Document{}
	if why := ctl.decodeFontStream(ok, "the glyph procedure"); why != "" || len(ok.Content) != 1<<20 {
		t.Fatalf("control: a 1 MiB stream answered %q with %d bytes, want it whole", why, len(ok.Content))
	}
	if ctl.fontDecoded != 1<<20 {
		t.Errorf("control: the decode charged %d bytes, want %d", ctl.fontDecoded, 1<<20)
	}
}

// TestNoCheckerReaderDecodesAStreamUncapped — /pending 807 R6's census.
//
// The defect was ADR-009's shape: a capped door existed and nine sites did not use it. This walks the package's
// production sources and refuses any call to pdfcpu's uncapped `Decode()` or a hand-rolled `DecodeWithLimit` —
// a new reader goes through `decodeFontStream`, `decodeWithin` or `decodedContent`.
func TestNoCheckerReaderDecodesAStreamUncapped(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	files := 0
	var bad []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files++
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if (sel.Sel.Name == "Decode" && len(call.Args) == 0) || sel.Sel.Name == "DecodeWithLimit" {
				bad = append(bad, fset.Position(call.Pos()).String())
			}
			return true
		})
	}
	if files < 20 {
		t.Fatalf("setup: the census read %d source files, so it is not reading this package", files)
	}
	if len(bad) > 0 {
		t.Errorf("uncapped stream decodes in the checker — route each through decodeFontStream, decodeWithin "+
			"or decodedContent:\n  %s", strings.Join(bad, "\n  "))
	}
}
