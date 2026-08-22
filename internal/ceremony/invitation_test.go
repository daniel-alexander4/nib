package ceremony

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"
)

func invited(t *testing.T) (Record, Invitation) {
	t.Helper()
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	r := draft(t, cfp, afp)
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	inv, err := oneInvitation(t, r)
	if err != nil {
		t.Fatal(err)
	}
	return r, inv
}

// TestAnInvitationRoundTripsThroughAPaste is the first acceptance clause, and the paste is
// the point: an invitation travels through email and chat, so the forms it must survive
// are the forms those tools produce.
func TestAnInvitationRoundTripsThroughAPaste(t *testing.T) {
	_, inv := invited(t)
	text, err := inv.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(text, "nib-invite-v1:") {
		t.Errorf("the text form does not announce what it is: %.30s", text)
	}

	for _, mangled := range []string{
		text,
		"  " + text + "  ",
		text[:40] + "\n" + text[40:],        // a mail client's line break
		text[:20] + "\r\n   " + text[20:],   // a quoted reply's wrap and indent
		"\n" + text[:60] + "\n" + text[60:], // pasted across three lines
	} {
		back, err := ParseInvitation(mangled)
		if err != nil {
			t.Fatalf("a pasted invitation was refused (%v) for input %.50q", err, mangled)
		}
		if back.ID != inv.ID || !bytes.Equal(back.Secret, inv.Secret) {
			t.Error("the round trip lost the id or the secret")
		}
		if len(back.Roster) != len(inv.Roster) {
			t.Fatalf("the round trip lost roster entries: %d, want %d", len(back.Roster), len(inv.Roster))
		}
		for i := range back.Roster {
			if back.Roster[i].Fingerprint != inv.Roster[i].Fingerprint {
				t.Errorf("roster entry %d changed across the round trip", i)
			}
		}
	}
}

// TestACorruptedInvitationIsRefusedWholly is the other half of that clause: "refused with a
// distinct error rather than a partial pairing".
//
// **Wholly** is the substance. A partially-applied invitation pins some parties and
// silently omits others, and the user has no way to see which — so the failure has to be
// all-or-nothing, and the message has to distinguish "you pasted the wrong thing" from
// "what you pasted arrived damaged".
func TestACorruptedInvitationIsRefusedWholly(t *testing.T) {
	_, inv := invited(t)
	text, err := inv.Encode()
	if err != nil {
		t.Fatal(err)
	}

	// The stimulus: the intact form really does parse, so every refusal below is about the
	// damage rather than about the format being unreadable in general.
	if _, err := ParseInvitation(text); err != nil {
		t.Fatalf("setup: the intact invitation does not parse: %v", err)
	}

	body := strings.TrimPrefix(text, "nib-invite-v1:")
	flip := func(s string, i int) string {
		b := []byte(s)
		if b[i] == 'A' {
			b[i] = 'B'
		} else {
			b[i] = 'A'
		}
		return string(b)
	}
	for _, c := range []struct {
		name string
		text string
		want error
	}{
		{"a flipped character mid-payload", "nib-invite-v1:" + flip(body, len(body)/2), ErrInvitationCorrupt},
		{"a truncated payload", "nib-invite-v1:" + body[:len(body)/2], ErrInvitationCorrupt},
		{"the checksum removed", "nib-invite-v1:" + strings.Split(body, ".")[0], ErrInvitationCorrupt},
		{"an ordinary sentence", "here is the link I promised", ErrInvitationFormat},
		{"an empty string", "", ErrInvitationFormat},
		{"a future version", "nib-invite-v9:abc.dead", ErrInvitationVersion},
	} {
		got, err := ParseInvitation(c.text)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
		if len(got.Roster) != 0 || len(got.Secret) != 0 {
			t.Errorf("%s: a refused invitation still returned %d roster entries and %d secret "+
				"bytes — a partial pairing is worse than none, because it pins some parties and "+
				"silently omits others", c.name, len(got.Roster), len(got.Secret))
		}
	}
}

