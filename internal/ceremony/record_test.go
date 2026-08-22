package ceremony

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nib/internal/p2p"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

func identity(t *testing.T, cn string) (certPEM, keyPEM []byte, fp string) {
	t.Helper()
	c, k, err := sign.GenerateIdentity(cn)
	if err != nil {
		t.Fatal(err)
	}
	b, err := sign.Fingerprint(c)
	if err != nil {
		t.Fatal(err)
	}
	return c, k, hex.EncodeToString(b)
}

// draft builds a record for a three-party ceremony with a NON-signing convener, which is
// the shape most of these tests want: `signs:false` is the axis PLAN-2 exists to protect
// and a roster where everyone signs cannot see it.
func draft(t *testing.T, convenerFP string, others ...string) Record {
	t.Helper()
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	r := Record{
		ID:      id,
		DocHash: strings.Repeat("ab", 32),
		Intent:  "We agree to co-sign the lease",
		Expires: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
		Roster:  []Party{{Fingerprint: convenerFP, Label: "Convener", Signs: false}},
	}
	for i, fp := range others {
		r.Roster = append(r.Roster, Party{Fingerprint: fp, Label: string(rune('A' + i)), Signs: true})
	}
	return r
}

// --- the commitment ----------------------------------------------------------

// TestRosterHashCoversEveryAxis is PLAN-2's specification turned into a check.
//
// The pin exists because "a commitment over all of the above" is a gesture: a slice would
// have decided what that meant and the decision would have been invisible. So every axis
// is varied ALONE and must move the hash. An axis short is the attack.
func TestRosterHashCoversEveryAxis(t *testing.T) {
	_, _, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	base := draft(t, cfp, afp)
	want, err := base.RosterHash()
	if err != nil {
		t.Fatal(err)
	}

	_, _, other := identity(t, "Someone else")
	for _, c := range []struct {
		name string
		mut  func(r *Record)
		why  string
	}{
		{"version", func(r *Record) { r.Version = 99 },
			"two format versions could produce the same commitment, so a party running a " +
				"different version would silently agree to a record it parsed differently (D32)"},
		{"id", func(r *Record) { r.ID = "00000000000000000000000000000000" },
			"two ceremonies could share a commitment"},
		{"docHash", func(r *Record) { r.DocHash = strings.Repeat("cd", 32) },
			"the commitment would not name the document, so signatures could attest to a " +
				"proceeding about different bytes"},
		{"intent", func(r *Record) { r.Intent = "We agree to something else entirely" },
			"the thing everyone is agreeing to could be changed without breaking the commitment"},
		{"expires", func(r *Record) { r.Expires = r.Expires.Add(24 * time.Hour) },
			"the deadline is an externally-supplied security parameter (D16 clock 3)"},
		{"a fingerprint", func(r *Record) { r.Roster[1].Fingerprint = other },
			"a party could be swapped for another and every signature would still verify " +
				"against the same commitment"},
		{"the signs flag", func(r *Record) { r.Roster[0].Signs = true },
			"a convener could present one roster to the signers and another to a verifier, " +
				"differing only in who was obliged to sign, and both would hash the same — " +
				"this is the whole class PLAN-2 exists to catch"},
		{"a label", func(r *Record) { r.Roster[1].Label = "Someone Else" },
			"the displayed roster could differ from the committed one"},
		{"roster order", func(r *Record) { r.Roster[0], r.Roster[1] = r.Roster[1], r.Roster[0] },
			"the order IS the signing order (D20), so reordering must break the commitment"},
	} {
		r := base
		r.Roster = append([]Party(nil), base.Roster...)
		c.mut(&r)
		got, err := r.RosterHash()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if string(got) == string(want) {
			t.Errorf("changing %s left the commitment unchanged — %s", c.name, c.why)
		}
	}
}

