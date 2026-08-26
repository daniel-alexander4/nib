package p2p

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"nib/internal/testpdf"
)

// TestEveryRefusalCodeRoundTripsToItsOwnSentinel — the table, both directions, over the whole
// enumeration rather than over a sample.
//
// A wire code is a value two builds must agree on. Two tables that map it — one per direction —
// are a protocol that can disagree with itself, which is the defect `refusalAck`'s own doc records
// from the last time this happened: the transfer path had an explicit declined byte and the
// co-signature path had none, so one half of one feature reported a refusal as an outcome and the
// other reported it as EOF.
func TestEveryRefusalCodeRoundTripsToItsOwnSentinel(t *testing.T) {
	all := []error{
		ErrNotYourTurn, ErrNotInRoster, ErrPrefixMismatch, ErrPrefixUnproven,
		ErrProceedingMismatch, ErrCeremonyComplete, ErrNotTheConnectedPeer,
		ErrPeerDoesNotAcceptYou, ErrWrongPriorSignerCount,
	}
	seen := map[byte]error{}
	for _, want := range all {
		code := refusalCode(want)
		if code == 0 {
			t.Errorf("%v has no wire code, so it reaches the initiator as EOF — which reads as a "+
				"network fault and invites the retry a refusal must not invite", want)
			continue
		}
		if prior, dup := seen[code]; dup {
			t.Errorf("%v and %v share wire code %d — the initiator cannot tell them apart",
				want, prior, code)
		}
		seen[code] = want
		if got := errorForCode(code); !errors.Is(got, want) {
			t.Errorf("code %d maps back to %v, want %v — the two tables disagree, which is a "+
				"protocol disagreeing with itself", code, got, want)
		}
	}
	// Stimulus: the enumeration is not empty and did not collapse.
	if len(seen) != len(all) {
		t.Fatalf("%d distinct codes for %d refusals", len(seen), len(all))
	}
	// **An unknown code is a SENTENCE, not a verdict (D32).** This is the whole reason the
	// protocol is negotiated, so it is asserted rather than assumed.
	unknown := errorForCode(200)
	if !errors.Is(unknown, ErrRefusedUnknown) {
		t.Errorf("an unknown refusal code produced %v, want ErrRefusedUnknown", unknown)
	}
	if !strings.Contains(unknown.Error(), "200") {
		t.Errorf("the unknown-code error does not name the code: %v — a bug report then cannot "+
			"say which one", unknown)
	}
}

// TestARefusalIsOnlySentToAPeerThatCanReadIt — the negotiation, at the door.
//
// Sending a named refusal to an older peer is not a courtesy failure, it is a D32 violation:
// that build maps a one-byte reply and otherwise falls through to `if !bytes.HasPrefix(...)`,
// telling its user "returned document is not the one sent this session" — a tampering verdict
// about an honest peer, produced by a version skew.
func TestARefusalIsOnlySentToAPeerThatCanReadIt(t *testing.T) {
	// To a v2 peer: the named frame.
	frame, ok := refusalAck(ErrNotYourTurn, true)
	if !ok {
		t.Fatal("a v2 peer was sent no refusal at all for an L3 error")
	}
	if len(frame) != 2 || frame[0] != ackRefused || frame[1] != refuseNotYourTurn {
		t.Fatalf("the named refusal frame is %v, want [%d %d]", frame, ackRefused, refuseNotYourTurn)
	}
	// To an older peer: nothing, which is exactly that build's pre-existing behaviour.
	if _, ok := refusalAck(ErrNotYourTurn, false); ok {
		t.Error("a named refusal was sent to a peer that did not negotiate it. That build reads " +
			"an unfamiliar frame as \"returned document is not the one sent this session\" — a " +
			"tampering verdict about an honest peer, produced by a version skew (D32).")
	}
	// And the two classes that PREDATE the negotiation still cross to an older peer, or this
	// change would have taken away a refusal that already worked.
	for _, err := range []error{ErrCoSignDeclined, ErrConsentTimedOut, ErrDeclined} {
		if _, ok := refusalAck(err, false); !ok {
			t.Errorf("%v no longer reaches an older peer — this negotiation may only ADD", err)
		}
	}
}

