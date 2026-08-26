package p2p

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"nib/internal/testpdf"
)

// channelOf establishes a Channel over a completed mTLS connection, failing the test if
// the peer's identity or the exporter is not available. It is TLSChannel and not a
// hand-built Channel on purpose: a test that assembled the struct itself could keep
// passing while TLSChannel stopped reading the fingerprint off the verified chain.
// It reports with Errorf and returns the zero Channel rather than calling Fatalf,
// because several call sites are inside accept goroutines and FailNow is only legal on
// the goroutine running the test. A zero Channel then fails Channel.check at the entry
// point — a clean named error rather than a nil dereference.
func channelOf(t *testing.T, conn *tls.Conn) Channel {
	t.Helper()
	ch, err := TLSChannel(conn)
	if err != nil {
		t.Errorf("establish channel: %v", err)
		return Channel{}
	}
	return ch
}

// livePair brings up a real mTLS session between two pinned identities and hands each
// side to its half of the exchange. Real TLS rather than a pipe, because the string binds
// the channel through ExportKeyingMaterial and a pipe has no keying material at all —
// a test over a pipe could not distinguish "bound to this channel" from "not bound".
func livePair(t *testing.T, tr transport, run func(initiator bool, ch Channel, myFP []byte) (string, error)) (string, string) {
	t.Helper()
	aCert, aKey := newIdentity(t)
	bCert, bKey := newIdentity(t)
	aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)

	ln, err := tr.listen("127.0.0.1:0", bCert, bKey, aFP)
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
		s, e := run(false, conn.Channel, bFP)
		rc <- res{s, e}
	}()

	conn, err := tr.dial(context.Background(), ln.Addr().String(), aCert, aKey, bFP, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	got, err := run(true, conn.Channel, aFP)
	if err != nil {
		t.Fatalf("initiator: %v", err)
	}
	r := <-rc
	if r.err != nil {
		t.Fatalf("receiver: %v", r.err)
	}
	return got, r.s
}

func exchangeBoth(t *testing.T, tr transport) (string, string) {
	t.Helper()
	return livePair(t, tr, func(initiator bool, ch Channel, myFP []byte) (string, error) {
		return verificationExchange(ch, initiator, myFP)
	})
}

// TestBothSidesDeriveTheSameWords — the first acceptance bullet. Two endpoints of one
// session, computing from opposite viewpoints, must produce the same four words, or the
// spoken check fails for honest users and tells them nothing.
func TestBothSidesDeriveTheSameWords(t *testing.T) {
	eachTransport(t, func(t *testing.T, tr transport) {
		a, b := exchangeBoth(t, tr)
		if a != b {
			t.Fatalf("the two sides derived different strings: %q vs %q", a, b)
		}
		if n := len(strings.Fields(a)); n != 4 {
			t.Errorf("the verification string is %d words, want 4: %q", n, a)
		}
	})
}