// TestRosterHashIsNotAmbiguousAcrossFieldBoundaries: the axes are length-prefixed, so two
// records that differ only in where a boundary falls must not collide.
//
// Without prefixes, a label ending in "x" followed by an empty next field is
// indistinguishable from an empty label followed by a field beginning "x" — the classic
// concatenation ambiguity, and it is exactly how a crafted label forges a commitment.
func TestRosterHashIsNotAmbiguousAcrossFieldBoundaries(t *testing.T) {
	_, _, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")

	one := draft(t, cfp, afp)
	one.Roster[1].Label = "AB"
	two := draft(t, cfp, afp)
	two.ID = one.ID
	two.Roster[1].Label = "A"
	two.Roster = append(two.Roster, Party{Fingerprint: afp, Label: "B", Signs: true})

	h1, err := one.RosterHash()
	if err != nil {
		t.Fatal(err)
	}
	h2, err := two.RosterHash()
	if err != nil {
		t.Fatal(err)
	}
	if string(h1) == string(h2) {
		t.Error("two records differing only in where a field boundary falls hash the same — " +
			"the axes are being concatenated without length prefixes, and a crafted label can " +
			"forge a commitment")
	}
}

// TestTheNameIsNotInTheCommitment is an exclusion, and an exclusion has to be tested or it
// is just a sentence in a doc comment.
//
// The six-word name is a pure function of the fingerprint (D3). Including it would tie this
// commitment to the wordlist, so a wordlist change — which the freeze permits with a
// version bump — would alter commitments already written into signed documents.
func TestTheNameIsNotInTheCommitment(t *testing.T) {
	_, _, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	base := draft(t, cfp, afp)
	want, err := base.RosterHash()
	if err != nil {
		t.Fatal(err)
	}
	base.Roster[1].Name = "totally different six words here"
	got, err := base.RosterHash()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Error("the six-word name changed the commitment. It is a function of the fingerprint, " +
			"so this ties every signed commitment to the wordlist — and a wordlist change would " +
			"invalidate records already signed")
	}
}

// --- the convener signature ---------------------------------------------------

func TestRecordSignsAndVerifies(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	r := draft(t, cfp, afp)
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	if r.Version != FormatVersion {
		t.Errorf("Sign left the version at %d, want %d — the version is the first axis of the "+
			"preimage, so an unset one commits to the wrong thing", r.Version, FormatVersion)
	}
	if err := r.Verify(time.Now()); err != nil {
		t.Fatalf("a freshly signed record does not verify: %v", err)
	}
	if c, ok := r.Convener(); !ok || c.Signs {
		t.Errorf("Convener() = %+v, ok=%v — want the non-signing roster entry", c, ok)
	}
}

// TestATamperedRecordIsRefusedByName covers the acceptance clause: "a record whose convener
// signature does not verify is refused before any pairing, with a distinct message".
//
// Distinct matters. A malformed record is a broken file; a record whose signature fails is
// a record somebody changed. Reporting both as "could not read the ceremony" tells the user
// to try again in one case and to stop in the other.
func TestATamperedRecordIsRefusedByName(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	r := draft(t, cfp, afp)
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	if err := r.Verify(time.Now()); err != nil {
		t.Fatalf("setup: the record does not verify before tampering: %v", err)
	}

	for _, c := range []struct {
		name string
		mut  func(r *Record)
	}{
		{"the intent", func(r *Record) { r.Intent = "We agree to something nobody said" }},
		{"a fingerprint", func(r *Record) { r.Roster[1].Fingerprint = strings.Repeat("11", 32) }},
		{"the signs flag", func(r *Record) { r.Roster[0].Signs = true }},
		{"the deadline", func(r *Record) { r.Expires = r.Expires.Add(365 * 24 * time.Hour) }},
	} {
		bad := r
		bad.Roster = append([]Party(nil), r.Roster...)
		c.mut(&bad)
		err := bad.Verify(time.Now())
		if !errors.Is(err, ErrBadConvenerSignature) {
			t.Errorf("altering %s gave %v, want ErrBadConvenerSignature", c.name, err)
		}
	}
}

