package server

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"nib/internal/p2p"
)

// What an arm says about the peers it turned away (/pending 669 #3, ADR-156).
//
// # The rule
//
// A refusal at the pinned-peer handshake never ends an arm and never spends it: the arm keeps
// listening and keeps trying its other candidates. What changed is that it is no longer SILENT. The
// cause is shown in the arm's status the moment it happens, and it is the reason given if the wait
// runs out with nobody through.
//
// # One count, one wording, three arms
//
// `p2p.Refusals` is counted where the check runs — inside the TLS config, which both transports
// share — so nothing here knows which transport an arm is on, and the sentence cannot differ between
// them. Each of the three arm goroutines (`runSession`, `runCeremonyReceive`, `armForDelivery`'s)
// names its count through `watchRefusals` and reports a spent wait through `noteWaitEnded`;
// `TestEveryArmNamesItsRefusals` holds that all three do both.
//
// # Why a stranger cannot spam it or hide anything with it
//
// The count is integers. The status carries ONE sentence chosen by `refusalCause`, and the clock
// reading — the only cause a user can fix, and the one a stranger cannot fake in its useful
// direction — outranks the rest, so a host hammering the port moves a number in the detail and never
// displaces it. Nothing is logged per refusal, and this is a field of its own: it does not write the
// sticky notice while the arm is alive, so it cannot overwrite a real failure either.

// turnedAwayView is the live "someone was turned away" line on the session status.
//
// Three fields and no counts of their own: the counts are in `Detail`, as words. `At` is the last
// refusal, so a reader can tell a sentence that is still happening from one that is an hour old.
type turnedAwayView struct {
	Summary string    `json:"summary"`
	Detail  string    `json:"detail,omitempty"`
	At      time.Time `json:"at"`
}

// watchRefusals names where the arm `a` anchors counts its refused peers. The ONE door (ADR-009):
// every arm goroutine calls it once, with its own listener's count.
func (se *session) watchRefusals(a consentAnchor, r *p2p.Refusals) {
	se.mu.Lock()
	defer se.mu.Unlock()
	if arm := a.armIn(se); arm != nil {
		arm.refused = r
	}
}

// refusalCause is the ONE wording of what was turned away: `cause` is the sentence, `detail` the
// counts and what they do and do not mean. ok is false when nothing was refused.
//
// **Ordered by what the person can act on, and by who can cause it.**
//
//  1. The clock. Reached only by a certificate the pinned identity signed, and it names a fix that
//     takes ten seconds.
//  2. The pinned peer turned THIS machine away. Only the pinned peer gets that far (this side checks
//     its certificate first). Its alert does not say whether it was the pin or the clock, so the
//     sentence says both.
//  3. Something else answered at an address this arm dialled.
//  4. Someone else connected here. ANYONE who can reach the port causes this, so it never says the
//     peer's identity is wrong — only that somebody who is not the pinned peer knocked.
func refusalCause(rep p2p.RefusalReport) (cause, detail string, ok bool) {
	if !rep.Any() {
		return "", "", false
	}
	switch {
	case rep.Clock > 0 && rep.Skew != nil:
		cause = "A connection carrying the pinned peer's identity was turned away over the date and " +
			"time: " + rep.Skew.Error() + "."
	case rep.RefusedUs > 0:
		cause = "The other side turned this computer away. Either it has a different identity pinned " +
			"for you, or its clock and this computer's disagree — ask them to check both."
	case rep.WrongAnswer > 0:
		cause = "Something answered at an address for the other side, and it was not the peer you " +
			"pinned. The address may be out of date, or they may be using a different identity."
	default:
		cause = "Someone other than the pinned peer connected to this computer and was turned away."
	}
	var parts []string
	count := func(n int, what string) {
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%s: %d", what, n))
		}
	}
	count(rep.Clock, "turned away over the date and time")
	count(rep.RefusedUs, "times the other side turned this computer away")
	count(rep.WrongAnswer, "answers that were not the pinned peer")
	count(rep.NotPinned, "connections here that were not the pinned peer")
	detail = strings.Join(parts, "; ") + "."
	if rep.NotPinned > 0 {
		detail += " Anything that can reach this computer's port can cause the last kind, so it " +
			"says nothing about the peer you pinned."
	}
	detail += " None of this ends the wait."
	return cause, detail, true
}

// turnedAwayOf is the status line for a count, nil when nothing was refused.
func turnedAwayOf(r *p2p.Refusals) *turnedAwayView {
	rep := r.Report()
	cause, detail, ok := refusalCause(rep)
	if !ok {
		return nil
	}
	return &turnedAwayView{Summary: "Still waiting. " + cause, Detail: detail, At: rep.Last}
}

// noteWaitEnded gives the refusals as the REASON when an arm's wait ran out with nobody through.
// The ONE door for that sentence; a wait that turned nobody away records nothing, as before.
//
// Sticky, where the status line above is live: by the time anybody reads it the arm is gone, and
// `noticeView` exists for exactly the failure whose symptom is the disarm.
func (se *session) noteWaitEnded(by armKind, r *p2p.Refusals) {
	cause, detail, ok := refusalCause(r.Report())
	if !ok {
		return
	}
	se.noteFailure(by, "wait-ended-turned-away",
		"The wait ended and nobody got through. "+cause, detail)
}

// dialRefusalSentence is the DIALLER's half of the same two facts: its own pin refused what
// answered, or the far side refused it. ok is false for every other error.
//
// The wording is `refusalCause`'s, said as an outcome instead of a wait, so the arm that turned a
// peer away and the dialler it turned away read as the same event.
func dialRefusalSentence(err error) (string, bool) {
	switch {
	case err == nil:
		return "", false
	case errors.Is(err, p2p.ErrPeerNotPinned):
		return "what answered at that address is not the peer you pinned — the address may be out " +
			"of date, or they may be using a different identity from the one you pinned", true
	case p2p.RefusedByPeer(err):
		return "the other side turned this computer away — either it has a different identity " +
			"pinned for you, or its clock and this computer's disagree; ask them to check both", true
	}
	return "", false
}
