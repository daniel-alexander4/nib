package p2p

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
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
		_, err = Initiate(conn.Channel, aSigned, aFP, okVerifier{})
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
