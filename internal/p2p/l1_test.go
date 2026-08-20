package p2p

import (
	"errors"
	"strings"
	"testing"
	"time"

	"nib/internal/sign"
)

// L1, driven: a lying rendezvous can waste your time and can never substitute a signer.
//
// # Why this test did not exist, and what it is not
//
// Every Dial in this repo pins the listener it reaches, with two exceptions — and neither
// is this attack. `TestAManInTheMiddleSeesTwoDifferentStrings` has A **tricked into pinning
// M**, which is the substitution the pin structurally cannot catch and the spoken string
// exists for. `TestQUICRejectsAPeerThatIsNotPinned` has the wrong *dialer* reaching a
// correctly-pinned listener.
//
// Neither is the rendezvous case. A lying rendezvous does not change who you pinned and
// does not dial you: it hands you an ADDRESS. So the shape is a dialer that pins B
// correctly, is handed M's address, and must refuse — which nothing in the tree drove.
//
// The hostile rendezvous is modelled as "an address arrived from somewhere untrusted",
// because nothing in the shipped binary consumes the DHT yet. Same threat, honest stimulus.
func TestAnAddressFromALyingRendezvousCannotSubstituteASigner(t *testing.T) {
	eachTransport(t, func(t *testing.T, tr transport) {
		certA, keyA, _ := sign.GenerateIdentity("Alice")
		certB, keyB, _ := sign.GenerateIdentity("Bob")
		certM, keyM, _ := sign.GenerateIdentity("Impostor")
		fpA, err := sign.Fingerprint(certA)
		if err != nil {
			t.Fatal(err)
		}
		fpB, err := sign.Fingerprint(certB)
		if err != nil {
			t.Fatal(err)
		}

		// M is armed and WILLING to talk to A — the impostor is not passive, it is
		// running a real listener that accepts A's identity. A rendezvous that lies
		// points A at it.
		lnM, err := tr.listen("127.0.0.1:0", certM, keyM, fpA)
		if err != nil {
			t.Fatal(err)
		}
		defer lnM.Close()
		go func() {
			for {
				c, err := lnM.Accept()
				if err != nil {
					return
				}
				c.Close()
			}
		}()

		// STIMULUS FIRST: the same rig with the pin that MATCHES the listener connects.
		// Without this, "the dial was refused" is satisfied by nothing listening, by a
		// closed port, or by a listener that refuses everyone — and the test would pass
		// against a build where the pin check had been deleted entirely.
		lnB, err := tr.listen("127.0.0.1:0", certB, keyB, fpA)
		if err != nil {
			t.Fatal(err)
		}
		defer lnB.Close()
		go func() {
			for {
				c, err := lnB.Accept()
				if err != nil {
					return
				}
				c.Close()
			}
		}()
		good, err := tr.dial(lnB.Addr().String(), certA, keyA, fpB, 5*time.Second)
		if err != nil {
			t.Fatalf("stimulus: A could not reach the REAL B, so a refusal below would "+
				"prove nothing about the pin: %v", err)
		}
		good.Close()

		// The attack: A pins B, and is handed M's address.
		conn, err := tr.dial(lnM.Addr().String(), certA, keyA, fpB, 5*time.Second)
		if err == nil && conn != nil && len(conn.PeerFP) > 0 {
			// The strongest assertion available, and the one that fails BY NAME: say
			// which peer we landed on. Without it a real L1 breach is reported by the
			// read below as "refused, but not at the pin: EOF" — because the impostor's
			// accept loop closes immediately — which reads like an instrument problem
			// rather than a substituted signer.
			defer conn.Close()
			t.Fatalf("A pinned %x, was handed the impostor's address, and established a "+
				"channel with %x — L1 has failed: a lying rendezvous substituted a signer",
				fpB[:6], conn.PeerFP[:6])
		}
		if err == nil {
			// Under TLS 1.3 the client can finish its handshake before the server has
			// processed the client certificate, so a rejected dial may return SUCCESS and
			// the refusal surface on the first read. Measured and documented at
			// TestQUICRejectsAPeerThatIsNotPinned; true of TCP identically. So a nil error
			// here is not yet a failure — exchange a frame and require it to fail.
			defer conn.Close()
			_ = writeFrame(conn.Stream, make([]byte, 32))
			_ = conn.Stream.SetDeadline(time.Now().Add(5 * time.Second))
			if _, rerr := readFrameMax(conn.Stream, 32); rerr == nil {
				t.Fatal("A pinned B, was handed the impostor's address, and exchanged bytes " +
					"with it — L1 has failed: a lying rendezvous substituted a signer")
			} else {
				err = rerr
			}
		}
		// It must fail for the RIGHT reason. "Refused" is not the claim; "refused because
		// the identity did not match the pin" is, and only that distinguishes L1 holding
		// from the port being shut.
		if !isPinRefusal(err) {
			t.Fatalf("refused, but not at the pin: %v", err)
		}
	})
}

// isPinRefusal reports whether an error is OUR pinned-peer refusal, on either transport.
//
// **One string, deliberately.** An earlier version also accepted "bad certificate",
// "certificate required" and "CRYPTO_ERROR" on the belief that QUIC surfaces the peer's
// alert rather than our own message. Measured, that is wrong: QUIC surfaces our own
// sentence, tagged `(local)` —
//
//	CRYPTO_ERROR 0x12a (local): peer identity does not match the pinned peer
//
// while the peer's refusal of US arrives tagged `(remote)` as "tls: bad certificate". So
// the wider alternation matched BOTH DIRECTIONS and could not tell the one under test from
// its mirror image — which is the whole claim. The exact string is present on both
// transports, so nothing is lost by requiring it.
func isPinRefusal(err error) bool {
	return err != nil && strings.Contains(err.Error(), "peer identity does not match the pinned peer")
}

