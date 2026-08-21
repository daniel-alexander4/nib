// Package ceremony holds the Ceremony Record: the one signed artifact a convener writes
// before a packet moves (D20).
//
// The record is the ceremony's identity, roster, order, intent and deadline, and every
// other decision in the design reads from it rather than asking a person. It is written
// once, before anyone connects, and every signature on the finished document carries a
// token committing to it — so a verifier can say that all of the signatures attest to one
// proceeding rather than to a chain of pairwise claims.
package ceremony

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"nib/internal/sign"
)

// FormatVersion is the record's format version, and it is the FIRST axis of the
// commitment preimage (D32).
//
// Being inside the preimage is what makes it load-bearing rather than documentation: two
// versions of the format cannot produce the same rosterHash even if every other field
// matches, so a party running a different version is refused rather than silently
// agreeing to a record it parsed differently.
//
// **Bumped to 2 (2026-08-20, P04.S03).** The preimage gained a leading domain tag, so a
// record signed by a v1 build hashes differently and its signature no longer verifies.
// Without the bump the failure would surface as `ErrBadConvenerSignature` — "the record
// was altered after it was written" — which is a *wrong* sentence, not merely a terse
// one: it accuses a counterparty of tampering when the truth is that the two builds
// disagree about the format. D32's rule is that a version mismatch produces a sentence,
// and this makes it the right one.
//
// **Bumped to 3 (2026-08-20, v1.116.17).** RosterHash gained the convener's fingerprint.
// Its exclusion list named the six-word name and the secret, so ConvenerCert's absence read
// as a decision; it was not one, and any roster member could re-sign an unchanged roster
// with their own key. Verify passed — it asks only that the signer appear somewhere in the
// roster — the hash was byte-identical so RosterToken was too, and Convener() then named
// the substitute. The bump follows for the same reason the first one did: the preimage
// changed, so a v2 record read by this build must produce a version sentence rather than a
// tampering accusation.
//
// Two bumps in one day is also why this comment now records BOTH. It said "Bumped to 2"
// above a constant reading 3 — a doc describing a version the code had already left, which
// is this repo's most-found defect shape wearing its most harmless-looking clothes.
const FormatVersion = 3

// MaxCeremonyLife caps how far ahead a record's deadline may be set.
//
// **Thirty days, and it is enforced rather than documented** — the plan's own words about
// the four externally-supplied security parameters, of which this is one. D16 calls clock 3
// "days, convener's choice", which is a parameter with no bound: it governs how long a
// listener may arm, how long invitation-scoped pins persist (D29) and how long a mirror
// lives, so a convener setting ten years was a config away.
//
// Checked against `now` rather than against a creation time, because a record carries no
// creation time — the same shape MaxCandidateLife uses, and it behaves correctly in both
// directions: a record written last year has an expiry in the past and passes, while one
// claiming ten years ahead fails whenever it is read.
const MaxCeremonyLife = 30 * 24 * time.Hour

// rosterDomain separates this preimage from every other thing the identity key signs.
// On-wire identifier: immutable once shipped (STANDARDS §9).
const rosterDomain = "nib-ceremony-record-v1"

// Party is one entry in the roster. The order of the roster IS the signing order.
type Party struct {
	// Name is the six-word display name (D3). Deliberately OUTSIDE the commitment
	// preimage: it is a pure function of the fingerprint, so including it would let a
	// wordlist change alter a commitment the list's freeze exists to keep stable.
	Name string `json:"name"`
	// Fingerprint is the SHA-256 SPKI, hex. This is what identifies the party.
	Fingerprint string `json:"fingerprint"`
	// Label is what the convener calls them, for display.
	Label string `json:"label,omitempty"`
	// Signs is false for a non-signing convener (D22). It is INSIDE the preimage: without
	// it a convener could present one roster to the signers and another to a verifier,
	// differing only in who was obliged to sign, and both would hash the same.
	Signs bool `json:"signs"`
}

// Record is the ceremony's founding artifact.
type Record struct {
	Version int `json:"version"`
	// ID is 128 random bits, hex. It names the hop that resumes; without it "continue" is
	// a guess about which ceremony.
	ID string `json:"id"`
	// DocHash is the SHA-256 of the prepared document **with this record removed** — see
	// DocumentHash. Every party agrees to the same bytes and a resumed hop can prove it.
	DocHash string `json:"docHash"`
	// Roster is ordered; the order is the signing order.
	Roster []Party `json:"roster"`
	// Intent is what everyone is agreeing to, and it is the ONLY home for it (D20's
	// plan-review pin): each signature's own intent is populated from this rather than
	// typed, so a finished document cannot say one thing in the record and another on a
	// signature block.
	Intent string `json:"intent"`
	// Expires is the ceremony deadline (D16's clock 3).
	Expires time.Time `json:"expires"`
	// ConvenerSig is the convener's signature over the commitment preimage, hex.
	ConvenerSig string `json:"convenerSig"`
	// ConvenerCert is the convener's certificate PEM, so a party holding only the document
	// can verify the signature and check the signer against the roster.
	ConvenerCert string `json:"convenerCert"`
}