// TestTwoSessionsDeriveDifferentWords — the second bullet. A string that repeated between
// sessions would let an attacker who once overheard it replay it, and would make the
// check meaningless on the second call between the same two people.
func TestTwoSessionsDeriveDifferentWords(t *testing.T) {
	eachTransport(t, func(t *testing.T, tr transport) {
		first, _ := exchangeBoth(t, tr)
		second, _ := exchangeBoth(t, tr)
		if first == second {
			t.Errorf("two sessions between fresh pairs produced the same string %q — the "+
				"derivation is not taking the per-session contributions or the channel binding", first)
		}
	})
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
		s, _ := verificationExchange(conn.Channel, false, mFP)
		legA <- s
	}()
	connA, err := Dial(context.Background(), lnM.Addr().String(), aCert, aKey, mFP, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer connA.Close()
	aString, err := verificationExchange(connA.Channel, true, aFP)
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
		s, _ := verificationExchange(conn.Channel, false, bFP)
		legB <- s
	}()
	connB, err := Dial(context.Background(), lnB.Addr().String(), mCert, mKey, bFP, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer connB.Close()
	if _, err := verificationExchange(connB.Channel, true, mFP); err != nil {
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
		// A dishonest receiver, hand-rolled rather than driven through
		// verificationExchange — the point is to do what that function refuses to.
		honest := make([]byte, contributionLen)
		_, _ = rand.Read(honest)
		commit := sha256.Sum256(honest)

		_, _ = readFrame(conn.Stream)          // the initiator's commitment
		_ = writeFrame(conn.Stream, commit[:]) // commit to `honest`…
		theirs, err := readFrame(conn.Stream)  // …see what they actually sent…
		sawContribution <- err == nil && len(theirs) == contributionLen
		chosen := make([]byte, contributionLen)
		_, _ = rand.Read(chosen)
		_ = writeFrame(conn.Stream, chosen) // …and reveal something else.
	}()

	conn, err := Dial(context.Background(), ln.Addr().String(), aCert, aKey, bFP, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	s, err := verificationExchange(conn.Channel, true, aFP)

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

// TestAnOversizedVerificationFrameIsRefusedBeforeAllocating: a peer that declares 128 MiB
// where 32 bytes are expected is refused on the header.
//
// maxFrame is 128 MiB because a document frame legitimately is, and the verification
// frames were reading with that bound — so a pinned peer could make the listener allocate
// 128 MiB with four bytes. Being pinned lowers the severity and does not remove it: a
// pinned peer is someone you agreed to sign with, not someone you agreed to let allocate.
//
// The assertion is on the ERROR, because "did not allocate" is not observable from here.
// What is observable is that the refusal names the bound it applied — and the bound it
// names is the small one, which it could only know if the small-bounded reader ran.
func TestAnOversizedVerificationFrameIsRefusedBeforeAllocating(t *testing.T) {
	aCert, aKey := newIdentity(t)
	bCert, bKey := newIdentity(t)
	aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)

	ln, err := Listen("127.0.0.1:0", bCert, bKey, aFP)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	declared := make(chan bool, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			declared <- false
			return
		}
		defer conn.Close()
		_, _ = readFrameMax(conn.Stream, sha256.Size) // the initiator's commitment
		// A four-byte header claiming the whole document budget, and no body.
		var hdr [4]byte
		binary.BigEndian.PutUint32(hdr[:], maxFrame)
		_, werr := conn.Stream.Write(hdr[:])
		declared <- werr == nil
	}()

	conn, err := Dial(context.Background(), ln.Addr().String(), aCert, aKey, bFP, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.Stream.SetDeadline(time.Now().Add(5 * time.Second))
	_, err = verificationExchange(conn.Channel, true, aFP)

	// Stimulus before response: the oversized header really was sent. Without this the
	// refusal below could be a closed connection and the test would pass having exercised
	// nothing.
	if !<-declared {
		t.Fatal("setup: the oversized header was never sent, so the bound was not tested")
	}
	if err == nil {
		t.Fatal("a frame declaring the full document budget was accepted where 32 bytes are expected")
	}
	if !strings.Contains(err.Error(), "max 32") {
		t.Errorf("refused with %v — the message does not name the 32-byte bound, so the "+
			"general 128 MiB reader may still be the one that ran", err)
	}
}

// --- L2: no path reaches the exchange unconfirmed --------------------------

// declining is a Verifier that says the words did not match — the man-in-the-middle
// answer. It counts its calls, so a test can tell "declined" from "never asked".
type declining struct {
	mu    sync.Mutex
	calls int
}

func (d *declining) ConfirmVerification(string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls++
	return false, nil
}

func (d *declining) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls
}

