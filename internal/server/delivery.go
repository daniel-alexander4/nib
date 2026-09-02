package server

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nib/internal/atomicfile"
	"nib/internal/ceremony"
	"nib/internal/p2p"
	"nib/internal/sign"
)

// The delivery leg (P08.S05d).
//
// A delivery leg hands a FINISHED ceremony document to a party who already signed it. It is not a
// hop: nothing is signed, nobody is asked anything, and the two ends have met before. Everything
// in this file exists because that makes it different from the transfer path it borrows.
//
// # Why both gates are unattended, and why that is not a hole in L2
//
// L2 says no path carries document bytes unconfirmed, and `runVerification` refuses a nil
// `Verifier` outright so a forgotten gate fails closed. Auto-confirming here is legitimate for a
// narrower and stronger reason than "the pin has not changed":
//
//	**These same two identities already completed a spoken check on this pin.** D22 makes the
//	convener a hub, so every delivery leg is convener↔party, and every party ran `runVerification`
//	against the convener at its own hop before it signed. The words are derived from both identity
//	fingerprints and the channel binding; re-deriving them on a second channel between the same
//	two keys re-asks a question that was answered, by the same people, about the same pin.
//
// That argument does NOT extend to a leg between two parties who never met, and the distinction
// matters: if the round ever delivered party-to-party, these gates would have to come back. Stated
// here rather than left implicit, because the weaker "the pin is unchanged" argument would license
// the wider case without anyone noticing the difference.
//
// # And the scope is structural, not a convention
//
// `TestTheUnattendedGatesHaveOneDoor` asserts that the two types below are referenced only from
// `deliverOneLeg`. An auto-confirming verifier reachable from `/api/session/arm` would silently
// remove the spoken check from the interactive path, which is the exact failure `runVerification`'s
// nil check was written to make impossible.

// autoVerifier confirms the spoken check without a human. See this file's header for why that is
// sound on a delivery leg and only there.
type autoVerifier struct{}

func (autoVerifier) ConfirmVerification(string) (bool, error) { return true, nil }

// autoAccepter accepts the delivered document without a human.
//
// **It does NOT write the file.** `sessionAccepter.Accept` persists inside the gate so `ackOK`
// means "the bytes are on disk" (P08.S05a), and a delivery leg needs the same guarantee — but it
// needs a different filename rule and a different verification, so the write is done by the caller
// through `saveDelivered` and this gate only reports consent. The ordering property is preserved
// by `deliverOneLeg`, which persists before it returns to `ReceiveDocument`'s acknowledgement.
type autoAccepter struct {
	// verify is the recipient's own check of what arrived. A gate that accepted first and checked
	// afterwards would acknowledge a document it then refused, which is the false receipt
	// P08.S05a removed from this path one layer down.
	verify func(doc []byte) error
	// saved receives the accepted document so the caller can persist it before the ack. nil is a
	// programmer error rather than "do not save", and it fails closed below.
	save func(doc []byte) error
}

func (a autoAccepter) Accept(_, doc []byte) (bool, error) {
	if a.verify == nil || a.save == nil {
		// Fail closed, for `runVerification`'s reason: a caller that forgot is not a caller that
		// meant to skip. Declining costs a retry; accepting would acknowledge bytes nothing checked.
		return false, errors.New("delivery accepter is not wired: no verify or save")
	}
	if err := a.verify(doc); err != nil {
		return false, err
	}
	if err := a.save(doc); err != nil {
		return false, err
	}
	return true, nil
}

// errDeliveryNotWired is returned when a leg is asked to run with no channel.
var errDeliveryNotWired = errors.New("delivery leg has no channel")

