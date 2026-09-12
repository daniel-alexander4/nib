package pdfops

import (
	"bytes"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Tag fate — `PLAN-accessibility.md` P01.S01, and `/pending 29`'s floor.
//
// # The law this file exists to keep
//
// **Nothing claims tagging it does not have.** No output may carry `/MarkInfo /Marked true`, a
// `/StructTreeRoot`, or a conformance assertion over content that is neither tagged nor marked as
// an artifact. *A visible loss is honest; a false claim is not, and it is worse than no tagging at
// all because it defeats the reader's own check* — a screen reader that is told a document is
// tagged stops looking for the fallbacks it would otherwise use.
//
// # What was measured, because this was not a hypothesis — and the cause is not what it looks like
//
// A hand-built tagged PDF — `/MarkInfo /Marked true`, a `/StructTreeRoot` with one `/StructElem`,
// one `/P <</MCID 0>> BDC … EMC` run, and `/StructParents 0` on the page — through `NUp(2)` on
// pdfcpu v0.13.0 comes out with:
//
//	/StructTreeRoot  1     (kept)
//	/MarkInfo        1     (kept)
//	/Marked          1     (kept — still true)
//	/StructElem      0     (GONE)
//	/StructParents   0     (GONE)
//
// So the composed document still *says* it is tagged while the tree it points at has no elements
// and no page links back to it. veraPDF confirms it differentially: the input fails ua1 clauses
// 7.1 t8, 7.1 t10 and 7.21.4.1 t1 (fixture limitations), and the n-upped output fails **those plus
// 7.1 t3** — a failure the input did not have.
//
// **But `nup` is not what destroys it.** A NO-OP `writeMutated` — read, validate, optimize, write,
// changing nothing — produces the same loss: `/StructElem 1 → 0`, `/StructParents 1 → 0`, while
// `/StructTreeRoot` and `/MarkInfo` survive. The READ half is fine: after
// `ReadValidateAndOptimize` the catalog still holds a complete `/StructTreeRoot` with
// `/K [8 0 R]` and a `/ParentTree`, and `api.Validate` reports the fixture clean. It is
// `WriteContext` that does not serialise the objects the tree points at, because nothing in its
// traversal reaches them — leaving a `/StructTreeRoot` whose `/K` dangles.
//
// That refutes `/pending 29`'s reason 1 (*"round-trips … intact through
// ReadValidateAndOptimize→WriteContext"*, measured 2026-06-23) and is filed as its own item. It
// also means this door is needed by far more than `nup`: on this evidence EVERY operation routed
// through `writeMutated` voids structure while keeping the claim. This slice fixes the one
// operation its plan scopes; the rest is `PLAN-accessibility.md` P01.S03/S04's, now with a measured
// premise instead of an assumed one.
//
// # Why DROP rather than tag the composed page
//
// Tagging an n-up sheet means authoring structure for a page that did not exist a moment ago, and
// that needs the tag-tree core this plan builds four phases later. D2 is explicit that preservation
// precedes authoring — *"tagging authored on top of a pipeline that eats tagging produces documents
// that are accessible until the user rotates a page"* — so the floor is honesty, not coverage.
//
// # One door
//
// Every operation that voids structure calls this, rather than each deleting three keys correctly.
// That is ADR-009's shape and law 2's: the guard checks the door, not the sites that happen to be
// right today.
//
// **The law itself is ADR-031**, which carries the measurement, the eight operations the guard
// found, and the argument for the check being a post-condition rather than an unconditional strip.

// hasTaggingClaim reports whether a document asserts that it is tagged.
//
// Used to keep `dropTaggingClaim` from rewriting a document that never claimed anything: a rewrite
// is not free — it re-encodes, which moves bytes and invalidates a signature — so an untagged
// document must come through untouched.
func hasTaggingClaim(ctx *model.Context) bool {
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		return false
	}
	if _, ok := root["StructTreeRoot"]; ok {
		return true
	}
	if mi := root["MarkInfo"]; mi != nil {
		if d, derr := ctx.DereferenceDict(mi); derr == nil && d != nil {
			if b := d.BooleanEntry("Marked"); b != nil && *b {
				return true
			}
		}
	}
	return false
}

// structureCount reports whether a document claims tagging and how many struct elements it actually
// carries — **by PARSING, never by counting bytes.**
//
// # The error this replaces, recorded because it cost real data
//
// The first version of this used `bytes.Count(pdf, []byte("/StructElem"))`. pdfcpu writes the
// structure tree into a **compressed object stream**, so that count is **0 for every pdfcpu output**
// — a perfectly tagged document and a stripped one are indistinguishable to it. Everything built on
// that measurement was wrong in the same direction: the predicate reported every tagged document as
// lying, and the "fix" then stripped tag trees that had survived intact.
//
// Measured with the parse, on a LibreOffice document with 14 elements: a no-op write keeps **14**,
// `Rotate` keeps **14**, `Optimize` keeps **14**. `NUp` and `Collect` drop the root and the elements
// together, which is honest. **No operation was lying.**
func structureCount(pdf []byte) (claimed bool, elements int) {
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return false, -1 // unreadable: not a claim we can judge, and -1 says so rather than 0
	}
	cat, cerr := ctx.XRefTable.Catalog()
	if cerr != nil {
		return false, -1
	}
	st, ok := cat["StructTreeRoot"]
	if !ok {
		return false, 0
	}
	d, derr := ctx.DereferenceDict(st)
	if derr != nil || d == nil {
		return true, 0
	}
	seen := map[string]bool{}
	var walk func(o types.Object) int
	walk = func(o types.Object) int {
		n := 0
		if arr, e := ctx.DereferenceArray(o); e == nil && arr != nil {
			for _, x := range arr {
				n += walk(x)
			}
			return n
		}
		dd, e := ctx.DereferenceDict(o)
		if e != nil || dd == nil {
			return 0
		}
		// A visited set, because a tree whose elements point back at their parents is ordinary and
		// `ContentDigest`'s non-termination (`/pending 454`) is this repo's standing lesson about
		// walking a PDF without one.
		key := dd.String()
		if seen[key] {
			return 0
		}
		seen[key] = true
		if t := dd.NameEntry("Type"); t != nil && *t == "StructElem" {
			n++
		}
		if k, ok := dd["K"]; ok {
			n += walk(k)
		}
		return n
	}
	return true, walk(d["K"])
}

// ClaimsTagging reports whether a document asserts that it is tagged.
func ClaimsTagging(pdf []byte) bool {
	claimed, _ := structureCount(pdf)
	return claimed
}

// structureElements reports how many struct elements a document actually carries, or -1 when it
// cannot be read.
//
// **Unexported, because nothing outside this package needs the count.** It was exported for a
// moment on the assumption the server's notice would want it; the notice asks `ClaimsTagging` and
// compares before with after, which is the question it actually has. `zerocaller_test.go` caught the
// export with no caller on the first run after the correction.
func structureElements(pdf []byte) int {
	_, n := structureCount(pdf)
	return n
}

// claimsTaggingItHasNot is law 1's violation, as a predicate — **parsed, never byte-counted.**
func claimsTaggingItHasNot(pdf []byte) bool {
	claimed, elements := structureCount(pdf)
	return claimed && elements == 0
}