// TestL2NoDocumentBytesCrossBeforeBothConfirmations is the guard the acceptance names:
// "a guard test named for L2 fails if any path reaches the signing exchange unconfirmed".
//
// Four entry points carry document bytes — Initiate, Receive, SendDocument,
// ReceiveDocument — and each is driven here with a DECLINING verifier. Two things are
// asserted for each, and the second is the one that matters:
//
//  1. the call fails, and fails with ErrVerificationDeclined rather than a network error;
//  2. the verifier was actually CALLED — because a path that never asked would also fail
//     if it happened to error for some other reason, and would pass check 1 while being
//     the exact defect L2 forbids.
func TestL2NoDocumentBytesCrossBeforeBothConfirmations(t *testing.T) {
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		run  func(t *testing.T, initiator bool, conn *Conn, myFP, peerFP []byte, v Verifier) error
	}{
		{"Initiate", func(t *testing.T, _ bool, conn *Conn, myFP, _ []byte, v Verifier) error {
			_, e := Initiate(conn.Channel, pdf, myFP, v)
			return e
		}},
		{"SendDocument", func(t *testing.T, _ bool, conn *Conn, myFP, _ []byte, v Verifier) error {
			return SendDocument(conn.Channel, pdf, myFP, v)
		}},
	} {
		t.Run(tc.name+" (dialing side)", func(t *testing.T) {
			// L2 over both transports. It is the clause most worth parameterising:
			// "no document bytes cross the wire before both confirmations" is a
			// property of the ORDERING the core imposes, and the core is the only
			// thing the two transports share — so a transport that reordered or
			// coalesced frames would break it without touching the core at all.
			eachTransport(t, func(t *testing.T, tr transport) {
				aCert, aKey := newIdentity(t)
				bCert, bKey := newIdentity(t)
				aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)

				ln, err := tr.listen("127.0.0.1:0", bCert, bKey, aFP)
				if err != nil {
					t.Fatal(err)
				}
				defer ln.Close()

				// The instrument is the size of the FIRST frame on the wire, and getting this
				// wrong is what the first version of this test did.
				//
				// It watched the peer's LAST read and asserted no large payload appeared. That
				// is not the clause. Driven against a build with the document write moved
				// BEFORE the gate — the exact defect — it still passed: the stream desynchronises,
				// the peer's later reads return garbage or nothing, and "no large payload at the
				// end" stays true while the document has already gone.
				//
				// The clause is "no document bytes cross the wire before both confirmations".
				// So: what is the first thing the initiator sends? It must be a 32-byte
				// commitment. If it is a document, its size says so immediately and
				// unambiguously.
				firstFrame := make(chan int, 1)
				go func() {
					conn, err := ln.Accept()
					if err != nil {
						firstFrame <- -1
						return
					}
					defer conn.Close()
					_ = conn.Stream.SetDeadline(time.Now().Add(5 * time.Second))
					first, err := readFrame(conn.Stream) // deliberately unbounded: measure what arrived
					if err != nil {
						firstFrame <- -1
						return
					}
					firstFrame <- len(first)
					// Then play honest so the initiator reaches its verifier rather than dying
					// on I/O — the test is about the gate, not about error handling.
					mine := make([]byte, contributionLen)
					_, _ = rand.Read(mine)
					commit := sha256.Sum256(mine)
					_ = writeFrame(conn.Stream, commit[:])
					_, _ = readFrameMax(conn.Stream, contributionLen)
					_ = writeFrame(conn.Stream, mine)
					_, _ = readFrame(conn.Stream)
				}()

				conn, err := tr.dial(context.Background(), ln.Addr().String(), aCert, aKey, bFP, 10*time.Second)
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close()
				v := &declining{}
				err = tc.run(t, true, conn, aFP, bFP, v)

				if v.count() == 0 {
					t.Fatalf("%s never asked the verifier — the path reaches the document "+
						"exchange unconfirmed, which is exactly what L2 forbids", tc.name)
				}
				if !errors.Is(err, ErrVerificationDeclined) {
					t.Errorf("%s returned %v, want ErrVerificationDeclined — a decline must never "+
						"read as a network failure, because 'could not connect' invites a retry and "+
						"a retry is the worst advice when someone is sitting between you", tc.name, err)
				}
				switch n := <-firstFrame; {
				case n < 0:
					t.Errorf("%s: the peer never received a first frame, so the ordering was not "+
						"observed and nothing above is measuring L2", tc.name)
				case n != sha256.Size:
					t.Errorf("%s put a %d-byte frame on the wire first, before the 32-byte "+
						"commitment. That is the document, and it crossed before anyone confirmed "+
						"who they were talking to — the whole of L2.", tc.name, n)
				}
			})
		})
	}
}

