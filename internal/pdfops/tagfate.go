package pdfops

import (
	"bytes"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Tag fate — `PLAN-accessibility.md` P01, ADR-031, and `/pending 29`'s floor.
//
// # The law this file exists to keep
//
// **Nothing claims tagging it has not.** No output may carry `/MarkInfo /Marked true`, a
// `/StructTreeRoot`, or a conformance assertion over content that is neither tagged nor marked as
// an artifact. *A visible loss is honest; a false claim is not, and it is worse than no tagging at
// all because it defeats the reader's own check* — a screen reader that is told a document is
// tagged stops looking for the fallbacks it would otherwise use.
//
// # This file has been wrong once, expensively, and the record of that is kept here
//
// The first version measured tagging with `bytes.Count(pdf, []byte("/StructElem"))`. **pdfcpu writes
// the structure tree into a compressed object stream**, so that count is `0` for every pdfcpu output
// whatever it contains — a perfectly tagged document and a stripped one are identical to it. On that
// evidence eight operations were reported as lying, enforcement was written against all of them, and
// it **stripped tag trees that had survived intact** (v1.129.9–.14, reverted at v1.129.15, never
// released). Everything in this file now parses. Nothing in it counts bytes.
//
// # What is true, measured three ways because two of them have been wrong before
//
// On a LibreOffice-produced tagged PDF — 4 pages, `/Marked true`, 45 `/StructElem`, every page
// carrying `/StructParents` — against pdfcpu v0.13.0:
//
//	operation              claim  elements  anchored  undescribed pages
//	Rotate / Optimize      yes    45        45        0            carried
//	Collect / RemovePages  no     0         —         —            dropped, honestly
//	Append(tagged first)   yes    45        45        1            PARTIAL
//	NUp(2)                 yes    45        **0**     2            **ORPHANED — law 1's violation**
//
// *Anchored* means the element's `/Pg` is a page still in the page tree. **`NUp` composes its
// sheets as new page objects and carries the old tree onto them**: all 45 elements point at pages
// that are no longer in the document, no composed page carries `/StructParents`, and the result
// says `/Marked true`. Nothing in that output is described by the tree it advertises.
//
// veraPDF agrees independently and names the law in its own words. ua1 against the same documents:
// the source and `Rotate`'s output fail the same three clauses (5 t1, 7.1 t9, 7.1 t10 — producer
// limitations); the n-up output fails **7.1 t3, *"Content shall be marked as Artifact or tagged as
// real content"*, with 24 failed checks** — a failure neither the input nor `Rotate` has.
//
// # Why the remedy is a POST-CONDITION and not a strip
//
// Dropping the claim is right exactly where the tree describes nothing that exists, because there
// is then nothing to preserve — and it is wrong everywhere else, which is what the reverted version
// got wrong. So `honest` asks the output a question and acts only on the answer, and `orphaned` is
// deliberately the most conservative form of that question it can be: it refuses to strip while
// *either* linkage survives — an element anchored to a live page, or any page carrying
// `/StructParents`.
//
// **Measured, so the remedy is not itself an untested claim:** stripping the n-up output's claim
// does not remove 7.1 t3 (the content is untagged either way) and adds 6.2 t1 and 7.1 t11, because
// PDF/UA requires a structure tree. A veraPDF failure *count* therefore scores honesty as a
// regression, which is why P01.S01's acceptance is a structural property and not a clause count.
//
// # One door
//
// Every operation that needs this calls `honest`, rather than each deleting two keys correctly.
// That is ADR-009's shape and law 2's. **The guard is stronger than the door**: the tag-fate table
// measures every operation it can drive and fails on an orphaned output whether or not that
// operation routes through here, so an operation that starts lying tomorrow goes red without
// anyone having remembered to wire it up.
//
// **The law itself is ADR-031.**

// tagState is what a document says about being tagged, set beside what it actually carries.
//
// The two are separate fields on purpose: law 1 is precisely the claim that the second does not
// support the first, and a single boolean cannot express it.
type tagState struct {
	readable bool // false when the document cannot be parsed at all
	marked   bool // /MarkInfo /Marked true
	tree     bool // /StructTreeRoot present in the catalog
	pages    int
	pagesSP  int // pages carrying /StructParents
	elements int // struct elements reachable from the tree root
	anchored int // of those, the ones whose /Pg is a page still in the page tree
	// undescribed counts pages that have a content stream and that **no struct element points at**.
	//
	// **It asked about `/StructParents` until v1.129.18, and that was the wrong question.**
	// `api.MergeRaw` merges two tagged documents by keeping the FIRST document's `/StructTreeRoot`
	// and `/ParentTree` whole while the second document's pages keep their own `/StructParents`
	// values — so an 8-page merge carries a 4-entry `/ParentTree` and eight pages indexing keys
	// 0–3. Pages 5–8 therefore HAVE a `/StructParents`, which the old predicate accepted, and it
	// resolves to structure describing entirely different content. Nothing in the tree points at
	// them, so a screen reader walking it never reaches those pages at all — under a document that
	// says `/Marked true`. **The census rated that state `carried`, its best verdict.**
	//
	// Reachability from the tree is the property that matters and it catches both shapes with one
	// predicate: an appended untagged page (no `/StructParents`, nothing points at it) and an
	// appended tagged page (a `/StructParents` that lies, nothing points at it).
	//
	// A page with no content stream at all is NOT undescribed — an inserted blank page has nothing
	// to tag, and counting it would make `InsertBlank` a violation for adding an empty page.
	undescribed int
}

// claims reports whether the document asserts tagging at all — either assertion law 1 names.
func (s tagState) claims() bool { return s.marked || s.tree }

// orphaned is law 1's violation: the document claims tagging, and the tree it advertises describes
// nothing that is in the document.
//
// **Both linkages have to be gone.** A tree anchors to content two ways — an element's `/Pg`
// pointing at a live page, and a page's `/StructParents` indexing into `/ParentTree` — and this
// refuses to strip while either survives. That conservatism is deliberate and is the direct lesson
// of the reverted version, which stripped on a predicate that could not see the tree at all.
//
// **Except when there is no tree, which the conservatism must not extend to.** `/MarkInfo /Marked
// true` with no `/StructTreeRoot` is a conformance assertion over nothing at all, and a page's
// `/StructParents` then indexes a `/ParentTree` that does not exist — so the second linkage is not
// a surviving anchor, it is a dangling number. Without this clause such a document scored
// `carried`, the census's BEST verdict, for having no structure whatsoever. Found by the P01 phase
// review; not reachable through any operation measured today, because pdfcpu drops `/MarkInfo` and
// the tree together, which is exactly why nothing had caught it.
func (s tagState) orphaned() bool {
	if !s.readable || !s.claims() {
		return false
	}
	if !s.tree {
		return true
	}
	return s.anchored == 0 && s.pagesSP == 0
}

// partial is a claim over a document where the tree is live but does not reach every page that has
// content. Recorded rather than enforced: see the tag-fate table's `partial` verdict.
func (s tagState) partial() bool {
	return s.readable && s.claims() && !s.orphaned() && s.undescribed > 0
}

// inspectTags parses a document and reports its tag state. **It parses; it never counts bytes.**
func inspectTags(pdf []byte) tagState {
	var s tagState
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return s // unreadable: not a claim we can judge, and readable=false says so
	}
	s.readable = true
	s.pages = ctx.PageCount
	live := map[int]bool{}       // object number of every page in the page tree
	hasContent := map[int]bool{} // ... of every page carrying a non-empty content stream
	for p := 1; p <= s.pages; p++ {
		d, _, _, perr := ctx.PageDict(p, false)
		if perr != nil || d == nil {
			continue
		}
		if _, ok := d["StructParents"]; ok {
			s.pagesSP++
		}
		ir, e := ctx.PageDictIndRef(p)
		if e != nil || ir == nil {
			continue
		}
		n := ir.ObjectNumber.Value()
		live[n] = true
		// `ErrNoContent` from an absent `/Contents` is the inserted-blank-page case and is not a
		// gap: a page with nothing on it has nothing to tag.
		if b, cerr := ctx.PageContent(d, p); cerr == nil && len(b) > 0 {
			hasContent[n] = true
		}
	}
	cat, cerr := ctx.XRefTable.Catalog()
	if cerr != nil {
		return s
	}
	if mi, ok := cat["MarkInfo"]; ok {
		if d, e := ctx.DereferenceDict(mi); e == nil && d != nil {
			if b := d.BooleanEntry("Marked"); b != nil && *b {
				s.marked = true
			}
		}
	}
	st, ok := cat["StructTreeRoot"]
	if !ok {
		return s
	}
	s.tree = true
	root, rerr := ctx.DereferenceDict(st)
	if rerr != nil || root == nil {
		return s
	}
	seen := map[string]bool{}
	described := map[int]bool{} // pages some struct element actually points at
	var walk func(o types.Object)
	walk = func(o types.Object) {
		if arr, e := ctx.DereferenceArray(o); e == nil && arr != nil {
			for _, x := range arr {
				walk(x)
			}
			return
		}
		d, e := ctx.DereferenceDict(o)
		if e != nil || d == nil {
			return
		}
		// A visited set, because a tree whose elements point back at their parents is ordinary and
		// `ContentDigest`'s non-termination (`/pending 454`) is this repo's standing lesson about
		// walking a PDF without one.
		key := d.String()
		if seen[key] {
			return
		}
		seen[key] = true
		if t := d.NameEntry("Type"); t != nil && *t == "StructElem" {
			s.elements++
			if pg, ok := d["Pg"]; ok {
				if ind, isInd := pg.(types.IndirectRef); isInd && live[ind.ObjectNumber.Value()] {
					s.anchored++
					described[ind.ObjectNumber.Value()] = true
				}
			}
		}
		if k, ok := d["K"]; ok {
			walk(k)
		}
	}
	walk(root["K"])
	for n := range hasContent {
		if !described[n] {
			s.undescribed++
		}
	}
	return s
}

