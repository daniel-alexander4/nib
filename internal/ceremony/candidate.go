package ceremony

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"golang.org/x/crypto/chacha20poly1305"

	"nib/internal/sign"
)

// The candidate record: what one party publishes at one hop so the other can dial it
// (D6, D30, plan-review C2).
//
// # Two boundaries, and the whole design is keeping them apart
//
// The CONFIDENTIALITY boundary is the ceremony. Everyone holding the invitation can read
// every candidate record of every hop, and that is by construction: D6's amendment keys
// the encryption to the invitation secret, and every roster member holds it.
//
// The AUTHENTICITY boundary is the PARTY, and it has to be drawn somewhere else, because
// the two boundaries being the same thing is exactly the defect the pending item filed on
// 2026-08-19 describes: within the roster, one party could otherwise publish candidates
// *as another*. The failure is quiet — a forged candidate does not break L1, it cannot
// substitute a signer, it just redirects punch traffic — so no existing acceptance
// criterion sees it.
//
// # Why the DHT's own signature cannot draw that line, though the plan said it could
//
// P04.S03's original acceptance argued that "BEP-44 mutable items are ed25519-signed by
// construction, so a per-party key makes the DHT itself refuse the write". Two facts kill
// it, and both were read off the tree rather than reasoned:
//
//   - Nib's identity key is **ECDSA P-256** (sign/identity.go:60), and BEP-44's is a
//     32-byte ed25519 public key with a 64-byte ed25519 signature. The identity key
//     cannot be the BEP-44 key.
//   - Any key derived from the invitation secret is derivable by **every roster member**,
//     since they all hold the secret. So a "per-party" secret-derived key makes the DHT
//     refuse nothing that any member wishes to write.
//
// The BEP-44 signature is therefore a GROUP credential: it proves the writer held the
// invitation, which keeps outsiders out and says nothing about which insider wrote it.
//
// # So the line is drawn here, by the identity key, over these bytes
//
// D31 is what made this possible and the old rejection stale: the invitation pins FULL
// 32-byte fingerprints, so a reader holds something to verify against before any
// handshake. The record carries the signer's SPKI (91 bytes, measured), the reader
// verifies the ECDSA signature against it and then checks that its SHA-256 is the roster
// fingerprint of the party this record claims to be. An invitation holder can encrypt,
// can publish, and cannot produce that signature — identity private keys live in each
// party's own vault and the invitation is not a signing credential.
//
// # What this does NOT stop, recorded so it is not assumed away
//
// **Preemption.** Any invitation holder derives the hop key and can publish at
// `seq = MaxInt64`; `CheckIncoming` then refuses every later write at that target
// (bep44/item.go:151-165) and the honest party's silence is indistinguishable from being
// offline. The inner signature does not address this and is not meant to. What makes it
// visible is counting "a record was present and failed this check" separately from
// "nothing was there" — filed against P04.S04, whose scope is already the flood cap.

// candidateDomain separates this preimage from every other thing the identity key signs.
// On-wire identifier: immutable once shipped (STANDARDS §9). Its counterpart is
// rosterDomain in record.go, and the pair is the point — see RosterHash's tag comment.
const candidateDomain = "nib-candidate-record-v1"

// CandidateFormatVersion is inside the preimage, first after the domain tag (D32).
const CandidateFormatVersion = 1

// MaxCandidates bounds one record. Eight is D33's candidate cap and the DHT's own bucket
// size, and it keeps the sealed record inside BEP-44's value cap with room to spare.
const MaxCandidates = 8

// MaxSealedRecord is the ceiling a sealed record must respect.
//
// BEP-44 refuses a value whose BENCODED length exceeds 1000 (bep44/item.go:132), and a
// byte string of n bytes bencodes to len(itoa(n))+1+n — so 996 is the real payload
// ceiling. The check belongs here rather than at the publisher, because the failure it
// prevents is silent: an over-size value is refused by our OWN store inside
// dht.Server.Put (server.go:1081) before any datagram is sent, getput.Put logs a warning
// per node and returns nil, and the record simply never leaves the machine.
const MaxSealedRecord = 996

