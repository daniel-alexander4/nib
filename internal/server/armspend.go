package server

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"nib/internal/p2p"
)

// ── What spends an arm (/pending 766, ADR-009: one rule, one door) ───────────────────────────────
//
// An arm is a party waiting to be reached. One connection reaches it, something comes of that, and
// the arm either goes on waiting or is over. The rule:
//
//   - **A decision made by a party who got through ends the arm.** A decline, a consent or a spoken
//     check nobody answered, the words rejected at either end, a failed commitment check, a document
//     this machine's own gates refused. Asking again would re-ask a person who has answered, or retry
//     under whatever made the check fail.
//   - **A failure of the channel keeps it.** A dropped connection, a peer that asked for something
//     this arm does not serve or this build cannot read, a consent slot already in use: nobody
//     decided anything, so the party is still waiting for the session it armed for.
//   - **A clean completion ends it, unless the arm now owes a re-delivery** (a ceremony hop it has
//     signed, or answered from its stored contribution): then it stays reachable for the bounded
//     window each arm loop runs.
//   - **Anything nobody classified ends it, and says so in the log** — the safe direction, because
//     the worst case of ending is a user who arms again and the worst case of waiting on is a retry
//     under attack.
//
// The rule was written twice before this and the two copies disagreed. `runSession` (TCP) asked
// "did this connection put anything in front of the user" and `runCeremonyReceive` (QUIC) asked "is
// the error a lost channel". They agreed on a decline and on a channel lost before anything was
// shown; they disagreed on a failed commitment check (the check runs before any word is shown, so
// the TCP arm kept listening after it), on a refused role and a busy consent slot (QUIC ended), on
// a channel lost after the words were up (TCP ended), and on a decided failure inside the
// re-delivery window (TCP waited the window out). Both arm loops now ask `armSpentBy` and neither
// decides for itself; `TestBothArmsAskTheOneSpendRule` holds that.
//
// `armForDelivery` is the named exemption (ADR-009): it asks nobody anything on its delivery leg —
// the gates there are the unattended ones — and it is spent by exactly one thing, a leg that
// delivered. A hop it serves on the way never spends it (/pending 500). See the note at its loop.
// It still reads this table for what an outcome IS: `deliveryArmKept` says a failed tamper check
// and paces a failure, as the arms this rule ends do.

// armOutcome is one row of the rule: an outcome a served connection can end with, and whether the
// arm goes on waiting after it.
type armOutcome struct {
	// name is the p2p sentinel this row answers for, as its declaration spells it, or "" for a row
	// that is a predicate. `TestEveryP2PSentinelHasASpendRow` reads the sentinels from p2p's source
	// and fails on one with no row, so a new sentinel cannot reach the default unnoticed.
	name  string
	is    func(error) bool
	class string
	keeps bool
}

func sentinelRow(name string, sentinel error, class string, keeps bool) armOutcome {
	return armOutcome{name: name, is: func(err error) bool { return errors.Is(err, sentinel) }, class: class, keeps: keeps}
}

// The classes a notice or a test branches on. The rest are named where they are used.
const (
	spendCompleted    = "completed"
	spendRedelivers   = "completed-redelivers"
	spendLostChannel  = "lost-channel"
	spendUnanswered   = "unanswered"
	spendTamperCheck  = "tamper-check"
	spendUnclassified = "unclassified"
)

