package p2p

import (
	"errors"
	"fmt"
	"strings"

	"nib/internal/sign"
)

// L3 — no contribution out of roster order (D23, P07.S03).
//
// # Why the predicate lives here, in primitives
//
// L3 must hold at both contribution entry points, and they are in different packages:
// `coSignExchange` below adds the RECEIVING party's block, and `internal/server`'s
// `buildCoSigned` adds the INITIATING party's. ADR-009 says the rule is written once and every
// site calls it, so it has to be somewhere both can reach — which is here.
//
// **The obvious shape does not compile, and that is measured rather than assumed.** The plan's
// task was to break an import cycle so `p2p` could take `ceremony.Record` directly. That cycle is
// now a PRODUCTION cycle: P07.S02a put `ceremony.Convene` in `internal/ceremony` calling `p2p`
// (`convene.go`), so adding the reverse edge fails to build —
// `imports nib/internal/ceremony from session.go / imports nib/internal/p2p from convene.go:
// import cycle not allowed`.
//
// And no cycle needs breaking. `ReadAttestations` already returns every fact L3 asks about, in
// signature order, so the predicate takes a roster of primitives and the caller — the side that
// holds the record it verified at arm time — supplies it. That is also what the slice's scope
// requires: **the gate reads the record the party verified at arm time**, never the one carried
// by the document, because a gate reading the document's own record answers its own question.

// RosterEntry is one party's position in the signing order, as L3 needs it.
type RosterEntry struct {
	// Fingerprint is the hex SPKI. Compared case-insensitively throughout, because a roster
	// reaches this from JSON and `ParseInvitation` normalises only the invitation's copy.
	Fingerprint string
	// Signs is whether this party is obliged to sign. A non-signing convener holds a position
	// in the roster and none in the signing order (D22), so the two sequences differ and the
	// prefix rule is about the SIGNING one.
	Signs bool
}

// Roster is what the gate is handed: the signing order, and the commitment every signature in
// this proceeding should carry.
type Roster struct {
	Entries []RosterEntry
	// Commitment is the record's RosterHash, hex, or "" when the caller has none to offer.
	//
	// **Checked where present and never REQUIRED, and that is a stated limit rather than
	// leniency.** No production attestation carries one today: neither `coSignExchange`'s `att`
	// nor `internal/server`'s `cosignAttestation` sets `Attestation.RosterHash`, so a gate that
	// demanded a commitment would refuse every honest ceremony — the same mistake P07.S02b
	// caught one door over, where `CheckDocument` would have refused every honest hop-1 arrival.
	// Making signatures carry it is **P07.S04's** (`att` sets no RosterHash is that slice's own
	// opening sentence). `TestTheCommitmentCheckIsLimitedUntilS04` asserts this boundary so the
	// next reader finds it instead of a green they will trust.
	Commitment string
}

var (
	// ErrNotYourTurn: this party is in the roster and it is somebody else's turn.
	ErrNotYourTurn = errors.New("it is not this party's turn to sign")
	// ErrNotInRoster: this party is not a signing member of this ceremony at all.
	//
	// Distinct from ErrNotYourTurn because they are different facts about the user and lead to
	// different actions: waiting, versus holding the wrong invitation.
	ErrNotInRoster = errors.New("this party is not one of the ceremony's signers")
	// ErrPrefixMismatch: the signatures already on the document are not the roster prefix.
	ErrPrefixMismatch = errors.New("the signatures on this document are not the ceremony's signing order")
	// ErrPrefixUnproven: a signature in the prefix does not verify, or is not cross-bound.
	//
	// Separate from ErrPrefixMismatch because the identities can be exactly right while the
	// evidence is not — L3 and D23 both say "each one valid and cross-bound", and a check that
	// only compared identities would satisfy the sentence while missing half of it.
	ErrPrefixUnproven = errors.New("a signature already on this document does not prove what the roster requires")
	// ErrProceedingMismatch: a signature carries a commitment to a different ceremony.
	ErrProceedingMismatch = errors.New("this document's signatures are not all from this ceremony")
	// ErrCeremonyComplete: every signing party has signed; there is no next contributor.
	ErrCeremonyComplete = errors.New("every signing party has already signed this document")
)

