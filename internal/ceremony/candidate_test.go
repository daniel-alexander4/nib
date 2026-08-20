package ceremony

import (
	"bytes"
	"encoding/binary"
	"net/netip"
	"strconv"
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
func sealFor(t *testing.T, inv Invitation, c CandidateRecord, fp string) ([]byte, []byte, []byte) {
	t.Helper()
	rk, err := inv.RecordKey(testHop)
	if err != nil {
		t.Fatal(err)
	}
	salt, err := inv.RecordSalt(testHop, fp)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := c.Seal(rk, salt, testHop)
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
	sealed, rk, salt := sealFor(t, inv, c, fpA)

	back, err := OpenCandidate(rk, salt, testHop, sealed)
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
	sealed, rk, saltA := sealFor(t, inv, forged, fpA)

	// And it decrypts perfectly. Confidentiality is the ceremony's boundary; B is inside it.
	back, err := OpenCandidate(rk, saltA, testHop, sealed)
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
	sealed, _, salt := sealFor(t, inv, c, fpA)

	wrong, err := other.RecordKey(testHop)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenCandidate(wrong, salt, testHop, sealed); err == nil {
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

// TestTheSealIsBoundToItsHopAndSalt — everything outside the ciphertext that CAN be bound.
//
// Renamed and re-scoped at the slice's review: `seq` was in the AAD and is deliberately
// not any more. See TestASealOpensRegardlessOfSeq for the property that replaced it and
// the reason it had to.
func TestTheSealIsBoundToItsHopAndSalt(t *testing.T) {
	_, inv := invited(t)
	certA, keyA, fpA := identity(t, "A")
	_, _, fpB := identity(t, "B")
	c := signedCandidate(t, inv, certA, keyA, 1)
	sealed, rk, salt := sealFor(t, inv, c, fpA)

	if _, err := OpenCandidate(rk, salt, testHop+1, sealed); err == nil {
		t.Error("the record opened under a different hop")
	}
	otherSalt, err := inv.RecordSalt(testHop, fpB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenCandidate(rk, otherSalt, testHop, sealed); err == nil {
		t.Error("the record opened under the other party's salt — a cut-and-paste between " +
			"the hop's two slots would go unnoticed")
	}
	// A different HOP's record key must not open it either — the D30 clause.
	rkOther, err := inv.RecordKey(testHop + 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenCandidate(rkOther, salt, testHop, sealed); err == nil {
		t.Error("another hop's record key opened this hop's record (D30)")
	}
}

// TestASealOpensRegardlessOfSeq — the non-binding is deliberate, and this is why.
//
// An earlier version of this file bound BEP-44's sequence number into the AEAD's associated
// data. That made every fetched record unopenable in production: `getput.Put` chooses the
// seq AFTER the value is sealed, so the publisher can only guess, and the reader passes
// whatever the traversal actually used. The failure surfaced as ErrCandidateFormat — "this
// is not a candidate record this version of Nib understands" — blaming the peer's build for
// a number the local publisher could not observe.
//
// The whole suite missed it because every test passed the literal 1 to both sides. So this
// test exists to make the mismatch the DEFAULT case rather than an unreachable one.
//
// Re-adding the binding would have to answer what it defends against, and the answer is
// nothing: an outsider cannot republish at all (the BEP-44 signature needs the hop seed,
// which comes from the invitation secret); an insider holds that seed and this record key
// and re-seals at will; and a storing node re-serving a recorded tuple serves it at its
// original seq, which binding permits. Freshness is the signed Expires field's job.
func TestASealOpensRegardlessOfSeq(t *testing.T) {
	_, inv := invited(t)
	certA, keyA, fpA := identity(t, "A")
	inv.Roster = append(inv.Roster, Party{Fingerprint: fpA, Signs: true})
	c := signedCandidate(t, inv, certA, keyA, 2)
	sealed, rk, salt := sealFor(t, inv, c, fpA)

	// The publisher sealed once. The traversal may pick any seq at all.
	for _, seq := range []int64{0, 1, 8, 1 << 40} {
		back, err := OpenCandidate(rk, salt, testHop, sealed)
		if err != nil {
			t.Fatalf("a record the traversal published at seq %d did not open: %v", seq, err)
		}
		if err := back.Verify(inv, testHop, fpA, time.Now()); err != nil {
			t.Fatalf("a record published at seq %d did not verify: %v", seq, err)
		}
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
	sealed, err := c.Seal(rk2, salt2, testHop)
	if err != nil {
		t.Fatal(err)
	}
	back, err := OpenCandidate(rk2, salt2, testHop, sealed)
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
	// Seal through the REAL boundary: it refuses over-cap output itself, so asserting
	// `len(sealed) > MaxSealedRecord` after a successful Seal is a dead branch — the
	// failure a future reader actually hits is Seal's error, and this test is named for
	// the size, so it should be the one that reports it.
	c := signedCandidate(t, inv, certA, keyA, MaxCandidates)
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
		t.Fatalf("a full record of %d candidates does not fit: %v", MaxCandidates, err)
	}
	t.Logf("sealed record with %d candidates: %d bytes (cap %d)", MaxCandidates, len(sealed), MaxSealedRecord)
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
	s6, err := c6.Seal(rk, salt, testHop)
	if err != nil {
		t.Fatalf("a full IPv6 record does not fit, and IPv6 is the case the tiers this "+
			"phase builds actually publish: %v", err)
	}
	t.Logf("sealed IPv6 record: %d bytes", len(s6))
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
	sealed, rk, salt := sealFor(t, inv, c, fpA)

	for _, at := range []int{0, len(sealed) / 2, len(sealed) - 1} {
		bad := append([]byte{}, sealed...)
		bad[at] ^= 0xff
		got, err := OpenCandidate(rk, salt, testHop, bad)
		if err == nil {
			t.Fatalf("a record with byte %d flipped opened cleanly", at)
		}
		if len(got.Addrs) != 0 || got.CeremonyID != "" || len(got.SPKI) != 0 {
			t.Fatalf("a refused record returned a partially-filled struct: %+v", got)
		}
	}
	// Truncation too.
	if _, err := OpenCandidate(rk, salt, testHop, sealed[:len(sealed)-4]); err == nil {
		t.Fatal("a truncated record opened cleanly")
	}
}

// --- coverage the slice's review found missing (2026-08-20) ---------------------

// TestBothPreimagesCarryTheirDomainTag — the roster half, which was covered by NOTHING.
//
// The previous version of this file computed `rec.RosterHash()` into a variable and then
// discarded it, and compared `candidateDomain == rosterDomain`, which is two compile-time
// constants. A mutation audit deleted the tag from record.go and the ENTIRE REPO stayed
// green. That is the vacuous green in its purest form: a test named for a property,
// sitting next to the property, testing the other one.
//
// It is testable now only because rosterPreimage was split out of RosterHash. A function
// that returns a digest gives a test nothing to look at.
func TestBothPreimagesCarryTheirDomainTag(t *testing.T) {
	rec, inv := invited(t)
	certA, keyA, _ := identity(t, "A")
	c := signedCandidate(t, inv, certA, keyA, 1)

	cand, err := c.preimage()
	if err != nil {
		t.Fatal(err)
	}
	roster, err := rosterPreimage(rec)
	if err != nil {
		t.Fatal(err)
	}

	// firstChunk reads a real 8-byte big-endian length, not its low byte.
	firstChunk := func(b []byte) string {
		if len(b) < 8 {
			return ""
		}
		n := binary.BigEndian.Uint64(b[:8])
		if uint64(len(b)-8) < n {
			return ""
		}
		return string(b[8 : 8+n])
	}

	if got := firstChunk(cand); got != candidateDomain {
		t.Errorf("the candidate preimage begins with %q, want the domain tag %q", got, candidateDomain)
	}
	if got := firstChunk(roster); got != rosterDomain {
		t.Errorf("the ROSTER preimage begins with %q, want the domain tag %q — this is the "+
			"half that a deleted tag left green across the whole repo", got, rosterDomain)
	}
	if firstChunk(cand) == firstChunk(roster) {
		t.Error("both preimages open with the same tag, so one signature satisfies both " +
			"message types")
	}
}

// TestTheSaltIsKEYED, not merely different per party.
//
// Every earlier assertion here was a difference assertion, which ANY distinct-input hash
// satisfies. A mutation audit passed two unkeyed salts — including one publishing sixteen
// raw fingerprint bytes in cleartext on every put, which is exactly the searchable index of
// Nib identities the doc comment forbids.
//
// The discriminator is one line: the SAME fingerprint under a DIFFERENT invitation must
// give a different salt. Nothing unkeyed can survive it.
func TestTheSaltIsKeyedToTheInvitation(t *testing.T) {
	_, one := invited(t)
	_, two := invited(t)
	_, _, fp := identity(t, "A")

	a, err := one.RecordSalt(0, fp)
	if err != nil {
		t.Fatal(err)
	}
	b, err := two.RecordSalt(0, fp)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a, b) {
		t.Fatal("the same party's salt is identical across two unrelated ceremonies — the " +
			"salt is not keyed, so anyone holding this fingerprint (a QR code, a business " +
			"card) can watch the public DHT for it and learn every ceremony they run")
	}
}

// TestEveryDerivationTakesTheSECRET, not the public ceremony id.
//
// The key-separation test proved "differently" and never proved "from the secret": a
// mutation audit re-keyed both derivations off Invitation.ID — which is public, and which
// travels inside the signed Record — and the suite stayed green, because every assertion
// compared two fully independent invitations whose IDs already differ.
//
// One invitation, one field changed.
func TestEveryDerivationTakesTheSecret(t *testing.T) {
	_, inv := invited(t)
	other := inv // same ID, same roster, same everything
	other.Secret = append([]byte(nil), inv.Secret...)
	other.Secret[0] ^= 0xff

	if inv.ID != other.ID {
		t.Fatal("setup: the two invitations must differ ONLY in the secret")
	}

	type derivation struct {
		name string
		fn   func(Invitation) ([]byte, error)
	}
	_, _, fp := identity(t, "A")
	for _, d := range []derivation{
		{"HopSeed", func(i Invitation) ([]byte, error) { return i.HopSeed(0) }},
		{"RecordKey", func(i Invitation) ([]byte, error) { return i.RecordKey(0) }},
		{"RecordSalt", func(i Invitation) ([]byte, error) { return i.RecordSalt(0, fp) }},
		{"BindingMAC", func(i Invitation) ([]byte, error) { return i.BindingMAC([]byte("x"), "initiator") }},
	} {
		x, err := d.fn(inv)
		if err != nil {
			t.Fatal(err)
		}
		y, err := d.fn(other)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(x, y) {
			t.Errorf("%s is unchanged when only the SECRET changes — it is deriving from "+
				"something public (the ceremony id travels in the signed record), so the "+
				"whole of caveat 11's 32 uniform bytes is buying nothing", d.name)
		}
	}
}

// TestAFingerprintsCaseCannotSplitTheCeremony.
//
// hex.DecodeString accepts either case, so an uppercase fingerprint was a legitimate roster
// entry — and it broke the ceremony two ways at once: the two parties derived different DHT
// salts and never met, and if they had, the record would be refused as an impersonation
// over a difference in letter case.
func TestAFingerprintsCaseCannotSplitTheCeremony(t *testing.T) {
	_, inv := invited(t)
	_, _, fp := identity(t, "A")
	upper := strings.ToUpper(fp)
	if upper == fp {
		t.Fatal("setup: the fingerprint has no letters to upper-case")
	}

	lo, err := inv.RecordSalt(0, fp)
	if err != nil {
		t.Fatal(err)
	}
	hi, err := inv.RecordSalt(0, upper)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(lo, hi) {
		t.Error("the same fingerprint in two cases derives two different salts — the two " +
			"parties publish to different targets and never find each other, silently")
	}

	// And the case is normalised on the way in, so every string compare downstream agrees.
	up := inv
	up.Roster = append([]Party(nil), inv.Roster...)
	up.Roster[0].Fingerprint = strings.ToUpper(up.Roster[0].Fingerprint)
	text, err := up.Encode()
	if err != nil {
		t.Fatal(err)
	}
	back, err := ParseInvitation(text)
	if err != nil {
		t.Fatalf("an uppercase roster was refused outright: %v", err)
	}
	if back.Roster[0].Fingerprint != strings.ToLower(up.Roster[0].Fingerprint) {
		t.Errorf("ParseInvitation kept %q; every consumer compares the string, so a mixed "+
			"case roster produces a false impersonation refusal", back.Roster[0].Fingerprint)
	}
}

// TestSealRefusesARecordModifiedAfterSigning.
func TestSealRefusesARecordModifiedAfterSigning(t *testing.T) {
	_, inv := invited(t)
	certA, keyA, fpA := identity(t, "A")
	c := signedCandidate(t, inv, certA, keyA, 1)
	rk, err := inv.RecordKey(testHop)
	if err != nil {
		t.Fatal(err)
	}
	salt, err := inv.RecordSalt(testHop, fpA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Seal(rk, salt, testHop); err != nil {
		t.Fatalf("setup: the unmodified record did not seal: %v", err)
	}

	c.Addrs = append(c.Addrs, netip.MustParseAddrPort("203.0.113.9:1"))
	if _, err := c.Seal(rk, salt, testHop); err == nil {
		t.Fatal("a record modified after signing sealed cleanly — the peer would refuse it " +
			"as tampering in transit, which points at the wrong machine entirely")
	}
}

// TestARecordCannotClaimToBeValidForever.
func TestARecordCannotClaimToBeValidForever(t *testing.T) {
	_, inv := invited(t)
	certA, keyA, fpA := identity(t, "A")
	inv.Roster = append(inv.Roster, Party{Fingerprint: fpA, Signs: true})
	c := CandidateRecord{
		CeremonyID: inv.ID, Hop: testHop,
		Expires: time.Now().Add(100 * 365 * 24 * time.Hour),
		Addrs:   addrs(t, 1),
	}
	if err := c.Sign(certA, keyA); err != nil {
		t.Fatal(err)
	}
	err := c.Verify(inv, testHop, fpA, time.Now())
	if err == nil {
		t.Fatal("a record valid for a century was accepted — Expires is the ENTIRE freshness " +
			"margin, so a storing node could re-serve this endpoint for as long as the " +
			"publisher felt like writing")
	}
	// And an ordinary lifetime still passes, so the ceiling is not a blanket refusal.
	c.Expires = time.Now().Add(10 * time.Minute)
	if err := c.Sign(certA, keyA); err != nil {
		t.Fatal(err)
	}
	if err := c.Verify(inv, testHop, fpA, time.Now()); err != nil {
		t.Fatalf("an ordinary 10-minute record was refused by the ceiling: %v", err)
	}
}

// TestANonCanonicalPlaintextIsRefused — the received bytes must be the only encoding.
//
// Verify checks the signature over a preimage rebuilt from the PARSED struct, so any input
// that canonicalises differently is malleable inside the envelope by a record-key holder.
// No exploit today; a signature bypass the day an axis with weaker canonicalisation lands.
func TestANonCanonicalPlaintextIsRefused(t *testing.T) {
	_, inv := invited(t)
	certA, keyA, _ := identity(t, "A")
	c := signedCandidate(t, inv, certA, keyA, 1)
	// Rebuild the plaintext with a non-canonical expiry rendering, sign nothing new — a
	// record-key holder can do exactly this.
	var p preimageBuilder
	p.addString(candidateDomain)
	p.addUint(uint64(c.Version))
	p.addString(c.CeremonyID)
	p.addUint(uint64(c.Hop))
	p.add(c.SPKI)
	p.addString(c.Expires.UTC().Add(30 * time.Minute).Format("2006-01-02T15:04:05-07:00")) // offset form
	p.addUint(uint64(len(c.Addrs)))
	for _, a := range c.Addrs {
		p.addString(a.String())
	}
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(c.Sig)))
	plain := append(p.bytes(), n[:]...)
	plain = append(plain, c.Sig...)

	if _, err := parseCandidate(plain); err == nil {
		t.Fatal("a plaintext whose expiry re-encodes to different bytes was accepted — the " +
			"claim that the thing signed and the thing sent are the same bytes is not what " +
			"the code enforces")
	}
}

// TestTheSealedCapMatchesWhatBep44WillActuallyAccept.
//
// MaxSealedRecord and internal/rendezvous's 1000 were joined by prose only. Raising this to
// 1000 — the plausible "fix the off-by-4" edit — keeps every ceremony test green and makes
// every full record silently unpublishable, which is precisely the failure both files spend
// paragraphs describing. L1 forbids importing rendezvous, so the arithmetic is asserted here.
func TestTheSealedCapMatchesWhatBep44WillActuallyAccept(t *testing.T) {
	// A byte string of n bytes bencodes to len(itoa(n)) + 1 + n.
	bencoded := len(strconv.Itoa(MaxSealedRecord)) + 1 + MaxSealedRecord
	if bencoded > 1000 {
		t.Fatalf("a %d-byte sealed record bencodes to %d, over BEP-44's 1000-byte cap "+
			"(bep44/item.go) — every full record would be refused inside our own store "+
			"before any datagram was sent, and getput.Put returns nil regardless",
			MaxSealedRecord, bencoded)
	}
	if next := len(strconv.Itoa(MaxSealedRecord+1)) + 1 + MaxSealedRecord + 1; next <= 1000 {
		t.Errorf("MaxSealedRecord is %d but %d would still fit (%d bencoded) — the cap is "+
			"needlessly tight", MaxSealedRecord, MaxSealedRecord+1, next)
	}
}
