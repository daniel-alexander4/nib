package p2p

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"nib/internal/sign"
	"nib/internal/testpdf"
)

// TestACoSignedReturnStopsAtTheLastSignature — /pending 629.
//
// `confirmCoSigned` checked each party's signature and never asked whether the document ended
// there: what went out, the peer's valid signature, then an unsigned update passed and was kept as
// the co-signed result. `Initiate`'s prefix check cannot see it either — the bytes DID grow from
// mine.
func TestACoSignedReturnStopsAtTheLastSignature(t *testing.T) {
	aCert, aKey := newIdentity(t) // initiator
	bCert, bKey := newIdentity(t) // peer
	aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)
	bAcceptsA := Attestation{Signer: "Bob", AcceptedPeer: hex.EncodeToString(aFP), AcceptedPeerLabel: "Alice", Intent: "I accept", When: time.Now()}
	mine := signAsInitiator(t, aCert, aKey, bFP)
	mutual := contribute(t, mine, bCert, bKey, bAcceptsA)

	// The control first: the honest return is accepted, or the refusal below is a check that
	// refuses everything.
	if err := confirmCoSigned(mine, mutual, bFP, aFP, false); err != nil {
		t.Fatalf("setup: the honest co-signed return was refused: %v", err)
	}

	appended := append(append([]byte{}, mutual...), "\n% content added after signing\n"...)
	// STIMULUS: both signatures still verify on it, so nothing but the coverage verdict can refuse.
	st := sign.Verify(appended)
	if st.State != sign.Valid || len(st.Signers) != 2 || st.AddedAfterCause != sign.AddedAfterAppended {
		t.Fatalf("setup: the appended return reads state %v, %d signer(s), cause %q — want two valid "+
			"signatures and `appended`", st.State, len(st.Signers), st.AddedAfterCause)
	}
	for _, inCeremony := range []bool{false, true} {
		err := confirmCoSigned(mine, appended, bFP, aFP, inCeremony)
		if err == nil {
			t.Fatalf("inCeremony=%t: a return carrying content after the peer's signature was "+
				"accepted as the co-signed document", inCeremony)
		}
		if !strings.Contains(err.Error(), "content added after the last signature") {
			t.Errorf("inCeremony=%t: refused with %q, which does not say why", inCeremony, err)
		}
	}

	// The third cause. A refused copy after the last signature was the named exemption while
	// ADR-060's Consequences bound `confirmCoSigned` to answer on it as on the untouched file; for
	// a RETURNED document that binding is superseded, and the copy is refused in its own words.
	copied, _, err := testpdf.CopiedSignatureDictionary(mutual, testpdf.CopyExact)
	if err != nil {
		t.Fatal(err)
	}
	if cs := sign.Verify(copied); cs.AddedAfterCause != sign.AddedAfterRefusedSignature {
		t.Fatalf("setup: the copy reads cause %q, want %q", cs.AddedAfterCause, sign.AddedAfterRefusedSignature)
	}
	for _, inCeremony := range []bool{false, true} {
		err := confirmCoSigned(mine, copied, bFP, aFP, inCeremony)
		if err == nil {
			t.Fatalf("inCeremony=%t: a return carrying a refused signature copy after the peer's "+
				"signature was accepted as the co-signed document", inCeremony)
		}
		if !strings.Contains(err.Error(), "a signature Nib refused") {
			t.Errorf("inCeremony=%t: refused with %q, which does not name the refused signature", inCeremony, err)
		}
	}
}

// TestTheReturnDoorFailsClosedWhenCoverageCouldNotBeChecked — the third cause, at the door itself:
// no document reaches it through `confirmCoSigned` with both signatures still counted, so it is
// asked directly.
func TestTheReturnDoorFailsClosedWhenCoverageCouldNotBeChecked(t *testing.T) {
	err := nothingAfterTheLastSignature(sign.Status{AddedAfter: true, AddedAfterCause: sign.AddedAfterCouldNotCheck})
	if err == nil || !strings.Contains(err.Error(), "could not be checked") {
		t.Errorf("a return whose coverage could not be checked answered %v, want a refusal saying so", err)
	}
	if err := nothingAfterTheLastSignature(sign.Status{}); err != nil {
		t.Errorf("a return with nothing after its last signature was refused: %v", err)
	}
}
