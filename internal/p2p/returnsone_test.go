package p2p

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"nib/internal/sign"
	"nib/internal/testpdf"
)

// TestAReturnIsWhatWasSentPlusOneSignature — /pending 629, the door itself, one conjunct a row.
//
// Synthetic statuses, because three of these shapes need an update that redefines an earlier
// signature's object and no fixture here writes one: the door reads `Signers`, `CoverageEnd` and
// the coverage cause and nothing else, so a Status is the whole of its input.
func TestAReturnIsWhatWasSentPlusOneSignature(t *testing.T) {
	const a, b, c = "aa", "bb", "cc"
	signer := func(fp string, end int64) sign.SignerInfo {
		return sign.SignerInfo{Fingerprint: fp, CoverageEnd: end, Valid: true}
	}
	failed := func(fp string, end int64) sign.SignerInfo {
		return sign.SignerInfo{Fingerprint: fp, CoverageEnd: end}
	}
	st := func(s ...sign.SignerInfo) sign.Status { return sign.Status{State: sign.Valid, Signers: s} }
	sent := st(signer(a, 100))
	const sentLen = 100
	for _, tc := range []struct {
		name  string
		sent  sign.Status
		final sign.Status
		want  string // "" = accepted
	}{
		{"what was sent plus the peer's signature", sent, st(signer(a, 100), signer(b, 200)), ""},
		{"the new signature has the lower object number", sent, st(signer(b, 200), signer(a, 100)), ""},
		{"an unsigned document sent, one signature back", st(), st(signer(b, 200)), ""},
		{"nothing new", sent, st(signer(a, 100)), "no new signature"},
		{"two new signatures, the peer's last", sent, st(signer(a, 100), signer(c, 150), signer(b, 200)), "2 new signatures"},
		{"two new signatures, the peer's first", sent, st(signer(a, 100), signer(b, 150), signer(c, 200)), "2 new signatures"},
		{"one new signature, somebody else's", sent, st(signer(a, 100), signer(c, 200)), "was sent to"},
		{"one new signature with no fingerprint", sent, st(signer(a, 100), signer("", 200)), "was sent to"},
		{"the peer's new signature does not verify", sent, st(signer(a, 100), failed(b, 200)), "does not verify"},
		{"a sent signature is gone", sent, st(signer(b, 200)), "it was sent with"},
		{"a sent signature is somebody else's now", sent, st(signer(c, 100), signer(b, 200)), "it was sent with"},
		{"a sent signature reaches somewhere else", sent, st(signer(a, 90), signer(b, 200)), "it was sent with"},
		{"a sent signature no longer verifies", sent, st(failed(a, 100), signer(b, 200)), "it was sent with"},
		{"an extra signature inside the sent bytes", sent, st(signer(a, 100), signer(c, 80), signer(b, 200)), "it was sent with"},
		{"sent signatures in another order", st(signer(a, 60), signer(c, 100)), st(signer(c, 100), signer(a, 60), signer(b, 200)), "it was sent with"},
	} {
		err := returnedWithOneSignature(tc.sent, tc.final, sentLen, b)
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%s: refused: %v", tc.name, err)
		case tc.want != "" && err == nil:
			t.Errorf("%s: accepted", tc.name)
		case tc.want != "" && !strings.Contains(err.Error(), tc.want):
			t.Errorf("%s: refused with %q, want a refusal naming %q", tc.name, err, tc.want)
		}
	}

	// Each coverage cause is refused, in words of its own, before a signer is counted.
	honest := st(signer(a, 100), signer(b, 200))
	said := map[string]sign.AddedAfterCause{}
	for _, cause := range []sign.AddedAfterCause{sign.AddedAfterAppended, sign.AddedAfterRefusedSignature, sign.AddedAfterCouldNotCheck} {
		final := honest
		final.AddedAfter, final.AddedAfterCause = true, cause
		err := returnedWithOneSignature(sent, final, sentLen, b)
		if err == nil {
			t.Errorf("a return whose coverage cause is %q was accepted", cause)
			continue
		}
		if other, dup := said[err.Error()]; dup {
			t.Errorf("causes %q and %q are refused in the same words: %q", other, cause, err)
		}
		said[err.Error()] = cause
	}
}