// deliverOneLeg is the ONE door onto the unattended gates (ADR-009), and it is what makes the
// guard above non-vacuous while the round that calls it is still being built (S05g).
//
// `send` chooses the side: the convener sends, the recipient receives. Both directions are here
// because the gates are the same fact about the same leg, and splitting them would give the two
// auto types two doors.
func (s *Server) deliverOneLeg(ch p2p.Channel, cer *ceremonyID, myFP []byte, pdf []byte, send bool) ([]byte, error) {
	if ch.Stream == nil {
		return nil, errDeliveryNotWired
	}
	if send {
		// PeerGatesUnattended: the round armed the far side itself, so the sender's third
		// deadline waits on no human. See p2p.DeliveryLegBudget.
		return nil, p2p.SendDocument(ch, pdf, myFP, autoVerifier{}, p2p.PeerGatesUnattended)
	}
	var kept []byte
	doc, err := p2p.ReceiveDocument(ch, autoAccepter{
		verify: func(d []byte) error { return s.checkDelivered(cer, d) },
		save: func(d []byte) error {
			path, serr := s.saveDelivered(cer, d)
			if serr != nil {
				return serr
			}
			kept = d
			s.sess.setReceived(armDelivery, &receivedInfo{Path: path, Peer: cer.peer})
			return nil
		},
	}, myFP, autoVerifier{})
	if err != nil {
		return nil, err
	}
	if kept == nil {
		// Belt and braces: ReceiveDocument returns the document only on accept, and accept only
		// runs after `save`. A nil here would mean the two disagreed.
		return nil, errors.New("delivery accepted but nothing was kept")
	}
	return doc, nil
}

// checkDelivered is the delivery arrival's gate, and it is NOT `checkArrival` (P08.S05d).
//
// # Why a second door rather than a reuse
//
// `checkArrival` composes three of the same checks, and reusing it would refuse exactly the
// documents this round exists to deliver. Its `recordOutlivesBudget(rec, now, 0)` arm exists to
// stop a party **being collected into a proceeding that has ended** (P08.S04a, D28). A delivery is
// the opposite case: the proceeding HAS ended, correctly, and this is its result — and S05b
// reserved grace precisely so the round may run at or past `Expires`. A shared door would make the
// last party's copy undeliverable on a ceremony that finished on time.
//
// ADR-009 is satisfied by the two doors being two RULES, not one rule with two spellings, and each
// says so at the other. Anything later that "unifies" them reintroduces the refusal above.
//
// # What it checks
//
//  1. the record is present, well-formed and signed by the convener (`CheckRecord`);
//  2. it is the ceremony this party accepted (`MatchesRecord`, the same binding `checkArrival` uses);
//  3. every obliged signer actually signed — a partial document is not a delivery;
//  4. it extends this party's OWN contribution byte for byte, so a document that dropped this
//     party's signature and re-collected the others cannot land as their copy.
//
// # Its blind spots, stated
//
// It cannot tell a convener who delivered an older complete document from one who delivered the
// newest, because nothing here carries a version and the mirror holds one `document.pdf` per
// ceremony. It does not re-verify the OTHER parties' signatures cryptographically beyond what
// `Attestations` reports as valid. And (4) is a prefix test, so a trailing incremental update
// signed by anyone in the roster still extends it — which is what makes a co-signature legal at
// all, and is why (3) rather than (4) is what refuses an incomplete document. **And (4) is
// skipped entirely when this machine has no readable mirror** — a convener who did not sign, or
// a moved `~/nib` — because refusing there would make a delivery undeliverable for a local
// storage problem while (3) still holds.
func (s *Server) checkDelivered(cer *ceremonyID, pdf []byte) error {
	if cer == nil {
		return errors.New("a delivered document arrived for no ceremony")
	}
	now := time.Now()
	rec, err := ceremony.CheckRecord(pdf, now)
	if err != nil {
		return fmt.Errorf("this delivered document's ceremony record could not be checked: %w", err)
	}
	if err := cer.inv.MatchesRecord(rec); err != nil {
		return err // unwrapped, for checkArrival's reason: the sentence already names the axis
	}
	proc := ceremony.ProceedingOf(pdf, now)
	signed, obliged := p2p.Completeness(p2p.Attestations(sign.Verify(pdf), proc), proc)
	if obliged == 0 || signed != obliged {
		return fmt.Errorf("this delivered document carries %d of %d obliged signatures, so the "+
			"proceeding it claims to complete is not complete", signed, obliged)
	}
	// **The mirror is read directly, NOT through `persistedFor` (this slice's own review).** That
	// function's parameter is *"the inbound document this hop was offered"*, and it answers
	// "is what is stored an answer to THIS question" — `persistedFor(nil)` would mean the empty
	// question, which its `HasPrefix(pdf, nil)` arm answers `true` for anything. It would work
	// today by accident of that identity and stop working the moment the miss rules change.
	// The question here is simply *what did this machine sign*, which is the mirror.
	_, mine, merr := ceremony.ReadMirror(defaultOutputDir(), cer.inv.ID, now)
	if merr != nil {
		// **A miss, not a refusal, and the asymmetry is deliberate.** The commonest reason there
		// is no mirror is that this party is the CONVENER who did not sign, or a machine that
		// restarted with `~/nib` moved. Refusing on an unreadable mirror would make a delivery
		// undeliverable for a local storage problem, and clause 3 above has already established
		// that every obliged party signed — which is the property that matters. Recorded as a
		// blind spot rather than papered over.
		return nil
	}
	if len(mine) > 0 && !bytes.HasPrefix(pdf, mine) {
		return errors.New("this delivered document does not extend the copy this machine signed, " +
			"so it is not this proceeding's result as this party contributed to it")
	}
	return nil
}

