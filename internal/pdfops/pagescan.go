package pdfops

import (
	"nib/internal/pdfread"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// pageRecord is one page, resolved once, for the sweeps that all want the same three things
// (/pending 530).
//
// # Why this exists: the gate walked the page tree four times per page
//
// `structureCarriedCompletely` calls `checkStructConsistency`, `parentTreeOwners` and
// `formDrawCounts`, and each began with its own `for p := 1; p <= ctx.PageCount; p++` over
// `ctx.PageDict(p, false)` — plus a `ctx.PageDictIndRef(p)` inside the first. **`PageDict` walks the
// page tree from the root on every call and caches nothing** (`model/xreftable.go:2141-2169`), so a
// p-page document paid 4p walks of a tree whose own walk is O(p). That is the O(pages²) shape
// `pageselect.go:79-81` already records as a measured 6 s of a 27 s digest, and it is the cost of
// every tagged subset and every tagged n-up, because the gate is shared through `completeOrHonest`
// (ADR-009).
//
// # What it deliberately does NOT change
//
// **`ctx.PageDict(p, false)` semantics, exactly.** The obvious fold is `collectLeaves`, which this
// package already has and which walks the tree once — and it RESOLVES the inheritable attributes,
// `/Resources` among them. `PageDict` with `consolidateRes=false` does not. The three sweeps read
// `d["Resources"]` directly, so switching would start showing them an inherited resource dictionary
// they have never seen, on a page that inherits one — a change to what the GATE sees, and the gate
// decides carried-versus-dropped on every tagged document. A cost fold may not move a correctness
// boundary; the dict here is the same dict those loops were reading.
type pageRecord struct {
	nr   int        // 1-based page number
	dict types.Dict // exactly what ctx.PageDict(p, false) returned
	ref  *types.IndirectRef
	res  types.Dict // the dereferenced /Resources, nil when absent — shared by two of the three sweeps
}

// scanPages resolves every page once.
//
// A page whose dictionary or reference cannot be read is SKIPPED, which is what all three sweeps did
// individually (`if err != nil || d == nil { continue }`) — the fold inherits the behaviour rather
// than deciding a new one. `ref` is what `ctx.PageDictIndRef(p)` answers, which is the same walk as
// `PageDict`'s and so the same reference.
//
// Since /pending 753 the pages come from `pdfread.Pages`, which answers `PageDict(p, false)` for every
// page from ONE walk of the tree: this function's own fold was 4p walks down to p, and p walks of a flat
// tree is still quadratic — 65 s of a profiled ceremony preparation on a 7,059-page document.
func scanPages(ctx *model.Context) []pageRecord {
	out := make([]pageRecord, 0, ctx.PageCount)
	for _, pg := range pdfread.Pages(ctx) {
		if pg.Err != nil || pg.Dict == nil {
			continue
		}
		rec := pageRecord{nr: pg.Nr, dict: pg.Dict, ref: pg.Ref}
		if res, e := ctx.DereferenceDict(pg.Dict["Resources"]); e == nil && res != nil {
			rec.res = res
		}
		out = append(out, rec)
	}
	return out
}

// livePageObjects is the object number of every page in the page tree — the set the structure-tree readers
// take as `live`, and which seven doors each built with a `PageDictIndRef` loop (/pending 753).
func livePageObjects(ctx *model.Context) map[int]bool {
	live := map[int]bool{}
	for _, pg := range pdfread.Pages(ctx) {
		if pg.Err == nil && pg.Ref != nil {
			live[pg.Ref.ObjectNumber.Value()] = true
		}
	}
	return live
}

// pageAt is page n as `ctx.PageDict(n, false)` answers it, taken from pages — a `pdfread.Pages(ctx)` walk the
// caller made once for its loop — and asked of pdfcpu only when n lies outside it (/pending 756). It is the one
// door every per-page helper resolves its page through, so a loop over the pages walks the tree once instead of
// once per page. A single-page operation passes nil pages: one `PageDict` call reaches a page no slower than the
// walk that would precede it, and a walk of every page to answer for one is the slower of the two.
//
// Out of range (a `/Pg` that names no page, page 0) is pdfcpu's own refusal, exactly as before.
func pageAt(ctx *model.Context, pages []pdfread.Page, n int) pdfread.Page {
	if n >= 1 && n <= len(pages) {
		return pages[n-1]
	}
	d, ref, attrs, err := ctx.PageDict(n, false)
	return pdfread.Page{Nr: n, Dict: d, Ref: ref, Attrs: attrs, Err: err}
}
