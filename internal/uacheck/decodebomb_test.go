package uacheck

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"nib/internal/pdfread"

	"github.com/pdfcpu/pdfcpu/pkg/filter"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// `/pending 748`: the content budget was charged with a stream's DECODED size after `sd.Decode()` returned, so a
// stream was inflated whole — up to pdfcpu's 512 MiB — before anything could refuse it; and nothing bounded a
// page's `/Contents` as a whole, so a ~100 KB file naming one 100 MiB stream six times ran the check through
// 600 MiB of content, and pdfcpu's optimize pass decoded it once per naming (13.7 s and 3.2 GiB at six namings of
// 200 MiB).

// flatedSpacesUA is `size` spaces flate-compressed, a MiB at a time.
func flatedSpacesUA(size int) []byte {
	var z bytes.Buffer
	zw, _ := zlib.NewWriterLevel(&z, zlib.BestCompression)
	chunk := bytes.Repeat([]byte{' '}, 1<<20)
	for left := size; left > 0; left -= len(chunk) {
		zw.Write(chunk[:min(left, len(chunk))])
	}
	zw.Close()
	return z.Bytes()
}

// TestANestedStreamIsDecodedOnlyAsFarAsTheBudgetHasLeft: with 1 MiB of the content budget left, a 100 MiB form
// is refused during its decode — allocating about what was left — rather than inflated and then refused.
func TestANestedStreamIsDecodedOnlyAsFarAsTheBudgetHasLeft(t *testing.T) {
	sd := types.NewStreamDict(types.Dict{}, 0, nil, nil, []types.PDFFilter{{Name: filter.Flate}})
	sd.Raw = flatedSpacesUA(100 << 20)
	d := &Document{contentBytes: maxContentBytes - 1<<20}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	src, err := d.decodedContent(&sd, 9)
	runtime.ReadMemStats(&after)
	if !errors.Is(err, pdfread.ErrDecodeLimit) {
		t.Fatalf("a 100 MiB form with 1 MiB of the budget left decoded to %d bytes (%v), want ErrDecodeLimit", len(src), err)
	}
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 32<<20 {
		t.Errorf("refusing it allocated %d MiB, want about the 1 MiB left", alloc>>20)
	}
	// The control: with the budget whole, the same form decodes.
	sd.Content = nil
	if src, err := (&Document{}).decodedContent(&sd, 9); err != nil || len(src) != 100<<20 {
		t.Fatalf("control: with the budget whole it decoded %d bytes (%v), want %d", len(src), err, 100<<20)
	}
}

// TestAPageNamingOneStreamRepeatedlyIsRefusedPromptly is the item's case through the checker's own entry: one
// 100 MiB stream named six times. The checker reads it once, through the page-content door bounded at
// `maxContentBytes`, and the content rules say so rather than reading 600 MiB of content.
//
// **It was the optimize pass's refusal (`ErrUnaffordable`) until `/pending 782`**: the pass decoded page content
// only for its per-page resource step, which the checker no longer runs (`checkerConfig`), so nothing in the read
// decodes the stream and the refusal moved to where the decode is.
func TestAPageNamingOneStreamRepeatedlyIsRefusedPromptly(t *testing.T) {
	z := flatedSpacesUA(100 << 20)
	pdf := buildPDF(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents [" +
			strings.TrimSpace(strings.Repeat("4 0 R ", 6)) + "] >>",
		4: fmt.Sprintf("<< /Filter /FlateDecode /Length %d >>\nstream\n%s\nendstream", len(z), z),
		7: "<< /Type /StructTreeRoot >>",
	})
	start := time.Now()
	rep, err := Check(pdf)
	if err != nil {
		t.Fatalf("one 100 MiB stream named six times: Check: %v", err)
	}
	var got Result
	for _, r := range rep.Results {
		if r.Clause == "7.1 t3" {
			got = r
		}
	}
	if got.Verdict != CannotCheck || got.Why == "" {
		t.Errorf("one 100 MiB stream named six times reports 7.1 t3 %v (%s), want CannotCheck saying the content was "+
			"not read in full", got.Verdict, got.Why)
	}
	if el := time.Since(start); el > 15*time.Second {
		t.Errorf("the check took %v to refuse it, want well under 15 s (one decode, not six)", el)
	}
}
