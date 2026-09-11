package pdfops

import (
	"bytes"
	"testing"

	"nib/internal/testpdf"
)

// Tag fate — `PLAN-accessibility.md` P01.S01, the floor `/pending 29` rests on.
//
// # The law
//
// **Nothing claims tagging it does not have.** A visible loss is honest; a false claim is worse
// than no tagging at all, because a screen reader told a document is tagged stops reaching for the
// fallbacks it would otherwise use.
//
// # Why the oracle here is STRUCTURAL and not veraPDF, which the plan asked for
//
// P01.S01's acceptance said *"a tagged input through `nup` produces no veraPDF failure the input
// did not already have"*. **Measured, that criterion can only be met by the option the same slice
// defers.** Dropping the claim is honest and is what law 1 demands — and it necessarily ADDS ua1
// failures, because PDF/UA requires a structure tree: an untagged file fails 6.2 t1 (*"MarkInfo …
// Marked … true"*) and 7.1 t11 (*"logical structure … rooted in the StructTreeRoot entry"*) by
// construction. Measured on the fixture below: input fails {7.1 t8, 7.1 t10, 7.21.4.1 t1}; n-upped
// before the fix fails those **plus 7.1 t3**; n-upped after the fix fails those plus 7.1 t3, 6.2 t1
// and 7.1 t11.
//
// So a ua1 failure COUNT cannot express law 1 — it scores honesty as a regression. What law 1
// actually says is a structural property of the output, and that is what is asserted here. The plan
// is amended in place with this measurement.
//
// The fixture is built inline for this slice only; P01.S03's acceptance moves it to D12's corpus.

// TestNUpDoesNotClaimTaggingItVoided — P01.S01's whole point.
//
// **The setup assertion is half the test.** Without it, "the output makes no claim" is satisfied by
// a fixture that never made one, which is the vacuous green this repo keeps finding — and it would
// be especially easy here, since every other fixture in this package is untagged.
func TestNUpDoesNotClaimTaggingItVoided(t *testing.T) {
	src := taggedFixture()
	in := claims(src)
	if in["/StructTreeRoot"] == 0 || in["/Marked"] == 0 || in["/StructElem"] == 0 {
		t.Fatalf("setup: the fixture is not actually tagged (%v), so every assertion below would "+
			"pass on a build that does nothing at all", in)
	}

	out, err := NUp(src, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	got := claims(out)
	for _, k := range []string{"/StructTreeRoot", "/MarkInfo", "/Marked", "/StructParents"} {
		if got[k] != 0 {
			t.Errorf("n-up output still carries %s (%d). Measured on pdfcpu v0.13.0, `api.NUp` keeps "+
				"the catalog's tagging claim while dropping every /StructElem and every page's "+
				"/StructParents — so the document says it is tagged over content nothing describes, "+
				"which is worse than no tagging because it defeats the reader's own check. "+
				"Full claims: %v", k, got[k], got)
		}
	}
	// The plan's second acceptance bullet, and dropping satisfies it by construction: no struct
	// element survives, so none can point at a page that is not in the document. Asserted rather
	// than reasoned, because "by construction" is how a criterion stops being checked.
	if got["/StructElem"] != 0 {
		t.Errorf("%d struct element(s) survived the n-up. Any that did would point at the pages of "+
			"the document that went IN, which are not the pages that came out", got["/StructElem"])
	}
}

// TestAnUntaggedDocumentIsNotRewrittenByTheDrop.
//
// A rewrite is not free: it re-encodes, which moves bytes and would invalidate a signature. The
// claim-removal must therefore be a no-op on the overwhelming majority of documents, which carry no
// claim at all — and "no-op" has to mean the same bytes, not merely the same meaning.
func TestAnUntaggedDocumentIsNotRewrittenByTheDrop(t *testing.T) {
	src, err := testpdf.Text("an ordinary untagged document")
	if err != nil {
		t.Fatal(err)
	}
	if c := claims(src); c["/StructTreeRoot"] != 0 || c["/Marked"] != 0 {
		t.Fatalf("setup: the untagged fixture carries a claim (%v)", c)
	}
	out, derr := dropTaggingClaim(src)
	if derr != nil {
		t.Fatal(derr)
	}
	if !bytes.Equal(out, src) {
		t.Errorf("an untagged document came back %d bytes instead of %d — the drop re-encoded a "+
			"document that had nothing to remove, which moves every byte and would invalidate a "+
			"signature on the way past", len(out), len(src))
	}
}

// TestTheDoorRemovesThePageBackReferenceToo.
//
// **This is asserted on the DOOR, not through `nup`, and the distinction is the point.** Measured:
// `api.NUp` has already dropped `/StructParents` by the time the claim-removal runs, so the line
// that deletes it is inert on that path — a mutation removing it leaves the n-up test green. It is
// not dead code: `/StructParents` is the page's index into `/ParentTree`, and an operation that
// keeps its pages (P01.S04's carriers, and `rotate` today) reaches this door with the key intact,
// where leaving it behind is a dangling reference into a tree that no longer exists.
//
// So the coverage is taken where the behaviour lives. Claiming the n-up test covered it would have
// been a check satisfied by a different function's side effect.
func TestTheDoorRemovesThePageBackReferenceToo(t *testing.T) {
	src := taggedFixture()
	if claims(src)["/StructParents"] == 0 {
		t.Fatal("setup: the fixture has no /StructParents, so this test cannot show it is removed")
	}
	out, err := dropTaggingClaim(src)
	if err != nil {
		t.Fatal(err)
	}
	if got := claims(out)["/StructParents"]; got != 0 {
		t.Errorf("%d /StructParents survived the drop. It is the page's index into /ParentTree, and "+
			"the tree is gone — so it points into nothing, which is a dangling reference rather "+
			"than a stale but harmless integer", got)
	}
}

// TestBookletInheritsTheSameHonesty — `nib booklet` composes through NUp, so the claim has to go
// there too. Asserted rather than assumed: the composition is one line and could be re-routed.
func TestBookletInheritsTheSameHonesty(t *testing.T) {
	src := taggedFixture()
	out, err := Booklet(src, false)
	if err != nil {
		t.Fatal(err)
	}
	got := claims(out)
	if got["/StructTreeRoot"] != 0 || got["/Marked"] != 0 {
		t.Errorf("a booklet imposed from a tagged document still claims tagging (%v). It composes "+
			"pages exactly as n-up does, so it voids structure exactly as n-up does", got)
	}
}