var (
	// ErrCandidateFormat: the plaintext is not a candidate record of a version we know.
	ErrCandidateFormat = errors.New("this is not a candidate record this version of Nib understands")
	// ErrCandidateSignature: it decrypted and its signature does not verify. Distinct
	// from ErrCandidateAuthor because they mean different things about who is at fault.
	ErrCandidateSignature = errors.New("the candidate record's signature does not verify")
	// ErrCandidateAuthor: the signature verifies and the signer is not who the record
	// says, or is not in the roster at all. This is the in-roster forgery refusal.
	ErrCandidateAuthor = errors.New("the candidate record was signed by someone other than the party it claims")
	// ErrCandidateExpired: valid, and too old to act on.
	ErrCandidateExpired = errors.New("the candidate record has expired")
	// ErrCandidateTooBig: it will not fit in a BEP-44 value.
	ErrCandidateTooBig = errors.New("the candidate record is too large to publish")
	// ErrCandidateContext: it is validly signed, by the right party, for a DIFFERENT
	// ceremony or hop. Its own error because it is the only failure here that implicates
	// nobody: the signer did nothing wrong, and someone in between moved their record.
	ErrCandidateContext = errors.New("the candidate record belongs to another ceremony or hop")
)

// CandidateRecord is one party's endpoints at one hop.
type CandidateRecord struct {
	Version int
	// CeremonyID binds the record to this ceremony. Without it a roster member who is in
	// two ceremonies with the same counterparty could lift a signed record from one and
	// republish it in the other — the signature would verify and the fingerprint would
	// match, because neither mentions a ceremony.
	CeremonyID string
	// Hop binds it to this hop. The ed25519 key is per hop, which makes it LOOK already
	// bound — but every roster member derives every hop's key from the secret, so a hop-0
	// record is publishable at hop 1 by anyone who holds the invitation.
	Hop int
	// SPKI is the signer's raw SubjectPublicKeyInfo. Its SHA-256 is the pin.
	SPKI []byte
	// Expires is absolute UTC. It is the ENTIRE freshness margin: BEP-44's `seq` cannot
	// go in the preimage (getput chooses it at publish time), and a recorded ciphertext
	// is replayable by anyone who saw it at a higher seq.
	Expires time.Time
	// Addrs are the endpoints to dial.
	Addrs []netip.AddrPort
	// Sig is ECDSA-P256-ASN.1 over the preimage digest.
	Sig []byte
}

// Fingerprint is the pin of whoever signed this record.
func (c CandidateRecord) Fingerprint() string {
	return hex.EncodeToString(sign.FingerprintSPKI(c.SPKI))
}

// preimage is the exact byte string the signature covers, and — with the signature
// appended — the exact byte string that is encrypted and published.
//
// The thing signed and the thing sent are the same bytes. That removes the question
// record.go had to answer separately (sign the JSON or sign a hash of it?): there is no
// second encoding to disagree with the first, and no field order or whitespace a
// re-encode can move.
//
// Every axis is length-prefixed with a fixed 8-byte big-endian count, copied from
// RosterHash (record.go:100-105) so no byte can migrate across a boundary — the axes here
// are variable-length and attacker-influenceable, which is precisely when that matters.
//
// # Deliberately OUTSIDE, each with its reason
//
//   - The BEP-44 `seq`: it is chosen by the publish traversal after the record is built
//     (getput.go:154), so it cannot be signed. It is bound by the AEAD's associated data
//     instead, which is why Seal takes it.
//   - The salt: a pure function of the hop and the party fingerprint, both of which ARE
//     in the preimage, so signing it would bind the same fact twice. It is in the AAD.
//   - The party's six-word name and label: pure functions of the fingerprint, and
//     including them would let a wordlist change alter a commitment — the same reasoning
//     that keeps Party.Name out of RosterHash.
func (c CandidateRecord) preimage() ([]byte, error) {
	if len(c.Addrs) > MaxCandidates {
		return nil, fmt.Errorf("%w: %d candidates, cap is %d", ErrCandidateTooBig, len(c.Addrs), MaxCandidates)
	}
	if len(c.SPKI) == 0 {
		return nil, errors.New("candidate record has no public key")
	}
	var b []byte
	lp := func(v []byte) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(v)))
		b = append(b, n[:]...)
		b = append(b, v...)
	}
	num := func(n int) []byte {
		var v [8]byte
		binary.BigEndian.PutUint64(v[:], uint64(n))
		return v[:]
	}
	lp([]byte(candidateDomain))
	lp(num(c.Version))
	lp([]byte(c.CeremonyID))
	lp(num(c.Hop))
	lp(c.SPKI)
	lp([]byte(c.Expires.UTC().Format(time.RFC3339)))
	lp(num(len(c.Addrs)))
	for _, a := range c.Addrs {
		lp([]byte(a.String()))
	}
	return b, nil
}

// Sign fills SPKI and Sig from the identity's certificate and key.
func (c *CandidateRecord) Sign(certPEM, keyPEM []byte) error {
	spki, err := sign.SPKIFromCert(certPEM)
	if err != nil {
		return err
	}
	c.Version = CandidateFormatVersion
	c.SPKI = spki
	pre, err := c.preimage()
	if err != nil {
		return err
	}
	sum := sha256.Sum256(pre)
	sig, err := sign.SignDigest(sum[:], keyPEM)
	if err != nil {
		return fmt.Errorf("sign candidate record: %w", err)
	}
	c.Sig = sig
	return nil
}