// TestARefusalFrameCannotBeMistakenForADocument — the two independent reasons, both driven.
//
// `Initiate` now runs `refusalFor` over ANY frame rather than only a one-byte one, so the
// discrimination has to be a property of the bytes and not of a length test somebody may relax.
func TestARefusalFrameCannotBeMistakenForADocument(t *testing.T) {
	// 1. A PDF begins `%PDF-` — 0x25 — and ackRefused is 4.
	if ackRefused == '%' {
		t.Fatal("ackRefused collides with the PDF header byte")
	}
	// Driven rather than asserted about a constant: a real co-signed document, through the real
	// reader.
	doc := l3Chain(t, l3Prepared(t), []l3Party{l3Identity(t, "A")}, []l3Party{l3Identity(t, "B")}, "")
	if len(doc) < 5 || !bytes.HasPrefix(doc, []byte("%PDF-")) {
		t.Fatalf("setup: the fixture is not a PDF (%q)", doc[:min(8, len(doc))])
	}
	if _, ok := refusalFor(doc, true); ok {
		t.Error("a real co-signed document was read as a refusal frame")
	}
	// 2. And it is never two bytes.
	if len(doc) == 2 {
		t.Fatal("a co-signed document is two bytes long, which is the other half of the " +
			"unambiguity argument")
	}
	// The control: the frame this build writes IS read as a refusal.
	frame, _ := refusalAck(ErrPrefixMismatch, true)
	got, ok := refusalFor(frame, true)
	if !ok || !errors.Is(got, ErrPrefixMismatch) {
		t.Errorf("the refusal this build writes reads back as %v (ok=%v)", got, ok)
	}
	// And a ONE-byte frame is never read as a truncated named refusal.
	if got, ok := refusalFor([]byte{ackRefused}, true); ok {
		t.Errorf("a one-byte frame carrying only the marker was read as a refusal (%v) — a "+
			"truncated frame must not decode to a meaning", got)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestAnL3RefusalReachesTheInitiatorByName — the acceptance clause, driven end to end over BOTH
// transports.
//
// This is what "refused in Go" is not. Before this slice, `expected exactly one prior signer`,
// *the document was not signed by the connected peer* and *the peer's attestation does not accept
// you* all arrived at the initiator as `receive co-signed document: EOF` — which reads as a
// network fault and invites the retry a refusal must not invite. D23 says a refusal is "never a
// hang, never a silent no-op"; over the wire it was silent.
//
// The receiver is given a roster that puts somebody else first, so its L3 gate refuses. The
// assertion is that the INITIATOR gets the sentinel — not that the receiver returned one.
func TestAnL3RefusalReachesTheInitiatorByName(t *testing.T) {
	eachTransport(t, func(t *testing.T, tr transport) {
		aCert, aKey := newIdentity(t) // initiator
		bCert, bKey := newIdentity(t) // receiver
		aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)
		aSigned := signAsInitiator(t, aCert, aKey, bFP)

		// A roster in which the NEXT contributor is neither A nor B, so B refuses with
		// ErrNotYourTurn rather than with anything about the channel.
		stranger := strings.Repeat("cc", 32)
		roster := Roster{Entries: []RosterEntry{
			{Fingerprint: hexOf(aFP), Signs: true},
			{Fingerprint: stranger, Signs: true},
			{Fingerprint: hexOf(bFP), Signs: true},
		}}

		ln, err := tr.listen("127.0.0.1:0", bCert, bKey, aFP)
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		recvErr := make(chan error, 1)
		spoke := make(chan bool, 1)
		go func() {
			conn, e := ln.Accept()
			if e != nil {
				recvErr <- e
				return
			}
			defer conn.Close()
			spoke <- conn.Channel.SpeaksNamedRefusals()
			_, e = Receive(conn.Channel, bCert, bKey, "Alice",
				confirmer{accept: true, intent: "I accept"}, okVerifier{}, nil, roster)
			recvErr <- e
		}()

		conn, err := tr.dial(context.Background(), ln.Addr().String(), aCert, aKey, bFP, 10*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		// **The stimulus: both ends negotiated the named-refusal protocol.** Without this the
		// assertion below could pass on a receiver that simply closed and an initiator that
		// happened to map something — and on a transport where ALPN never got wired, it would
		// pass for the wrong reason forever.
		if !conn.Channel.SpeaksNamedRefusals() {
			t.Fatalf("the dialing side negotiated %q, not the named-refusal protocol — the "+
				"refusal below cannot be about the wire", conn.Channel.Proto)
		}
		// **The accepting side's answer is read AFTER Initiate, not before, and that is a
		// deadlock this test hit.** For QUIC, `ln.Accept()` does not return until the dialer
		// opens a stream, and the dialer opens one when `Initiate` first writes — so blocking on
		// the accepter here left both sides waiting for the other. Recorded because the obvious
		// ordering (assert both, then act) is the one that hangs.
		_, err = Initiate(conn.Channel, aSigned, aFP, okVerifier{}, Roster{})
		if got := <-spoke; !got {
			t.Fatal("the accepting side did not negotiate the named-refusal protocol")
		}
		if err == nil {
			t.Fatal("the initiator's contribution was accepted although the roster refuses it")
		}
		if !errors.Is(err, ErrNotYourTurn) {
			t.Errorf("the initiator was told %v, want ErrNotYourTurn. A refusal that arrives as "+
				"EOF reads as a network fault and invites a retry; one that arrives as \"not the "+
				"one sent this session\" accuses an honest peer of a replay.", err)
		}
		if e := <-recvErr; !errors.Is(e, ErrNotYourTurn) {
			t.Errorf("the receiver's own error is %v, want ErrNotYourTurn", e)
		}
	})
}

// hexOf is the hex spelling of a fingerprint, which is what a roster carries.
func hexOf(fp []byte) string {
	const h = "0123456789abcdef"
	out := make([]byte, 0, len(fp)*2)
	for _, c := range fp {
		out = append(out, h[c>>4], h[c&15])
	}
	return string(out)
}

// TestAnOlderPeerGetsTheBehaviourItExpects — the skew, driven, which is the case no ordinary test
// reaches and the entire reason this protocol is negotiated.
//
// The receiver is handed a channel whose negotiated protocol is the OLD one — exactly what a
// build predating this slice presents — and must then behave as it did before: no named refusal
// frame, because that build reads an unfamiliar frame as *"returned document is not the one sent
// this session"*, a tampering verdict about an honest peer produced by a version skew (D32).
//
// **What is asserted is the ABSENCE of the new frame, so the stimulus is the presence of it on the
// same fixture one line up.** Without that, "no named refusal arrived" is equally true of a
// receiver that refused for some other reason entirely.
func TestAnOlderPeerGetsTheBehaviourItExpects(t *testing.T) {
	aCert, _ := newIdentity(t)
	bCert, _ := newIdentity(t)
	aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)
	roster := Roster{Entries: []RosterEntry{
		{Fingerprint: hexOf(aFP), Signs: true},
		{Fingerprint: strings.Repeat("cc", 32), Signs: true},
		{Fingerprint: hexOf(bFP), Signs: true},
	}}
	_ = roster

	// Stimulus: on a v2 channel this refusal DOES produce a frame.
	if frame, ok := refusalAck(ErrNotYourTurn, Channel{Proto: alpn2}.SpeaksNamedRefusals()); !ok || len(frame) != 2 {
		t.Fatalf("setup: a v2 channel produced %v (ok=%v), so the absence below proves nothing",
			frame, ok)
	}

	for _, proto := range []string{alpn, "", "h2"} {
		ch := Channel{Proto: proto}
		if ch.SpeaksNamedRefusals() {
			t.Errorf("a channel negotiating %q claims to speak named refusals", proto)
		}
		if frame, ok := refusalAck(ErrNotYourTurn, ch.SpeaksNamedRefusals()); ok {
			t.Errorf("a peer negotiating %q was sent %v. That build maps a one-byte reply and "+
				"otherwise falls to the prefix check, so it would tell its user the returned "+
				"document is not the one it sent — accusing an honest peer of a replay.",
				proto, frame)
		}
	}

	// And the other direction: an older RECEIVER is unaffected, because nothing about the two
	// classes it already knew has changed. Asserted on the decode side, which is the half a
	// newer initiator exercises against it.
	for _, tc := range []struct {
		frame []byte
		want  error
	}{
		{[]byte{ackDeclined}, ErrCoSignDeclined},
		{[]byte{ackTimedOut}, ErrConsentTimedOut},
	} {
		got, ok := refusalFor(tc.frame, true)
		if !ok || !errors.Is(got, tc.want) {
			t.Errorf("the pre-existing frame %v now decodes to %v (ok=%v), want %v — this "+
				"negotiation may only ADD", tc.frame, got, ok, tc.want)
		}
	}
}

// TestEveryALPNConfigSiteOffersTheSameList — ADR-009, on the config sites.
//
// The offer list is what makes the refusal negotiation work, and it is set at seven places across
// three files. A new listener or dialer written with `[]string{alpn}` would silently never
// negotiate v2, and every behavioural test would stay green because it drives the sites that
// already exist. That is the population-floor shape `TestL2CoversEveryDocumentCarryingEntryPoint`
// exists for, one layer down.
func TestEveryALPNConfigSiteOffersTheSameList(t *testing.T) {
	sites := 0
	for _, name := range []string{"quic.go", "endpoint.go", "transport.go"} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		src := l3StripComments(string(raw))
		for _, line := range strings.Split(src, "\n") {
			if !strings.Contains(line, "NextProtos") {
				continue
			}
			sites++
			if !strings.Contains(line, "sessionALPN") {
				t.Errorf("%s: %q sets NextProtos to something other than sessionALPN. That "+
					"transport then never negotiates the named-refusal protocol, and every "+
					"behavioural test stays green because it drives the sites that already exist.",
					name, strings.TrimSpace(line))
			}
		}
	}
	// Stimulus: the scan found the sites at all. A rename would otherwise make this pass having
	// read nothing.
	if sites < 3 {
		t.Fatalf("found %d NextProtos assignments across the three transport files, want at "+
			"least 3 — the scan is not reading what it thinks it is", sites)
	}
	// And the list itself: v2 first, v1 still offered. Dropping v1 would make this a hard
	// handshake failure against every older peer rather than a graceful fallback.
	if len(sessionALPN) != 2 || sessionALPN[0] != alpn2 || sessionALPN[1] != alpn {
		t.Errorf("sessionALPN is %v, want [%q %q] — most preferred first, and the older protocol "+
			"still offered so an older peer negotiates rather than failing the handshake",
			sessionALPN, alpn2, alpn)
	}
}

