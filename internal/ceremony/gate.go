package ceremony

import (
	"errors"
	"fmt"
	"net/netip"
	"time"
)

// The flood cap (plan-review C2, D33), and the object it belongs to.
//
// # Why a gate and not a check
//
// P04.S04's acceptance bounds what "one rendezvous key" may yield. A key is a BEP-44
// target, and a target yields a **sequence** of records across D16's 300 s race — clock 1
// says candidates trickle, and the counterparty legitimately republishes as its own
// gathering completes. So `MaxCandidates` inside `parseCandidate` is a *message validity*
// rule and cannot see the thing the criterion is about: eight candidates per record, times
// however many records arrive, is not eight.
//
// The gate is that missing unit. Its lifetime IS one (hop, counterparty) pair, so "no more
// than N per key" becomes a property of an object rather than a hope about a loop.
//
// # Two refusal shapes, deliberately, at two objects
//
// A RECORD that carries more than N is refused **whole** — `parseCandidate` does that, and
// it is stronger than the plan's original "drop the N+1th". A record is one signed
// statement: keeping 8 of 12 acts on a statement the signer never made, and since the
// attacker orders the list, "keep the first N" hands them the selection. No honest peer can
// produce one either, because `Sign` refuses to build it — so an over-cap record is
// evidence of a modified peer, not a large one.
//
// A CANDIDATE beyond N in the ACCUMULATED SET is dropped and counted. That set is assembled
// locally from several *valid* records, nobody signed it as a whole, and the race must
// degrade rather than abort — which is what D33's "drops and reports; it never fails the
// ceremony" actually asks for. Refusing a record does not fail the ceremony either: the
// ladder falls to the next tier (D19 cause 2).
//
// # It composes, it does not replace
//
// `OpenCandidate` and `Verify` stay separate, for the reason OpenCandidate's own comment
// gives — a caller holding neither roster nor expected fingerprint must not be able to
// believe it has checked something. The gate holds both and runs them in order.

// CandidateStats is one counter per cause. Split rather than summed because they want
// different advice: a wrong-key record means somebody else is publishing at our target, an
// author mismatch means a roster member is forging, and an expired one means we are late.
type CandidateStats struct {
	// Accepted is candidates added to the race set.
	Accepted uint64
	// DroppedOverCap is candidates from VALID records discarded because the set was full.
	// This is the acceptance's "the N+1th is dropped and reported".
	DroppedOverCap uint64
	// DroppedDuplicate is a candidate repeated WITHIN one record. **Not cosmetic**: eight
	// copies of one address concentrate the entire per-candidate punch budget on a single
	// victim, which is the worst case the cap does not otherwise bound. No honest
	// publisher emits one, so non-zero here is a modified peer.
	DroppedDuplicate uint64
	// Reoffered is a candidate we already hold, arriving again in a LATER record.
	//
	// Split from DroppedDuplicate because they mean opposite things and the ordinary path
	// produces this one constantly: BEP-44 serves the same value to every fetch, and D16's
	// amended cadence runs roughly ten fetches across a 300 s race, so an honest peer with
	// eight candidates and one publish yields Accepted=8 and Reoffered=72. Counting those
	// as duplicates would make the attack indicator read high on every healthy ceremony —
	// an alarm that is always on is not an alarm.
	Reoffered       uint64
	acceptedRecords uint64

	// EmptyRecords is valid, verified records carrying no candidates at all.
	//
	// Without it such a record moves NOTHING — not Accepted, not any refusal, and not
	// rendezvous.Stats().FetchEmpty either, since something was in fact there. That is a
	// hole in the exact instrument built to separate "a record was present and failed"
	// from "nothing was there". Only the counterparty can produce one (the author check
	// bars everyone else), so it is a diagnosis, not an attack.
	EmptyRecords uint64

	// Refusals, by cause. Their SUM is "a record was present at the peer's target and
	// failed the inner check", which is the fact P04.S03 filed against this slice: without
	// it, in-roster preemption is indistinguishable from an offline peer, because both
	// look like an empty fetch.
	RefusedSealed     uint64 // wrong key/salt/hop, or altered
	RefusedTooBig     uint64 // over MaxSealedRecord, refused before any decryption
	RefusedFormat     uint64 // decrypted and did not parse
	RefusedTooMany    uint64 // more than MaxCandidates in one record
	RefusedUnroutable uint64 // named an address Nib will not dial
	RefusedSignature  uint64 // signature did not verify
	RefusedAuthor     uint64 // signed by someone other than the party claimed — in-roster forgery
	RefusedContext    uint64 // another ceremony or another hop
	RefusedExpired    uint64 // valid, and stale
}

