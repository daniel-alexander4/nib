package server

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"nib/internal/ceremony"
	"nib/internal/sign"
	"nib/internal/vault"
)

// Close-out: the end of D29's lifecycle, on this machine (P08.S06).
//
// D29 states it once — **end state → delivery round → close-out** — and puts the pin drop and the
// prune at the last step. `internal/ceremony/closeout.go` holds the prune's disk half and the whole
// argument for why it is a move; this file is the part that also has a vault open, because three of
// the four stores a close-out touches are in it.

// closeOutStores tears down every store that holds material scoped to one ceremony.
//
// **FOUR stores, and the count is the finding.** P08.S06's own scope said three — pins, secrets and
// the mirror — and `unconvene`, the only teardown in the tree when this was written, takes four:
// the invitee-side stored invitation is the fourth, added at P08.S01 with the reason this door
// inherits verbatim — *"'almost always' is not a reason for a teardown to reach one of two
// stores, and a convener that also accepted an invitation to its own ceremony is exactly the case
// nobody would think to test."* Built to the stated three, a close-out would leave the ceremony
// secret on disk: precisely the defect S01 fixed one door over, re-introduced by a plan bullet
// written before that fix existed.
//
// **Reported separately, never aggregated**, for `unconvene`'s reason: the four fail independently
// and a user can act on each. Returns the first error so a caller can report the close-out as
// incomplete, having already tried all four — a teardown that stops at the first failure leaves
// more behind than one that does not.
//
// **The mirror is NOT one of the four here.** It is the caller's, because what happens to it
// differs: `unconvene` deletes (a convene that never committed has no contribution to keep) and
// close-out moves. Folding it in would have forced one of the two to take the wrong verb.
func closeOutStores(v *vault.Vault, id, why string) error {
	var first error
	note := func(err error, what string) {
		if err == nil {
			return
		}
		if first == nil {
			first = err
		}
		log.Printf("%s %s: %s: %v", why, id, what, err)
	}
	if _, err := v.PruneCeremonySecrets(id); err != nil {
		note(err, "could not remove its invitation secrets from the vault — they are key "+
			"material and this machine still holds them")
	}
	if _, err := v.PruneCeremonyInvitations(id); err != nil {
		note(err, "could not remove its stored invitation — it carries the ceremony secret "+
			"and this machine still holds it")
	}
	if _, err := v.PruneCeremonyPeers(id); err != nil {
		note(err, "could not remove its peer pins — the peer list still carries pins this "+
			"machine took on only for this ceremony")
	}
	return first
}

// closeOutCeremony is the ONE close-out door: the mirror moves, then the vault stores go.
//
// **The order is load-bearing and it is the opposite of the intuitive one.** The move first, so
// that if the vault teardown fails the user still has their signed contribution at a path they can
// be told about; the stores second, because they are the half that destroys. Reversed, a machine
// that lost its vault mid-close-out would have dropped the pins and secrets and still be holding a
// live-looking ceremony directory.
//
// **A failed vault teardown does NOT undo the move.** There is nothing to undo it to — the
// directory's new location is the safe one, and rolling it back into the live set would put a
// ceremony the pins no longer support back in front of the sweep to try again forever.
func (s *Server) closeOutCeremony(v *vault.Vault, id, state string, now time.Time) error {
	root := defaultOutputDir()
	if err := ceremony.CloseOutMirror(root, id); err != nil {
		if errors.Is(err, ceremony.ErrAlreadyClosedOut) {
			// A second pass over a ceremony the previous one moved. The stores below are
			// idempotent, so falling through finishes a close-out interrupted between the two
			// halves rather than reporting a fault about work already done.
			log.Printf("close-out %s: already moved; finishing the vault teardown", id)
		} else {
			return fmt.Errorf("could not move this ceremony's folder out of the live set: %w", err)
		}
	}
	// **The receipt is written between the two halves**, not after both. It is what the sweep
	// reads to know this ceremony has been dealt with, and a close-out whose vault teardown fails
	// must not be retried from the top — the move has already happened and the stores are
	// idempotent. Written after a failed teardown it would still be correct; written before, it is
	// correct on every path.
	if err := ceremony.WriteReceipt(root, id, ceremony.Receipt{
		Ceremony: id, State: state, ObservedAt: now,
	}); err != nil && !errors.Is(err, ceremony.ErrReceiptConflict) {
		log.Printf("close-out %s: could not write the local receipt: %v — this machine now has "+
			"no record of when it decided the ceremony had ended", id, err)
	}
	// **The fourth store, and it is in memory rather than on disk** (`/pending 312`).
	//
	// `closeOutStores` below takes the three that persist; `punchBudgets` is the one that does
	// not, and it had no `delete` anywhere in the tree — a `*punchBudget` per hop, surviving every
	// disarm and living for the process lifetime. It is dropped HERE, before that call and after
	// the receipt, so it happens on every path that reaches "this machine considers the ceremony
	// over": the receipt is what the sweep reads to know so, and a failing vault teardown must not
	// leave the counters behind when the pins they belonged to are already gone.
	//
	// Logged only when something was dropped. Zero is the ordinary case — most ceremonies never
	// punch at all — and a line per close-out saying "dropped 0" is the noise that gets a log
	// ignored.
	if n := s.dropPunchBudgets(id); n > 0 {
		log.Printf("close-out %s: dropped %d punch budget(s)", id, n)
	}
	return closeOutStores(v, id, "close-out")
}

