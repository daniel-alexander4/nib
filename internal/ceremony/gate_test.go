package ceremony

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
)

// gateFor builds a gate for A's records at testHop, with A in the roster.
func gateFor(t *testing.T) (*CandidateGate, Invitation, []byte, []byte, string) {
	t.Helper()
	_, inv := invited(t)
	certA, keyA, fpA := identity(t, "A")
	inv.Roster = append(inv.Roster, Party{Fingerprint: fpA, Signs: true})
	// The gate READS A's records, so A is the counterparty and the reader is somebody else.
	// The convener is the other end of the hop here — a hop needs two distinct parties and
	// the constructor now refuses a gate whose two ends are the same fingerprint.
	g, err := NewCandidateGate(inv, testHop, inv.ConvenerFingerprint, fpA)
	if err != nil {
		t.Fatal(err)
	}
	return g, inv, certA, keyA, fpA
}

// sealN makes a valid record from A carrying n candidates starting at `from`.
func sealN(t *testing.T, inv Invitation, certA, keyA []byte, fpA string, from, n int) []byte {
	t.Helper()
	var addrs []Endpoint
	for i := 0; i < n; i++ {
		addrs = append(addrs, ep("93.184.216."+itoa(from+i)+":34154"))
	}
	c := CandidateRecord{CeremonyID: inv.ID, Hop: testHop,
		Expires: time.Now().Add(10 * time.Minute), Addrs: addrs}
	if err := c.Sign(certA, keyA); err != nil {
		t.Fatal(err)
	}
	rk, err := inv.RecordKey(testHop)
	if err != nil {
		t.Fatal(err)
	}
	salt, err := inv.RecordSalt(testHop, fpA)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := c.Seal(rk, salt, testHop)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

// TestOneKeyYieldsNoMoreThanNAcrossTheWholeRace — the acceptance's real unit.
//
// A rendezvous key is a target that yields a SEQUENCE of records over D16's 300 s race —
// clock 1 says candidates trickle and the counterparty republishes as its gathering
// completes. So the per-record cap in parseCandidate, which is a message-validity rule,
// cannot see this: eight per record times however many records arrive is not eight.
func TestOneKeyYieldsNoMoreThanNAcrossTheWholeRace(t *testing.T) {
	g, inv, certA, keyA, fpA := gateFor(t)

	// Three VALID records, six candidates each, all distinct: 18 legitimate candidates
	// arriving over one race from the real counterparty.
	for i := 0; i < 3; i++ {
		if err := g.Accept(sealN(t, inv, certA, keyA, fpA, 1+i*6, 6), time.Now()); err != nil {
			t.Fatalf("record %d was refused: %v", i, err)
		}
	}
	// STIMULUS: more arrived than the cap, so the cap has something to do.
	if got := g.Stats().Accepted + g.Stats().DroppedOverCap; got != 18 {
		t.Fatalf("setup: %d candidates offered, want 18 — the cap has nothing to bound", got)
	}
	if got := len(g.Candidates()); got != MaxCandidates {
		t.Errorf("the key yielded %d candidates across the race, cap is %d", got, MaxCandidates)
	}
	if got := g.Stats().DroppedOverCap; got != 18-MaxCandidates {
		t.Errorf("DroppedOverCap = %d, want %d — the drop is the acceptance's 'reported'",
			got, 18-MaxCandidates)
	}
}

// TestAnOverCapRecordIsRefusedWhole — N+50, driven at the parse boundary.
//
// The plan says "driven by publishing N+50". Measured, that cannot be published: 58
// candidates seal to 1,873 bytes against BEP-44's 996-byte ceiling, so it is refused before
// a datagram leaves — and `Sign` will not build one either. So the stimulus is injected
// where an attacker's bytes actually arrive: the parse.
func TestAnOverCapRecordIsRefusedWhole(t *testing.T) {
	g, inv, certA, keyA, fpA := gateFor(t)

	// Hand-build the plaintext: MaxCandidates+1 addresses, framed exactly as preimage()
	// does, because preimage() itself refuses to produce this.
	//
	// **+1, not +50.** The +50 version sealed to 1,859 bytes — past MaxSealedRecord and
	// past BEP-44's own 1000-byte value cap, so it is a record no peer could ever deliver.
	// Once the read path grew its own size ceiling (v1.116.6) that record was refused as
	// TOO BIG before the candidate cap was reached, and this test failed. It was measuring
	// the cap through a stimulus the cap would never see. +1 is the shape an attacker
	// actually has: a record that fits the wire and carries one candidate too many.
	spki, err := signCandidateSPKIForTest(t, certA, keyA)
	if err != nil {
		t.Fatal(err)
	}
	n := MaxCandidates + 1
	var p preimageBuilder
	p.addString(candidateDomain)
	p.addUint(uint64(CandidateFormatVersion))
	p.addString(inv.ID)
	p.addUint(uint64(testHop))
	p.add(spki)
	p.addString(time.Now().Add(time.Minute).UTC().Format(time.RFC3339))
	p.addUint(uint64(n))
	for i := 0; i < n; i++ {
		p.addString("93.184.216." + itoa(1+i%200) + ":34154")
	}
	var l [8]byte
	binary.BigEndian.PutUint64(l[:], 8)
	plain := append(p.bytes(), l[:]...)
	plain = append(plain, make([]byte, 8)...)

	rk, err := inv.RecordKey(testHop)
	if err != nil {
		t.Fatal(err)
	}
	salt, err := inv.RecordSalt(testHop, fpA)
	if err != nil {
		t.Fatal(err)
	}
	aead, err := chacha20poly1305.NewX(rk)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	sealed := aead.Seal(nonce, nonce, plain, candidateAAD(salt, testHop))

	// STIMULUS: it decrypts. Without this the refusal below could be an AEAD failure and
	// would say nothing about the cap.
	if _, err := aead.Open(nil, nonce, sealed[chacha20poly1305.NonceSizeX:], candidateAAD(salt, testHop)); err != nil {
		t.Fatalf("setup: the hand-built record does not decrypt, so the cap is untested: %v", err)
	}

	err = g.Accept(sealed, time.Now())
	if !errors.Is(err, ErrCandidateTooMany) {
		t.Fatalf("a record of %d candidates was not refused as over-cap: %v", n, err)
	}
	if g.Stats().RefusedTooMany != 1 {
		t.Errorf("RefusedTooMany = %d, want 1 — the cause must be countable, and it was "+
			"fused with wrong-key ciphertext until this slice split it", g.Stats().RefusedTooMany)
	}
	// Refused WHOLE: not one candidate survives.
	if got := len(g.Candidates()); got != 0 {
		t.Errorf("an over-cap record contributed %d candidates — keeping a prefix acts on a "+
			"statement the signer never made, and the attacker orders the list", got)
	}
}

func signCandidateSPKIForTest(t *testing.T, certPEM, keyPEM []byte) ([]byte, error) {
	t.Helper()
	c := CandidateRecord{CeremonyID: "x", Hop: 0, Expires: time.Now().Add(time.Minute),
		Addrs: []Endpoint{ep("93.184.216.34:34154")}}
	if err := c.Sign(certPEM, keyPEM); err != nil {
		return nil, err
	}
	return c.SPKI, nil
}

// TestARepeatWITHINARecordIsNotTheSameFactAsARefetch.
//
// Both were counted as "duplicate" until this slice's review. They mean opposite things:
// a repeat inside one record is an attack shape no honest publisher emits (eight copies of
// one address concentrate the whole per-candidate punch budget on one victim), while the
// same record arriving again is what BEP-44 does on every fetch of a 300 s race — roughly
// ten times. Conflating them makes the attack indicator read high on every healthy
// ceremony, and an alarm that is always on is not an alarm.
func TestARepeatWithinARecordIsNotTheSameFactAsARefetch(t *testing.T) {
	g, inv, certA, keyA, fpA := gateFor(t)

	// (a) the ordinary path: one record, fetched four times.
	rec := sealN(t, inv, certA, keyA, fpA, 1, 3)
	for i := 0; i < 4; i++ {
		if err := g.Accept(rec, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(g.Candidates()); got != 3 {
		t.Errorf("the same record four times yielded %d candidates, want 3", got)
	}
	if g.Stats().Reoffered != 9 {
		t.Errorf("Reoffered = %d, want 9 — a re-fetch is the ordinary case", g.Stats().Reoffered)
	}
	if g.Stats().DroppedDuplicate != 0 {
		t.Errorf("DroppedDuplicate = %d after four honest fetches — the attack indicator is "+
			"firing on the healthy path", g.Stats().DroppedDuplicate)
	}

	// (b) the attack shape: one record naming the same address repeatedly.
	g2, inv2, certB, keyB, fpB := gateFor(t)
	victim := ep("93.184.216.55:34154")
	c := CandidateRecord{CeremonyID: inv2.ID, Hop: testHop, Expires: time.Now().Add(time.Minute),
		Addrs: []Endpoint{victim, victim, victim, victim}}
	if err := c.Sign(certB, keyB); err != nil {
		t.Fatal(err)
	}
	rk, err := inv2.RecordKey(testHop)
	if err != nil {
		t.Fatal(err)
	}
	salt, err := inv2.RecordSalt(testHop, fpB)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := c.Seal(rk, salt, testHop)
	if err != nil {
		t.Fatal(err)
	}
	if err := g2.Accept(sealed, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := len(g2.Candidates()); got != 1 {
		t.Errorf("four copies of one address yielded %d candidates, want 1", got)
	}
	if g2.Stats().DroppedDuplicate != 3 {
		t.Errorf("DroppedDuplicate = %d, want 3 — this is the shape that aims the whole "+
			"budget at one victim", g2.Stats().DroppedDuplicate)
	}
	if g2.Stats().Reoffered != 0 {
		t.Errorf("Reoffered = %d on a single record", g2.Stats().Reoffered)
	}
}

// TestAValidRecordWithNoCandidatesIsVisible.
//
// It moves neither Accepted nor any refusal, and rendezvous's FetchEmpty does not move
// either because something WAS there — so without its own counter it is invisible in the
// instrument built to tell "present and wrong" from "absent".
func TestAValidRecordWithNoCandidatesIsVisible(t *testing.T) {
	g, inv, certA, keyA, fpA := gateFor(t)
	c := CandidateRecord{CeremonyID: inv.ID, Hop: testHop, Expires: time.Now().Add(time.Minute)}
	if err := c.Sign(certA, keyA); err != nil {
		t.Fatal(err)
	}
	rk, err := inv.RecordKey(testHop)
	if err != nil {
		t.Fatal(err)
	}
	salt, err := inv.RecordSalt(testHop, fpA)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := c.Seal(rk, salt, testHop)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Accept(sealed, time.Now()); err != nil {
		t.Fatalf("an empty candidate list was refused: %v", err)
	}
	st := g.Stats()
	if st.EmptyRecords != 1 {
		t.Errorf("EmptyRecords = %d, want 1 — otherwise this record moves nothing at all "+
			"and is invisible to the very readout built to see it", st.EmptyRecords)
	}
	if st.Accepted != 0 || st.Refused() != 0 {
		t.Errorf("an empty record moved another counter: %+v", st)
	}
}

// TestADuplicateIsAttributedToDuplicationEvenWhenTheSetIsFull — the order of the two
// checks is a judgment, and nothing recorded it.
func TestADuplicateIsAttributedToDuplicationWhenFull(t *testing.T) {
	g, inv, certA, keyA, fpA := gateFor(t)
	if err := g.Accept(sealN(t, inv, certA, keyA, fpA, 1, MaxCandidates), time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(g.Candidates()) != MaxCandidates {
		t.Fatal("setup: the set is not full, so the ordering is untested")
	}
	before := g.Stats()
	// Re-offer an address already held, while the set is full. It is a re-offer, not an
	// over-cap drop: fullness was not the reason it was not added.
	if err := g.Accept(sealN(t, inv, certA, keyA, fpA, 1, 1), time.Now()); err != nil {
		t.Fatal(err)
	}
	after := g.Stats()
	if after.Reoffered != before.Reoffered+1 {
		t.Errorf("a re-offer while full was not counted as a re-offer (%d -> %d)",
			before.Reoffered, after.Reoffered)
	}
	if after.DroppedOverCap != before.DroppedOverCap {
		t.Errorf("a re-offer while full was counted as an over-cap drop — fullness was not " +
			"why it was skipped, and the two counters advise differently")
	}
}

// TestEveryRefusalCauseIsCountedSeparately — and their sum is the preemption signal.
//
// P04.S03 filed this: "a record was present and failed the inner check" must be
// distinguishable from "nothing was there", or in-roster preemption looks exactly like an
// offline peer. The sum of these counters IS that fact; rendezvous.Stats().FetchEmpty is
// the other half.
func TestEveryRefusalCauseIsCountedSeparately(t *testing.T) {
	g, inv, certA, keyA, fpA := gateFor(t)
	certB, keyB, fpB := identity(t, "B")
	g.inv.Roster = append(g.inv.Roster, Party{Fingerprint: fpB, Signs: true})

	// wrong key -> Sealed
	if err := g.Accept(make([]byte, 80), time.Now()); !errors.Is(err, ErrCandidateSealed) {
		t.Fatalf("garbage was not refused as unsealed: %v", err)
	}
	// a valid record from B at A's salt -> Author (the in-roster forgery)
	rk, _ := inv.RecordKey(testHop)
	saltA, _ := inv.RecordSalt(testHop, fpA)
	cb := CandidateRecord{CeremonyID: inv.ID, Hop: testHop, Expires: time.Now().Add(time.Minute),
		Addrs: []Endpoint{ep("93.184.216.7:34154")}}
	if err := cb.Sign(certB, keyB); err != nil {
		t.Fatal(err)
	}
	forged, err := cb.Seal(rk, saltA, testHop)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Accept(forged, time.Now()); !errors.Is(err, ErrCandidateAuthor) {
		t.Fatalf("an in-roster forgery was not refused as such: %v", err)
	}
	// expired -> Expired
	ce := CandidateRecord{CeremonyID: inv.ID, Hop: testHop, Expires: time.Now().Add(-time.Minute),
		Addrs: []Endpoint{ep("93.184.216.8:34154")}}
	if err := ce.Sign(certA, keyA); err != nil {
		t.Fatal(err)
	}
	stale, err := ce.Seal(rk, saltA, testHop)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Accept(stale, time.Now()); !errors.Is(err, ErrCandidateExpired) {
		t.Fatalf("a stale record was not refused as expired: %v", err)
	}

	st := g.Stats()
	if st.RefusedSealed != 1 || st.RefusedAuthor != 1 || st.RefusedExpired != 1 {
		t.Errorf("causes were not counted separately: %+v", st)
	}
	if st.Refused() != 3 {
		t.Errorf("Refused() = %d, want 3 — this sum is what makes in-roster preemption "+
			"distinguishable from an offline peer", st.Refused())
	}
	if st.Accepted != 0 {
		t.Errorf("a refused record contributed %d candidates", st.Accepted)
	}
}

// TestABogonCandidateIsRefused — measured against the list that got through before.
func TestABogonCandidateIsRefused(t *testing.T) {
	_, inv := invited(t)
	certA, keyA, fpA := identity(t, "A")
	for _, bad := range []string{
		"127.0.0.1:22", "[::1]:53", "192.168.1.1:53", "10.0.0.1:11211",
		"224.0.0.1:5353", "255.255.255.255:9", "100.64.0.1:123", "1.2.3.4:0",
		"93.184.216.34:53",         // routable host, system port — the reflection case
		"[::ffff:240.0.0.1]:34154", // v4-mapped reserved: clears a v4 prefix loop unless unmapped
	} {
		c := CandidateRecord{CeremonyID: inv.ID, Hop: testHop,
			Expires: time.Now().Add(time.Minute),
			Addrs:   []Endpoint{ep(bad)}}
		if err := c.Sign(certA, keyA); !errors.Is(err, ErrCandidateUnroutable) {
			t.Errorf("%s: Sign returned %v, want ErrCandidateUnroutable — Nib would publish "+
				"an address it will not dial", bad, err)
		}
	}
	_ = fpA
}

// TestAnOversizeRosterIsRefused — D33's third figure, unenforced until this slice.
func TestAnOversizeRosterIsRefused(t *testing.T) {
	_, inv := invited(t)
	big := inv
	big.Roster = nil
	for i := 0; i <= MaxRoster; i++ {
		big.Roster = append(big.Roster, Party{
			Fingerprint: strings.Repeat("ab", 32), Signs: true,
		})
	}
	text, err := big.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseInvitation(text); !errors.Is(err, ErrInvitationCorrupt) {
		t.Fatalf("an invitation naming %d parties gave %v — that is %d hops, and the punch "+
			"budget is now per hop", len(big.Roster), err, len(big.Roster)-1)
	}
	// And the ordinary size still parses.
	ok := inv
	if _, err := ParseInvitation(mustEncode(t, ok)); err != nil {
		t.Fatalf("an ordinary roster was refused by the cap: %v", err)
	}
}

func mustEncode(t *testing.T, i Invitation) string {
	t.Helper()
	s, err := i.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// sealRaw seals a hand-built plaintext, bypassing Sign/Seal's own gates — which is the
// only way to produce what an attacker sends, since the constructors refuse to build it.
func sealRaw(t *testing.T, inv Invitation, fp string, plain []byte) []byte {
	t.Helper()
	rk, err := inv.RecordKey(testHop)
	if err != nil {
		t.Fatal(err)
	}
	salt, err := inv.RecordSalt(testHop, fp)
	if err != nil {
		t.Fatal(err)
	}
	aead, err := chacha20poly1305.NewX(rk)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	return aead.Seal(nonce, nonce, plain, candidateAAD(salt, testHop))
}

// rawPlaintext frames a record exactly as preimage() does, with a caller-chosen address
// list and signature — so a test can produce what preimage() refuses to.
func rawPlaintext(t *testing.T, inv Invitation, spki []byte, addrs []string, sig []byte, expires time.Time) []byte {
	t.Helper()
	var p preimageBuilder
	p.addString(candidateDomain)
	p.addUint(uint64(CandidateFormatVersion))
	p.addString(inv.ID)
	p.addUint(uint64(testHop))
	p.add(spki)
	p.addString(expires.UTC().Format(time.RFC3339))
	p.addUint(uint64(len(addrs)))
	for _, a := range addrs {
		p.addString(a)
		// The transport chunk, interleaved, exactly as preimage() writes it. Restated by
		// hand ON PURPOSE: this helper exists so a test can build a plaintext WITHOUT the
		// function under test. Sealing with preimage() would move both sides together,
		// which is how the original defect hid (see the note on TestSealRefusesAModified…).
		p.addUint(uint64(TransportTCP))
	}
	var l [8]byte
	binary.BigEndian.PutUint64(l[:], uint64(len(sig)))
	out := append(p.bytes(), l[:]...)
	return append(out, sig...)
}

// TestABogonCandidateIsRefusedONTHEREADPATH.
//
// The write-side test above drives `Sign`, which is the door an attacker never comes
// through. Deleting the read-path check left the ENTIRE REPO green — measured — so this is
// the assertion the criterion actually needs: the criterion is about a *received* record.
func TestABogonCandidateIsRefusedOnTheReadPath(t *testing.T) {
	for _, bad := range []string{
		"127.0.0.1:22", "192.168.1.1:53", "10.0.0.1:11211", "224.0.0.1:5353",
		"100.64.0.1:123", "[::c0a8:101]:34154", // v4-COMPATIBLE v6: the second family hole
		"93.184.216.34:53", // routable host, reflection port
	} {
		t.Run(bad, func(t *testing.T) {
			g, inv, certA, keyA, fpA := gateFor(t)
			spki, err := signCandidateSPKIForTest(t, certA, keyA)
			if err != nil {
				t.Fatal(err)
			}
			plain := rawPlaintext(t, inv, spki, []string{bad}, make([]byte, 8), time.Now().Add(time.Minute))
			err = g.Accept(sealRaw(t, inv, fpA, plain), time.Now())
			if !errors.Is(err, ErrCandidateUnroutable) {
				t.Fatalf("a received record naming %s was not refused as unroutable: %v", bad, err)
			}
			if g.Stats().RefusedUnroutable != 1 {
				t.Errorf("RefusedUnroutable = %d, want 1", g.Stats().RefusedUnroutable)
			}
			if len(g.Candidates()) != 0 {
				t.Errorf("%s reached the race set", bad)
			}
		})
	}
}

// TestABrokenSignatureIsCountedAsASignatureFailure — the arm was undriven.
func TestABrokenSignatureIsCountedAsASignatureFailure(t *testing.T) {
	g, inv, certA, keyA, fpA := gateFor(t)
	spki, err := signCandidateSPKIForTest(t, certA, keyA)
	if err != nil {
		t.Fatal(err)
	}
	// A well-formed record whose signature is simply wrong.
	plain := rawPlaintext(t, inv, spki, []string{"93.184.216.34:34154"},
		make([]byte, 71), time.Now().Add(time.Minute))
	err = g.Accept(sealRaw(t, inv, fpA, plain), time.Now())
	if !errors.Is(err, ErrCandidateSignature) {
		t.Fatalf("a bad signature was not refused as one: %v", err)
	}
	if g.Stats().RefusedSignature != 1 {
		t.Errorf("RefusedSignature = %d, want 1 — without this arm the cause is filed as "+
			"'the peer's build is broken'", g.Stats().RefusedSignature)
	}
}

// TestACrossCeremonyReplayIsCountedAsAContextFailure — the arm was undriven, and the
// attack is real: a roster member of two ceremonies re-seals A's record from one into the
// other. Everything passes except the ceremony id.
func TestACrossCeremonyReplayIsCountedAsAContextFailure(t *testing.T) {
	_, one := invited(t)
	_, two := invited(t)
	certA, keyA, fpA := identity(t, "A")
	one.Roster = append(one.Roster, Party{Fingerprint: fpA, Signs: true})
	two.Roster = append(two.Roster, Party{Fingerprint: fpA, Signs: true})

	c := signedCandidate(t, one, certA, keyA, 1) // ceremony ONE's id inside
	rk2, err := two.RecordKey(testHop)
	if err != nil {
		t.Fatal(err)
	}
	salt2, err := two.RecordSalt(testHop, fpA)
	if err != nil {
		t.Fatal(err)
	}
	moved, err := c.Seal(rk2, salt2, testHop) // re-sealed under ceremony TWO
	if err != nil {
		t.Fatal(err)
	}
	g, err := NewCandidateGate(two, testHop, two.ConvenerFingerprint, fpA)
	if err != nil {
		t.Fatal(err)
	}
	err = g.Accept(moved, time.Now())
	if !errors.Is(err, ErrCandidateContext) {
		t.Fatalf("a cross-ceremony replay was not refused as a context failure: %v", err)
	}
	if g.Stats().RefusedContext != 1 {
		t.Errorf("RefusedContext = %d, want 1 — filed as RefusedFormat it reads as 'the "+
			"peer's build is broken' instead of 'a roster member is replaying across "+
			"ceremonies'", g.Stats().RefusedContext)
	}
}

// TestAHostileDocumentCannotCarryAnUnboundedRoster.
//
// The cap was on ParseInvitation only, so it bound the RECIPIENTS and not the convener —
// and the convener is the party that dials every hop and emits every packet. A Record
// arrives from external input (Extract → Decode → Verify) and the convener never parses
// its own invitation.
func TestAHostileDocumentCannotCarryAnUnboundedRoster(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	r := draft(t, cfp)
	for i := 0; i <= MaxRoster; i++ {
		r.Roster = append(r.Roster, Party{Fingerprint: strings.Repeat("cd", 32), Signs: true})
	}
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	// STIMULUS: it is a genuinely well-formed, correctly self-signed record. Only the
	// length is wrong, so nothing else can be the reason it is refused.
	if len(r.Roster) <= MaxRoster {
		t.Fatal("setup: the roster is not oversize")
	}
	err := r.Verify(time.Now())
	if err == nil {
		t.Fatalf("a self-signed record naming %d parties verified — that is %d hops of "+
			"punch budget and %d signature pages", len(r.Roster), len(r.Roster)-1, len(r.Roster))
	}
	if !strings.Contains(err.Error(), "limit") {
		t.Fatalf("refused for the wrong reason: %v", err)
	}
}

// TestAnOverSizeSealedRecordIsRefusedAtTheRead.
//
// `MaxSealedRecord` was a producer-only bound: `Seal` refused over 996 bytes and `Accept`
// handed a slice of any length to `aead.Open`, which allocates `len(ct)`. The only thing
// bounding it was anacrolix/dht's bencode check — in a package L1 forbids this one from
// importing, so the ceiling was a hope about a transitive dependency rather than a property
// of this package.
func TestAnOverSizeSealedRecordIsRefusedAtTheRead(t *testing.T) {
	g, _, _, _, _ := gateFor(t)

	// Well-formed enough to reach the AEAD, and far past the ceiling.
	huge := make([]byte, MaxSealedRecord*64)
	if _, err := rand.Read(huge[:64]); err != nil {
		t.Fatal(err)
	}
	err := g.Accept(huge, time.Now())
	if !errors.Is(err, ErrCandidateTooBig) {
		t.Fatalf("a %d-byte record was not refused at the read: %v", len(huge), err)
	}
	// RefusedTooBig, not RefusedSealed. It was the latter until 2026-08-20, and that
	// counter means "wrong key/salt/hop, or altered" — a tampering signal for a record
	// nothing has decrypted. The breakdown is read to tell in-roster preemption from noise.
	if g.Stats().RefusedTooBig != 1 {
		t.Errorf("RefusedTooBig = %d, want 1 — an over-size record must be countable",
			g.Stats().RefusedTooBig)
	}
	if g.Stats().RefusedSealed != 0 {
		t.Errorf("RefusedSealed = %d, want 0 — an over-size record was never opened",
			g.Stats().RefusedSealed)
	}

	// The boundary, and it is the assertion that stops the ceiling being "refuse
	// everything": a record exactly at the cap must still be admitted to the AEAD. It
	// fails there (these are random bytes), which is a DIFFERENT refusal — and that is the
	// point: the size gate did not swallow it.
	atCap := make([]byte, MaxSealedRecord)
	if _, err := rand.Read(atCap); err != nil {
		t.Fatal(err)
	}
	if err := g.Accept(atCap, time.Now()); errors.Is(err, ErrCandidateTooBig) {
		t.Error("a record exactly at MaxSealedRecord was refused as over-cap")
	}
}

// TestEveryRefusalCauseIsCountedUnderItsOwnName drives each cause to a distinct counter, and
// asserts the sum. The one it was written for is the oversize case: it incremented
// RefusedSealed, whose documented meaning is "wrong key/salt/hop, or altered" — a tampering
// signal — for a record that had not been decrypted at all. The breakdown exists so an
// operator can tell in-roster preemption from noise, and that is the reading it made wrong.
func TestEveryRefusalCauseIsCountedUnderItsOwnName(t *testing.T) {
	g, _, _, _, _ := gateFor(t)

	before := g.Stats()
	if err := g.Accept(make([]byte, MaxSealedRecord+1), time.Now()); err == nil {
		t.Fatal("an oversize record was accepted, so this test never reaches its subject")
	}
	after := g.Stats()
	if after.RefusedTooBig != before.RefusedTooBig+1 {
		t.Errorf("an oversize record did not increment RefusedTooBig (%d → %d)",
			before.RefusedTooBig, after.RefusedTooBig)
	}
	if after.RefusedSealed != before.RefusedSealed {
		t.Errorf("an oversize record incremented RefusedSealed (%d → %d) — that counter means "+
			"'wrong key/salt/hop, or altered', and nothing here has been decrypted",
			before.RefusedSealed, after.RefusedSealed)
	}

	// And a genuinely unsealable record still lands on RefusedSealed, or the split above
	// has simply moved the problem.
	junk := make([]byte, 128)
	for i := range junk {
		junk[i] = byte(i)
	}
	mid := g.Stats()
	if err := g.Accept(junk, time.Now()); err == nil {
		t.Fatal("random bytes opened as a sealed record")
	}
	if g.Stats().RefusedSealed != mid.RefusedSealed+1 {
		t.Errorf("an unsealable record did not increment RefusedSealed")
	}

	// The sum is the number the CLI prints beside an empty fetch, so it must include the
	// new cause. A field added to the struct and forgotten in Refused() is invisible.
	s := g.Stats()
	if s.Refused() < s.RefusedTooBig+s.RefusedSealed {
		t.Errorf("Refused() = %d does not include every cause it was incremented for "+
			"(too-big %d + sealed %d)", s.Refused(), s.RefusedTooBig, s.RefusedSealed)
	}
}

// TestAHopHasTwoSaltsAndTheyAreNotInterchangeable — the defect the only in-tree example
// could not have caught, because that example was a one-party ceremony.
//
// A hop has two parties and BOTH publish. `RecordSalt` is per party precisely so the two do
// not share a BEP-44 target, where the higher-seq write would silently clobber the other. So
// each side must SEAL at its own salt and READ at its counterparty's — and until PublishSalt
// existed, the only salt the gate exposed was the read salt, so the only value a caller
// could reach for was the wrong one.
//
// **Why nothing caught it.** `nib rendezvous --self-test` publishes at `gate.Salt()` and
// fetches at `gate.Salt()`, which is coherent only because its counterparty was itself. Its
// two salts were the same 32 bytes. Copied to a real ceremony each side writes where nobody
// is looking and reads where nobody wrote — and the symptom is `ErrCandidateSealed`, "this
// record was not written for this ceremony, or has been altered", which accuses a
// counterparty of tampering over a local wiring mistake.
func TestAHopHasTwoSaltsAndTheyAreNotInterchangeable(t *testing.T) {
	g, inv, certA, keyA, fpA := gateFor(t)

	// SETUP: the two salts really are different values. If they were equal — which is
	// exactly the one-party case — every assertion below would hold with the salts swapped
	// and the test would prove nothing.
	if string(g.Salt()) == string(g.PublishSalt()) {
		t.Fatal("setup: this gate's read and publish salts are identical, so it is a " +
			"one-party ceremony and cannot distinguish the two")
	}

	rk, err := inv.RecordKey(testHop)
	if err != nil {
		t.Fatal(err)
	}
	rec := CandidateRecord{
		CeremonyID: inv.ID, Hop: testHop,
		Expires: time.Now().Add(time.Minute),
		Addrs:   []Endpoint{ep("93.184.216.34:34154")},
	}
	if err := rec.Sign(certA, keyA); err != nil {
		t.Fatal(err)
	}

	// A publishes at A's OWN salt. This gate reads A's records, so A's own salt is exactly
	// what this gate's `Salt()` returns — the gate belongs to the other end of the hop.
	right, err := rec.Seal(rk, g.Salt(), testHop)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Accept(right, time.Now()); err != nil {
		t.Fatalf("a record sealed at the salt this gate reads was refused: %v", err)
	}

	// And sealed at the WRONG salt — the mistake the API used to make the easy one — it is
	// refused as sealed, which is the misleading sentence. This asserts the failure mode so
	// that the diagnosis is on record: if somebody later reports "the peer's records are all
	// refused as tampered", this is what it means.
	g2, err := NewCandidateGate(inv, testHop, inv.ConvenerFingerprint, fpA)
	if err != nil {
		t.Fatal(err)
	}
	wrong, err := rec.Seal(rk, g2.PublishSalt(), testHop)
	if err != nil {
		t.Fatal(err)
	}
	err = g2.Accept(wrong, time.Now())
	if !errors.Is(err, ErrCandidateSealed) {
		t.Errorf("a record sealed at the publisher's own salt and read at the same gate was "+
			"refused as %v; the AEAD binds the salt, so this must fail — and it failing as "+
			"SEALED is the whole reason PublishSalt exists, because that sentence accuses a "+
			"counterparty of tampering over a local mistake", err)
	}
}

// TestAHopNeedsTwoDistinctParties — the constructor refuses a gate whose two ends are the
// same fingerprint.
//
// Without it, a caller that computed the hop wrong builds a gate where every salt, key and
// target is self-consistent — so nothing fails, and the symptom is a counterparty who
// appears never to publish. That is indistinguishable from an offline peer, which is the
// most expensive thing a connectivity bug can look like.
func TestAHopNeedsTwoDistinctParties(t *testing.T) {
	_, inv := invited(t)
	_, _, fpA := identity(t, "A")
	inv.Roster = append(inv.Roster, Party{Fingerprint: fpA, Signs: true})

	// SETUP: the same call with two DIFFERENT parties succeeds, so the refusal below is a
	// bound and not a broken constructor.
	if _, err := NewCandidateGate(inv, testHop, inv.ConvenerFingerprint, fpA); err != nil {
		t.Fatalf("setup: a two-party gate was refused (%v), so the refusal below proves "+
			"nothing about the same-party case", err)
	}

	if _, err := NewCandidateGate(inv, testHop, fpA, fpA); err == nil {
		t.Error("a gate was built with one fingerprint as both ends of the hop. Every salt, " +
			"key and target it derives would be self-consistent, so nothing would fail — the " +
			"symptom is a counterparty who never publishes, which reads as an offline peer.")
	}
}