// armOutcomes is the rule's table. First match wins; the named sentinels come before the two
// predicates that read an error's shape, so a decision is never mistaken for a lost channel.
var armOutcomes = []armOutcome{
	// A person on this machine answered, or did not.
	sentinelRow("ErrCoSignDeclined", p2p.ErrCoSignDeclined, "declined", false),
	sentinelRow("ErrDeclined", p2p.ErrDeclined, "declined", false),
	sentinelRow("ErrConsentTimedOut", p2p.ErrConsentTimedOut, spendUnanswered, false),
	sentinelRow("ErrVerificationTimedOut", p2p.ErrVerificationTimedOut, spendUnanswered, false),
	sentinelRow("ErrVerificationDeclined", p2p.ErrVerificationDeclined, "words-rejected", false),
	sentinelRow("ErrPeerDidNotConfirm", p2p.ErrPeerDidNotConfirm, "words-rejected-by-peer", false),
	// This machine's gates refused what arrived.
	sentinelRow("ErrCeremonyEnded", p2p.ErrCeremonyEnded, "arrival-refused", false),
	sentinelRow("ErrRosterMismatch", p2p.ErrRosterMismatch, "arrival-refused", false),
	sentinelRow("ErrDocumentSubstituted", p2p.ErrDocumentSubstituted, "arrival-refused", false),
	sentinelRow("ErrCannotReadOwnRecord", p2p.ErrCannotReadOwnRecord, "arrival-refused", false),
	sentinelRow("ErrCeremonyRuleRetired", p2p.ErrCeremonyRuleRetired, "arrival-refused", false),
	sentinelRow("ErrCeremonyRuleTooNew", p2p.ErrCeremonyRuleTooNew, "arrival-refused", false),
	sentinelRow("ErrNotYourTurn", p2p.ErrNotYourTurn, "signing-refused", false),
	sentinelRow("ErrNotInRoster", p2p.ErrNotInRoster, "signing-refused", false),
	sentinelRow("ErrPrefixMismatch", p2p.ErrPrefixMismatch, "signing-refused", false),
	sentinelRow("ErrPrefixUnproven", p2p.ErrPrefixUnproven, "signing-refused", false),
	sentinelRow("ErrProceedingMismatch", p2p.ErrProceedingMismatch, "signing-refused", false),
	sentinelRow("ErrNoSignaturePages", p2p.ErrNoSignaturePages, "signing-refused", false),
	sentinelRow("ErrBlockOffThePage", p2p.ErrBlockOffThePage, "signing-refused", false),
	sentinelRow("ErrCeremonyComplete", p2p.ErrCeremonyComplete, "signing-refused", false),
	sentinelRow("ErrNoCeremonyIntent", p2p.ErrNoCeremonyIntent, "signing-refused", false),
	sentinelRow("ErrReadmeOverflow", p2p.ErrReadmeOverflow, "signing-refused", false),
	// The signature block could not be drawn for what is being signed (ADR-158): too many lines for
	// its frame, a character no shipped face has, or the faces themselves failed to load. Nothing
	// was signed, and asking again cannot change the answer.
	sentinelRow("ErrBlockTooTall", p2p.ErrBlockTooTall, "signing-refused", false),
	sentinelRow("ErrBlockFaces", p2p.ErrBlockFaces, "signing-refused", false),
	{is: func(err error) bool { var u *p2p.UndrawableError; return errors.As(err, &u) }, class: "signing-refused"},
	sentinelRow("ErrNotStored", p2p.ErrNotStored, "not-saved", false),
	// The far side's verdicts on what this side sent. A receiving arm sends nothing to be judged,
	// so these are the dialling side's; they have rows so that reaching one here is an end with a
	// name and not the default.
	sentinelRow("ErrNotTheConnectedPeer", p2p.ErrNotTheConnectedPeer, "refused-by-peer", false),
	sentinelRow("ErrPeerDoesNotAcceptYou", p2p.ErrPeerDoesNotAcceptYou, "refused-by-peer", false),
	sentinelRow("ErrWrongPriorSignerCount", p2p.ErrWrongPriorSignerCount, "refused-by-peer", false),
	sentinelRow("ErrRefusedUnknown", p2p.ErrRefusedUnknown, "refused-by-peer", false),
	sentinelRow("ErrPeerNotPinned", p2p.ErrPeerNotPinned, "refused-by-peer", false),
	// Nobody got through: the arm is still waiting for the session it was armed for.
	sentinelRow("ErrRoleRefused", p2p.ErrRoleRefused, "wrong-role", true),
	sentinelRow("ErrRoleUnreadable", p2p.ErrRoleUnreadable, "role-unreadable", true),
	sentinelRow("ErrConsentBusy", p2p.ErrConsentBusy, "busy", true),
	{is: func(err error) bool { return errors.Is(err, errVerifyBusy) }, class: "busy", keeps: true},
	// The arm this connection belonged to is already gone; there is nothing left to keep.
	{is: func(err error) bool { return errors.Is(err, errConsentNotArmed) }, class: "arm-gone"},
	// The commit-reveal check. Before the lost-channel row, though the two cannot overlap: a broken
	// commitment is never wrapped around a network error.
	{is: p2p.IsCommitmentFailure, class: spendTamperCheck},
	{is: isTransportLoss, class: spendLostChannel, keeps: true},
}

