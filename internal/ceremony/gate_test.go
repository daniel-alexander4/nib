package ceremony

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"net/netip"
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
	g, err := NewCandidateGate(inv, testHop, fpA)
	if err != nil {
		t.Fatal(err)
	}
	return g, inv, certA, keyA, fpA
}

// sealN makes a valid record from A carrying n candidates starting at `from`.
func sealN(t *testing.T, inv Invitation, certA, keyA []byte, fpA string, from, n int) []byte {
	t.Helper()
	var addrs []netip.AddrPort
	for i := 0; i < n; i++ {
		addrs = append(addrs, netip.MustParseAddrPort("93.184.216."+itoa(from+i)+":34154"))
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

	// Hand-build the plaintext: MaxCandidates+50 addresses, framed exactly as preimage()
	// does, because preimage() itself refuses to produce this.
	spki, err := signCandidateSPKIForTest(t, certA, keyA)
	if err != nil {
		t.Fatal(err)
	}
	n := MaxCandidates + 50
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
		Addrs: []netip.AddrPort{netip.MustParseAddrPort("93.184.216.34:34154")}}
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
	victim := netip.MustParseAddrPort("93.184.216.55:34154")
	c := CandidateRecord{CeremonyID: inv2.ID, Hop: testHop, Expires: time.Now().Add(time.Minute),
		Addrs: []netip.AddrPort{victim, victim, victim, victim}}
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
		Addrs: []netip.AddrPort{netip.MustParseAddrPort("93.184.216.7:34154")}}
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
		Addrs: []netip.AddrPort{netip.MustParseAddrPort("93.184.216.8:34154")}}
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
			Addrs:   []netip.AddrPort{netip.MustParseAddrPort(bad)}}
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
	g, err := NewCandidateGate(two, testHop, fpA)
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
	err := r.Verify()
	if err == nil {
		t.Fatalf("a self-signed record naming %d parties verified — that is %d hops of "+
			"punch budget and %d signature pages", len(r.Roster), len(r.Roster)-1, len(r.Roster))
	}
	if !strings.Contains(err.Error(), "limit") {
		t.Fatalf("refused for the wrong reason: %v", err)
	}
}