// TestARosterEntryMustBeAFullFingerprint: the invited path's whole claim is a 256-bit pin.
// An entry that is not a full fingerprint has to be refused, not truncated or padded.
func TestARosterEntryMustBeAFullFingerprint(t *testing.T) {
	_, inv := invited(t)
	inv.Roster[1].Fingerprint = "abcd" // 66 bits' worth would look like this
	text, err := inv.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseInvitation(text); !errors.Is(err, ErrInvitationCorrupt) {
		t.Errorf("an invitation with a short fingerprint gave %v — the invited path is a "+
			"256-bit pin, and a short entry is the one corruption that must never pass", err)
	}
}

// --- key derivation -----------------------------------------------------------

// TestTheSecretKeysEverythingAndKeysThemDifferently is the third acceptance clause plus the
// domain separation that keeps it safe.
//
// Two ceremonies between the same parties must produce different keys — that is the point
// of re-keying to the secret rather than to the names, which are public by design. And no
// two purposes may share a key: a rendezvous key that equalled a record key would let
// anyone who could find the record decrypt it.
func TestTheSecretKeysEverythingAndKeysThemDifferently(t *testing.T) {
	_, one := invited(t)
	_, two := invited(t)

	rk1, err := one.HopSeed(0)
	if err != nil {
		t.Fatal(err)
	}
	rk2, err := two.HopSeed(0)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(rk1, rk2) {
		t.Error("two ceremonies produced the same rendezvous key — the derivation is not " +
			"taking the secret, so a stable pair of people would publish under one key forever")
	}

	rec1, err := one.RecordKey(0)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(rk1, rec1) {
		t.Error("the rendezvous key and the record key are the same value — anyone who could " +
			"find the record could decrypt it")
	}

	bind1, err := one.BindingMAC([]byte("exporter"), "initiator")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(bind1, rec1) || bytes.Equal(bind1, rk1) {
		t.Error("the channel binding shares a key with another purpose")
	}

	// Per hop (D30): two hops of one ceremony must not publish under the same key.
	hop1, err := one.HopSeed(1)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(rk1, hop1) {
		t.Error("two hops of one ceremony share a rendezvous key — an observer who learns " +
			"one hop's key can follow the rest (D30)")
	}

	// And the RECORD key is per hop too, which it was not until P04.S03.
	//
	// This is the clause D30 asks P05 to drive ("a party cannot read the candidates of a
	// hop it is not in") and it was unsatisfiable while one key decrypted every hop.
	// Asserting it here, at the derivation, is what makes it true before anything is
	// built on top of it.
	recHop1, err := one.RecordKey(1)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(rec1, recHop1) {
		t.Error("two hops of one ceremony share a RECORD key — per-hop addressing without " +
			"per-hop encryption means the ciphertext of a hop you are not in decrypts with " +
			"the key you already hold (D30)")
	}
	if bytes.Equal(recHop1, hop1) {
		t.Error("the hop's record key and its ed25519 seed are the same value")
	}

	// The salt is keyed: two parties differ, and neither is a function anyone else can
	// recognise. An unkeyed salt would index Nib identities in the public DHT.
	fpA := strings.Repeat("a1", sha256.Size)
	fpB := strings.Repeat("b2", sha256.Size)
	sA, err := one.RecordSalt(0, fpA)
	if err != nil {
		t.Fatal(err)
	}
	sB, err := one.RecordSalt(0, fpB)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(sA, sB) {
		t.Error("both parties of a hop derive the same salt — they would publish to one " +
			"target and the higher seq would silently clobber the other")
	}
	if bytes.Contains(sA, mustHex(t, fpA)) {
		t.Error("the salt contains the fingerprint verbatim — the salt travels in cleartext " +
			"in every put, so this publishes a permanent identity pin to every storing node")
	}
	sHop1, err := one.RecordSalt(1, fpA)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(sA, sHop1) {
		t.Error("one party's salt is the same at two hops — the two records collide")
	}
	if _, err := one.RecordSalt(0, "not-a-fingerprint"); err == nil {
		t.Error("a short fingerprint was accepted as salt input")
	}
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// --- caveat 11: the channel binding -------------------------------------------

// TestTheBindingProvesTheSecretOnThisChannel is caveat 11's mechanism, driven.
//
// Three properties, and each is a separate way the binding could be useless:
// a peer without the secret cannot produce the MAC; a MAC from one channel does not verify
// on another; and the two directions are distinct, so the initiator's MAC cannot be
// reflected back as the responder's.
func TestTheBindingProvesTheSecretOnThisChannel(t *testing.T) {
	_, inv := invited(t)
	_, other := invited(t)
	exporter := bytes.Repeat([]byte{7}, 32)

	mine, err := inv.BindingMAC(exporter, "initiator")
	if err != nil {
		t.Fatal(err)
	}
	// The stimulus: the honest case verifies, so every refusal below is about the attack.
	if err := inv.CheckBindingMAC(exporter, "initiator", mine); err != nil {
		t.Fatalf("setup: an honest MAC does not verify: %v", err)
	}

	theirs, err := other.BindingMAC(exporter, "initiator")
	if err != nil {
		t.Fatal(err)
	}
	if err := inv.CheckBindingMAC(exporter, "initiator", theirs); err == nil {
		t.Error("a MAC computed with a DIFFERENT invitation's secret verified — the binding " +
			"proves nothing about holding this ceremony's secret")
	}

	otherChannel := bytes.Repeat([]byte{9}, 32)
	if err := inv.CheckBindingMAC(otherChannel, "initiator", mine); err == nil {
		t.Error("a MAC captured on one channel verified on another — a recording of one " +
			"session would be replayable into the next")
	}

	if err := inv.CheckBindingMAC(exporter, "responder", mine); err == nil {
		t.Error("the initiator's MAC verified as the responder's — an attacker who can echo " +
			"bytes would look like a peer holding the secret")
	}

	if _, err := inv.BindingMAC(nil, "initiator"); err == nil {
		t.Error("a MAC was produced with no channel binding material, so it binds no channel")
	}
}

// --- the record comparison ----------------------------------------------------

// TestAOneByteAlteredInvitationIsRefusedByName is the plan-review pin on D21.
//
// Nothing signs the invitation, so tampering cannot be caught when it is read. It is caught
// at the first moment there is an independently-signed copy of the roster to compare
// against — the record inside the document. The full-fingerprint clause cannot see this: a
// tampered invitation satisfies it perfectly by pinning the WRONG key at full length.
func TestAOneByteAlteredInvitationIsRefusedByName(t *testing.T) {
	rec, inv := invited(t)

	// The stimulus: an untampered invitation matches, so the refusals below are the
	// tampering and not a comparison that always fails.
	if err := inv.MatchesRecord(rec); err != nil {
		t.Fatalf("setup: an honest invitation does not match its own record: %v", err)
	}

	_, _, outsider := identity(t, "Outsider")
	for _, c := range []struct {
		name string
		mut  func(i *Invitation)
		says string
	}{
		{"a fingerprint swapped", func(i *Invitation) { i.Roster[1].Fingerprint = outsider },
			"party 2"},
		{"the signs flag flipped", func(i *Invitation) { i.Roster[0].Signs = true },
			"not signing"},
		{"a party added", func(i *Invitation) { i.Roster = append(i.Roster, Party{Fingerprint: outsider, Signs: true}) },
			"parties"},
		{"the ceremony id changed", func(i *Invitation) { i.ID = "0123456789abcdef0123456789abcdef" },
			"ceremony"},
	} {
		bad := inv
		bad.Roster = append([]Party(nil), inv.Roster...)
		c.mut(&bad)
		err := bad.MatchesRecord(rec)
		if !errors.Is(err, ErrRosterMismatch) {
			t.Errorf("%s: got %v, want a roster mismatch", c.name, err)
			continue
		}
		// "Refused BY NAME" — the message has to say which party, or the user is told
		// something is wrong and given nothing to act on.
		if !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: the refusal says %q, which does not name what differs", c.name, err)
		}
	}
}