// ClaimsTagging reports whether a document asserts that it is tagged — **either** assertion law 1
// names, because `/MarkInfo /Marked true` with no tree is as much a claim as a tree is.
//
// The server's tagging notice asks this before and after a mutation; it is the only exported
// member of this file.
func ClaimsTagging(pdf []byte) bool { return inspectTags(pdf).claims() }

// honest is the one door. It returns the document unchanged unless the document claims tagging over
// a tree that describes nothing in it, in which case it removes the claim.
//
// **The unchanged path costs a parse and never a rewrite**, which matters for more than speed: a
// rewrite re-encodes, which moves bytes and invalidates any signature over them.
//
// **Measured at 88 ms on a 1.4 MB, 22-page document** — the parse, against `api.NUp`'s own 147 ms
// for the composition. Its caller asks `inspectTags(...).orphaned()` directly before attempting a
// carry, so since v1.129.19 `honest` is reached only by a document whose tree was re-anchored, and
// what it costs there is the verification of that remap rather than a toll on every n-up.
//
// **There is a cheap pre-filter available and it is deliberately NOT taken.** The catalog-level keys
// do survive uncompressed, so a byte scan for `/StructTreeRoot` would skip the parse for the
// overwhelmingly common untagged document. A false negative there — a catalog written into an
// object stream, which PDF 1.5 permits — would silently switch law 1 off for `NUp` and leave no
// trace, and that is the exact failure this file exists because of. 123 ms is cheaper than finding
// out.
func honest(pdf []byte) ([]byte, error) {
	if !inspectTags(pdf).orphaned() {
		return pdf, nil
	}
	return dropTaggingClaim(pdf)
}

// dropTaggingClaim removes `/StructTreeRoot` and `/MarkInfo` from the catalog.
//
// **Unexported and called only through `honest`**, so the decision to strip is taken in exactly one
// place against exactly one predicate. The reverted version had the strip reachable on its own, and
// that is how it came to run over documents whose trees were intact.
//
// **That sentence was false for one commit** (v1.129.19): `NUp` called this directly, having already
// established `orphaned`, to save a re-parse. The saving was real and the precedent was not worth
// it — a second caller is how the predicate and the strip drift apart, and this comment asserting
// otherwise is the shape of defect this repo keeps paying for. Restored to one caller at the P01
// phase review; `zerocaller_test.go` cannot police this, because the function is unexported and has
// a caller either way.
func dropTaggingClaim(pdf []byte) ([]byte, error) {
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return nil, err
	}
	cat, err := ctx.XRefTable.Catalog()
	if err != nil {
		return nil, err
	}
	delete(cat, "StructTreeRoot")
	delete(cat, "MarkInfo")
	var out bytes.Buffer
	if err := api.WriteContext(ctx, &out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
