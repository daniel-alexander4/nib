package pdfops

import (
	"bytes"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
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

// dropTaggingClaim removes every assertion that a document is tagged, and returns pdf unchanged
// when there was no claim to remove.
//
// **`/StructParents` goes too, and it is not decoration.** It is the page's index into
// `/ParentTree`; left behind after the tree is gone it points into nothing, which is a dangling
// reference rather than a stale-but-harmless integer. `/StructParent` on annotations is the same
// key one level down and is removed for the same reason.
func dropTaggingClaim(pdf []byte) ([]byte, error) {
	// **Read once to ASK, and return the original bytes when there is nothing to remove.** A
	// rewrite is not free — it re-encodes, which moves bytes and would invalidate a signature — so
	// a document that never claimed to be tagged must come through untouched rather than
	// "unchanged apart from the bytes".
	probe, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil || !hasTaggingClaim(probe) {
		return pdf, nil
	}
	return writeMutated(pdf, func(ctx *model.Context) error {
		root, err := ctx.XRefTable.Catalog()
		if err != nil {
			return err
		}
		delete(root, "StructTreeRoot")
		delete(root, "MarkInfo")
		// The pages' own back-references. Walked through `ctx.PageDict`, which resolves the page
		// tree rather than assuming a flat `/Kids` — a document with intermediate page-tree nodes
		// is ordinary and a naive walk would miss every page under one.
		n := ctx.PageCount
		for i := 1; i <= n; i++ {
			d, _, _, perr := ctx.PageDict(i, false)
			if perr != nil || d == nil {
				continue // a page that will not resolve has no claim of ours to remove
			}
			delete(d, "StructParents")
			// Annotations carry the same key one level down.
			if arr := d.ArrayEntry("Annots"); arr != nil {
				for _, o := range arr {
					ad, aerr := ctx.DereferenceDict(o)
					if aerr != nil || ad == nil {
						continue
					}
					delete(ad, "StructParent")
				}
			}
		}
		return nil
	})
}

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
