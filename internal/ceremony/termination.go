package ceremony

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"nib/internal/sign"
)

// A Termination is the convener's signed statement that a proceeding has ended — P08.S04b, D28's
// *declined* and *completed* states.
//
// # What it buys, and what it cannot
//
// **Earliness, not enforcement.** It cannot bind the convener: a convener who wants to continue
// simply does not mint one, and under D22's hub it is also the sole courier, so no claim reaches a
// party except through it. **Every consumer must read absence as UNKNOWN, never as live** — which
// is exactly why P08.S04a's expiry rule does not depend on this object existing.
//
// What it does buy is worth the file: an honest convener ends a proceeding at every party
// *promptly*, instead of leaving them to derive *abandoned* from `Expires` plus S06's grace up to
// thirty days later — and it leaves non-repudiable evidence of who ended it.
//
// # Only two of D28's four states can be attested
//
// *Declined* and *completed* have a convener to sign them. *Expired* and *abandoned* do not —
// abandoned means the convener never came back — so those two are derived locally and identically
// by every machine from the record's own `Expires` plus grace. A reader who assumes all four are
// attested will build the wrong verifier, which is why the closed set below is exactly two.
type Termination struct {
	// Version is this object's OWN format number, deliberately not FormatVersion (D32): the
	// record's format and this one move for different reasons, and sharing a number makes a bump
	// in either look like a skew in both.
	Version int `json:"version"`
	// Ceremony is the proceeding's id. A LOOKUP field, not the binding — `RosterHash` below is
	// what actually refuses a substitution, because it commits to the id as well.
	Ceremony string `json:"ceremony"`
	// RosterHash is the record's commitment, hex. **This one field is the whole binding.** It is
	// what `ConvenerSig` signs and it commits to the version, the convener, the id, the DocHash,
	// the intent, the deadline and every roster entry — so a cross-ceremony replay and the
	// same-id substitution /pending 318 closes both fall to a single comparison.
	RosterHash string `json:"rosterHash"`
	// State is the end state, and the set is closed at two — see the type doc.
	State string `json:"state"`
	// ConvenerCert is the signer's certificate, PEM. Carried so a reader can verify without
	// already holding it, exactly as `Record.ConvenerCert` is.
	ConvenerCert string `json:"convenerCert"`
	// Sig is the signature over the preimage, hex.
	Sig string `json:"sig"`
}

// The two states, and the set is closed. A third would need a convener able to observe it, which
// is the whole reason *expired* and *abandoned* are derived rather than attested.
const (
	StateDeclined  = "declined"
	StateCompleted = "completed"
)

// terminationVersion is this object's own format number.
const terminationVersion = 1

// terminationDomain separates this preimage from every other signature in the product.
//
// **It must be INSIDE the preimage, not around it.** `sign.SignDigest` signs a bare digest and does
// no hashing of its own, so a tag applied outside the digest is a tag the signature does not cover.
const terminationDomain = "nib-ceremony-termination-v1"

var (
	// ErrNoTermination reports that no termination object is stored for this ceremony.
	//
	// **A distinct sentinel because absence is the ordinary case and must never read as damage.**
	// Most ceremonies never terminate explicitly, and a convener that declines to mint one is
	// indistinguishable from one that has not decided — so this is "nothing here", never "something
	// is wrong here".
	ErrNoTermination = errors.New("no termination is stored for this ceremony")
	// ErrBadTermination reports a termination that is present and does not verify.
	//
	// Its own sentinel, and deliberately NOT `ErrMirrorDamaged`: that word is about this machine's
	// own disk, and a termination that fails to verify is far more likely to be a planted or
	// substituted file than a corrupted one. Conflating them would tell a user to suspect their
	// hardware.
	ErrBadTermination = errors.New("this ceremony's stored termination does not verify")
	// ErrTerminationConflict reports a second termination naming a different end state.
	ErrTerminationConflict = errors.New("this ceremony is already recorded as ended in a different state")
)