// TestTheClockSkewRefusalNamesADirectionAndAMagnitude — D19's fifth cause.
//
// Before this, both directions produced "peer transport certificate is expired or not yet
// valid" — two opposite facts in one sentence, naming neither, arriving at the user inside
// cause 4's "could not connect". The numbers were in hand at the refusal and discarded.
func TestTheClockSkewRefusalNamesADirectionAndAMagnitude(t *testing.T) {
	certA, keyA := newIdentity(t)
	fpA := fingerprint(t, certA)
	chain := rawChain(t, certA, keyA)
	var err error

	// STIMULUS: at the real time the chain verifies, so the refusals below are about the
	// clock and nothing else.
	if _, err := verifyPinnedPeer(chain, fpA, time.Now()); err != nil {
		t.Fatalf("stimulus: the chain does not verify at the current time: %v", err)
	}

	// Our clock is BEHIND theirs: their certificate is not yet valid here.
	_, err = verifyPinnedPeer(chain, fpA, time.Now().Add(-2*time.Hour))
	var behind *ClockSkewError
	if !errors.As(err, &behind) {
		t.Fatalf("a clock two hours behind gave %v, want a ClockSkewError", err)
	}
	if !behind.Behind {
		t.Error("the direction is inverted: our clock was behind and it reported ahead")
	}
	if behind.By < time.Hour {
		t.Errorf("magnitude %s, want at least an hour", behind.By)
	}
	if !strings.Contains(behind.Error(), "behind") {
		t.Errorf("the sentence does not name the direction: %q", behind.Error())
	}

	// Our clock is AHEAD: their certificate has already expired here.
	_, err = verifyPinnedPeer(chain, fpA, time.Now().Add(48*time.Hour))
	var ahead *ClockSkewError
	if !errors.As(err, &ahead) {
		t.Fatalf("a clock two days ahead gave %v, want a ClockSkewError", err)
	}
	if ahead.Behind {
		t.Error("the direction is inverted: our clock was ahead and it reported behind")
	}
	if !strings.Contains(ahead.Error(), "ahead") {
		t.Errorf("the sentence does not name the direction: %q", ahead.Error())
	}
	// And it must not be the old both-facts-at-once sentence.
	for _, e := range []error{behind, ahead} {
		if strings.Contains(e.Error(), "expired or not yet valid") {
			t.Errorf("still the generic sentence, which names neither direction: %q", e.Error())
		}
	}
}

// TestTheClockSkewCauseSurvivesTheTLSBoundary.
//
// verifyPinnedPeer runs inside crypto/tls's VerifyPeerCertificate hook, and its error has
// to cross that boundary to reach anyone. tls sends an alert and the peer sees something
// else entirely, so "the typed error exists" and "a caller can recover it" are different
// claims — and only the second is worth anything. A unit test of the type alone would be
// green on a build where no caller could ever read the cause.
func TestTheClockSkewCauseSurvivesTheTLSBoundary(t *testing.T) {
	realNow := time.Now

	aCert, aKey := newIdentity(t)
	bCert, bKey := newIdentity(t)
	aFP := fingerprint(t, aCert)
	bFP := fingerprint(t, bCert)

	ln, err := Listen("127.0.0.1:0", bCert, bKey, aFP)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, e := ln.Accept()
			if e != nil {
				return
			}
			go c.Close()
		}
	}()

	// STIMULUS: at the true time the dial succeeds, so the failure below is the clock and
	// nothing about the rig.
	warm, werr := Dial(ln.Addr().String(), aCert, aKey, bFP, 10*time.Second)
	if werr != nil {
		t.Fatalf("stimulus: the dial fails even at the correct time: %v", werr)
	}
	warm.Close()

	// A SECOND, untouched listener for the skewed dial.
	//
	// Reusing the first one made this test position-dependent — it passed when another
	// test had run before it and timed out when it ran first, which is a flake and a flake
	// is worse than no test. tlsListener.Accept runs the handshake inline, so the listener
	// carries state from the connection before; the property under test is about one
	// handshake and should not depend on the one before it.
	ln2, err := Listen("127.0.0.1:0", bCert, bKey, aFP)
	if err != nil {
		t.Fatal(err)
	}
	defer ln2.Close()
	go func() {
		for {
			c, e := ln2.Accept()
			if e != nil {
				return
			}
			go c.Close()
		}
	}()

	// Now this machine believes it is two hours in the past.
	defer setClock(func() time.Time { return realNow().Add(-2 * time.Hour) })()
	_, err = Dial(ln2.Addr().String(), aCert, aKey, bFP, 6*time.Second)
	if err == nil {
		t.Fatal("a two-hour clock skew was accepted")
	}
	var skew *ClockSkewError
	if !errors.As(err, &skew) {
		t.Fatalf("the cause did not survive the TLS boundary — a caller sees %q and cannot "+
			"tell a clock problem from any other refusal, which is cause 5 collapsing back "+
			"into cause 4's \"something else\"", err)
	}
	if !skew.Behind {
		t.Error("direction inverted across the boundary")
	}
	if skew.By < time.Hour {
		t.Errorf("magnitude %s did not survive", skew.By)
	}
}
