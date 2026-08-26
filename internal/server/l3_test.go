package server

import (
	"bytes"
	"encoding/hex"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
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

// TestTheAttestationsRouteDoesNotReVerify — P07.S04 clause 6, asserted on the ROUTING because the
// cost cannot be asserted at this granularity.
//
// The route was calling `p2p.ReadAttestations(s.docBytes(doc))`, which verifies the whole file
// again — while `document.sig` sits beside the bytes, computed wherever they were installed.
// Measured at the slice's grill: the cost is dominated by document SIZE rather than by signature
// count, because each signature's byte range is hashed over the whole file. Nine signatures on a
// 31 KB document is single-digit milliseconds; the plan's 5.2 s figure needs ~95 MiB.
//
// **So this guard is structural, and says why rather than pretending otherwise.** A timing
// assertion on a small fixture would measure noise, and building a 95 MiB fixture in a unit test
// trades a request-path regression for a minute on every `go test` run. What is checkable, and
// what actually regresses, is whether the handler re-verifies at all.
func TestTheAttestationsRouteDoesNotReVerify(t *testing.T) {
	src, err := os.ReadFile("cosign.go")
	if err != nil {
		t.Fatal(err)
	}
	code := stripLineComments(string(src))
	i := strings.Index(code, "func (s *Server) handleAttestations(")
	if i < 0 {
		// The handler may be named differently; find it by the response type it writes.
		i = strings.Index(code, "attestationsResponse{Attestations: views}")
		if i < 0 {
			t.Fatal("cannot find the attestations handler — this guard is reading the wrong file")
		}
		i = strings.LastIndex(code[:i], "func (s *Server) ")
	}
	body := funcBodyFrom(code, i)
	if body == "" {
		t.Fatal("could not brace-match the attestations handler")
	}
	// Stimulus: it really is the handler that builds the attestation views.
	if !strings.Contains(body, "attestationsResponse{") {
		t.Fatal("the function found does not write attestationsResponse — wrong subject")
	}
	if strings.Contains(body, "ReadAttestations(") {
		t.Error("the attestations route calls ReadAttestations, which verifies the whole " +
			"document again. document.sig is already computed beside the bytes, and the cost " +
			"of re-verifying is signature-count × document-SIZE on a request path.")
	}
	if !strings.Contains(body, "doc.sig") {
		t.Error("the attestations route does not read the cached signature status")
	}
	// **The proceeding lookup is UNCONDITIONAL, and that is the corrected state (P07.S05a).**
	//
	// This guard used to assert the lookup was GATED — on `ClaimsAProceeding`, so an ordinary
	// document paid no pdfcpu parse. The gate was wrong on its own terms: a convened but UNSIGNED
	// document has no signatures, so it never claimed a proceeding, and it is exactly C18's case.
	// Measured at tier 6, the route reported no counts at all for it.
	//
	// Both cheaper replacements were measured and refused — a byte scan for the attachment name is
	// a false negative on a real record (pdfcpu compresses the file-spec into an object stream),
	// and caching is unsound because fourteen sites assign `sig` and the record expires. The
	// reasoning is written out at the call site.
	//
	// **So what is guarded here is that the lookup happens at all**, and the hot-path property is
	// carried by the two assertions above: the route does not re-verify, and it reads the cached
	// status. That is the request-path work S04 removed, and it is an order of magnitude more than
	// the one read this adds.
	if !strings.Contains(body, "ProceedingOf(") {
		t.Error("the attestations route no longer resolves the document's proceeding, so a " +
			"ceremony document reports no obliged-signer count and C18 cannot render")
	}
	if strings.Contains(body, "ClaimsAProceeding(") {
		t.Error("the attestations route gates the proceeding lookup on a signature naming a " +
			"ceremony — which a convened but UNSIGNED document never does, so C18's own extreme " +
			"case (0 of N signed) reports nothing")
	}
}

// TestWhetherYouSignIsReadOffTheRoster — C07's structural half.
//
// A `signs:false` roster member moves the baton and adds nothing. What makes this safe is that it
// is not a *choice*: there is no carry route, no flag on the request, and no branch a caller can
// take by mistake — `/api/session/initiate` reads the roster and takes the carry path or the
// contribution path accordingly. So a non-signing convener cannot accidentally sign, and a signer
// cannot accidentally skip their turn.
func TestWhetherYouSignIsReadOffTheRoster(t *testing.T) {
	conv := strings.Repeat("c0", 32)
	signer := strings.Repeat("a1", 32)
	inv := ceremony.Invitation{Roster: []ceremony.Party{
		{Fingerprint: conv, Signs: false},
		{Fingerprint: signer, Signs: true},
	}}
	cer := &ceremonyID{inv: inv}
	if !cer.carries(conv) {
		t.Error("a signs:false roster member does not carry — they would contribute a signature " +
			"to a ceremony they were convened not to sign")
	}
	if cer.carries(signer) {
		t.Error("a SIGNING party carries — they would skip their own turn, and the chain would " +
			"never advance past them")
	}
	if cer.carries(strings.Repeat("ff", 32)) {
		t.Error("a party outside the roster carries")
	}
	if (*ceremonyID)(nil).carries(conv) {
		t.Error("the manual path carries — there is no roster there and nothing to carry for")
	}

	// **And the ROUTING**, because the predicate alone says nothing about whether the handler
	// asks it. Asserted with `//` stripped, and on the ORDER: `buildCoSigned` applies the local
	// signature, so a carry decided after it has already signed.
	src, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatal(err)
	}
	code := stripLineComments(string(src))
	body := funcBodyFrom(code, strings.Index(code, "func (s *Server) handleSessionInitiate("))
	if body == "" {
		t.Fatal("cannot find handleSessionInitiate")
	}
	decide := strings.Index(body, "cer.carries(")
	sign := strings.Index(body, "buildCoSigned(")
	carry := strings.Index(body, "p2p.Carry(")
	if decide < 0 || sign < 0 || carry < 0 {
		t.Fatalf("handleSessionInitiate: carries=%d buildCoSigned=%d Carry=%d — one of the three "+
			"is gone and the route can no longer both carry and contribute", decide, sign, carry)
	}
	if decide > sign {
		t.Error("the route decides whether it carries AFTER buildCoSigned, which applies the " +
			"local signature — so a carrier has already signed by the time anything asks " +
			"whether they should have, and a signature cannot be taken back off a document")
	}
}