// preimage is the signed bytes.
//
// **Five chunks, and two more were deliberately left out.** The object the design first proposed
// also carried the declining `party` and a `When`, and both are attacks:
//
//   - `party` — a convener-signed *"X declined"* is an accusation the CONVENER authored about a
//     third party, non-repudiable, mintable at any moment including before that party's hop has
//     run. It frames a named innocent who cannot contradict it. The convener can honestly attest
//     only *that* the proceeding ended and in which state; who ended it is answered by
//     `record.Convener()`, which is signed.
//   - `When` — convener-chosen and unverifiable, and letting it drive S06's grace would hand a
//     convener control over when other machines prune. Retention starts from the local receipt's
//     observed-at time (C11), which is the honest clock.
//
// **Their removal is why this object needs no `Canonical`/`IsCanonical` pair.** Those two fields
// were the only malleable axes — a timestamp has sub-second and timezone renderings, a label has
// case and whitespace. What is left is fixed-width, a lowercase-hex fingerprint derived from the
// certificate, or one of two closed literals, so there is no second encoding of the same value for
// a canonical form to fold. `TestTheTerminationPreimageHasNoMalleableAxis` is what keeps that true.
func (t Termination) preimage() ([]byte, error) {
	var p preimageBuilder
	p.addString(terminationDomain)
	p.addUint(uint64(t.Version))
	p.addString(convenerFingerprint(t.ConvenerCert))
	raw, err := hex.DecodeString(t.RosterHash)
	if err != nil {
		return nil, fmt.Errorf("this termination's roster hash is not hex: %w", err)
	}
	// RAW bytes, not the hex string — `rosterPreimage` makes the same choice for the same reason:
	// a hex rendering has an upper-case twin and the raw bytes do not.
	p.add(raw)
	p.addString(t.State)
	return p.bytes(), nil
}

// SignTermination mints the object for a ceremony that has ended.
func SignTermination(rec Record, state string, certPEM, keyPEM []byte) (Termination, error) {
	if state != StateDeclined && state != StateCompleted {
		return Termination{}, fmt.Errorf("%q is not an end state a convener can attest — only %q "+
			"and %q have a convener to sign them; expired and abandoned are derived locally",
			state, StateDeclined, StateCompleted)
	}
	h, err := rec.RosterHash()
	if err != nil {
		return Termination{}, err
	}
	t := Termination{
		Version:      terminationVersion,
		Ceremony:     rec.ID,
		RosterHash:   hex.EncodeToString(h),
		State:        state,
		ConvenerCert: string(certPEM),
	}
	pre, err := t.preimage()
	if err != nil {
		return Termination{}, err
	}
	sum := sha256.Sum256(pre)
	sig, err := sign.SignDigest(sum[:], keyPEM)
	if err != nil {
		return Termination{}, err
	}
	t.Sig = hex.EncodeToString(sig)
	return t, nil
}

// An Anchor is what a Termination is checked AGAINST: the commitment it must match, and the
// identity that must have signed it.
//
// **It exists because a party who has not signed yet holds no record** (D16, `/pending 378`). The
// end state of a proceeding has to reach them, and until this the only way to check one was
// `Verify(rec)` — so the parties who most need to know a ceremony has ended were the ones who could
// not be told. An invitation carries both of these values directly (`RosterHash` and
// `ConvenerFingerprint`), and this type is what lets it supply them without a second copy of the
// checks.
//
// **Two values and not the whole record, because those are the only two `Verify` ever used.** Read
// at the line before this was written: `rec.RosterHash()` and `rec.Convener().Fingerprint`, and
// nothing else. A door taking a `Record` where two fields will do is a door an invitation cannot
// use, which is the whole of the problem.
//
// **It does not weaken what an anchor asserts.** `Record.Convener()` resolves the signer's
// certificate against the ROSTER and fails when it is not a member; `Invitation.Anchor` performs
// the same membership check, so neither source can produce an anchor naming a convener the roster
// does not contain.
type Anchor struct {
	// RosterHash is the record's commitment, raw bytes.
	RosterHash []byte
	// Convener is the hex fingerprint of the key entitled to end this proceeding.
	Convener string
}

// Anchor derives the checking values from a record.
func (r Record) Anchor() (Anchor, error) {
	h, err := r.RosterHash()
	if err != nil {
		return Anchor{}, err
	}
	conv, ok := r.Convener()
	if !ok {
		return Anchor{}, fmt.Errorf("%w: this record names no convener to compare against",
			ErrBadTermination)
	}
	return Anchor{RosterHash: h, Convener: conv.Fingerprint}, nil
}

