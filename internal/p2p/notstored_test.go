package p2p

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// TestSpeaksNamedRefusalsIsAFloorNotAnEquality — /pending 338, and it is the arm nothing had.
//
// The predicate was `c.Proto == alpn2`. A third version added to `sessionALPN` would negotiate
// fine and then report FALSE, denying named refusals to the NEWEST peers — a silent downgrade in
// the one direction no test looked. `TestAnOlderPeerGetsTheBehaviourItExpects` enumerates
// `{alpn, "", "h2"}` and asserts each does NOT speak, so a newer protocol falling into that class
// reads to it as correct.
//
// The stimulus is the prepend itself: against the old equality this test fails on the very
// assertion it exists for, because "nib/3" is not alpn2.
func TestSpeaksNamedRefusalsIsAFloorNotAnEquality(t *testing.T) {
	const future = "nib/3"

	// SETUP: the future version must genuinely be unknown right now, or "it speaks after the
	// prepend" is true for the wrong reason.
	if (Channel{Proto: future}).SpeaksNamedRefusals() {
		t.Fatalf("setup: %q already speaks named refusals before it was offered, so the "+
			"prepend below proves nothing", future)
	}
	if protoRank(future) != 0 {
		t.Fatalf("setup: %q ranks %d, want 0 — it is supposed to be a version this build has "+
			"never heard of", future, protoRank(future))
	}

	saved := sessionALPN
	sessionALPN = append([]string{future}, saved...)
	t.Cleanup(func() { sessionALPN = saved })

	if !(Channel{Proto: future}).SpeaksNamedRefusals() {
		t.Errorf("a peer negotiating %q — a version NEWER than %q — was told it cannot read a "+
			"named refusal. The predicate is an equality against %q rather than a floor, so the "+
			"next ALPN bump silently downgrades the newest peers and the older-peer guard blesses "+
			"it, because %q is not in the list of protocols that test enumerates.",
			future, alpn2, alpn2, future)
	}

	// And the floor still refuses everything below it — a floor that admits everything is not a
	// fix, it is the opposite defect.
	for _, old := range []string{alpn, "", "h2"} {
		if (Channel{Proto: old}).SpeaksNamedRefusals() {
			t.Errorf("with %q offered, a peer negotiating %q now claims to speak named refusals; "+
				"the floor has become a pass-through", future, old)
		}
	}
}

// TestAnUnknownOneByteReceiptIsNotAnAccusation is why `ackNotStored` needs no ALPN gate.
//
// A peer that predates the byte decodes it through `refusalFor`'s one-byte `default`, which
// declines to name it — and `SendDocument` then says "unexpected receipt from peer". That is
// uninformative and it is NOT a tampering verdict, which is the whole of what D32 forbids a
// version difference from producing.
//
// A two-byte named refusal could not be used here for an unrelated reason the second half
// asserts: `SendDocument` reads its receipt with `readFrameMax(conn, 1)`.
func TestAnUnknownOneByteReceiptIsNotAnAccusation(t *testing.T) {
	// SETUP: a byte this build DOES know decodes, or "the unknown one does not" is vacuous.
	if _, ok := refusalFor([]byte{ackNotStored}, false); !ok {
		t.Fatal("setup: this build cannot decode ackNotStored, so the unknown-code arm below " +
			"is measuring a decoder that recognises nothing")
	}

	// An older build's view: a one-byte code outside its switch.
	err, ok := refusalFor([]byte{99}, false)
	if ok || err != nil {
		t.Errorf("an unrecognised one-byte receipt decoded as %v (ok=%v); it must fall to the "+
			"default so the caller says 'unexpected receipt' rather than naming a refusal the "+
			"peer did not send", err, ok)
	}

	// The sentence that reaches the user for that case must carry no tampering vocabulary. This
	// is the string `SendDocument` returns when `refusalFor` declines to name the frame.
	const sentence = "unexpected receipt from peer"
	for _, banned := range []string{"not the one sent", "tamper", "replay", "forged", "modified"} {
		if strings.Contains(sentence, banned) {
			t.Errorf("the fallback sentence %q contains %q — a version difference must not be "+
				"reported as tampering (D32)", sentence, banned)
		}
	}

	// The other half: two bytes cannot be read on this path at all, which is why the new receipt
	// is one byte rather than a named refusal.
	twoByteFrame := bytes.NewReader([]byte{0, 0, 0, 2, ackRefused, refuseNotYourTurn})
	if _, err := readFrameMax(twoByteFrame, 1); err == nil {
		t.Error("a two-byte frame was accepted by the one-byte receipt reader; if that ever " +
			"becomes true, ackNotStored's one-byte argument needs re-deriving")
	}
}