// TestARelayReplacesTheBatonRatherThanAccumulating — the clause, driven at nine hops.
//
// Every hop of an N-party ceremony returns the SAME proceeding one signature further on. Through
// `addDoc` — D10's arrival path, and where this used to go — each one opened a document alongside
// the last, so a nine-party relay left the convener holding **nine copies of one ceremony against
// a count cap of eight**: the ninth hop would have been refused for a reason that has nothing to
// do with the ceremony.
//
// Nine rather than two, because two is also what a door that replaces *the active* document looks
// like, and because eight is where the count cap is: a fixture that stops at seven cannot tell a
// working replace from a lucky one.
func TestARelayReplacesTheBatonRatherThanAccumulating(t *testing.T) {
	s := &Server{epoch: "test-epoch"}
	const ceremony = "ceremony-abc"
	var last *document
	for hop := 1; hop <= 9; hop++ {
		// Each hop's bytes are longer, the way an appended signature makes them.
		data := bytes.Repeat([]byte{'x'}, 100*hop)
		got, err := s.installCeremonyResult(ceremony, "relay.pdf", data)
		if err != nil {
			t.Fatalf("hop %d: %v", hop, err)
		}
		s.mu.Lock()
		n := len(s.docs)
		s.mu.Unlock()
		if n != 1 {
			t.Fatalf("after hop %d the registry holds %d documents, want 1 — a relay that "+
				"accumulates hits the count cap of %d before a nine-party ceremony finishes",
				hop, n, maxOpenDocs)
		}
		if hop > 1 && got != last {
			t.Errorf("hop %d opened a NEW document rather than replacing the baton", hop)
		}
		if !bytes.Equal(got.data, data) {
			t.Errorf("hop %d: the document does not carry that hop's bytes", hop)
		}
		if got.id != s.activeID {
			t.Errorf("hop %d: the baton is not the active document, so the user is looking at "+
				"something else while the ceremony advances", hop)
		}
		last = got
	}

	// **The control, and it is what stops this becoming "replace everything".** A SECOND ceremony
	// gets its own document, and an ordinary document is untouched by either.
	other, err := s.installCeremonyResult("ceremony-def", "other.pdf", []byte("second"))
	if err != nil {
		t.Fatal(err)
	}
	if other == last {
		t.Fatal("a different ceremony replaced the first one's baton — one document would then " +
			"carry two proceedings")
	}
	s.mu.Lock()
	n := len(s.docs)
	s.mu.Unlock()
	if n != 2 {
		t.Errorf("two ceremonies hold %d documents, want 2", n)
	}
	// And a non-ceremony arrival still ADDS, because D10 is unchanged for it.
	if _, err := s.installCeremonyResult("", "arrival.pdf", []byte("plain")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.installCeremonyResult("", "arrival2.pdf", []byte("plain")); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	n = len(s.docs)
	s.mu.Unlock()
	if n != 4 {
		t.Errorf("two ordinary arrivals beside two ceremonies hold %d documents, want 4 — an "+
			"arrival with no ceremony must still add, which is D10 and is unchanged", n)
	}
}

// TestTheRelayDoorHonoursTheByteCap — ADR-008, at the fourth door.
//
// "The byte cap binds every door that grows a document", and a relay hop grows one. The door that
// used to carry these bytes — `addDoc` — applies no cap at all, so this is a tightening rather
// than a preserved property, and it is asserted rather than assumed.
func TestTheRelayDoorHonoursTheByteCap(t *testing.T) {
	s := &Server{epoch: "test-epoch", maxDocBytes: 1000}
	if _, err := s.installCeremonyResult("c1", "relay.pdf", bytes.Repeat([]byte{'x'}, 900)); err != nil {
		t.Fatalf("the first hop was refused under the budget: %v", err)
	}
	// A hop that would push past the ceiling is refused, and with ADR-008's error.
	_, err := s.installCeremonyResult("c1", "relay.pdf", bytes.Repeat([]byte{'x'}, 1200))
	if !errors.Is(err, ErrTooManyBytes) {
		t.Errorf("a hop past the byte ceiling returned %v, want ErrTooManyBytes — ADR-008 says "+
			"the cap binds every door that grows a document, and this is the door a relay grows "+
			"one through", err)
	}
	// **And the replace is measured on the TOTAL, not the delta**: a hop that makes the document
	// SMALLER can never be refused, however tight the budget.
	if _, err := s.installCeremonyResult("c1", "relay.pdf", []byte("small")); err != nil {
		t.Errorf("a hop that shrinks the document was refused (%v)", err)
	}
}

// TestTheInitiateRouteInstallsThroughTheRelayDoor — ADR-009, on the ROUTING.
//
// `installCeremonyResult` replacing correctly says nothing about whether the route uses it. Found
// by mutation: swapping the call back to `addDoc` left every behavioural test above green, because
// they drive the door directly and nothing drives the route's installation.
func TestTheInitiateRouteInstallsThroughTheRelayDoor(t *testing.T) {
	src, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatal(err)
	}
	code := stripLineComments(string(src))
	body := funcBodyFrom(code, strings.Index(code, "func (s *Server) handleSessionInitiate("))
	if body == "" {
		t.Fatal("cannot find handleSessionInitiate")
	}
	// Stimulus: this really is the handler that installs the result.
	if !strings.Contains(body, "docResponse(") {
		t.Fatal("the function found does not write a document response — wrong subject")
	}
	if !strings.Contains(body, "installCeremonyResult(") {
		t.Error("the initiate route does not install through the relay door, so each hop of an " +
			"N-party ceremony opens a new document — nine copies of one proceeding against a " +
			"count cap of eight, and no byte cap at all on the way in")
	}
	if strings.Contains(body, "s.addDoc(") {
		t.Error("the initiate route still calls addDoc, which is D10's UNCAPPED arrival path — " +
			"correct for a document that arrives out of the blue, wrong for the same proceeding " +
			"coming back one signature further on")
	}
}