// NextContributor answers *whose turn is it* — the question form of L3.
//
// **The question form is not a convenience.** P06's own criterion promises the panel computes its
// enabled action from "the same function the server's L3 check uses"; a predicate that could only
// refuse would force that slice to retrofit a read-only query, which is two derivations of one
// rule and the ADR-009 shape this gate exists to avoid.
//
// It returns the roster entry whose contribution the document is waiting for, having first
// established that everything already on the document is the prefix before it.
func NextContributor(pdf []byte, r Roster) (RosterEntry, error) {
	signing := make([]RosterEntry, 0, len(r.Entries))
	for _, e := range r.Entries {
		if e.Signs {
			signing = append(signing, e)
		}
	}
	if len(signing) == 0 {
		return RosterEntry{}, fmt.Errorf("%w: this roster has no signing parties", ErrPrefixMismatch)
	}
	// **A destroyed signature does not report itself as invalid — it VANISHES, and that is
	// measured.** Tampering with a signed document's body leaves `sign.Verify` reporting
	// `unsigned` with zero signers; corrupting the `/Contents` blob leaves it `invalid`, also
	// with zero signers. `ReadAttestations` iterates `st.Signers`, so in both cases it returns
	// an EMPTY slice and the per-attestation `Valid` check below cannot fire.
	//
	// So the reachable defence is the document's own state, asked here: `invalid` means a
	// signature blob is present and unparseable, which the attestations cannot see at all. A
	// document in that state can never become valid however many honest blocks are added to it,
	// so admitting a contribution onto it produces a file nobody can verify.
	//
	// **What this does NOT reach, stated rather than implied:** a body tamper that destroys the
	// only prior signature leaves the document looking `unsigned`, which at position 1 is a
	// legitimate state and indistinguishable from one. Catching that needs a per-hop continuity
	// check over the document's bytes, which `embed.go` records as unsolved and which S05 and
	// S06 inherit. L3 is about ORDER, and it says so here rather than appearing to cover it.
	if st := sign.Verify(pdf); st.State == sign.Invalid {
		return RosterEntry{}, fmt.Errorf("%w: this document carries a signature that cannot be "+
			"read, so what is already on it cannot be checked against the roster",
			ErrPrefixUnproven)
	}
	ats := ReadAttestations(pdf)
	if len(ats) > len(signing) {
		return RosterEntry{}, fmt.Errorf("%w: the document carries %d signature(s) and the "+
			"ceremony has %d signing part(ies)", ErrPrefixMismatch, len(ats), len(signing))
	}
	for i, a := range ats {
		// Kept as defence and NOT as the reachable check — see the state test above. Measured:
		// no tamper this project can construct produces a signer with `Valid == false`, because
		// the library drops an unparseable signer rather than reporting one. It stays because
		// `SignerAttestation.Valid` is part of the type's contract and a future library that
		// honoured it would otherwise walk straight past here.
		if !a.Valid {
			return RosterEntry{}, fmt.Errorf("%w: signature %d (%s) does not verify",
				ErrPrefixUnproven, i+1, shortFP(a.Fingerprint))
		}
		if !strings.EqualFold(a.Fingerprint, signing[i].Fingerprint) {
			return RosterEntry{}, fmt.Errorf("%w: signature %d is %s and the roster's %s signer "+
				"is %s", ErrPrefixMismatch, i+1, shortFP(a.Fingerprint), ordinal(i+1),
				shortFP(signing[i].Fingerprint))
		}
		// **Cross-binding, and the FIRST signature is exempt — inverted at P07.S05, measured.**
		//
		// `Matched` means the signer's accepted peer is itself a valid signer on this document.
		// This rule was written as "all but the LAST" while `AcceptedPeer` named the wire peer,
		// which in a two-party exchange is the signer's successor — so the most recent signature
		// attested to somebody who had not signed yet.
		//
		// D22's amendment points it the other way: a signature accepts its PREDECESSOR
		// (`PredecessorOf`), so every signature after the first attests to a party who has
		// already signed, and the first accepts nobody because there is nobody before it. That is
		// C14 as amended in as many words — *"every signature that has a signing predecessor
		// reports Matched; the first signer reports its own state"*.
		//
		// Measured when it was still "all but the last": a four-party carry-route ceremony failed
		// at hop 2 with *"signature 1 attests to a peer who is not a valid signer of this
		// document"* — the first signer, exempt under the new direction and not under the old.
		if i > 0 && !a.Matched {
			return RosterEntry{}, fmt.Errorf("%w: signature %d (%s) attests to a peer who is "+
				"not a valid signer of this document", ErrPrefixUnproven, i+1,
				shortFP(a.Fingerprint))
		}
		if r.Commitment != "" && a.RosterHash != "" &&
			!strings.EqualFold(a.RosterHash, r.Commitment) {
			return RosterEntry{}, fmt.Errorf("%w: signature %d commits to proceeding %s and this "+
				"one is %s", ErrProceedingMismatch, i+1, shortFP(a.RosterHash),
				shortFP(r.Commitment))
		}
	}
	if len(ats) == len(signing) {
		return RosterEntry{}, ErrCeremonyComplete
	}
	return signing[len(ats)], nil
}

