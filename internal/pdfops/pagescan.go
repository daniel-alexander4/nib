package pdfops

import (
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
// than deciding a new one. `ref` may be nil where only `checkStructConsistency` wanted it and the
// lookup failed; that caller already had a `continue` for exactly that.
func scanPages(ctx *model.Context) []pageRecord {
	out := make([]pageRecord, 0, ctx.PageCount)
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, err := ctx.PageDict(p, false)
		if err != nil || d == nil {
			continue
		}
		rec := pageRecord{nr: p, dict: d}
		if ir, e := ctx.PageDictIndRef(p); e == nil && ir != nil {
			rec.ref = ir
		}
		if res, e := ctx.DereferenceDict(d["Resources"]); e == nil && res != nil {
			rec.res = res
		}
		out = append(out, rec)
	}
	return out
}
