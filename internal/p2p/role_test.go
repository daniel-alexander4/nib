package p2p

import (
	"errors"
	"net"
	"testing"
)

// pipeChannels returns two Channels over one in-memory connection, both negotiating `proto`.
func pipeChannels(t *testing.T, proto string) (dialer, responder Channel) {
	t.Helper()
	a, b := net.Pipe()
	t.Cleanup(func() { a.Close(); b.Close() })
	fp := []byte("0123456789abcdef0123456789abcdef")
	return Channel{Stream: a, PeerFP: fp, Export: nil, Proto: proto},
		Channel{Stream: b, PeerFP: fp, Export: nil, Proto: proto}
}

// TestTheDialDeclaresItsRoleAndTheAnswerComesBack — ADR-028, the happy path and its acknowledgement.
//
// The ack is the half worth asserting: a responder that simply closed on a role it does not serve
// would reach the initiator as a bare EOF, which is the class this repo has found at four
// sentinels — a decision arriving dressed as a dropped connection.
func TestTheDialDeclaresItsRoleAndTheAnswerComesBack(t *testing.T) {
	for _, want := range []Role{RoleCoSign, RoleTransfer} {
		t.Run(want.String(), func(t *testing.T) {
			d, r := pipeChannels(t, alpn3)
			done := make(chan error, 1)
			go func() { done <- WriteRole(d, want) }()

			got, err := ReadRole(r)
			if err != nil {
				t.Fatalf("ReadRole: %v", err)
			}
			if got != want {
				t.Errorf("the responder read role %s, want %s — the dial's declaration is what "+
					"chooses the gate set, so reading the wrong one runs the wrong exchange", got, want)
			}
			if err := AcceptRole(r); err != nil {
				t.Fatalf("AcceptRole: %v", err)
			}
			if err := <-done; err != nil {
				t.Errorf("the dialer's WriteRole returned %v after the responder accepted", err)
			}
		})
	}
}

// TestARefusedRoleIsAnAnswerAndNotADroppedConnection — ADR-028's reason for the round trip.
func TestARefusedRoleIsAnAnswerAndNotADroppedConnection(t *testing.T) {
	d, r := pipeChannels(t, alpn3)
	done := make(chan error, 1)
	go func() { done <- WriteRole(d, RoleTransfer) }()

	if _, err := ReadRole(r); err != nil {
		t.Fatalf("setup: ReadRole: %v", err)
	}
	if err := RefuseRole(r); !errors.Is(err, ErrRoleRefused) {
		t.Errorf("RefuseRole returned %v, want ErrRoleRefused — the refusing side must report the "+
			"same sentinel it puts on the wire", err)
	}
	err := <-done
	if !errors.Is(err, ErrRoleRefused) {
		t.Errorf("the dialer saw %v, want ErrRoleRefused. A refusal reported as a transport error "+
			"invites a retry, and a retry is the wrong advice when the answer will not change — "+
			"the argument verify.go makes for ErrVerificationDeclined, one gate over", err)
	}
	// And it survives the wire as a CODE, so the far side's sentence is this build's own.
	if code := refusalCode(ErrRoleRefused); code != refuseWrongRole {
		t.Errorf("ErrRoleRefused encodes as %d, want %d — without a code it crosses as a bare EOF",
			code, refuseWrongRole)
	}
	if got := errorForCode(refuseWrongRole); !errors.Is(got, ErrRoleRefused) {
		t.Errorf("code %d decodes to %v, want ErrRoleRefused — the round trip must close or the "+
			"refusing side names one thing and the initiator prints another", refuseWrongRole, got)
	}
}

// TestAPeerThatDidNotNegotiateTheRoleFrameExchangesNothing — the compatibility half, and the one
// that would desynchronise the exchange rather than merely degrade it.
//
// **A role frame is a frame in BOTH directions**, unlike a named refusal, which an older peer can
// simply never be sent. Writing one to a peer that will not read it leaves a byte in front of the
// verification exchange, and the far side reads it as the start of something else — a version skew
// rendered as a verdict about the counterparty, which is what D32 forbids.
func TestAPeerThatDidNotNegotiateTheRoleFrameExchangesNothing(t *testing.T) {
	for _, proto := range []string{alpn2, alpn, "", "h2"} {
		name := proto
		if name == "" {
			name = "(none)"
		}
		t.Run(name, func(t *testing.T) {
			d, r := pipeChannels(t, proto)
			if (Channel{Proto: proto}).SpeaksRoleFrame() {
				t.Fatalf("setup: %q reports that it speaks the role frame, so the assertions "+
					"below are about the wrong case", proto)
			}
			// Nothing is written and nothing is read: net.Pipe is UNBUFFERED, so a write with no
			// reader blocks forever. That is what makes this assertion real rather than nominal —
			// if WriteRole put a byte on the wire here, this test would hang rather than pass.
			if err := WriteRole(d, RoleCoSign); err != nil {
				t.Errorf("WriteRole to a pre-role peer returned %v, want nil and no frame", err)
			}
			got, err := ReadRole(r)
			if err != nil {
				t.Errorf("ReadRole from a pre-role peer returned %v, want no read at all", err)
			}
			if got != RoleCoSign {
				t.Errorf("a pre-role peer read as %s, want %s — every dial that predates the "+
					"frame meant a co-sign, and defaulting to anything else would silently change "+
					"what those peers are asking for", got, RoleCoSign)
			}
			if err := AcceptRole(r); err != nil {
				t.Errorf("AcceptRole to a pre-role peer returned %v, want nil and no frame", err)
			}
		})
	}
}

