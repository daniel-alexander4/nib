package ceremony

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"nib/internal/p2p"
	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// TestADigestVersionSkewSaysSoRatherThanAccusing — D32 applied to the content-digest rule.
//
// `pdfops.ContentDigestVersion`'s own doc says it exists so that "improving the coverage"
// does not "accuse a counterparty of tampering". **Measured at the P07.S02 grill: it could
// not do that**, because it was bound INTO the digest and carried nowhere beside it — three
// occurrences in the whole tree, no reader. Binding a version inside a hash changes the
// number; it cannot produce a sentence, because nothing has anything to compare.
//
// The failure this prevents is a Nib point release telling a solicitor that a counterparty
// substituted their document. The two outcomes are opposite in meaning and must be opposite
// in wording.
func TestADigestVersionSkewSaysSoRatherThanAccusing(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := p2p.PrepareDocument(base)
	if err != nil {
		t.Fatal(err)
	}
	h, err := DocumentHash(prepared)
	if err != nil {
		t.Fatal(err)
	}
	r := draft(t, cfp, afp)
	r.DocHash = h
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	doc, err := Embed(prepared, r)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	// Setup assertion: it must pass BEFORE the skew, or the refusal below proves nothing.
	if _, err := CheckDocument(doc, now); err != nil {
		t.Fatalf("setup: a freshly convened document must pass CheckDocument: %v", err)
	}

	// The skew: a record written under a digest rule this build does not use. Built by
	// re-signing rather than by editing the JSON, because DigestVersion is inside the
	// preimage — an edited copy would fail as a bad signature and prove the wrong thing.
	skewed := draft(t, cfp, afp)
	skewed.DocHash = h
	skewed.DigestVersion = pdfops.ContentDigestVersion + 1
	if err := skewed.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	if skewed.DigestVersion == pdfops.ContentDigestVersion {
		t.Fatal("Sign overwrote the skewed DigestVersion — this test cannot construct its own " +
			"stimulus and would pass for the wrong reason")
	}
	doc2, err := Embed(prepared, skewed)
	if err != nil {
		t.Fatal(err)
	}
	_, err = CheckDocument(doc2, now)
	if !errors.Is(err, ErrDigestVersion) {
		t.Fatalf("a digest-rule skew reported %v — want ErrDigestVersion. A hash mismatch says "+
			"somebody changed the document; this says two builds measure it differently and "+
			"nobody has done anything wrong.", err)
	}
	// The sentence, not just the sentinel: it must name BOTH numbers, or the reader cannot
	// tell which side is behind.
	msg := err.Error()
	for _, want := range []string{
		fmt.Sprintf("rule %d", skewed.DigestVersion),
		fmt.Sprintf("rule %d", pdfops.ContentDigestVersion),
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("the skew sentence does not contain %q: %s", want, msg)
		}
	}
	if strings.Contains(msg, "not the same document") {
		t.Errorf("the skew produced the TAMPERING sentence: %s", msg)
	}
}
