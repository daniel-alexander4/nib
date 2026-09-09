package ceremony

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/chacha20poly1305"

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

// The THREE states a convener can attest, and the set is still closed — it is closed at three now
// rather than at two (P01.S02c, `/pending 428`).
//
// # Why the old argument admits this one rather than being overturned by it
//
// This block used to read *"the set is closed. A third would need a convener able to observe it,
// which is the whole reason expired and abandoned are derived rather than attested."* That is a
// CONDITIONAL and not a prohibition, and its test is *can the convener observe it*. A convener
// stopping their own proceeding is the one fact in this system they observe with more authority
// than anybody — so `StateStopped` passes the gate the old wording set rather than violating it.
// Both derived states still fail it, for their own reasons and unchanged: nobody can attest a
// clock, and *abandoned* means the convener never came back, so the party who would sign it is
// precisely the one who stopped answering.
//
// # Why it is `stopped` and not `abandoned`, and not `cancelled`
//
// **`abandoned` is taken, and it means the OPPOSITE.** `closeout.go` defines it as *"a proceeding
// that ended without reaching this machine at all — the deadline and the grace both passed and
// nothing ever said what happened"*, and `renderEndedCeremonies` prints it as **"No further word"**.
// Attesting under that word would put the product's considered phrase for silence in front of every
// party for the one act the convener performed deliberately and told everybody about. Worse, the
// two vocabularies meet in `Receipt.State`, whose conflict rule is `prev.State == r.State` — a
// STRING comparison — so an attested and a derived *abandoned* would compare equal and merge with
// no trace, defeating the guard whose own doc says it exists to stop *"destroying the better answer
// with the worse one"*.
//
// **`cancelled` is worse, and the count is the argument.** `web/index.html` carries 28 `>Cancel<`
// buttons and two of them are on the ceremony panel itself — `cerAcceptCancel` and
// `cerConveneCancel`, inches from where this control renders — every one meaning *close this dialog
// and do nothing*. A "Cancel ceremony" meaning *irrevocably end this for everyone* beside them is a
// collision no wording fixes.
//
// `stopped` collides with nothing in the ceremony domain (searched: `grep -rn "'stopped'\|\"stopped\""`
// over `internal/` and `web/` returns nothing), it names an ACT rather than a duration — which
// matters, because this object carries no `When` and so cannot honestly assert a duration — and it
// reads correctly unexplained in the two places the vocabulary surfaces: *"Stopped by the convener"*
// in the ended list, and *"the convener stopped this proceeding"* in what a party is told.
const (
	StateDeclined  = "declined"
	StateCompleted = "completed"
	// StateStopped is the convener ending their own proceeding before it finished (D12).
	StateStopped = "stopped"
)

// DeliversDocument reports whether a proceeding that ended in this state has a finished document
// to hand out.
//
// **One predicate replacing a `== StateDeclined` literal at four sites, three of which produced a
// FALSE STATEMENT for any third value.** `runDeliveryRound` chose its payload with
// `case t.State == StateDeclined` and an `else` that shipped the mirror document — so a stopped
// ceremony would have delivered the partially-signed file AS the finished one, which is precisely
// the failure that arm's own comment records ("swallowing it here shipped the partially-signed
// mirror document to every party instead of the attestation"). `roundIsFinished` fell through to
// `alreadyDelivered`, a stat on a finished document that will never exist — its doc says that
// mistake already held every declined ceremony open until the three-day grace. `tellEndState`
// defaulted to *"One of the parties refused"*. And the card badge read
// `c.ended === 'declined' ? 'Declined' : 'Completed'`, rendering a stopped ceremony as **Completed**.
//
// **Phrased as a question about the DOCUMENT rather than as a list of states**, because that is the
// fact all four sites actually need: what the round carries, whether a signer should wait for a
// file, what the user is told, and what the badge says are all downstream of "is there a finished
// document at the end of this". A list would have to be edited again at the next state; this does
// not.
func DeliversDocument(state string) bool { return state == StateCompleted }

// attestable reports whether a convener can sign this end state.
//
// **One door, because the rule had TWO implementations and neither was tested.** `SignTermination`
// and `VerifyAgainst` each carried their own copy of the closed set, and a probe found the second
// was reached by no test in the tree: deleting `VerifyAgainst`'s state arm entirely left
// `./internal/ceremony`, `./internal/server` and `./internal/cli` all green. Adding a third value to
// two hand-written lists is exactly the ADR-009 shape, so both now ask here.
func attestable(state string) bool {
	return state == StateDeclined || state == StateCompleted || state == StateStopped
}

