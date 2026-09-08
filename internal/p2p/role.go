package p2p

import (
	"errors"
	"fmt"
	"time"
)

// ── The dial declares what it is FOR, before either side commits to a gate set ──────────────
//
// # The defect this exists against (/pending 385)
//
// Which responder role runs was decided by the ARM, before the wire: `coSignExchange` was
// reachable only through `Receive`, and `ReceiveDocument` only from the delivery arm and the
// interactive listener. Nothing was read off the connection to choose between them.
//
// That is fine while a machine holds at most one kind of arm, and it stops being fine the moment
// one machine needs both. A party that has committed its contribution HAS a record, so the hop
// sweep skips it (`ceremonyarm.go`: `if st.State == ceremony.LoadOK { continue }`) and the
// delivery sweep arms it instead — and that arm auto-confirms and can never serve a stored
// contribution, because `ReceiveDocument` does not reach `coSignExchange`. The resumed hop then
// meets an arm that structurally cannot answer it.
//
// The party cannot choose the right arm either, and that is the part that settles the design:
// nothing is written between `persistContribution` and the frame reaching the initiator, so a
// party that died in that window is byte-identical on disk to one whose hop landed. Any scheme
// that makes the party GUESS is guessing on a fact it does not hold. So the guess is removed
// rather than resolved — the dialer says what it wants, and one arm serves both.
//
// # It is ADR-010's argument one level down
//
// That ADR added a transport byte because "a port without its transport is not an address", and
// announcement v3 added the hop for the same reason. A CONNECTION that cannot say what it is for
// is the same defect again, one layer in.
//
// # Zero is not a role, deliberately
//
// `RoleCoSign` is 1 and not 0. This repo has been caught twice by a meaningful zero — an unset
// `candidate.Source` spending another tier's share, and an unset `Announcement.Hop` announcing
// itself as the convener's own index — so a producer that forgets the field must fail closed
// rather than silently claim the commonest role. A zero byte is `roleUnset` and is refused by
// name.

// RoleDeadline bounds the role round trip.
//
// **It is a MACHINE round trip and it is armed anyway.** No human is in it, so the honest figure
// is small — but "small enough not to bother" is how `exchangeDeadline` ended up spanning two
// human waits once already, and an unarmed read here would inherit whatever deadline the dial
// happened to leave on the socket, which is a budget nobody chose. `DeliveryLegBudget` reserves
// this same constant, so the number the round reserves and the number the code arms cannot drift.
const RoleDeadline = 30 * time.Second

// Role is what a dialer is asking the far side to do on this connection.
type Role byte

const (
	// roleUnset is the zero value and is never valid on the wire. See the header.
	roleUnset Role = 0
	// RoleCoSign asks for the ceremony hop exchange: the responder contributes its signature
	// and hands the document back. Served by `Receive`.
	RoleCoSign Role = 1
	// RoleTransfer asks for the one-way document transfer: the responder keeps the bytes and
	// acknowledges. Served by `ReceiveDocument`.
	RoleTransfer Role = 2
)

func (r Role) String() string {
	switch r {
	case RoleCoSign:
		return "co-sign"
	case RoleTransfer:
		return "transfer"
	}
	return fmt.Sprintf("role(%d)", byte(r))
}

// valid reports whether r is a role this build serves. Fails closed on the zero value and on
// anything a future peer might send.
func (r Role) valid() bool { return r == RoleCoSign || r == RoleTransfer }

// ErrRoleRefused is the far side saying it does not serve the role this dial asked for.
//
// **It is a refusal and not a transport failure**, for `internal/p2p/verify.go`'s standing reason
// one gate over: reporting a decision as a network error invites a retry, and a retry is the wrong
// advice when the answer will not change. It carries a wire code so it survives the trip.
var ErrRoleRefused = errors.New("the other side is not listening for that kind of connection")

// ErrRoleUnreadable is a role frame that arrived malformed — not one byte, or a byte this build
// does not know. Distinct from ErrRoleRefused: one is a peer answering "not that", the other is a
// peer this build cannot understand at all, and folding them would tell a user their counterparty
// declined when the truth is a version skew.
//
// No wire code: it is raised by the side that could not READ the frame, about the frame it was
// just sent. Encoding it would mean answering a peer this build cannot parse with a two-byte
// refusal that peer has, by construction, given no evidence of being able to parse either — and
// the ALPN negotiation is what makes the case reachable at all, so the honest response is to stop
// rather than to reply in a dialect already in doubt.
var ErrRoleUnreadable = errors.New("the other side asked for something this version does not know")