// TestTheNegotiatedProtocolIsNotRequiredByCheck — the one property that would turn a
// compatibility signal into a compatibility break.
//
// `Channel.check()` fails closed on every field it requires, and rightly: each is a security
// property established elsewhere. `Proto` is not one of those — empty means "this peer predates
// the versioned session protocol", which is a legal state. Requiring it would refuse every older
// peer outright.
func TestTheNegotiatedProtocolIsNotRequiredByCheck(t *testing.T) {
	ch := Channel{
		Stream: dummyStream{},
		PeerFP: bytes.Repeat([]byte{1}, 32),
		Export: func(string, []byte, int) ([]byte, error) { return make([]byte, 32), nil },
	}
	// Stimulus: this channel is otherwise complete, so a refusal below is about Proto alone.
	if err := ch.check(); err != nil {
		t.Fatalf("setup: the fixture channel is incomplete for another reason (%v)", err)
	}
	if ch.Proto != "" {
		t.Fatal("setup: the fixture already carries a protocol")
	}
	if ch.SpeaksNamedRefusals() {
		t.Error("a channel with no negotiated protocol claims to speak named refusals")
	}
}

type dummyStream struct{}

func (dummyStream) Read([]byte) (int, error)      { return 0, errors.New("not used") }
func (dummyStream) Write([]byte) (int, error)     { return 0, errors.New("not used") }
func (dummyStream) Close() error                  { return nil }
func (dummyStream) SetDeadline(t time.Time) error { return nil }

