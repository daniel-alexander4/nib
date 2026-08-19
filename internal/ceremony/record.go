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
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
const FormatVersion = 1

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
	ErrVersion              = errors.New("unsupported ceremony record version")
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
	h := sha256.New()
	writeLP := func(b []byte) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(b)))
		h.Write(n[:])
		h.Write(b)
	}
	var ver [8]byte
	binary.BigEndian.PutUint64(ver[:], uint64(r.Version))
	writeLP(ver[:])
	writeLP([]byte(r.ID))
	writeLP([]byte(r.DocHash))
	writeLP([]byte(r.Intent))
	writeLP([]byte(r.Expires.UTC().Format(time.RFC3339)))
	for _, p := range r.Roster {
		fp, err := hex.DecodeString(p.Fingerprint)
		if err != nil || len(fp) != sha256.Size {
			return nil, fmt.Errorf("roster entry %q has an invalid fingerprint", p.Label)
		}
		writeLP(fp)
		var s byte
		if p.Signs {
			s = 1
		}
		writeLP([]byte{s})
		writeLP([]byte(p.Label))
	}
	return h.Sum(nil), nil
}

// RosterToken is the [NibRoster:<hash>] token each signature's /Reason carries, so every
// signature on a finished document attests to the same proceeding (D2's UX pin).
func (r Record) RosterToken() (string, error) {
	h, err := r.RosterHash()
	if err != nil {
		return "", err
	}
	return "[NibRoster:" + hex.EncodeToString(h) + "]", nil
}

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
func (r Record) Verify() error {
	if r.Version != FormatVersion {
		return fmt.Errorf("%w: %d (this build writes %d)", ErrVersion, r.Version, FormatVersion)
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
	for _, p := range r.Roster {
		if p.Fingerprint == want {
			return nil
		}
	}
	return ErrConvenerNotInRoster
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
func (r Record) Encode() ([]byte, error) { return json.Marshal(r) }

func Decode(b []byte) (Record, error) {
	var r Record
	if err := json.Unmarshal(b, &r); err != nil {
		return Record{}, fmt.Errorf("ceremony record is not readable: %w", err)
	}
	return r, nil
}
