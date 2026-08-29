package p2p

import (
	"crypto/tls"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"nib/internal/sign"
)

// P07.S09c: the two skew surfaces D32 had not been given a sentence — the attestation tag, and a
// session protocol with no version in common.

// TestANewerAttestationTagIsLegibleAsASkewRatherThanAsSilence is the fourth surface.
//
// `Attestations` required `[NibCoSign:1]` verbatim, so a signature written by a build that had
// moved to `[NibCoSign:2]` matched nothing and arrived with every field empty. That is
// indistinguishable from a signature that carries no attestation at all — and
// `markOneProceeding` treats an empty commitment on a valid signature as disqualifying, so ONE
// such signature made the whole document report "not one proceeding": an accusation about the
// parties, caused by an upgrade.
func TestANewerAttestationTagIsLegibleAsASkewRatherThanAsSilence(t *testing.T) {
	st := sign.Status{Signers: []sign.SignerInfo{
		{Name: "current", Fingerprint: strings.Repeat("aa", 32), Valid: true,
			Reason: attestationTag + " Accepts X [SPKI:" + strings.Repeat("bb", 32) + "]"},
		{Name: "newer", Fingerprint: strings.Repeat("bb", 32), Valid: true,
			Reason: "[NibCoSign:2] a format this build does not know"},
		{Name: "none", Fingerprint: strings.Repeat("cc", 32), Valid: true,
			Reason: "Finalized in Nib"},
	}}
	got := Attestations(st, Proceeding{})
	if len(got) != 3 {
		t.Fatalf("setup: %d attestations for 3 signatures", len(got))
	}

	// SETUP: this build's own tag still parses, or "the newer one is different" is true of a
	// reader that parses nothing.
	if got[0].TagVersion != attestationTagVersion {
		t.Fatalf("setup: a signature at this build's own tag version reports TagVersion %d, want %d",
			got[0].TagVersion, attestationTagVersion)
	}
	if got[0].AcceptedPeer == "" {
		t.Fatal("setup: this build no longer parses its own attestation format")
	}

	// The newer one: legible AS a skew.
	if got[1].TagVersion != 2 {
		t.Errorf("a signature tagged [NibCoSign:2] reports TagVersion %d. Without the version a "+
			"reader cannot tell 'this build cannot read that signature' from 'that party "+
			"attested to nothing', and the second is an accusation", got[1].TagVersion)
	}
	// And still not TRUSTED: a format this build does not know has no fields it may interpret.
	if got[1].AcceptedPeer != "" || got[1].RosterHash != "" {
		t.Errorf("a newer tag's fields were parsed anyway (peer=%q roster=%q). Reading the "+
			"version is not the same as understanding the payload, and guessing at a format this "+
			"build does not know is how a skew becomes a wrong answer rather than no answer",
			got[1].AcceptedPeer, got[1].RosterHash)
	}

	// A signature with NO tag is the third state and must stay distinct from both.
	if got[2].TagVersion != 0 {
		t.Errorf("an ordinary Finalize signature reports TagVersion %d, want 0 — a signature that "+
			"is not a Nib attestation at all is not a version skew", got[2].TagVersion)
	}
}

// TestADisjointProtocolIsASentenceNotATLSAlert is the third surface, and it drives a REAL
// handshake rather than asserting the substring the classifier matches on.
//
// `asProtocolSkew` matches "no application protocol" in somebody else's error text. A test that
// asserted that constant would be checking the constant against itself; this one stands up a
// listener speaking a protocol this build does not offer and dials it for real, so if the stdlib
// ever rewords the alert this goes red instead of silently returning the raw error.
func TestADisjointProtocolIsASentenceNotATLSAlert(t *testing.T) {
	certPEM, keyPEM, err := sign.GenerateIdentity("listener")
	if err != nil {
		t.Fatal(err)
	}
	fp, err := sign.Fingerprint(certPEM)
	if err != nil {
		t.Fatal(err)
	}
	dialCert, dialKey, err := sign.GenerateIdentity("dialer")
	if err != nil {
		t.Fatal(err)
	}
	dialFP, err := sign.Fingerprint(dialCert)
	if err != nil {
		t.Fatal(err)
	}
	srvCfg, err := SessionTLS(certPEM, keyPEM, dialFP, true)
	if err != nil {
		t.Fatal(err)
	}
	// A build from the future: it speaks only a version this one has never heard of.
	srvCfg.NextProtos = []string{"nib/99"}
	srvCfg.ClientAuth = tls.NoClientCert
	ln, err := tls.Listen("tcp", "127.0.0.1:0", srvCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, aerr := ln.Accept()
			if aerr != nil {
				return
			}
			_ = c.(*tls.Conn).Handshake()
			c.Close()
		}
	}()

	_, derr := Dial(t.Context(), ln.Addr().String(), dialCert, dialKey, fp, 3*time.Second)
	if derr == nil {
		t.Fatal("dialling a peer that speaks no protocol version in common succeeded")
	}

	var skew *ProtocolSkewError
	if !errors.As(derr, &skew) {
		t.Fatalf("a disjoint protocol arrived as %v — a raw TLS alert on the connect path, where "+
			"every other failure means the peer is unreachable. D32: a version skew produces a "+
			"sentence naming the mismatch, not a parse error, and an alert IS the parse error "+
			"one layer down", derr)
	}
	msg := skew.Error()
	if !strings.Contains(msg, "version of Nib") || !strings.Contains(msg, "Update") {
		t.Errorf("the sentence does not name the mismatch or the fix: %q", msg)
	}
	// It says which versions were on the table, so a user reporting it says something useful.
	for _, want := range sessionALPN {
		if !strings.Contains(msg, want) {
			t.Errorf("the sentence does not name this build's offer %q: %q", want, msg)
		}
	}
}

// TestAnOrdinaryDialFailureIsNotCalledAVersionSkew is the control. A classifier that returned the
// typed error for every handshake failure would relabel every unreachable peer as an out-of-date
// one — which is the same misdiagnosis in the other direction, and the more common case by far.
func TestAnOrdinaryDialFailureIsNotCalledAVersionSkew(t *testing.T) {
	// A closed port: the ordinary "nothing is listening" failure.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()

	certPEM, keyPEM, err := sign.GenerateIdentity("dialer")
	if err != nil {
		t.Fatal(err)
	}
	_, derr := Dial(t.Context(), addr, certPEM, keyPEM, make([]byte, 32), 3*time.Second)
	if derr == nil {
		t.Fatal("setup: dialling a closed port succeeded")
	}
	var skew *ProtocolSkewError
	if errors.As(derr, &skew) {
		t.Errorf("an unreachable peer was reported as a version skew: %v. Telling a user to "+
			"update Nib when the other machine is simply not listening sends them to the one "+
			"place the problem is not", derr)
	}
}