var (
	// ErrBadConvenerSignature is the refusal a party gives before any pairing when the
	// record it was handed does not verify. Distinct so it never reads as a parse error:
	// a malformed record is a broken file, and a record whose signature fails is a record
	// somebody changed.
	ErrBadConvenerSignature = errors.New("the ceremony record's convener signature does not verify")
	ErrConvenerNotInRoster  = errors.New("the ceremony record was signed by someone not in its roster")
	// ErrCeremonyTooLong is its own sentinel rather than a shape of ErrVersion or a bare
	// error, because it is the one refusal here that names a MISCONFIGURATION rather than an
	// attack — the convener set a deadline this build will not honour, and the fix is theirs.
	ErrCeremonyTooLong = errors.New("the ceremony record's deadline is too far ahead")
	// ErrNotAHop reports two parties that do not share a hop — not in the roster, the same
	// party twice, or not adjacent in it.
	ErrNotAHop = errors.New("those two parties do not share a hop")
	ErrVersion = errors.New("unsupported ceremony record version")
)

// RosterHash is the commitment every signature carries.
//
// The preimage's axes, in order, each length-prefixed (D20's Stage 6 pin, crypto pack
// PLAN-2): the format version, id, docHash, intent, expires, and then for each roster
// entry in order — fingerprint (32 RAW bytes), signs (one byte), label.
//
// Length-prefixing every axis is what stops two different records hashing the same by
// moving a byte across a boundary: without it a label ending in a hex digit and a
// fingerprint beginning with one are indistinguishable from the other split.
//
// **Deliberately excluded, and the exclusion is part of the specification.** The six-word
// name, because it is a pure function of the fingerprint (D3) and including it would let a
// wordlist change alter this commitment. And the invitation secret, because a verifier who
// was not a participant must be able to check this from the document alone.
func (r Record) RosterHash() ([]byte, error) {
	return rosterDigest(r)
}

// rosterPreimage is the exact byte string RosterHash digests.
//
// Split out at P04.S03 so the preimage can be ASSERTED rather than merely described. The
// domain tag added there was covered by nothing — deleting it left the whole repo green —
// because a function that returns only a digest gives a test nothing to look at. See
// preimage.go.
// convenerFingerprint is the hex SPKI fingerprint of the record's convener certificate,
// or "" when there is none (an unsigned draft).
func convenerFingerprint(certPEM string) string {
	if certPEM == "" {
		return ""
	}
	fp, err := sign.Fingerprint([]byte(certPEM))
	if err != nil {
		return ""
	}
	return hex.EncodeToString(fp)
}

func rosterPreimage(r Record) ([]byte, error) {
	var p preimageBuilder
	// The domain tag, FIRST (P04.S03).
	//
	// This identity key signs more than one kind of message now — the Ceremony Record
	// here, and the candidate record — and `sign.SignDigest` signs a bare 32-byte digest
	// (identity.go): it does not re-hash and it does not know what the digest means. Two
	// preimage languages built from the same length-prefixed chunks, with no tag to tell
	// them apart, are two message types one signature can satisfy.
	//
	// The candidate record's preimage carries attacker-influenceable bytes (a candidate is
	// an IP:port learned from DHT nodes, and P04.S02 established that a node controls the
	// `ip` field it returns), so the direction that matters is a candidate record coaxed
	// into also parsing as a roster preimage — which would forge a ConvenerSig for a
	// ceremony the signer never convened. No such collision is demonstrated; the tag makes
	// the question unrepresentable rather than merely unlikely, and it costs one chunk.
	p.addString(rosterDomain)
	p.addUint(uint64(r.Version))
	// WHO CONVENED, bound as a distinct axis (v3).
	//
	// The roster hash already commits to the SET of fingerprints, and the convener is one
	// of them — so what was missing was never their identity, it was which of them holds
	// the role. Any roster member could re-sign an unchanged roster with their own key:
	// `Verify` asks only that the signer appear SOMEWHERE in the roster, the hash was
	// byte-identical, `RosterToken` was byte-identical, and `Convener()` then named the
	// new signer. A verifier reading a finished document could not tell them apart.
	//
	// The FINGERPRINT, not the certificate. A cert can be re-issued for the same key and
	// the same identity, which would change the token for a ceremony that has not changed;
	// the fingerprint is the stable identity and is already this roster's own currency.
	// Empty when unsigned, which is a distinct value rather than an absent axis — an
	// omission and a decision look identical in code, which is the whole reason this is
	// here rather than being left out with no comment.
	p.addString(convenerFingerprint(r.ConvenerCert))
	p.addString(r.ID)
	p.addString(r.DocHash)
	p.addString(r.Intent)
	p.addString(r.Expires.UTC().Format(time.RFC3339))
	for _, party := range r.Roster {
		fp, err := hex.DecodeString(party.Fingerprint)
		if err != nil || len(fp) != sha256.Size {
			return nil, fmt.Errorf("roster entry %q has an invalid fingerprint", party.Label)
		}
		p.add(fp)
		var sg byte
		if party.Signs {
			sg = 1
		}
		p.add([]byte{sg})
		p.addString(party.Label)
	}
	return p.bytes(), nil
}

