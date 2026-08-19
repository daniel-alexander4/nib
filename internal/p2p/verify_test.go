package p2p

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"errors"
	"strings"
	"testing"
	"time"
)

// livePair brings up a real mTLS session between two pinned identities and hands each
// side to its half of the exchange. Real TLS rather than a pipe, because the string binds
// the channel through ExportKeyingMaterial and a pipe has no keying material at all —
// a test over a pipe could not distinguish "bound to this channel" from "not bound".
func livePair(t *testing.T, run func(initiator bool, conn *tls.Conn, myFP, peerFP []byte) (string, error)) (string, string) {
	t.Helper()
	aCert, aKey := newIdentity(t)
	bCert, bKey := newIdentity(t)
	aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)

	ln, err := Listen("127.0.0.1:0", bCert, bKey, aFP)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	type res struct {
		s   string
		err error
	}
	rc := make(chan res, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			rc <- res{err: err}
			return
		}
		defer conn.Close()
		tc := conn.(*tls.Conn)
		if err := tc.Handshake(); err != nil {
			rc <- res{err: err}
			return
		}
		s, e := run(false, tc, bFP, aFP)
		rc <- res{s, e}
	}()

	conn, err := Dial(ln.Addr().String(), aCert, aKey, bFP, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	got, err := run(true, conn, aFP, bFP)
	if err != nil {
		t.Fatalf("initiator: %v", err)
	}
	r := <-rc
	if r.err != nil {
		t.Fatalf("receiver: %v", r.err)
	}
	return got, r.s
}

func exchangeBoth(t *testing.T) (string, string) {
	t.Helper()
	return livePair(t, func(initiator bool, conn *tls.Conn, myFP, peerFP []byte) (string, error) {
		return verificationExchange(conn, initiator, myFP, peerFP)
	})
}

// TestBothSidesDeriveTheSameWords — the first acceptance bullet. Two endpoints of one
// session, computing from opposite viewpoints, must produce the same four words, or the
// spoken check fails for honest users and tells them nothing.
func TestBothSidesDeriveTheSameWords(t *testing.T) {
	a, b := exchangeBoth(t)
	if a != b {
		t.Fatalf("the two sides derived different strings: %q vs %q", a, b)
	}
	if n := len(strings.Fields(a)); n != 4 {
		t.Errorf("the verification string is %d words, want 4: %q", n, a)
	}
}

// TestTwoSessionsDeriveDifferentWords — the second bullet. A string that repeated between
// sessions would let an attacker who once overheard it replay it, and would make the
// check meaningless on the second call between the same two people.
func TestTwoSessionsDeriveDifferentWords(t *testing.T) {
	first, _ := exchangeBoth(t)
	second, _ := exchangeBoth(t)
	if first == second {
		t.Errorf("two sessions between fresh pairs produced the same string %q — the "+
			"derivation is not taking the per-session contributions or the channel binding", first)
	}
}

