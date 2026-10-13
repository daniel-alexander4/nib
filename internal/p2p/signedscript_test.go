package p2p

import (
	"strings"
	"testing"
	"time"

	"nib/internal/sign"
)

// TestAContributionInAnyScriptIsSignedAsWritten is /pending 879 at the door a ceremony signs through
// (ADR-163): an attestation whose signer, pinned label and recital are in Japanese, Arabic and
// Chinese is signed, verifies, and its `/Reason` reads back as the attestation's own sentence. Each
// of the three texts holds a character the signing library used to write as a broken string — 山
// (U+5C71), ب (U+0628), 就 (U+5C31) — so ADR-158 drew the block for a signature that was then
// refused, or, for the third, made over a reason nobody wrote.
func TestAContributionInAnyScriptIsSignedAsWritten(t *testing.T) {
	a := l3Identity(t, "A")
	att := Attestation{Signer: "山田 太郎", AcceptedPeer: strings.Repeat("ab12", 16),
		AcceptedPeerLabel: "محمد العربي", Intent: "就这样签署。", When: time.Date(2026, 10, 12, 14, 30, 0, 0, time.UTC)}
	// STIMULUS: the block itself is drawable, so a failure below is the signing and not ADR-158's door.
	if _, err := att.BlockImage(); err != nil {
		t.Fatalf("STIMULUS: the block does not draw: %v", err)
	}
	doc := l3Prepared(t)
	place, err := NextPlacement(doc)
	if err != nil {
		t.Fatal(err)
	}
	signed, err := Contribute(doc, []byte(a.cert), []byte(a.key), att, place)
	if err != nil {
		t.Fatalf("a contribution in Japanese, Arabic and Chinese was not signed: %v", err)
	}
	st := sign.Verify(signed)
	if st.State != sign.Valid || len(st.Signers) != 1 {
		t.Fatalf("state %s with %d signers, want valid/1", st.State, len(st.Signers))
	}
	if s := st.Signers[0]; s.Name != att.Signer || s.Reason != att.reason() {
		t.Errorf("signed as name %q reason %q\nwant      name %q reason %q", s.Name, s.Reason, att.Signer, att.reason())
	}
}
