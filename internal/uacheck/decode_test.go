package uacheck

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"strings"
	"testing"
	"time"
)

// /pending 721: a form XObject was decoded afresh at every `Do` — pdfcpu's `DereferenceStreamDict` hands back a
// copy, so nothing cached the decode — and `contentOps` counts operators, so a stream that is nothing but
// whitespace spent no budget at all. A 66 KB file drawing one 64 MiB flate-compressed form of spaces N times ran
// 7.95 s at N=20 and 25.1 s at N=80 (the P05 phase-close review, measured), reached from `handleUACheck` with no
// deadline. The decode now happens once per form per check, and every walk charges the bytes it walks.

// whitespaceFormDrawn is one page drawing a flate-compressed form of `size` spaces `draws` times.
func whitespaceFormDrawn(size, draws int) []byte {
	var z bytes.Buffer
	zw, _ := zlib.NewWriterLevel(&z, zlib.BestCompression)
	chunk := bytes.Repeat([]byte{' '}, 1<<20)
	for left := size; left > 0; left -= len(chunk) {
		if left < len(chunk) {
			chunk = chunk[:left]
		}
		zw.Write(chunk)
	}
	zw.Close()
	page := strings.Repeat("/X0 Do ", draws)
	return buildPDF(map[int]string{
		1:   "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2:   "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:   "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /X0 100 0 R >> >> /Contents 4 0 R >>",
		4:   fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(page), page),
		7:   "<< /Type /StructTreeRoot >>",
		100: fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Filter /FlateDecode /Length %d >>\nstream\n%s\nendstream", z.Len(), z.String()),
	})
}

// TestAFormDrawnRepeatedlyIsDecodedOnce is the item's own case with the time taken out: N draws of one form cost
// ONE decode. Red on the old code, which decoded it at every `Do`.
func TestAFormDrawnRepeatedlyIsDecodedOnce(t *testing.T) {
	pdf := whitespaceFormDrawn(1<<10, 50)
	d, err := open(pdf)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, why := d.contentEvents(); why != "" {
		t.Fatalf("control: 50 draws of a 1 KiB form of spaces refused (%s), want it read in full", why)
	}
	if d.streamDecodes != 1 {
		t.Fatalf("50 draws of one form XObject decoded it %d times, want once — the decode is cached per object", d.streamDecodes)
	}
}

// TestAHugeFormDrawnRepeatedlyIsStoppedPromptly is the measured case: a 64 MiB form of spaces drawn 80 times,
// 25.1 s before. Every walk charges the bytes it walks, so whitespace — which spends no operator — cannot fill
// the walk for free; the check answers CannotCheck naming the byte budget, promptly.
func TestAHugeFormDrawnRepeatedlyIsStoppedPromptly(t *testing.T) {
	pdf := whitespaceFormDrawn(64<<20, 80)
	var got Result
	start := time.Now()
	withinSeconds(t, 30, "80 draws of a 64 MiB form of spaces", func() { got = verdictOf(t, pdf, "7.1 t3") })
	if el := time.Since(start); el > 10*time.Second {
		t.Fatalf("80 draws of a 64 MiB form of spaces took %v, want under 10 s (it was 25.1 s; ~2.3 s now, the walk of 512 MiB)", el)
	}
	if got.Verdict != CannotCheck || !strings.Contains(got.Why, "bytes of content") {
		t.Fatalf("80 draws of a 64 MiB form of spaces report %v (%s), want CannotCheck naming the content byte budget", got.Verdict, got.Why)
	}
}
