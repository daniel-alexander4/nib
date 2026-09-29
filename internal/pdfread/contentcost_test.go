package pdfread_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// distinctStreams is a one-page context whose `/Contents` names n DIFFERENT flate streams of `size` spaces each —
// the estimate's second trigger: no stream is named twice, and only the raw length times Flate's ceiling says
// the page may decode past the budget.
func distinctStreams(t *testing.T, size, n int) *model.Context {
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
	for i := range n {
		sd, err := ctx.NewStreamDictForBuf(append(bytes.Repeat([]byte{' '}, size), fmt.Sprintf("%% %d", i)...))
		if err != nil {
			t.Fatal(err)
		}
		if err := sd.Encode(); err != nil {
			t.Fatal(err)
		}
		sd.Content = nil
		ref, err := ctx.IndRefForNewObject(*sd)
		if err != nil {
			t.Fatal(err)
		}
		arr = append(arr, *ref)
	}
	page["Contents"] = arr
	return ctx
}

// TestTheEstimateRunsOnlyWherePdfcpusPassDecodesPageContent is the gate: the same six-namings file is refused
// under a configuration whose pass decodes page content (the default VALIDATE command, or
// OptimizeDuplicateContentStreams), and under the Open's LISTPROPERTIES — whose pass decodes none — it is
// neither refused nor decoded, by the estimate or by the pass after it.
func TestTheEstimateRunsOnlyWherePdfcpusPassDecodesPageContent(t *testing.T) {
	ctx, _ := namedRepeatedly(t, 100<<20, 6)
	if !pdfread.PassDecodesPageContent(ctx) || pdfread.Unaffordable(ctx) == "" {
		t.Fatal("control: under the default VALIDATE command the six-namings page is affordable, want refused")
	}

	ctx.Cmd = model.LISTPROPERTIES
	if pdfread.PassDecodesPageContent(ctx) {
		t.Fatal("the gate says LISTPROPERTIES decodes page content; pdfcpu's pass does not (optimize.go:1647)")
	}
	var why string
	var err error
	alloc := allocatedBy(func() {
		if why = pdfread.Unaffordable(ctx); why == "" {
			err = api.OptimizeContext(ctx)
		}
	})
	if why != "" || err != nil {
		t.Fatalf("under LISTPROPERTIES the page is refused (%q, %v), want the pass to run: it decodes no content", why, err)
	}
	if alloc > 32<<20 {
		t.Errorf("under LISTPROPERTIES the estimate and pass allocated %d MiB, want nothing decoded", alloc>>20)
	}

	dup, _ := namedRepeatedly(t, 100<<20, 6)
	dup.Cmd, dup.OptimizeDuplicateContentStreams = model.LISTPROPERTIES, true
	if !pdfread.PassDecodesPageContent(dup) || !strings.Contains(pdfread.Unaffordable(dup), "page content decodes past") {
		t.Error("with OptimizeDuplicateContentStreams the pass decodes every content stream (optimize.go:997), " +
			"and the page was not refused")
	}
}

// TestARepeatedNamingOpensTheDecode: a stream named twice is decoded by the estimate (the six-namings refusal is
// the estimate's own test), and a page whose streams are each named once is not decoded at all.
func TestARepeatedNamingOpensTheDecode(t *testing.T) {
	repeated, _ := namedRepeatedly(t, 1<<20, 2)
	if alloc := allocatedBy(func() { pdfread.Unaffordable(repeated) }); alloc < 1<<20 {
		t.Errorf("a stream named twice was not decoded (%d bytes allocated), want the repeat to open the decode", alloc)
	}
	quiet := distinctStreams(t, 1<<20, 6)
	if alloc := allocatedBy(func() {
		if why := pdfread.Unaffordable(quiet); why != "" {
			t.Errorf("control: six 1 MiB streams refused (%s)", why)
		}
	}); alloc > 1<<20 {
		t.Errorf("a page naming each stream once allocated %d KiB in the estimate, want no decode", alloc>>10)
	}
}

// TestDistinctStreamsAreADeclaredGap — Dan's decision, 2026-09-29 (/pending 748, option A): six DIFFERENT 100 MiB
// streams on one page, 600 MiB of content from ~600 KB of file, are NOT estimated, and the pass is left to decode
// them. That cost is bounded by pdfcpu's per-decode cap (512 MiB) times the streams a file can carry — the file's
// size times flate's ~1032:1 — where a length × worst-expansion trigger would have opened a decode on every
// document past ~508 KiB of raw page content (+21-62 % measured on a 200-page, 50 MiB-content read). This test
// fails when the gap closes, so closing it is a decision, not a side effect: change the test with the decision.
func TestDistinctStreamsAreADeclaredGap(t *testing.T) {
	distinct := distinctStreams(t, 100<<20, 6)
	var why string
	alloc := allocatedBy(func() { why = pdfread.Unaffordable(distinct) })
	if why != "" {
		t.Fatalf("six distinct 100 MiB streams were refused (%q): the declared gap has closed — if that is "+
			"intended, it is a new decision; update contentcost.go's file comment, ADR-064 and this test with it", why)
	}
	if alloc > 1<<20 {
		t.Errorf("the estimate decoded %d MiB of distinct streams, want none — the gap is that they are not estimated", alloc>>20)
	}
}

// TestDecodingOnceChangesNothingWritten — decode-once leaves the estimate's bytes on the table for the pass. The
// writer writes `sd.Raw` (writeObjects.go:465), so every stream written after the pass has the bytes it has
// without the cache (checked the same way over the 36-file producer corpus with every content stream cached: 36
// of 36 identical), and the pass does not decode what the estimate already decoded.
func TestDecodingOnceChangesNothingWritten(t *testing.T) {
	written := func(once bool) ([]byte, uint64) {
		*pdfread.DecodeOnce = once
		defer func() { *pdfread.DecodeOnce = true }()
		ctx, _ := namedRepeatedly(t, 8<<20, 3) // a repeat, so the estimate decodes
		var err error
		alloc := allocatedBy(func() { err = pdfread.Optimize(ctx) })
		if err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		if err := api.WriteContext(ctx, &b); err != nil {
			t.Fatal(err)
		}
		return b.Bytes(), alloc
	}
	with, allocWith := written(true)
	without, allocWithout := written(false)
	if a, b := writtenStreams(t, with), writtenStreams(t, without); a != b {
		t.Fatalf("decode-once changed what was written:\n with    %s\n without %s", a, b)
	}
	// Three namings of 8 MiB: without the cache the pass decodes all three again, with it none.
	if allocWith+16<<20 > allocWithout {
		t.Errorf("the optimize pass allocated %d MiB with decode-once and %d MiB without, want at least 16 MiB saved",
			allocWith>>20, allocWithout>>20)
	}
}

// writtenStreams is a written file's streams as a sorted multiset of their raw bytes' hashes, with its object
// count — what the writer put down for each stream. pdfcpu's pass and writer number objects and order
// dictionaries run to run (two writes of one context without the cache differed in 35 of 36 producer files), so
// the files themselves are not compared; every stream's bytes are, and they are what a re-encode would change.
func writtenStreams(t *testing.T, pdf []byte) string {
	t.Helper()
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	var hs []string
	objs := 0
	for _, e := range ctx.Table {
		if e == nil || e.Free || e.Object == nil {
			continue
		}
		objs++
		if sd, ok := e.Object.(types.StreamDict); ok {
			h := sha256.Sum256(sd.Raw)
			hs = append(hs, hex.EncodeToString(h[:]))
		}
	}
	sort.Strings(hs)
	return fmt.Sprintf("%d objects, streams %v", objs, hs)
}
