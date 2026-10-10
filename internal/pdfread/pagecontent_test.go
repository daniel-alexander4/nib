package pdfread_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/contentstream"
	"nib/internal/pdfread"
	"nib/internal/scaling"
	"nib/internal/testpdf"
)

// meaning is a stream's tokens with the white-space dropped — what a reader acts on. Two joins that differ only
// in white-space mean the same page.
func meaning(src []byte) []string {
	var out []string
	for _, t := range contentstream.Tokenize(src) {
		if t.Kind != contentstream.Whitespace {
			out = append(out, string(t.Bytes(src)))
		}
	}
	return out
}

func sameMeaning(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestAContentsArrayIsJoinedAtTokenBoundaries — ADR-056, `PLAN-text-reflow.md` P05.S01. A `/Contents` array may
// divide a page at any token boundary; pdfcpu's join fuses two of the legal divisions, and this door must not.
//
// **The stimulus is asserted first**: each fused shape is checked to MISREAD through pdfcpu's own join, so a
// fixture that stopped dividing where it claims cannot pass by having nothing to repair.
func TestAContentsArrayIsJoinedAtTokenBoundaries(t *testing.T) {
	for _, c := range []struct {
		name  string
		shape testpdf.JoinShape
		fused bool
	}{
		{"a regular byte meets a regular byte (`Tj` + `ET`)", testpdf.JoinRegular, true},
		{"the first stream ends inside a comment", testpdf.JoinComment, true},
		{"the first stream ends on white-space", testpdf.JoinSafe, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			pdf, meant, err := testpdf.SplitContents("Hello, world", c.shape)
			if err != nil {
				t.Fatal(err)
			}
			ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
			if err != nil {
				t.Fatal(err)
			}
			page, _, _, err := ctx.PageDict(1, false)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := ctx.PageContent(page, 1)
			if err != nil {
				t.Fatal(err)
			}
			if got := !sameMeaning(meaning(raw), meaning(meant)); got != c.fused {
				t.Fatalf("the fixture does not do what it is named for: pdfcpu's join misreads it = %v, want %v\n  raw %q",
					got, c.fused, raw)
			}
			door, err := pdfread.PageContent(ctx, page, 1)
			if err != nil {
				t.Fatal(err)
			}
			if !sameMeaning(meaning(door), meaning(meant)) {
				t.Errorf("the door's join does not mean the page it was divided from\n  door  %q\n  meant %q", door, meant)
			}
			if !c.fused && !bytes.Equal(door, raw) {
				t.Errorf("a join pdfcpu already reads correctly came back changed — the door must be byte-identical "+
					"to pdfcpu wherever pdfcpu is right\n  door %q\n  raw  %q", door, raw)
			}
		})
	}
}

// TestASingleStreamIsPdfcpusOwnRead — the common case passes straight through, byte for byte.
func TestASingleStreamIsPdfcpusOwnRead(t *testing.T) {
	pdf, err := testpdf.Text("one stream")
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	page, _, _, err := ctx.PageDict(1, false)
	if err != nil {
		t.Fatal(err)
	}
	raw, rerr := ctx.PageContent(page, 1)
	door, derr := pdfread.PageContent(ctx, page, 1)
	if rerr != nil || derr != nil || len(raw) == 0 || !bytes.Equal(raw, door) {
		t.Fatalf("a single stream must read exactly as pdfcpu reads it: raw %d bytes (%v), door %d bytes (%v)",
			len(raw), rerr, len(door), derr)
	}
	delete(page, "Contents")
	if _, err := pdfread.PageContent(ctx, page, 1); err != model.ErrNoContent {
		t.Errorf("a page with no /Contents must be model.ErrNoContent, as pdfcpu's is; got %v", err)
	}
}

// dividedPage is testpdf.Text's page with its /Contents replaced by n copies of chunk, each its own stream.
func dividedPage(t *testing.T, n int, chunk []byte) (*model.Context, types.Dict) {
	t.Helper()
	pdf, err := testpdf.Text("x")
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	page, _, _, err := ctx.PageDict(1, false)
	if err != nil {
		t.Fatal(err)
	}
	var arr types.Array
	for i := 0; i < n; i++ {
		sd, err := ctx.NewStreamDictForBuf(append([]byte{}, chunk...))
		if err != nil {
			t.Fatal(err)
		}
		if err := sd.Encode(); err != nil {
			t.Fatal(err)
		}
		ref, err := ctx.IndRefForNewObject(*sd)
		if err != nil {
			t.Fatal(err)
		}
		arr = append(arr, *ref)
	}
	page["Contents"] = arr
	return ctx, page
}