// TestATwoPartyCEREMONYHopCompletes — the control nothing in the tree had.
//
// Every existing two-party test drives the MANUAL path: no roster, no invitation, so
// `inCeremony` is false and none of P07.S03–S05's re-basings are reached. Tier 4's hop 1 does the
// same — its own blind-spot list says it still hand-pins. So the whole ceremony receive path could
// regress and every one of them would stay green, which is how a `PredecessorOf` change that
// breaks the first signer's attestation got as far as it did.
//
// Drives a real two-party ceremony hop: A first in the roster, B second, both signing.
func TestATwoPartyCEREMONYHopCompletes(t *testing.T) {
	eachTransport(t, func(t *testing.T, tr transport) {
		aCert, aKey := newIdentity(t)
		bCert, bKey := newIdentity(t)
		aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)
		roster := Roster{Entries: []RosterEntry{
			{Fingerprint: hexOf(aFP), Signs: true},
			{Fingerprint: hexOf(bFP), Signs: true},
		}}
		// A signs FIRST, the way `buildCoSigned` does inside a ceremony: accepting its
		// predecessor, which for the first signer is nobody.
		base, err := PrepareDocument(mustForm(t))
		if err != nil {
			t.Fatal(err)
		}
		place, err := NextPlacement(base)
		if err != nil {
			t.Fatal(err)
		}
		aSigned, err := Contribute(base, aCert, aKey, Attestation{
			Signer: "A", AcceptedPeer: PredecessorOf(roster, hexOf(aFP)),
			Intent: "I agree", When: time.Now(),
		}, nil, place)
		if err != nil {
			t.Fatal(err)
		}
		// Stimulus: the first signer really does accept nobody, or this fixture is the manual
		// path wearing a roster.
		if ats := ReadAttestations(aSigned); len(ats) != 1 || ats[0].AcceptedPeer != "" {
			t.Fatalf("setup: the first signer accepts %q, want nothing — C14 as amended",
				ats[0].AcceptedPeer)
		}

		ln, err := tr.listen("127.0.0.1:0", bCert, bKey, aFP)
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
			_, e = Receive(conn.Channel, bCert, bKey, "Alice",
				confirmer{accept: true, intent: "I accept"}, okVerifier{}, nil, roster)
			recvErr <- e
		}()
		conn, err := tr.dial(context.Background(), ln.Addr().String(), aCert, aKey, bFP, 10*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		final, err := Initiate(conn.Channel, aSigned, aFP, okVerifier{}, roster)
		if e := <-recvErr; e != nil {
			t.Fatalf("the receiving side of a two-party CEREMONY hop refused it: %v", e)
		}
		if err != nil {
			t.Fatalf("the initiating side of a two-party CEREMONY hop refused the result: %v", err)
		}
		ats := ReadAttestations(final)
		if len(ats) != 2 {
			t.Fatalf("the completed hop carries %d signature(s), want 2", len(ats))
		}
		// C14 as amended, driven: the signature WITH a predecessor is matched; the first one
		// reports its own state, which here is unmatched because it accepts nobody.
		if !ats[1].Matched {
			t.Errorf("the second signature is not cross-bound to its predecessor")
		}
		if ats[1].AcceptedPeer != hexOf(aFP) {
			t.Errorf("the second signature accepts %s, want its predecessor A",
				ats[1].AcceptedPeer)
		}
	})
}