// closeOutEnded sweeps the live ceremonies and closes out the ones that are over.
//
// **Hung off the same door as `rearmDeliveries` and under the same gate**, which is not a
// convenience. `adoptVault` is the ONE place "the vault just opened" happens, and the
// `deliveryRearm` gate exists because *"a `Server` value constructed in a test isolates
// `configDir` and NOT `$HOME`"* — an ungated sweep read the developer's real ceremonies. That
// hazard is strictly worse here: this sweep does not merely read them, it moves them and drops
// their pins.
//
// **Never on a wall-clock timer, and the slice's own bullet says why**: every ceremony route is
// behind `requireUnlocked`, so a machine left locked over a weekend prunes nothing while wall time
// runs — a timer would fire into a vault it cannot open and log a failure per tick. The triggers
// are the two moments a vault is known to be open: this one, and the listing route.
//
// **"At startup" is deliberately not a third trigger.** It was in the plan's bullet and it is
// unreachable: at startup there is no unlocked vault, so three of the four stores cannot be
// touched, and a close-out that moved the directory without dropping the pins is exactly the
// split-brain `closeOutCeremony`'s ordering exists to avoid.
//
// **Identity is resolved ONCE, outside the loop**, and its failure aborts the sweep rather than
// being skipped per ceremony: without a fingerprint this machine cannot tell whether it is the
// convener, and the convener and a signer finish a round on different evidence.
func (s *Server) closeOutEnded(v *vault.Vault, now time.Time) {
	if v == nil {
		return
	}
	// **A non-primary Nib does not close out, and the listing route already says so in words the
	// user reads**: *"another copy of Nib is already running on this machine. This one can show
	// your ceremonies but must not continue or REMOVE them, because both would be writing to the
	// same folder."* This is the code that makes that sentence true. The repo deliberately lets a
	// second Nib run (P08.S03 killed the planned lock on `~/nib/ceremonies/`, because
	// `instanceToken` already carries the signal), so the signal has to be READ at every door that
	// writes — and this door moves directories and drops pins.
	//
	// Checked here rather than at the two call sites: ADR-009, one rule and one door. The unlock
	// hook is `deliveryRearm`-gated and the route is `primary`-aware, and neither of those gates
	// is this one.
	s.mu.Lock()
	primary := s.instanceToken != ""
	s.mu.Unlock()
	if !primary {
		return
	}
	root := defaultOutputDir()
	stored, err := ceremony.ListStored(root, now)
	if err != nil {
		return
	}
	cert, _, ierr := identity(v)
	if ierr != nil {
		return
	}
	myFP, ferr := sign.Fingerprint(cert)
	if ferr != nil {
		return
	}
	me := hex.EncodeToString(myFP)
	for _, st := range stored {
		rec, _, rerr := ceremony.ReadMirror(root, st.ID, now)
		if rerr != nil {
			// Unreadable from the stronger door even though the listing classified it `LoadOK`.
			// Left alone for `closeOutReason`'s reason: a directory is never moved on the
			// strength of a record this machine could not read.
			continue
		}
		state, ok := closeOutReason(st, rec, me, now)
		if !ok {
			continue
		}
		if cerr := s.closeOutCeremony(v, st.ID, state, now); cerr != nil {
			log.Printf("close-out %s: %v", st.ID, cerr)
		}
	}
}