// TestAThirdSignatureOnATwoPartyReturnIsRefused — /pending 629's second parked half, on real
// documents. `AddedAfter` is bounded by ANY valid signer, so what was sent, the peer's signature,
// then a third key's valid signature over all of it is a document with nothing after its last
// signature — and both parties' signatures on it verify. Only the count can refuse it.
func TestAThirdSignatureOnATwoPartyReturnIsRefused(t *testing.T) {
	aCert, aKey := newIdentity(t) // initiator
	bCert, bKey := newIdentity(t) // peer
	cCert, cKey := newIdentity(t) // nobody this exchange is with
	aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)
	bAcceptsA := Attestation{Signer: "Bob", AcceptedPeer: hexOf(aFP), AcceptedPeerLabel: "Alice", Intent: "I accept", When: time.Now()}
	cAtt := Attestation{Signer: "Carol", AcceptedPeer: hexOf(aFP), Intent: "I accept", When: time.Now()}
	mine := signAsInitiator(t, aCert, aKey, bFP)
	mutual := contribute(t, mine, bCert, bKey, bAcceptsA)
	if err := confirmCoSigned(mine, mutual, bFP, aFP, false); err != nil {
		t.Fatalf("setup: the honest return was refused: %v", err)
	}

	for _, tc := range []struct {
		name  string
		final []byte
	}{
		{"the peer, then a third key", contribute(t, mutual, cCert, cKey, cAtt)},
		{"a third key, then the peer", contribute(t, contribute(t, mine, cCert, cKey, cAtt), bCert, bKey, bAcceptsA)},
	} {
		// STIMULUS: three valid signatures and nothing after the last, so every check that
		// existed before the count passes.
		st := sign.Verify(tc.final)
		if st.State != sign.Valid || len(st.Signers) != 3 || st.AddedAfter {
			t.Fatalf("%s: setup: reads state %v, %d signer(s), addedAfter=%v", tc.name, st.State, len(st.Signers), st.AddedAfter)
		}
		err := confirmCoSigned(mine, tc.final, bFP, aFP, false)
		if err == nil {
			t.Fatalf("%s: a return carrying a signature by a party this exchange was not with was accepted", tc.name)
		}
		if !strings.Contains(err.Error(), "2 new signatures") {
			t.Errorf("%s: refused with %q, which does not say why", tc.name, err)
		}
	}

	// And a return that is the sent document, unchanged, is not co-signed by anyone new. Asked
	// with what was sent set to the mutual document, so both parties' signatures are on it and
	// nothing but the count refuses.
	if err := confirmCoSigned(mutual, mutual, bFP, aFP, false); err == nil || !strings.Contains(err.Error(), "no new signature") {
		t.Errorf("a return with no new signature answered %v", err)
	}
}