// Anchor derives the same checking values from an invitation, for a party who holds no record.
//
// **The membership check is not optional and is why this is not two field reads.**
// `ConvenerFingerprint` is a bare field that `ParseInvitation` does not normalise or check against
// the roster — `rosterEntry`'s own doc says so, and `handleCeremonyAccept` refuses an invitation
// naming a non-member convener at its door for exactly this reason. Without the check here an
// anchor could name a convener this ceremony does not have, and the resulting `Verify` would refuse
// every honest termination while accepting one signed by whoever the field named.
func (i Invitation) Anchor() (Anchor, error) {
	if i.RosterHash == "" {
		return Anchor{}, fmt.Errorf("%w: this invitation carries no commitment, so nothing can be "+
			"bound to it", ErrBadTermination)
	}
	h, err := hex.DecodeString(i.RosterHash)
	if err != nil {
		return Anchor{}, fmt.Errorf("%w: this invitation's commitment is not hex", ErrBadTermination)
	}
	want := strings.ToLower(i.ConvenerFingerprint)
	for _, p := range i.Roster {
		if strings.EqualFold(p.Fingerprint, want) {
			return Anchor{RosterHash: h, Convener: strings.ToLower(p.Fingerprint)}, nil
		}
	}
	return Anchor{}, fmt.Errorf("%w: this invitation names a convener who is not one of its "+
		"parties, so it describes a ceremony with nobody entitled to end it", ErrBadTermination)
}

// Verify checks the object against the record it claims to end.
//
// **`rec` must come from the document or the invitation, never from the `record.json` sitting
// beside the termination.** A planted pair — a matching record and termination for another
// proceeding, dropped into this ceremony's directory — verifies perfectly against itself; the
// signature is valid and the roster hashes agree. Only an anchor the attacker does not control
// refuses it, and that is the caller's responsibility because this function cannot tell where its
// argument came from.
//
// **It is `VerifyAgainst` with the record's anchor and nothing else** (ADR-009). A second
// implementation for the invitation would be two versions of a SIGNATURE check, where the two
// disagreeing is a security bug rather than an inconsistency.
func (t Termination) Verify(rec Record) error {
	a, err := rec.Anchor()
	if err != nil {
		return err
	}
	return t.VerifyAgainst(a)
}

// VerifyAgainst is the one door: every check, against values either a record or an invitation can
// supply.
func (t Termination) VerifyAgainst(anchor Anchor) error {
	if t.Version != terminationVersion {
		return fmt.Errorf("%w: it is version %d and this build writes %d", ErrBadTermination,
			t.Version, terminationVersion)
	}
	if t.State != StateDeclined && t.State != StateCompleted {
		return fmt.Errorf("%w: %q is not an end state this build knows", ErrBadTermination, t.State)
	}
	want := anchor.RosterHash
	got, err := hex.DecodeString(t.RosterHash)
	if err != nil {
		return fmt.Errorf("%w: its roster hash is not hex", ErrBadTermination)
	}
	// The whole binding, in one comparison — see the field's own doc.
	if !bytes.Equal(want, got) {
		return fmt.Errorf("%w: it ends a proceeding with a different roster commitment, so it is "+
			"not this ceremony's", ErrBadTermination)
	}
	pre, err := t.preimage()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBadTermination, err)
	}
	sigb, err := hex.DecodeString(t.Sig)
	if err != nil {
		return fmt.Errorf("%w: its signature is not hex", ErrBadTermination)
	}
	sum := sha256.Sum256(pre)
	if err := sign.VerifyDigest(sum[:], sigb, []byte(t.ConvenerCert)); err != nil {
		return fmt.Errorf("%w: its signature does not check out", ErrBadTermination)
	}
	// **And the signer must be the CONVENER**, not merely someone with a valid signature. Without
	// this any roster member could end a proceeding they are only a party to.
	if !strings.EqualFold(convenerFingerprint(t.ConvenerCert), anchor.Convener) {
		return fmt.Errorf("%w: it was signed by a party who is not this ceremony's convener",
			ErrBadTermination)
	}
	return nil
}

// Encode renders the object for storage.
func (t Termination) Encode() ([]byte, error) { return json.MarshalIndent(t, "", "  ") }

// DecodeTermination reads one back.
func DecodeTermination(b []byte) (Termination, error) {
	var t Termination
	if err := json.Unmarshal(b, &t); err != nil {
		return Termination{}, fmt.Errorf("%w: %v", ErrBadTermination, err)
	}
	return t, nil
}

// Ended reports the state for a surface to render, or "" when there is none.
func (t Termination) Ended() string { return t.State }