// TestL2CoversEveryDocumentCarryingEntryPoint is the population floor.
//
// The behavioural test above enumerates the paths it knows about. A fifth entry point
// added later would inherit nothing from it and the suite would stay green — so the count
// is pinned at the source. Every exported function in this package that writes or reads a
// document frame must take a Verifier.
func TestL2CoversEveryDocumentCarryingEntryPoint(t *testing.T) {
	src, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"func Initiate(ch Channel, mySignedPDF, myFingerprint []byte, v Verifier)",
		// The trailing Roster arrived at P07.S03 (the L3 gate). Updated here rather than
		// loosened to a prefix match: the point of pinning the WHOLE signature is that a
		// parameter list which changes shape gets looked at, and a prefix match would have
		// accepted this edit silently.
		"func Receive(ch Channel, myCertPEM, myKeyPEM []byte, peerLabel string, c Confirmer, v Verifier, rd ReDeliverer, roster Roster)",
		"func SendDocument(ch Channel, pdf []byte, myFingerprint []byte, v Verifier)",
		"func ReceiveDocument(ch Channel, a Accepter, myFingerprint []byte, v Verifier)",
	}
	for _, sig := range want {
		if !strings.Contains(string(src), sig) {
			t.Errorf("this signature is gone or changed:\n  %s\nEvery document-carrying entry "+
				"point takes a Verifier (L2). If the signature moved, update this list; if the "+
				"Verifier went, the path can now carry a document unconfirmed.", sig)
		}
	}
	// A fifth path. Re-based from "*tls.Conn" to "Channel" by P02.S04, and the re-basing
	// is the point: the old regex was `^func ([A-Z]\w+)\(conn \*tls\.Conn`, which now
	// matches NOTHING in this file — it would have gone quietly to zero and compared 0
	// against 4, reporting a failure whose message named the wrong problem. A guard whose
	// subject is re-typed has to be re-typed with it.
	//
	// Stream as well as Channel: a new entry point taking a raw Stream would bypass both
	// the Verifier and Channel.check, which is the same hole one type further down.
	got := regexp.MustCompile(`(?m)^func ([A-Z]\w+)\((?:ch Channel|s Stream|conn Stream)`).FindAllStringSubmatch(string(src), -1)
	if len(got) != len(want) {
		names := []string{}
		for _, m := range got {
			names = append(names, m[1])
		}
		t.Errorf("%d exported functions take a Channel or a Stream (%v) but %d are pinned above "+
			"— a new session entry point must either carry the verification gate or say why it cannot",
			len(got), names, len(want))
	}
}

// TestTheSessionCoreDoesNotNameATransport is P02.S04's acceptance clause as a guard.
//
// D14 keeps TCP beside QUIC, and two transports cannot share one core while the core
// names one of them in its signatures. So the rule is stronger than "the four entry
// points changed shape": the whole session and verification path must not mention
// *tls.Conn at all, and the package's entire TLS surface must be the one constructor
// that exists to bridge it.
func TestTheSessionCoreDoesNotNameATransport(t *testing.T) {
	for _, f := range []string{"session.go", "verify.go", "channel.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		// Stimulus first: this really is the file that carries the core. Without it the
		// absence below is equally true of an empty file or a renamed one.
		if !strings.Contains(string(src), "func ") {
			t.Fatalf("%s has no functions in it — the check below would pass on anything", f)
		}
		if strings.Contains(string(src), "*tls.Conn") {
			t.Errorf("%s names *tls.Conn. The session core takes a Channel; a transport type "+
				"in here is D14's two-transports-one-core rule broken at the signature.", f)
		}
	}
	// And the bridge is exactly one function, in the one file that is allowed to know
	// what a transport is — so there is a single place to read to learn how a Channel
	// comes into existence, and a single place S05 adds QUICChannel beside.
	src, err := os.ReadFile("transport.go")
	if err != nil {
		t.Fatal(err)
	}
	got := regexp.MustCompile(`(?m)^func ([A-Z]\w+)\(conn \*tls\.Conn`).FindAllStringSubmatch(string(src), -1)
	if len(got) != 1 || got[0][1] != "TLSChannel" {
		t.Errorf("transport.go exports %v taking a *tls.Conn, want exactly [TLSChannel]", got)
	}

	// And the WHOLE package builds a Channel in exactly as many places as there are
	// transports. Added at P02's phase close, because the graduation pass found the
	// check above claiming more reach than it had: it counts *tls.Conn functions in
	// ONE file, so P02.S05 added a second Channel constructor — `quicChannel`,
	// unexported, in quic.go — without it firing, which is precisely the deliberate
	// update the row said it would force.
	//
	// Every constructor here is a place PeerFP could come from something that was not
	// verified, so a third should be a decision somebody made on purpose.
	var all []byte
	for _, f := range []string{"transport.go", "quic.go", "channel.go", "session.go", "verify.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, b...)
	}
	ctors := regexp.MustCompile(`(?m)^func (\w+)\([^)]*\) \(Channel, error\)`).
		FindAllStringSubmatch(string(all), -1)
	if len(ctors) != len(transports) {
		names := []string{}
		for _, m := range ctors {
			names = append(names, m[1])
		}
		t.Errorf("the package builds a Channel in %d places (%v) but there are %d transports. "+
			"Each constructor is a place the peer's verified identity enters the core, so a new "+
			"one is either a new transport — put it in the table — or a way in that nothing tests.",
			len(ctors), names, len(transports))
	}
}