// TestARecordSignedByAnOutsiderIsRefusedDifferently: a signature that verifies but belongs
// to nobody in the roster is a well-formed lie, not a corruption, and says so.
func TestARecordSignedByAnOutsiderIsRefusedDifferently(t *testing.T) {
	_, _, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	outCert, outKey, _ := identity(t, "Outsider")

	r := draft(t, cfp, afp)
	if err := r.Sign(outCert, outKey); err != nil {
		t.Fatal(err)
	}
	err := r.Verify(time.Now())
	if !errors.Is(err, ErrConvenerNotInRoster) {
		t.Errorf("a record signed by someone outside its roster gave %v, want "+
			"ErrConvenerNotInRoster — it is internally consistent and describes a proceeding "+
			"its own signer is not part of, which is a different failure from tampering", err)
	}
	if errors.Is(err, ErrBadConvenerSignature) {
		t.Error("the two failures are indistinguishable, so a user cannot tell a corrupted " +
			"record from a forged one")
	}
}

// TestAnUnknownVersionIsRefused (D32): the version is the first axis, and a record from a
// future format must be refused rather than parsed optimistically.
func TestAnUnknownVersionIsRefused(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	r := draft(t, cfp, afp)
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	r.Version = FormatVersion + 1
	if err := r.Verify(time.Now()); !errors.Is(err, ErrVersion) {
		t.Errorf("a record claiming version %d gave %v, want the version error",
			r.Version, err)
	}
}

// TestTheVersionIsWrittenAtCreation is the D32 pin, driven by reading it back rather than
// by trusting Sign to have set it.
//
// P07's skew bullet tests two versions meeting; it says nothing about whether the field
// existed three phases earlier — and rosterHash's preimage BEGINS with it, so a record
// written without one commits to zero and every later comparison is against the wrong
// number.
func TestTheVersionIsWrittenAtCreation(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	r := draft(t, cfp, afp) // deliberately not setting Version
	if r.Version != 0 {
		t.Fatal("setup: the draft already carries a version, so Sign writing one proves nothing")
	}
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	b, err := r.Encode()
	if err != nil {
		t.Fatal(err)
	}
	back, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if back.Version != FormatVersion {
		t.Errorf("the encoded record carries version %d, want %d — read back off the bytes, "+
			"not off the struct that was signed", back.Version, FormatVersion)
	}
	if err := back.Verify(time.Now()); err != nil {
		t.Errorf("the record does not survive an encode/decode round trip: %v", err)
	}
}

// --- the document ------------------------------------------------------------

// TestARecordSurvivesIncrementalSignatures is caveat 10, driven rather than read out of
// pdfcpu's documentation — and the arc's own failure-mode #1 is a "verified" claim about a
// dependency that was never re-verified.
//
// Three incremental signatures, and the record must still be readable, still verify, and
// the document must still hash to what the record says. The last is the hop-4 clause: the
// convener's own bytes satisfy a round-trip without anyone recomputing anything, so the
// recompute has to happen after somebody else has signed.
func TestARecordSurvivesIncrementalSignatures(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	aCert, aKey, afp := identity(t, "A")
	bCert, bKey, bfp := identity(t, "B")

	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	r := draft(t, cfp, afp, bfp)
	hash, err := DocumentHash(base)
	if err != nil {
		t.Fatal(err)
	}
	r.DocHash = hash
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	doc, err := Embed(base, r)
	if err != nil {
		t.Fatal(err)
	}

	// The stimulus, asserted before anything is graded: the record really is in there and
	// really checks out before a single signature is added.
	if _, err := CheckDocument(doc, time.Now()); err != nil {
		t.Fatalf("setup: the freshly embedded record does not check out: %v", err)
	}

	for i, id := range []struct{ cert, key []byte }{{aCert, aKey}, {bCert, bKey}, {aCert, aKey}} {
		doc, err = sign.SignApproval(doc, id.cert, id.key, sign.Options{
			Name:   "signer",
			Reason: "co-signing",
			When:   time.Now(),
		})
		if err != nil {
			t.Fatalf("signature %d: %v", i+1, err)
		}
	}

	// The stimulus for the assertion below: there really are three signatures on it now.
	st := sign.Verify(doc)
	if n := len(st.Signers); n != 3 {
		t.Fatalf("setup: the document carries %d signatures, want 3 — the recompute below "+
			"would not be crossing any incremental updates", n)
	}
	if st.State != sign.Valid {
		t.Fatalf("setup: the signatures do not verify (%s), so this is not the case caveat 10 names", st.State)
	}

	// Caveat 10, and the hop-4 clause with it: a party holding the document after three
	// incremental signatures reads the record, verifies it, and recomputes the hash itself.
	got, err := CheckDocument(doc, time.Now())
	if err != nil {
		t.Fatalf("after three incremental signatures the record no longer checks out: %v", err)
	}
	if got.ID != r.ID {
		t.Errorf("the record came back with id %s, want %s", got.ID, r.ID)
	}
	if got.DocHash != hash {
		t.Errorf("the recomputed document hash is %s, the record says %s — a later party "+
			"cannot prove it holds the document the ceremony was written for", got.DocHash, hash)
	}
}