// Verify checks the context, the signature, the author, and the clock — in that order,
// because they fail differently and a caller wants to know which.
//
// inv supplies the ceremony id and the roster. hop is the hop the caller fetched from.
// want is the fingerprint of the party this record is supposed to be from — the salt
// named it, and the salt is addressing rather than authentication, so this is where the
// claim gets checked.
//
// # Binding a field and checking it are two different jobs, and this file shipped with
// # only the first
//
// The ceremony id and the hop are inside the preimage, which stops an attacker EDITING
// them: any change invalidates the signature. It does not stop an attacker REUSING a
// record that was validly signed elsewhere, because the signature is perfectly good — it
// simply attests to a different context. Caught by TestACandidateFromAnotherCeremonyIsRefused,
// which went red against the first version of this function: a record signed by A for
// ceremony one verified in ceremony two, since A was in both rosters and nothing asked
// which ceremony the bytes claimed.
//
// So both fields are compared here, not merely covered by the signature.
func (c CandidateRecord) Verify(inv Invitation, hop int, want string, now time.Time) error {
	if c.Version != CandidateFormatVersion {
		return fmt.Errorf("%w: version %d", ErrCandidateFormat, c.Version)
	}
	if c.CeremonyID != inv.ID {
		return fmt.Errorf("%w: the record is for ceremony %s and this is %s",
			ErrCandidateContext, short(c.CeremonyID), short(inv.ID))
	}
	if c.Hop != hop {
		return fmt.Errorf("%w: the record is for hop %d and this is hop %d",
			ErrCandidateContext, c.Hop, hop)
	}
	roster := inv.Roster
	pre, err := c.preimage()
	if err != nil {
		return err
	}
	sum := sha256.Sum256(pre)
	if err := sign.VerifyDigestSPKI(sum[:], c.Sig, c.SPKI); err != nil {
		return ErrCandidateSignature
	}
	got := c.Fingerprint()
	if got != want {
		return fmt.Errorf("%w: signed by %s, expected %s", ErrCandidateAuthor, short(got), short(want))
	}
	inRoster := false
	for _, p := range roster {
		if p.Fingerprint == got {
			inRoster = true
			break
		}
	}
	if !inRoster {
		return fmt.Errorf("%w: %s is not in this ceremony's roster", ErrCandidateAuthor, short(got))
	}
	if !now.Before(c.Expires) {
		return fmt.Errorf("%w: it expired at %s", ErrCandidateExpired, c.Expires.UTC().Format(time.RFC3339))
	}
	return nil
}

// Seal encrypts a signed record for publication.
//
// # XChaCha20-Poly1305, and why not the AES-GCM this repo uses everywhere else
//
// The vault's AEAD is AES-256-GCM with a 12-byte random nonce (vault.go:944-965), and
// consistency would say copy it. Correctness says otherwise, and correctness outranks
// consistency here. This setting is the nonce-reuse trap in its textbook form: ONE fixed
// key (RecordKey is a pure function of the secret and the hop), TWO independent writers
// (both parties of the hop publish under it), periodic REPUBLICATION, and process
// restarts designed in. Under AES-GCM two ciphertexts sharing a (key, nonce) leak the
// XOR of the plaintexts and — via the forbidden-attack — the authentication subkey,
// after which an attacker forges records that authenticate under the record key for the
// rest of the ceremony. A 96-bit random nonce is safe here only by counting messages,
// which is an argument that has to stay true as the design changes.
//
// A 192-bit random nonce removes the argument instead of winning it. The cost is 24 bytes
// of a 996-byte budget and no new dependency: golang.org/x/crypto is already required.
//
// # The associated data is everything that lives outside the ciphertext
//
// salt, hop, version and seq are all visible to, or chosen by, someone other than us —
// the salt travels in cleartext in the put, the seq is picked by the traversal. Binding
// them here means an attacker who rewrites any of them gets a decryption failure rather
// than a record that reads as ours under different circumstances.
func (c CandidateRecord) Seal(recordKey, salt []byte, hop int, seq int64) ([]byte, error) {
	if len(c.Sig) == 0 {
		return nil, errors.New("cannot seal an unsigned candidate record")
	}
	pre, err := c.preimage()
	if err != nil {
		return nil, err
	}
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(c.Sig)))
	plain := append(append([]byte{}, pre...), n[:]...)
	plain = append(plain, c.Sig...)

	aead, err := chacha20poly1305.NewX(recordKey)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := aead.Seal(nonce, nonce, plain, candidateAAD(salt, hop, seq))
	if len(out) > MaxSealedRecord {
		return nil, fmt.Errorf("%w: %d bytes, cap is %d", ErrCandidateTooBig, len(out), MaxSealedRecord)
	}
	return out, nil
}

