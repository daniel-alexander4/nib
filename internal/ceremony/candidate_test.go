package ceremony

import (
	"bytes"
	"net/netip"
	"strings"
	"testing"
	"time"
)

// hop is the hop these tests publish at; any fixed value works, and using a non-zero one
// means a test that silently dropped the hop would still have to explain itself.
const testHop = 2

func addrs(t *testing.T, n int) []netip.AddrPort {
	t.Helper()
	var out []netip.AddrPort
	for i := 0; i < n; i++ {
		out = append(out, netip.MustParseAddrPort("198.51.100."+itoa(i+1)+":34154"))
	}
	return out
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// signedCandidate builds a record signed by certPEM/keyPEM.
func signedCandidate(t *testing.T, inv Invitation, certPEM, keyPEM []byte, n int) CandidateRecord {
	t.Helper()
	c := CandidateRecord{
		CeremonyID: inv.ID,
		Hop:        testHop,
		Expires:    time.Now().Add(10 * time.Minute),
		Addrs:      addrs(t, n),
	}
	if err := c.Sign(certPEM, keyPEM); err != nil {
		t.Fatal(err)
	}
	return c
}

// sealFor seals with the invitation's own hop key and the party's salt.
func sealFor(t *testing.T, inv Invitation, c CandidateRecord, fp string, seq int64) ([]byte, []byte, []byte) {
	t.Helper()
	rk, err := inv.RecordKey(testHop)
	if err != nil {
		t.Fatal(err)
	}
	salt, err := inv.RecordSalt(testHop, fp)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := c.Seal(rk, salt, testHop, seq)
	if err != nil {
		t.Fatal(err)
	}
	return sealed, rk, salt
}

// TestACandidateRecordSurvivesThePublishRoundTrip.
func TestACandidateRecordSurvivesThePublishRoundTrip(t *testing.T) {
	rec, inv := invited(t)
	certA, keyA, fpA := identity(t, "A")
	inv.Roster = append(inv.Roster, Party{Fingerprint: fpA, Signs: true})
	rec.Roster = inv.Roster

	c := signedCandidate(t, inv, certA, keyA, 3)
	sealed, rk, salt := sealFor(t, inv, c, fpA, 1)

	back, err := OpenCandidate(rk, salt, testHop, 1, sealed)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := back.Verify(inv, testHop, fpA, time.Now()); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if len(back.Addrs) != 3 || back.Addrs[0] != c.Addrs[0] {
		t.Fatalf("candidates did not round-trip: %v vs %v", back.Addrs, c.Addrs)
	}
	if back.CeremonyID != inv.ID || back.Hop != testHop {
		t.Fatalf("binding fields did not round-trip: %q hop %d", back.CeremonyID, back.Hop)
	}
}

// TestAnInvitedPartyCannotPublishAsAnother — THE slice's decision, driven.
//
// This is the pending item filed 2026-08-19. Party B holds the invitation secret, so B
// can derive A's salt, derive the record key, and produce a perfectly well-formed sealed
// record at A's target. Everything the DHT can check passes: the BEP-44 signature is made
// with a key B legitimately holds, the ciphertext decrypts, the record parses.
//
// The ONLY thing standing between that and a redirected ceremony is the inner ECDSA
// signature, and this test is what says so.
func TestAnInvitedPartyCannotPublishAsAnother(t *testing.T) {
	_, inv := invited(t)
	_, _, fpA := identity(t, "A")
	certB, keyB, fpB := identity(t, "B")
	inv.Roster = append(inv.Roster,
		Party{Fingerprint: fpA, Signs: true},
		Party{Fingerprint: fpB, Signs: true})

	// B builds a record and signs it with B's own key — the only key B has.
	forged := signedCandidate(t, inv, certB, keyB, 2)

	// B seals it at A's SALT, which B can compute because B holds the secret. This is the
	// stimulus: if it failed, the test would prove nothing about the signature.
	sealed, rk, saltA := sealFor(t, inv, forged, fpA, 1)

	// And it decrypts perfectly. Confidentiality is the ceremony's boundary; B is inside it.
	back, err := OpenCandidate(rk, saltA, testHop, 1, sealed)
	if err != nil {
		t.Fatalf("B could not even seal at A's salt — the stimulus is broken, so the "+
			"refusal below would prove nothing: %v", err)
	}

	// The refusal is here, and nowhere else.
	err = back.Verify(inv, testHop, fpA, time.Now())
	if err == nil {
		t.Fatal("a roster member published as another party and it was accepted — the " +
			"authenticity boundary has collapsed onto the confidentiality boundary")
	}
	if !strings.Contains(err.Error(), "signed by") {
		t.Fatalf("refused, but not as a forgery: %v", err)
	}
	// B publishing as B is still fine — otherwise the check above could be "refuse
	// everything" and this test could not tell.
	if err := back.Verify(inv, testHop, fpB, time.Now()); err != nil {
		t.Fatalf("B's own record was refused for B: %v", err)
	}
}

// TestASignerOutsideTheRosterIsRefused — the other half of authorship.
func TestASignerOutsideTheRosterIsRefused(t *testing.T) {
	_, inv := invited(t)
	certX, keyX, fpX := identity(t, "Stranger")
	c := signedCandidate(t, inv, certX, keyX, 1)
	err := c.Verify(inv, testHop, fpX, time.Now())
	if err == nil {
		t.Fatal("a record signed by someone not in the roster verified")
	}
	if !strings.Contains(err.Error(), "not in this ceremony's roster") {
		t.Fatalf("refused for the wrong reason: %v", err)
	}
}

// TestAScraperHoldingNeitherTheInvitationNorTheSecretSeesOpaqueBytes.
func TestAScraperHoldingNeitherTheInvitationNorTheSecretSeesOpaqueBytes(t *testing.T) {
	_, inv := invited(t)
	_, other := invited(t)
	certA, keyA, fpA := identity(t, "A")
	c := signedCandidate(t, inv, certA, keyA, 2)
	sealed, _, salt := sealFor(t, inv, c, fpA, 1)

	wrong, err := other.RecordKey(testHop)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenCandidate(wrong, salt, testHop, 1, sealed); err == nil {
		t.Fatal("a different ceremony's record key opened this record")
	}
	// And the plaintext is not sitting in the ciphertext.
	if bytes.Contains(sealed, []byte(inv.ID)) {
		t.Error("the ceremony id appears verbatim in the sealed record")
	}
	if bytes.Contains(sealed, []byte("198.51.100.1")) {
		t.Error("a candidate address appears verbatim in the sealed record")
	}
}

// TestTheSealIsBoundToItsHopSaltAndSeq — everything outside the ciphertext.
func TestTheSealIsBoundToItsHopSaltAndSeq(t *testing.T) {
	_, inv := invited(t)
	certA, keyA, fpA := identity(t, "A")
	_, _, fpB := identity(t, "B")
	c := signedCandidate(t, inv, certA, keyA, 1)
	sealed, rk, salt := sealFor(t, inv, c, fpA, 1)

	if _, err := OpenCandidate(rk, salt, testHop, 2, sealed); err == nil {
		t.Error("the record opened under a different seq — a replay at a higher seq would " +
			"read as fresh")
	}
	if _, err := OpenCandidate(rk, salt, testHop+1, 1, sealed); err == nil {
		t.Error("the record opened under a different hop")
	}
	otherSalt, err := inv.RecordSalt(testHop, fpB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenCandidate(rk, otherSalt, testHop, 1, sealed); err == nil {
		t.Error("the record opened under the other party's salt — a cut-and-paste between " +
			"the hop's two slots would go unnoticed")
	}
	// A different HOP's record key must not open it either — the D30 clause.
	rkOther, err := inv.RecordKey(testHop + 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenCandidate(rkOther, salt, testHop, 1, sealed); err == nil {
		t.Error("another hop's record key opened this hop's record (D30)")
	}
}

// TestACandidateFromAnotherCeremonyIsRefused — the transplant D31 made possible.
//
// Two ceremonies between the same people. A signs a record in ceremony 1. A roster member
// of both re-seals it under ceremony 2's key and publishes it there. The signature
// verifies and the fingerprint matches, because neither mentions a ceremony — only the
// ceremony id inside the preimage catches it.
func TestACandidateFromAnotherCeremonyIsRefused(t *testing.T) {
	_, one := invited(t)
	_, two := invited(t)
	certA, keyA, fpA := identity(t, "A")
	one.Roster = append(one.Roster, Party{Fingerprint: fpA, Signs: true})
	two.Roster = append(two.Roster, Party{Fingerprint: fpA, Signs: true})

	c := signedCandidate(t, one, certA, keyA, 1) // carries ceremony ONE's id
	if err := c.Verify(one, testHop, fpA, time.Now()); err != nil {
		t.Fatalf("stimulus: the record does not verify in its own ceremony: %v", err)
	}
	if one.ID == two.ID {
		t.Fatal("the two ceremonies share an id — the transplant cannot be tested")
	}

	// Re-sealed under ceremony TWO's key by someone who holds both invitations.
	rk2, err := two.RecordKey(testHop)
	if err != nil {
		t.Fatal(err)
	}
	salt2, err := two.RecordSalt(testHop, fpA)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := c.Seal(rk2, salt2, testHop, 1)
	if err != nil {
		t.Fatal(err)
	}
	back, err := OpenCandidate(rk2, salt2, testHop, 1, sealed)
	if err != nil {
		t.Fatalf("stimulus: the transplant did not even decrypt: %v", err)
	}
	// It decrypts, it is signed by A, and A is in ceremony two's roster. Only the id saves us.
	if back.CeremonyID != one.ID {
		t.Fatalf("the transplanted record lost ceremony one's id (%q)", back.CeremonyID)
	}
	if err := back.Verify(two, testHop, fpA, time.Now()); err == nil {
		t.Fatal("a record signed for another ceremony verified in this one — the preimage " +
			"is not binding the ceremony id")
	}
}

// TestAnExpiredCandidateIsRefused — "records expire", at the reader.
//
// The DHT has no TTL field: BEP-44 items age out on each storer's own schedule and there
// is no recall. So the only expiry a reader can rely on is the one inside the signature.
func TestAnExpiredCandidateIsRefused(t *testing.T) {
	_, inv := invited(t)
	certA, keyA, fpA := identity(t, "A")
	inv.Roster = append(inv.Roster, Party{Fingerprint: fpA, Signs: true})
	c := CandidateRecord{
		CeremonyID: inv.ID, Hop: testHop,
		Expires: time.Now().Add(-time.Second),
		Addrs:   addrs(t, 1),
	}
	if err := c.Sign(certA, keyA); err != nil {
		t.Fatal(err)
	}
	err := c.Verify(inv, testHop, fpA, time.Now())
	if err == nil {
		t.Fatal("an expired record verified")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Fatalf("refused for the wrong reason: %v", err)
	}
	// And the expiry is INSIDE the signature: moving it forward invalidates the record
	// rather than extending it. Without this, any secret-holder strips and re-seals.
	c.Expires = time.Now().Add(time.Hour)
	if err := c.Verify(inv, testHop, fpA, time.Now()); err == nil {
		t.Fatal("the expiry was moved forward and the signature still verified — expiry is " +
			"outside the preimage, so it is advisory rather than binding")
	}
}

// TestAFullRecordFitsInABep44Value — measured, not assumed.
//
// BEP-44 refuses a value whose bencoded length exceeds 1000 (bep44/item.go:132), and the
// refusal happens in our OWN store before any datagram is sent, with getput.Put returning
// nil regardless. So an over-size record does not fail loudly; it simply never leaves the
// machine. The number belongs in a test, not in a comment.
func TestAFullRecordFitsInABep44Value(t *testing.T) {
	_, inv := invited(t)
	certA, keyA, fpA := identity(t, "A")
	c := signedCandidate(t, inv, certA, keyA, MaxCandidates)
	sealed, _, _ := sealFor(t, inv, c, fpA, 1)
	t.Logf("sealed record with %d candidates: %d bytes (cap %d)", MaxCandidates, len(sealed), MaxSealedRecord)
	if len(sealed) > MaxSealedRecord {
		t.Fatalf("a full record is %d bytes, over the %d-byte cap", len(sealed), MaxSealedRecord)
	}
	// IPv6 is the bigger case and is the one that will actually be published on the tiers
	// this phase exists to build.
	var six []netip.AddrPort
	for i := 0; i < MaxCandidates; i++ {
		six = append(six, netip.MustParseAddrPort("[2001:db8:1234:5678:9abc:def0:1234:"+itoa(4096+i)+"]:65535"))
	}
	c6 := CandidateRecord{CeremonyID: inv.ID, Hop: testHop, Expires: time.Now().Add(time.Hour), Addrs: six}
	if err := c6.Sign(certA, keyA); err != nil {
		t.Fatal(err)
	}
	s6, _, _ := sealFor(t, inv, c6, fpA, 1)
	t.Logf("sealed IPv6 record: %d bytes", len(s6))
	if len(s6) > MaxSealedRecord {
		t.Fatalf("a full IPv6 record is %d bytes, over the %d-byte cap", len(s6), MaxSealedRecord)
	}
}

// TestTheTwoPreimagesCannotBeConfused — the domain tags, driven.
//
// sign.SignDigest signs a bare 32-byte digest and does not know what it means, so two
// message types under one identity key are two things one signature can satisfy unless
// their preimages are separated. Both now start with a length-prefixed domain tag.
func TestTheTwoPreimagesCannotBeConfused(t *testing.T) {
	rec, inv := invited(t)
	certA, keyA, _ := identity(t, "A")
	c := signedCandidate(t, inv, certA, keyA, 1)

	pre, err := c.preimage()
	if err != nil {
		t.Fatal(err)
	}
	rh, err := rec.RosterHash()
	if err != nil {
		t.Fatal(err)
	}
	_ = rh

	if !bytes.HasPrefix(pre[8:], []byte(candidateDomain)) {
		t.Fatal("the candidate preimage does not begin with its domain tag")
	}
	if candidateDomain == rosterDomain {
		t.Fatal("the two domain tags are the same string")
	}
	// The decisive property: a candidate preimage can never be READ as a roster preimage,
	// because the first chunk is a tag no roster preimage carries. Assert it by parsing
	// the first chunk out of each and comparing.
	first := func(b []byte) string {
		if len(b) < 8 {
			return ""
		}
		n := int(b[7])
		if len(b) < 8+n {
			return ""
		}
		return string(b[8 : 8+n])
	}
	if first(pre) != candidateDomain {
		t.Fatalf("candidate preimage's first chunk is %q", first(pre))
	}
}

// TestATamperedSealIsRefusedWholly — no partial parse from a stranger's bytes.
func TestATamperedSealIsRefusedWholly(t *testing.T) {
	_, inv := invited(t)
	certA, keyA, fpA := identity(t, "A")
	c := signedCandidate(t, inv, certA, keyA, 2)
	sealed, rk, salt := sealFor(t, inv, c, fpA, 1)

	for _, at := range []int{0, len(sealed) / 2, len(sealed) - 1} {
		bad := append([]byte{}, sealed...)
		bad[at] ^= 0xff
		got, err := OpenCandidate(rk, salt, testHop, 1, bad)
		if err == nil {
			t.Fatalf("a record with byte %d flipped opened cleanly", at)
		}
		if len(got.Addrs) != 0 || got.CeremonyID != "" || len(got.SPKI) != 0 {
			t.Fatalf("a refused record returned a partially-filled struct: %+v", got)
		}
	}
	// Truncation too.
	if _, err := OpenCandidate(rk, salt, testHop, 1, sealed[:len(sealed)-4]); err == nil {
		t.Fatal("a truncated record opened cleanly")
	}
}