// TestEmbedRefusesASignedDocument — the same refusal PrepareDocument already makes about
// the readme, for the same measured reason: a structural rewrite invalidates every
// signature on the file.
func TestEmbedRefusesASignedDocument(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	aCert, aKey, afp := identity(t, "A")
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	signed, err := sign.SignApproval(base, aCert, aKey, sign.Options{Name: "A", When: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if sign.Verify(signed).State != sign.Valid {
		t.Fatal("setup: the document is not signed, so the refusal below proves nothing")
	}
	r := draft(t, cfp, afp)
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	if _, err := Embed(signed, r); err == nil {
		t.Error("a ceremony record was embedded into an already-signed document — the rewrite " +
			"invalidates every signature already on it")
	}
}

// TestADocumentWithoutARecordSaysSo: an ordinary PDF has no record, and that is not a
// corruption.
func TestADocumentWithoutARecordSaysSo(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CheckDocument(base, time.Now()); !errors.Is(err, ErrNoRecord) {
		t.Errorf("an ordinary PDF gave %v, want ErrNoRecord", err)
	}
}

// TestASwappedDocumentIsCaught: the record travels with the document, so the check that
// matters is that it is the document the record was written for.
func TestASwappedDocumentIsCaught(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	other, err := testpdf.Text("a different document entirely")
	if err != nil {
		t.Fatal(err)
	}

	r := draft(t, cfp, afp)
	h, err := DocumentHash(base)
	if err != nil {
		t.Fatal(err)
	}
	r.DocHash = h
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}

	// The record, valid and correctly signed, moved onto a DIFFERENT document.
	swapped, err := Embed(other, r)
	if err != nil {
		t.Fatal(err)
	}
	_, err = CheckDocument(swapped, time.Now())
	if err == nil {
		t.Fatal("a correctly-signed record was accepted on a document it was not written for")
	}
	if errors.Is(err, ErrBadConvenerSignature) {
		t.Error("reported as a signature failure — the signature is fine; the document is wrong, " +
			"and telling the user the record was tampered with sends them after the wrong thing")
	}
	if !strings.Contains(err.Error(), "not the same document") {
		t.Errorf("error = %v, want it to name the mismatch", err)
	}
}

// --- the roster token in signatures -------------------------------------------

// TestEverySignatureCarriesOneCommitment is the acceptance clause: "[NibRoster:<hash>]
// appears in each signer's /Reason and cross-binds; a document whose signers do not share
// one commitment is reported as such rather than as co-signed".
//
// Both halves are driven, and the second is the one that matters — a document where the
// signers agreed to different ceremonies must not read as a co-signed document, because
// "co-signed" is what a person acts on.
func TestEverySignatureCarriesOneCommitment(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	aCert, aKey, afp := identity(t, "A")
	bCert, bKey, bfp := identity(t, "B")

	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	r := draft(t, cfp, afp, bfp)
	h, err := DocumentHash(base)
	if err != nil {
		t.Fatal(err)
	}
	r.DocHash = h
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	tok, err := r.RosterHash()
	if err != nil {
		t.Fatal(err)
	}
	rosterHex := hex.EncodeToString(tok)

	doc, err := Embed(base, r)
	if err != nil {
		t.Fatal(err)
	}

	sameCeremony := signWithRoster(t, doc, aCert, aKey, afp, bfp, rosterHex)
	sameCeremony = signWithRoster(t, sameCeremony, bCert, bKey, bfp, afp, rosterHex)

	ats := p2p.ReadAttestations(sameCeremony)
	if len(ats) != 2 {
		t.Fatalf("setup: %d attestations, want 2", len(ats))
	}
	for i, a := range ats {
		if a.RosterHash != rosterHex {
			t.Errorf("signature %d carries roster %q, want %q", i, a.RosterHash, rosterHex)
		}
		if !a.OneProceeding {
			t.Errorf("signature %d is not reported as part of one proceeding", i)
		}
	}

	// The half that matters: two signers who committed to DIFFERENT ceremonies.
	other := draft(t, cfp, afp, bfp)
	otherTokBytes, err := other.RosterHash()
	if err != nil {
		t.Fatal(err)
	}
	otherHex := hex.EncodeToString(otherTokBytes)
	if otherHex == rosterHex {
		t.Fatal("setup: the two ceremonies share a commitment, so the split below is not a split")
	}

	split := signWithRoster(t, doc, aCert, aKey, afp, bfp, rosterHex)
	split = signWithRoster(t, split, bCert, bKey, bfp, afp, otherHex)

	sats := p2p.ReadAttestations(split)
	if len(sats) != 2 {
		t.Fatalf("setup: %d attestations on the split document, want 2", len(sats))
	}
	for i, a := range sats {
		if a.OneProceeding {
			t.Errorf("signature %d on a document whose signers committed to DIFFERENT ceremonies "+
				"is reported as part of one proceeding — a verifier would call this co-signed, "+
				"and the two people signed up to different things", i)
		}
	}
}

// signWithRoster adds one co-signature carrying a roster commitment.
func signWithRoster(t *testing.T, pdf, certPEM, keyPEM []byte, myFP, peerFP, roster string) []byte {
	t.Helper()
	place, err := p2p.NextPlacement(pdf)
	if err != nil {
		t.Fatal(err)
	}
	att := p2p.Attestation{
		Signer:       "signer",
		AcceptedPeer: peerFP,
		Intent:       "I agree to co-sign",
		When:         time.Now(),
		RosterHash:   roster,
	}
	out, err := p2p.Contribute(pdf, certPEM, keyPEM, att, nil, place)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// --- the mirror ---------------------------------------------------------------

func TestTheMirrorRoundTrips(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	r := draft(t, cfp, afp)
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	dir, err := WriteMirror(root, r, []byte("%PDF-1.7\nfake"))
	if err != nil {
		t.Fatal(err)
	}
	back, pdf, err := ReadMirror(root, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if back.ID != r.ID || string(pdf) != "%PDF-1.7\nfake" {
		t.Errorf("mirror round-trip lost data: id=%s pdf=%d bytes", back.ID, len(pdf))
	}
	if err := back.Verify(time.Now()); err != nil {
		t.Errorf("the record does not verify after a trip through the mirror: %v", err)
	}
	if err := RemoveMirror(root, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("the ceremony directory survived the prune")
	}
}

// TestTheMirrorRefusesAnUnsafeID: the id comes out of a record, and a record can arrive
// from another party — so it is attacker-controlled input being used to build a path.
func TestTheMirrorRefusesAnUnsafeID(t *testing.T) {
	root := t.TempDir()
	for _, bad := range []string{
		"../../etc", "..", "", "abc", strings.Repeat("z", 32),
		"0123456789abcdef0123456789abcde/", "0123456789abcdef0123456789abcdeF",
	} {
		if _, err := MirrorDir(root, bad); err == nil {
			t.Errorf("MirrorDir accepted %q — a ceremony id names a directory, and this one "+
				"either escapes the root or is not an id at all", bad)
		}
	}
	// The positive control: a real id is accepted, or the loop above would pass against a
	// function that refused everything.
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := MirrorDir(root, id); err != nil {
		t.Errorf("a freshly generated id was refused: %v", err)
	}
}

// TestTheMirrorHoldsNoSecret is D29's clause, and it is an ABSENCE check by design: a test
// that asserts the vault contains the invitation secret cannot see a copy left on disk
// beside the document.
//
// The mirror writes what it is given, so the guard is that nothing in this package ever
// writes a secret field — asserted over the bytes that actually land on disk.
func TestTheMirrorHoldsNoSecret(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	r := draft(t, cfp, afp)
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	inv, err := NewInvitation(r)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	dir, err := WriteMirror(root, r, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "record.json"))
	if err != nil {
		t.Fatal(err)
	}
	// The stimulus: the file really has content, so "no secret in it" is not true of an
	// empty file.
	if len(b) < 100 {
		t.Fatalf("setup: the mirrored record is %d bytes, which is not a record", len(b))
	}
	// The ACTUAL secret bytes of the invitation THIS ceremony issued — not the word
	// "secret", and not a freshly-minted one.
	//
	// The first draft called NewInvitation *after* writing the mirror, which mints a new
	// random secret every call: it searched the file for a value that had never existed
	// anywhere, so it could not have failed. Caught by probing, and it is the same shape as
	// the vacuity this repo keeps finding — an assertion whose subject was never present.
	if len(inv.Secret) != SecretLen {
		t.Fatal("setup: the invitation has no secret, so searching for it proves nothing")
	}
	for _, form := range [][]byte{
		inv.Secret,
		[]byte(hex.EncodeToString(inv.Secret)),
		[]byte(base64.StdEncoding.EncodeToString(inv.Secret)),
		[]byte(base64.RawURLEncoding.EncodeToString(inv.Secret)),
	} {
		if bytes.Contains(b, form) {
			t.Errorf("the invitation secret is in the mirrored record, encoded as %.12s… . The "+
				"mirror is ordinary files under the user's home; the secret belongs in the vault, "+
				"which is sealed to the user's SSH key (D29)", form)
		}
	}
	// The field-name scan too, which catches a secret stored in a form this test did not
	// think of encoding.
	for _, forbidden := range []string{"secret", "Secret", "privateKey"} {
		if strings.Contains(string(b), forbidden) {
			t.Errorf("the mirrored record contains the field name %q", forbidden)
		}
	}
}

// TestTheRosterHashBindsWhoConvened.
//
// v2 left the convener outside the roster preimage, and the omission was **unargued** —
// `RosterHash`'s exclusion list named only the six-word name and the secret, so this looked
// exactly like a decision. It was not: any roster member could re-sign an unchanged roster
// with their own key and `Verify` still passed, because it asks only that the signer appear
// SOMEWHERE in the roster. The hash was byte-identical, so `RosterToken` was too, and a
// verifier reading the finished document could not tell which of them convened.
//
// The FINGERPRINT is bound, not the certificate: a cert can be re-issued for the same key
// and the same identity, and binding the bytes would change the token for a ceremony that
// has not changed.
func TestTheRosterHashBindsWhoConvened(t *testing.T) {
	certA, keyA, fpA := identity(t, "Convener")
	certB, keyB, fpB := identity(t, "Other")
	rec := draft(t, fpA, fpB)
	if err := rec.Sign(certA, keyA); err != nil {
		t.Fatal(err)
	}
	first, err := rec.RosterHash()
	if err != nil {
		t.Fatal(err)
	}

	// The other roster member re-signs the IDENTICAL roster.
	forged := rec
	if err := forged.Sign(certB, keyB); err != nil {
		t.Fatal(err)
	}
	// STIMULUS: the roster itself did not change, so any difference below is the convener
	// axis and nothing else.
	if len(forged.Roster) != len(rec.Roster) {
		t.Fatal("setup: the roster changed, so this measures more than the convener")
	}
	for i := range rec.Roster {
		if forged.Roster[i] != rec.Roster[i] {
			t.Fatal("setup: a roster entry changed")
		}
	}
	// And it still VERIFIES — the forgery is not prevented, it is made visible. Saying so
	// is the point: a test that asserted refusal here would be describing a property this
	// change does not have.
	if err := forged.Verify(time.Now()); err != nil {
		t.Fatalf("setup: the re-signed record does not verify (%v), so this is not the "+
			"in-roster case the binding is about", err)
	}

	second, err := forged.RosterHash()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, second) {
		t.Error("the roster hash is unchanged when a different roster member convenes — " +
			"the token in a finished document names a set of parties and not who called them")
	}

	// The control: re-signing with the SAME identity must reproduce the same hash, or the
	// token stops being a stable commitment to the ceremony.
	again := rec
	if err := again.Sign(certA, keyA); err != nil {
		t.Fatal(err)
	}
	third, err := again.RosterHash()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, third) {
		t.Error("re-signing with the same identity changed the roster hash — the token is " +
			"not a stable commitment to the ceremony")
	}
}