// TestTheJoinIsLinearInThePage — the P05.S01 review's critical: the separator test was handed the whole join so far,
// so every join whose prefix held a `%` re-tokenized the page — 13.8 s for 1,000 streams, ~3 minutes for 4,000,
// against pdfcpu's 10 ms. Each chunk ends in a space and holds `%` inside a string, which is the shape that reaches
// the tokenize at every join.
//
// Bounded by SCALING, not by the clock (/pending 841): it was "1,000 streams within 2 s", which measures the machine.
// Four times the streams cost a linear join ×4 and the quadratic one ×16; ×8 is the midpoint on a log scale, and the
// sizes are interleaved through `scaling`.
func TestTheJoinIsLinearInThePage(t *testing.T) {
	chunk := []byte(strings.Repeat("(5%) Tj ", 100))
	type fixture struct {
		ctx  *model.Context
		page types.Dict
	}
	built := map[int]fixture{}
	scaling.GrowsLinearly(t, "joining a page's streams", 250, 1000, 8, func(n int) time.Duration {
		f, ok := built[n]
		if !ok {
			f.ctx, f.page = dividedPage(t, n, chunk)
			built[n] = f
		}
		var out []byte
		var err error
		took := scaling.TimeOnce(func() { out, err = pdfread.PageContent(f.ctx, f.page, 1) })
		if err != nil {
			t.Fatal(err)
		}
		// The stimulus: every chunk holds a `%` and ends in a space, so every join reaches the tokenize — and none
		// is due a separator, which the length confirms.
		if want := n * len(chunk); len(out) != want {
			t.Fatalf("the join is %d bytes, want %d — no separator was due, so the fixture is not what it claims", len(out), want)
		}
		return took
	})
}

// TestAnArrayElementThatIsNotAStreamIsRefused — the door's errors are pdfcpu's: an element pdfcpu cannot read as a
// stream fails the read rather than being skipped, and a nested array — which pdfcpu's own per-element call would
// accept — is refused as pdfcpu's array walk refuses it.
func TestAnArrayElementThatIsNotAStreamIsRefused(t *testing.T) {
	for name, elem := range map[string]types.Object{
		"a dictionary":   types.Dict{"Type": types.Name("NotAStream")},
		"a nested array": types.Array{},
	} {
		ctx, page := dividedPage(t, 1, []byte("q Q"))
		ref, err := ctx.IndRefForNewObject(elem)
		if err != nil {
			t.Fatal(err)
		}
		page["Contents"] = append(page["Contents"].(types.Array), *ref)
		if _, rerr := ctx.PageContent(page, 1); rerr == nil {
			t.Fatalf("%s: pdfcpu's own join accepts it, so the fixture shows nothing", name)
		}
		if _, err := pdfread.PageContent(ctx, page, 1); err == nil {
			t.Errorf("%s in a /Contents array was skipped; pdfcpu refuses the page and so must the door", name)
		}
	}
}

// TestEmptyElementsAreSkippedAndAnEmptyPageIsNoContent — an empty stream or a null in a /Contents array contributes
// nothing and fails nothing, as in pdfcpu's own walk; a page whose every element is empty is `model.ErrNoContent`, the
// answer every caller keys on for "draws nothing".
func TestEmptyElementsAreSkippedAndAnEmptyPageIsNoContent(t *testing.T) {
	ctx, page := dividedPage(t, 1, []byte("q Q"))
	empty, err := ctx.NewStreamDictForBuf(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := empty.Encode(); err != nil {
		t.Fatal(err)
	}
	ref, err := ctx.IndRefForNewObject(*empty)
	if err != nil {
		t.Fatal(err)
	}
	content := page["Contents"].(types.Array)[0]
	page["Contents"] = types.Array{*ref, nil, content}
	if b, err := pdfread.PageContent(ctx, page, 1); err != nil || string(b) != "q Q" {
		t.Errorf("an empty element and a null beside `q Q` read as %q, %v — want the content and no error", b, err)
	}
	page["Contents"] = types.Array{*ref, nil, *ref}
	if _, err := pdfread.PageContent(ctx, page, 1); err != model.ErrNoContent {
		t.Errorf("a page whose every element is empty read as %v, want model.ErrNoContent", err)
	}
}
