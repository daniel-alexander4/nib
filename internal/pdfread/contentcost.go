package pdfread

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The optimize pass's third estimate — `/pending 748`: the page content the pass DECODES.
//
// **The pass is the first decode of a page's content, before any reader of nib's runs**, on the reads that ask it
// to: `optimizeResourceDicts` (pdfcpu optimize.go:1592, v0.13.0) calls `PageDict(i, true)` for every page, which
// reaches `consolidateResourcesWithContent` (model/xreftable.go:1981) and so `XRefTable.PageContent`
// (xreftable.go:1986), decoding every stream of every page's `/Contents` at every naming. Each decode is capped at
// pdfcpu's 512 MiB and nothing caps the sum, so one stream named many times — by a `/Contents` array, or by many
// pages — is inflated at each naming. Measured through `ReadOptimizedOrRefuse` on a ~100 KB file naming one
// 100 MiB flate stream N times: N=1 0.8 s, N=10 8.3 s and 2.6 GiB peak heap, N=40 36.7 s and 9.8 GiB.
//
// **It costs nothing where it cannot matter**, in two steps, because every optimized read — the Open included —
// asks it (measured on the 36-file producer corpus with a decode on every read: +19.5 % median on the Open's
// read, +60 % on a 200-page document of 50 MiB of content):
//
//   - `passDecodesPageContent`: the Open's read (`FlagsJSON`, `LISTPROPERTIES`) does not decode page content at
//     all, so nothing is estimated for it.
//   - `contentMayExceed`: a document whose streams are each named once decodes, in the pass, no more than each
//     stream's own pdfcpu-capped inflation — work proportional to the file, not amplified past it — so it is not
//     estimated. No producer file measured names a stream twice.
//
// **Declared gap — Dan's decision, 2026-09-29.** Distinct streams that each inflate towards pdfcpu's per-decode
// cap are NOT estimated, even when together they decode past the budget: six different 100 MiB streams (~600 KB
// of file) pass. The cost is bounded by pdfcpu's per-decode cap (512 MiB) times the number of streams the file
// can carry, i.e. by the file's size times flate's ceiling (~1032:1); a raw-length × worst-expansion trigger that
// closed it would open a decode on every document with more than ~508 KiB of raw page content, and was measured
// at +21-62 % on a 200-page, 50 MiB-content read. `TestDistinctStreamsAreADeclaredGap` holds the gap open, so
// closing it later is a decision someone makes, not a drift.
//
// Only then is each stream decoded — ONCE, through `DecodeWithin`, limited to what is left of
// `MaxPageContentBytes` — and charged at every naming. And the decode is not repeated: an affordable estimate
// leaves each decoded stream's bytes on its table entry (`sd.Content`), which pdfcpu's pass then reads instead of
// decoding again (`DecodeLengthWithLimit` returns a non-nil Content, streamdict.go:436-445). The writer writes
// `sd.Raw` and never `sd.Content` (writeObjects.go:465), so what is written is unchanged; the price is the
// document's decoded page content held for the context's life, which the budget bounds. An unaffordable estimate
// takes the bytes back off, since no pass will read them.

// passDecodesPageContent is pdfcpu v0.13.0's own condition for decoding page content during its optimize pass,
// restated in ONE place so an upgrade that changes it is a diff here, not a silent hole:
//
//   - optimize.go:1647-1653 — `optimizeResourceDicts` runs when `ctx.Cmd` is VALIDATE, OPTIMIZE, LISTIMAGES,
//     EXTRACTIMAGES or UPDATEIMAGES and `ctx.Conf.OptimizeResourceDicts` is set; it consolidates every page's
//     resources against its content, decoding it.
//   - optimize.go:997 — `ctx.OptimizeDuplicateContentStreams` runs `optimizePageContent`, whose
//     `removeEmptyContentStreams` (optimize.go:69) decodes each content stream to see whether it is empty.
func passDecodesPageContent(ctx *model.Context) bool {
	if ctx.Configuration == nil || ctx.Conf == nil {
		return true // no configuration to read: assume the pass decodes
	}
	switch ctx.Cmd {
	case model.VALIDATE, model.OPTIMIZE, model.LISTIMAGES, model.EXTRACTIMAGES, model.UPDATEIMAGES:
		if ctx.Conf.OptimizeResourceDicts { // pdfcpu reads the XRefTable's Conf here, not the Context's
			return true
		}
	}
	return ctx.OptimizeDuplicateContentStreams
}