// TestNotStoredIsItsOwnSentinel — the receipt must not collapse into the decline it is not.
func TestNotStoredIsItsOwnSentinel(t *testing.T) {
	if errors.Is(ErrNotStored, ErrDeclined) || errors.Is(ErrDeclined, ErrNotStored) {
		t.Error("ErrNotStored and ErrDeclined are the same sentinel; a disk failure would be " +
			"reported to the sender as the receiving user refusing the document")
	}
	frame, ok := refusalAck(ErrNotStored, false)
	if !ok || len(frame) != 1 || frame[0] != ackNotStored {
		t.Fatalf("refusalAck(ErrNotStored) produced %v (ok=%v); want the single byte %d",
			frame, ok, ackNotStored)
	}
	// Unconditional: `named` is false here, and it must still produce the frame. Gating it would
	// send nothing to an older peer, which is an EOF — the shape `ackDeclined` exists to avoid.
	got, ok := refusalFor(frame, false)
	if !ok || !errors.Is(got, ErrNotStored) {
		t.Errorf("the byte decoded back to %v (ok=%v); want ErrNotStored", got, ok)
	}
}

// TestAnUnknownProtocolCannotBeGrantedTheCapability — the fail-OPEN the floor introduced.
//
// The floor looks its own threshold up in `sessionALPN`. If `alpn2` ever leaves that list, the
// threshold is 0 and `rank >= 0` is true for EVERY peer — including one negotiating nothing —
// which would send two-byte named refusals to a `nib/1` peer and make it print a tampering verdict
// caused by a version skew. The equality this replaced degraded the opposite way, so the direction
// had to be restored deliberately rather than inherited.
func TestAnUnknownProtocolCannotBeGrantedTheCapability(t *testing.T) {
	saved := sessionALPN
	t.Cleanup(func() { sessionALPN = saved })

	// SETUP: with the real list, a v2 peer DOES speak — or the assertions below pass against a
	// predicate that refuses everything.
	if !(Channel{Proto: alpn2}).SpeaksNamedRefusals() {
		t.Fatal("setup: a v2 channel does not speak named refusals with the shipped offer list")
	}

	// The degenerate: alpn2 is gone from the list, so the threshold itself ranks 0.
	sessionALPN = []string{alpn}
	for _, proto := range []string{"", alpn, alpn2, "h2", "nib/9"} {
		if (Channel{Proto: proto}).SpeaksNamedRefusals() {
			t.Errorf("with %q absent from the offer list, a peer negotiating %q was GRANTED named "+
				"refusals. The floor is looked up in the list, so a missing threshold ranks 0 and "+
				"admits everyone — and a two-byte refusal to a v1 peer is read as a document "+
				"mismatch, which is a tampering verdict produced by a version skew (D32).",
				alpn2, proto)
		}
	}
}

// TestTheOfferListIsOrderedNewestFirst pins the invariant `protoRank` reads position as.
//
// It derives newness from index, so a version APPENDED rather than prepended ranks BELOW alpn2 and
// is denied the capability it introduced — the defect this ranking exists to fix, returning by the
// back door. The instinct to append is actively encouraged by the refusal-code block next door
// ("append only, never renumber"), which is why this is an assertion and not a comment.
func TestTheOfferListIsOrderedNewestFirst(t *testing.T) {
	if len(sessionALPN) < 2 {
		t.Fatalf("setup: the offer list holds %d entry(s); ordering cannot be checked",
			len(sessionALPN))
	}
	// The shipped list, oldest last. Any new version belongs at the FRONT.
	if sessionALPN[len(sessionALPN)-1] != alpn {
		t.Errorf("the last entry of sessionALPN is %q, want the oldest (%q). protoRank reads "+
			"position as age, so an entry out of order silently mis-ranks every capability floor.",
			sessionALPN[len(sessionALPN)-1], alpn)
	}
	for i := 1; i < len(sessionALPN); i++ {
		if protoRank(sessionALPN[i-1]) <= protoRank(sessionALPN[i]) {
			t.Errorf("sessionALPN[%d]=%q does not rank above sessionALPN[%d]=%q", i-1,
				sessionALPN[i-1], i, sessionALPN[i])
		}
	}
}

// TestACoSignInitiatorIgnoresTheNotStoredByte — finding 8 of the slice review.
//
// No co-sign path can emit it: an unstorable co-signature is still delivered, because the peer
// needs it and the signer keeps the document open. So byte 5 arriving at `Initiate` is a hostile
// or buggy peer choosing a sentence that is FALSE about this product's contract, and the decoder
// must decline to name it rather than print it.
func TestACoSignInitiatorIgnoresTheNotStoredByte(t *testing.T) {
	// SETUP: the transfer direction DOES name it, or "the co-sign direction does not" is vacuous.
	if _, ok := refusalFor([]byte{ackNotStored}, false); !ok {
		t.Fatal("setup: the transfer direction no longer decodes ackNotStored")
	}
	if err, ok := refusalFor([]byte{ackNotStored}, true); ok || err != nil {
		t.Errorf("a co-sign initiator decoded byte %d as %v; a peer could make this build tell "+
			"its user the far side 'could not save it' about a flow where an unsaved signature is "+
			"delivered anyway", ackNotStored, err)
	}
}