// PredecessorOf names the signing party immediately BEFORE `me` in the roster, or "" when `me` is
// the first signer or is not a signing member (P07.S05, D22 as amended).
//
// **This is what a signature accepts, and it is not the wire peer.** `Attestation.AcceptedPeer`
// used to be set to whoever was on the other end of the socket, which is correct only while every
// carrier also signs. Under a carry route the wire peer is a non-signing convener, and a signature
// accepting THEM attests to somebody who never signs.
//
// **The PREVIOUS entry, not the next, and the direction is the whole of C14 as amended:** *"every
// signature that has a signing predecessor reports `Matched`; the first signer reports its own
// state"*. `crossBind` sets `Matched` when the accepted party is itself a valid signer **on this
// document**, and only a predecessor can be — a successor has not signed yet, so accepting forward
// would leave every signature unmatched until the one after it landed, and the last one unmatched
// forever. Measured: pointing this forward broke three two-party ceremony tests with *"peer's
// signature does not accept you"*.
//
// The first signer accepts "" and that is correct rather than a gap: there is nobody before them.
func PredecessorOf(r Roster, me string) string {
	signing := make([]string, 0, len(r.Entries))
	for _, e := range r.Entries {
		if e.Signs {
			signing = append(signing, e.Fingerprint)
		}
	}
	for i, fp := range signing {
		if strings.EqualFold(fp, me) {
			if i > 0 {
				return signing[i-1]
			}
			return ""
		}
	}
	return ""
}

// InRoster reports whether a fingerprint is a party to this ceremony at all — signing or not.
//
// **What replaces "the document was signed by the connected peer" inside a ceremony (P07.S05).**
// That check conflated the signer with the socket, which holds only while every carrier signs. The
// honest residue is that the party on the other end is a party to THIS proceeding: the TLS pin
// still says who they are, and L3's prefix rule still says the signatures are the roster's, so
// what this adds is that the two belong to the same ceremony.
func InRoster(r Roster, fp string) bool {
	for _, e := range r.Entries {
		if strings.EqualFold(e.Fingerprint, fp) {
			return true
		}
	}
	return false
}

// AdmitContribution answers *may I contribute now* — the refusal form, over the same predicate.
//
// `me` is the local party's hex SPKI fingerprint.
func AdmitContribution(pdf []byte, r Roster, me string) error {
	next, err := NextContributor(pdf, r)
	if err != nil {
		return err
	}
	if strings.EqualFold(next.Fingerprint, me) {
		return nil
	}
	// In the roster but not next, versus not in it at all: two different facts about the user,
	// and only one of them is answered by waiting.
	for _, e := range r.Entries {
		if strings.EqualFold(e.Fingerprint, me) {
			if !e.Signs {
				return fmt.Errorf("%w: %s is in this ceremony's roster as a non-signing party",
					ErrNotInRoster, shortFP(me))
			}
			return fmt.Errorf("%w: the document is waiting for %s and this is %s",
				ErrNotYourTurn, shortFP(next.Fingerprint), shortFP(me))
		}
	}
	return fmt.Errorf("%w: %s is not in this ceremony's roster", ErrNotInRoster, shortFP(me))
}

func shortFP(fp string) string {
	if len(fp) > 12 {
		return fp[:12] + "…"
	}
	if fp == "" {
		return "(none)"
	}
	return fp
}

func ordinal(n int) string {
	switch {
	case n%100 >= 11 && n%100 <= 13:
		return fmt.Sprintf("%dth", n)
	case n%10 == 1:
		return fmt.Sprintf("%dst", n)
	case n%10 == 2:
		return fmt.Sprintf("%dnd", n)
	case n%10 == 3:
		return fmt.Sprintf("%drd", n)
	}
	return fmt.Sprintf("%dth", n)
}