// TestAnInvitationIsNotASigningCredential (D21).
//
// The property is structural and is asserted as such rather than dressed as a handshake
// drive: an invitation carries fingerprints and a secret, and **no private key**. A holder
// therefore has nothing to sign with, which is why an intercepted invitation reaches the
// rendezvous and stops at the handshake — the pin is the fingerprint and mTLS demands the
// key behind it.
//
// The handshake half of that is P02's transport, already guarded by
// `TestSessionTLSRefusesUnpinnedPeer`; what this slice owes is that the invitation does not
// smuggle a key into it.
func TestAnInvitationIsNotASigningCredential(t *testing.T) {
	_, inv := invited(t)
	text, err := inv.Encode()
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"PRIVATE KEY", "BEGIN EC", "BEGIN RSA", "privateKey", "keyPEM"} {
		if strings.Contains(text, marker) {
			t.Errorf("the invitation's text form contains %q — an invitation is not a signing "+
				"credential, and an intercepted one must stop at the handshake", marker)
		}
	}
	// And the parsed form carries no key-shaped field. Checked on the decoded struct rather
	// than only on the text, so an encoding change cannot quietly reintroduce one.
	back, err := ParseInvitation(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Secret) != SecretLen {
		t.Fatalf("setup: the parsed invitation has no secret, so this test is not looking at "+
			"a real one (%d bytes)", len(back.Secret))
	}
	for _, p := range back.Roster {
		if len(p.Fingerprint) != 64 {
			t.Errorf("roster entry carries %d characters, want a 64-hex fingerprint", len(p.Fingerprint))
		}
	}
}