// terminationVersion is this object's own format number.
//
// **Bumped 1 -> 2 on 2026-09-09 for `StateStopped` (`/pending 428`), and the reason is D32 rather
// than tidiness.** The argument against bumping was that the state field is itself a version
// signal — an unknown state can only have come from a newer build — so a sentinel added here could
// carry the news without breaking version 1. That is wrong in the one direction that matters: the
// RELEASED build's behaviour is already fixed, and the only field it inspects before the state is
// this one (`VerifyAgainst` checks the version first, deliberately). So no sentinel added in a new
// build can ever reach an old peer, and without a bump an upgrade makes an older Nib call an honest
// termination a file that *"does not verify"* — `ErrBadTermination`, whose own doc calls such an
// object *"far more likely a planted or substituted file than a corrupted one"*. That is the
// tampering accusation D32 exists to forbid.
//
// **What the bump costs, measured rather than estimated:** exactly one test, whose mutation is the
// hard-coded literal `x.Version = 2` in `TestTheTerminationPreimageHasNoMalleableAxis` — with the
// constant at 2 that perturbation becomes a no-op and the test correctly reports the axis as having
// a second encoding. It mutates to `terminationVersion + 1` now, so the next bump cannot repeat it.
// Version 1 objects on disk are refused, which is the zero-users doctrine this repo has already
// invoked three times for `FormatVersion` and once for `InvitationVersion`.
const terminationVersion = 2

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

	// ErrTerminationVersion / ErrTerminationOldVersion: a version this build does not know, split
	// by DIRECTION (P01.S02c, `/pending 428`).
	//
	// **Split because a direction-blind check can only ever say one of the two things, and this
	// package has already shipped that bug once.** `invitation.go`'s own doc records it: the check
	// read the version out of a prefix, so every mismatch produced *"made by a newer version of
	// Nib"* — and *"the first time InvitationVersion is bumped, a NEW build handed an ORDINARY v1
	// invitation would have told the user their invitation came from the future"*. That comment
	// names the bump as the change that would reach the bug. `terminationVersion` has just been
	// bumped, so this is that moment, and the split lands with it rather than after it.
	//
	// **Neither wears `ErrBadTermination`, and that is the whole point.** D32's rule is that a
	// version mismatch produces a SENTENCE and never a tampering accusation, and
	// `ErrBadTermination`'s own doc says an object failing it is *"far more likely a planted or
	// substituted file than a corrupted one"*. Telling a user that an honest newer Nib's
	// attestation is a probable forgery is the failure this repo already paid for once, where one
	// signature from a newer build made a whole document report as not one proceeding.
	ErrTerminationVersion    = errors.New("this end state was written by a newer version of Nib — update Nib to read it. It is not damaged")
	ErrTerminationOldVersion = errors.New("this end state was written by an older version of Nib")
	// ErrUnknownEndState: the object verifies and names a state this build does not know.
	//
	// **Reachable only on a version this build DOES know**, because the version is checked first —
	// so it means a build that shares this format and not this vocabulary, which is a narrower and
	// more useful thing to tell a reader than "does not verify".
	ErrUnknownEndState = errors.New("this end state names an outcome this version of Nib does not know")

	// ErrEndStateTooBig: a sealed end state exceeds what the rendezvous will carry.
	//
	// **A distinct sentinel because it is not a verification failure**, and reporting it as one
	// sends the reader to the wrong place entirely — it says the object is untrustworthy when the
	// object is fine and the TRANSPORT is the constraint. Caught by reading this test's own output
	// while building it: `a sealed end state is 1223 bytes ...` arrived wearing
	// `ErrBadTermination`'s "does not verify" prefix. `ErrCandidateTooBig` is the same distinction
	// one file over.
	ErrEndStateTooBig = errors.New("this end state is too large to publish")
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
	if !attestable(state) {
		return Termination{}, fmt.Errorf("%q is not an end state a convener can attest — only %q, "+
			"%q and %q have a convener to sign them; expired and abandoned are derived locally, "+
			"because nobody can attest a clock and the party who would sign 'abandoned' is the one "+
			"who stopped answering", state, StateDeclined, StateCompleted, StateStopped)
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
	// **Version FIRST and direction-aware.** A version this build does not know means the preimage
	// layout may differ, so every check below it — including the signature — would be asking the
	// wrong question of the wrong bytes. `Record.Verify` orders itself the same way and `ReadStored`
	// classifies skew before damage for the same reason: *"classifying in the other order would
	// report a newer Nib's ceremony as forged."*
	if t.Version > terminationVersion {
		return fmt.Errorf("%w: it is version %d and this build reads %d", ErrTerminationVersion,
			t.Version, terminationVersion)
	}
	if t.Version < terminationVersion {
		return fmt.Errorf("%w: it is version %d and this build reads %d", ErrTerminationOldVersion,
			t.Version, terminationVersion)
	}
	if !attestable(t.State) {
		return fmt.Errorf("%w: %q", ErrUnknownEndState, t.State)
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

// --- the published end state (/pending 380) -----------------------------------

// endStateAADDomain separates the SEALED record's AAD from every other length-prefixed structure
// this package builds — the same separation `candidateDomain` provides one file over.
//
// **Distinct from `terminationDomain`, which is the SIGNATURE preimage's tag, and the compiler
// caught the collision.** They are different objects at different layers: one is what the convener
// signs, the other is what binds a sealed copy to the target it was published at. Sharing a tag
// would make a value used for one purpose usable for the other, which is the failure `derive`'s own
// doc names.
const endStateAADDomain = "nib-end-state-record-v1"

// terminationAAD binds a sealed end state to the target it was published at.
//
// **No hop, unlike `candidateAAD`.** The end state is a fact about the proceeding rather than about
// one leg of it, and `EndStateSalt` is not hop-scoped — adding a hop here would be a second, weaker
// derivation of a thing that has none.
func terminationAAD(salt []byte) []byte {
	var p preimageBuilder
	p.addString(endStateAADDomain)
	p.add(salt)
	return p.bytes()
}

// Seal encrypts this end state for publication at the end-state rendezvous.
//
// **It verifies BEFORE it seals, which is `CandidateRecord.Seal`'s discipline and is here for the
// same reason**: an object that does not check out must never be handed to the network wearing this
// package's seal. The anchor is the caller's — a convener passes its record's, so a termination
// that does not bind to the ceremony it is being published for cannot leave the machine.
//
// **The size ceiling is checked HERE and not at the publisher, and that is the load-bearing part.**
// `MaxSealedRecord`'s own doc says why: an over-size value is refused by our own store inside
// `dht.Server.Put` before any datagram is sent, `getput.Put` logs a warning per node and returns
// nil, and **the record simply never leaves the machine**. Measured while building this: a
// termination carrying a PEM certificate bencodes to 827 bytes for the 8-character common name
// production actually mints (`GenerateIdentity("Nib User")`), and to **1172 bytes — over the cap —**
// for a 128-character one. The margin is real today and it is a function of a name; if that name
// ever becomes user-supplied, this check is what turns a silent non-publish into a refusal a caller
// can report. `TestASealedEndStateFitsTheRendezvous` is its guard.
func (t Termination) Seal(key, salt []byte, anchor Anchor) ([]byte, error) {
	if err := t.VerifyAgainst(anchor); err != nil {
		return nil, fmt.Errorf("this end state does not verify, so it will not be published: %w", err)
	}
	plain, err := json.Marshal(t)
	if err != nil {
		return nil, err
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := aead.Seal(nonce, nonce, plain, terminationAAD(salt))
	if len(out) > MaxSealedRecord {
		return nil, fmt.Errorf("%w: %d bytes against a cap of %d — it would be dropped by this "+
			"machine's own store before any datagram was sent, silently",
			ErrEndStateTooBig, len(out), MaxSealedRecord)
	}
	return out, nil
}

// OpenEndState decrypts and VERIFIES an end state fetched from the rendezvous.
//
// **Verification is inside this door and not beside it.** `ReadTermination`'s doc already states
// the rule this follows — *"`rec` must come from the document or the invitation, never from the
// `record.json` sitting beside it"* — and here the anchor comes from the invitation, which is the
// only thing a pre-hop party holds. A reader that could open without verifying is a reader that
// will eventually be called by somebody who forgets, which is the ADR-009 shape a single door
// exists to refuse.
//
// The bytes come off the public DHT and are written by whoever reached that target first, so every
// failure below is the ordinary case rather than an anomaly: a wrong key, a truncated value, a
// stranger's noise and a planted object all land here and all return an error.
func OpenEndState(key, salt, sealed []byte, anchor Anchor) (Termination, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return Termination{}, err
	}
	if len(sealed) < chacha20poly1305.NonceSizeX {
		return Termination{}, fmt.Errorf("%w: a sealed end state is %d bytes, shorter than its nonce",
			ErrBadTermination, len(sealed))
	}
	nonce, ct := sealed[:chacha20poly1305.NonceSizeX], sealed[chacha20poly1305.NonceSizeX:]
	plain, err := aead.Open(nil, nonce, ct, terminationAAD(salt))
	if err != nil {
		return Termination{}, fmt.Errorf("%w: this end state does not open at that target", ErrBadTermination)
	}
	var t Termination
	if err := json.Unmarshal(plain, &t); err != nil {
		return Termination{}, fmt.Errorf("%w: %v", ErrBadTermination, err)
	}
	if err := t.VerifyAgainst(anchor); err != nil {
		return Termination{}, err
	}
	return t, nil
}