// TestAManInTheMiddleSeesTwoDifferentStrings — the third bullet, driven with a real
// attacker rather than by comparing two unrelated sessions.
//
// **The first version of this test was two independent sessions with fresh identities and
// an assertion that their strings differed — which is TestTwoSessionsDeriveDifferentWords
// under a more impressive name.** It would have passed against a derivation that ignored
// the peer's identity entirely. What the bullet asks for is a SUBSTITUTED identity, so
// that is what runs here.
//
// M sits between A and B, holding its own identity and pinned by both. Each leg is a
// perfectly sound mTLS session — that is the point, and it is why the machine cannot
// catch this. The two humans can: their strings differ, because A's derives over
// {A, M} and B's over {M, B}, with different contributions and different channels.
func TestAManInTheMiddleSeesTwoDifferentStrings(t *testing.T) {
	aCert, aKey := newIdentity(t)
	bCert, bKey := newIdentity(t)
	mCert, mKey := newIdentity(t)
	aFP, bFP, mFP := fingerprint(t, aCert), fingerprint(t, bCert), fingerprint(t, mCert)

	// Leg 1: A dials, believing it reached B. M answers with M's identity, and A has been
	// tricked into pinning M — the substitution the bullet names.
	lnM, err := Listen("127.0.0.1:0", mCert, mKey, aFP)
	if err != nil {
		t.Fatal(err)
	}
	defer lnM.Close()
	legA := make(chan string, 1)
	go func() {
		conn, err := lnM.Accept()
		if err != nil {
			legA <- ""
			return
		}
		defer conn.Close()
		tc := conn.(*tls.Conn)
		if err := tc.Handshake(); err != nil {
			legA <- ""
			return
		}
		s, _ := verificationExchange(tc, false, mFP, aFP)
		legA <- s
	}()
	connA, err := Dial(lnM.Addr().String(), aCert, aKey, mFP, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer connA.Close()
	aString, err := verificationExchange(connA, true, aFP, mFP)
	if err != nil {
		t.Fatal(err)
	}
	mOnLegA := <-legA

	// Leg 2: B listens, and M dials it holding M's identity.
	lnB, err := Listen("127.0.0.1:0", bCert, bKey, mFP)
	if err != nil {
		t.Fatal(err)
	}
	defer lnB.Close()
	legB := make(chan string, 1)
	go func() {
		conn, err := lnB.Accept()
		if err != nil {
			legB <- ""
			return
		}
		defer conn.Close()
		tc := conn.(*tls.Conn)
		if err := tc.Handshake(); err != nil {
			legB <- ""
			return
		}
		s, _ := verificationExchange(tc, false, bFP, mFP)
		legB <- s
	}()
	connB, err := Dial(lnB.Addr().String(), mCert, mKey, bFP, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer connB.Close()
	if _, err := verificationExchange(connB, true, mFP, bFP); err != nil {
		t.Fatal(err)
	}
	bString := <-legB

	// The stimulus: both legs really completed, and each is internally consistent. Without
	// this the inequality below is satisfied by a leg that failed and returned "".
	if aString == "" || bString == "" || mOnLegA == "" {
		t.Fatal("setup: a leg did not complete, so the comparison is not about the attacker")
	}
	if aString != mOnLegA {
		t.Fatalf("setup: leg 1's two ends disagree (%q vs %q) — the attacker's own session is "+
			"broken, so this is not testing what it claims", aString, mOnLegA)
	}

	// The property. A reads its words to B over the phone; they do not match B's screen.
	if aString == bString {
		t.Errorf("A and B derived the same string %q while talking through M. The spoken "+
			"check would succeed and neither would learn there is a third party holding both "+
			"halves of the conversation.", aString)
	}
}

// TestARevealAfterSeeingIsRejectedBeforeAnyStringExists is the fourth bullet, and it is
// the only test here that can see the attack the commitment step is for.
//
// The other three are satisfied by ANY per-session derivation. This one drives the
// out-of-order case directly: a peer that commits to one value and then, having seen the
// other side's contribution, reveals a DIFFERENT one — which is exactly what choosing
// after seeing looks like on the wire. It must be refused, and refused before a string is
// derived, because a string derived from an uncommitted contribution is one the attacker
// selected.
func TestARevealAfterSeeingIsRejectedBeforeAnyStringExists(t *testing.T) {
	aCert, aKey := newIdentity(t)
	bCert, bKey := newIdentity(t)
	aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)

	ln, err := Listen("127.0.0.1:0", bCert, bKey, aFP)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	sawContribution := make(chan bool, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			sawContribution <- false
			return
		}
		defer conn.Close()
		tc := conn.(*tls.Conn)
		if err := tc.Handshake(); err != nil {
			sawContribution <- false
			return
		}
		// A dishonest receiver, hand-rolled rather than driven through
		// verificationExchange — the point is to do what that function refuses to.
		honest := make([]byte, contributionLen)
		_, _ = rand.Read(honest)
		commit := sha256.Sum256(honest)

		_, _ = readFrame(tc)          // the initiator's commitment
		_ = writeFrame(tc, commit[:]) // commit to `honest`…
		theirs, err := readFrame(tc)  // …see what they actually sent…
		sawContribution <- err == nil && len(theirs) == contributionLen
		chosen := make([]byte, contributionLen)
		_, _ = rand.Read(chosen)
		_ = writeFrame(tc, chosen) // …and reveal something else.
	}()

	conn, err := Dial(ln.Addr().String(), aCert, aKey, bFP, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	s, err := verificationExchange(conn, true, aFP, bFP)

	// The stimulus, asserted before the response is graded: the dishonest peer really did
	// get to see the initiator's contribution before choosing. Without this the refusal
	// below could be any protocol error — a closed connection, a short frame — and the
	// test would pass without the attack ever having been attempted.
	if !<-sawContribution {
		t.Fatal("setup: the dishonest peer never received a contribution to choose against, " +
			"so the out-of-order case was not exercised")
	}

	if err == nil {
		t.Fatalf("a contribution that did not match its commitment was accepted, and a "+
			"verification string was derived from it: %q. That string is one the peer chose "+
			"after seeing ours, which is the 2²² birthday search the commitment exists to stop", s)
	}
	if !errors.Is(err, errCommitmentBroken) {
		t.Errorf("refused with %v, want the broken-commitment error — a different error means "+
			"the exchange failed for an unrelated reason and the commitment check may never "+
			"have run", err)
	}
	if s != "" {
		t.Errorf("a string was returned alongside the refusal: %q. The clause is that the peer "+
			"is rejected BEFORE any string is derived", s)
	}
}

// TestTheDerivationTakesEveryInput holds everything constant and varies one input at a
// time, because through the live exchange none of them can be seen.
//
// Measured before this test existed: deleting the fingerprints from the hash, or deleting
// the channel binding, left every end-to-end test in this file GREEN. Fresh contributions
// make each session's string differ on their own, so the other inputs are invisible to any
// test that goes through a real session. D4 requires the string to bind "both identity
// keys plus the handshake transcript"; this is the only place that requirement is checked
// rather than asserted in a comment.
func TestTheDerivationTakesEveryInput(t *testing.T) {
	base := struct{ ekm, fpA, fpB, cA, cB []byte }{
		ekm: bytes.Repeat([]byte{1}, 32),
		fpA: bytes.Repeat([]byte{2}, 32),
		fpB: bytes.Repeat([]byte{3}, 32),
		cA:  bytes.Repeat([]byte{4}, 32),
		cB:  bytes.Repeat([]byte{5}, 32),
	}
	want, err := deriveVerification(base.ekm, base.fpA, base.fpB, base.cA, base.cB)
	if err != nil {
		t.Fatal(err)
	}
	// Deterministic: the same inputs must give the same words, or the two sides never agree.
	again, err := deriveVerification(base.ekm, base.fpA, base.fpB, base.cA, base.cB)
	if err != nil {
		t.Fatal(err)
	}
	if want != again {
		t.Fatalf("the derivation is not deterministic: %q then %q", want, again)
	}

	// Each alternative preserves the SORT ORDER of the pair it belongs to, and that is not
	// fussiness. The derivation orders the two fingerprints canonically, so an alternative
	// that re-sorts them changes which variable holds which value — and the string then
	// changes for the wrong reason. Measured: with an alternative that re-sorted, dropping
	// `h.Write(loFP)` entirely left this test green, because varying fpA moved it to the
	// hi slot and the hi write still carried it.
	alt := bytes.Repeat([]byte{9}, 32)   // for inputs that are not sorted
	altLo := bytes.Repeat([]byte{1}, 32) // still below fpB, so still the lo fingerprint
	altHi := bytes.Repeat([]byte{4}, 32) // still above fpA, so still the hi fingerprint
	for _, c := range []struct {
		name              string
		ekm, a, b, ca, cb []byte
		why               string
	}{
		{"channel binding", alt, base.fpA, base.fpB, base.cA, base.cB,
			"the string is not bound to the channel, so a confirmation computed on one " +
				"connection would be valid on another (D18, which P01.S05 has to enforce)"},
		{"our fingerprint", base.ekm, altLo, base.fpB, base.cA, base.cB,
			"the string does not bind our identity key (D4)"},
		{"their fingerprint", base.ekm, base.fpA, altHi, base.cA, base.cB,
			"the string does not bind the peer's identity key (D4) — a substituted peer " +
				"would produce the same words"},
		{"our contribution", base.ekm, base.fpA, base.fpB, alt, base.cB,
			"our committed contribution does not reach the string, so committing to it bought nothing"},
		{"their contribution", base.ekm, base.fpA, base.fpB, base.cA, alt,
			"their committed contribution does not reach the string"},
	} {
		got, err := deriveVerification(c.ekm, c.a, c.b, c.ca, c.cb)
		if err != nil {
			t.Fatal(err)
		}
		if got == want {
			t.Errorf("changing the %s left the string at %q — %s", c.name, got, c.why)
		}
	}
}

// TestTheDerivationIsViewpointIndependent: the two sides hold the same pair in opposite
// order, and must still hash identical bytes. Without the canonical ordering the honest
// case fails and two people staring at different words conclude they are under attack.
func TestTheDerivationIsViewpointIndependent(t *testing.T) {
	ekm := bytes.Repeat([]byte{1}, 32)
	fpA, fpB := bytes.Repeat([]byte{2}, 32), bytes.Repeat([]byte{3}, 32)
	cA, cB := bytes.Repeat([]byte{4}, 32), bytes.Repeat([]byte{5}, 32)

	fromA, err := deriveVerification(ekm, fpA, fpB, cA, cB)
	if err != nil {
		t.Fatal(err)
	}
	// B sees itself as "mine" and A as "theirs", and its own contribution as "mine".
	fromB, err := deriveVerification(ekm, fpB, fpA, cB, cA)
	if err != nil {
		t.Fatal(err)
	}
	if fromA != fromB {
		t.Errorf("the two viewpoints derive %q and %q — the canonical ordering is wrong, so "+
			"honest peers would see a mismatch and read it as an attack", fromA, fromB)
	}
}