// TestACeremonyDeadlineHasACeiling — the plan's fourth externally-supplied security
// parameter, and its own words are "enforced, not documented".
//
// D16 calls clock 3 "days, convener's choice", which is a parameter with no bound. It
// governs how long a listener may arm, how long invitation-scoped pins persist (D29) and how
// long a mirror lives — so a convener setting ten years was a config away, and nothing in
// the tree said otherwise. Before this, `Record.Expires` was read at exactly ONE line in the
// whole repo (the roster preimage) and never compared to a clock at all.
func TestACeremonyDeadlineHasACeiling(t *testing.T) {
	cert, key, cfp := identity(t, "convener")
	_, _, afp := identity(t, "alice")
	now := time.Now()

	sign := func(d time.Duration) Record {
		t.Helper()
		r := draft(t, cfp, afp)
		r.Expires = now.Add(d)
		if err := r.Sign(cert, key); err != nil {
			t.Fatal(err)
		}
		return r
	}

	// SETUP: a deadline inside the ceiling verifies. Without this the refusal below is
	// equally true of a Verify that refuses everything, which would make every ceremony
	// unopenable and every other test in this file fail for a reason nobody would read.
	ok := sign(MaxCeremonyLife - time.Hour)
	if err := ok.Verify(now); err != nil {
		t.Fatalf("setup: a deadline inside the ceiling was refused (%v), so the ceiling "+
			"assertion below cannot distinguish a bound from a blanket refusal", err)
	}

	over := sign(MaxCeremonyLife + time.Hour)
	err := over.Verify(now)
	if !errors.Is(err, ErrCeremonyTooLong) {
		t.Errorf("a deadline %s ahead verified as %v; want ErrCeremonyTooLong. This is an "+
			"externally-supplied security parameter and the plan's own rule for all four of "+
			"them is that they are enforced rather than documented.",
			MaxCeremonyLife+time.Hour, err)
	}

	// **An EXPIRED ceremony still verifies, and that is the half a ceiling gets wrong.**
	// An expired record is a liveness fact about the proceeding, not a validity fact about
	// the document: a verifier reading a finished PDF next year must still be able to check
	// who was on the roster and that the convener signed. A Verify that refused a past
	// deadline would make every completed ceremony unverifiable the day after it ended.
	past := sign(-90 * 24 * time.Hour)
	if err := past.Verify(now); err != nil {
		t.Errorf("a ceremony that ended three months ago no longer verifies (%v) — a signed "+
			"record must stay checkable after the proceeding it describes is over, or the "+
			"document's own evidence expires with it", err)
	}

	// And the ordering: a record that is BOTH forged and over-long is reported as forged.
	// A convener who signed a ten-year deadline is a misconfiguration and the fix is theirs;
	// one who did not sign at all is an attacker, and that is the sentence a user needs.
	forged := sign(MaxCeremonyLife + time.Hour)
	forged.Roster[1].Fingerprint = afp[:len(afp)-2] + "ff"
	if err := forged.Verify(now); errors.Is(err, ErrCeremonyTooLong) {
		t.Errorf("a record with a broken roster AND an over-long deadline was reported as "+
			"too long; the signature failure is the one that matters and must be reported "+
			"first (got %v)", err)
	}
}