// WriteRole declares this dial's role and waits for the far side to accept it.
//
// **A round trip, not a fire-and-forget byte, and the acknowledgement is the point.** A responder
// that simply closed on a role it does not serve would reach the initiator as a bare EOF — the
// `/pending 315` class this repo has now found at four sentinels, where a decision arrives looking
// like a dropped connection. The ack costs one round trip inside a deadline that is already armed,
// and it buys a sentence.
//
// Silent on a peer that did not negotiate the role frame, so an older build is not broken by a
// frame it would read as a document.
func WriteRole(ch Channel, r Role) error {
	if !ch.SpeaksRoleFrame() {
		return nil
	}
	if !r.valid() {
		return fmt.Errorf("%w: %s", ErrRoleUnreadable, r)
	}
	_ = ch.Stream.SetDeadline(time.Now().Add(RoleDeadline))
	if err := writeFrame(ch.Stream, []byte{byte(r)}); err != nil {
		return fmt.Errorf("declare role: %w", err)
	}
	// **Two bytes, not one, and the first cut capped it at one.** `RefuseRole` answers with the
	// NAMED form — `{ackRefused, code}` — so a one-byte cap turned every refusal into "declared
	// frame too large", which is a transport sentence about a decision: precisely the failure the
	// named refusal exists to prevent, reintroduced by the reader. Caught by this frame's own
	// test rather than by review.
	ack, err := readFrameMax(ch.Stream, 2)
	if err != nil {
		return fmt.Errorf("await role acknowledgement: %w", err)
	}
	if len(ack) == 1 && ack[0] == ackOK {
		return nil
	}
	// Decoded through the one door the receipt path already uses, so a role refusal and every
	// other named refusal are read by the same function rather than by two that must agree.
	if rerr, ok := refusalFor(ack, false); ok {
		return rerr
	}
	return ErrRoleRefused
}

// ReadRole reads the role a dialer is asking for. It does NOT acknowledge — the caller does, once
// it knows whether it will serve that role, through AcceptRole or RefuseRole.
//
// **Split from the acknowledgement deliberately.** Whether a role can be served is a decision
// about arms and gate sets, which is `internal/server`'s to make and not this package's; folding
// the ack in here would put that policy behind an API that cannot see it.
//
// A peer that did not negotiate the role frame sent none, and gets this build's historical
// behaviour: RoleCoSign, which is what every pre-role dial meant on an interactive arm.
func ReadRole(ch Channel) (Role, error) {
	if !ch.SpeaksRoleFrame() {
		return RoleCoSign, nil
	}
	_ = ch.Stream.SetDeadline(time.Now().Add(RoleDeadline))
	b, err := readFrameMax(ch.Stream, 1)
	if err != nil {
		return roleUnset, fmt.Errorf("read role: %w", err)
	}
	if len(b) != 1 {
		return roleUnset, ErrRoleUnreadable
	}
	r := Role(b[0])
	if !r.valid() {
		return roleUnset, fmt.Errorf("%w: %s", ErrRoleUnreadable, r)
	}
	return r, nil
}

// AcceptRole tells the dialer this side will serve the role it asked for.
func AcceptRole(ch Channel) error {
	if !ch.SpeaksRoleFrame() {
		return nil
	}
	if err := writeFrame(ch.Stream, []byte{ackOK}); err != nil {
		return fmt.Errorf("acknowledge role: %w", err)
	}
	return nil
}

// RefuseRole tells the dialer this side does not serve that role, and returns the sentinel the
// caller should report. Best-effort on the write for the reason every other refusal receipt is:
// the refusal stands whether or not the peer is still there to read it.
func RefuseRole(ch Channel) error {
	if ch.SpeaksRoleFrame() {
		_ = writeFrame(ch.Stream, []byte{ackRefused, refuseWrongRole})
	}
	return ErrRoleRefused
}
