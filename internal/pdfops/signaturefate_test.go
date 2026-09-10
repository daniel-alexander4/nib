package pdfops

import (
	"testing"

	"nib/internal/sign"
	"nib/internal/testpdf"
)

// TestWhatEachDocumentPrimitiveDoesToASignature — /pending 455.
//
// # Why this is measured rather than written down
//
// The entry that opened this said the split was `writeMutated` (loud) against `Collect`/`MergeRaw`
// (silent), and scoped a server-side refusal to "the routes that rebuild". **Running it falsified
// that in two places at once**: `api.MergeRaw` PRESERVES the signature blob, and `api.RemovePages`
// erases it without being a rebuild path at all. A taxonomy by primitive family, or by route, is
// wrong — and it was wrong in a way that only running it could show, because every one of these is
// a pdfcpu call whose behaviour is not visible from its name.
//
// **It is also not ours to keep true.** Each row here is pdfcpu's behaviour, not Nib's, so a
// dependency upgrade can flip one silently. That is the whole reason this exists as a table with
// recorded expectations rather than as a sentence in a comment: the day `api.Rotate` starts
// dropping the AcroForm, or `Collect` starts keeping it, this goes red and the two doors that
// refuse on it (`commitMutation`, `commitBarrier`) get re-read.
//
// **The refusal itself does NOT read this table**, and that is deliberate. Those doors compare the
// document they hold against the result they were handed and refuse when a signature went missing —
// an observation, not a prediction. A predicted set can be stale; an observed one cannot. This table
// exists so that a change in the fates is *visible*, not so that anything branches on it.
func TestWhatEachDocumentPrimitiveDoesToASignature(t *testing.T) {
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	// Four pages, so the page operations have something to work on. Built with Append, which is
	// one of the primitives under test — on an UNSIGNED document, where its fate is not the
	// question.
	for i := 0; i < 3; i++ {
		other, terr := testpdf.Text("another page")
		if terr != nil {
			t.Fatal(terr)
		}
		if base, err = Append(base, other); err != nil {
			t.Fatal(err)
		}
	}
	cert, key, err := sign.GenerateIdentity("Signer")
	if err != nil {
		t.Fatal(err)
	}
	signed, err := sign.SignApproval(base, cert, key, sign.Options{Name: "A", Reason: "r"})
	if err != nil {
		t.Fatal(err)
	}
	// The stimulus floor, and it is the one that matters: if the fixture is not actually signed,
	// every row below reads "erases" and the table agrees with itself about nothing.
	if st := sign.Verify(signed); st.State != sign.Valid || !sign.HasSignatureBlob(signed) {
		t.Fatalf("setup: the fixture did not come out validly signed (state=%s blob=%v) — every "+
			"row below would report an erasure that is the fixture's, not the primitive's",
			st.State, sign.HasSignatureBlob(signed))
	}

	const (
		breaks = "breaks" // the signature survives as evidence: state invalid, blob present
		erases = "erases" // no trace remains: state unsigned, no blob
	)
	for _, c := range []struct {
		name, want, where string
		op                func([]byte) ([]byte, error)
	}{
		{"Rotate", breaks, "/api/pages op=rotate",
			func(b []byte) ([]byte, error) { return Rotate(b, []string{"1"}, 90) }},
		{"InsertBlank", breaks, "/api/pages op=insertblank",
			func(b []byte) ([]byte, error) { return InsertBlank(b, 1) }},
		{"Append", breaks, "/api/pages op=append",
			func(b []byte) ([]byte, error) { return Append(b, base) }},
		{"RemovePages", erases, "/api/pages op=delete",
			func(b []byte) ([]byte, error) { return RemovePages(b, []string{"4"}) }},
		{"Collect", erases, "/api/pages op=reorder, and /api/export's extract",
			func(b []byte) ([]byte, error) { return Collect(b, []string{"2", "1"}) }},
	} {
		out, oerr := c.op(signed)
		if oerr != nil {
			t.Errorf("%s: %v", c.name, oerr)
			continue
		}
		got := breaks
		if !sign.HasSignatureBlob(out) {
			got = erases
		}
		if got != c.want {
			t.Errorf("%s (%s) now %s a signature, and this table recorded %s.\n"+
				"    This is pdfcpu's behaviour rather than Nib's, so an upgrade can flip it. "+
				"Re-read the two doors that refuse on a vanished signature — commitMutation and "+
				"commitBarrier — and the confirm wording the client shows before one.",
				c.name, c.where, got, c.want)
		}
		// The state and the blob must agree, or "erases" and "breaks" are not the two cases.
		if st := sign.Verify(out); (st.State == sign.Unsigned) != (got == erases) {
			t.Errorf("%s: blob says %q and sign.Verify says state=%s — the two halves of this "+
				"classification disagree, so neither is a reliable test for the other",
				c.name, got, st.State)
		}
	}
}