// TestACompletedHopReachesTheMirror — C22, and the gap it closes is that `WriteMirror` had exactly
// ONE caller.
//
// Before this, the mirror recorded what a convener STARTED and never what anybody signed: the
// durable record of a ceremony stopped at the moment it began. C22 says every hop's output is
// written before the response returns.
//
// Driven through the door both sides call, with the three states it has to tell apart: a ceremony
// document is mirrored, an ordinary co-sign is not, and neither is a failure.
func TestACompletedHopReachesTheMirror(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	s := &Server{epoch: "test-epoch"}

	doc := convenedBytes(t)
	rec, err := ceremony.Extract(doc)
	if err != nil {
		t.Fatalf("setup: the fixture carries no record (%v)", err)
	}
	// Stimulus: nothing is there yet, so the file below is this call's doing.
	dir, err := ceremony.MirrorDir(home+"/nib", rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "document.pdf")); err == nil {
		t.Fatal("setup: the mirror already holds this ceremony's document")
	}

	s.mirrorHop(doc)
	if _, err := os.Stat(filepath.Join(dir, "document.pdf")); err != nil {
		t.Fatalf("a completed hop left no document in the mirror (%v) — this machine keeps no "+
			"durable copy of what it just signed", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "record.json")); err != nil {
		t.Errorf("the mirror holds a document and no record (%v)", err)
	}

	// **An ordinary co-sign is NOT mirrored**, and that is what stops this becoming "write every
	// arrival to ~/nib/ceremonies": there is no ceremony, so there is nothing to record.
	before := countFiles(t, filepath.Join(home, "nib", "ceremonies"))
	plain, perr := testpdf.Text("an ordinary page")
	if perr != nil {
		t.Fatal(perr)
	}
	s.mirrorHop(plain)
	if after := countFiles(t, filepath.Join(home, "nib", "ceremonies")); after != before {
		t.Errorf("a document with no ceremony record wrote %d file(s) into the ceremony mirror",
			after-before)
	}
}

