package server

import (
	"encoding/hex"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"nib/internal/p2p"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// TestTheInitiatingSideIsGatedToo — L3 at the entry point in THIS package, driven through the
// function the route calls rather than through the predicate.
//
// The receiving side's gate is exercised in `internal/p2p`; this is the other half, and it is the
// one where refusing late would be irreversible: `buildCoSigned` applies the LOCAL user's
// signature, and a signature cannot be taken back off a document. The ordering is asserted by
// checking that nothing was signed, not merely that an error came back.
func TestTheInitiatingSideIsGatedToo(t *testing.T) {
	aCert, aKey, err := sign.GenerateIdentity("A")
	if err != nil {
		t.Fatal(err)
	}
	aFPb, _ := sign.Fingerprint(aCert)
	aFP := hex.EncodeToString(aFPb)
	bFP := strings.Repeat("bb", 32)

	doc, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{epoch: "test-epoch"}
	att := p2p.Attestation{Signer: "A", AcceptedPeer: bFP, Intent: "ok", When: time.Now()}

	// The roster puts B first, so it is not A's turn.
	roster := p2p.Roster{Entries: []p2p.RosterEntry{
		{Fingerprint: bFP, Signs: true},
		{Fingerprint: aFP, Signs: true},
	}}

	// The CONTROL first, and it is what stops this being a test of a gate that refuses
	// everything: with A first in the roster, A may sign.
	ok := p2p.Roster{Entries: []p2p.RosterEntry{
		{Fingerprint: aFP, Signs: true},
		{Fingerprint: bFP, Signs: true},
	}}
	w := httptest.NewRecorder()
	signed, good := s.buildCoSigned(w, doc, aCert, aKey, att, nil, ok)
	if !good {
		t.Fatalf("the party whose turn it IS was refused: %d %s", w.Code, w.Body.String())
	}
	if st := sign.Verify(signed); st.State != sign.Valid {
		t.Fatalf("setup: the control did not produce a valid signature (%s)", st.State)
	}

	// And the refusal.
	w2 := httptest.NewRecorder()
	out, good2 := s.buildCoSigned(w2, doc, aCert, aKey, att, nil, roster)
	if good2 {
		t.Fatal("a party signed out of roster order through the initiating door")
	}
	if out != nil {
		t.Error("the refusal returned a document")
	}
	if w2.Code != 409 {
		t.Errorf("the refusal is %d, want 409 — this is a refusal about the STATE of a "+
			"proceeding, and the user's action is to wait rather than to correct a field",
			w2.Code)
	}
	if !strings.Contains(w2.Body.String(), "not this party's turn") {
		t.Errorf("the refusal does not name its reason: %s", w2.Body.String())
	}
	// **Nothing was signed, and this is the assertion that makes the ordering load-bearing.**
	// A gate that ran after `Contribute` could return exactly the error above while leaving the
	// user's signature on the document — and there is no way to take one back off.
	if st := sign.Verify(doc); st.State != sign.Unsigned {
		t.Errorf("the input document is %s after a refused contribution — the gate ran after "+
			"the signature was applied", st.State)
	}
}

// TestTheManualCoSignPathIsNotGated — the other half of T05's conditioning, driven.
//
// The gate exists only where there is a roster. An ordinary two-party co-sign has none, and if
// the zero Roster were read as "an empty signing order that nobody is in", every manual co-sign
// in the product would be refused.
func TestTheManualCoSignPathIsNotGated(t *testing.T) {
	aCert, aKey, err := sign.GenerateIdentity("A")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{epoch: "test-epoch"}
	att := p2p.Attestation{Signer: "A", AcceptedPeer: strings.Repeat("bb", 32), Intent: "ok", When: time.Now()}
	w := httptest.NewRecorder()
	if _, ok := s.buildCoSigned(w, doc, aCert, aKey, att, nil, p2p.Roster{}); !ok {
		t.Fatalf("a manual co-sign with no ceremony was refused: %d %s", w.Code, w.Body.String())
	}
}

// TestARefusalIsNotReportedAsAConnectFailure — the layer above the wire, and the one that undid it.
//
// Measured at tier 4 on the day the wire started carrying names: the refusal crossed correctly and
// came out of the API as
// `{"error":"could not connect to peer: a co-signature takes exactly one prior signer"}` — a 502,
// wrapped in a false claim, and on the ceremony path also given a D19 *network* cause. The peer
// connected perfectly well and said no.
//
// `verify.go` states the harm for its own case in words that apply unchanged: "could not connect"
// invites a retry, and a retry is the wrong advice for every one of these.
//
// **Asserted on the ROUTING and on the whole enumeration**, not on one sentence: the failure mode
// is a class of refusals falling through, so a test naming one of them would go green while eight
// others still landed in the 502.
func TestARefusalIsNotReportedAsAConnectFailure(t *testing.T) {
	// Every refusal the wire can carry must be recognised as one.
	for _, err := range []error{
		p2p.ErrNotYourTurn, p2p.ErrNotInRoster, p2p.ErrPrefixMismatch, p2p.ErrPrefixUnproven,
		p2p.ErrProceedingMismatch, p2p.ErrCeremonyComplete, p2p.ErrNotTheConnectedPeer,
		p2p.ErrPeerDoesNotAcceptYou, p2p.ErrWrongPriorSignerCount, p2p.ErrRefusedUnknown,
	} {
		if !p2p.IsContributionRefusal(err) {
			t.Errorf("%v is not recognised as a refusal, so it reaches writeConnectDiagnosis "+
				"and is reported as a 502 'could not connect to peer' with a D19 network "+
				"cause — for an exchange the peer connected to and refused", err)
		}
		// And it must NOT be dressed as a connect failure even if something calls that directly.
		if got := connectFailure(err); strings.Contains(got, "could not connect to peer") {
			t.Errorf("connectFailure(%v) = %q — a refusal wearing a transport sentence", err, got)
		}
	}
	// The control, and it is what stops the predicate becoming "everything is a refusal": a
	// genuine transport error still is one.
	transport := errors.New("tried 3 address(es), none answered as the pinned peer")
	if p2p.IsContributionRefusal(transport) {
		t.Error("a genuine dial failure is being reported as a refusal, so a user whose network " +
			"is broken is told the other party said no")
	}
	if !strings.Contains(connectFailure(transport), "could not connect to peer") {
		t.Error("a genuine dial failure lost its connect sentence")
	}

	// **The routing** — the handler lifts refusals BEFORE writeConnectDiagnosis. Asserting the
	// predicate alone says nothing about whether anything calls it.
	src, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatal(err)
	}
	code := stripLineComments(string(src))
	i := strings.Index(code, "func (s *Server) handleSessionInitiate(")
	if i < 0 {
		t.Fatal("cannot find handleSessionInitiate")
	}
	body := funcBodyFrom(code, i)
	lift := strings.Index(body, "IsContributionRefusal(")
	diag := strings.Index(body, "writeConnectDiagnosis(")
	if lift < 0 {
		t.Fatal("handleSessionInitiate does not lift contribution refusals at all")
	}
	if diag < 0 {
		t.Fatal("cannot find writeConnectDiagnosis in handleSessionInitiate")
	}
	if lift > diag {
		t.Error("refusals are lifted AFTER writeConnectDiagnosis, so they never reach the lift — " +
			"the diagnosis has already written a 502 and chosen a D19 cause")
	}
}
