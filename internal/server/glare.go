package server

import "bytes"

// glareKeepsDial resolves glare: when both parties dial AND listen over the one shared endpoint
// (P05.S09), each may complete TWO connections to the other — one it dialed, one it accepted — and
// both ends must converge on the SAME physical connection or the document runs over two half-open
// channels. The rule is a single comparison of the two identity fingerprints, not a per-channel
// decision: the surviving connection is the one DIALED BY THE PARTY WITH THE LOWER FINGERPRINT.
//
// It converges because the two ends compute it from opposite viewpoints and reach the same
// connection. Say A's fingerprint sorts below B's. A computes glareKeepsDial(aFP, bFP) = true and
// keeps the connection A DIALED (A→B). B computes glareKeepsDial(bFP, aFP) = false and keeps the
// connection it ACCEPTED — which is that same A→B. Both drop the B→A connection. One channel
// survives on both ends, chosen without any exchange beyond the fingerprints already in hand from
// the handshake (glare tie-break, D17).
//
// Equal fingerprints cannot occur between two distinct pinned identities (the fingerprint is over
// the identity key); the comparison returns false there, which is harmless and deterministic.
func glareKeepsDial(myFP, peerFP []byte) bool {
	return bytes.Compare(myFP, peerFP) < 0
}

// glareChoice is what connect does next, given which of the two racing connections have formed.
type glareChoice int

const (
	glareWait   glareChoice = iota // neither the survivor nor a usable fallback is decided yet
	glareDial                      // keep the connection this side DIALED
	glareAccept                    // keep the connection this side ACCEPTED
	glareFail                      // both attempts are done and neither connected
)

// glareDecide resolves the join. keepDial is glareKeepsDial(myFP, peerFP) — whether THIS side's
// dialed connection is the designated survivor. The survivor is deterministic and both ends agree
// on it, so the rule is: take the survivor the moment it forms; otherwise, once the settle window
// has expired or both attempts are done, fall back to whichever connection DID form (both ends
// fall back to the same one, because it is the same physical connection); fail only when neither
// formed. Pure, so the whole table is unit-tested without a network.
func glareDecide(keepDial, haveDial, haveAccept, settleExpired, bothDone bool) glareChoice {
	survivorHere := (keepDial && haveDial) || (!keepDial && haveAccept)
	if survivorHere {
		if keepDial {
			return glareDial
		}
		return glareAccept
	}
	// The survivor has not formed. Keep waiting for it unless time is up.
	if !settleExpired && !bothDone {
		return glareWait
	}
	// Time is up: fall back to the connection that did form, if any.
	if haveDial {
		return glareDial
	}
	if haveAccept {
		return glareAccept
	}
	return glareFail
}