func countFiles(t *testing.T, root string) int {
	t.Helper()
	n := 0
	_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			n++
		}
		return nil
	})
	return n
}

// TestBothSidesOfAHopMirrorIt — ADR-009 on the routing, and on C22's ORDER.
//
// `mirrorHop` writing correctly says nothing about whether either side calls it. And the order is
// part of the clause: mirroring AFTER the response would tell the user the hop completed and then
// write the record, so a crash in between leaves a user who was told their signature is safe and a
// machine with no copy of it.
func TestBothSidesOfAHopMirrorIt(t *testing.T) {
	src, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatal(err)
	}
	code := stripLineComments(string(src))
	for _, fn := range []string{"handleSessionInitiate", "openArrival"} {
		body := funcBodyFrom(code, strings.Index(code, "func (s *Server) "+fn+"("))
		if body == "" {
			t.Fatalf("cannot find %s", fn)
		}
		if !strings.Contains(body, "mirrorHop(") {
			t.Errorf("%s does not mirror the hop's output — C22 says EVERY hop's output is "+
				"written, and a rule holding at one of the two sides is the ADR-009 shape", fn)
		}
	}
	// The ORDER, on the side that has a response to return.
	body := funcBodyFrom(code, strings.Index(code, "func (s *Server) handleSessionInitiate("))
	mirror := strings.Index(body, "mirrorHop(")
	reply := strings.LastIndex(body, "writeJSON(")
	if mirror < 0 || reply < 0 {
		t.Fatal("handleSessionInitiate: cannot find both the mirror and the reply")
	}
	if mirror > reply {
		t.Error("the hop is mirrored AFTER the response returns, so a crash in between leaves a " +
			"user who was told their signature is safe and a machine with no copy of it")
	}
}