func rosterDigest(r Record) ([]byte, error) {
	pre, err := rosterPreimage(r)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(pre)
	return sum[:], nil
}

// **RosterToken was deleted here (2026-08-20).** It formatted the [NibRoster:<hash>] token
// and said "each signature's /Reason carries" it — which is true, and was true of code
// somewhere else: p2p's Attestation.reason builds the token itself, and that is the only
// producer on the real path. Two implementations of one wire format, of which the tested one
// was the dead one; p2p's could have changed shape and the ceremony test would still have
// passed. p2p cannot import ceremony (this package's own tests import p2p, so the edge would
// be a cycle), so the duplicate goes and the producer gets the coverage — see
// TestTheRosterTokenIsWellFormedWhereItIsActuallyBuilt.

// NewID returns 128 random bits as hex.
func NewID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Sign fills in the convener's signature over the commitment preimage.
//
// The signature is over the ROSTER HASH, not over the JSON: the JSON's field order,
// whitespace and time formatting are all things a re-encode can change, and a signature
// over bytes that a round-trip can alter is one that fails for reasons having nothing to
// do with tampering.
func (r *Record) Sign(certPEM, keyPEM []byte) error {
	r.Version = FormatVersion
	r.ConvenerCert = string(certPEM)
	h, err := r.RosterHash()
	if err != nil {
		return err
	}
	sig, err := sign.SignDigest(h, keyPEM)
	if err != nil {
		return fmt.Errorf("sign ceremony record: %w", err)
	}
	r.ConvenerSig = hex.EncodeToString(sig)
	return nil
}

// Verify checks the convener's signature and that the signer is in the roster.
//
// Both halves matter and they fail differently. A signature that does not verify means the
// record was altered after it was written. A signature that verifies but belongs to nobody
// in the roster means the record is internally consistent and describes a proceeding its
// own signer is not part of — which is a well-formed lie rather than a corruption.
// Verify checks the convener's signature over the roster and the record's own shape.
//
// **It takes `now` rather than reading the clock**, for C11's reason: a wall-clock read
// inside a validation verdict is nondeterminism reaching a decision, and it makes the
// ceiling below untestable without faking time. The sibling `CandidateRecord.Verify` already
// takes one.
//
// **It does NOT refuse an expired ceremony, and that is deliberate.** An expired record is a
// liveness fact about the ceremony, not a validity fact about the document — a verifier
// reading a finished PDF next year must still be able to check who was on the roster and
// that the convener signed it. Refusing here would make a completed ceremony unverifiable
// the day after it ended, which is the opposite of what a signed record is for. Whether a
// hop may still START is the caller's question and is bounded by the deadline plus one full
// exchange budget (D16's Stage 6 pin), which lives where exchangeDeadline does.
func (r Record) Verify(now time.Time) error {
	if r.Version != FormatVersion {
		return fmt.Errorf("%w: %d (this build writes %d)", ErrVersion, r.Version, FormatVersion)
	}
	// The roster bound belongs HERE too, not only in ParseInvitation.
	//
	// A Record arrives from external input — Extract(pdf) → Decode → Verify — and the
	// CONVENER, who is the party that dials every hop and emits every packet, never parses
	// its own invitation. So a cap enforced only on the pasted-invitation path binds the
	// recipients and leaves the emitter unbounded, which is the half that matters now that
	// P04.S04 made the punch budget per hop. D25 also allocates signature pages from this
	// length, so an unbounded roster is an unbounded page count.
	if len(r.Roster) == 0 || len(r.Roster) > MaxRoster {
		return fmt.Errorf("%w: it names %d parties (the limit is %d)",
			ErrRosterMismatch, len(r.Roster), MaxRoster)
	}
	h, err := r.RosterHash()
	if err != nil {
		return err
	}
	sig, err := hex.DecodeString(r.ConvenerSig)
	if err != nil {
		return ErrBadConvenerSignature
	}
	if err := sign.VerifyDigest(h, sig, []byte(r.ConvenerCert)); err != nil {
		return ErrBadConvenerSignature
	}
	fp, err := sign.Fingerprint([]byte(r.ConvenerCert))
	if err != nil {
		return ErrBadConvenerSignature
	}
	want := hex.EncodeToString(fp)
	inRoster := false
	for _, p := range r.Roster {
		if p.Fingerprint == want {
			inRoster = true
			break
		}
	}
	if !inRoster {
		return ErrConvenerNotInRoster
	}
	// The ceiling, LAST — after the signature, so a record that fails both is reported as
	// forged rather than as over-long. A convener who signed a ten-year deadline is a
	// misconfiguration; one who did not sign at all is an attacker, and the second sentence
	// is the one a user needs.
	if r.Expires.After(now.Add(MaxCeremonyLife)) {
		return fmt.Errorf("%w: it claims to run until %s, more than %s from now",
			ErrCeremonyTooLong, r.Expires.UTC().Format(time.RFC3339), MaxCeremonyLife)
	}
	return nil
}