// OpenCandidate decrypts and parses. It does NOT verify — Verify is a separate call
// because a caller needs the roster and the expected fingerprint to do it, and folding
// them in would let a caller who has neither believe it had checked something.
func OpenCandidate(recordKey, salt []byte, hop int, seq int64, sealed []byte) (CandidateRecord, error) {
	aead, err := chacha20poly1305.NewX(recordKey)
	if err != nil {
		return CandidateRecord{}, err
	}
	if len(sealed) < chacha20poly1305.NonceSizeX {
		return CandidateRecord{}, ErrCandidateFormat
	}
	nonce, ct := sealed[:chacha20poly1305.NonceSizeX], sealed[chacha20poly1305.NonceSizeX:]
	plain, err := aead.Open(nil, nonce, ct, candidateAAD(salt, hop, seq))
	if err != nil {
		return CandidateRecord{}, ErrCandidateFormat
	}
	return parseCandidate(plain)
}

// candidateAAD is the associated data, built the same length-prefixed way as the preimage
// so the same reasoning about boundaries applies.
func candidateAAD(salt []byte, hop int, seq int64) []byte {
	var b []byte
	lp := func(v []byte) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(v)))
		b = append(b, n[:]...)
		b = append(b, v...)
	}
	var h, s [8]byte
	binary.BigEndian.PutUint64(h[:], uint64(hop))
	binary.BigEndian.PutUint64(s[:], uint64(seq))
	lp([]byte(candidateDomain))
	lp([]byte{byte(CandidateFormatVersion)})
	lp(salt)
	lp(h[:])
	lp(s[:])
	return b
}

// parseCandidate walks the length-prefixed plaintext.
//
// Every read is bounds-checked against the remaining bytes, and a short or over-long
// chunk refuses the WHOLE record rather than returning what was parsed so far. The
// discipline is P03.S01's, earned there: three length-mismatch paths in the announcement
// parser returned a partially-filled struct alongside their error, and a caller who
// ignores an error is the ordinary mistake on a path that reads from strangers.
func parseCandidate(b []byte) (CandidateRecord, error) {
	var c CandidateRecord
	next := func() ([]byte, bool) {
		if len(b) < 8 {
			return nil, false
		}
		n := binary.BigEndian.Uint64(b[:8])
		// The length must be representable AND present. A declared length longer than
		// what is in hand is the allocation-bomb shape this repo already paid for once.
		if n > uint64(len(b)-8) {
			return nil, false
		}
		v := b[8 : 8+n]
		b = b[8+n:]
		return v, true
	}
	num := func() (int, bool) {
		v, ok := next()
		if !ok || len(v) != 8 {
			return 0, false
		}
		return int(binary.BigEndian.Uint64(v)), true
	}
	dom, ok := next()
	if !ok || string(dom) != candidateDomain {
		return CandidateRecord{}, ErrCandidateFormat
	}
	if c.Version, ok = num(); !ok {
		return CandidateRecord{}, ErrCandidateFormat
	}
	id, ok := next()
	if !ok {
		return CandidateRecord{}, ErrCandidateFormat
	}
	c.CeremonyID = string(id)
	if c.Hop, ok = num(); !ok {
		return CandidateRecord{}, ErrCandidateFormat
	}
	if c.SPKI, ok = next(); !ok {
		return CandidateRecord{}, ErrCandidateFormat
	}
	exp, ok := next()
	if !ok {
		return CandidateRecord{}, ErrCandidateFormat
	}
	t, err := time.Parse(time.RFC3339, string(exp))
	if err != nil {
		return CandidateRecord{}, ErrCandidateFormat
	}
	c.Expires = t
	n, ok := num()
	if !ok || n < 0 || n > MaxCandidates {
		return CandidateRecord{}, ErrCandidateFormat
	}
	for i := 0; i < n; i++ {
		v, ok := next()
		if !ok {
			return CandidateRecord{}, ErrCandidateFormat
		}
		ap, err := netip.ParseAddrPort(string(v))
		if err != nil {
			return CandidateRecord{}, ErrCandidateFormat
		}
		c.Addrs = append(c.Addrs, ap)
	}
	if c.Sig, ok = next(); !ok || len(c.Sig) == 0 {
		return CandidateRecord{}, ErrCandidateFormat
	}
	if len(b) != 0 {
		// Trailing bytes are not tolerated: they are outside the signature and outside
		// the AAD, so accepting them would be accepting unauthenticated data.
		return CandidateRecord{}, ErrCandidateFormat
	}
	return c, nil
}