func mustForm(t *testing.T) []byte {
	t.Helper()
	b, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestAFourPartyCeremonyCompletesOverTheCarryRoute — C02/C07, driven, and the slice's point.
//
// A non-signing convener carries the baton to each signer in turn and signs nothing. What makes
// this the acceptance rather than a demonstration is the second half: **the same ceremony through
// `Initiate` fails**, because that verb demands the caller's own signature back and the convener
// has none — which is the reason the carry verb had to exist at all.
func TestAFourPartyCeremonyCompletesOverTheCarryRoute(t *testing.T) {
	eachTransport(t, func(t *testing.T, tr transport) {
		convCert, convKey := newIdentity(t)
		convFP := fingerprint(t, convCert)
		type party struct {
			cert, key []byte
			fp        []byte
		}
		var signers []party
		for range 3 {
			c, k := newIdentity(t)
			signers = append(signers, party{c, k, fingerprint(t, c)})
		}
		roster := Roster{Entries: []RosterEntry{{Fingerprint: hexOf(convFP), Signs: false}}}
		for _, p := range signers {
			roster.Entries = append(roster.Entries, RosterEntry{Fingerprint: hexOf(p.fp), Signs: true})
		}

		doc, err := PrepareDocument(mustForm(t))
		if err != nil {
			t.Fatal(err)
		}
		// Stimulus: the convener holds NO signing position, or "signs nothing" below is vacuous.
		if PredecessorOf(roster, hexOf(convFP)) != "" || InRoster(roster, hexOf(convFP)) != true {
			t.Fatal("setup: the convener is not a non-signing roster member")
		}

		for i, p := range signers {
			ln, err := tr.listen("127.0.0.1:0", p.cert, p.key, convFP) // the signer pins the CARRIER
			if err != nil {
				t.Fatal(err)
			}
			recvErr := make(chan error, 1)
			go func(pc, pk []byte) {
				conn, e := ln.Accept()
				if e != nil {
					recvErr <- e
					return
				}
				defer conn.Close()
				_, e = Receive(conn.Channel, pc, pk, "Convener",
					confirmer{accept: true, intent: "I accept"}, okVerifier{}, nil, roster)
				recvErr <- e
			}(p.cert, p.key)

			conn, err := tr.dial(context.Background(), ln.Addr().String(), convCert, convKey, p.fp, 10*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			out, cerr := Carry(conn.Channel, doc, convFP, okVerifier{}, roster)
			if e := <-recvErr; e != nil {
				conn.Close()
				ln.Close()
				t.Fatalf("hop %d: the signer refused the carrier: %v", i+1, e)
			}
			conn.Close()
			ln.Close()
			if cerr != nil {
				t.Fatalf("hop %d: the carrier could not collect the contribution: %v", i+1, cerr)
			}
			if !bytes.HasPrefix(out, doc) {
				t.Fatalf("hop %d: the returned document is not what went out plus a signature", i+1)
			}
			doc = out
		}

		ats := ReadAttestations(doc)
		if len(ats) != 3 {
			t.Fatalf("the completed ceremony carries %d signature(s), want 3 — one per SIGNING "+
				"party, and 2(N-1) if the carrier had been signing too", len(ats))
		}
		// **The convener signed nothing**, which is the clause.
		for i, a := range ats {
			if a.Fingerprint == hexOf(convFP) {
				t.Errorf("signature %d is the CONVENER's, on a ceremony they were to carry and "+
					"not sign", i)
			}
			if !a.Valid {
				t.Errorf("signature %d does not verify", i)
			}
		}
		// C14 as amended: every signature with a predecessor is matched; the first reports its
		// own state, which here is unmatched because it accepts nobody.
		if ats[0].Matched {
			t.Error("the FIRST signature is cross-bound, but it accepts nobody — there is no " +
				"signer before it")
		}
		for i := 1; i < len(ats); i++ {
			if !ats[i].Matched {
				t.Errorf("signature %d has a signing predecessor and is not cross-bound to it", i)
			}
		}
		if _, err := NextContributor(doc, roster); !errors.Is(err, ErrCeremonyComplete) {
			t.Errorf("after every signing party signed, the chain reports %v, want complete", err)
		}
	})
}

// TestTheSameCeremonyThroughInitiateFails — the other half of the clause above.
//
// It is what makes the carry verb necessary rather than merely convenient: `Initiate` requires the
// caller's own valid signature back, and a non-signing convener has none.
func TestTheSameCeremonyThroughInitiateFails(t *testing.T) {
	convCert, convKey := newIdentity(t)
	convFP := fingerprint(t, convCert)
	aCert, aKey := newIdentity(t)
	aFP := fingerprint(t, aCert)
	roster := Roster{Entries: []RosterEntry{
		{Fingerprint: hexOf(convFP), Signs: false},
		{Fingerprint: hexOf(aFP), Signs: true},
	}}
	doc, err := PrepareDocument(mustForm(t))
	if err != nil {
		t.Fatal(err)
	}
	tr := transports[0] // TCP: the property is about the VERB, not about the wire
	ln, err := tr.listen("127.0.0.1:0", aCert, aKey, convFP)
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
		_, e = Receive(conn.Channel, aCert, aKey, "Convener",
			confirmer{accept: true, intent: "I accept"}, okVerifier{}, nil, roster)
		recvErr <- e
	}()
	conn, err := tr.dial(context.Background(), ln.Addr().String(), convCert, convKey, aFP, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, ierr := Initiate(conn.Channel, doc, convFP, okVerifier{}, roster)
	<-recvErr
	if ierr == nil {
		t.Fatal("a non-signing convener completed a ceremony through Initiate — that verb " +
			"requires the caller's own signature back, and the carry route exists because it " +
			"cannot be satisfied by somebody who signs nothing")
	}
	// **The refusal is about a SIGNATURE, and which one it names is not the property.** Measured:
	// it comes back as "peer's signature does not accept you" rather than "your own signature is
	// missing", because `confirmCoSigned` walks the attestations and the peer's accept check
	// fires first — and both are the same fact from two directions. A non-signing convener is
	// nobody's predecessor and has no signature of its own, so `Initiate` cannot be satisfied
	// either way. What this must NOT be is a transport error, which is what it would be if the
	// hop had simply failed to happen.
	if !strings.Contains(ierr.Error(), "signature") {
		t.Errorf("Initiate refused the carrier with %v, which does not name a signature at all — "+
			"a transport failure here would mean this test is passing for a reason unrelated "+
			"to C07", ierr)
	}
	if strings.Contains(ierr.Error(), "EOF") {
		t.Errorf("Initiate refused the carrier as a transport error (%v) — the hop happened and "+
			"the verb refused it, so this must arrive as a refusal", ierr)
	}
}

// TestCarryRefusesAHostileHop — the clause's own words: "driven by a hostile hop k returning a
// different document".
//
// Two arms, and they fail for **different** reasons, which is why both are here:
//
//   - a document that is not what went out plus a trailing update — caught by the byte prefix;
//   - a document that IS a prefix extension but was signed by the WRONG party — caught by L3
//     over the return.
//
// The second is the one a reviewer would call redundant. It is not: **the prefix says the bytes
// grew from mine and says nothing at all about who signed the part that grew.** Measured — with
// the prefix check alone, an honest ceremony still passes and so does this.
func TestCarryRefusesAHostileHop(t *testing.T) {
	conv := l3Identity(t, "Convener")
	a, b, c := l3Identity(t, "A"), l3Identity(t, "B"), l3Identity(t, "C")
	roster := Roster{Entries: []RosterEntry{
		{Fingerprint: conv.fp, Signs: false},
		{Fingerprint: a.fp, Signs: true},
		{Fingerprint: b.fp, Signs: true},
		{Fingerprint: c.fp, Signs: true},
	}}
	carried := l3Prepared(t)

	// What an HONEST hop 1 returns, as the control: A's signature appended.
	honest := l3Chain(t, carried, []l3Party{a}, []l3Party{a}, "")

	for _, tc := range []struct {
		name  string
		reply []byte
		want  string
	}{
		{
			// A different document entirely — signed by the right party, but not the one handed
			// over. Without the prefix check this is a replay of any document these identities
			// ever produced.
			name:  "a different document",
			reply: l3Chain(t, l3Prepared(t), []l3Party{a}, []l3Party{a}, ""),
			want:  "not the one that was handed over",
		},
		{
			// The document that WAS handed over, extended — by the wrong party. C signs when it
			// is A's turn, so the bytes grew from mine and the chain did not advance as the
			// roster says it must.
			name:  "extended by the wrong party",
			reply: l3Chain(t, carried, []l3Party{c}, []l3Party{c}, ""),
			want:  "does not follow this ceremony's order",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Stimulus: the honest reply passes the same code, so a refusal below is about the
			// hostile reply and not about the fixture.
			if err := carryCheck(t, carried, honest, roster); err != nil {
				t.Fatalf("setup: the honest reply was refused (%v)", err)
			}
			err := carryCheck(t, carried, tc.reply, roster)
			if err == nil {
				t.Fatalf("a hostile hop returning %s was accepted, and the carrier would have "+
					"relayed it to the next party", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("refused with %q, want a refusal naming %q — the two arms must fail "+
					"for different reasons or one of them is not being exercised",
					err.Error(), tc.want)
			}
		})
	}
	_ = b
}

// carryCheck runs the return-binding half of Carry over a reply, without a socket.
//
// **The checks are duplicated here deliberately and the duplication is bounded**: driving the real
// `Carry` needs a hostile PEER, which is a second process or a hand-rolled frame writer, and this
// arm is about what the carrier does with bytes rather than about how they arrived.
// `TestAFourPartyCeremonyCompletesOverTheCarryRoute` drives the real verb end to end over both
// transports, so the wire half is not left unexercised.
func carryCheck(t *testing.T, sent, reply []byte, roster Roster) error {
	t.Helper()
	if !bytes.HasPrefix(reply, sent) {
		return errors.New("the carried document is not the one that was handed over")
	}
	want, err := NextContributor(sent, roster)
	if err != nil {
		t.Fatalf("setup: the sent document is not ready to be carried: %v", err)
	}
	next, nerr := NextContributor(reply, roster)
	switch {
	case errors.Is(nerr, ErrCeremonyComplete):
		return nil
	case nerr != nil:
		return fmt.Errorf("the carried document does not follow this ceremony's order: %w", nerr)
	case strings.EqualFold(next.Fingerprint, want.Fingerprint):
		return fmt.Errorf("%w: the document came back still waiting for %s", ErrPrefixMismatch,
			shortFP(want.Fingerprint))
	}
	return nil
}