// TestAnInvitationCatchesARecordConvenedBySomeoneElse.
//
// `ConvenerFingerprint`'s doc says it exists "so a party can check the record's signer
// against what the invitation told them to expect". `MatchesRecord` compared the id, the
// fingerprints and the signs flags — and never the convener. The field was written at two
// sites and **read nowhere**: the check it exists for did not exist.
//
// It matters because `RosterHash` does not bind who convened. `Record.Verify` asks only
// that the signer appear SOMEWHERE in the roster, so any roster member can re-sign an
// unchanged roster with their own key and the record still verifies — and `Convener()` then
// names them. The invitation is the second, independent statement of who it should be.
func TestAnInvitationCatchesARecordConvenedBySomeoneElse(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	certA, keyA, afp := identity(t, "A")
	rec := draft(t, cfp, afp)
	if err := rec.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	inv, err := oneInvitation(t, rec)
	if err != nil {
		t.Fatal(err)
	}

	// STIMULUS: the honest pair must match, or the refusal below proves nothing.
	if err := inv.MatchesRecord(rec); err != nil {
		t.Fatalf("setup: the honest invitation and record do not match: %v", err)
	}

	// The OTHER roster member re-signs the IDENTICAL roster with their own key.
	forged := rec
	if err := forged.Sign(certA, keyA); err != nil {
		t.Fatal(err)
	}

	// STIMULUS, and it is the finding: the forgery VERIFIES. Nothing about the record
	// itself can see it — the roster hash is byte-identical, so RosterToken is too.
	if err := forged.Verify(time.Now()); err != nil {
		t.Fatalf("setup: the re-signed record does not verify, so this is not the attack "+
			"the check is for: %v", err)
	}
	// The roster token now DIFFERS, and that is the v3 improvement rather than a broken
	// stimulus.
	//
	// This assertion was written inverted — "the token must be unchanged, or the record
	// alone can already tell" — because at v2 the convener was outside the preimage, so a
	// re-signed roster produced byte-identical bytes and only the invitation could catch
	// it. v3 binds the convener's fingerprint as its own axis, so a verifier reading a
	// FINISHED DOCUMENT can tell too, with no invitation in hand. Both checks now exist and
	// they answer different questions: this one is "who does this record say convened",
	// MatchesRecord's is "is that who the invitation told me to expect".
	origH, _ := rec.RosterHash()
	forgedH, _ := forged.RosterHash()
	origTok, forgedTok := hex.EncodeToString(origH), hex.EncodeToString(forgedH)
	if origTok == forgedTok {
		t.Errorf("the roster token is identical after another roster member re-signed the "+
			"same roster (%s) — a verifier reading the finished document cannot tell which "+
			"of them convened", origTok)
	}
	if conv, ok := forged.Convener(); !ok || conv.Fingerprint == cfp {
		t.Fatalf("setup: Convener() still names the original convener")
	}

	if err := inv.MatchesRecord(forged); !errors.Is(err, ErrRosterMismatch) {
		t.Errorf("MatchesRecord accepted a record convened by a different roster member: "+
			"%v. A verifier reading the finished document cannot tell which of them "+
			"convened, and the invitation is the only independent statement of it", err)
	}
}

