package pdfread_test

import (
	"bytes"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
	"nib/internal/scaling"
	"nib/internal/testpdf"
)

// `/pending 748`: nothing bounded a page's content as a whole. pdfcpu caps one stream's decode at 512 MiB, and a
// `/Contents` array may name one stream any number of times — six bytes a naming — so the page decoded to that
// stream's size times the count: 1.2 GiB from a 199 KB file at six namings of a 200 MiB stream, and pdfcpu's
// optimize pass, the FIRST decode on every optimized read, inflated it once per naming (36.7 s, 9.8 GiB peak heap
// at forty namings of 100 MiB).

// namedRepeatedly is a one-page context whose `/Contents` names ONE flate stream of `size` spaces `n` times.
func namedRepeatedly(t *testing.T, size, n int) (*model.Context, types.Dict) {
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
	sd, err := ctx.NewStreamDictForBuf(bytes.Repeat([]byte{' '}, size))
	if err != nil {
		t.Fatal(err)
	}
	if err := sd.Encode(); err != nil {
		t.Fatal(err)
	}
	sd.Content = nil // what a read of the file holds: the encoded bytes only
	ref, err := ctx.IndRefForNewObject(*sd)
	if err != nil {
		t.Fatal(err)
	}
	var arr types.Array
	for range n {
		arr = append(arr, *ref)
	}
	page["Contents"] = arr
	return ctx, page
}

// allocatedBy is the heap f allocates, in bytes.
func allocatedBy(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// TestAPageNamingOneStreamRepeatedlyIsRefusedAsAWhole is the item's own case at both page-content doors: one
// 100 MiB stream named six times is a 600 MiB page, past `MaxPageContentBytes`, and both joins refuse it rather
// than return it. On the old code both returned all 600 MiB.
//
// The refusal's cost is held against reading the same stream named ONCE, in the same rounds (/pending 841; it was
// "under 20 s", which measures the machine): the stream is decoded once however often it is named, so refusing six
// namings is one decode and five appends — ×3.6 to ×4.8 the one naming, measured — where a decode at every naming measured
// ×8.0; the bound is between them. (The 20 s could not see that regression at all: it took 2.95 s.)
func TestAPageNamingOneStreamRepeatedlyIsRefusedAsAWhole(t *testing.T) {
	ctx, page := namedRepeatedly(t, 100<<20, 6)
	once, opage := namedRepeatedly(t, 100<<20, 1)
	for name, read := range map[string]func(*model.Context, types.Dict, int) ([]byte, error){
		"PageContent": pdfread.PageContent, "PageContentAsPdfcpu": pdfread.PageContentAsPdfcpu,
	} {
		scaling.WithinFactor(t, name+": refusing one 100 MiB stream named six times, against reading it named once", 5.5,
			func() time.Duration {
				return scaling.TimeOnce(func() {
					if b, err := read(once, opage, 1); err != nil || len(b) != 100<<20 {
						t.Fatalf("%s: the base, one 100 MiB stream named once, read as %d bytes (%v)", name, len(b), err)
					}
				})
			},
			func() time.Duration {
				return scaling.TimeOnce(func() {
					if b, err := read(ctx, page, 1); !errors.Is(err, pdfread.ErrDecodeLimit) {
						t.Fatalf("%s: one 100 MiB stream named six times read as %d MiB (err %v), want ErrDecodeLimit", name, len(b)>>20, err)
					}
				})
			})
	}
	// The control: under the bound, the page is read whole, and the two joins agree on these bytes.
	small, spage := namedRepeatedly(t, 1<<20, 6)
	b, err := pdfread.PageContent(small, spage, 1)
	if err != nil || len(b) != 6<<20 {
		t.Fatalf("control: one 1 MiB stream named six times read as %d bytes (%v), want %d", len(b), err, 6<<20)
	}
}

// TestTheOptimizePassIsSkippedWhenItsPageContentDecodesPastTheBudget: the same page is unaffordable to pdfcpu's
// optimize pass, which decodes every naming, and the estimate refuses it having decoded the stream ONCE. On the
// old code `Unaffordable` answered "" and the pass ran.
func TestTheOptimizePassIsSkippedWhenItsPageContentDecodesPastTheBudget(t *testing.T) {
	ctx, _ := namedRepeatedly(t, 100<<20, 6)
	var why string
	alloc := allocatedBy(func() { why = pdfread.Unaffordable(ctx) })
	if !strings.Contains(why, "page content decodes past") {
		t.Fatalf("a page naming one 100 MiB stream six times is affordable to the optimize pass (%q), want refused", why)
	}
	// One decode of 100 MiB grows its buffer to at most twice that; a decode per naming would be six.
	if alloc > 400<<20 {
		t.Errorf("the estimate allocated %d MiB, want one decode's worth (under 400 MiB)", alloc>>20)
	}
	if err := pdfread.Optimize(ctx); err != nil {
		t.Errorf("Optimize over it answered %v, want the pass skipped", err)
	}
	small, _ := namedRepeatedly(t, 1<<20, 6)
	if why := pdfread.Unaffordable(small); why != "" {
		t.Fatalf("control: six namings of a 1 MiB stream are unaffordable (%s), want affordable", why)
	}
}

// TestDecodeWithinStopsTheDecodeAtTheLimit: the capped decode is refused DURING the decode — a 256 MiB stream
// under a 1 MiB limit allocates about the limit, not the stream — and a stream decoded earlier, or unfiltered, is
// held to the same limit by its length.
func TestDecodeWithinStopsTheDecodeAtTheLimit(t *testing.T) {
	ctx, _ := namedRepeatedly(t, 1, 1)
	sd, err := ctx.NewStreamDictForBuf(bytes.Repeat([]byte{' '}, 256<<20))
	if err != nil {
		t.Fatal(err)
	}
	if err := sd.Encode(); err != nil {
		t.Fatal(err)
	}
	sd.Content = nil
	var derr error
	alloc := allocatedBy(func() { derr = pdfread.DecodeWithin(sd, 1<<20) })
	if !errors.Is(derr, pdfread.ErrDecodeLimit) {
		t.Fatalf("a 256 MiB stream under a 1 MiB limit decoded (%v), want ErrDecodeLimit", derr)
	}
	if alloc > 32<<20 {
		t.Errorf("refusing a 256 MiB stream at 1 MiB allocated %d MiB, want about the limit", alloc>>20)
	}
	if err := pdfread.DecodeWithin(sd, 256<<20); err != nil || len(sd.Content) != 256<<20 {
		t.Fatalf("control: at its own size it decoded %d bytes (%v)", len(sd.Content), err)
	}
	if err := pdfread.DecodeWithin(sd, 1<<20); !errors.Is(err, pdfread.ErrDecodeLimit) {
		t.Errorf("an already-decoded 256 MiB stream passed a 1 MiB limit (%v)", err)
	}
	raw := types.NewStreamDict(types.Dict{}, 0, nil, nil, nil)
	raw.Raw = make([]byte, 2<<20)
	if err := pdfread.DecodeWithin(&raw, 1<<20); !errors.Is(err, pdfread.ErrDecodeLimit) {
		t.Errorf("an unfiltered 2 MiB stream passed a 1 MiB limit (%v)", err)
	}
	if err := pdfread.DecodeWithin(&raw, 0); !errors.Is(err, pdfread.ErrDecodeLimit) {
		t.Errorf("a zero limit admitted a 2 MiB stream (%v) — pdfcpu reads 0 as its default", err)
	}
}