// contentNaming is one page-content stream and how many times the page tree names it.
type contentNaming struct {
	ref   types.IndirectRef
	count int
}

// contentNamings is every indirect stream the pages' `/Contents` name (pages from `pageDicts`), in first-naming order, with its count.
func contentNamings(ctx *model.Context, pages []types.Dict) []*contentNaming {
	var out []*contentNaming
	byNr := map[int]*contentNaming{}
	for _, d := range pages {
		for _, ir := range contentRefs(ctx, d["Contents"]) {
			nr := ir.ObjectNumber.Value()
			if n := byNr[nr]; n != nil {
				n.count++
				continue
			}
			n := &contentNaming{ref: ir, count: 1}
			byNr[nr] = n
			out = append(out, n)
		}
	}
	return out
}

// contentMayExceed reports whether the pass's page-content decode may be amplified past what the file itself
// carries: some stream is named more than once (see the file comment for the declared gap).
func contentMayExceed(namings []*contentNaming) bool {
	for _, n := range namings {
		if n.count > 1 {
			return true
		}
	}
	return false
}

// contentUnaffordable is the estimate (see the file comment): "" when the pass's page-content decode stays under
// MaxPageContentBytes, having left the decoded bytes on the table for the pass.
func contentUnaffordable(ctx *model.Context, pages []types.Dict) string {
	if !passDecodesPageContent(ctx) {
		return ""
	}
	namings := contentNamings(ctx, pages)
	if !contentMayExceed(namings) {
		return ""
	}
	var cached []*model.XRefTableEntry
	uncache := func() {
		for _, e := range cached {
			if sd, ok := e.Object.(types.StreamDict); ok {
				sd.Content = nil
				e.Object = sd
			}
		}
	}
	total := 0
	for _, n := range namings {
		entry, found := ctx.FindTableEntry(n.ref.ObjectNumber.Value(), n.ref.GenerationNumber.Value())
		if !found || entry.Free || entry.Object == nil {
			continue
		}
		sd, isStream := entry.Object.(types.StreamDict)
		if !isStream {
			continue
		}
		hadContent := sd.Content != nil
		switch err := DecodeWithin(&sd, int64(MaxPageContentBytes-total)); {
		case errors.Is(err, ErrDecodeLimit):
			uncache()
			return contentTooLarge()
		case err != nil:
			continue // a stream that will not decode is charged nothing: the pass reports it itself
		}
		if !hadContent && decodeOnce {
			// Kept at its length: the decode's buffer grows by doubling, and holding it whole kept twice the
			// content alive (measured on 50 MiB of page content: +102 MiB retained over the read without it, +53 MiB
			// once copied to size).
			if cap(sd.Content) > len(sd.Content) {
				sd.Content = bytes.Clone(sd.Content)
			}
			entry.Object = sd
			cached = append(cached, entry)
		}
		if total += len(sd.Content) * n.count; total > MaxPageContentBytes {
			uncache()
			return contentTooLarge()
		}
	}
	return ""
}

// decodeOnce keeps the estimate's decoded bytes for the pass (the file comment's last paragraph). A variable only
// so the test that proves the written bytes unchanged can compare against the read without it.
var decodeOnce = true

func contentTooLarge() string {
	return fmt.Sprintf("its page content decodes past %d bytes, counting a stream again at every page or "+
		"/Contents entry that names it, and the optimizer decodes every one", MaxPageContentBytes)
}

// contentRefs is the indirect streams a page's `/Contents` names, in order and with repeats: the reference
// itself, or the indirect elements of the array it is (directly or through a reference). A stream is always an
// indirect object (ISO 32000-1 §7.3.8), so nothing is missed.
func contentRefs(ctx *model.Context, o types.Object) []types.IndirectRef {
	if ir, ok := o.(types.IndirectRef); ok {
		r, err := ctx.Dereference(ir)
		if err != nil {
			return nil
		}
		if _, isStream := r.(types.StreamDict); isStream {
			return []types.IndirectRef{ir}
		}
		o = r
	}
	arr, _ := o.(types.Array)
	var out []types.IndirectRef
	for _, e := range arr {
		if ir, ok := e.(types.IndirectRef); ok {
			out = append(out, ir)
		}
	}
	return out
}