// Refused is the total across every cause — the "something was there and it was wrong"
// number, to be read beside rendezvous.Stats().FetchEmpty ("nothing was there").
func (s CandidateStats) Refused() uint64 {
	return s.RefusedSealed + s.RefusedTooBig + s.RefusedFormat + s.RefusedTooMany +
		s.RefusedUnroutable + s.RefusedSignature + s.RefusedAuthor + s.RefusedContext +
		s.RefusedExpired
}

// Records is every record the gate has been offered — accepted, empty or refused. The
// denominator for Refused(), which is a count of RECORDS while Accepted counts ADDRESSES.
func (s CandidateStats) Records() uint64 {
	return s.Refused() + s.EmptyRecords + s.acceptedRecords
}

// CandidateGate accumulates one counterparty's candidates at one hop.
//
// Not safe for concurrent use: one gate belongs to one hop's race, which is one goroutine's
// loop. Sharing one across hops would defeat the unit it exists to be.
type CandidateGate struct {
	inv  Invitation
	hop  int
	want string
	key  []byte
	salt []byte

	seen  map[netip.AddrPort]bool
	addrs []netip.AddrPort
	stats CandidateStats
}

// NewCandidateGate derives the hop's record key and the counterparty's salt once.
func NewCandidateGate(inv Invitation, hop int, counterparty string) (*CandidateGate, error) {
	key, err := inv.RecordKey(hop)
	if err != nil {
		return nil, err
	}
	salt, err := inv.RecordSalt(hop, counterparty)
	if err != nil {
		return nil, err
	}
	// Copy the roster: inv is stored by value but Roster is a slice header, so a caller
	// mutating inv.Roster after construction would silently rewrite the gate's
	// authorization list. NewInvitation copies for the same reason.
	inv.Roster = append([]Party(nil), inv.Roster...)
	return &CandidateGate{
		inv: inv, hop: hop, want: counterparty,
		key: key, salt: salt,
		seen: make(map[netip.AddrPort]bool),
	}, nil
}

// Salt is the BEP-44 salt this gate reads at — handed to the transport as opaque bytes,
// since the transport may not know what a ceremony is (L1).
func (g *CandidateGate) Salt() []byte { return g.salt }

