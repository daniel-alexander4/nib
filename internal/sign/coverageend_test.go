package sign

import (
	"testing"

	"nib/internal/testpdf"
)

// TestEachSignerCarriesItsOwnCoverageEnd — PLAN-returned-document P03.S02 (D9): Signers come in the library's
// object-number order, not coverage order, so "before or after yours" is read from each signer's own coverage end.
func TestEachSignerCarriesItsOwnCoverageEnd(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	a, b := newIdentity(t, "Alice"), newIdentity(t, "Bob")
	sA := signAs(t, base, a, "first")
	sAB := signAs(t, sA, b, "second")
	st := Verify(sAB)
	if len(st.Signers) != 2 {
		t.Fatalf("stimulus: %d signers, want 2", len(st.Signers))
	}
	want := map[string]int64{a.fp: int64(len(sA)), b.fp: int64(len(sAB))}
	for _, s := range st.Signers {
		if w, ok := want[s.Fingerprint]; !ok || s.CoverageEnd != w {
			t.Errorf("signer %.16s… covers to %d, want %d (its own signed version's length)", s.Fingerprint, s.CoverageEnd, w)
		}
	}
}