// TestAReconnectNeedsItsOwnConfirmation is D18's clause, driven rather than asserted.
//
// The clause reads "a confirmation computed on one channel is rejected on any other", which
// sounds like a replay test — and there is nothing to replay. A confirmation is not a
// token: it is a local boolean, and no bytes carry it. So the property that IS decidable,
// and is the one a user experiences, is that reconnecting does not inherit the last
// channel's answer: the same two identities on a second connection are asked again, and
// are shown DIFFERENT words to compare.
//
// **What this test can and cannot see.** It shows the user-visible property: reconnecting
// shows different words and asks again. It CANNOT say which input makes them differ —
// measured, by removing the channel binding from the derivation and watching this test stay
// green, because the fresh per-connection contributions make the strings differ on their
// own. The exporter's contribution is isolated in TestTheDerivationTakesEveryInput, which
// holds everything else constant; this test would be reporting a cause it cannot
// distinguish if it claimed otherwise.
func TestAReconnectNeedsItsOwnConfirmation(t *testing.T) {
	aCert, aKey := newIdentity(t)
	bCert, bKey := newIdentity(t)
	aFP, bFP := fingerprint(t, aCert), fingerprint(t, bCert)

	ln, err := Listen("127.0.0.1:0", bCert, bKey, aFP)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	// The SAME identities both times — a fresh pair would prove nothing about reconnecting.
	// Driven through runVerification rather than verificationExchange, so the GATE is
	// entered on each connection and the recorder counts real entries. (The first draft of
	// this test called a helper twice and asserted it had been called twice, which is an
	// assertion about the test's own loop.)
	rec := &recordingVerifier{}
	once := func() {
		done := make(chan struct{})
		go func() {
			defer close(done)
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
			_ = runVerification(conn.Channel, false, bFP, okVerifier{})
		}()
		conn, err := Dial(context.Background(), ln.Addr().String(), aCert, aKey, bFP, 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		if err := runVerification(conn.Channel, true, aFP, rec); err != nil {
			t.Fatal(err)
		}
		<-done
	}

	once()
	first := rec.last()
	once()
	second := rec.last()

	// The gate is entered once per connection, not cached across them.
	if rec.count() != 2 {
		t.Fatalf("the verifier was asked %d times across two connections, want 2 — a second "+
			"channel inherited the first one's answer", rec.count())
	}
	if first == "" || second == "" {
		t.Fatal("setup: an exchange produced no string, so nothing below is comparing channels")
	}
	if first == second {
		t.Errorf("both connections between the SAME two identities produced %q — so a "+
			"confirmation given on one connection describes the next one too, and a user could "+
			"be walked onto a substituted channel showing words they had already agreed to", first)
	}
}

// recordingVerifier confirms and remembers, so a test can compare what successive
// connections showed and count how often the gate was entered.
type recordingVerifier struct {
	mu    sync.Mutex
	words string
	calls int
}

func (r *recordingVerifier) ConfirmVerification(words string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.words = words
	r.calls++
	return true, nil
}

func (r *recordingVerifier) last() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.words
}

func (r *recordingVerifier) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

// okVerifier confirms without looking, for the far side of a test whose subject is
// something else.
type okVerifier struct{}

func (okVerifier) ConfirmVerification(string) (bool, error) { return true, nil }
