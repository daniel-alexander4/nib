package sign

import "testing"

// TestASignatureFieldNestedUnderKidsIsSigned — the P08 phase-close review, C1: `HasSignatureBlob` read `/Fields`' top
// level only, with `/FT` taken from the field itself. A validly signed document whose signature field sits under a
// parent's `/Kids`, or inherits `/FT` from its parent, answered false — and every gate on the answer (the reflow refusal,
// tag writes, the co-sign arrival gate, undo's signature guard, the PDF/UA drop) treated it as unsigned.
//
// **The stimulus is asserted**: each document verifies valid, with one signer.
func TestASignatureFieldNestedUnderKidsIsSigned(t *testing.T) {
	a := newIdentity(t, "Alice")
	for _, c := range []struct {
		name        string
		parent, kid string
	}{
		{"the field under a parent's /Kids", "<</T(parent)/Kids[6 0 R]>>", "<</FT/Sig/T(child)/Parent 4 0 R/V 5 0 R>>"},
		{"/FT inherited from the parent", "<</FT/Sig/T(parent)/Kids[6 0 R]>>", "<</T(child)/Parent 4 0 R/V 5 0 R>>"},
	} {
		t.Run(c.name, func(t *testing.T) {
			objs := baseObjs(sigDict("1", ""))
			objs[3] = sobj{num: 4, body: c.parent}
			objs = append(objs, sobj{num: 6, body: c.kid})
			doc := fillSig(t, synthRevision(t, nil, objs, 1), "1", nil, detached(t, a))
			if st := Verify(doc); st.State != "valid" || len(st.Signers) != 1 {
				t.Fatalf("setup: Verify reads %s with %d signers, want valid with one", st.State, len(st.Signers))
			}
			if !HasSignatureBlob(doc) {
				t.Errorf("HasSignatureBlob is false for a validly signed document")
			}
		})
	}
	// The control: a parent with an unsigned kid (no /V) is not signed.
	objs := baseObjs(sigDict("1", ""))
	objs[3] = sobj{num: 4, body: "<</FT/Sig/T(parent)/Kids[6 0 R]>>"}
	objs = append(objs, sobj{num: 6, body: "<</T(child)/Parent 4 0 R>>"})
	if HasSignatureBlob(synthRevision(t, nil, objs, 1)) {
		t.Errorf("an empty nested signature field reads as signed")
	}
}