// spendClass is the rule as a pure function: what class of outcome err is, and whether the arm is
// spent by it. `redelivers` is whether the arm now owes a re-delivery, which matters only to a clean
// completion.
func spendClass(err error, redelivers bool) (class string, spent bool) {
	if err == nil {
		if redelivers {
			return spendRedelivers, false
		}
		return spendCompleted, true
	}
	for _, o := range armOutcomes {
		if o.is(err) {
			return o.class, !o.keeps
		}
	}
	return spendUnclassified, true
}

// armSpentBy reports whether the outcome of one served connection ends the arm `anchor` names. It
// is the ONE place that is decided: `runSession` and `runCeremonyReceive` both call it with what
// `serveOneSession` (or the role read before it) returned, and act on the answer.
//
// When the arm ends on something the user at this machine did not do themselves — nobody answered,
// or the tamper check failed — it leaves the reason on the status notice, in the same words whichever
// transport the arm was on. The arm has no response to write into, and by the time anybody looks the
// only symptom is that it is no longer waiting.
func (s *Server) armSpentBy(anchor consentAnchor, cer *ceremonyID, err error) bool {
	redelivers := cer != nil && (cer.hasSigned() || cer.servedReDelivery())
	class, spent := spendClass(err, redelivers)
	if !spent {
		return false
	}
	switch class {
	case spendUnanswered:
		s.sess.noteArmEnded(anchor, "arm-ended-unanswered",
			"Nobody answered here in time, so this computer stopped waiting.",
			fmt.Sprintf("The other side connected and a question was on this screen for %s with no "+
				"answer.%s Nothing was signed and nothing was saved. To carry on, start waiting again "+
				"and ask them to send it again.", consentWait(sessionConsentTimeout).Round(time.Second),
				whichSigning(cer)))
	case spendTamperCheck:
		s.sess.noteArmEnded(anchor, "arm-ended-tamper-check",
			"The connection failed a tamper check, so this computer stopped waiting.",
			"While setting up the safety words, the other side's numbers did not match what it had "+
				"promised to send. That is what a machine sitting between you and the other person "+
				"looks like, though a faulty program can cause it too. No words were shown and nothing "+
				"was signed or saved. Do not simply try again: check with the other person over a "+
				"channel you trust first, then start waiting again.")
	case spendUnclassified:
		// One line, naming what it was: an outcome with no row is a gap in the table above, and
		// this is the only place it can be seen.
		log.Printf("arm ended on an outcome the spend rule has no row for (%T): %v", err, err)
	}
	return true
}

// whichSigning names the ceremony an ended arm was waiting for, as a sentence to sit inside a
// notice, or "" for an arm with no ceremony (/pending 884). A machine resumes an arm for every
// ceremony it has accepted, so "this computer stopped waiting" does not say for which; the
// recital is the one name every party has for it. Cut at 80 characters: the notice is one line.
func whichSigning(cer *ceremonyID) string {
	if cer == nil {
		return ""
	}
	name := []rune(strings.TrimSpace(cer.inv.Intent))
	if len(name) == 0 {
		return ""
	}
	if len(name) > 80 {
		name = append(name[:80:80], '…')
	}
	return " It was for the signing \u201c" + string(name) + "\u201d."
}

// noteArmEnded records why an arm stopped waiting — or, for the delivery arm, what it survived
// (`deliveryArmKept`) — only while `anchor` still names an armed session. A consent gate torn down by Cancel or by a newer arm leaves through the same sentinel a
// timeout does (`sessionConfirmer.Confirm`), and telling that user "nobody answered" about a wait
// they ended themselves, or writing over the notice of the arm that replaced this one, would be a
// false statement on the one surface that outlives the arm.
func (se *session) noteArmEnded(anchor consentAnchor, what, summary, detail string) {
	se.mu.Lock()
	defer se.mu.Unlock()
	if !anchor.current(se) {
		return
	}
	se.notice = &noticeView{What: what, Summary: summary, Detail: detail, At: time.Now()}
	se.noticeBy = anchor.kind
}

// consentWaitForTest is consentWait's override, in nanoseconds; zero in every shipped run.
var consentWaitForTest atomic.Int64

// consentWait is how long a consent gate or the spoken check waits for an answer: `d`, which every
// caller passes as `sessionConsentTimeout`, unless a test has shortened it. The override is atomic
// because arm goroutines from one test can still be running when the next sets it.
func consentWait(d time.Duration) time.Duration {
	if o := consentWaitForTest.Load(); o > 0 {
		return time.Duration(o)
	}
	return d
}