// TestTheBindingMACCannotBeSlidAcrossItsFieldBoundary.
//
// `BindingMAC` concatenated `role` and `exporter` with no length prefixes and no domain
// tag, bypassing `preimageBuilder` — whose own doc calls itself "the one length-prefix
// encoder every signed preimage in this package uses", which this made false.
//
// Bare concatenation is ambiguous: ("a","bc") and ("ab","c") write identical bytes. Two
// mitigations existed and neither was written down or asserted — the key is purpose-derived
// so confusion needs the same key, and the only roles anyone passes happen to be the same
// length. `role` is a free string parameter on an exported method, so neither is a property
// of the code.
func TestTheBindingMACCannotBeSlidAcrossItsFieldBoundary(t *testing.T) {
	_, inv := invited(t)

	a, err := inv.BindingMAC([]byte("bc"), "a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := inv.BindingMAC([]byte("c"), "ab")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a, b) {
		t.Error(`BindingMAC("bc","a") == BindingMAC("c","ab") — the boundary between the ` +
			"role and the channel binding can be slid, so one MAC attests to two different " +
			"(role, exporter) pairs")
	}

	// The control: the same inputs must still agree with themselves, or the fix is just noise.
	again, err := inv.BindingMAC([]byte("bc"), "a")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, again) {
		t.Fatal("BindingMAC is not deterministic")
	}
	// And the property the MAC exists for: the two roles must differ, so a reflection of
	// the initiator's MAC cannot pass as the responder's.
	init, _ := inv.BindingMAC([]byte("x"), "initiator")
	resp, _ := inv.BindingMAC([]byte("x"), "responder")
	if bytes.Equal(init, resp) {
		t.Error("the two roles produce one MAC — a reflection passes as a peer holding the secret")
	}
}

// TestTheConvenerCheckIsNotOptIn drives the two ways the convener comparison could be walked
// past without any check failing.
func TestTheConvenerCheckIsNotOptIn(t *testing.T) {
	rec, inv := invited(t)
	// The stimulus, restated here rather than inherited: an honest invitation matches. A
	// comparison that always failed would satisfy both assertions below.
	if err := inv.MatchesRecord(rec); err != nil {
		t.Fatalf("setup: an honest invitation does not match its own record: %v", err)
	}
	if inv.ConvenerFingerprint == "" {
		t.Fatal("setup: the invitation names no convener, so neither case below is reached")
	}

	// 1. Blanking the field skipped the check entirely — and the field is plain JSON on a
	// document a party receives, so this is a one-byte edit, not an old build's oversight.
	blank := inv
	blank.ConvenerFingerprint = ""
	if err := blank.MatchesRecord(rec); err == nil {
		t.Error("an invitation that names no convener matched — the one comparison the field " +
			"exists for is skipped by deleting the field")
	}

	// 2. Upper-case hex is the same fingerprint. Both sides are hex and nothing normalises
	// them, and the sentence a mismatch produces accuses a counterparty of substituting the
	// convener — the loudest wrong answer this function can give.
	upper := inv
	upper.ConvenerFingerprint = strings.ToUpper(inv.ConvenerFingerprint)
	if upper.ConvenerFingerprint == inv.ConvenerFingerprint {
		t.Skip("the fixture fingerprint has no letters to upper-case")
	}
	if err := upper.MatchesRecord(rec); err != nil {
		t.Errorf("the same fingerprint in upper-case hex was reported as a different "+
			"convener: %v", err)
	}
}