// deliveredName is the delivered file's name, and it is DETERMINISTIC (P08.S05d).
//
// **It does not reuse `receivedName`.** That builder reads `time.Now()` inside itself at second
// granularity, so two documents from one peer inside a second collide and the second silently
// overwrites the first — measured at P08.S05a's first honest tier-6 run and filed as /pending 342.
// A delivery round hands one machine several documents in quick succession, which is exactly that
// window.
//
// The name carries the ceremony id, so two ceremonies never collide, and a human half derived from
// the record's own intent, so a finished lease is distinguishable from Monday's copy by name alone
// rather than by opening it. The id goes LAST and in full: a truncated id is a collision nobody
// can see, and the human half is the part a user scans.
func deliveredName(rec ceremony.Record) string {
	slug := labelSlug(rec.Intent)
	if len(slug) > 48 {
		slug = strings.TrimRight(slug[:48], "-")
	}
	if slug == "" {
		slug = "ceremony"
	}
	return slug + "-" + rec.ID + ".pdf"
}

// saveDelivered writes the delivered document under ~/nib, OUTSIDE ~/nib/ceremonies/.
//
// **Outside, and that is load-bearing rather than tidy.** The mirror holds one `document.pdf` per
// ceremony and `persistedFor` reads it as "this hop's previously co-signed output". A finished
// N-party document written there would be returned as a hop's re-delivery — handing a later
// party's completed document back to an earlier one as though it were that hop's own result.
//
// `signed/` because the document IS signed, which is where `receivedSubdir` already routes a signed
// arrival; the delivered file is an ordinary signed PDF as far as everything downstream is concerned.
func (s *Server) saveDelivered(cer *ceremonyID, pdf []byte) (string, error) {
	rec, err := ceremony.CheckRecord(pdf, time.Now())
	if err != nil {
		return "", fmt.Errorf("%w: %v", p2p.ErrNotStored, err)
	}
	path := filepath.Join(defaultOutputDir(), "signed", deliveredName(rec))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("%w: %v", p2p.ErrNotStored, err)
	}
	if err := atomicfile.WriteDurable(path, pdf, 0o600); err != nil {
		return "", fmt.Errorf("%w: %v", p2p.ErrNotStored, err)
	}
	return path, nil
}