// TestTheHopNumberComesFromTheSignedRoster — the definition that did not exist.
//
// Eight functions take a `hop`, including every key, salt and seed derivation, and until now
// nothing in the tree derived one or checked one: the only assignment anywhere was a literal
// `0` in a self-test. A caller's off-by-one produced a valid key and salt at a hop that does
// not exist, published into the void, and the result read as `FetchEmpty` — which is
// indistinguishable from a counterparty who has not arrived yet.
func TestTheHopNumberComesFromTheSignedRoster(t *testing.T) {
	_, _, c := identity(t, "convener")
	_, _, a := identity(t, "alice")
	_, _, b := identity(t, "bob")
	r := draft(t, c, a, b) // roster: convener, alice, bob

	if got := r.Hops(); got != 2 {
		t.Fatalf("setup: a three-party roster must have 2 hops, got %d — every assertion "+
			"below is about the mapping and needs the count to be right first", got)
	}

	// Adjacency IS the hop, and both directions give the same number: the two ends of one
	// hop must agree without negotiating, which is the whole reason this is read off a
	// convener-signed artifact rather than counted.
	for _, tc := range []struct {
		x, y string
		want int
		what string
	}{
		{c, a, 0, "convener→alice"},
		{a, c, 0, "alice→convener, the same hop from the other end"},
		{a, b, 1, "alice→bob"},
		{b, a, 1, "bob→alice"},
	} {
		got, err := r.Hop(tc.x, tc.y)
		if err != nil {
			t.Errorf("%s: %v", tc.what, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s is hop %d, want %d", tc.what, got, tc.want)
		}
	}

	// **Non-adjacent parties share no hop — this is criterion 19 made structural.** The
	// convener holds candidates for every party, and the rule is that it never dials a
	// later one during this hop. Enforced here, it cannot even derive that hop's key by
	// asking for it, so the rule is a property rather than a discipline.
	if _, err := r.Hop(c, b); !errors.Is(err, ErrNotAHop) {
		t.Errorf("convener and bob are two apart and got hop %v; a hop joins adjacent "+
			"parties, and letting a non-adjacent pair name one is how a convener ends up "+
			"dialling a party three positions away", err)
	}
	if _, err := r.Hop(a, a); !errors.Is(err, ErrNotAHop) {
		t.Error("one party was accepted as both ends of a hop")
	}
	stranger := strings.Repeat("cd", 32)
	if _, err := r.Hop(a, stranger); !errors.Is(err, ErrNotAHop) {
		t.Error("a fingerprint outside the roster was given a hop number")
	}

	// Case, because a fingerprint differing only in case is the same party — the same fact
	// ParseInvitation normalises for, and that Convener() gets wrong one function away.
	if got, err := r.Hop(strings.ToUpper(c), a); err != nil || got != 0 {
		t.Errorf("an upper-case fingerprint did not resolve to its own party (%d, %v) — the "+
			"two ends of a hop would then derive different keys for it", got, err)
	}
}

// TestBothRostersUseOneHopRule — ADR-009 applied to a rule that has two natural homes.
//
// Record and Invitation both carry a roster and both get asked for hop numbers. Two
// implementations would be worse than the usual duplicate: they would agree on every roster
// anyone tested and disagree exactly where order was subtle — a party appearing twice, a
// case-differing fingerprint, an off-by-one at the ends.
func TestBothRostersUseOneHopRule(t *testing.T) {
	cert, key, c := identity(t, "convener")
	_, _, a := identity(t, "alice")
	_, _, b := identity(t, "bob")
	r := draft(t, c, a, b)
	// Signed, because NewInvitation resolves the convener from the record's own certificate
	// — an unsigned draft has none and is refused.
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	inv, err := NewInvitation(r)
	if err != nil {
		t.Fatal(err)
	}

	// SETUP: both rosters really are the same, or "they agree" is trivially true of two
	// different questions.
	if len(inv.Roster) != len(r.Roster) {
		t.Fatalf("setup: the invitation's roster has %d parties and the record's %d",
			len(inv.Roster), len(r.Roster))
	}
	if inv.Hops() != r.Hops() {
		t.Errorf("hop counts differ: invitation %d, record %d", inv.Hops(), r.Hops())
	}

	for _, pair := range [][2]string{{c, a}, {a, b}, {b, a}, {c, b}, {a, a}} {
		rh, rerr := r.Hop(pair[0], pair[1])
		ih, ierr := inv.Hop(pair[0], pair[1])
		if (rerr == nil) != (ierr == nil) {
			t.Errorf("%s/%s: record says %v and invitation says %v — one rule, two answers",
				short(pair[0]), short(pair[1]), rerr, ierr)
			continue
		}
		if rerr == nil && rh != ih {
			t.Errorf("%s/%s: record says hop %d and invitation says hop %d",
				short(pair[0]), short(pair[1]), rh, ih)
		}
	}
}