// TestOnePartyCannotDeriveAnothersHop — D30's stated harm, actually fixed.
//
// D30 says the problem in its own words: under one key "every party can read every other
// party's IP addresses". It chose per-hop derivation as the remedy, and per-hop derivation
// does not reach it — `derive` consults the secret, the ceremony id and an info string, with
// **no per-party input anywhere**, so every holder of a shared secret computes every hop's
// key, salt and seed. `RecordKey`'s own doc conceded it: "any roster member can derive ANY
// hop's key from the secret."
//
// With one secret per party, the boundary becomes the one D22's topology already draws: a
// hub, where every hop is convener-to-party and two counterparties never connect. So a
// secret is shared by exactly the two ends of the hop it is for.
func TestOnePartyCannotDeriveAnothersHop(t *testing.T) {
	cert, key, c := identity(t, "convener")
	_, _, a := identity(t, "alice")
	_, _, b := identity(t, "bob")
	r := draft(t, c, a, b)
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	invs, err := NewInvitations(r)
	if err != nil {
		t.Fatal(err)
	}

	// SETUP: one invitation per non-convener party, and the convener gets none of its own.
	if len(invs) != 2 {
		t.Fatalf("setup: want 2 invitations for a three-party roster, got %d", len(invs))
	}
	if _, ok := invs[c]; ok {
		t.Error("the convener was minted an invitation of its own; it holds every party's")
	}
	alice, bob := invs[a], invs[b]

	// **The property.** Alice's secret and Bob's differ, so everything derived from them
	// differs — and Alice cannot compute Bob's hop material at all, because she does not
	// hold the input.
	if string(alice.Secret) == string(bob.Secret) {
		t.Fatal("setup: the two parties were given the SAME secret, which is the whole " +
			"defect this test exists for — every assertion below would be trivially false")
	}

	hopA, err := alice.Hop(c, a)
	if err != nil {
		t.Fatal(err)
	}
	hopB, err := bob.Hop(c, b)
	if err != nil {
		t.Fatal(err)
	}
	if hopA == hopB {
		t.Fatalf("setup: both parties resolved to hop %d, so the comparison below is "+
			"between one hop and itself", hopA)
	}

	// Alice derives Bob's hop material from HER invitation — which is the attack: she knows
	// the hop number, she knows Bob's fingerprint, she has a secret. What she does not have
	// is Bob's secret, and every derivation is rooted in it.
	for _, d := range []struct {
		name         string
		mine, theirs func(Invitation) ([]byte, error)
	}{
		{"the record key", func(i Invitation) ([]byte, error) { return i.RecordKey(hopB) },
			func(i Invitation) ([]byte, error) { return i.RecordKey(hopB) }},
		{"the BEP-44 seed", func(i Invitation) ([]byte, error) { return i.HopSeed(hopB) },
			func(i Invitation) ([]byte, error) { return i.HopSeed(hopB) }},
		{"the rendezvous salt", func(i Invitation) ([]byte, error) { return i.RecordSalt(hopB, b) },
			func(i Invitation) ([]byte, error) { return i.RecordSalt(hopB, b) }},
	} {
		guess, err := d.mine(alice)
		if err != nil {
			t.Fatal(err)
		}
		real, err := d.theirs(bob)
		if err != nil {
			t.Fatal(err)
		}
		if string(guess) == string(real) {
			t.Errorf("Alice derived %s for Bob's hop and got the RIGHT value. She can then "+
				"locate his BEP-44 target, read his candidate addresses, overwrite his "+
				"record, and take his key to the sequence-number ceiling — which is D30's "+
				"stated harm, unfixed.", d.name)
		}
	}

	// **And the convener CAN, with the right invitation — which is not a gap, it is D22.**
	// The convener carries the document and dials everyone; it holds every party's secret by
	// construction. Asserted so the limit is on record rather than assumed away.
	convGuess, err := invs[b].RecordKey(hopB)
	if err != nil {
		t.Fatal(err)
	}
	realB, err := bob.RecordKey(hopB)
	if err != nil {
		t.Fatal(err)
	}
	if string(convGuess) != string(realB) {
		t.Error("the convener could not derive a party's hop key from that party's own " +
			"invitation — both ends of a hop must agree, and the convener is one of them")
	}
}
