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

	"nib/internal/pdfops"
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
//
// **Bumped to 4 (2026-08-24, P07.S02).** `Party` gained `Capacity` inside the preimage (D20's
// capacity amendment), so a v3 record read by this build hashes differently and its signature
// no longer verifies — the same reason as both earlier bumps, and the same remedy: a version
// sentence rather than a tampering accusation.
//
// **Taken NOW because P07.S02 is the first code in the product that ever constructs a
// `Record`** — every literal in the tree before this slice was inside a `_test.go`. There are
// no records in the field, so the bump costs nothing today and costs a migration forever after
// P07 ships.
//
// **What guards it, because until this slice nothing did.** No test pinned this constant to a
// literal — every reference compared it to itself — and there was no golden vector anywhere in
// the package, so `rosterPreimage` could change shape with the whole repo green.
// `TestRosterHashGoldenVector` now pins both: it carries a hex literal and asserts this
// constant against a literal, so a preimage edit without a bump goes red, and a bump without a
// re-cut vector goes red.
const FormatVersion = 4

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
//
// # The six-word name is NOT a field here, and its absence IS the property (D21, 2026-08-22)
//
// It used to be one: a `Name string`, JSON-serialized, documented at length, deliberately
// outside the commitment preimage — and written by nobody and read by nobody. D21 requires
// *"an invitation whose name and fingerprint disagree"* to be **refused rather than resolved
// either way**, and MatchesRecord compared Fingerprint and Signs and never Name, so nothing
// could refuse it. That was latent only because no name was ever set; it would have opened
// the moment a display surface populated one, and the harm is a roster showing one party's
// six words beside another party's fingerprint — two people read the same words aloud, the
// verification "succeeds", and the pin is the attacker's.
//
// The name is a pure function of the fingerprint (`pairing.Name`), so storing it beside the
// fingerprint states one fact twice in a shape where the two can disagree — the class ADR-009
// exists for, and the class ConvenerFingerprint records this package being burned by. Deleting
// it makes the disagreement UNREPRESENTABLE rather than merely checked: a check can be
// forgotten at a new call site, and a type that cannot express the disagreement cannot be.
//
// Anything wanting the words derives them from the fingerprint this entry already carries.
// `internal/server/peers.go:nameOrEmpty` is that call, already doing exactly this for pinned
// peers. Nothing signed moved when the field went — it sat outside every preimage, so no
// commitment, token or signature changed, and a decoder reading an older JSON simply ignores
// the now-unknown key.
//
// The wordlist-freeze reasoning the old field's comment carried survives with the fact it was
// about: the commitment cannot depend on the wordlist, because there is no wordlist-derived
// value in a roster entry at all. `TestEveryPartyFieldIsInTheCommitment` is the guard.
type Party struct {
	// Fingerprint is the SHA-256 SPKI, hex. This is what identifies the party.
	Fingerprint string `json:"fingerprint"`
	// Label is what the convener calls them, for display.
	Label string `json:"label,omitempty"`
	// Signs is false for a non-signing convener (D22). It is INSIDE the preimage: without
	// it a convener could present one roster to the signers and another to a verifier,
	// differing only in who was obliged to sign, and both would hash the same.
	Signs bool `json:"signs"`
	// Capacity is the role this party signs in — "as Director of Acme Ltd", "as attorney
	// under a power of attorney dated 3 June" — and it is INSIDE the preimage (D20's
	// capacity amendment, 2026-08-23).
	//
	// # Why it is a field rather than part of Intent
	//
	// D20 made `intent` one recital with one home, and P07 writes it verbatim into every
	// signature's `/Reason`. Real instruments have parties signing in DIFFERENT capacities —
	// as principal, as guarantor, "signed for and on behalf of" a company — and a witness is
	// bound by nothing the principal agreed to. Under one shared intent a witness's own key
	// signs "I agree to sign this document", which is an affirmative false statement about
	// that party's obligation, inside the artifact, signed by them.
	//
	// # Why it is inside the commitment, on the same argument that put Signs there
	//
	// A convener who could present one roster to the signers and another to a verifier,
	// differing only in who was obliged to do WHAT, would have both hash the same. Capacity
	// is a claim about a party's AUTHORITY, which is what a third party relies on.
	//
	// Empty is the ordinary case and renders nothing; a ceremony that does not need
	// capacities must not look misconfigured. **The preimage chunk is written even when the
	// value is empty** — see rosterPreimage, where the reason is that a conditional chunk
	// makes the chunk count vary with the data.
	Capacity string `json:"capacity,omitempty"`
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
	// DigestVersion is the ContentDigestVersion `DocHash` was computed under (D32).
	//
	// # Why a number that is already inside the digest needs to be beside it too
	//
	// `pdfops.ContentDigestVersion` is bound INTO the digest, and its own doc comment says
	// that exists so "improving the coverage" does not "accuse a counterparty of tampering".
	// **It did not achieve that, and could not.** Binding a version inside a hash changes the
	// hash; it cannot produce a sentence, because the reader has nothing to compare. Measured
	// at the P07.S02 grill: the constant occurred three times in the whole tree, all inside
	// `attachments.go`, with no reader anywhere. So a build whose digest covers more than the
	// writer's told the user *"these are not the same document"* — the exact accusation the
	// constant was added to prevent, produced by a Nib point release.
	//
	// D32's rule is that a version mismatch produces a sentence naming the mismatch. That
	// needs the version carried where a reader can reach it, which is here. `FormatVersion`
	// has had this shape since v1.116.17 and this is the same remedy applied to the number
	// that lacked it.
	//
	// Inside the preimage, so the convener signs which digest rule they used — otherwise it
	// is an unsigned hint a tamperer sets to whatever makes the comparison pass.
	DigestVersion int `json:"digestVersion"`
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

// rosterPreimage is the exact byte string RosterHash digests.
//
// Split out at P04.S03 so the preimage can be ASSERTED rather than merely described. The
// domain tag added there was covered by nothing — deleting it left the whole repo green —
// because a function that returns only a digest gives a test nothing to look at. See
// preimage.go.
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
	// The digest rule DocHash was computed under, v4. Signed, so it cannot be edited to make
	// a comparison pass — see Record.DigestVersion for why carrying it beside the hash is not
	// redundant with binding it inside the hash.
	p.addUint(uint64(r.DigestVersion))
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
		// Capacity, v4. **Written unconditionally, including when empty**, and that is the
		// whole of its correctness argument rather than a style choice.
		//
		// The builder's injectivity rests on every chunk carrying a fixed 8-byte length, which
		// makes the encoding prefix-free — but only while the NUMBER of chunks is determined by
		// the roster's length rather than by its contents. `if party.Capacity != "" { add }`
		// would make the chunk count vary with the data, and injectivity would then rest on a
		// length coincidence (fingerprints always 32, signs always 1) instead of on the prefix
		// property. C19's "an empty capacity renders nothing" is about RENDERING and is one
		// careless reading away from an omitted chunk.
		p.addString(party.Capacity)
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

// ErrNotCanonical reports a record whose stored form is not the one its own commitment
// binds. Distinct from a tampering accusation, because it is neither: it is a record built
// by a door that did not canonicalise.
var ErrNotCanonical = errors.New("the ceremony record is not in canonical form")

// Canonical returns the record in its ONE representation.
//
// # Why this exists: every axis the preimage NORMALISES is malleable
//
// `rosterPreimage` does not digest the fields as stored. It digests a PROJECTION of them:
// the fingerprint is `hex.DecodeString`d to 32 raw bytes, so letter case is folded away, and
// `Expires` is rendered `.UTC().Format(time.RFC3339)`, so the location and everything finer
// than a second are dropped. Each of those is a place where two DIFFERENT stored records
// produce ONE commitment and ONE valid ConvenerSig.
//
// Both were measured at the P07.S02 grill, and neither is theoretical:
//
//   - **Case.** An on-path party uppercases one roster fingerprint. Verify passed, the hash
//     was byte-identical, the signature still verified — and the invited party was then
//     refused by `MatchesRecord` with a message printing two strings that differ only in
//     case. A targeted denial reading as an accusation of forgery against the convener.
//   - **Sub-second.** Two records whose deadlines differ by 999ms hash identically and both
//     verify, while the JSON carries the difference. Bounded to under a second, so small —
//     but it is the same shape, and the same fix closes it.
//
// The repair is not to loosen the comparisons around the commitment; that is a check to
// forget at the next call site. It is to make the disagreement UNREPRESENTABLE: the record is
// canonicalised before it is signed, and `Verify` refuses one that is not. Then the stored
// bytes are derivable from the preimage and there is nothing to disagree about. Same move,
// and the same reasoning, as deleting `Party.Name`.
func (r Record) Canonical() Record {
	out := r
	// The digest rule this build writes. Set here rather than at the convene door for the
	// same reason Sign canonicalises here: a rule enforced at the caller is one the next
	// caller forgets. A record arriving from the wire keeps whatever it carries — Canonical
	// is only ever applied to a record this build is about to sign.
	if out.DigestVersion == 0 {
		out.DigestVersion = pdfops.ContentDigestVersion
	}
	// UTC and second-truncated, because that is exactly what the preimage renders.
	out.Expires = r.Expires.UTC().Truncate(time.Second)
	out.Roster = append([]Party(nil), r.Roster...)
	for i := range out.Roster {
		out.Roster[i].Fingerprint = strings.ToLower(out.Roster[i].Fingerprint)
	}
	return out
}

// IsCanonical reports whether r is already in the form Canonical would return.
//
// Compared field-by-field against `Canonical()` rather than by re-encoding, so a future
// normalised axis is caught by adding it to `Canonical` alone.
func (r Record) IsCanonical() bool {
	c := r.Canonical()
	if !r.Expires.Equal(c.Expires) || r.Expires.Location() != time.UTC {
		return false
	}
	if len(r.Roster) != len(c.Roster) {
		return false
	}
	for i := range r.Roster {
		if r.Roster[i] != c.Roster[i] {
			return false
		}
	}
	return true
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
	// Canonicalise BEFORE hashing, so the bytes that get stored are the bytes the commitment
	// binds. Doing it here rather than at the convene door is deliberate: Sign is the one
	// place a record becomes authoritative, and a rule enforced at the caller is a rule the
	// next caller forgets (ADR-009).
	*r = r.Canonical()
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
	// **The id's SHAPE, and it is here for ordering rather than for path safety** (/pending 308).
	//
	// The traversal exposure is already closed: `MirrorDir` refuses a bad id before
	// `filepath.Join` is reached, at every path site. What it cannot fix is WHEN. Without this
	// check `checkArrival` → `CheckRecord` → `Verify` passes, `MatchesRecord` passes (a hostile
	// id equals itself), the user consents, `Contribute` runs, and only then does
	// `persistContribution` → `WriteMirror` → `MirrorDir` refuse — so the convener holds the
	// signature and the signer is told "Signed, but not saved". The refusal belongs BEFORE
	// consent, and this is where the record is first judged.
	//
	// Placed after the version check so a future format reads as a skew rather than as a
	// malformed id, and before the signature check for `IsCanonical`'s stated reason: a
	// mis-built record should be reported as mis-built rather than accused of forgery.
	//
	// It refuses no legitimate record. `Verify` gates on `r.Version != FormatVersion` — exact
	// equality — and `NewID` (32 lowercase hex, one non-test caller) is the only producer, so no
	// record any Nib has emitted can fail this.
	if err := ValidID(r.ID); err != nil {
		return err
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
	// Canonical form, BEFORE the signature check — see Canonical's doc for the two measured
	// malleabilities this closes. It comes first because a non-canonical record whose
	// signature also fails should be reported as the thing the reader can act on: the
	// signature would verify, since the preimage folds exactly the axes that differ, so
	// "forged" would be the wrong word for a record that is merely mis-built.
	if !r.IsCanonical() {
		return fmt.Errorf("%w: its stored form is not the form its own commitment binds, so two "+
			"different records could carry this signature", ErrNotCanonical)
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
	// **The text bounds, and they are here for the reason the roster bound one block up is**
	// (/pending 308's grill). `checkIntent` and `checkRosterText` had exactly two callers, both
	// in `Convene` — the CONVENER's own door. So the caps bound the emitter and left every
	// recipient unbounded, which is the precise mirror of the asymmetry the roster-bound comment
	// above already describes. Measured before the fix: `Verify` accepted a 5003-rune intent and
	// 5000-rune labels and capacities.
	//
	// It matters more after P08: D25 allocates signature pages from this text's rendered height,
	// and S05 puts the record's intent into a delivered filename on every party's disk.
	if err := checkIntent(r.Intent); err != nil {
		return err
	}
	if err := checkRosterText(r.Roster); err != nil {
		return err
	}
	// The JOINT height rule (/pending 286): a record whose blocks cannot be drawn legibly is one
	// this build would render past the floor, and Verify is where a record arriving from ANOTHER
	// machine is judged.
	if err := checkBlocksFit(r.Roster, r.Intent); err != nil {
		return err
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
func (r Record) Hop(a, b string) (int, error) {
	c, ok := r.Convener()
	if !ok {
		return 0, fmt.Errorf("%w: this record's convener is not in its own roster", ErrNotAHop)
	}
	return hopBetween(r.Roster, c.Fingerprint, a, b)
}

// hopBetween is the one door. Record and Invitation both carry a roster and both are asked
// for hop numbers, and two implementations of one rule is the shape ADR-009 exists to refuse
// — here it would be worse than usual, because the two would disagree only for the rosters
// where it mattered.
//
// **The topology is a HUB and this is the fact the first version of this function got
// wrong.** D22: "the convener writes the record, prepares the document, dials each party in
// roster order… Every hop is exactly today's two-party session: one dialer, one listener,
// one pinned peer." So the convener is one end of EVERY hop, and two non-convener parties
// never share one — Alice and Bob in a three-party ceremony never speak. The first version
// read "the order of the roster IS the signing order" (Party's own doc) and inferred a
// CHAIN, roster[i] to roster[i+1], which is a different ceremony: it had Alice handing to
// Bob directly, gave them a shared hop key, and would have had each party arming for a peer
// D22's own TRIPWIRE argument says they never accept.
//
// Hops are numbered by the roster position of the NON-convener party, so hop 0 is the
// convener's first counterparty. The convener need not be roster[0]; it is whoever the
// record's certificate resolves to.
func hopBetween(roster []Party, convener, a, b string) (int, error) {
	if !strings.EqualFold(a, convener) && !strings.EqualFold(b, convener) {
		return 0, fmt.Errorf("%w: %s and %s are both counterparties, and under a convener "+
			"hub every hop has the convener at one end", ErrNotAHop, short(a), short(b))
	}
	if strings.EqualFold(a, b) {
		return 0, fmt.Errorf("%w: %s was given as both ends", ErrNotAHop, short(a))
	}
	other := a
	if strings.EqualFold(a, convener) {
		other = b
	}
	n := 0
	for _, p := range roster {
		if strings.EqualFold(p.Fingerprint, convener) {
			continue
		}
		if strings.EqualFold(p.Fingerprint, other) {
			return n, nil
		}
		n++
	}
	return 0, fmt.Errorf("%w: %s is not in this ceremony's roster", ErrNotAHop, short(other))
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