// relayHop runs one hop over a real transport: `carrier` dials `signer`, who runs the honest
// `Receive`, and the carrier collects through `verb` (`Carry` or `Initiate`).
func relayHop(t *testing.T, carrier, signer l3Party, doc []byte, roster Roster,
	verb func(Channel, []byte, []byte, Verifier, Roster) ([]byte, error)) ([]byte, error) {
	t.Helper()
	tr := transports[0]
	ln, err := tr.listen("127.0.0.1:0", signer.cert, signer.key, mustFP(t, carrier.cert))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	recvErr := make(chan error, 1)
	go func() {
		conn, e := ln.Accept()
		if e != nil {
			recvErr <- e
			return
		}
		defer conn.Close()
		_, e = Receive(conn.Channel, signer.cert, signer.key, "Carrier",
			confirmer{accept: true, intent: "I accept"}, okVerifier{}, nil, roster)
		recvErr <- e
	}()
	conn, err := tr.dial(context.Background(), ln.Addr().String(), carrier.cert, carrier.key,
		mustFP(t, signer.cert), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	out, verr := verb(conn.Channel, doc, mustFP(t, carrier.cert), okVerifier{}, roster)
	conn.Close()
	if e := <-recvErr; e != nil && verr == nil {
		t.Fatalf("the signer refused the hop: %v", e)
	}
	return out, verr
}

// TestEveryHonestRelayShapePassesTheReturnDoor — the control for /pending 629's rule: nothing
// honest is refused. Each shape is a ceremony the multi-process tiers drive (`pairrepro.sh`'s
// two-party co-sign and its relay through `Carry`, `ceremonyrepro.sh`), walked here in one process
// over a real transport with the honest `Receive` on the far side of every hop.
//
// A step is either a hop to a signer or the convener signing their own turn locally (ADR-135),
// which is no hop at all — the document the next carry hands over then already holds it.
func TestEveryHonestRelayShapePassesTheReturnDoor(t *testing.T) {
	conv := l3Identity(t, "Convener")
	a, b, c := l3Identity(t, "A"), l3Identity(t, "B"), l3Identity(t, "C")
	signing := func(ps ...l3Party) Roster { return l3Roster(ps...) }
	withCarrier := func(ps ...l3Party) Roster {
		r := Roster{Entries: []RosterEntry{{Fingerprint: conv.fp, Signs: false}}}
		r.Entries = append(r.Entries, l3Roster(ps...).Entries...)
		return r
	}
	const (
		carry    = "carry"
		initiate = "initiate"
		local    = "local"
	)
	type step struct {
		how string
		to  l3Party
	}
	for _, tc := range []struct {
		name   string
		roster Roster
		steps  []step
	}{
		{"two parties, the convener signs first and initiates", signing(conv, a),
			[]step{{local, conv}, {initiate, a}}},
		{"two parties, the convener signs first and carries", signing(conv, a),
			[]step{{local, conv}, {carry, a}}},
		{"three parties, a non-signing convener carries", withCarrier(a, b),
			[]step{{carry, a}, {carry, b}}},
		{"four parties, a non-signing convener carries", withCarrier(a, b, c),
			[]step{{carry, a}, {carry, b}, {carry, c}}},
		{"three parties, the convener signs first", signing(conv, a, b),
			[]step{{local, conv}, {carry, a}, {carry, b}}},
		{"three parties, the convener signs first and initiates the first hop", signing(conv, a, b),
			[]step{{local, conv}, {initiate, a}, {carry, b}}},
		{"three parties, the convener signs in the middle", signing(a, conv, b),
			[]step{{carry, a}, {local, conv}, {carry, b}}},
		{"four parties, the convener signs last", signing(a, b, c, conv),
			[]step{{carry, a}, {carry, b}, {carry, c}, {local, conv}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := l3Prepared(t)
			for i, s := range tc.steps {
				switch s.how {
				case local:
					pred := conv
					if p := PredecessorOf(tc.roster, conv.fp); p != "" {
						for _, q := range []l3Party{a, b, c} {
							if SameParty(q.fp, p) {
								pred = q
							}
						}
					}
					doc = l3Chain(t, doc, []l3Party{conv}, []l3Party{pred}, "")
				case carry, initiate:
					verb := Carry
					if s.how == initiate {
						verb = Initiate
					}
					out, err := relayHop(t, conv, s.to, doc, tc.roster, verb)
					if err != nil {
						t.Fatalf("step %d (%s): an honest hop was refused: %v", i+1, s.how, err)
					}
					doc = out
				}
			}
			pr, err := ContributionProgress(doc, tc.roster)
			if err != nil || !pr.Complete {
				t.Fatalf("the ceremony did not complete: done %d of %d, %v", pr.Done, len(pr.Order), err)
			}
		})
	}
}

// TestCarryHandsTheDocumentOnlyToTheNextSigner — `Carry` never compared the party on the wire with
// the party whose turn it is. The refusal comes before a document byte is sent.
func TestCarryHandsTheDocumentOnlyToTheNextSigner(t *testing.T) {
	conv := l3Identity(t, "Convener")
	a, c := l3Identity(t, "A"), l3Identity(t, "C")
	roster := Roster{Entries: []RosterEntry{
		{Fingerprint: conv.fp, Signs: false},
		{Fingerprint: a.fp, Signs: true},
		{Fingerprint: c.fp, Signs: true},
	}}
	carried := l3Prepared(t)
	// The reply is what an honest A would return, sent by C: a pinned ceremony member whose turn
	// it is not. STIMULUS: handed to `Carry` over a channel to A, the same reply is accepted
	// (`TestCarryRefusesAHostileHop`'s control), so only who is on the wire differs.
	reply := l3Chain(t, carried, []l3Party{a}, []l3Party{a}, "")

	tr := transports[0]
	ln, err := tr.listen("127.0.0.1:0", c.cert, c.key, mustFP(t, conv.cert))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	recv := make(chan error, 1)
	hostileReceiver(t, ln, c.cert, c.key, reply, recv)
	conn, err := tr.dial(context.Background(), ln.Addr().String(), conv.cert, conv.key, mustFP(t, c.cert), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	sent := &countingStream{Stream: conn.Channel.Stream}
	ch := conn.Channel
	ch.Stream = sent
	_, cerr := Carry(ch, carried, mustFP(t, conv.cert), okVerifier{}, roster)
	conn.Close()
	<-recv
	if cerr == nil {
		t.Fatal("a carry to a party whose turn it is not was accepted")
	}
	if !errors.Is(cerr, ErrNotYourTurn) || !strings.Contains(cerr.Error(), "this hop is for") {
		t.Errorf("refused with %q, want the turn refusal naming who the hop is for", cerr)
	}
	if sent.wrote != 0 {
		t.Errorf("%d byte(s) went to the wrong party before the refusal", sent.wrote)
	}
}

// countingStream counts what is written to the wire.
type countingStream struct {
	Stream
	wrote int
}

func (c *countingStream) Write(p []byte) (int, error) {
	c.wrote += len(p)
	return c.Stream.Write(p)
}

// TestCarryRefusesAReturnThatIsNotOneSignature — the two shapes every check `Carry` had passed.
func TestCarryRefusesAReturnThatIsNotOneSignature(t *testing.T) {
	conv := l3Identity(t, "Convener")
	a, c := l3Identity(t, "A"), l3Identity(t, "C")
	roster := Roster{Entries: []RosterEntry{
		{Fingerprint: conv.fp, Signs: false},
		{Fingerprint: a.fp, Signs: true},
		{Fingerprint: c.fp, Signs: true},
	}}
	carried := l3Prepared(t)
	honest := l3Chain(t, carried, []l3Party{a}, []l3Party{a}, "")
	// A and C both sign in answer to one hop: the roster's order, each valid and cross-bound, the
	// chain advanced — by two.
	both := l3Chain(t, carried, []l3Party{a, c}, []l3Party{a, a}, "")
	if pr, err := ContributionProgress(both, roster); err != nil || !pr.Complete {
		t.Fatalf("setup: the two-signature reply is not the roster's own chain: %+v %v", pr, err)
	}
	copied, _, err := testpdf.CopiedSignatureDictionary(honest, testpdf.CopyExact)
	if err != nil {
		t.Fatal(err)
	}
	if cs := sign.Verify(copied); cs.AddedAfterCause != sign.AddedAfterRefusedSignature || len(cs.Signers) != 1 {
		t.Fatalf("setup: the copy reads cause %q with %d signer(s)", cs.AddedAfterCause, len(cs.Signers))
	}
	for _, tc := range []struct {
		name  string
		reply []byte
		want  string
	}{
		{"the next two signers answer one hop", both, "2 new signatures"},
		{"a refused signature copy after the party's signature", copied, "a signature Nib refused"},
		{"the honest reply", honest, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := transports[0]
			ln, err := tr.listen("127.0.0.1:0", a.cert, a.key, mustFP(t, conv.cert))
			if err != nil {
				t.Fatal(err)
			}
			defer ln.Close()
			recv := make(chan error, 1)
			hostileReceiver(t, ln, a.cert, a.key, tc.reply, recv)
			conn, err := tr.dial(context.Background(), ln.Addr().String(), conv.cert, conv.key, mustFP(t, a.cert), 10*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_, cerr := Carry(conn.Channel, carried, mustFP(t, conv.cert), okVerifier{}, roster)
			<-recv
			switch {
			case tc.want == "" && cerr != nil:
				t.Fatalf("the honest reply was refused: %v", cerr)
			case tc.want != "" && cerr == nil:
				t.Fatalf("a return carrying %s was accepted", tc.name)
			case tc.want != "" && !strings.Contains(cerr.Error(), tc.want):
				t.Errorf("refused with %q, want a refusal naming %q", cerr, tc.want)
			}
		})
	}
}