// Convener returns the roster entry that signed this record.
func (r Record) Convener() (Party, bool) {
	fp, err := sign.Fingerprint([]byte(r.ConvenerCert))
	if err != nil {
		return Party{}, false
	}
	want := hex.EncodeToString(fp)
	for _, p := range r.Roster {
		if p.Fingerprint == want {
			return p, true
		}
	}
	return Party{}, false
}

// MarshalJSON / UnmarshalJSON are the plain encoding; the signature covers the roster hash
// rather than these bytes, so a re-encode is safe.
// Hop returns the hop index joining two parties, and it is the ONLY definition of a hop
// number in this project.
//
// **The roster's order is the definition.** `Party`'s own doc says "the order of the roster
// IS the signing order", and D22 makes connectivity a sequence of pairs — so hop *i* joins
// `Roster[i]` to `Roster[i+1]`, and a ceremony with N parties has N-1 hops. Both ends of a
// hop compute the same number from the same convener-signed artifact, which is what makes it
// agreed rather than negotiated: there is no counter to get out of step across a
// quit-and-reopen (D24), because nothing is counted.
//
// **Before this, a hop was an unbounded `int` that nothing derived and nothing checked.**
// Eight functions took one — every key, salt and seed derivation among them — with no range
// check at any of them, and the only assignment in the whole tree was a literal `0` in a
// self-test. A caller's off-by-one derived a perfectly valid key and salt at a hop that does
// not exist, published into the void, and the outcome was reported as `FetchEmpty`:
// indistinguishable from a counterparty who has not arrived yet.
func (r Record) Hop(a, b string) (int, error) { return hopBetween(r.Roster, a, b) }

// hopBetween is the one door. Record and Invitation both carry a roster and both are asked
// for hop numbers, and two implementations of one rule is the shape ADR-009 exists to refuse
// — here it would be worse than usual, because the two would disagree only for the rosters
// where it mattered.
func hopBetween(roster []Party, a, b string) (int, error) {
	ia, ib := -1, -1
	for i, p := range roster {
		// Case-insensitive, because a fingerprint that differs only in case is the same
		// party — the same fact ParseInvitation normalises for the invitation's roster and
		// that Convener() compares case-sensitively one function away.
		if strings.EqualFold(p.Fingerprint, a) {
			ia = i
		}
		if strings.EqualFold(p.Fingerprint, b) {
			ib = i
		}
	}
	if ia < 0 || ib < 0 {
		return 0, fmt.Errorf("%w: one of %s and %s is not in this ceremony's roster",
			ErrNotAHop, short(a), short(b))
	}
	if ia == ib {
		return 0, fmt.Errorf("%w: %s was given as both ends", ErrNotAHop, short(a))
	}
	lo, hi := ia, ib
	if lo > hi {
		lo, hi = hi, lo
	}
	if hi-lo != 1 {
		// Non-adjacent parties share no hop. This is criterion 19's rule expressed where it
		// can be enforced rather than remembered: a convener holding candidates for a party
		// three positions away cannot even derive that hop's key by asking for it.
		return 0, fmt.Errorf("%w: %s and %s are %d apart in the roster, and a hop joins "+
			"adjacent parties", ErrNotAHop, short(a), short(b), hi-lo)
	}
	return lo, nil
}

// Hops is how many hops this ceremony has: one fewer than its roster.
func (r Record) Hops() int {
	if len(r.Roster) < 2 {
		return 0
	}
	return len(r.Roster) - 1
}

func (r Record) Encode() ([]byte, error) { return json.Marshal(r) }

func Decode(b []byte) (Record, error) {
	var r Record
	if err := json.Unmarshal(b, &r); err != nil {
		return Record{}, fmt.Errorf("ceremony record is not readable: %w", err)
	}
	return r, nil
}