// closeOutReason decides whether one stored ceremony is over, and what to call it.
//
// **Two ways in, and the delivery round gates the first.** A ceremony that ended — declined or
// completed — is closed out only once its round has finished, because D29's lifecycle puts the
// round BEFORE the close-out and the round needs the very mirror this would move.
// `Stored.Ended` is set from a verified termination and is what the round's own re-arm reads.
//
// **The second way in needs no end state at all**, and it is what the grace exists for: a record
// whose own `Expires` passed more than `closeOutGrace` ago with nothing having said what happened.
// That is `StateAbandoned` — a conclusion, never an attestation, which is why `Termination`'s set
// is closed at two and the receipt's is not. It is also the escape hatch that stops one
// permanently unreachable party pinning a directory open forever: past the grace, an unfinished
// round no longer holds the close-out.
//
// **A ceremony that cannot be READ is never closed out.** An unparseable or unverifiable
// `record.json` has no trustworthy `Expires`, and moving a directory on the strength of a field
// that did not verify is how a live ceremony disappears because one file was damaged. P08.S03
// built those four classes for exactly this decision and they all mean "leave it".
func closeOutReason(st ceremony.Stored, rec ceremony.Record, me string, now time.Time) (string, bool) {
	if st.State != ceremony.LoadOK {
		return "", false
	}
	past := !st.Expires.IsZero() && now.After(st.Expires.Add(closeOutGrace))
	switch {
	case st.Ended != "":
		if roundIsFinished(rec, me, st.Ended) || past {
			return st.Ended, true
		}
	case alreadyDelivered(rec) && roundIsFinished(rec, me, ceremony.StateCompleted):
		// **C11's central case, and it needs no termination.** *"A ceremony directory is gone
		// after the ceremony has ended and its document has been DELIVERED or saved."* On a
		// completed ceremony the round delivers the finished document and not an attestation —
		// `runDeliveryRound` chooses between the two — so a signer never receives a `completed`
		// termination and `Stored.Ended` stays empty on its machine forever.
		//
		// Without this branch the only way out was the grace, which meant every completed
		// ceremony sat in the live set for three days past its deadline and was then recorded as
		// `abandoned` — a durable local lie about a proceeding that finished exactly as intended.
		// Measured at tier 4: two relay ceremonies whose documents were in `~/nib/signed/` were
		// still in `~/nib/ceremonies/` with nothing able to move them.
		//
		// **The arrival of the document is itself the evidence**, which is why it is enough.
		// `checkDelivered` has a completeness clause, so what lands here is the FINISHED document
		// — every signature present. A ceremony cannot produce one and still be live.
		return ceremony.StateCompleted, true
	case past:
		return ceremony.StateAbandoned, true
	}
	return "", false
}

// roundIsFinished answers whether the delivery round is done with this ceremony, on THIS machine.
//
// **The convener and a signer finish on different evidence, and that asymmetry is the function.**
// The convener runs the round, so what it has is one `delivered/<fingerprint>` marker per party it
// reached — `markDelivered`'s files, written when a party acknowledged its copy. A signer never
// runs a round; what it has is the payload that arrived.
//
// **And WHICH payload depends on the end state, which is the half the first cut got wrong.**
// `runDeliveryRound` chooses between two: a completed ceremony delivers the finished document, a
// declined one delivers the convener's signed termination, *"because there is no finished document
// and the parties who already signed are otherwise left believing it is still travelling."* The
// first cut asked `alreadyDelivered` for both — a stat on `~/nib/signed/<name>`, the FINISHED
// DOCUMENT's path — so on a declined ceremony it asked after a file the round will never send.
// That is permanently false, so a signer who had been told the proceeding was over held the
// directory and its pins until the three-day grace expired, on a ceremony nothing was going to add
// to. Found by trying to drive the clause at tier 4, not by reading this function.
//
// A signer learns `ended` only by receiving and verifying the termination — `ReadStored` reads it
// from this machine's own directory — so on the declined path the end state IS the delivery
// receipt, and asking for a second one asks twice.
//
// **The party that ENDED the proceeding is not required to have a marker**, for the reason
// `runDeliveryRound` already skips them: declining closes out on their machine, so they have
// nothing to arm a rendezvous with, and they hold no mirror to check an attestation against. A
// round that skipped them is finished, not incomplete — waiting for a marker that by construction
// will never be written would hold every declined ceremony open until the grace ran out.
func roundIsFinished(rec ceremony.Record, me, ended string) bool {
	if !strings.EqualFold(me, convenerFingerprintOf(rec)) {
		if ended == ceremony.StateDeclined {
			return true
		}
		return alreadyDelivered(rec)
	}
	ender := endedBy(rec.ID)
	for _, party := range rec.Roster {
		switch {
		case strings.EqualFold(party.Fingerprint, me):
			continue // the convener already holds it
		case ender != "" && strings.EqualFold(party.Fingerprint, ender):
			continue // skipped by the round itself, and correctly
		case !wasDelivered(rec.ID, party.Fingerprint):
			return false
		}
	}
	return true
}