// Accept opens, verifies and admits one fetched record.
//
// It returns the error so a caller can log or diagnose, but the counters are the report:
// the acceptance says a counter something reads, not a log line.
func (g *CandidateGate) Accept(sealed []byte, now time.Time) error {
	// The ceiling, HERE, at the read — not only at Seal.
	//
	// MaxSealedRecord was a producer-only bound: `Seal` refused over 996 bytes and `Accept`
	// took a slice of any length straight to `aead.Open`, which allocates len(ct). The only
	// thing bounding it was anacrolix/dht's bencode check, in a package L1 forbids this one
	// from importing — so the ceiling was a hope about a transitive dependency rather than a
	// property of this package. Both other doors in this package already argue the opposite:
	// validateSeeds is "called at BOTH doors, and that is the whole point", and
	// parseCandidate is "Refused HERE, at the parse, not at the dialer".
	if len(sealed) > MaxSealedRecord {
		// Its OWN cause. This incremented RefusedSealed, whose documented meaning is "wrong
		// key/salt/hop, or altered" — a tampering signal. An oversize record has not been
		// decrypted at all, so nothing yet says it was altered, and the counter exists
		// precisely so an operator can tell in-roster preemption from noise. Folding a
		// size refusal into the tampering count makes the one reading that matters wrong.
		g.stats.RefusedTooBig++
		return fmt.Errorf("%w: %d bytes, cap is %d", ErrCandidateTooBig, len(sealed), MaxSealedRecord)
	}
	rec, err := OpenCandidate(g.key, g.salt, g.hop, sealed)
	if err != nil {
		switch {
		case errors.Is(err, ErrCandidateSealed):
			g.stats.RefusedSealed++
		case errors.Is(err, ErrCandidateTooMany):
			g.stats.RefusedTooMany++
		case errors.Is(err, ErrCandidateUnroutable):
			g.stats.RefusedUnroutable++
		default:
			g.stats.RefusedFormat++
		}
		return err
	}
	if err := rec.Verify(g.inv, g.hop, g.want, now); err != nil {
		switch {
		case errors.Is(err, ErrCandidateSignature):
			g.stats.RefusedSignature++
		case errors.Is(err, ErrCandidateAuthor):
			g.stats.RefusedAuthor++
		case errors.Is(err, ErrCandidateContext):
			g.stats.RefusedContext++
		case errors.Is(err, ErrCandidateExpired):
			g.stats.RefusedExpired++
		case errors.Is(err, ErrCandidateUnroutable):
			// Reachable only if parseCandidate's own check is ever reordered after Verify.
			// Its correctness would otherwise depend on the ORDER of two checks in a
			// different function, which is exactly the coupling the split exists to remove.
			g.stats.RefusedUnroutable++
		case errors.Is(err, ErrCandidateTooMany), errors.Is(err, ErrCandidateTooBig):
			g.stats.RefusedTooMany++
		default:
			g.stats.RefusedFormat++
		}
		return err
	}
	g.stats.acceptedRecords++
	if len(rec.Addrs) == 0 {
		g.stats.EmptyRecords++
		g.stats.acceptedRecords--
		return nil
	}
	inThisRecord := make(map[netip.AddrPort]bool, len(rec.Addrs))
	for _, a := range rec.Addrs {
		switch {
		case inThisRecord[a]:
			// Repeated inside one record: nobody honest does this.
			g.stats.DroppedDuplicate++
			continue
		case g.seen[a]:
			// We already hold it from an earlier record. The ordinary case.
			inThisRecord[a] = true
			g.stats.Reoffered++
			continue
		}
		inThisRecord[a] = true
		if len(g.addrs) >= MaxCandidates {
			// The cap is on the TOTAL distinct addresses admitted over the race, not on
			// the live set, and it is deliberately not an eviction policy. Evicting to
			// make room would keep the set at eight while letting the race punch at
			// sixteen distinct victims over its life — which is the amplification the cap
			// exists to bound, defeated by the thing meant to enforce it.
			//
			// **The cost, recorded rather than discovered:** D15 refreshes a port mapping
			// while armed, and a refreshed mapping is a NEW external port. A peer whose
			// eight candidates are already admitted cannot then publish its live one, and
			// the race spends the rest of its budget dialling an endpoint that has expired.
			// That trade is only measurable once the ladder and the mapping refresh both
			// exist, so it is filed for P05 rather than guessed at here.
			g.stats.DroppedOverCap++
			continue
		}
		g.seen[a] = true
		g.addrs = append(g.addrs, a)
		g.stats.Accepted++
	}
	return nil
}

// Candidates is the race set, in arrival order.
func (g *CandidateGate) Candidates() []netip.AddrPort {
	return append([]netip.AddrPort(nil), g.addrs...)
}

// Stats is the report.
func (g *CandidateGate) Stats() CandidateStats { return g.stats }