// TestRoleZeroIsNotARole — the meaningful-zero trap, twice paid for in this repo.
//
// `candidate.Source` unset spent another tier's share; `Announcement.Hop` unset announced itself
// as the convener's own index. A producer that forgets this field must fail closed rather than
// silently claim the commonest role.
func TestRoleZeroIsNotARole(t *testing.T) {
	if roleUnset.valid() {
		t.Error("the zero Role is valid, so a producer that never set the field declares a real " +
			"role by omission — the shape that cost /pending 385's sibling two measurements")
	}
	if !RoleCoSign.valid() || !RoleTransfer.valid() {
		t.Fatal("setup: a real role is reported invalid, so the assertion above proves nothing")
	}
	if byte(RoleCoSign) == 0 {
		t.Error("RoleCoSign is the zero byte, which is exactly what makes an unset field " +
			"indistinguishable from the commonest declaration")
	}

	// A byte this build does not know is unreadable, NOT a default. It fails closed on the read.
	d, r := pipeChannels(t, alpn3)
	go func() { _ = writeFrame(d.Stream, []byte{99}) }()
	if _, err := ReadRole(r); !errors.Is(err, ErrRoleUnreadable) {
		t.Errorf("a role byte of 99 read as %v, want ErrRoleUnreadable — an unknown role must not "+
			"fall through to a default, which is how a future peer would silently get the wrong "+
			"exchange", err)
	}
	// And the zero byte specifically, since that is the one a forgetful producer sends.
	d2, r2 := pipeChannels(t, alpn3)
	go func() { _ = writeFrame(d2.Stream, []byte{0}) }()
	if _, err := ReadRole(r2); !errors.Is(err, ErrRoleUnreadable) {
		t.Errorf("a role byte of 0 read as %v, want ErrRoleUnreadable", err)
	}
}

// TestSpeaksRoleFrameIsAFloorThatFailsClosed — the same property SpeaksNamedRefusals records, and
// the reason it is asserted separately: that predicate's own doc says a third version would expose
// an equality, and alpn3 IS the third version.
func TestSpeaksRoleFrameIsAFloorThatFailsClosed(t *testing.T) {
	if !(Channel{Proto: alpn3}).SpeaksRoleFrame() {
		t.Fatal("setup: the version that introduced the frame does not speak it")
	}
	for _, older := range []string{alpn2, alpn, "", "h2"} {
		if (Channel{Proto: older}).SpeaksRoleFrame() {
			t.Errorf("a peer negotiating %q speaks the role frame; it predates it, and sending "+
				"one desynchronises its exchange", older)
		}
	}
	// A NEWER peer must speak it — the equality defect, in the direction nothing checks.
	const future = "nib/99"
	saved := sessionALPN
	sessionALPN = append([]string{future}, saved...)
	t.Cleanup(func() { sessionALPN = saved })
	if !(Channel{Proto: future}).SpeaksRoleFrame() {
		t.Errorf("a peer negotiating %q — NEWER than %q — was told it cannot read a role frame. "+
			"The predicate is an equality rather than a floor, so the newest peers are denied the "+
			"capability, which is the defect SpeaksNamedRefusals' doc warned a third version "+
			"would expose", future, alpn3)
	}
	// And it fails CLOSED if alpn3 ever leaves the offer list: an unrankable threshold must mean
	// "nobody speaks it", never "everybody does".
	sessionALPN = []string{alpn}
	if (Channel{Proto: alpn}).SpeaksRoleFrame() {
		t.Error("with alpn3 no longer offered, a peer still reports that it speaks the role " +
			"frame. The floor must fail closed — otherwise retiring the version sends frames to " +
			"peers that cannot read them, which is worse than the skew it was minted to avoid")
	}
}
